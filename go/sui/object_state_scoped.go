package sui

import (
	"context"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/k4k3ru-hub/onchain/go/sui/internal/rpcv2"
	"google.golang.org/grpc"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/fieldmaskpb"
)

// ObjectStateScope includes the subscribed object and fields owned by ParentIDs.
// Parents must include every dynamic-field container needed to reconstruct state.
type ObjectStateScope struct{ ParentIDs []Address }

type ScopedObjectStateSubscriber interface {
	SubscribeObjectStateScoped(context.Context, Address, ObjectStateScope) (*TransactionSubscription, error)
}

const scopedObjectConcurrency = 4
const scopedObjectBatchSize = 32
const scopedObjectLimit = 512

// SubscribeObjectStateScoped streams ordered changes for one object and selected parents.
// It reads lightweight effects, then fetches exact required versions in the background.
// Same-owner replacements carry a Before reference (Move is nil) and full After.
// Deletions and ownership changes also fetch the full Before payload.
// At most four notifications and four batches are in flight; batches contain at most
// 32 objects. More than 512 selected versions in one transaction is an error.
// Missing versions, digest conflicts and ownership ambiguity terminate the stream.
// The caller must rebuild the subscription if dynamic-field parent IDs change.
//
// Parameters:
//   - ctx: Subscription lifetime.
//   - object: Affected object selecting upstream transactions.
//   - scope: Dynamic-field parents; copied before subscription.
//
// Returns:
//   - Selected ObjectChanges, transaction metadata and ordered progress notifications.
//
// Version:
//   - 2026-10-12: Added.
func (c *GRPCClient) SubscribeObjectStateScoped(ctx context.Context, object Address, scope ObjectStateScope) (*TransactionSubscription, error) {
	const op = "failed to subscribe scoped sui object state"
	if c == nil || ctx == nil {
		return nil, fmt.Errorf("%s: dependency=null", op)
	}
	if object.IsZero() {
		return nil, fmt.Errorf("%s: object=empty", op)
	}
	if len(scope.ParentIDs) > 16 {
		return nil, fmt.Errorf("%s: parents=too_long max_length=16", op)
	}
	parents := make(map[Address]bool, len(scope.ParentIDs))
	for _, parent := range scope.ParentIDs {
		if parent.IsZero() || parents[parent] {
			return nil, fmt.Errorf("%s: parents=invalid", op)
		}
		parents[parent] = true
	}
	provider, ok := c.transactionProvider.(interface {
		subscribeObjectStateScoped(context.Context, Address, map[Address]bool) (liveTransactionReceiver, error)
	})
	if !ok {
		return nil, fmt.Errorf("%s: provider=unsupported", op)
	}
	receiver, err := provider.subscribeObjectStateScoped(ctx, object, parents)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}
	sub, err := NewTransactionSubscription(receiver)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}
	return sub, nil
}

type scopedObjectReader interface {
	GetObject(context.Context, *rpcv2.GetObjectRequest, ...grpc.CallOption) (*rpcv2.GetObjectResponse, error)
	BatchGetObjects(context.Context, *rpcv2.BatchGetObjectsRequest, ...grpc.CallOption) (*rpcv2.BatchGetObjectsResponse, error)
}
type scopedObjectWire interface {
	Recv() (*rpcv2.SubscribeTransactionsResponse, error)
}
type scopedObjectResult struct {
	value *rpcv2.SubscribeTransactionsResponse
	err   error
}
type scopedObjectStream struct {
	ctx     context.Context
	pending chan chan scopedObjectResult
	slots   chan struct{}
}

func (a *grpcAdapter) subscribeObjectStateScoped(ctx context.Context, object Address, parents map[Address]bool) (liveTransactionReceiver, error) {
	if a.client == nil || a.ledgerClient == nil {
		return nil, fmt.Errorf("failed to compose scoped sui state: transport=null")
	}
	ctx, cancel := context.WithCancel(ctx)
	address := object.String()
	request := &rpcv2.SubscribeTransactionsRequest{
		ReadMask: &fieldmaskpb.FieldMask{Paths: []string{"digest", "effects.status", "effects.changed_objects", "checkpoint", "transaction_index", "timestamp"}},
		Filter:   &rpcv2.TransactionFilter{Terms: []*rpcv2.TransactionTerm{{Literals: []*rpcv2.TransactionLiteral{{Predicate: &rpcv2.TransactionLiteral_AffectedObject{AffectedObject: &rpcv2.AffectedObjectFilter{ObjectId: &address}}}}}}},
	}
	wire, err := a.client.SubscribeTransactions(ctx, request)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("failed to subscribe scoped sui metadata: %w", err)
	}
	if wire == nil {
		cancel()
		return nil, fmt.Errorf("failed to subscribe scoped sui metadata: stream=null")
	}
	stream := newScopedObjectStream(ctx, wire, a.ledgerClient, object, parents)
	return &grpcTransactionReceiver{stream: stream, cancel: cancel, includeObjects: true, referenceBefore: true}, nil
}

