//go:build sui_fieldmask_diagnostic

package sui

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/k4k3ru-hub/onchain/go/sui/internal/rpcv2"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/fieldmaskpb"
	"google.golang.org/protobuf/types/known/structpb"
)

// These opt-in diagnostics use only public data. Neither a signer nor an
// execution client is composed, and default SDK tests never open a network.
type diagnosticMask struct {
	Name    string   `json:"name"`
	Paths   []string `json:"paths"`
	Objects bool     `json:"objects"`
	Events  bool     `json:"events"`
}

func diagnosticMasks() []diagnosticMask {
	base := []string{"digest", "effects.status", "checkpoint", "transaction_index", "timestamp"}
	full := append(append([]string(nil), base...), "effects.changed_objects", "objects", "events")
	compact := append(append([]string(nil), base...), "effects.changed_objects", "objects.objects.object_id", "objects.objects.version", "objects.objects.digest", "objects.objects.object_type", "objects.objects.json", "events")
	return []diagnosticMask{{Name: "full", Paths: full, Objects: true, Events: true},
		{Name: "compact_objects", Paths: compact, Objects: true, Events: true},
		{Name: "events_only", Paths: append(append([]string(nil), base...), "events"), Events: true},
		{Name: "summary", Paths: base}}
}

type diagnosticMaskPoint struct {
	ReceivedAt     time.Time  `json:"receivedAt"`
	TransactionAt  *time.Time `json:"transactionAt,omitempty"`
	Transaction    string     `json:"transaction,omitempty"`
	Checkpoint     *uint64    `json:"checkpoint,omitempty"`
	ProtobufBytes  int        `json:"protobufBytes"`
	DecodeUs       int64      `json:"decodeUs"`
	Objects        int        `json:"objects"`
	Events         int        `json:"events"`
	ObjectHash     string     `json:"objectHash,omitempty"`
	EventHash      string     `json:"eventHash,omitempty"`
	ObjectWireHash string     `json:"objectWireHash,omitempty"`
	RawObjects     int        `json:"rawObjects"`
	EventBCSHash   string     `json:"eventBCSHash,omitempty"`
	ObjectError    bool       `json:"objectError"`
	EventError     bool       `json:"eventError"`
}

type diagnosticMaskResult struct {
	Mask      diagnosticMask        `json:"mask"`
	Samples   []diagnosticMaskPoint `json:"samples"`
	Complete  bool                  `json:"complete"`
	ErrorCode string                `json:"errorCode,omitempty"`
}

type diagnosticWire struct {
	value *rpcv2.SubscribeTransactionsResponse
}

// Recv supplies one immutable response to the SDK decoder.
//
// Version:
//   - 2026-10-08: Added.
func (w *diagnosticWire) Recv() (*rpcv2.SubscribeTransactionsResponse, error) { return w.value, nil }

func diagnosticHash(value any) (string, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return "", fmt.Errorf("failed to encode diagnostic fingerprint: %w", err)
	}
	return fmt.Sprintf("%x", sha256.Sum256(data)), nil
}

func diagnosticObjectWireHash(objects *rpcv2.ObjectSet) (string, error) {
	selected := &rpcv2.ObjectSet{}
	for _, object := range objects.GetObjects() {
		if object == nil {
			return "", fmt.Errorf("failed to fingerprint wire objects: object=null")
		}
		selected.Objects = append(selected.Objects, &rpcv2.Object{ObjectId: object.ObjectId, Version: object.Version,
			Digest: object.Digest, ObjectType: object.ObjectType, Contents: object.Contents})
	}
	data, err := (proto.MarshalOptions{Deterministic: true}).Marshal(selected)
	if err != nil {
		return "", fmt.Errorf("failed to fingerprint wire objects: %w", err)
	}
	return fmt.Sprintf("%x", sha256.Sum256(data)), nil
}

