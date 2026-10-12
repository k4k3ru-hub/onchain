package clmm

import (
	"context"
	"fmt"
	"time"

	"github.com/k4k3ru-hub/onchain/go/sui"
)

func (c *StateCache) subscribeRetainedObjects(ctx context.Context, subscriber ObjectStateSubscriber) (*sui.TransactionSubscription, error) {
	scoped, ok := subscriber.(sui.ScopedObjectStateSubscriber)
	if !ok {
		return subscriber.SubscribeObjectState(ctx, c.pool)
	}
	readCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	head, err := sui.QuoteCheckpoint(readCtx, c.reader, sui.CheckpointSequenceNumber(c.floor.Load()))
	if err != nil {
		return nil, fmt.Errorf("failed to resolve turbos subscription scope: %w", err)
	}
	obj, err := c.reader.ObjectAtCheckpoint(readCtx, c.pool, head.SequenceNumber)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve turbos subscription scope: %w", err)
	}
	if obj == nil || obj.Address != c.pool || obj.Version == 0 {
		return nil, fmt.Errorf("failed to resolve turbos subscription scope: object=invalid")
	}
	state, err := capturePool(obj, head.SequenceNumber)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve turbos subscription scope: %w", err)
	}
	scope := sui.ObjectStateScope{ParentIDs: []sui.Address{state.bitmap, state.ticks}}
	c.retainedMu.Lock()
	c.retainedScope = &scope
	c.retainedMu.Unlock()
	return scoped.SubscribeObjectStateScoped(ctx, c.pool, scope)
}
