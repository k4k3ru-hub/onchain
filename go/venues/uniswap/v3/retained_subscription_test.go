package v3

import (
	"context"
	"errors"
	"math/big"
	"sync"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/k4k3ru-hub/onchain/go/evm"
)

// TestRetainedThreeWordWindow verifies bitmap bounds and edge detection.
//
// Version:
//   - 2026-09-11: Verify center ±1 acquisition and recentering.
func TestRetainedThreeWordWindow(t *testing.T) {
	c, rpc := newTestCache(t)
	s, err := c.captureRetainedWindow(context.Background(), big.NewInt(1000000), true, 0, common.Hash{})
	if err != nil {
		t.Fatal(err)
	}
	if len(s.words) != 3 || s.words[-1] == nil || s.words[0] == nil || s.words[1] == nil {
		t.Fatalf("unexpected bitmap window: %v", s.words)
	}
	if rpc.calls != 6 { // slot0, liquidity, spacing, and three bitmap reads.
		t.Fatalf("contract calls = %d, want 6", rpc.calls)
	}
	c.retained = s
	for _, tick := range []int32{-1, 0, 15359} {
		// A negative tick is in word -1, so its lower spare is already absent.
		c.retained.tick = tick
		c.retained.price = sqrtAtTick(tick)
		want := tick < 0
		if got := c.retainedNeedsCapture(big.NewInt(1000000), true); got != want {
			t.Fatalf("tick=%d needs_capture=%t want=%t", tick, got, want)
		}
	}
	for _, tick := range []int32{-15361, 15360, 120000} {
		c.retained.tick = tick
		c.retained.price = sqrtAtTick(tick)
		if !c.retainedNeedsCapture(big.NewInt(1000000), true) {
			t.Fatalf("tick=%d did not request a new window", tick)
		}
	}
}

// TestRetainedWindowRejectsReorg rejects inconsistent baseline hashes.
//
// Version:
//   - 2026-09-10: Added.
func TestRetainedWindowRejectsReorg(t *testing.T) {
	c, rpc := newTestCache(t)
	rpc.reorg = true
	if _, err := c.captureRetainedWindow(context.Background(), big.NewInt(1000000), true, 0, common.Hash{}); err == nil {
		t.Fatal("accepted a window whose baseline changed")
	}
}

// TestRetainedWindowJumpFetchesNewNeighborhood verifies large moves do not scan intermediate words.
//
// Version:
//   - 2026-09-11: Verify three words around the destination.
func TestRetainedWindowJumpFetchesNewNeighborhood(t *testing.T) {
	c, fake := newTestCache(t)
	c.rpc = &windowRPC{fake: fake, height: 100, tick: 120000}
	s, err := c.captureRetainedWindow(context.Background(), big.NewInt(1000000), true, 0, common.Hash{})
	if err != nil {
		t.Fatal(err)
	}
	if len(s.words) != 3 || s.words[6] == nil || s.words[7] == nil || s.words[8] == nil || fake.calls != 6 {
		t.Fatalf("jump scanned unwanted words: words=%v calls=%d", s.words, fake.calls)
	}
}

// TestRetainedCaptureCancellationReleasesSession verifies a blocked reader stops with the subscription.
//
// Version:
//   - 2026-09-10: Added.
func TestRetainedCaptureCancellationReleasesSession(t *testing.T) {
	c, fake := newTestCache(t)
	started := make(chan struct{})
	c.rpc = &windowRPC{fake: fake, height: 100, started: started, release: make(chan struct{})}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ws := &stateWS{ready: make(chan struct{})}
	done := make(chan error, 1)
	go func() { done <- c.RunRetained(ctx, ws, big.NewInt(1000000), true) }()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("capture did not start")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("blocked capture outlived session")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.running || c.retained != nil {
		t.Fatal("canceled session retained active state")
	}
}

