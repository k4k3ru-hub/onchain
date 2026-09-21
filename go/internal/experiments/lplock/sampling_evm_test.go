package lplock

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/k4k3ru-hub/onchain/go/evm"
	"github.com/k4k3ru-hub/onchain/go/evm/clliquidity"
)

type samplePosition struct {
	ID         string         `json:"token_id"`
	Manager    common.Address `json:"manager"`
	Owner      common.Address `json:"owner"`
	Approved   common.Address `json:"approved"`
	Lower      int32          `json:"lower"`
	Upper      int32          `json:"upper"`
	Liquidity  string         `json:"liquidity"`
	CodeHash   common.Hash    `json:"owner_code_hash"`
	CodeBytes  int            `json:"owner_code_bytes"`
	Withdrawal string         `json:"withdrawal_assessment"`
}

func sampleCoverage(state clliquidity.State, positions []samplePosition) bool {
	if !state.Complete || len(positions) == 0 {
		return false
	}
	gross, net := map[int32]*big.Int{}, map[int32]*big.Int{}
	for _, p := range positions {
		l, ok := new(big.Int).SetString(p.Liquidity, 10)
		if !ok || l.Sign() <= 0 || p.Lower >= p.Upper {
			return false
		}
		for _, tick := range []int32{p.Lower, p.Upper} {
			if gross[tick] == nil {
				gross[tick], net[tick] = new(big.Int), new(big.Int)
			}
			gross[tick].Add(gross[tick], l)
		}
		net[p.Lower].Add(net[p.Lower], l)
		net[p.Upper].Sub(net[p.Upper], l)
	}
	if len(gross) != len(state.Ticks) {
		return false
	}
	for _, tick := range state.Ticks {
		if gross[tick.Index] == nil || gross[tick.Index].Cmp(tick.Gross) != 0 || net[tick.Index].Cmp(tick.Net) != 0 {
			return false
		}
	}
	_, err := clliquidity.Calculate(state)
	return err == nil
}

// TestSampleCoverageRequiresAllPositions rejects unexplained gross liquidity.
//
// Version:
//   - 2026-09-20: Added.
func TestSampleCoverageRequiresAllPositions(t *testing.T) {
	s := clliquidity.State{Complete: true, Spacing: 10, SqrtPriceX96: new(big.Int).Lsh(big.NewInt(1), 96), Tick: 0, ActiveLiquidity: big.NewInt(5), Ticks: []clliquidity.Tick{{Index: -10, Gross: big.NewInt(5), Net: big.NewInt(5)}, {Index: 10, Gross: big.NewInt(5), Net: big.NewInt(-5)}}}
	p := []samplePosition{{Lower: -10, Upper: 10, Liquidity: "2"}, {Lower: -10, Upper: 10, Liquidity: "3"}}
	if !sampleCoverage(s, p) || sampleCoverage(s, p[:1]) {
		t.Fatal("failed to verify sample coverage: result=invalid")
	}
}

