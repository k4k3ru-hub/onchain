package lpprotection_test

import (
	"context"
	"encoding/json"
	"errors"
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
	"github.com/k4k3ru-hub/onchain/go/evm/lpprotection"
)

type pinnedPrincipalRPC struct {
	*ethclient.Client
	number *big.Int
}

type pacedTransport struct {
	mu       sync.Mutex
	next     time.Time
	attempts int
}

// RoundTrip spaces public RPC requests without hiding retries from the reader.
//
// Version:
//   - 2026-09-23: Added.
func (p *pacedTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	p.mu.Lock()
	wait := time.Until(p.next)
	if wait > 0 {
		timer := time.NewTimer(wait)
		select {
		case <-req.Context().Done():
			timer.Stop()
			p.mu.Unlock()
			return nil, req.Context().Err()
		case <-timer.C:
		}
	}
	p.next = time.Now().Add(2500 * time.Millisecond)
	p.attempts++
	p.mu.Unlock()
	return http.DefaultTransport.RoundTrip(req)
}

// HeaderByNumber pins the reusable principal reader to the fixture block.
//
// Version:
//   - 2026-09-23: Added.
func (p pinnedPrincipalRPC) HeaderByNumber(ctx context.Context, n *big.Int) (*types.Header, error) {
	if n == nil {
		n = p.number
	}
	return p.Client.HeaderByNumber(ctx, n)
}

// TestLPProtectionLive checks pool-first analysis against public Base RPC.
// It reads a frozen historical block, makes no transactions, and is opt-in.
//
// Version:
//   - 2026-09-23: Check receipt reuse and unresolved operator evidence.
func TestLPProtectionLive(t *testing.T) {
	if os.Getenv("ONCHAIN_LP_PROTECTION_LIVE") != "1" {
		t.Skip("set ONCHAIN_LP_PROTECTION_LIVE=1 for read-only Base verification")
	}
	var rows []struct {
		Protocol               clliquidity.Protocol
		Pool                   common.Address
		Spacing                int32
		Creation               types.Log
		BlockNumber            uint64
		BlockHash              common.Hash
		ExpectedToken0Locked   *string
		ExpectedToken1Locked   *string
		ExpectedAllProtected   bool
		ExpectedReason         string
		ExpectedPositionReason string
	}
	b, err := os.ReadFile("testdata/live-pools.json")
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(b, &rows); err != nil {
		t.Fatal(err)
	}
	transport := &pacedTransport{}
	for _, row := range rows {
		t.Run(row.Pool.Hex(), func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
			defer cancel()
			client, err := rpc.DialOptions(ctx, "https://mainnet.base.org", rpc.WithHTTPClient(&http.Client{Transport: transport, Timeout: 20 * time.Second}))
			if err != nil {
				t.Fatal(err)
			}
			defer client.Close()
			raw := ethclient.NewClient(client)
			principal, err := clliquidity.NewReader(pinnedPrincipalRPC{raw, new(big.Int).SetUint64(row.BlockNumber)}, clliquidity.Limits{Timeout: 2 * time.Minute, ChunkSize: 128, MaxReads: 1024, MaxCalls: 32, MaxTicks: 128, MaxResponseBytes: 2 << 20})
			if err != nil {
				t.Fatal(err)
			}
			snapshot, err := principal.Capture(ctx, clliquidity.Pool{Protocol: row.Protocol, Address: row.Pool, Spacing: row.Spacing, Multicall: common.HexToAddress("0xcA11bde05977b3631167028862bE2a173976CA11")})
			if err != nil {
				t.Fatal(err)
			}
			if snapshot.BlockHash != row.BlockHash {
				t.Fatal("historical block mismatch")
			}
			reader, err := lpprotection.NewReader(raw, lpprotection.Limits{Timeout: 2 * time.Minute, MaxCalls: 128, MaxReceipts: 64, LogBlockRange: 1000, MaxLogs: 4096, MaxPositions: 256, MaxResponseBytes: 4 << 20})
			if err != nil {
				t.Fatal(err)
			}
			started := time.Now()
			before := transport.attempts
			request := lpprotection.Request{ChainID: 8453, Protocol: row.Protocol, Creation: row.Creation, Principal: lpprotection.Principal{Pool: row.Pool, Snapshot: snapshot}}
			result, err := reader.Analyze(ctx, request)
			encoded, marshalErr := json.Marshal(result)
			if marshalErr != nil {
				t.Fatal(marshalErr)
			}
			t.Logf("principal_setup_calls=%d lp_elapsed=%s result=%s", snapshot.Calls, time.Since(started), encoded)
			if transport.attempts-before != result.Metrics.Calls {
				t.Fatal("transport attempt count mismatch")
			}
			if result.ModelVersion != lpprotection.ModelVersion || result.Metrics.Methods["eth_getLogs"] != 0 || len(result.PositionIDs) == 0 {
				t.Fatal("creation receipt reuse failed")
			}
			if row.ExpectedReason != "" {
				if !errors.Is(err, lpprotection.ErrBudget) || result.Observation != nil || result.Reason != row.ExpectedReason || len(result.Positions) != 1 || result.Positions[0].Reason != row.ExpectedPositionReason || result.Positions[0].Kind != "" || result.Positions[0].CanWeakenProtection != nil {
					t.Fatalf("unverified operator history must remain unresolved: %+v %v", result, err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			equal := func(a, b *string) bool {
				if a == nil || b == nil {
					return a == nil && b == nil
				}
				return *a == *b
			}
			if result.Observation == nil || !equal(result.Observation.Token0.LockedPercentage, row.ExpectedToken0Locked) || !equal(result.Observation.Token1.LockedPercentage, row.ExpectedToken1Locked) || result.Observation.AllPositionsProtected != row.ExpectedAllProtected {
				t.Fatal("principal protection differs from independent historical fixture")
			}
			// Reuse the discovered index at exactly the same block. No historical
			// mint scan should recur, even though mutable ownership is re-read.
			request.PositionIDs = result.PositionIDs
			cached, err := reader.Analyze(ctx, request)
			if err != nil {
				t.Fatal(err)
			}
			if cached.Observation == nil || cached.Metrics.Methods["eth_getLogs"] != 0 {
				t.Fatalf("index reuse failed: %+v", cached)
			}
			t.Logf("index_reuse_metrics=%+v", cached.Metrics)
		})
	}
}
