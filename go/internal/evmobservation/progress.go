// Package evmobservation proves idle progress independently of log delivery order.
package evmobservation

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/k4k3ru-hub/onchain/go/evm"
)

const Interval = 5 * time.Second
const maxBlocks = 1024

// readFailure distinguishes missing evidence from verified state inconsistency.
// Its cause remains inspectable; background callers can retry without recapture.
type readFailure struct{ cause error }

// Error describes the failed evidence request.
//
// Version:
//   - 2026-10-01: Added.
func (e *readFailure) Error() string { return e.cause.Error() }

// Unwrap preserves the transport or scheduling failure.
//
// Version:
//   - 2026-10-01: Added.
func (e *readFailure) Unwrap() error { return e.cause }

// Start starts optional HTTP log verification and returns a cancel-and-join function.
// Readers without FilterLogs keep their existing event-only observations.
//
// Version:
//   - 2026-10-01: Added.
func Start(ctx context.Context, source any, query ethereum.FilterQuery, read func() (State, Evidence, bool), apply func(State, time.Time) bool) (<-chan error, func()) {
	if ctx == nil {
		failures := make(chan error, 1)
		failures <- fmt.Errorf("failed to watch evm observation: context=null")
		return failures, func() {}
	}
	rpc, ok := source.(Reader)
	if !ok {
		return nil, func() {}
	}
	ctx, cancel := context.WithCancel(ctx)
	errors := make(chan error, 1)
	done := make(chan struct{})
	go func() {
		defer close(done)
		if err := Run(ctx, rpc, query, Interval, read, apply); err != nil {
			errors <- err
		}
	}()
	return errors, func() { cancel(); <-done }
}

type Reader interface {
	LatestHeader(context.Context) (evm.BlockHeader, error)
	HeaderByNumber(context.Context, uint64) (evm.BlockHeader, error)
	FilterLogs(context.Context, ethereum.FilterQuery) ([]types.Log, error)
}

// Cursor describes the last applied log, or a completely applied block.
type Cursor struct {
	Block  uint64
	Hash   common.Hash
	Index  uint
	HasLog bool
}

// State fences asynchronous confirmation against input replacement and reconnects.
type State struct {
	Baseline        Cursor
	Cursor          Cursor
	Epoch, Revision uint64
	ReceivedAt      time.Time
}

// Progress keeps confirmation separate from the source input receipt time.
type Progress struct {
	State State
	At    time.Time
}

// ConfirmedAt returns progress only for the exact retained state that was verified.
//
// Version:
//   - 2026-10-01: Added.
func (p Progress) ConfirmedAt(state State) time.Time {
	if p.State == state && p.At.After(state.ReceivedAt) {
		return p.At
	}
	return state.ReceivedAt
}

// Prove verifies an advancing canonical range has no unapplied matching logs.
// A pending log or unchanged head produces no progress. RPC failures are retryable
// missing evidence; verified inconsistencies require owner recovery.
//
// Version:
//   - 2026-10-01: Distinguish unavailable RPC evidence from inconsistent retained state.
//   - 2026-10-01: Added.
func Prove(ctx context.Context, rpc Reader, query ethereum.FilterQuery, cursor Cursor, applied Evidence) (Cursor, bool, error) {
	const op = "failed to prove evm observation"
	if ctx == nil || rpc == nil || cursor.Hash == (common.Hash{}) || len(query.Addresses) == 0 {
		return Cursor{}, false, fmt.Errorf("%s: input=invalid", op)
	}
	head, err := rpc.LatestHeader(ctx)
	if err != nil {
		return Cursor{}, false, fmt.Errorf("%s: %w", op, &readFailure{err})
	}
	if head.Hash == (common.Hash{}) {
		return Cursor{}, false, fmt.Errorf("%s: head=invalid", op)
	}
	if head.Number < cursor.Block {
		return Cursor{}, false, &readFailure{fmt.Errorf("%s: head=behind", op)}
	}
	check := func(number uint64, hash common.Hash) error {
		h, err := rpc.HeaderByNumber(ctx, number)
		if err != nil {
			return fmt.Errorf("%s: %w", op, &readFailure{err})
		}
		if h.Number != number || h.Hash != hash {
			return fmt.Errorf("%s: block_hash=mismatch block_number=%d", op, number)
		}
		return nil
	}
	if err := check(cursor.Block, cursor.Hash); err != nil {
		return Cursor{}, false, err
	}
	if head.Number == cursor.Block && head.Hash != cursor.Hash {
		return Cursor{}, false, fmt.Errorf("%s: block_hash=mismatch block_number=%d", op, cursor.Block)
	}
	if head.Number == cursor.Block && !cursor.HasLog {
		return cursor, false, nil
	}
	if head.Number-cursor.Block > maxBlocks {
		return Cursor{}, false, fmt.Errorf("%s: range=too_long max_length=%d", op, maxBlocks)
	}
	from := cursor.Block
	if !cursor.HasLog {
		from++
	}
	query.BlockHash = nil
	query.FromBlock, query.ToBlock = new(big.Int).SetUint64(from), new(big.Int).SetUint64(head.Number)
	logs, err := rpc.FilterLogs(ctx, query)
	if err != nil {
		return Cursor{}, false, fmt.Errorf("%s: %w", op, &readFailure{err})
	}
	canonical := make(Evidence, len(logs))
	for _, log := range logs {
		if log.Removed || log.BlockHash == (common.Hash{}) || log.BlockNumber < from || log.BlockNumber > head.Number {
			return Cursor{}, false, fmt.Errorf("%s: log=invalid", op)
		}
		if log.BlockNumber == cursor.Block && log.BlockHash != cursor.Hash {
			return Cursor{}, false, fmt.Errorf("%s: block_hash=mismatch block_number=%d", op, cursor.Block)
		}
		if !applied.contains(log) {
			return Cursor{}, false, nil
		}
		canonical[logKey{log.BlockNumber, log.BlockHash, log.Index}] = fingerprint(log)
	}
	for key, digest := range applied {
		if key.block > head.Number {
			return Cursor{}, false, nil
		}
		if key.block >= from {
			if value, ok := canonical[key]; !ok || value != digest {
				return Cursor{}, false, fmt.Errorf("%s: applied_log=mismatch block_number=%d", op, key.block)
			}
		}
	}
	// The filter and both endpoints must describe the same canonical chain.
	if err := check(cursor.Block, cursor.Hash); err != nil {
		return Cursor{}, false, err
	}
	if err := check(head.Number, head.Hash); err != nil {
		return Cursor{}, false, err
	}
	if err := ctx.Err(); err != nil {
		return Cursor{}, false, fmt.Errorf("%s: %w", op, &readFailure{err})
	}
	return Cursor{Block: head.Number, Hash: head.Hash}, true, nil
}

