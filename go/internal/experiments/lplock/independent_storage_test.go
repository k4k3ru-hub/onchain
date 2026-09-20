package lplock

import (
	"context"
	"encoding/json"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/k4k3ru-hub/onchain/go/evm"
)

type storageField struct {
	Mapping    string `json:"mapping_slot"`
	Slot       string `json:"slot"`
	Offset     int    `json:"offset"`
	Bytes      int    `json:"bytes"`
	Comparison string `json:"comparison"`
}

func (f storageField) location(key *big.Int) (common.Hash, error) {
	slot, ok := new(big.Int).SetString(f.Slot, 10)
	if !ok || slot.Sign() < 0 || f.Offset < 0 || f.Bytes < 1 || f.Offset+f.Bytes > 32 {
		return common.Hash{}, fmt.Errorf("failed to locate record field: layout=invalid")
	}
	if f.Mapping != "" {
		base, ok := new(big.Int).SetString(f.Mapping, 10)
		if !ok || key == nil || key.Sign() < 0 {
			return common.Hash{}, fmt.Errorf("failed to locate record field: mapping=invalid")
		}
		slot.Add(slot, crypto.Keccak256Hash(word(key), word(base)).Big())
	}
	return common.BigToHash(slot), nil
}

func (f storageField) extract(raw []byte) *big.Int {
	v := new(big.Int).Rsh(new(big.Int).SetBytes(raw), uint(f.Offset*8))
	return v.And(v, new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), uint(f.Bytes*8)), big.NewInt(1)))
}

func integerLeaves(value reflect.Value, found map[string]*big.Int) {
	if !value.IsValid() {
		return
	}
	if value.CanInterface() {
		if v, ok := value.Interface().(*big.Int); ok {
			if v != nil && v.Sign() >= 0 {
				found[v.String()] = new(big.Int).Set(v)
			}
			return
		}
	}
	switch value.Kind() {
	case reflect.Interface, reflect.Pointer:
		if !value.IsNil() {
			integerLeaves(value.Elem(), found)
		}
	case reflect.Struct:
		for i := 0; i < value.NumField(); i++ {
			integerLeaves(value.Field(i), found)
		}
	case reflect.Slice, reflect.Array:
		// Fixed byte arrays represent addresses/hashes, not mapping key hints.
		if value.Type().Elem().Kind() == reflect.Uint8 {
			return
		}
		for i := 0; i < value.Len(); i++ {
			integerLeaves(value.Index(i), found)
		}
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		v := new(big.Int).SetUint64(value.Uint())
		found[v.String()] = v
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		if value.Int() >= 0 {
			v := big.NewInt(value.Int())
			found[v.String()] = v
		}
	}
}