// TestSuiDiagnosticObjectFingerprint distinguishes binary data from JSON output.
//
// Version:
//   - 2026-10-08: Added.
func TestSuiDiagnosticObjectFingerprint(t *testing.T) {
	objects := &rpcv2.ObjectSet{Objects: []*rpcv2.Object{{ObjectId: proto.String("0x1"), Version: proto.Uint64(1),
		Digest: proto.String("digest"), ObjectType: proto.String("object"), Contents: &rpcv2.Bcs{Value: []byte{1}},
		Json: structpb.NewStringValue("first")}}}
	want, err := diagnosticObjectWireHash(objects)
	if err != nil {
		t.Fatal(err)
	}
	objects.Objects[0].Json = structpb.NewStringValue("second")
	got, err := diagnosticObjectWireHash(objects)
	if err != nil || got != want {
		t.Fatalf("JSON changed binary fingerprint: %v", err)
	}
	objects.Objects[0].Contents.Value[0] = 2
	got, err = diagnosticObjectWireHash(objects)
	if err != nil || got == want {
		t.Fatalf("different BCS had the same fingerprint: %v", err)
	}
	objects.Objects = append(objects.Objects, nil)
	if _, err := diagnosticObjectWireHash(objects); err == nil {
		t.Fatal("nil object accepted")
	}
}

func diagnosticReceive(ctx context.Context, client rpcv2.SubscriptionServiceClient, address string, mask diagnosticMask) diagnosticMaskResult {
	r := diagnosticMaskResult{Mask: mask}
	request := &rpcv2.SubscribeTransactionsRequest{ReadMask: &fieldmaskpb.FieldMask{Paths: mask.Paths},
		Filter: &rpcv2.TransactionFilter{Terms: []*rpcv2.TransactionTerm{{Literals: []*rpcv2.TransactionLiteral{{Predicate: &rpcv2.TransactionLiteral_AffectedObject{AffectedObject: &rpcv2.AffectedObjectFilter{ObjectId: &address}}}}}}}}
	wire, err := client.SubscribeTransactions(ctx, request)
	if err != nil {
		r.ErrorCode = status.Code(err).String()
		return r
	}
	for {
		response, err := wire.Recv()
		received := time.Now().UTC()
		if err != nil {
			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				r.Complete = true
			} else {
				r.ErrorCode = status.Code(err).String()
			}
			return r
		}
		started := time.Now()
		decoder := &grpcTransactionReceiver{stream: &diagnosticWire{value: response}, includeObjects: mask.Objects, includeEvents: mask.Events}
		n, err := decoder.Recv()
		decodeUs := time.Since(started).Microseconds()
		if err != nil {
			r.ErrorCode = "sdk_decode_failed"
			return r
		}
		p := diagnosticMaskPoint{ReceivedAt: received, ProtobufBytes: proto.Size(response), DecodeUs: decodeUs,
			Objects: len(n.ObjectChanges), Events: len(n.Events), ObjectError: n.ObjectChangesError != nil, EventError: n.EventsError != nil}
		objects := response.GetTransaction().GetObjects()
		p.RawObjects = len(objects.GetObjects())
		if p.RawObjects > 0 {
			p.ObjectWireHash, err = diagnosticObjectWireHash(objects)
			if err != nil {
				r.ErrorCode = "wire_object_fingerprint_failed"
				return r
			}
		}
		if n.Effects != nil {
			p.Transaction, p.TransactionAt = n.Effects.Digest.String(), n.Effects.Timestamp
			if n.Effects.Checkpoint != nil {
				x := n.Effects.Checkpoint.Uint64()
				p.Checkpoint = &x
			}
		}
		if mask.Objects && n.ObjectChangesError == nil {
			p.ObjectHash, err = diagnosticHash(n.ObjectChanges)
			if err != nil {
				r.ErrorCode = "object_fingerprint_failed"
				return r
			}
		}
		if mask.Events && n.EventsError == nil {
			p.EventHash, err = diagnosticHash(n.Events)
			if err != nil {
				r.ErrorCode = "event_fingerprint_failed"
				return r
			}
			binaryEvents := append([]LiveEvent(nil), n.Events...)
			for i := range binaryEvents {
				binaryEvents[i].JSON = nil
			}
			p.EventBCSHash, err = diagnosticHash(binaryEvents)
			if err != nil {
				r.ErrorCode = "binary_event_fingerprint_failed"
				return r
			}
		}
		r.Samples = append(r.Samples, p)
		if len(r.Samples) >= 2000 {
			r.ErrorCode = "notification_limit_reached"
			return r
		}
	}
}

// TestSuiSelectiveFieldMaskDiagnostic compares existing and reduced read masks.
// Set the output environment variable explicitly and enable the build tag to
// run the bounded public Mainnet read. This never constructs or sends an order.
//
// Version:
//   - 2026-10-08: Added.
func TestSuiSelectiveFieldMaskDiagnostic(t *testing.T) {
	runFieldMaskDiagnostic(t, diagnosticMasks(), "selective_fieldmask_diagnostic_not_backtest_input")
}