func newScopedObjectStream(ctx context.Context, wire scopedObjectWire, reader scopedObjectReader, object Address, parents map[Address]bool) *scopedObjectStream {
	s := &scopedObjectStream{ctx: ctx, pending: make(chan chan scopedObjectResult, scopedObjectConcurrency), slots: make(chan struct{}, scopedObjectConcurrency)}
	go func() {
		defer close(s.pending)
		var workers sync.WaitGroup
		defer workers.Wait()
		for {
			select {
			case s.slots <- struct{}{}:
			case <-ctx.Done():
				return
			}
			r, err := wire.Recv()
			result := make(chan scopedObjectResult, 1)
			select {
			case s.pending <- result:
			case <-ctx.Done():
				return
			}
			if err != nil {
				result <- scopedObjectResult{err: err}
				return
			}
			workers.Add(1)
			go func() {
				defer workers.Done()
				value, err := hydrateScopedObjects(ctx, reader, object, parents, r)
				result <- scopedObjectResult{value, err}
			}()
		}
	}()
	return s
}

// Recv releases results in upstream order, including progress-only frames.
//
// Version:
//   - 2026-10-12: Added.
func (s *scopedObjectStream) Recv() (*rpcv2.SubscribeTransactionsResponse, error) {
	select {
	case <-s.ctx.Done():
		return nil, s.ctx.Err()
	case result, ok := <-s.pending:
		if !ok {
			return nil, io.EOF
		}
		select {
		case <-s.ctx.Done():
			return nil, s.ctx.Err()
		case r := <-result:
			<-s.slots
			return r.value, r.err
		}
	}
}

