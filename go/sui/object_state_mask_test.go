package sui

import (
	"context"
	"errors"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/k4k3ru-hub/onchain/go/sui/internal/rpcv2"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/known/fieldmaskpb"
	"google.golang.org/protobuf/types/known/structpb"
)

// TestObjectStateSelectiveMask verifies both subscriptions retain decoder inputs.
//
// Version:
//   - 2026-10-08: Added.
func TestObjectStateSelectiveMask(t *testing.T) {
	for _, events := range []bool{false, true} {
		sentinel := errors.New("unavailable")
		transport := &objectStreamClient{err: sentinel}
		client := composeGRPCClient(GRPCConfig{URL: "https://example.com"}, &grpcAdapter{client: transport}, nil)
		id, err := ParseAddress("0x9")
		if err != nil {
			t.Fatal(err)
		}
		if events {
			_, err = client.SubscribePoolTransactions(context.Background(), id)
		} else {
			_, err = client.SubscribeObjectState(context.Background(), id)
		}
		if !errors.Is(err, sentinel) || transport.request == nil {
			t.Fatalf("subscription did not reach transport: %v", err)
		}
		mask := transport.request.ReadMask
		for _, path := range []string{"effects.changed_objects", "effects.status", "digest", "checkpoint", "transaction_index", "timestamp"} {
			if !slices.Contains(mask.Paths, path) {
				t.Fatalf("missing transaction field %s", path)
			}
		}
		if slices.Contains(mask.Paths, "events") != events {
			t.Fatal("changed optional event subscription")
		}
		for _, path := range mask.Paths {
			if path == "objects" || path == "objects.objects" {
				t.Fatal("unbounded object field mask")
			}
		}
		for _, scenario := range []string{"mutated", "created", "deleted", "package", "missing_version", "wrong_digest", "missing_json"} {
			t.Run(scenario+map[bool]string{false: "_state", true: "_events"}[events], func(t *testing.T) {
				full := objectMaskFixture(t, scenario)
				selected := projectObjectMask(t, full, mask)
				want, wantErr := decodeObjectChanges(full)
				got, gotErr := decodeObjectChanges(selected)
				if (wantErr != nil) != (gotErr != nil) {
					t.Fatalf("changed decode error: full=%v selected=%v", wantErr, gotErr)
				}
				if strings.HasPrefix(scenario, "missing_") || scenario == "wrong_digest" {
					if gotErr == nil {
						t.Fatal("invalid object accepted")
					}
					return
				}
				if wantErr != nil || !reflect.DeepEqual(want, got) || len(got) != 1 {
					t.Fatalf("changed object semantics: full=%+v selected=%+v error=%v", want, got, wantErr)
				}
				if scenario == "mutated" && (got[0].Before.Version != 10 || got[0].After.Version != 11) {
					t.Fatal("selected wrong object version")
				}
				if scenario == "deleted" && (!got[0].Deleted || got[0].After != nil) {
					t.Fatal("deleted object retained")
				}
				if scenario == "package" && (!got[0].After.Package || got[0].After.Move != nil) {
					t.Fatal("package changed to Move object")
				}
				if scenario == "mutated" && (got[0].InputParent.IsZero() || got[0].OutputParent.IsZero()) {
					t.Fatal("lost effects ownership")
				}
			})
		}
	}
}

func objectMaskFixture(t *testing.T, scenario string) *rpcv2.ExecutedTransaction {
	t.Helper()
	digest := strings.Repeat("1", 32)
	value, err := structpb.NewValue(map[string]any{"liquidity": "100"})
	if err != nil {
		t.Fatal(err)
	}
	before := &rpcv2.Object{ObjectId: proto.String("0x9"), Version: proto.Uint64(10), Digest: &digest,
		ObjectType: proto.String("0x1::pool::Pool"), Json: value,
		Bcs: &rpcv2.Bcs{Value: []byte{1, 2, 3}}, Contents: &rpcv2.Bcs{Value: []byte{4, 5}},
		StorageRebate: proto.Uint64(99), Owner: &rpcv2.Owner{Kind: rpcv2.Owner_ADDRESS.Enum(), Address: proto.String("0x77")}}
	after := proto.Clone(before).(*rpcv2.Object)
	after.Version = proto.Uint64(11)
	change := &rpcv2.ChangedObject{ObjectId: before.ObjectId,
		InputState: rpcv2.ChangedObject_INPUT_OBJECT_STATE_EXISTS.Enum(), InputVersion: before.Version, InputDigest: &digest,
		InputOwner:  &rpcv2.Owner{Kind: rpcv2.Owner_OBJECT.Enum(), Address: proto.String("0x5")},
		OutputState: rpcv2.ChangedObject_OUTPUT_OBJECT_STATE_OBJECT_WRITE.Enum(), OutputVersion: after.Version, OutputDigest: &digest,
		OutputOwner: &rpcv2.Owner{Kind: rpcv2.Owner_OBJECT.Enum(), Address: proto.String("0x6")}}
	tx := &rpcv2.ExecutedTransaction{Effects: &rpcv2.TransactionEffects{ChangedObjects: []*rpcv2.ChangedObject{change}},
		Objects: &rpcv2.ObjectSet{Objects: []*rpcv2.Object{after, before}}}
	switch scenario {
	case "created", "package":
		change.InputState = rpcv2.ChangedObject_INPUT_OBJECT_STATE_DOES_NOT_EXIST.Enum()
		change.InputOwner, change.InputVersion, change.InputDigest = nil, nil, nil
		tx.Objects.Objects = []*rpcv2.Object{after}
		if scenario == "package" {
			after.ObjectType, after.Json = proto.String("package"), nil
			after.Package = &rpcv2.Package{}
			change.OutputState = rpcv2.ChangedObject_OUTPUT_OBJECT_STATE_PACKAGE_WRITE.Enum()
		}
	case "deleted":
		change.OutputState = rpcv2.ChangedObject_OUTPUT_OBJECT_STATE_DOES_NOT_EXIST.Enum()
		change.OutputOwner, change.OutputVersion, change.OutputDigest = nil, nil, nil
		tx.Objects.Objects = []*rpcv2.Object{before}
	case "missing_version":
		tx.Objects.Objects = []*rpcv2.Object{before}
	case "wrong_digest":
		change.OutputDigest = proto.String(strings.Repeat("2", 32))
	case "missing_json":
		after.Json = nil
	}
	return tx
}

func projectObjectMask(t *testing.T, source *rpcv2.ExecutedTransaction, mask *fieldmaskpb.FieldMask) *rpcv2.ExecutedTransaction {
	t.Helper()
	result := proto.Clone(source).(*rpcv2.ExecutedTransaction)
	fields := make(map[protoreflect.Name]bool)
	for _, path := range mask.Paths {
		if field, ok := strings.CutPrefix(path, "objects.objects."); ok {
			fields[protoreflect.Name(field)] = true
		}
	}
	for _, object := range result.Objects.Objects {
		message := object.ProtoReflect()
		message.Range(func(field protoreflect.FieldDescriptor, _ protoreflect.Value) bool {
			if !fields[field.Name()] {
				message.Clear(field)
			}
			return true
		})
		if object.Bcs != nil || object.Contents != nil || object.Owner != nil || object.StorageRebate != nil || object.Package != nil {
			t.Fatal("retained unused object fields")
		}
	}
	return result
}
