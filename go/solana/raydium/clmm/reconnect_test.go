package clmm

import (
	"context"
	solana "github.com/k4k3ru-hub/onchain/go/solana"
	"testing"
	"time"
)

type idleReconnectStream struct{}

// Recv waits until the test session ends.
func (idleReconnectStream) Recv(ctx context.Context) (solana.Slot, error) {
	<-ctx.Done()
	return 0, ctx.Err()
}

// Close releases the test subscription.
func (idleReconnectStream) Close() {}
func TestBootstrapSurvivesSubscriptionReconfiguration(t *testing.T) {
	c, _ := cacheFixture(t)
	requests := []ExactInputRequest{{InputMint: testAddress(4), AmountIn: 1000000}}
	for attempt := 0; attempt < 2; attempt++ {
		if _, err := c.QuoteExactInputs(context.Background(), requests); err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan error, 1)
		ready := make(chan struct{}, 16)
		c.SetQuoteSnapshotObserver(func(s *QuoteSnapshot) {
			if c.connected == len(c.addresses) {
				ready <- struct{}{}
			}
		})
		go func() {
			done <- c.Run(ctx, func(solana.Address) (AccountChanges, error) { return idleReconnectStream{}, nil })
		}()
		wait := func() {
			select {
			case <-ready:
			case <-time.After(time.Second):
				cancel()
				<-done
				t.Fatal("subscriptions not ready")
			}
		}
		wait()
		c.changed <- struct{}{}
		wait()
		c.mu.Lock()
		frozen := c.retained.Freeze()
		c.mu.Unlock()
		_, quoteErr := c.QuoteRetainedExactInputs(context.Background(), requests)
		cancel()
		if quoteErr != nil {
			t.Fatal(quoteErr)
		}
		if err := <-done; err != nil {
			t.Fatal(err)
		}
		if len(frozen.Accounts) == 0 {
			t.Fatal("subscription replacement erased bootstrapped accounts")
		}
		if len(c.retained.Freeze().Accounts) != 0 {
			t.Fatal("ended session retained accounts")
		}
	}
}
