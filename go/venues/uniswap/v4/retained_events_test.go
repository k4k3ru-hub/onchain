package v4

import (
	"context"
	"errors"
	"math/big"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
)

// TestRetainedEventOrder distinguishes replay, conflict, regression and reorg.
//
// Version:
//   - 2026-09-10: Added.
func TestRetainedEventOrder(t *testing.T) {
	cases := []struct {
		name, reason string
		change       func(*types.Log)
	}{
		{"duplicate", "", func(*types.Log) {}},
		{"content", "log content conflict", func(l *types.Log) { l.Data = []byte{1} }},
		{"transaction", "log content conflict", func(l *types.Log) { l.TxHash = common.HexToHash("abc") }},
		{"order", "out of order log", func(l *types.Log) { l.Index-- }},
		{"hash", "stream block hash mismatch", func(l *types.Log) { l.BlockHash = common.HexToHash("def") }},
		{"removed", "removed log", func(l *types.Log) { l.Removed = true }},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			var order retainedLogOrder
			log := testSwapLog(t, common.HexToAddress("05"))
			log.BlockHash = windowHeader(10).Hash
			if duplicate, err := order.accept(log); duplicate || err != nil {
				t.Fatal(duplicate, err)
			}
			tt.change(&log)
			duplicate, err := order.accept(log)
			if tt.reason == "" {
				if !duplicate || err != nil {
					t.Fatal(duplicate, err)
				}
				return
			}
			if duplicate || err == nil || !strings.Contains(err.Error(), tt.reason) {
				t.Fatal(duplicate, err)
			}
		})
	}
	var order retainedLogOrder
	log := testSwapLog(t, common.HexToAddress("05"))
	log.BlockHash = windowHeader(10).Hash
	for i := uint(0); i <= retainedReplayLimit; i++ {
		log.Index = i
		if _, err := order.accept(log); err != nil {
			t.Fatal(err)
		}
	}
	if len(order.seen) != retainedReplayLimit {
		t.Fatal("unbounded history")
	}
	log.Index = 0
	if duplicate, err := order.accept(log); duplicate || err == nil {
		t.Fatal("evicted log guessed to be duplicate")
	}
}

// TestRetainedEventsSurviveRecovery delivers live swaps once while rebuilding inputs.
//
// Version:
//   - 2026-09-10: Added.
func TestRetainedEventsSurviveRecovery(t *testing.T) {
	c, fake := newTestCache(t)
	started, release := make(chan struct{}), make(chan struct{})
	rpc := &windowRPC{fake: fake, height: 100, started: started, release: release}
	c.rpc = rpc
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	ws := &stateWS{ready: make(chan struct{})}
	swaps := make(chan types.Log, 16)
	recoveries := make(chan error, 16)
	updates := make(chan *QuoteSnapshot, 128)
	c.SetQuoteSnapshotObserver(func(s *QuoteSnapshot) { updates <- s })
	subscribed := 0
	done := make(chan error, 1)
	go func() {
		done <- c.RunRetainedWithEvents(ctx, ws, big.NewInt(1000000), true, RetainedEvents{
			OnSubscribed: func() { subscribed++ },
			OnSwapLog: func(_ context.Context, s types.Log, at time.Time) error {
				if at.IsZero() {
					return errors.New("missing receipt")
				}
				swaps <- s
				return nil
			},
			OnRecovery: func(err error) { recoveries <- err },
		})
	}()
	defer func() {
		cancel()
		if err := <-done; err != nil {
			t.Error(err)
		}
		if subscribed != 1 {
			t.Errorf("subscriptions=%d", subscribed)
		}
	}()
	select {
	case <-started:
	case <-ctx.Done():
		t.Fatal("capture did not start")
	}
	live := testSwapLog(t, c.pool)
	live.Topics[1] = c.poolID
	live.BlockNumber = 101
	live.BlockHash = windowHeader(101).Hash
	sqrtAtTick(0).FillBytes(live.Data[64:96])
	integer("1000000000000000000").FillBytes(live.Data[96:128])
	big.NewInt(0).FillBytes(live.Data[128:160])
	ws.logs <- live
	select {
	case <-swaps:
	case <-ctx.Done():
		t.Fatal("bootstrap blocked live swap")
	}
	ws.logs <- live // duplicate while capture is blocked
	removed := live
	removed.Removed = true
	ws.logs <- removed
	select {
	case err := <-recoveries:
		if !strings.Contains(err.Error(), "removed log") {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatal("missing invalidation")
	}
	// An RPC head behind the discarded events must not install an older baseline.
	select {
	case err := <-recoveries:
		if !strings.Contains(err.Error(), "rpc_head=behind") {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatal("lagging recovery baseline was not rejected")
	}
	// Cancellation of the obsolete capture must not close the WS or publish its result.
	rpc.mu.Lock()
	rpc.height = 101
	rpc.mu.Unlock()
	live.BlockNumber = 102
	live.BlockHash = windowHeader(102).Hash
	ws.logs <- live
	select {
	case s := <-swaps:
		if s.BlockNumber != 102 {
			t.Fatal("duplicate was re-emitted")
		}
	case <-ctx.Done():
		t.Fatal("recovery blocked live swap")
	}
	for {
		select {
		case s := <-updates:
			if s == nil {
				continue
			}
			if s.cache.retained.header.Number == 100 {
				t.Fatal("obsolete capture installed")
			}
			if s.cache.retainedBlock == 102 {
				select {
				case <-swaps:
					t.Fatal("replay re-emitted swap")
				default:
				}

				// A recordable late live swap must survive state-order rejection.
				late := live
				late.BlockNumber, late.BlockHash = 101, windowHeader(101).Hash
				late.Index += 100
				ws.logs <- late
				select {
				case got := <-swaps:
					if got.BlockNumber != 101 {
						t.Fatal("late live swap was not delivered")
					}
				case <-ctx.Done():
					t.Fatal("state order validation suppressed live swap")
				}
				return
			}
		case <-ctx.Done():
			t.Fatal("recovery did not replay buffered swap")
		}
	}
}

func testSwapLog(t *testing.T, pool common.Address) types.Log {
	t.Helper()
	log := types.Log{Address: pool, BlockNumber: 10, Index: 2, Topics: []common.Hash{crypto.Keccak256Hash([]byte("Swap(bytes32,address,int128,int128,uint160,uint128,int24,uint24)")), {}, {}}, Data: make([]byte, 192)}
	big.NewInt(3000).FillBytes(log.Data[160:192])
	return log
}
