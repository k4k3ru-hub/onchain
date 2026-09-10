package suiwindow

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/k4k3ru-hub/onchain/go/sui"
)

type Update struct {
	Notification *sui.TransactionNotification
	ReceivedAt   time.Time
}

// Run continues applying live objects while a detached replacement is acquired.
// Failed captures retain live inputs and retry with bounded backoff. Installation
// must replay Updates and reject regressions before publishing a replacement.
//
// Version:
//   - 2026-09-11: Added.
func Run[T any](ctx context.Context, recv func(context.Context) (*sui.TransactionNotification, error), apply func(Update) error, missing func() bool, capture func(context.Context) (T, error), install func(T, []Update) error) error {
	ctx, cancel := context.WithCancel(ctx)
	var workers sync.WaitGroup
	defer func() { cancel(); workers.Wait() }()
	type received struct {
		update Update
		err    error
	}
	updates := make(chan received, 1)
	workers.Add(1)
	go func() {
		defer workers.Done()
		for {
			n, err := recv(ctx)
			item := received{Update{n, time.Now().UTC()}, err}
			select {
			case updates <- item:
			case <-ctx.Done():
				return
			}
			if err != nil {
				return
			}
		}
	}()
	type result struct {
		value T
		err   error
	}
	done := make(chan result, 1)
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	var replay []Update
	var running, overflow bool
	var lastErr error
	var next time.Time
	backoff := time.Second
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case item := <-updates:
			if item.err != nil {
				return errors.Join(item.err, lastErr)
			}
			if item.update.Notification == nil || item.update.Notification.Effects == nil {
				continue
			}
			if err := apply(item.update); err != nil {
				return err
			}
			if running && !overflow {
				if len(replay) >= 4096 {
					overflow = true
					replay = nil
				} else {
					replay = append(replay, item.update)
				}
			}
		case <-ticker.C:
			if running || time.Now().Before(next) || !missing() {
				continue
			}
			running, overflow, replay = true, false, nil
			workers.Add(1)
			go func() {
				defer workers.Done()
				captureCtx, stop := context.WithTimeout(ctx, 30*time.Second)
				defer stop()
				value, err := capture(captureCtx)
				done <- result{value, err}
			}()
		case result := <-done:
			running = false
			lastErr = result.err
			if lastErr == nil && overflow {
				lastErr = fmt.Errorf("failed to install sui tick window: replay=too_long")
			}
			if lastErr == nil {
				lastErr = install(result.value, replay)
			}
			replay = nil
			if lastErr != nil {
				next = time.Now().Add(backoff)
				backoff = min(30*time.Second, backoff*2)
			} else {
				next = time.Time{}
				backoff = time.Second
			}
		}
	}
}