// TestIndependentStorageLive reads source-derived slots without any custodian-specific call.
//
// Version:
//   - 2026-09-20: Added.
func TestIndependentStorageLive(t *testing.T) {
	if os.Getenv("ONCHAIN_INDEPENDENT_STORAGE") != "1" {
		t.Skip("set ONCHAIN_INDEPENDENT_STORAGE=1 for storage RPC reads")
	}
	directory := os.Getenv("ONCHAIN_ANALYSIS_DIR")
	if directory == "" {
		t.Fatal("failed to read analysis plan: directory=empty")
	}
	readJSON := func(name string, out any) {
		t.Helper()
		raw, err := os.ReadFile(filepath.Join(directory, name))
		if err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(raw, out); err != nil {
			t.Fatal(err)
		}
	}
	var discovery struct {
		Pool      common.Address `json:"pool"`
		Manager   common.Address `json:"manager"`
		TokenID   string         `json:"token_id"`
		Custodian common.Address `json:"custodian"`
		Block     uint64         `json:"block_number"`
		Hash      common.Hash    `json:"block_hash"`
		Timestamp uint64         `json:"block_timestamp"`
		Logs      []*types.Log   `json:"custody_receipt_logs"`
		Complete  bool           `json:"complete_single_position_coverage"`
	}
	var source struct {
		ABI json.RawMessage `json:"abi"`
	}
	var plan struct {
		Records []struct {
			Manager storageField   `json:"manager_field"`
			NFT     storageField   `json:"nft_field"`
			Times   []storageField `json:"time_fields"`
			Owners  []storageField `json:"owner_fields"`
		} `json:"records"`
		Gates []struct {
			Slot    string `json:"slot"`
			Offset  int    `json:"offset"`
			Bytes   int    `json:"bytes"`
			Writers []struct {
				Selector string `json:"selector"`
				Roles    []struct {
					Slot   string `json:"slot"`
					Offset int    `json:"offset"`
				} `json:"role_storage"`
			} `json:"writers"`
		} `json:"mutable_gates"`
		Unresolved []string `json:"unresolved"`
	}
	readJSON("discovery.json", &discovery)
	readJSON("source.json", &source)
	readJSON("analysis-plan.json", &plan)
	if len(plan.Records) != 1 || !discovery.Complete {
		t.Fatal("failed to read independent record: coverage=unsupported")
	}
	tokenID, ok := new(big.Int).SetString(discovery.TokenID, 10)
	if !ok {
		t.Fatal("failed to read discovered token: id=invalid")
	}
	contractABI, err := abi.JSON(strings.NewReader(string(source.ABI)))
	if err != nil {
		t.Fatal(err)
	}
	candidates := make(map[string]*big.Int)
	for _, log := range discovery.Logs {
		if log == nil || log.Removed || log.Address != discovery.Custodian || len(log.Topics) == 0 {
			continue
		}
		event, err := contractABI.EventByID(log.Topics[0])
		if err != nil {
			t.Fatal(err)
		}
		values, err := event.Inputs.NonIndexed().Unpack(log.Data)
		if err != nil {
			t.Fatal(err)
		}
		for _, value := range values {
			integerLeaves(reflect.ValueOf(value), candidates)
		}
		indexed := make(abi.Arguments, 0)
		for _, arg := range event.Inputs {
			if arg.Indexed {
				indexed = append(indexed, arg)
			}
		}
		if len(indexed) != len(log.Topics)-1 {
			t.Fatal("failed to decode discovered event: topics=invalid")
		}
		for i, arg := range indexed {
			if arg.Type.T == abi.UintTy {
				key := log.Topics[i+1].Big()
				candidates[key.String()] = key
			}
		}
	}
	if len(candidates) == 0 || len(candidates) > 64 {
		t.Fatal("failed to discover mapping keys: candidates=out_of_range")
	}
	keys := make([]*big.Int, 0, len(candidates))
	for _, k := range candidates {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i].Cmp(keys[j]) < 0 })
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
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
	block := new(big.Int).SetUint64(discovery.Block)
	before, err := p.HeaderByNumber(ctx, block)
	if err != nil {
		t.Fatal(err)
	}
	if before.Hash() != discovery.Hash {
		t.Fatal("failed to pin storage reads: block=mismatch")
	}
	rpcEntries := 0
	readFields := func(fields []storageField, recordKeys []*big.Int) []*big.Int {
		t.Helper()
		if len(fields) == 0 || len(fields) != len(recordKeys) || len(fields) > 64 {
			t.Fatal("failed to batch storage reads: fields=invalid")
		}
		values := make([]hexutil.Bytes, len(fields))
		batch := make([]rpc.BatchElem, len(fields))
		for i, f := range fields {
			location, err := f.location(recordKeys[i])
			if err != nil {
				t.Fatal(err)
			}
			batch[i] = rpc.BatchElem{Method: "eth_getStorageAt", Args: []any{discovery.Custodian, location, hexutil.EncodeUint64(discovery.Block)}, Result: &values[i]}
		}
		err := p.invoke(ctx, "storage_batch_http", func() error { rpcEntries += len(batch); return raw.Client().BatchCallContext(ctx, batch) })
		if err != nil {
			t.Fatal(err)
		}
		out := make([]*big.Int, len(fields))
		for i, f := range fields {
			if batch[i].Error != nil {
				t.Fatal(batch[i].Error)
			}
			if len(values[i]) != 32 {
				t.Fatal("failed to decode storage: value=invalid")
			}
			out[i] = f.extract(values[i])
		}
		return out
	}
	shape := plan.Records[0]
	fields := make([]storageField, len(keys))
	for i := range fields {
		fields[i] = shape.NFT
	}
	ids := readFields(fields, keys)
	var key *big.Int
	for i, id := range ids {
		if id.Cmp(tokenID) == 0 {
			if key != nil {
				t.Fatal("failed to match storage record: matching_keys=ambiguous")
			}
			key = keys[i]
		}
	}
	if key == nil {
		t.Fatal("failed to match storage record: matching_key=empty")
	}
	fields = []storageField{shape.Manager}
	fields = append(fields, shape.Times...)
	fields = append(fields, shape.Owners...)
	recordKeys := make([]*big.Int, len(fields))
	for i := range recordKeys {
		recordKeys[i] = key
	}
	values := readFields(fields, recordKeys)
	if common.BigToAddress(values[0]) != discovery.Manager {
		t.Fatal("failed to match storage record: position_manager=mismatch")
	}
	times := make([]map[string]any, 0, len(shape.Times))
	for i, f := range shape.Times {
		x := values[i+1]
		now := new(big.Int).SetUint64(discovery.Timestamp)
		passes := x.Cmp(now) < 0
		if f.Comparison == "<=" {
			passes = x.Cmp(now) <= 0
		}
		times = append(times, map[string]any{"value": x.String(), "comparison": f.Comparison, "condition_passes": passes})
	}
	owners := make([]string, 0, len(shape.Owners))
	for i := range shape.Owners {
		owners = append(owners, common.BigToAddress(values[1+len(shape.Times)+i]).Hex())
	}
	gates := make([]map[string]any, 0, len(plan.Gates))
	for _, gate := range plan.Gates {
		fields = []storageField{{Slot: gate.Slot, Offset: gate.Offset, Bytes: gate.Bytes}}
		for _, writer := range gate.Writers {
			for _, role := range writer.Roles {
				fields = append(fields, storageField{Slot: role.Slot, Offset: role.Offset, Bytes: 20})
			}
		}
		values = readFields(fields, make([]*big.Int, len(fields)))
		roles := make([]string, 0)
		for _, v := range values[1:] {
			roles = append(roles, common.BigToAddress(v).Hex())
		}
		gates = append(gates, map[string]any{"address": common.BigToAddress(values[0]).Hex(), "writer_count": len(gate.Writers), "writer_role_addresses": roles})
	}
	after, err := p.HeaderByNumber(ctx, block)
	if err != nil {
		t.Fatal(err)
	}
	if after.Hash() != discovery.Hash {
		t.Fatal("failed to verify storage block: hash=mismatch")
	}
	result := map[string]any{
		"pool": discovery.Pool.Hex(), "block_number": discovery.Block, "block_hash": discovery.Hash.Hex(), "custodian": discovery.Custodian.Hex(),
		"token_id": tokenID.String(), "record_key_discovered": key.String(), "candidate_key_count": len(keys), "time_fields": times, "record_owner_addresses": owners, "mutable_gates": gates,
		"locked_liquidity_percentage": nil, "unresolved": plan.Unresolved,
		"storage_rpc_entries_including_retries": rpcEntries, "rpc_http_attempts": p.methods, "rate_limit_retries": p.retries, "wall_seconds": time.Since(started).Seconds(),
	}
	encoded, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "independent-result.json"), append(encoded, '\n'), 0600); err != nil {
		t.Fatal(err)
	}
	t.Log(string(encoded))
}