// TestSuiObjectMaterializationDiagnostic isolates Object and event JSON requests.
// BCS-only objects deliberately bypass the JSON state decoder and cannot be used
// as a replacement for production Pool state. The same explicit opt-in applies.
//
// Version:
//   - 2026-10-08: Added.
func TestSuiObjectMaterializationDiagnostic(t *testing.T) {
	base := []string{"digest", "effects.status", "checkpoint", "transaction_index", "timestamp"}
	objects := append(append([]string(nil), base...), "effects.changed_objects", "objects.objects.object_id",
		"objects.objects.version", "objects.objects.digest", "objects.objects.object_type", "objects.objects.contents", "events")
	binaryEvents := append(append([]string(nil), base...), "events.events.package_id", "events.events.module",
		"events.events.sender", "events.events.event_type", "events.events.contents")
	masks := []diagnosticMask{
		{Name: "objects_json", Paths: append(append([]string(nil), objects...), "objects.objects.json"), Objects: true, Events: true},
		{Name: "objects_bcs", Paths: objects, Events: true},
		{Name: "events_json", Paths: append(append([]string(nil), base...), "events"), Events: true},
		{Name: "events_bcs", Paths: binaryEvents, Events: true},
	}
	runFieldMaskDiagnostic(t, masks, "object_materialization_diagnostic_not_backtest_input")
}

func runFieldMaskDiagnostic(t *testing.T, masks []diagnosticMask, classification string) {
	t.Helper()
	output := os.Getenv("SUI_FIELDMASK_DIAGNOSTIC_OUTPUT")
	if output == "" {
		t.Skip("public diagnostic is not enabled")
	}
	if !filepath.IsAbs(output) {
		t.Fatal("failed to start fieldmask diagnostic: output_path=invalid")
	}
	file, err := os.OpenFile(output, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(fmt.Errorf("failed to create diagnostic output: %w", err))
	}
	defer func() {
		if err := file.Close(); err != nil {
			t.Error(fmt.Errorf("failed to close diagnostic output: %w", err))
		}
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	connection, err := grpc.NewClient("dns:///sui-grpc.publicnode.com:443", grpc.WithTransportCredentials(credentials.NewTLS(&tls.Config{MinVersion: tls.VersionTLS12, ServerName: "sui-grpc.publicnode.com"})))
	if err != nil {
		t.Fatal(fmt.Errorf("failed to create diagnostic connection: %w", err))
	}
	defer func() {
		if err := connection.Close(); err != nil {
			t.Error(fmt.Errorf("failed to close diagnostic connection: %w", err))
		}
	}()
	client := rpcv2.NewSubscriptionServiceClient(connection)
	results := make([]diagnosticMaskResult, len(masks))
	started := time.Now().UTC()
	var wg sync.WaitGroup
	for i, mask := range masks {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results[i] = diagnosticReceive(ctx, client, "0x51e883ba7c0b566a26cbc8a94cd33eb0abd418a77cc1e60ad22fd9b1f29cd2ab", mask)
		}()
	}
	wg.Wait()
	result := map[string]any{"classification": classification, "startedAt": started, "stoppedAt": time.Now().UTC(),
		"endpoint": "sui-grpc.publicnode.com:443", "poolId": "0x51e883ba7c0b566a26cbc8a94cd33eb0abd418a77cc1e60ad22fd9b1f29cd2ab", "ordersSubmitted": 0, "results": results,
		"limits": []string{"One shared gRPC connection; server internals are not observed.", "Payload sizes are protobuf serialization sizes, not compressed wire-byte measurements.", "Digest-matched decoded fingerprints compare required semantic objects and events without storing raw payloads.", "ObjectWireHash compares object identity and BCS contents, excluding JSON.", "Objects=false does not validate binary objects as decoded Pool state.", "EventBCSHash compares decoded event identity and BCS, excluding JSON.", "The diagnostic does not change SDK production behavior or MarketHub configuration."}}
	if err := json.NewEncoder(file).Encode(result); err != nil {
		t.Fatal(fmt.Errorf("failed to encode fieldmask diagnostic: %w", err))
	}
	for _, r := range results {
		t.Logf("mode=%s complete=%t notifications=%d error_code=%q", r.Mask.Name, r.Complete, len(r.Samples), r.ErrorCode)
	}
}
