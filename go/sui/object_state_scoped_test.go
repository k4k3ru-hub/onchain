package sui

import (
	"context"
	"errors"
	"fmt"
	"io"
	"reflect"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/k4k3ru-hub/onchain/go/sui/internal/rpcv2"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type scopedLedgerFake struct {
	rpcv2.LedgerServiceClient
	read func(context.Context, *rpcv2.BatchGetObjectsRequest) (*rpcv2.BatchGetObjectsResponse, error)
}

// BatchGetObjects delegates exact-version reads to the test fixture.
//
// Version:
//   - 2026-10-12: Added.
func (f *scopedLedgerFake) BatchGetObjects(ctx context.Context, r *rpcv2.BatchGetObjectsRequest, _ ...grpc.CallOption) (*rpcv2.BatchGetObjectsResponse, error) {
	return f.read(ctx, r)
}

// GetObject delegates singleton reads without losing the requested mask or version.
//
// Version:
//   - 2026-10-12: Added.
func (f *scopedLedgerFake) GetObject(ctx context.Context, r *rpcv2.GetObjectRequest, _ ...grpc.CallOption) (*rpcv2.GetObjectResponse, error) {
	batch, err := f.read(ctx, &rpcv2.BatchGetObjectsRequest{Requests: []*rpcv2.GetObjectRequest{r}, ReadMask: r.ReadMask})
	if err != nil {
		return nil, err
	}
	if batch == nil || len(batch.Objects) != 1 {
		return nil, fmt.Errorf("failed to test singleton read: count=invalid")
	}
	if e := batch.Objects[0].GetError(); e != nil {
		return nil, status.ErrorProto(e)
	}
	return &rpcv2.GetObjectResponse{Object: batch.Objects[0].GetObject()}, nil
}

func scopedAddress(t *testing.T, value string) Address {
	t.Helper()
	a, err := ParseAddress(value)
	if err != nil {
		t.Fatal(err)
	}
	return a
}

func scopedResponse(t *testing.T, scenario string) *rpcv2.SubscribeTransactionsResponse {
	t.Helper()
	tx := objectMaskFixture(t, scenario)
	tx.Digest, tx.Checkpoint = proto.String(strings.Repeat("1", 32)), proto.Uint64(42)
	tx.TransactionIndex = proto.Uint64(7)
	tx.Timestamp = timestamppb.New(time.Unix(100, 123000000))
	tx.Effects.Status = &rpcv2.ExecutionStatus{Success: proto.Bool(true)}
	return &rpcv2.SubscribeTransactionsResponse{Transaction: tx, Watermark: &rpcv2.Watermark{Cursor: []byte{1}, Checkpoint: proto.Uint64(42)}}
}

func scopedReadFixture(objects []*rpcv2.Object, r *rpcv2.BatchGetObjectsRequest) (*rpcv2.BatchGetObjectsResponse, error) {
	result := &rpcv2.BatchGetObjectsResponse{}
	for _, q := range r.Requests {
		if q.Version == nil || q.GetVersion() == 0 {
			return nil, fmt.Errorf("failed to test scoped read: version=empty")
		}
		var found *rpcv2.Object
		for _, o := range objects {
			if o.GetObjectId() == q.GetObjectId() && o.GetVersion() == q.GetVersion() {
				found = o
				break
			}
		}
		if found == nil {
			result.Objects = append(result.Objects, &rpcv2.GetObjectResult{Result: &rpcv2.GetObjectResult_Error{Error: status.New(codes.NotFound, "missing version").Proto()}})
		} else {
			result.Objects = append(result.Objects, &rpcv2.GetObjectResult{Result: &rpcv2.GetObjectResult_Object{Object: proto.Clone(found).(*rpcv2.Object)}})
		}
	}
	return result, nil
}

