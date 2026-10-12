package clmm

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/k4k3ru-hub/onchain/go/sui"
)

type scopedSubscriberFake struct {
	progressSubscriber
	pool        sui.Address
	scope       sui.ObjectStateScope
	onSubscribe func()
}

// SubscribeObjectStateScoped records the selected tick container before initialization.
//
// Version:
//   - 2026-10-12: Added.
func (s *scopedSubscriberFake) SubscribeObjectStateScoped(ctx context.Context, pool sui.Address, scope sui.ObjectStateScope) (*sui.TransactionSubscription, error) {
	s.pool, s.scope = pool, scope
	if s.onSubscribe != nil {
		s.onSubscribe()
	}
	return s.progressSubscriber.SubscribeObjectState(ctx, pool)
}

// TestRetainedScopedSubscription verifies parent discovery, local quotes, and disconnected withdrawal.
//
// Version:
//   - 2026-10-12: Added.
func TestRetainedScopedSubscription(t *testing.T) {
	for _, changed := range []bool{false, true} {
		t.Run(map[bool]string{false: "stable", true: "changed_parent"}[changed], func(t *testing.T) {
			c, reader, p := cacheFixture(t)
			s := &scopedSubscriberFake{progressSubscriber: progressSubscriber{reader.cp + 1}}
			if changed {
				s.onSubscribe = func() {
					reader.obj.Move.JSON = bytes.ReplaceAll(reader.obj.Move.JSON, []byte(`"id":"0x8"`), []byte(`"id":"0xa"`))
				}
			}
			var latest, ready *QuoteSnapshot
			c.SetQuoteSnapshotObserver(func(q *QuoteSnapshot) {
				latest = q
				if q != nil && q.Inputs().Observation.Active {
					ready = q
				}
			})
			err := c.RunRetained(t.Context(), s)
			handle, parseErr := sui.ParseAddress("0x8")
			if parseErr != nil {
				t.Fatal(parseErr)
			}
			if s.pool != c.pool || len(s.scope.ParentIDs) != 1 || s.scope.ParentIDs[0] != handle {
				t.Fatal("wrong subscription scope")
			}
			if changed {
				if err == nil || !strings.Contains(err.Error(), "subscription_scope=changed") || ready != nil {
					t.Fatalf("scope mismatch published inputs: %v", err)
				}
			} else {
				if !errors.Is(err, io.EOF) || ready == nil {
					t.Fatalf("scoped initialization failed: %v", err)
				}
				reads := reader.batches
				if _, err := ready.cache.QuoteRetainedPair(t.Context(), p); err != nil || reader.batches != reads {
					t.Fatalf("frozen quote required reads: %v", err)
				}
			}
			if latest != nil || c.CaptureQuoteSnapshot() != nil {
				t.Fatal("disconnected state remained available")
			}
		})
	}
}
