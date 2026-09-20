package lplock

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/k4k3ru-hub/onchain/go/evm"
	"github.com/k4k3ru-hub/onchain/go/evm/clliquidity"
	"github.com/k4k3ru-hub/onchain/go/venues/uniswap/v3/protocol"
	"io"
	"math/big"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestIndependentDiscoveryLive discovers custody and fetches its verified source without a locker registry.
//
// Version:
//   - 2026-09-20: Added.
//   - 2026-09-20: Include position liquidity for independent exit simulations.
func TestIndependentDiscoveryLive(t *testing.T) {
	if os.Getenv("ONCHAIN_INDEPENDENT_DISCOVERY") != "1" {
		t.Skip("set ONCHAIN_INDEPENDENT_DISCOVERY=1 for public RPC reads")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	started := time.Now()
	sdk, err := evm.NewHTTPClient(ctx, evm.HTTPConfig{URL: baseRPC})
	if err != nil {
		t.Fatal(err)
	}
	defer sdk.Close()
	raw, err := ethclient.DialContext(ctx, baseRPC)
	if err != nil {
		t.Fatal(err)
	}
	defer raw.Close()
	p := &measuredRPC{sdk: sdk, raw: raw, methods: make(map[string]int)}
	factory := common.HexToAddress("0x33128a8fc17869897dce68ed026d694621f6fdfd")
	manager := common.HexToAddress("0x03a520b32c04bf3beef7beb72e919cf822ed34f1")
	input, err := os.ReadFile("testdata/pool_created.json")
	if err != nil {
		t.Fatal(err)
	}
	var event types.Log
	if err := json.Unmarshal(input, &event); err != nil {
		t.Fatal(err)
	}
	if event.Removed || event.Address != factory || len(event.Topics) != 4 || event.Topics[0] != crypto.Keccak256Hash([]byte("PoolCreated(address,address,uint24,int24,address)")) {
		t.Fatal("failed to validate creation input: event=invalid")
	}
	creation, err := decodeWords(event.Data, 2)
	if err != nil {
		t.Fatal(err)
	}
	pool := common.BigToAddress(creation[1])
	spacing := int32(creation[0].Int64())
	key := protocol.PoolKey{Token0: protocol.NewCurrency(common.BytesToAddress(event.Topics[1][:])), Token1: protocol.NewCurrency(common.BytesToAddress(event.Topics[2][:])), Fee: uint32(event.Topics[3].Big().Uint64())}
	derived, err := key.Address(factory, common.HexToHash("0xe34f199b19b2b4f47f68442619d555527d244f78a3297ea89325f843f87b8b54"))
	if err != nil {
		t.Fatal(err)
	}
	if derived != pool {
		t.Fatal("failed to validate creation input: pool=mismatch")
	}
	var receipt *types.Receipt
	err = p.invoke(ctx, "eth_getTransactionReceipt", func() error { receipt, err = sdk.TransactionReceipt(ctx, event.TxHash); return err })
	if err != nil {
		t.Fatal(err)
	}
	if receipt.Status != 1 || receipt.BlockHash != event.BlockHash || receipt.BlockNumber.Uint64() != event.BlockNumber {
		t.Fatal("failed to validate creation receipt: receipt=mismatch")
	}
	creationHeader, err := p.HeaderByNumber(ctx, receipt.BlockNumber)
	if err != nil {
		t.Fatal(err)
	}
	if creationHeader.Hash() != event.BlockHash {
		t.Fatal("failed to validate creation header: block=mismatch")
	}
	found, managerMint := false, false
	ids := make(map[string]*big.Int)
	for _, log := range receipt.Logs {
		if log.Address == factory && log.Index == event.Index && bytes.Equal(log.Data, event.Data) && len(log.Topics) == 4 {
			found = true
			for i := range log.Topics {
				if log.Topics[i] != event.Topics[i] {
					found = false
				}
			}
		}
		if log.Address == pool && len(log.Topics) == 4 && log.Topics[0] == crypto.Keccak256Hash([]byte("Mint(address,address,int24,int24,uint128,uint256,uint256)")) && common.BytesToAddress(log.Topics[1][:]) == manager {
			managerMint = true
		}
		if log.Address == manager && len(log.Topics) == 2 && log.Topics[0] == crypto.Keccak256Hash([]byte("IncreaseLiquidity(uint256,uint128,uint256,uint256)")) {
			ids[log.Topics[1].Hex()] = log.Topics[1].Big()
		}
	}
	if !found || !managerMint || len(ids) == 0 || len(ids) > 16 {
		t.Fatal("failed to discover pool positions: creation_mints=unsupported")
	}
	reader, err := clliquidity.NewReader(p, clliquidity.Limits{Timeout: time.Minute, ChunkSize: 64, MaxReads: 256, MaxCalls: 24, MaxTicks: 64, MaxResponseBytes: 1_000_000})
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := reader.Capture(ctx, clliquidity.Pool{Protocol: clliquidity.V3, Address: pool, Spacing: spacing, Multicall: common.HexToAddress("0xcA11bde05977b3631167028862bE2a173976CA11")})
	if err != nil {
		t.Fatal(err)
	}
	block := new(big.Int).SetUint64(snapshot.BlockNumber)
	call := func(target common.Address, signature string, count int, args ...*big.Int) []*big.Int {
		t.Helper()
		data, err := p.CallContract(ctx, ethereum.CallMsg{To: &target, Data: query(signature, args...)}, block)
		if err != nil {
			t.Fatal(err)
		}
		out, err := decodeWords(data, count)
		if err != nil {
			t.Fatal(err)
		}
		return out
	}
	var tokenID *big.Int
	var position []*big.Int
	for _, id := range ids {
		w := call(manager, "positions(uint256)", 12, id)
		if common.BigToAddress(w[2]) != key.Token0.Address() || common.BigToAddress(w[3]) != key.Token1.Address() || w[4].Uint64() != uint64(key.Fee) || w[7].Sign() == 0 {
			continue
		}
		if tokenID != nil {
			t.Fatal("failed to verify sample coverage: multiple_positions=unsupported")
		}
		tokenID, position = id, w
	}
	if tokenID == nil {
		t.Fatal("failed to discover pool positions: nonzero_position=empty")
	}
	lower, upper := signedTick(position[5]), signedTick(position[6])
	packed := append(append(append([]byte(nil), manager[:]...), word(big.NewInt(int64(lower)))[29:]...), word(big.NewInt(int64(upper)))[29:]...)
	core := call(pool, "positions(bytes32)", 5, crypto.Keccak256Hash(packed).Big())
	if !fullSinglePosition(snapshot.State, lower, upper, position[7], core[0]) {
		t.Fatal("failed to verify sample coverage: liquidity=incomplete")
	}
	owner := common.BigToAddress(call(manager, "ownerOf(uint256)", 1, tokenID)[0])
	approval := common.BigToAddress(call(manager, "getApproved(uint256)", 1, tokenID)[0])
	if approval != (common.Address{}) {
		t.Fatal("failed to verify custody: approval=unresolved")
	}
	var code []byte
	err = p.invoke(ctx, "eth_getCode", func() error { code, err = raw.CodeAt(ctx, owner, block); return err })
	if err != nil {
		t.Fatal(err)
	}

	directory := os.Getenv("ONCHAIN_ANALYSIS_DIR")
	if directory == "" {
		t.Fatal("failed to store discovery: output_directory=empty")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://sourcify.dev/server/v2/contract/8453/"+owner.Hex()+"?fields=all", nil)
	if err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Timeout: 30 * time.Second}
	response, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	sourceBytes, readErr := io.ReadAll(io.LimitReader(response.Body, 4_000_001))
	closeErr := response.Body.Close()
	if readErr != nil {
		t.Fatal(readErr)
	}
	if closeErr != nil {
		t.Fatal(closeErr)
	}
	if response.StatusCode != 200 || len(sourceBytes) > 4_000_000 {
		t.Fatal("failed to fetch source: response=invalid")
	}
	var source struct {
		RuntimeMatch    string `json:"runtimeMatch"`
		RuntimeBytecode struct {
			OnchainBytecode string `json:"onchainBytecode"`
		} `json:"runtimeBytecode"`
		ProxyResolution struct {
			IsProxy bool `json:"isProxy"`
		} `json:"proxyResolution"`
	}
	if err := json.Unmarshal(sourceBytes, &source); err != nil {
		t.Fatal(err)
	}
	deployed, err := hexutil.Decode(source.RuntimeBytecode.OnchainBytecode)
	if err != nil {
		t.Fatal(err)
	}
	if source.RuntimeMatch != "exact_match" || !bytes.Equal(deployed, code) || source.ProxyResolution.IsProxy {
		t.Fatal("failed to verify discovered source: runtime_or_proxy=unsupported")
	}
	after, err := p.HeaderByNumber(ctx, block)
	if err != nil {
		t.Fatal(err)
	}
	if after.Hash() != snapshot.BlockHash {
		t.Fatal("failed to verify discovery block: block=mismatch")
	}
	result := map[string]any{
		"chain_id": 8453, "pool": pool.Hex(), "manager": manager.Hex(), "token_id": tokenID.String(), "custodian": owner.Hex(),
		"block_number": snapshot.BlockNumber, "block_hash": snapshot.BlockHash.Hex(), "block_timestamp": snapshot.BlockTime.Unix(),
		"creation_transaction": event.TxHash.Hex(), "custody_receipt_logs": receipt.Logs,
		"complete_single_position_coverage": true, "principal_token0": snapshot.Amounts.Token0.String(), "principal_token1": snapshot.Amounts.Token1.String(),
		"position_liquidity": position[7].String(), "token0": key.Token0.Address().Hex(), "token1": key.Token1.Address().Hex(),
		"runtime_hash_observed": crypto.Keccak256Hash(code).Hex(), "source_match": source.RuntimeMatch,
		"rpc_http_attempts": p.methods, "rate_limit_retries": p.retries, "source_http_requests": 1, "wall_seconds": time.Since(started).Seconds(),
	}
	encoded, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(directory, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "discovery.json"), append(encoded, '\n'), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "source.json"), sourceBytes, 0600); err != nil {
		t.Fatal(err)
	}
	t.Logf("pool=%s token_id=%s custodian=%s block=%d source_match=%s methods=%v retries=%d", pool.Hex(), tokenID, owner.Hex(), snapshot.BlockNumber, source.RuntimeMatch, p.methods, p.retries)
}