type windowRPC struct {
	mu      sync.Mutex
	fake    *stateFake
	height  uint64
	tick    int32
	started chan struct{}
	release chan struct{}
	failure error
}

// LatestHeader returns the controlled chain tip.
//
// Version:
//   - 2026-09-10: Added.
func (r *windowRPC) LatestHeader(context.Context) (evm.BlockHeader, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return windowHeader(r.height), nil
}

// HeaderByNumber returns the controlled canonical hash.
//
// Version:
//   - 2026-09-10: Added.
func (r *windowRPC) HeaderByNumber(_ context.Context, number uint64) (evm.BlockHeader, error) {
	return windowHeader(number), nil
}

func windowHeader(number uint64) evm.BlockHeader {
	return evm.BlockHeader{Number: number, Hash: common.BigToHash(new(big.Int).SetUint64(number))}
}

// CallContract reads a pinned fixture and can suspend a refresh while logs arrive.
//
// Version:
//   - 2026-09-10: Support a transient capture failure.
//   - 2026-09-10: Added.
func (r *windowRPC) CallContract(ctx context.Context, msg ethereum.CallMsg, block *big.Int) ([]byte, error) {
	r.mu.Lock()
	start, release := r.started, r.release
	r.started = nil
	tick := r.tick
	r.mu.Unlock()
	if start != nil {
		close(start)
		select {
		case <-release:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.failure != nil {
		err := r.failure
		r.failure = nil
		return nil, err
	}
	data, err := r.fake.CallContract(ctx, msg, block)
	if err != nil {
		return nil, err
	}
	if string(msg.Data[:4]) == string(crypto.Keccak256([]byte("slot0()"))[:4]) {
		sqrtAtTick(tick).FillBytes(data[:32])
		n := big.NewInt(int64(tick))
		if tick < 0 {
			n.Add(n, power2(256))
		}
		n.FillBytes(data[32:64])
	}
	return data, nil
}

// TestRetainedMissingCoverageRetriesWithoutRestart verifies quote-triggered repair survives an RPC failure.
//
// Version:
//   - 2026-09-10: Added.
func TestRetainedMissingCoverageRetriesWithoutRestart(t *testing.T) {
	c, fake := newTestCache(t)
	rpc := &windowRPC{fake: fake, height: 100}
	c.rpc = rpc
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	updates := make(chan struct{}, 128)
	c.SetQuoteSnapshotObserver(func(*QuoteSnapshot) {
		select {
		case updates <- struct{}{}:
		default:
		}
	})
	waitState := func(check func() bool) {
		t.Helper()
		for {
			c.mu.Lock()
			ok := check()
			c.mu.Unlock()
			if ok {
				return
			}
			select {
			case <-updates:
			case <-ctx.Done():
				t.Fatal("state recovery timed out")
			}
		}
	}
	ws := &stateWS{ready: make(chan struct{})}
	done := make(chan error, 1)
	go func() { done <- c.RunRetained(ctx, ws, big.NewInt(1000000), true) }()
	defer func() {
		cancel()
		if err := <-done; err != nil {
			t.Error(err)
		}
	}()
	<-ws.ready
	waitState(func() bool { return c.retained != nil })
	c.mu.Lock()
	source, generation := c.retained, c.generation
	delete(source.words, 0)
	c.mu.Unlock()
	started, release := make(chan struct{}), make(chan struct{})
	rpc.mu.Lock()
	rpc.started, rpc.release, rpc.failure = started, release, errors.New("temporary rpc failure")
	before := fake.calls
	rpc.mu.Unlock()
	if _, err := c.QuoteRetainedPair(ctx, big.NewInt(1000000), true); !errors.Is(err, errStateReadBudget) {
		t.Fatalf("missing coverage did not fail locally: %v", err)
	}
	select {
	case <-started:
	case <-ctx.Done():
		t.Fatal("quote did not request capture")
	}
	rpc.mu.Lock()
	after := fake.calls
	rpc.mu.Unlock()
	if after != before {
		t.Fatal("quote fetched missing inputs while capture was blocked")
	}
	c.mu.Lock()
	retained := c.retained == source && c.running && c.active
	c.mu.Unlock()
	if !retained {
		t.Fatal("repair discarded the streamed state")
	}
	close(release)
	waitState(func() bool { return c.retained != source && c.retained != nil && c.retained.words[0] != nil })
	c.mu.Lock()
	sameSession := c.running && c.active && c.generation == generation
	c.mu.Unlock()
	if !sameSession {
		t.Fatal("coverage repair restarted the subscription")
	}
	rpc.mu.Lock()
	defer rpc.mu.Unlock()
	before = fake.calls
	if _, err := c.QuoteRetainedPair(ctx, big.NewInt(1000000), true); err != nil {
		t.Fatalf("quote remained unavailable after retry: %v", err)
	}
	if fake.calls != before {
		t.Fatal("recovered quote made an RPC call")
	}
}

// TestRetainedWindowRefreshKeepsStreamAndReplaysDeltas verifies nonblocking refresh and liquidity replay.
//
// Version:
//   - 2026-09-10: Added.
func TestRetainedWindowRefreshKeepsStreamAndReplaysDeltas(t *testing.T) {
	c, fake := newTestCache(t)
	rpc := &windowRPC{fake: fake, height: 100}
	c.rpc = rpc
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	updates := make(chan struct{}, 128)
	c.SetQuoteSnapshotObserver(func(*QuoteSnapshot) {
		select {
		case updates <- struct{}{}:
		default:
		}
	})
	waitState := func(check func() bool) {
		t.Helper()
		for {
			c.mu.Lock()
			ok := check()
			c.mu.Unlock()
			if ok {
				return
			}
			select {
			case <-updates:
			case <-ctx.Done():
				t.Fatal("state update timed out")
			}
		}
	}
	ws := &stateWS{ready: make(chan struct{})}
	done := make(chan error, 1)
	go func() { done <- c.RunRetained(ctx, ws, big.NewInt(1000000), true) }()
	defer func() {
		cancel()
		if err := <-done; err != nil {
			t.Error(err)
		}
	}()
	<-ws.ready
	waitState(func() bool { return c.retained != nil })
	rpc.mu.Lock()
	rpc.height, rpc.tick = 101, 15360
	started, release := make(chan struct{}), make(chan struct{})
	rpc.started, rpc.release = started, release
	rpc.mu.Unlock()
	log := testSwapLog(t, c.pool)
	log.BlockNumber, log.BlockHash = 101, windowHeader(101).Hash
	sqrtAtTick(15360).FillBytes(log.Data[64:96])
	integer("1000000000000000000").FillBytes(log.Data[96:128])
	big.NewInt(15360).FillBytes(log.Data[128:160])
	ws.logs <- log
	select {
	case <-started:
	case <-ctx.Done():
		t.Fatal("edge did not start refresh")
	}
	// The old window covers the current word, even though its spare is absent.
	if _, err := c.QuoteRetainedPair(ctx, big.NewInt(1000000), true); err != nil {
		t.Fatalf("usable state was withdrawn during refresh: %v", err)
	}
	mint := liquidityLog(c, true, 15300, 15420, 100)
	mint.BlockNumber, mint.BlockHash = 102, windowHeader(102).Hash
	ws.logs <- mint
	waitState(func() bool { return c.retained != nil && c.retainedBlock == 102 })
	close(release)
	waitState(func() bool { return c.retained != nil && c.retained.header.Number == 101 })
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.retained.words[2] == nil || c.retainedBlock != 102 || c.retained.liquidity.Cmp(integer("1000000000000000100")) != 0 {
		t.Errorf("refresh lost stream deltas: block=%d liquidity=%s", c.retainedBlock, c.retained.liquidity)
	}
}