// TestScopedObjectsSelection verifies pool and parent selection without fetching unrelated objects.
//
// Version:
//   - 2026-10-12: Added.
func TestScopedObjectsSelection(t *testing.T) {
	for _, scenario := range []string{"mutated", "created", "deleted", "package"} {
		for _, selection := range []string{"pool", "input_parent", "output_parent", "unrelated"} {
			t.Run(scenario+"/"+selection, func(t *testing.T) {
				r := scopedResponse(t, scenario)
				original := proto.Clone(r)
				objects := r.Transaction.Objects.Objects
				pool := scopedAddress(t, "0xff")
				parents := map[Address]bool{}
				selected := true
				switch selection {
				case "pool":
					pool = scopedAddress(t, "0x9")
				case "input_parent":
					parents[scopedAddress(t, "0x5")] = true
					selected = scenario != "created" && scenario != "package"
				case "output_parent":
					parents[scopedAddress(t, "0x6")] = true
					selected = scenario != "deleted"
				case "unrelated":
					selected = false
				}
				calls := 0
				reader := &scopedLedgerFake{read: func(ctx context.Context, request *rpcv2.BatchGetObjectsRequest) (*rpcv2.BatchGetObjectsResponse, error) {
					calls++
					if deadline, ok := ctx.Deadline(); !ok || time.Until(deadline) > 30*time.Second {
						return nil, fmt.Errorf("failed to test scoped read: deadline=invalid")
					}
					if !reflect.DeepEqual(request.ReadMask.Paths, []string{"object_id", "version", "digest", "object_type", "json"}) {
						return nil, fmt.Errorf("failed to test scoped read: mask=invalid")
					}
					return scopedReadFixture(objects, request)
				}}
				got, err := hydrateScopedObjects(t.Context(), reader, pool, parents, r)
				if err != nil {
					t.Fatal(err)
				}
				changes, err := decodeObjectChanges(got.Transaction)
				if err != nil {
					t.Fatal(err)
				}
				if selected {
					want, err := decodeObjectChanges(r.Transaction)
					if err != nil || !reflect.DeepEqual(want, changes) || calls != 1 {
						t.Fatalf("changed full-object semantics: got=%+v want=%+v calls=%d err=%v", changes, want, calls, err)
					}
				} else if calls != 0 || len(changes) != 0 {
					t.Fatal("fetched or retained unrelated objects")
				}
				if !proto.Equal(r, original) || !proto.Equal(got.Watermark, r.Watermark) {
					t.Fatal("mutated caller payload or lost watermark")
				}
			})
		}
	}
}

// TestScopedReplacementReferences verifies normal updates need only the new payload.
// Input references remain validated and the legacy full-object decoder stays strict.
//
// Version:
//   - 2026-10-12: Added.
func TestScopedReplacementReferences(t *testing.T) {
	r := scopedResponse(t, "mutated")
	change := r.Transaction.Effects.ChangedObjects[0]
	change.OutputOwner = proto.Clone(change.InputOwner).(*rpcv2.Owner)
	// The historical input body is deliberately absent, as on a pruned provider.
	r.Transaction.Objects.Objects = r.Transaction.Objects.Objects[:1]
	reads := 0
	reader := &scopedLedgerFake{read: func(_ context.Context, q *rpcv2.BatchGetObjectsRequest) (*rpcv2.BatchGetObjectsResponse, error) {
		reads += len(q.Requests)
		return scopedReadFixture(r.Transaction.Objects.Objects, q)
	}}
	got, err := hydrateScopedObjects(t.Context(), reader, scopedAddress(t, "0x9"), nil, r)
	if err != nil {
		t.Fatal(err)
	}
	changes, err := decodeObjectChangesWithReferences(got.Transaction, true)
	if err != nil || len(changes) != 1 || reads != 1 {
		t.Fatalf("replacement required historical body: reads=%d err=%v", reads, err)
	}
	before, after := changes[0].Before, changes[0].After
	if before == nil || before.Version != 10 || before.Move != nil || after == nil || after.Version != 11 || after.Move == nil {
		t.Fatal("lost effects references or replacement payload")
	}
	if _, err := decodeObjectChanges(got.Transaction); err == nil {
		t.Fatal("legacy decoder silently accepted missing input")
	}
	for _, invalid := range []string{"version", "digest"} {
		tx := proto.Clone(got.Transaction).(*rpcv2.ExecutedTransaction)
		if invalid == "version" {
			tx.Effects.ChangedObjects[0].InputVersion = nil
		} else {
			tx.Effects.ChangedObjects[0].InputDigest = proto.String("invalid")
		}
		if _, err := decodeObjectChangesWithReferences(tx, true); err == nil {
			t.Fatal("invalid input reference accepted")
		}
	}
}

