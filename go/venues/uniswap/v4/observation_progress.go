package v4

import (
	"context"
	"github.com/ethereum/go-ethereum"
	"github.com/k4k3ru-hub/onchain/go/internal/evmobservation"
	"time"
)

func (c *StateCache) priceObservationStateLocked() evmobservation.State {
	baseline := evmobservation.Cursor{Block: c.retained.header.Number, Hash: c.retained.header.Hash}
	position := baseline
	if c.retainedHasLog {
		position = evmobservation.Cursor{Block: c.retainedBlock, Hash: c.retainedHash, Index: c.retainedIndex, HasLog: c.retainedHasLog}
	}
	return evmobservation.State{Baseline: baseline, Cursor: position, Epoch: c.generation, ReceivedAt: c.retained.received}
}

func (c *StateCache) observeIdleProgress(ctx context.Context, query ethereum.FilterQuery) (<-chan error, func()) {
	return evmobservation.Start(ctx, c.rpc, query, c.readPriceObservation, c.confirmPriceObservation)
}
func (c *StateCache) readPriceObservation() (evmobservation.State, evmobservation.Evidence, bool) {

	c.mu.Lock()
	defer c.mu.Unlock()
	if !(c.running && c.active && c.priceObservationActive && c.retained != nil) {
		return evmobservation.State{}, nil, false
	}
	return c.priceObservationStateLocked(), c.observationJournal.Snapshot(), true
}
func (c *StateCache) confirmPriceObservation(state evmobservation.State, at time.Time) bool {

	c.mu.Lock()
	defer c.mu.Unlock()
	if !(c.running && c.active && c.priceObservationActive && c.retained != nil) || c.priceObservationStateLocked() != state {
		return false
	}
	c.observationProgress = evmobservation.Progress{State: state, At: at}
	c.publishQuoteSnapshotLocked()
	return true
}
