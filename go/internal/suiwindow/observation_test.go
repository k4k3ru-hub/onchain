package suiwindow

import (
	"context"
	"errors"
	"github.com/k4k3ru-hub/onchain/go/quotestate"
	"github.com/k4k3ru-hub/onchain/go/sui"
	"testing"
	"time"
)

// TestObservationProgress requires an advancing watermark and preserves trade-time absence.
//
// Version:
//   - 2026-10-01: Added.
func TestObservationProgress(t *testing.T) {
	at := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	obs := quotestate.Observation{Epoch: 1, Active: true}
	var last uint64
	cp := sui.CheckpointSequenceNumber(100)
	n := &sui.TransactionNotification{Watermark: sui.EventWatermark{Checkpoint: &cp}}
	if err := ObserveProgress(&obs, &last, n, 100, at); err != nil {
		t.Fatal(err)
	}
	if !obs.ConfirmedAt.Equal(at) || !obs.SourceTime.IsZero() || last != 100 {
		t.Fatal("progress not preserved")
	}
	if err := ObserveProgress(&obs, &last, n, 100, at.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if !obs.ConfirmedAt.Equal(at) {
		t.Fatal("duplicate checkpoint extended coverage")
	}
	cp = 99
	if err := ObserveProgress(&obs, &last, n, 100, at); err == nil {
		t.Fatal("regression accepted")
	}
	cp = 101
	obs.Active = false
	if err := ObserveProgress(&obs, &last, n, 100, at.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if last != 100 {
		t.Fatal("inactive epoch confirmed")
	}
}

// TestRunDeliversProgress proves progress-only notifications reach the retained-state consumer.
//
// Version:
//   - 2026-10-01: Added.
func TestRunDeliversProgress(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	cp := sui.CheckpointSequenceNumber(10)
	n := &sui.TransactionNotification{Watermark: sui.EventWatermark{Checkpoint: &cp}}
	sent := false
	stop := errors.New("done")
	err := Run(ctx, func(ctx context.Context) (*sui.TransactionNotification, error) {
		if !sent {
			sent = true
			return n, nil
		}
		<-ctx.Done()
		return nil, ctx.Err()
	}, func(u Update) error {
		if u.Notification != n || u.ReceivedAt.IsZero() {
			t.Error("progress dropped")
		}
		return stop
	}, func() bool { return false }, func(context.Context) (int, error) { return 0, nil }, func(int, []Update) error { return nil })
	if !errors.Is(err, stop) {
		t.Fatalf("progress not applied: %v", err)
	}
}
