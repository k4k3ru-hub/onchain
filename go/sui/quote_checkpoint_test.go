package sui

import (
	"context"
	"errors"
	"github.com/k4k3ru-hub/onchain/go/quotestate"
	"testing"
)

type quoteHeadFake struct{ head Checkpoint }

func (f quoteHeadFake) LatestCheckpoint(context.Context) (Checkpoint, error) { return f.head, nil }

type quoteNumberFake struct {
	quoteHeadFake
	calls  int
	result Checkpoint
	err    error
}

func (f *quoteNumberFake) CheckpointBySequenceNumber(_ context.Context, n CheckpointSequenceNumber) (Checkpoint, error) {
	f.calls++
	return f.result, f.err
}

// TestQuoteCheckpointUsesObservedNumber verifies quote-state recovery invariants.
//
// Version:
//   - 2026-09-09: Added.
func TestQuoteCheckpointUsesObservedNumber(t *testing.T) {
	f := &quoteNumberFake{quoteHeadFake: quoteHeadFake{Checkpoint{SequenceNumber: 10}}, result: Checkpoint{SequenceNumber: 12}}
	got, err := QuoteCheckpoint(context.Background(), f, 12)
	if err != nil || got.SequenceNumber != 12 || f.calls != 1 {
		t.Fatalf("numbered recovery: %v %v", got, err)
	}
	_, err = QuoteCheckpoint(context.Background(), f, 9)
	if err != nil || f.calls != 1 {
		t.Fatal("unnecessary numbered request", err)
	}
	f.result.SequenceNumber = 11
	if _, err = QuoteCheckpoint(context.Background(), f, 12); err == nil {
		t.Fatal("mismatch accepted")
	}
	underlying := errors.New("transport failure")
	f.err = underlying
	_, err = QuoteCheckpoint(context.Background(), f, 12)
	if !errors.Is(err, underlying) || quotestate.IsStateChange(err) {
		t.Fatal("transport classification lost", err)
	}
	_, err = QuoteCheckpoint(context.Background(), f.quoteHeadFake, 12)
	if !quotestate.IsStateChange(err) {
		t.Fatal("lag not retryable", err)
	}
}