// TestScopedObjectsRejectMissing verifies missing versions and ambiguous ownership cannot advance state.
//
// Version:
//   - 2026-10-12: Added.
func TestScopedObjectsRejectMissing(t *testing.T) {
	for _, scenario := range []string{"missing_version", "wrong_digest", "missing_json", "partial", "duplicate", "wrong_version", "transport", "owner", "owner_kind", "input_state", "watermark"} {
		t.Run(scenario, func(t *testing.T) {
			r := scopedResponse(t, scenario)
			sentinel := errors.New("read unavailable")
			switch scenario {
			case "owner":
				r.Transaction.Effects.ChangedObjects[0].InputOwner = nil
			case "owner_kind":
				r.Transaction.Effects.ChangedObjects[0].InputOwner.Kind = rpcv2.Owner_OWNER_KIND_UNKNOWN.Enum()
			case "input_state":
				r.Transaction.Effects.ChangedObjects[0].InputState = nil
			case "watermark":
				r.Watermark = nil
			}
			reader := &scopedLedgerFake{read: func(_ context.Context, request *rpcv2.BatchGetObjectsRequest) (*rpcv2.BatchGetObjectsResponse, error) {
				if scenario == "transport" {
					return nil, sentinel
				}
				result, err := scopedReadFixture(r.Transaction.Objects.Objects, request)
				if err != nil {
					return nil, err
				}
				switch scenario {
				case "partial":
					result.Objects = result.Objects[:1]
				case "duplicate":
					result.Objects[1] = result.Objects[0]
				case "wrong_version":
					result.Objects[0].GetObject().Version = proto.Uint64(999)
				}
				return result, nil
			}}
			got, err := hydrateScopedObjects(t.Context(), reader, scopedAddress(t, "0x9"), nil, r)
			if err == nil {
				_, err = decodeObjectChanges(got.Transaction)
			}
			if err == nil {
				t.Fatal("incomplete state accepted")
			}
			if scenario == "transport" && !errors.Is(err, sentinel) {
				t.Fatalf("lost transport error: %v", err)
			}
			if scenario == "missing_version" && status.Code(err) != codes.NotFound {
				t.Fatalf("lost per-object status: %v", err)
			}
		})
	}
}

// TestScopedObjectsLimits verifies chunk sizes and rejects excessive selected state before any read.
//
// Version:
//   - 2026-10-12: Added.
func TestScopedObjectsLimits(t *testing.T) {
	for _, count := range []int{40, scopedObjectLimit + 1} {
		r := scopedResponse(t, "created")
		change, object := r.Transaction.Effects.ChangedObjects[0], r.Transaction.Objects.Objects[0]
		r.Transaction.Effects.ChangedObjects = nil
		r.Transaction.Objects.Objects = nil
		for i := 1; i <= count; i++ {
			ch, obj := proto.Clone(change).(*rpcv2.ChangedObject), proto.Clone(object).(*rpcv2.Object)
			id := fmt.Sprintf("0x%x", i)
			ch.ObjectId, obj.ObjectId = &id, &id
			r.Transaction.Effects.ChangedObjects = append(r.Transaction.Effects.ChangedObjects, ch)
			r.Transaction.Objects.Objects = append(r.Transaction.Objects.Objects, obj)
		}
		var batches []int
		reader := &scopedLedgerFake{read: func(_ context.Context, request *rpcv2.BatchGetObjectsRequest) (*rpcv2.BatchGetObjectsResponse, error) {
			batches = append(batches, len(request.Requests))
			return scopedReadFixture(r.Transaction.Objects.Objects, request)
		}}
		_, err := hydrateScopedObjects(t.Context(), reader, scopedAddress(t, "0xffff"), map[Address]bool{scopedAddress(t, "0x6"): true}, r)
		if count > scopedObjectLimit {
			if err == nil || len(batches) != 0 {
				t.Fatal("oversized transaction issued reads")
			}
		} else if err != nil || !reflect.DeepEqual(batches, []int{32, 8}) {
			t.Fatalf("unexpected batches: %v err=%v", batches, err)
		}
	}
}

