package clmm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/k4k3ru-hub/onchain/go/sui"
)

type scopedSubscriberFake struct {
	pool        sui.Address
	scope       sui.ObjectStateScope
	onSubscribe func()
}

// SubscribeObjectState rejects accidental fallback to full Object delivery.
//
// Version:
//   - 2026-10-12: Added.
func (s *scopedSubscriberFake) SubscribeObjectState(context.Context, sui.Address) (*sui.TransactionSubscription, error) {
	return nil, errors.New("unexpected full state subscription")
}

// SubscribeObjectStateScoped captures the bitmap and tick parents before initialization.
//
// Version:
//   - 2026-10-12: Added.
func (s *scopedSubscriberFake) SubscribeObjectStateScoped(_ context.Context, pool sui.Address, scope sui.ObjectStateScope) (*sui.TransactionSubscription, error) {
	s.pool, s.scope = pool, scope
	if s.onSubscribe != nil {
		s.onSubscribe()
	}
	return sui.NewTransactionSubscription(scopedEndReceiver{})
}

type scopedEndReceiver struct{}

// Recv simulates a disconnected stream after startup.
//
// Version:
//   - 2026-10-12: Added.
func (scopedEndReceiver) Recv() (*sui.TransactionNotification, error) { return nil, io.EOF }

// Close releases the fixture subscription.
//
// Version:
//   - 2026-10-12: Added.
func (scopedEndReceiver) Close() {}

type relocatedBitmapReader struct{ *stateReaderFake }

// DynamicValuesByKeysAtCheckpoint serves the same fixture bitmap after parent replacement.
//
// Version:
//   - 2026-10-12: Added.
func (r relocatedBitmapReader) DynamicValuesByKeysAtCheckpoint(ctx context.Context, parent sui.Address, cp sui.CheckpointSequenceNumber, keys []sui.DynamicFieldKey) ([]json.RawMessage, error) {
	if parent == testAddress("0xc") {
		parent = testAddress("0xb")
	}
	return r.stateReaderFake.DynamicValuesByKeysAtCheckpoint(ctx, parent, cp, keys)
}

// TestRetainedScopedSubscription verifies scoped bootstrap, local quotes and reconnect invalidation.
//
// Version:
//   - 2026-10-12: Added.
func TestRetainedScopedSubscription(t *testing.T) {
	for _, changed := range []bool{false, true} {
		t.Run(map[bool]string{false: "stable", true: "changed_parent"}[changed], func(t *testing.T) {
			c, reader, p := stateFixture(t)
			s := &scopedSubscriberFake{}
			if changed {
				c.reader = relocatedBitmapReader{reader}
				s.onSubscribe = func() {
					reader.obj.Move.JSON = bytes.ReplaceAll(reader.obj.Move.JSON, []byte(`"id":"0xb"`), []byte(`"id":"0xc"`))
					reader.obj.Version++
				}
			}
			var latest, ready *QuoteSnapshot
			c.SetQuoteSnapshotObserver(func(q *QuoteSnapshot) {
				latest = q
				if q != nil && q.Inputs().Observation.Active {
					ready = q
				}
			})
			for epoch := uint64(1); epoch <= 2; epoch++ {
				ready = nil
				reader.obj.Move.JSON = bytes.ReplaceAll(reader.obj.Move.JSON, []byte(`"id":"0xc"`), []byte(`"id":"0xb"`))
				err := c.RunRetained(t.Context(), s, c.Warm)
				if s.pool != c.pool || len(s.scope.ParentIDs) != 2 || s.scope.ParentIDs[1] != c.pool {
					t.Fatal("wrong subscription scope")
				}
				if changed {
					if err == nil || !strings.Contains(err.Error(), "subscription_scope=changed") || ready != nil {
						t.Fatalf("scope mismatch published inputs: %v", err)
					}
				} else {
					if s.scope.ParentIDs[0] != testAddress("0xb") || !errors.Is(err, io.EOF) || ready == nil || ready.Inputs().Observation.Epoch != epoch {
						t.Fatalf("scoped initialization failed: %v", err)
					}
					reads := reader.reads
					if _, err := ready.cache.QuoteRetainedPair(t.Context(), p); err != nil || reader.reads != reads {
						t.Fatalf("frozen quote required reads: %v", err)
					}
				}
				if latest != nil || c.CaptureQuoteSnapshot() != nil {
					t.Fatal("disconnected state remained available")
				}
			}
		})
	}
}
