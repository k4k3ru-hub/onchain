package lpprotection

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/k4k3ru-hub/onchain/go/evm/clliquidity"
)

type creationLiveRPC struct{ *ethclient.Client }

// HeaderByNumber pins principal capture to the independently verified sample.
//
// Version:
//   - 2026-09-23: Added.
func (c creationLiveRPC) HeaderByNumber(ctx context.Context, n *big.Int) (*types.Header, error) {
	if n == nil {
		n = big.NewInt(51596896)
	}
	return c.Client.HeaderByNumber(ctx, n)
}

type creationLiveTransport struct {
	mu       sync.Mutex
	next     time.Time
	attempts int
}

// RoundTrip paces public read-only calls without retrying or replacing evidence.
//
// Version:
//   - 2026-09-23: Added.
func (p *creationLiveTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if wait := time.Until(p.next); wait > 0 {
		timer := time.NewTimer(wait)
		select {
		case <-timer.C:
		case <-req.Context().Done():
			timer.Stop()
			return nil, req.Context().Err()
		}
	}
	p.next = time.Now().Add(2500 * time.Millisecond)
	p.attempts++
	return http.DefaultTransport.RoundTrip(req)
}

// TestCreationRuleLive verifies the production reader against read-only Base RPC,
// first cold and then with its own persisted evidence. A cold timeout remains
// unknown; an explicit later evaluation exercises saved partial progress. It never manufactures a
// full-history response or injects the research probe's positive verdict.
//
// Version:
//   - 2026-09-23: Added.
func TestCreationRuleLive(t *testing.T) {
	if os.Getenv("ONCHAIN_LP_CREATION_LIVE") != "1" {
		t.Skip("opt-in public Base read-only verification")
	}
	_, req := creationSample(t)
	transport := &creationLiveTransport{}
	ctx, cancel := context.WithTimeout(context.Background(), 7*time.Minute)
	defer cancel()
	client, err := rpc.DialOptions(ctx, "https://mainnet.base.org", rpc.WithHTTPClient(&http.Client{Transport: transport, Timeout: 20 * time.Second}))
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	raw := ethclient.NewClient(client)
	principal, err := clliquidity.NewReader(creationLiveRPC{raw}, clliquidity.Limits{Timeout: 2 * time.Minute, ChunkSize: 128, MaxReads: 1024, MaxCalls: 32, MaxTicks: 128, MaxResponseBytes: 2 << 20})
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := principal.Capture(ctx, clliquidity.Pool{Protocol: clliquidity.Slipstream, Address: req.Principal.Pool, Spacing: 200, Multicall: common.HexToAddress("0xcA11bde05977b3631167028862bE2a173976CA11")})
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.BlockHash != req.Principal.Snapshot.BlockHash {
		t.Fatal("historical snapshot mismatch")
	}
	req.Principal.Snapshot = snapshot
	lim := limits()
	lim.LogBlockRange = 1000
	reader, err := NewReaderWithCreationRPC(raw, raw, lim)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("principal_setup_calls=%d", snapshot.Calls)
	completed := false
	for evaluation := 0; evaluation < 3; evaluation++ {
		phase := "cold"
		if evaluation > 0 {
			phase = "resume"
		}
		if completed {
			phase = "restored"
		}
		started := time.Now()
		before := transport.attempts
		result, err := reader.Analyze(ctx, req)
		b, encodeErr := json.Marshal(result.Evidence)
		if encodeErr != nil {
			t.Fatal(encodeErr)
		}
		t.Logf("phase=%s elapsed=%s metrics=%+v evidence_bytes=%d observation=%+v error=%v", phase, time.Since(started), result.Metrics, len(b), result.Observation, err)
		if err != nil {
			if errors.Is(err, context.DeadlineExceeded) && result.Evidence != nil && result.Observation == nil {
				req.Evidence = result.Evidence
				continue
			}
			t.Fatal(err)
		}
		if transport.attempts-before != result.Metrics.Calls {
			t.Fatal("actual RPC accounting mismatch")
		}
		if result.Observation == nil || !result.Observation.AllPositionsProtected || result.Observation.Token0.LockedPercentage != nil || result.Observation.Token1.LockedPercentage == nil || *result.Observation.Token1.LockedPercentage != "100" || result.Observation.CanWeakenProtection == nil || !*result.Observation.CanWeakenProtection || result.Observation.EarliestUnlockAt.Unix() != 4294967295 {
			t.Fatalf("unexpected result: %+v", result)
		}
		if phase == "restored" && (result.Metrics.Methods["eth_getLogs"] != 0 || result.Metrics.Methods["eth_getTransactionReceipt"] != 0 || result.Metrics.Methods["eth_getTransactionByHash"] != 0 || result.Metrics.Methods["eth_getTransactionCount"] != 0) {
			t.Fatal("reused evidence reacquired")
		}
		req.Evidence, err = RestoreEvidence(b, lim.MaxResponseBytes)
		if err != nil {
			t.Fatal(err)
		}
		req.CreationHints = nil
		t.Log(fmt.Sprintf("token0=null token1_locked=%s token1_permanent=%s all_positions_protected=%t can_weaken=%t unlock=%s", *result.Observation.Token1.LockedPercentage, *result.Observation.Token1.PermanentlyProtectedPercentage, result.Observation.AllPositionsProtected, *result.Observation.CanWeakenProtection, result.Observation.EarliestUnlockAt))
		if phase == "restored" {
			return
		}
		completed = true
	}
	t.Fatal("bounded evaluations did not finish the restored-evidence check")
}
