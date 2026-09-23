//go:build lp_operator_research

package operatorproof

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/k4k3ru-hub/onchain/go/evm/clliquidity"
	"github.com/k4k3ru-hub/onchain/go/evm/lpprotection"
)

type measuredTransport struct {
	mu    sync.Mutex
	next  time.Time
	Calls []map[string]any
}

// RoundTrip records actual read-only RPC sends without hidden retries.
//
// Version:
//   - 2026-09-23: Added for isolated research.
func (m *measuredTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.Calls) >= 64 {
		return nil, fmt.Errorf("failed to collect research evidence: calls=too_long")
	}
	if wait := time.Until(m.next); wait > 0 {
		timer := time.NewTimer(wait)
		select {
		case <-timer.C:
		case <-req.Context().Done():
			timer.Stop()
			return nil, req.Context().Err()
		}
	}
	m.next = time.Now().Add(2500 * time.Millisecond)
	b, err := io.ReadAll(io.LimitReader(req.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	if err := req.Body.Close(); err != nil {
		return nil, err
	}
	req.Body = io.NopCloser(bytes.NewReader(b))
	var call map[string]any
	if err := json.Unmarshal(b, &call); err != nil {
		return nil, err
	}
	switch call["method"] {
	case "eth_chainId", "eth_getBlockByNumber", "eth_call", "eth_getCode", "eth_getLogs", "eth_getTransactionReceipt":
	default:
		return nil, fmt.Errorf("failed to collect research evidence: method=invalid")
	}
	m.Calls = append(m.Calls, call)
	response, err := http.DefaultTransport.RoundTrip(req)
	if err != nil {
		return nil, err
	}
	raw, err := io.ReadAll(io.LimitReader(response.Body, 4<<20))
	if err != nil {
		return nil, fmt.Errorf("failed to read research rpc response: %w", errors.Join(err, response.Body.Close()))
	}
	if err := response.Body.Close(); err != nil {
		return nil, err
	}
	response.Body = io.NopCloser(bytes.NewReader(raw))
	call["httpStatus"] = response.StatusCode
	call["responseBytes"] = len(raw)
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, err
	}
	call["response"] = value
	return response, nil
}

type pinnedRPC struct {
	*ethclient.Client
	number *big.Int
}

// HeaderByNumber pins principal capture to the historical observation.
//
// Version:
//   - 2026-09-23: Added for isolated research.
func (p pinnedRPC) HeaderByNumber(ctx context.Context, n *big.Int) (*types.Header, error) {
	if n == nil {
		n = p.number
	}
	return p.Client.HeaderByNumber(ctx, n)
}

type provenRPC struct {
	*ethclient.Client
	manager, owner common.Address
	number         uint64
	cacheHits      int
}

// ChainID reuses the public endpoint's already verified identity.
//
// Version:
//   - 2026-09-23: Added for isolated research.
func (p *provenRPC) ChainID(context.Context) (*big.Int, error) {
	p.cacheHits++
	return big.NewInt(8453), nil
}

// FilterLogs materializes one complete empty operator history using the birth
// proof and seven cached ranges validated by TestCreationProof in this process.
// This single-sample adapter is not shipped as an SDK feature.
//
// Version:
//   - 2026-09-23: Added for isolated research.
func (p *provenRPC) FilterLogs(_ context.Context, q ethereum.FilterQuery) ([]types.Log, error) {
	if q.FromBlock == nil || q.ToBlock == nil || q.FromBlock.Sign() != 0 || !q.ToBlock.IsUint64() || q.ToBlock.Uint64() != p.number || len(q.Addresses) != 1 || q.Addresses[0] != p.manager || len(q.Topics) != 2 || len(q.Topics[0]) != 1 || len(q.Topics[1]) != 1 || q.Topics[0][0] != crypto.Keccak256Hash([]byte("ApprovalForAll(address,address,bool)")) || q.Topics[1][0] != common.BytesToHash(p.owner[:]) {
		return nil, fmt.Errorf("failed to reuse operator proof: identity=invalid")
	}
	p.cacheHits++
	return []types.Log{}, nil
}