// TestLPVenueSamplesLive inspects a frozen cohort or selects recent creation events before examining custody.
//
// Version:
//   - 2026-09-20: Added.
//   - 2026-09-21: Support bounded cohort sizes, cached observations and delegated EOA withdrawal probes.
func TestLPVenueSamplesLive(t *testing.T) {
	venue := os.Getenv("ONCHAIN_SAMPLE_VENUE")
	if venue == "" {
		t.Skip("set ONCHAIN_SAMPLE_VENUE for read-only live sampling")
	}
	dir := os.Getenv("ONCHAIN_SAMPLE_DIR")
	if dir == "" {
		t.Fatal("failed to sample venue: directory=empty")
	}
	count := 10
	if value := os.Getenv("ONCHAIN_SAMPLE_COUNT"); value != "" {
		parsed, err := strconv.Atoi(value)
		if err != nil || parsed < 1 || parsed > 40 {
			t.Fatal("failed to select samples: count=out_of_range")
		}
		count = parsed
	}
	factory := common.HexToAddress("0x33128a8fc17869897dce68ed026d694621f6fdfd")
	manager := common.HexToAddress("0x03a520b32c04bf3beef7beb72e919cf822ed34f1")
	protocol := clliquidity.V3
	signature := "PoolCreated(address,address,uint24,int24,address)"
	if venue == "aerodrome" {
		factory = common.HexToAddress("0xf8f2eB4940CFE7d13603DDDD87f123820Fc061Ef")
		manager = common.HexToAddress("0xe1f8cd9AC4e4A65F54f38a5CdAfCA44f6dD68b53")
		protocol = clliquidity.Slipstream
		signature = "PoolCreated(address,address,int24,address)"
	} else if venue != "uniswap-v3" {
		t.Fatal("failed to sample venue: venue=unsupported")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Minute)
	defer cancel()
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
	p := &measuredRPC{sdk: sdk, raw: raw, methods: map[string]int{}, retryAll: true}
	if value := os.Getenv("ONCHAIN_SAMPLE_BLOCK"); value != "" {
		block, ok := new(big.Int).SetString(value, 10)
		if !ok || block.Sign() <= 0 {
			t.Fatal("failed to pin sample block: block=invalid")
		}
		p.pinBlock = block
	}
	started := time.Now()
	write := func(name string, value any) {
		t.Helper()
		b, err := json.MarshalIndent(value, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, name), append(b, '\n'), 0600); err != nil {
			t.Fatal(err)
		}
	}
	header, err := p.HeaderByNumber(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	p.pinBlock = header.Number
	end := header.Number.Uint64()
	topic := crypto.Keccak256Hash([]byte(signature))
	var events []types.Log
	if manifestPath := os.Getenv("ONCHAIN_SAMPLE_MANIFEST"); manifestPath != "" {
		data, err := os.ReadFile(manifestPath)
		if err != nil {
			t.Fatal(err)
		}
		var manifest struct {
			Venue   string         `json:"venue"`
			Factory common.Address `json:"factory"`
			Events  []types.Log    `json:"events"`
		}
		if err := json.Unmarshal(data, &manifest); err != nil {
			t.Fatal(err)
		}
		if manifest.Venue != venue || manifest.Factory != factory || len(manifest.Events) == 0 || len(manifest.Events) > 40 {
			t.Fatal("failed to reuse sample manifest: manifest=invalid")
		}
		events = manifest.Events
	}
	logs := func(addresses []common.Address, topic common.Hash, from, to uint64) ([]types.Log, error) {
		var result []types.Log
		err := p.invoke(ctx, "eth_getLogs", func() error {
			var err error
			result, err = raw.FilterLogs(ctx, ethereum.FilterQuery{Addresses: addresses, Topics: [][]common.Hash{{topic}}, FromBlock: new(big.Int).SetUint64(from), ToBlock: new(big.Int).SetUint64(to)})
			return err
		})
		return result, err
	}
	for offset := uint64(0); os.Getenv("ONCHAIN_SAMPLE_MANIFEST") == "" && offset < 400000 && len(events) < count; offset += 2000 {
		batch, err := logs([]common.Address{factory}, topic, end-offset-1999, end-offset)
		if err != nil {
			t.Fatal(err)
		}
		events = append(events, batch...)
	}
	sort.Slice(events, func(i, j int) bool {
		if events[i].BlockNumber != events[j].BlockNumber {
			return events[i].BlockNumber > events[j].BlockNumber
		}
		return events[i].Index > events[j].Index
	})
	if len(events) < count && os.Getenv("ONCHAIN_SAMPLE_MANIFEST") == "" {
		write(venue+"-insufficient.json", events)
		t.Fatal("failed to select sample: creation_events=too_short")
	}
	selection := "frozen externally before custody inspection"
	if os.Getenv("ONCHAIN_SAMPLE_MANIFEST") == "" {
		events = events[:count]
		selection = fmt.Sprintf("latest %d distinct creation events before custody inspection", count)
	}
	write(venue+"-manifest.json", map[string]any{"venue": venue, "chain": "base", "network": "mainnet", "factory": factory, "manager": manager, "block_number": end, "block_hash": header.Hash(), "block_timestamp": header.Time, "selection": selection, "events": events})
	pools := make([]common.Address, len(events))
	seenPools := make(map[common.Address]bool, len(events))
	for i, e := range events {
		if e.Removed || e.Address != factory || e.BlockNumber > end || len(e.Topics) != 4 || e.Topics[0] != topic || len(e.Data) < 32 {
			t.Fatal("failed to validate sample input: event=invalid")
		}
		pools[i] = common.BytesToAddress(e.Data[len(e.Data)-20:])
		if pools[i] == (common.Address{}) || seenPools[pools[i]] {
			t.Fatal("failed to validate sample input: pool=invalid")
		}
		seenPools[pools[i]] = true
	}
	if os.Getenv("ONCHAIN_SAMPLE_DISCOVERY_ONLY") == "1" {
		write(venue+"-discovery-metrics.json", map[string]any{"rpc_http_attempts": p.methods, "selection_count": len(events), "block_number": end})
		return
	}
	mintTopic := crypto.Keccak256Hash([]byte("Mint(address,address,int24,int24,uint128,uint256,uint256)"))
	var mints []types.Log
	startBlock := events[len(events)-1].BlockNumber
	if cachePath := os.Getenv("ONCHAIN_SAMPLE_MINT_CACHE"); cachePath != "" {
		data, err := os.ReadFile(cachePath)
		if err != nil {
			t.Fatal(err)
		}
		var cache struct {
			From  uint64      `json:"from_block"`
			To    uint64      `json:"to_block"`
			Hash  common.Hash `json:"block_hash"`
			Mints []types.Log `json:"mints"`
		}
		if err := json.Unmarshal(data, &cache); err != nil {
			t.Fatal(err)
		}
		if cache.From > startBlock || cache.To < startBlock || cache.To > end {
			t.Fatal("failed to reuse mint cache: range=invalid")
		}
		cachedHeader, err := p.HeaderByNumber(ctx, new(big.Int).SetUint64(cache.To))
		if err != nil || cachedHeader.Hash() != cache.Hash {
			t.Fatal("failed to reuse mint cache: block_hash=unverified", err)
		}
		for _, mint := range cache.Mints {
			if seenPools[mint.Address] && mint.BlockNumber >= startBlock && mint.BlockNumber <= cache.To && !mint.Removed {
				mints = append(mints, mint)
			}
		}
		startBlock = cache.To + 1
	}
	for start := startBlock; start <= end; start += 2000 {
		to := start + 1999
		if to > end {
			to = end
		}
		batch, err := logs(pools, mintTopic, start, to)
		if err != nil {
			t.Fatal(err)
		}
		mints = append(mints, batch...)
	}
	write(venue+"-mints.json", mints)
	receipts := map[common.Hash]*types.Receipt{}
	receipt := func(hash common.Hash) (*types.Receipt, error) {
		if value := receipts[hash]; value != nil {
			return value, nil
		}
		var value *types.Receipt
		err := p.invoke(ctx, "eth_getTransactionReceipt", func() error { var err error; value, err = raw.TransactionReceipt(ctx, hash); return err })
		if err == nil {
			receipts[hash] = value
		}
		return value, err
	}
	call := func(to common.Address, sig string, n int, args ...*big.Int) ([]*big.Int, error) {
		data, err := p.CallContract(ctx, ethereum.CallMsg{To: &to, Data: query(sig, args...)}, header.Number)
		if err != nil {
			return nil, err
		}
		return decodeWords(data, n)
	}
	ownerCodes := map[common.Address][]byte{}
	results := make([]map[string]any, 0, len(events))
	for i, event := range events {
		pool := pools[i]
		begin := time.Now()
		before := map[string]int{}
		for k, v := range p.methods {
			before[k] = v
		}
		row := map[string]any{"pool": pool.Hex(), "creation_event": event, "locked_liquidity_percentage": nil, "reasons": []string{}, "block_number": end, "block_hash": header.Hash(), "positions": []samplePosition{}}
		inspect := func() error {
			created, err := receipt(event.TxHash)
			if err != nil {
				return err
			}
			if created.Status != 1 || created.BlockHash != event.BlockHash {
				return fmt.Errorf("failed to verify creation: receipt=mismatch")
			}
			spacing := int32(0)
			pairThird := event.Topics[3].Big()
			if protocol == clliquidity.V3 {
				w, err := decodeWords(event.Data, 2)
				if err != nil {
					return err
				}
				spacing = int32(w[0].Int64())
			} else {
				spacing = signedTick(pairThird)
			}
			row["tick_spacing"] = spacing
			mintTransactions := map[common.Hash]bool{}
			for _, mint := range mints {
				if mint.Address == pool && !mint.Removed && len(mint.Topics) == 4 {
					mintTransactions[mint.TxHash] = true
				}
			}
			row["mint_transaction_count"] = len(mintTransactions)
			if len(mintTransactions) > 64 {
				return fmt.Errorf("failed to enumerate positions: mint_transactions=too_long")
			}
			factoryArgs := []*big.Int{event.Topics[1].Big(), event.Topics[2].Big(), pairThird}
			factorySig := "getPool(address,address,uint24)"
			if protocol == clliquidity.Slipstream {
				factorySig = "getPool(address,address,int24)"
			}
			derived, err := call(factory, factorySig, 1, factoryArgs...)
			if err != nil {
				return err
			}
			if common.BigToAddress(derived[0]) != pool {
				return fmt.Errorf("failed to verify sample: pool_key=mismatch")
			}
			reader, err := clliquidity.NewReader(p, clliquidity.Limits{Timeout: 3 * time.Minute, ChunkSize: 256, MaxReads: 10000, MaxCalls: 80, MaxTicks: 512, MaxResponseBytes: 2000000})
			if err != nil {
				return err
			}
			snapshot, err := reader.Capture(ctx, clliquidity.Pool{Protocol: protocol, Address: pool, Spacing: spacing, Multicall: common.HexToAddress("0xcA11bde05977b3631167028862bE2a173976CA11")})
			if err != nil {
				return err
			}
			row["principal_token0"], row["principal_token1"] = snapshot.Amounts.Token0.String(), snapshot.Amounts.Token1.String()
			ids := map[string]*big.Int{}
			for hash := range mintTransactions {
				r, err := receipt(hash)
				if err != nil {
					return err
				}
				for _, log := range r.Logs {
					if log.Address == manager && len(log.Topics) == 2 && log.Topics[0] == crypto.Keccak256Hash([]byte("IncreaseLiquidity(uint256,uint128,uint256,uint256)")) {
						ids[log.Topics[1].Hex()] = log.Topics[1].Big()
					}
				}
			}
			positions := []samplePosition{}
			for _, id := range ids {
				words, err := call(manager, "positions(uint256)", 12, id)
				if err != nil {
					if _, revertErr := probeRevert(err); revertErr == nil {
						continue
					}
					return err
				}
				if common.BigToAddress(words[2]) != common.BytesToAddress(event.Topics[1][:]) || common.BigToAddress(words[3]) != common.BytesToAddress(event.Topics[2][:]) || words[4].Cmp(pairThird) != 0 || words[7].Sign() == 0 {
					continue
				}
				owner, err := call(manager, "ownerOf(uint256)", 1, id)
				if err != nil {
					return err
				}
				approval, err := call(manager, "getApproved(uint256)", 1, id)
				if err != nil {
					return err
				}
				position := samplePosition{ID: id.String(), Manager: manager, Owner: common.BigToAddress(owner[0]), Approved: common.BigToAddress(approval[0]), Lower: signedTick(words[5]), Upper: signedTick(words[6]), Liquidity: words[7].String(), Withdrawal: "unresolved"}
				code, cached := ownerCodes[position.Owner]
				if !cached {
					if err := p.invoke(ctx, "eth_getCode", func() error { var err error; code, err = raw.CodeAt(ctx, position.Owner, header.Number); return err }); err != nil {
						return err
					}
					ownerCodes[position.Owner] = code
				}
				position.CodeBytes, position.CodeHash = len(code), crypto.Keccak256Hash(code)
				delegated := len(code) == 23 && bytes.Equal(code[:3], []byte{0xef, 0x01, 0x00})
				if (len(code) == 0 || delegated) && position.Owner != (common.Address{}) && position.Owner != common.HexToAddress("0xdead") {
					data := query("decreaseLiquidity((uint256,uint128,uint256,uint256,uint256))", id, words[7], new(big.Int), new(big.Int), new(big.Int).SetUint64(header.Time+3600))
					if returned, err := p.CallContract(ctx, ethereum.CallMsg{From: position.Owner, To: &manager, Data: data}, header.Number); err == nil {
						if _, err := decodeWords(returned, 2); err != nil {
							return fmt.Errorf("failed to verify withdrawal response: %w", err)
						}
						position.Withdrawal = "owner_full_decrease_succeeded"
						if delegated {
							position.Withdrawal = "eip7702_owner_full_decrease_succeeded"
						}
					} else {
						if _, e := probeRevert(err); e != nil {
							return err
						}
						position.Withdrawal = "owner_decrease_reverted"
					}
				}
				positions = append(positions, position)
			}
			sort.Slice(positions, func(i, j int) bool { return positions[i].ID < positions[j].ID })
			row["positions"] = positions
			coverage := sampleCoverage(snapshot.State, positions)
			row["complete_position_coverage"] = coverage
			if !coverage {
				row["reasons"] = []string{"pool_principal_not_fully_explained_by_discovered_positions"}
				return nil
			}
			if snapshot.Amounts.Token0.Sign() == 0 && snapshot.Amounts.Token1.Sign() == 0 {
				row["reasons"] = []string{"pool_principal_denominator_zero"}
				return nil
			}
			allOpen := true
			for _, position := range positions {
				if position.Withdrawal != "owner_full_decrease_succeeded" && position.Withdrawal != "eip7702_owner_full_decrease_succeeded" {
					allOpen = false
				}
			}
			if allOpen {
				row["locked_liquidity_percentage"] = "0"
				row["reasons"] = []string{}
			} else {
				row["reasons"] = []string{"custodian_or_withdrawal_conditions_unresolved"}
			}
			return nil
		}
		if err := inspect(); err != nil {
			row["error"] = err.Error()
			row["reasons"] = []string{"inspection_incomplete"}
		}
		metrics := map[string]int{}
		for k, v := range p.methods {
			if v-before[k] > 0 {
				metrics[k] = v - before[k]
			}
		}
		row["rpc_http_attempts"] = metrics
		row["wall_seconds"] = time.Since(begin).Seconds()
		results = append(results, row)
		write(venue+"-samples.json", map[string]any{"venue": venue, "samples": results, "rpc_http_attempts": p.methods, "rate_limit_retries": p.retries, "wall_seconds": time.Since(started).Seconds(), "selection_count": len(events), "block_number": end, "block_hash": header.Hash()})
		t.Logf("venue=%s sample=%d pool=%s percentage=%v reasons=%v", venue, i+1, pool.Hex(), row["locked_liquidity_percentage"], row["reasons"])
	}
	after, err := p.HeaderByNumber(ctx, header.Number)
	if err != nil {
		t.Fatal(err)
	}
	if after.Hash() != header.Hash() {
		t.Fatal("failed to verify samples: block_hash=mismatch")
	}
	write(venue+"-samples.json", map[string]any{"venue": venue, "samples": results, "rpc_http_attempts": p.methods, "rate_limit_retries": p.retries, "wall_seconds": time.Since(started).Seconds(), "selection_count": len(events), "block_number": end, "block_hash": header.Hash(), "final_block_hash_verified": true})
}
