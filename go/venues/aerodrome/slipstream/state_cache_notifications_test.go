package slipstream

import (
	"context"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"math/big"
	"strings"
	"testing"
	"time"
)

// TestConcurrentCapturedBlockNotifications verifies safe adoption and conservative rejection.
//
// Version:
//   - 2026-09-08: Added.
func TestConcurrentCapturedBlockNotifications(t *testing.T) {
	same := types.Log{BlockNumber: 100, BlockHash: common.HexToHash("01")}
	newer := types.Log{BlockNumber: 101, BlockHash: common.HexToHash("02")}
	removed := same
	removed.Removed = true
	wrong := same
	wrong.BlockHash = common.HexToHash("03")
	missing := same
	missing.BlockHash = common.Hash{}
	older := same
	older.BlockNumber = 99
	for _, tc := range []struct {
		name               string
		logs               []types.Log
		disconnect, reject bool
	}{
		{"same_block", []types.Log{same}, false, false},
		{"pool_and_factory", []types.Log{same, same}, false, false},
		{"newer", []types.Log{newer}, false, true},
		{"newer_then_same", []types.Log{newer, same}, false, true},
		{"removed_then_same", []types.Log{removed, same}, false, true},
		{"wrong_then_same", []types.Log{wrong, same}, false, true},
		{"missing_hash", []types.Log{missing}, false, true},
		{"older_unverified", []types.Log{older}, false, true},
		{"disconnect", []types.Log{same}, true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, f := newTestCache(t)
			ctx, cancel := context.WithCancel(context.Background())
			ws := &stateWS{ready: make(chan struct{})}
			done := make(chan error, 1)
			go func() { done <- c.Run(ctx, ws) }()
			defer func() {
				cancel()
				if err := <-done; err != nil {
					t.Error(err)
				}
			}()
			<-ws.ready
			wait := func(check func() bool) {
				t.Helper()
				deadline := time.Now().Add(time.Second)
				for {
					c.mu.Lock()
					ok := check()
					c.mu.Unlock()
					if ok {
						return
					}
					if time.Now().After(deadline) {
						t.Fatal("notification timeout")
					}
					time.Sleep(time.Millisecond)
				}
			}
			wait(func() bool { return c.active })
			f.hook = func() {
				f.hook = nil
				for i, log := range tc.logs {
					log.Address = c.pool
					if i%2 == 1 {
						log.Address = c.factory
					}
					c.mu.Lock()
					gen := c.generation
					c.mu.Unlock()
					ws.logs <- log
					wait(func() bool { return c.generation > gen })
				}
				if tc.disconnect {
					cancel()
					wait(func() bool { return !c.active })
				}
			}
			pair, err := c.QuotePair(context.Background(), big.NewInt(1000000), true)
			stats := c.Diagnostics()
			if tc.name == "pool_and_factory" && (stats["notification.pool.no_snapshot"] != 1 || stats["notification.factory.no_snapshot"] != 1) {
				t.Fatalf("notification sources mixed: %v", stats)
			}
			if stats["refresh.no_snapshot"] != 1 {
				t.Fatalf("missing refresh reason: %v", stats)
			}
			if tc.reject {
				reason := map[string]string{"newer": "newer_block", "newer_then_same": "mixed_blocks", "removed_then_same": "removed", "wrong_then_same": "hash_mismatch", "missing_hash": "missing_hash", "older_unverified": "older_block", "disconnect": "lifecycle"}[tc.name]
				if stats["invalidation.total"] != 1 || stats["invalidation."+reason] != 1 {
					t.Fatalf("missing rejection reason: %v", stats)
				}
			} else if stats["quote.concurrent_notifications_accepted"] != 1 {
				t.Fatalf("missing adoption: %v", stats)
			}
			stats["refresh.no_snapshot"] = 999
			if c.Diagnostics()["refresh.no_snapshot"] != 1 {
				t.Fatal("mutable counters escaped")
			}
			if tc.reject {
				if err == nil || !strings.Contains(err.Error(), "snapshot=invalidated") {
					t.Fatalf("expected invalidation: %v", err)
				}
				if c.snapshot != nil {
					t.Fatal("rejected snapshot retained")
				}
			} else {
				if err != nil {
					t.Fatal(err)
				}
				if pair.BlockNumber != 100 || pair.BlockHash != same.BlockHash {
					t.Fatal("wrong provenance")
				}
				calls := f.calls
				reused, err := c.QuotePair(context.Background(), big.NewInt(1000000), true)
				if err != nil {
					t.Fatal(err)
				}
				if f.calls != calls || !reused.ObservedAt.Equal(pair.ObservedAt) || reused.BidAmountOut.Cmp(pair.BidAmountOut) != 0 || reused.AskAmountIn.Cmp(pair.AskAmountIn) != 0 {
					t.Fatal("snapshot not reused coherently")
				}
			}
			c.mu.Lock()
			pending := c.notifications != nil
			c.mu.Unlock()
			if pending {
				t.Fatal("quote observations leaked")
			}
		})
	}
}