type scopedWireFake struct {
	ctx  context.Context
	rows chan *rpcv2.SubscribeTransactionsResponse
}

// Recv reads a cancellable sequence of upstream responses.
//
// Version:
//   - 2026-10-12: Added.
func (w *scopedWireFake) Recv() (*rpcv2.SubscribeTransactionsResponse, error) {
	select {
	case <-w.ctx.Done():
		return nil, w.ctx.Err()
	case r, ok := <-w.rows:
		if !ok {
			return nil, io.EOF
		}
		return r, nil
	}
}

// TestScopedStreamOrdering verifies ordered delivery despite parallel reads and later progress.
//
// Version:
//   - 2026-10-12: Added.
func TestScopedStreamOrdering(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	first := scopedResponse(t, "mutated")
	second := scopedResponse(t, "mutated")
	second.Transaction.Effects.ChangedObjects[0].ObjectId = proto.String("0xa")
	for _, o := range second.Transaction.Objects.Objects {
		o.ObjectId = proto.String("0xa")
	}
	second.Watermark.Cursor = []byte{2}
	progress := &rpcv2.SubscribeTransactionsResponse{Watermark: &rpcv2.Watermark{Cursor: []byte{3}}}
	wire := &scopedWireFake{ctx: ctx, rows: make(chan *rpcv2.SubscribeTransactionsResponse, 3)}
	wire.rows <- first
	wire.rows <- second
	wire.rows <- progress
	close(wire.rows)
	release, secondReady := make(chan struct{}), make(chan struct{})
	reader := &scopedLedgerFake{read: func(ctx context.Context, q *rpcv2.BatchGetObjectsRequest) (*rpcv2.BatchGetObjectsResponse, error) {
		if q.Requests[0].GetObjectId() == "0x9" {
			select {
			case <-release:
			case <-ctx.Done():
				return nil, ctx.Err()
			}
			return scopedReadFixture(first.Transaction.Objects.Objects, q)
		}
		close(secondReady)
		return scopedReadFixture(second.Transaction.Objects.Objects, q)
	}}
	stream := newScopedObjectStream(ctx, wire, reader, scopedAddress(t, "0x9"), map[Address]bool{scopedAddress(t, "0x5"): true})
	select {
	case <-secondReady:
	case <-ctx.Done():
		t.Fatal("second read did not run concurrently")
	}
	done := make(chan scopedObjectResult, 1)
	go func() { r, err := stream.Recv(); done <- scopedObjectResult{r, err} }()
	select {
	case <-done:
		t.Fatal("later data passed blocked first transaction")
	default:
	}
	close(release)
	select {
	case r := <-done:
		if r.err != nil || r.value.Watermark.Cursor[0] != 1 {
			t.Fatalf("first receive: %v", r.err)
		}
	case <-ctx.Done():
		t.Fatal("first read did not complete")
	}
	for _, want := range []byte{2, 3} {
		r, err := stream.Recv()
		if err != nil || r.Watermark.Cursor[0] != want {
			t.Fatalf("lost transaction/watermark order: want=%d err=%v", want, err)
		}
	}
	if _, err := stream.Recv(); !errors.Is(err, io.EOF) {
		t.Fatalf("stream did not finish: %v", err)
	}
}

