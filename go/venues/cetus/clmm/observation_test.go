package clmm

import (
	"context"
	"errors"
	"io"
	"testing"

	"github.com/k4k3ru-hub/onchain/go/sui"
)

type progressSubscriber struct{ checkpoint sui.CheckpointSequenceNumber }
type progressReceiver struct {
	checkpoint sui.CheckpointSequenceNumber
	sent       bool
}

// SubscribeObjectState creates a finite fake stream with a no-transaction watermark.
//
// Version:
//   - 2026-10-01: Added.
func (s progressSubscriber) SubscribeObjectState(context.Context, sui.Address) (*sui.TransactionSubscription, error) {
	return sui.NewTransactionSubscription(&progressReceiver{checkpoint: s.checkpoint})
}

// Recv emits progress before simulating transport loss.
//
// Version:
//   - 2026-10-01: Added.
func (r *progressReceiver) Recv() (*sui.TransactionNotification, error) {
	if r.sent {
		return nil, io.EOF
	}
	r.sent = true
	return &sui.TransactionNotification{Watermark: sui.EventWatermark{Checkpoint: &r.checkpoint}}, nil
}

// Close closes the fake stream.
//
// Version:
//   - 2026-10-01: Added.
func (r *progressReceiver) Close() {}

// TestRetainedObservationReconnect verifies fresh baselines and frozen disconnect metadata.
//
// Version:
//   - 2026-10-12: Verify disconnected inputs are withdrawn from observers and captures.
//   - 2026-10-01: Added.
func TestRetainedObservationReconnect(t *testing.T) {
	c, reader, _ := cacheFixture(t)
	var frozen, latest *QuoteSnapshot
	c.SetQuoteSnapshotObserver(func(s *QuoteSnapshot) {
		latest = s
		if s != nil {
			o := s.Inputs().Observation
			if o.Active {
				frozen = s
			}
		}
	})
	for epoch := uint64(1); epoch <= 2; epoch++ {
		before := reader.batches
		err := c.RunRetained(t.Context(), progressSubscriber{reader.cp + 1})
		if !errors.Is(err, io.EOF) {
			t.Fatalf("unexpected stream result: %v", err)
		}
		if reader.batches <= before {
			t.Fatal("reconnect reused unverified baseline")
		}
		if frozen == nil || !frozen.Inputs().Observation.Active || frozen.Inputs().Observation.Epoch != epoch {
			t.Fatal("validated epoch not published")
		}
		last := c.stateObservation
		if last.Active || last.Epoch != epoch || last.ConfirmedAt.IsZero() {
			t.Fatal("disconnect metadata lost")
		}
		if latest != nil || c.CaptureQuoteSnapshot() != nil {
			t.Fatal("disconnected inputs remain available")
		}
		if c.observationCheckpoint != uint64(reader.cp+1) {
			t.Fatal("progress-only frame dropped")
		}
	}
}