func hydrateScopedObjects(ctx context.Context, reader scopedObjectReader, pool Address, parents map[Address]bool, response *rpcv2.SubscribeTransactionsResponse) (*rpcv2.SubscribeTransactionsResponse, error) {
	const op = "failed to acquire scoped sui objects"
	if response == nil || response.Watermark == nil || len(response.Watermark.Cursor) == 0 {
		return nil, fmt.Errorf("%s: watermark=invalid", op)
	}
	if response.Transaction == nil {
		return response, nil
	}
	if response.Transaction.Effects == nil {
		return nil, fmt.Errorf("%s: effects=null", op)
	}
	response = proto.Clone(response).(*rpcv2.SubscribeTransactionsResponse)
	tx := response.Transaction
	selected := []*rpcv2.ChangedObject{}
	type key struct {
		address string
		version uint64
	}
	expected := map[key]string{}
	requests := []*rpcv2.GetObjectRequest{}
	add := func(id string, version uint64, digest string) error {
		if version == 0 || digest == "" {
			return fmt.Errorf("%s: object_reference=invalid", op)
		}
		k := key{id, version}
		if old, ok := expected[k]; ok {
			if old != digest {
				return fmt.Errorf("%s: object_digest=conflict", op)
			}
			return nil
		}
		if len(requests) >= scopedObjectLimit {
			return fmt.Errorf("%s: objects=too_long max_length=%d", op, scopedObjectLimit)
		}
		expected[k] = digest
		requests = append(requests, &rpcv2.GetObjectRequest{ObjectId: proto.String(id), Version: proto.Uint64(version)})
		return nil
	}
	for _, change := range tx.Effects.ChangedObjects {
		if change == nil {
			return nil, fmt.Errorf("%s: change=null", op)
		}
		if change.GetOutputState() == rpcv2.ChangedObject_OUTPUT_OBJECT_STATE_ACCUMULATOR_WRITE {
			continue
		}
		id, err := ParseAddress(change.GetObjectId())
		if err != nil {
			return nil, fmt.Errorf("%s: %w", op, err)
		}
		included := id == pool
		if change.GetInputState() != rpcv2.ChangedObject_INPUT_OBJECT_STATE_EXISTS && change.GetInputState() != rpcv2.ChangedObject_INPUT_OBJECT_STATE_DOES_NOT_EXIST {
			return nil, fmt.Errorf("%s: input_state=invalid", op)
		}
		if change.GetInputState() == rpcv2.ChangedObject_INPUT_OBJECT_STATE_EXISTS && change.InputOwner == nil {
			return nil, fmt.Errorf("%s: input_owner=null", op)
		}
		output := change.GetOutputState()
		if (output == rpcv2.ChangedObject_OUTPUT_OBJECT_STATE_OBJECT_WRITE || output == rpcv2.ChangedObject_OUTPUT_OBJECT_STATE_PACKAGE_WRITE) && change.OutputOwner == nil {
			return nil, fmt.Errorf("%s: output_owner=null", op)
		}
		if output != rpcv2.ChangedObject_OUTPUT_OBJECT_STATE_OBJECT_WRITE && output != rpcv2.ChangedObject_OUTPUT_OBJECT_STATE_PACKAGE_WRITE && output != rpcv2.ChangedObject_OUTPUT_OBJECT_STATE_DOES_NOT_EXIST {
			return nil, fmt.Errorf("%s: output_state=invalid", op)
		}
		for _, owner := range []*rpcv2.Owner{change.InputOwner, change.OutputOwner} {
			if owner != nil {
				switch owner.GetKind() {
				case rpcv2.Owner_ADDRESS, rpcv2.Owner_OBJECT, rpcv2.Owner_SHARED, rpcv2.Owner_IMMUTABLE, rpcv2.Owner_CONSENSUS_ADDRESS:
				default:
					return nil, fmt.Errorf("%s: owner_kind=invalid", op)
				}
			}
			if owner != nil && owner.GetKind() == rpcv2.Owner_OBJECT {
				parent, err := ParseAddress(owner.GetAddress())
				if err != nil {
					return nil, fmt.Errorf("%s: %w", op, err)
				}
				included = included || parents[parent]
			}
		}
		if !included {
			continue
		}
		selected = append(selected, change)
		if change.GetInputState() == rpcv2.ChangedObject_INPUT_OBJECT_STATE_EXISTS && !scopedBeforeReference(change) {
			if err := add(change.GetObjectId(), change.GetInputVersion(), change.GetInputDigest()); err != nil {
				return nil, err
			}
		}
		if output == rpcv2.ChangedObject_OUTPUT_OBJECT_STATE_OBJECT_WRITE || output == rpcv2.ChangedObject_OUTPUT_OBJECT_STATE_PACKAGE_WRITE {
			if err := add(change.GetObjectId(), change.GetOutputVersion(), change.GetOutputDigest()); err != nil {
				return nil, err
			}
		}
	}
	tx.Effects.ChangedObjects = selected
	tx.Objects = &rpcv2.ObjectSet{}
	for start := 0; start < len(requests); start += scopedObjectBatchSize {
		chunk := requests[start:min(start+scopedObjectBatchSize, len(requests))]
		readCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		mask := &fieldmaskpb.FieldMask{Paths: []string{"object_id", "version", "digest", "object_type", "json"}}
		result, err := readScopedObjects(readCtx, reader, chunk, mask)
		cancel()
		if err != nil {
			return nil, fmt.Errorf("%s: %w", op, err)
		}
		if result == nil || len(result.Objects) != len(chunk) {
			return nil, fmt.Errorf("%s: objects=invalid", op)
		}
		seen := map[key]bool{}
		for _, result := range result.Objects {
			if result == nil {
				return nil, fmt.Errorf("%s: object=null", op)
			}
			if result.GetError() != nil {
				if err := status.ErrorProto(result.GetError()); err != nil {
					return nil, fmt.Errorf("%s: %w", op, err)
				}
				return nil, fmt.Errorf("%s: object_status=invalid", op)
			}
			obj := result.GetObject()
			if obj == nil {
				return nil, fmt.Errorf("%s: object=null", op)
			}
			k := key{obj.GetObjectId(), obj.GetVersion()}
			if expected[k] == "" || obj.GetDigest() != expected[k] || seen[k] {
				return nil, fmt.Errorf("%s: object_reference=invalid", op)
			}
			seen[k] = true
			tx.Objects.Objects = append(tx.Objects.Objects, obj)
		}
		for _, q := range chunk {
			if !seen[key{q.GetObjectId(), q.GetVersion()}] {
				return nil, fmt.Errorf("%s: object=missing", op)
			}
		}
	}
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}
	return response, nil
}

func scopedBeforeReference(change *rpcv2.ChangedObject) bool {
	return change.GetOutputState() == rpcv2.ChangedObject_OUTPUT_OBJECT_STATE_OBJECT_WRITE &&
		change.InputOwner != nil && change.OutputOwner != nil && proto.Equal(change.InputOwner, change.OutputOwner)
}

func readScopedObjects(ctx context.Context, reader scopedObjectReader, requests []*rpcv2.GetObjectRequest, mask *fieldmaskpb.FieldMask) (*rpcv2.BatchGetObjectsResponse, error) {
	if len(requests) != 1 {
		return reader.BatchGetObjects(ctx, &rpcv2.BatchGetObjectsRequest{Requests: requests, ReadMask: mask})
	}
	request := proto.Clone(requests[0]).(*rpcv2.GetObjectRequest)
	request.ReadMask = mask
	result, err := reader.GetObject(ctx, request)
	if err != nil {
		return nil, err
	}
	return &rpcv2.BatchGetObjectsResponse{Objects: []*rpcv2.GetObjectResult{{Result: &rpcv2.GetObjectResult_Object{Object: result.GetObject()}}}}, nil
}
