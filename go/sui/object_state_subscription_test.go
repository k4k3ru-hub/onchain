package sui

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/k4k3ru-hub/onchain/go/sui/internal/rpcv2"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"
)

// TestObjectStateComposition verifies the full-object mask and object filter.
//
// Version:
//   - 2026-09-09: Added.
func TestObjectStateComposition(t *testing.T) {
	sentinel := errors.New("unavailable")
	transport := &objectStreamClient{err: sentinel}
	client := composeGRPCClient(GRPCConfig{URL: "https://example.com"}, &grpcAdapter{client: transport}, nil)
	id, err := ParseAddress("0x9")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.SubscribeObjectState(context.Background(), id); !errors.Is(err, sentinel) {
		t.Fatal(err)
	}
	r := transport.request
	if r == nil || r.Filter.Terms[0].Literals[0].GetAffectedObject().GetObjectId() != id.String() {
		t.Fatal("wrong filter")
	}
	paths := strings.Join(r.ReadMask.Paths, ",")
	if !strings.Contains(paths, "transaction_index") || !strings.Contains(paths, "objects") || !strings.Contains(paths, "effects.changed_objects") || strings.Contains(paths, "balance_changes") {
		t.Fatal(paths)
	}
}

// TestObjectChangesSelectOutputVersion verifies input/output selection and detached JSON.
//
// Version:
//   - 2026-09-09: Added.
func TestObjectChangesSelectOutputVersion(t *testing.T) {
	id, err := ParseAddress("0x9")
	if err != nil {
		t.Fatal(err)
	}
	digest := strings.Repeat("1", 32)
	jsonValue, err := structpb.NewValue(map[string]any{"liquidity": "100"})
	if err != nil {
		t.Fatal(err)
	}
	before := &rpcv2.Object{ObjectId: proto.String(id.String()), Version: proto.Uint64(10), Digest: &digest, ObjectType: proto.String("0x1::pool::Pool"), Json: jsonValue}
	after := proto.Clone(before).(*rpcv2.Object)
	after.Version = proto.Uint64(11)
	change := &rpcv2.ChangedObject{ObjectId: before.ObjectId, InputOwner: &rpcv2.Owner{Kind: rpcv2.Owner_SHARED.Enum()}, OutputOwner: &rpcv2.Owner{Kind: rpcv2.Owner_SHARED.Enum()}, InputState: rpcv2.ChangedObject_INPUT_OBJECT_STATE_EXISTS.Enum(), InputVersion: before.Version, InputDigest: &digest, OutputState: rpcv2.ChangedObject_OUTPUT_OBJECT_STATE_OBJECT_WRITE.Enum(), OutputVersion: after.Version, OutputDigest: &digest}
	tx := &rpcv2.ExecutedTransaction{Effects: &rpcv2.TransactionEffects{ChangedObjects: []*rpcv2.ChangedObject{change}}, Objects: &rpcv2.ObjectSet{Objects: []*rpcv2.Object{after, before}}}
	changes, err := decodeObjectChanges(tx)
	if err != nil {
		t.Fatal(err)
	}
	if len(changes) != 1 || changes[0].Before.Version != 10 || changes[0].After.Version != 11 {
		t.Fatal("selected wrong version")
	}
	after.Json = structpb.NewNullValue()
	if string(changes[0].After.Move.JSON) == "null" {
		t.Fatal("borrowed transport data")
	}
	tx.Objects.Objects = []*rpcv2.Object{before}
	if _, err := decodeObjectChanges(tx); err == nil {
		t.Fatal("missing output replaced by input")
	}
	change.OutputState = rpcv2.ChangedObject_OUTPUT_OBJECT_STATE_DOES_NOT_EXIST.Enum()
	changes, err = decodeObjectChanges(tx)
	if err != nil {
		t.Fatal(err)
	}
	if !changes[0].Deleted || changes[0].After != nil || changes[0].Before == nil {
		t.Fatal("deleted object retained")
	}
}
