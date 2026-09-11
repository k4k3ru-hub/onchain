package sui

import (
	"context"
	"fmt"

	"github.com/k4k3ru-hub/onchain/go/sui/internal/rpcv2"
	"google.golang.org/protobuf/types/known/fieldmaskpb"
)

// ObjectChange contains only effects-selected input/output versions, never an
// arbitrary entry from the transaction's set of input and output objects.
type ObjectChange struct {
	Address                   Address
	Before, After             *Object
	InputParent, OutputParent Address
	Deleted                   bool
}

// SubscribeObjectState streams full changed objects for transactions affecting a pool.
// This is separate from the lightweight Trade/checkpoint subscription.
//
// Version:
//   - 2026-09-10: Retain optional checkpoint-local transaction position.
//   - 2026-09-09: Added.
func (c *GRPCClient) SubscribeObjectState(ctx context.Context, object Address) (*TransactionSubscription, error) {
	if c == nil || object.IsZero() {
		return nil, fmt.Errorf("failed to subscribe sui object state: parameters=invalid")
	}
	provider, ok := c.transactionProvider.(interface {
		subscribeObjectState(context.Context, Address) (liveTransactionReceiver, error)
	})
	if !ok {
		return nil, fmt.Errorf("failed to subscribe sui object state: provider=unsupported")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	receiver, err := provider.subscribeObjectState(ctx, object)
	if err != nil {
		return nil, fmt.Errorf("failed to subscribe sui object state: %w", err)
	}
	if receiver == nil {
		return nil, fmt.Errorf("failed to subscribe sui object state: receiver=null")
	}
	return &TransactionSubscription{receiver: receiver}, nil
}

func (a *grpcAdapter) subscribeObjectState(ctx context.Context, object Address) (liveTransactionReceiver, error) {
	return a.subscribePoolTransactions(ctx, object, false)
}

func (a *grpcAdapter) subscribePoolTransactions(ctx context.Context, object Address, events bool) (liveTransactionReceiver, error) {
	ctx, cancel := context.WithCancel(ctx)
	address := object.String()
	request := &rpcv2.SubscribeTransactionsRequest{
		ReadMask: &fieldmaskpb.FieldMask{Paths: []string{"digest", "effects.status", "effects.changed_objects", "checkpoint", "transaction_index", "timestamp", "objects"}},
		Filter:   &rpcv2.TransactionFilter{Terms: []*rpcv2.TransactionTerm{{Literals: []*rpcv2.TransactionLiteral{{Predicate: &rpcv2.TransactionLiteral_AffectedObject{AffectedObject: &rpcv2.AffectedObjectFilter{ObjectId: &address}}}}}}},
	}
	if events {
		request.ReadMask.Paths = append(request.ReadMask.Paths, "events")
	}
	stream, err := a.client.SubscribeTransactions(ctx, request)
	if err != nil {
		cancel()
		return nil, err
	}
	if stream == nil {
		cancel()
		return nil, fmt.Errorf("failed to subscribe sui object state: stream=null")
	}
	return &grpcTransactionReceiver{stream: stream, cancel: cancel, includeObjects: true, includeEvents: events}, nil
}

func decodeObjectChanges(tx *rpcv2.ExecutedTransaction) ([]ObjectChange, error) {
	if tx == nil || tx.Effects == nil {
		return nil, fmt.Errorf("failed to decode sui object changes: effects=null")
	}
	type key struct {
		id      string
		version uint64
	}
	objects := make(map[key]*rpcv2.Object)
	for _, obj := range tx.GetObjects().GetObjects() {
		if obj == nil {
			return nil, fmt.Errorf("failed to decode sui object changes: object=null")
		}
		k := key{obj.GetObjectId(), obj.GetVersion()}
		if _, exists := objects[k]; exists {
			return nil, fmt.Errorf("failed to decode sui object changes: object=duplicate")
		}
		objects[k] = obj
	}
	decode := func(id string, version uint64, digest string) (*Object, error) {
		obj := objects[key{id, version}]
		if obj == nil || version == 0 || digest == "" || obj.GetDigest() != digest {
			return nil, fmt.Errorf("failed to decode sui object changes: object_reference=mismatch")
		}
		address, err := ParseAddress(id)
		if err != nil {
			return nil, fmt.Errorf("failed to decode sui object changes: %w", err)
		}
		d, err := ParseObjectDigest(digest)
		if err != nil {
			return nil, fmt.Errorf("failed to decode sui object changes: %w", err)
		}
		result := &Object{Address: address, Version: version, Digest: d, Package: obj.GetObjectType() == "package"}
		if !result.Package {
			if obj.Json == nil || obj.GetObjectType() == "" {
				return nil, fmt.Errorf("failed to decode sui object changes: move_object=null")
			}
			data, err := obj.Json.MarshalJSON()
			if err != nil {
				return nil, fmt.Errorf("failed to decode sui object changes: %w", err)
			}
			if string(data) == "null" {
				return nil, fmt.Errorf("failed to decode sui object changes: move_object=null")
			}
			result.Move = &MoveObject{Type: obj.GetObjectType(), JSON: data}
		}
		return result, nil
	}
	parent := func(owner *rpcv2.Owner) (Address, error) {
		if owner == nil || owner.GetKind() != rpcv2.Owner_OBJECT {
			return Address{}, nil
		}
		value, err := ParseAddress(owner.GetAddress())
		if err != nil {
			return Address{}, fmt.Errorf("failed to decode sui object owner: %w", err)
		}
		return value, nil
	}
	var result []ObjectChange
	seen := make(map[Address]bool)
	for _, change := range tx.Effects.ChangedObjects {
		if change == nil {
			return nil, fmt.Errorf("failed to decode sui object changes: change=null")
		}
		if change.GetOutputState() == rpcv2.ChangedObject_OUTPUT_OBJECT_STATE_ACCUMULATOR_WRITE {
			continue
		}
		id, err := ParseAddress(change.GetObjectId())
		if err != nil {
			return nil, fmt.Errorf("failed to decode sui object changes: %w", err)
		}
		if seen[id] {
			return nil, fmt.Errorf("failed to decode sui object changes: change=duplicate")
		}
		seen[id] = true
		item := ObjectChange{Address: id}
		if change.GetInputState() == rpcv2.ChangedObject_INPUT_OBJECT_STATE_EXISTS {
			if change.InputOwner == nil {
				return nil, fmt.Errorf("failed to decode sui object changes: input_owner=null")
			}
			item.Before, err = decode(change.GetObjectId(), change.GetInputVersion(), change.GetInputDigest())
			if err != nil {
				return nil, err
			}
		}
		item.InputParent, err = parent(change.InputOwner)
		if err != nil {
			return nil, err
		}
		item.OutputParent, err = parent(change.OutputOwner)
		if err != nil {
			return nil, err
		}
		switch change.GetOutputState() {
		case rpcv2.ChangedObject_OUTPUT_OBJECT_STATE_OBJECT_WRITE, rpcv2.ChangedObject_OUTPUT_OBJECT_STATE_PACKAGE_WRITE:
			if change.OutputOwner == nil {
				return nil, fmt.Errorf("failed to decode sui object changes: output_owner=null")
			}
			item.After, err = decode(change.GetObjectId(), change.GetOutputVersion(), change.GetOutputDigest())
			if err != nil {
				return nil, err
			}
		case rpcv2.ChangedObject_OUTPUT_OBJECT_STATE_DOES_NOT_EXIST:
			item.Deleted = true
		case rpcv2.ChangedObject_OUTPUT_OBJECT_STATE_ACCUMULATOR_WRITE:
			continue // Balance accumulator writes are not Move pool objects.
		default:
			return nil, fmt.Errorf("failed to decode sui object changes: output_state=invalid")
		}
		result = append(result, item)
	}
	return result, nil
}