// TestProtectedReader calculates principal protection after validating the birth
// and history evidence, then rereading mutable state at the same historical hash.
//
// Version:
//   - 2026-09-23: Added for isolated research.
func TestProtectedReader(t *testing.T) {
	if os.Getenv("ONCHAIN_LP_OPERATOR_RESEARCH_LIVE") != "1" {
		t.Skip("opt-in read-only RPC test")
	}
	TestCreationProof(t)
	var evidence struct{ Results map[string]record }
	load(t, "rpc-evidence.json", &evidence)
	r := evidence.Results
	pr := decode[types.Receipt](t, r, "pool_receipt")
	manager := common.HexToAddress("0xe1f8cd9ac4e4a65f54f38a5cdafca44f6dd68b53")
	owner := common.HexToAddress("0xdbf7a3d301e39871e6644a17fb1af1a3889078af")
	pool := common.HexToAddress("0x197A0913aCC071cc6D0B5611A72b65F5838797eE")
	var creation types.Log
	for _, l := range pr.Logs {
		if l.Address == common.HexToAddress("0xf8f2eb4940cfe7d13603dddd87f123820fc061ef") && len(l.Topics) == 4 && l.Topics[0] == crypto.Keccak256Hash([]byte("PoolCreated(address,address,int24,address)")) && common.BytesToAddress(l.Data) == pool {
			creation = *l
		}
	}
	if creation.TxHash == (common.Hash{}) {
		t.Fatal("creation event missing")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	transport := &measuredTransport{}
	defer func() {
		b, err := json.MarshalIndent(transport.Calls, "", "  ")
		if err != nil {
			t.Error(err)
			return
		}
		if err := os.WriteFile(filepath.Join(directory(), "reader-rpc.json"), append(b, '\n'), 0644); err != nil {
			t.Error(err)
		}
	}()
	client, err := rpc.DialOptions(ctx, "https://mainnet.base.org", rpc.WithHTTPClient(&http.Client{Transport: transport, Timeout: 20 * time.Second}))
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	raw := ethclient.NewClient(client)
	principal, err := clliquidity.NewReader(pinnedRPC{raw, big.NewInt(51596896)}, clliquidity.Limits{Timeout: 2 * time.Minute, ChunkSize: 128, MaxReads: 1024, MaxCalls: 32, MaxTicks: 128, MaxResponseBytes: 2 << 20})
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := principal.Capture(ctx, clliquidity.Pool{Protocol: clliquidity.Slipstream, Address: pool, Spacing: 200, Multicall: common.HexToAddress("0xcA11bde05977b3631167028862bE2a173976CA11")})
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.BlockHash != common.HexToHash("0xacc268320c2198258d4b13b4fb4f90bb810d626b9329fe58ac60e52c55796ba2") {
		t.Fatal("snapshot mismatch")
	}
	adapter := &provenRPC{Client: raw, manager: manager, owner: owner, number: 51596896}
	// A synthetic full-range query is served entirely by verified evidence.
	// This range must never be forwarded to public RPC or configured in production.
	reader, err := lpprotection.NewReader(adapter, lpprotection.Limits{Timeout: 2 * time.Minute, MaxCalls: 128, MaxReceipts: 64, LogBlockRange: 51596897, MaxLogs: 4096, MaxPositions: 256, MaxResponseBytes: 4 << 20})
	if err != nil {
		t.Fatal(err)
	}
	before := len(transport.Calls)
	started := time.Now()
	result, err := reader.Analyze(ctx, lpprotection.Request{ChainID: 8453, Protocol: clliquidity.Slipstream, Creation: creation, Principal: lpprotection.Principal{Pool: pool, Snapshot: snapshot}, Receipts: map[common.Hash]*types.Receipt{pr.TxHash: &pr}})
	elapsed := time.Since(started)
	if err != nil {
		t.Fatal(err)
	}
	if result.Observation == nil || !result.Observation.AllPositionsProtected || result.Observation.Token0.LockedPercentage != nil || result.Observation.Token1.LockedPercentage == nil || *result.Observation.Token1.LockedPercentage != "100" || result.Observation.CanWeakenProtection == nil || !*result.Observation.CanWeakenProtection || result.Observation.EarliestUnlockAt == nil || result.Observation.EarliestUnlockAt.Unix() != 4294967295 {
		t.Fatalf("unexpected protection result: %+v", result)
	}
	if len(transport.Calls)-before != result.Metrics.Calls-adapter.cacheHits {
		t.Fatal("outbound count mismatch")
	}
	methods := map[string]int{}
	for _, call := range transport.Calls[before:] {
		methods[call["method"].(string)]++
	}
	out := map[string]any{"scope": "research-only-single-sample-with-verified-adapter", "proofFile": "proof.json", "result": result, "principalSetupRPC": before, "protectionActualRPC": len(transport.Calls) - before, "protectionElapsedSeconds": elapsed.Seconds(), "adapterCacheHits": adapter.cacheHits, "actualMethods": methods}
	b, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory(), "result.json"), append(b, '\n'), 0644); err != nil {
		t.Fatal(err)
	}
	t.Log(string(b))
}