// Run confirms idle progress in the background, never from a quote request.
// apply must compare the supplied state under the owner's lock before publishing.
//
// Version:
//   - 2026-10-01: Preserve verified prefixes across subsequent streamed updates.
//   - 2026-10-01: Back off unavailable evidence without restarting the pool's log subscription.
//   - 2026-10-01: Added.
func Run(ctx context.Context, rpc Reader, query ethereum.FilterQuery, interval time.Duration, read func() (State, Evidence, bool), apply func(State, time.Time) bool) error {
	if ctx == nil || rpc == nil || interval <= 0 || read == nil || apply == nil {
		return fmt.Errorf("failed to watch evm observation: dependency=invalid")
	}
	timer := time.NewTimer(interval)
	defer timer.Stop()
	retryDelay := interval
	var previous State
	var evidence Evidence
	var frontier Cursor
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-timer.C:
		}
		state, applied, active := read()
		if !active {
			previous = State{}
			timer.Reset(interval)
			continue
		}
		if !continuesProof(previous, state, evidence, applied, frontier) {
			frontier = state.Baseline
		}
		previous, evidence = state, applied
		if state.Cursor.Block < state.Baseline.Block {
			return fmt.Errorf("failed to watch evm observation: position=behind_baseline")
		}
		attempt, cancel := context.WithTimeout(ctx, 10*time.Second)
		next, advanced, err := Prove(attempt, rpc, query, frontier, applied)
		cancel()
		if ctx.Err() != nil {
			return nil
		}
		current, currentEvidence, active := read()
		var unavailable *readFailure
		if errors.As(err, &unavailable) {
			if !active {
				previous = State{}
			}
			// Missing RPC evidence cannot prove a gap in the live stream.
			// Preserve inputs and retry at five seconds up to one minute.
			timer.Reset(retryDelay)
			retryDelay = min(retryDelay*2, 12*interval)
			continue
		}
		if !active {
			previous = State{}
			timer.Reset(interval)
			continue
		}
		if current != state {
			// Newly applied logs after this proof do not undo its verified
			// prefix. Keep it without publishing a timestamp for stale inputs.
			if advanced && continuesProof(state, current, applied, currentEvidence, next) {
				frontier = next
			}
			timer.Reset(interval)
			continue
		}
		if err != nil {
			return fmt.Errorf("failed to watch evm observation: %w", err)
		}
		retryDelay = interval
		timer.Reset(interval)
		if advanced {
			frontier = next
			// apply fences a last-moment input change. Even if publication is
			// rejected, the prefix can be checked against the next read.
			apply(state, time.Now().UTC())
		}
	}
}

func continuesProof(previous, current State, before, after Evidence, frontier Cursor) bool {
	if frontier.Hash == (common.Hash{}) || previous.Baseline != current.Baseline ||
		previous.Epoch != current.Epoch || previous.Revision != current.Revision ||
		current.ReceivedAt.Before(previous.ReceivedAt) {
		return false
	}
	if previous.Cursor == current.Cursor {
		if previous.ReceivedAt != current.ReceivedAt {
			return false // A new receipt without a new source position needs a fresh proof.
		}
	} else if current.Cursor.Block <= frontier.Block || current.Cursor.Block < previous.Cursor.Block ||
		(current.Cursor.Block == previous.Cursor.Block &&
			(current.Cursor.Hash != previous.Cursor.Hash || !current.Cursor.HasLog ||
				(previous.Cursor.HasLog && current.Cursor.Index <= previous.Cursor.Index))) {
		return false
	}
	// A new or changed event in a previously verified block invalidates that
	// prefix. Eviction of old journal entries alone does not change live inputs.
	for key, digest := range after {
		if key.block <= frontier.Block {
			if old, ok := before[key]; !ok || old != digest {
				return false
			}
		}
	}
	return true
}
