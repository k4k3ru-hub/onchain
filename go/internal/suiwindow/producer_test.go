package suiwindow

import (
	"context"
	"github.com/k4k3ru-hub/onchain/go/sui"
	"testing"
	"time"
)

// TestRunAppliesLiveUpdatesDuringCapture verifies replacement IO never blocks live application and replay retains receipt times.
//
// Version:
//   - 2026-09-11: Added.
func TestRunAppliesLiveUpdatesDuringCapture(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	feed := make(chan *sui.TransactionNotification)
	started := make(chan struct{})
	release := make(chan struct{})
	applied := make(chan Update, 1)
	installed := make(chan []Update, 1)
	done := make(chan error, 1)
	missing := true
	go func() {
		done <- Run(ctx, func(ctx context.Context) (*sui.TransactionNotification, error) {
			select {
			case n := <-feed:
				return n, nil
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}, func(u Update) error { applied <- u; return nil }, func() bool { return missing }, func(ctx context.Context) (int, error) {
			close(started)
			select {
			case <-release:
				return 1, nil
			case <-ctx.Done():
				return 0, ctx.Err()
			}
		}, func(value int, replay []Update) error { missing = false; installed <- replay; return nil })
	}()
	select {
	case <-started:
	case <-ctx.Done():
		t.Fatal("capture not started")
	}
	notification := &sui.TransactionNotification{Effects: &sui.TransactionEffects{}}
	select {
	case feed <- notification:
	case <-ctx.Done():
		t.Fatal("producer stopped receiving")
	}
	var live Update
	select {
	case live = <-applied:
	case <-ctx.Done():
		t.Fatal("live application blocked by IO")
	}
	close(release)
	select {
	case replay := <-installed:
		if len(replay) != 1 || replay[0].Notification != notification || replay[0].ReceivedAt != live.ReceivedAt {
			t.Fatal("lost concurrent update")
		}
	case <-ctx.Done():
		t.Fatal("not installed")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("producer goroutines did not stop")
	}
}