// TestScopedStreamBoundAndCancel verifies bounded background reads and cancellation.
//
// Version:
//   - 2026-10-12: Added.
func TestScopedStreamBoundAndCancel(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	wire := &scopedWireFake{ctx: ctx, rows: make(chan *rpcv2.SubscribeTransactionsResponse, 20)}
	for i := 0; i < cap(wire.rows); i++ {
		wire.rows <- scopedResponse(t, "mutated")
	}
	started, finished := make(chan struct{}, 20), make(chan struct{}, 20)
	var reads atomic.Int32
	reader := &scopedLedgerFake{read: func(ctx context.Context, _ *rpcv2.BatchGetObjectsRequest) (*rpcv2.BatchGetObjectsResponse, error) {
		reads.Add(1)
		started <- struct{}{}
		<-ctx.Done()
		finished <- struct{}{}
		return nil, ctx.Err()
	}}
	stream := newScopedObjectStream(ctx, wire, reader, scopedAddress(t, "0x9"), nil)
	for i := 0; i < scopedObjectConcurrency; i++ {
		select {
		case <-started:
		case <-ctx.Done():
			t.Fatal("read did not start")
		}
	}
	if reads.Load() != scopedObjectConcurrency || len(wire.rows) != 20-scopedObjectConcurrency {
		t.Fatal("unbounded acquisition")
	}
	cancel()
	for i := 0; i < scopedObjectConcurrency; i++ {
		select {
		case <-finished:
		case <-time.After(3 * time.Second):
			t.Fatal("background read survived cancellation")
		}
	}
	if _, err := stream.Recv(); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation lost: %v", err)
	}
}

// TestScopedObjectsComposition verifies the public API uses the composed ledger and event transports.
//
// Version:
//   - 2026-10-12: Added.
func TestScopedObjectsComposition(t *testing.T) {
	r := scopedResponse(t, "mutated")
	transport := &poolEventsTransport{response: r}
	reader := &scopedLedgerFake{read: func(_ context.Context, q *rpcv2.BatchGetObjectsRequest) (*rpcv2.BatchGetObjectsResponse, error) {
		return scopedReadFixture(r.Transaction.Objects.Objects, q)
	}}
	client := composeGRPCClient(GRPCConfig{}, &grpcAdapter{client: transport, ledgerClient: reader}, nil)
	id := scopedAddress(t, "0x9")
	sub, err := client.SubscribeObjectStateScoped(t.Context(), id, ObjectStateScope{ParentIDs: []Address{scopedAddress(t, "0x5")}})
	if err != nil {
		t.Fatal(err)
	}
	defer sub.Close()
	if transport.request.Filter.Terms[0].Literals[0].GetAffectedObject().GetObjectId() != id.String() {
		t.Fatal("wrong filter")
	}
	for _, path := range transport.request.ReadMask.Paths {
		if strings.HasPrefix(path, "objects") || strings.HasPrefix(path, "events") {
			t.Fatal("metadata stream includes full payloads")
		}
	}
	if !slices.Contains(transport.request.ReadMask.Paths, "effects.changed_objects") {
		t.Fatal("missing selection metadata")
	}
	n, err := sub.Recv(t.Context())
	if err != nil || n.Effects == nil || len(n.ObjectChanges) != 1 || n.ObjectChanges[0].After.Version != 11 {
		t.Fatalf("failed composed decode: %v", err)
	}
	if n.Effects.TransactionIndex == nil || *n.Effects.TransactionIndex != 7 || n.Effects.Timestamp == nil || !n.Effects.Timestamp.Equal(r.Transaction.Timestamp.AsTime()) || !reflect.DeepEqual(n.Watermark.Cursor, []byte{1}) {
		t.Fatal("lost ordering or source time")
	}
	sub.Close()
	if transport.ctx.Err() != context.Canceled {
		t.Fatal("close did not cancel transports")
	}
	for _, scope := range []ObjectStateScope{{ParentIDs: []Address{{}}}, {ParentIDs: []Address{id, id}}, {ParentIDs: make([]Address, 17)}} {
		if _, err := client.SubscribeObjectStateScoped(t.Context(), id, scope); err == nil {
			t.Fatal("accepted invalid parents")
		}
	}
}
