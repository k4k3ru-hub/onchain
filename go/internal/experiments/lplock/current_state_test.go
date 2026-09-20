package lplock

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/k4k3ru-hub/onchain/go/evm"
)

func probeRevert(err error) (string, error) {
	if err == nil {
		return "", fmt.Errorf("failed to verify blocked exit: call=succeeded")
	}
	var dataErr rpc.DataError
	if !errors.As(err, &dataErr) {
		return "", fmt.Errorf("failed to verify blocked exit: %w", err)
	}
	data, ok := dataErr.ErrorData().(string)
	if !ok && dataErr.ErrorData() != nil {
		return "", fmt.Errorf("failed to verify blocked exit: %w: revert_data=invalid", err)
	}
	if data == "0x" || dataErr.ErrorData() == nil {
		var rpcErr rpc.Error
		if !errors.As(err, &rpcErr) || rpcErr.ErrorCode() != 3 {
			return "", fmt.Errorf("failed to verify blocked exit: %w", err)
		}
		return "empty_revert", nil
	}
	return decodeRevert(err)
}

type emptyRPCError struct {
	code int
	data any
}

// Error describes an empty RPC revert fixture.
//
// Version:
//   - 2026-09-20: Added.
func (e emptyRPCError) Error() string { return "execution reverted" }

// ErrorCode returns the RPC status of the fixture.
//
// Version:
//   - 2026-09-20: Added.
func (e emptyRPCError) ErrorCode() int { return e.code }

// ErrorData returns the optional revert bytes of the fixture.
//
// Version:
//   - 2026-09-20: Added.
func (e emptyRPCError) ErrorData() any { return e.data }

// TestEmptyRevertRequiresExecutionCode rejects transport errors and malformed data.
//
// Version:
//   - 2026-09-20: Added.
func TestEmptyRevertRequiresExecutionCode(t *testing.T) {
	for _, data := range []any{nil, "0x"} {
		reason, err := probeRevert(fmt.Errorf("failed to call contract: %w", emptyRPCError{code: 3, data: data}))
		if err != nil || reason != "empty_revert" {
			t.Fatalf("failed to decode empty revert: err=%v", err)
		}
	}
	for _, input := range []error{nil, emptyRPCError{code: -32000}, emptyRPCError{code: 429}, emptyRPCError{code: 3, data: 7}, emptyRPCError{code: 3, data: "0x00"}} {
		if _, err := probeRevert(input); err == nil {
			t.Fatal("failed to reject invalid revert: error=empty")
		}
	}
	transport := errors.New("connection failed")
	if _, err := probeRevert(transport); !errors.Is(err, transport) {
		t.Fatal("failed to preserve transport error: error_chain=invalid")
	}
}

// TestIndependentCurrentStateLive checks source-discovered exits at the discovery block.
//
// Version:
//   - 2026-09-20: Added.
func TestIndependentCurrentStateLive(t *testing.T) {
	if os.Getenv("ONCHAIN_INDEPENDENT_CURRENT") != "1" {
		t.Skip("set ONCHAIN_INDEPENDENT_CURRENT=1 for read-only exit simulations")
	}
	directory := os.Getenv("ONCHAIN_ANALYSIS_DIR")
	if directory == "" {
		t.Fatal("failed to read current analysis: directory=empty")
	}
	read := func(name string, result any) {
		t.Helper()
		data, err := os.ReadFile(filepath.Join(directory, name))
		if err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(data, result); err != nil {
			t.Fatal(err)
		}
	}
	var discovery struct {
		Pool      common.Address `json:"pool"`
		Manager   common.Address `json:"manager"`
		Custodian common.Address `json:"custodian"`
		TokenID   string         `json:"token_id"`
		Liquidity string         `json:"position_liquidity"`
		Block     uint64         `json:"block_number"`
		Hash      common.Hash    `json:"block_hash"`
		Timestamp uint64         `json:"block_timestamp"`
		Runtime   common.Hash    `json:"runtime_hash_observed"`
	}
	var observed struct {
		Key    string           `json:"record_key_discovered"`
		Owners []common.Address `json:"record_owner_addresses"`
	}
	var source struct {
		ABI json.RawMessage `json:"abi"`
	}
	var plan struct {
		Probes []struct {
			Selector  string   `json:"selector"`
			Operation string   `json:"operation"`
			Reasons   []string `json:"expected_revert_reasons"`
		} `json:"probe_entries"`
		Unresolved []string `json:"unresolved"`
	}
	read("discovery.json", &discovery)
	read("independent-result.json", &observed)
	read("source.json", &source)
	read("current-plan.json", &plan)
	if len(observed.Owners) != 1 || len(plan.Unresolved) != 0 || len(plan.Probes) == 0 {
		t.Fatal("failed to verify current analysis: evidence=incomplete")
	}
	integer := func(s string) *big.Int {
		t.Helper()
		n, ok := new(big.Int).SetString(s, 10)
		if !ok || n.Sign() < 0 {
			t.Fatal("failed to parse observation: integer=invalid")
		}
		return n
	}
	token, liquidity, key := integer(discovery.TokenID), integer(discovery.Liquidity), integer(observed.Key)
	contractABI, err := abi.JSON(strings.NewReader(string(source.ABI)))
	if err != nil {
		t.Fatal(err)
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
	defer func() { t.Logf("rpc_http_attempts=%v rate_limit_retries=%d", p.methods, p.retries) }()
	block := new(big.Int).SetUint64(discovery.Block)
	header, err := p.HeaderByNumber(ctx, block)
	if err != nil {
		t.Fatal(err)
	}
	if header.Hash() != discovery.Hash {
		t.Fatal("failed to verify observation: block_hash=mismatch")
	}
	var runtime []byte
	if err := p.invoke(ctx, "eth_getCode", func() error {
		var err error
		runtime, err = raw.CodeAt(ctx, discovery.Custodian, block)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if crypto.Keccak256Hash(runtime) != discovery.Runtime {
		t.Fatal("failed to verify source: runtime=mismatch")
	}
	owner := observed.Owners[0]
	checks := make([]map[string]any, 0, len(plan.Probes))
	revert := func(from, target common.Address, data []byte) string {
		t.Helper()
		_, err := p.CallContract(ctx, ethereum.CallMsg{From: from, To: &target, Data: data}, block)
		reason, err := probeRevert(err)
		if err != nil {
			t.Fatal(err)
		}
		return reason
	}
	for _, probe := range plan.Probes {
		selector, err := hexutil.Decode("0x" + probe.Selector)
		if err != nil {
			t.Fatal(err)
		}
		method, err := contractABI.MethodById(selector)
		if err != nil {
			t.Fatal(err)
		}
		data := append([]byte(nil), selector...)
		for _, input := range method.Inputs {
			switch input.Type.String() {
			case "uint256":
				data = append(data, word(key)...)
			case "address":
				data = append(data, word(owner.Big())...)
			case "(uint256,uint128,uint256,uint256,uint256)":
				if probe.Operation != "lp_decrease" {
					t.Fatal("failed to encode exit probe: tuple=unsupported")
				}
				for _, v := range []*big.Int{token, liquidity, new(big.Int), new(big.Int), new(big.Int).SetUint64(discovery.Timestamp + 3600)} {
					data = append(data, word(v)...)
				}
			default:
				t.Fatal("failed to encode exit probe: input_type=unsupported")
			}
		}
		if _, err := method.Inputs.Unpack(data[4:]); err != nil {
			t.Fatal(err)
		}
		reason := revert(owner, discovery.Custodian, data)
		matched := false
		for _, expected := range probe.Reasons {
			if reason == expected {
				matched = true
			}
		}
		if !matched {
			t.Fatalf("failed to match source guard: selector=%q reason=%q", probe.Selector, reason)
		}
		checks = append(checks, map[string]any{"selector": probe.Selector, "operation": probe.Operation, "name_observed": method.RawName, "reverted": true, "reason": reason})
	}
	for _, signature := range []string{"ownerOf(uint256)", "getApproved(uint256)"} {
		result, err := p.CallContract(ctx, ethereum.CallMsg{To: &discovery.Manager, Data: query(signature, token)}, block)
		if err != nil {
			t.Fatal(err)
		}
		words, err := decodeWords(result, 1)
		if err != nil {
			t.Fatal(err)
		}
		expected := common.Address{}
		if signature == "ownerOf(uint256)" {
			expected = discovery.Custodian
		}
		if common.BigToAddress(words[0]) != expected {
			t.Fatal("failed to verify custody: owner_or_approval=mismatch")
		}
	}
	incomingReason := revert(discovery.Custodian, discovery.Manager, query("safeTransferFrom(address,address,uint256)", owner.Big(), discovery.Custodian.Big(), token))
	directReason := revert(owner, discovery.Manager, query("decreaseLiquidity((uint256,uint128,uint256,uint256,uint256))", token, liquidity, new(big.Int), new(big.Int), new(big.Int).SetUint64(discovery.Timestamp+3600)))
	// The refund helper discovered in source encodes ERC20.transfer, whose
	// selector differs from ERC721.transferFrom. Test the actual NFT manager.
	erc20Reason := revert(discovery.Custodian, discovery.Manager, query("transfer(address,uint256)", owner.Big(), token))
	after, err := p.HeaderByNumber(ctx, block)
	if err != nil {
		t.Fatal(err)
	}
	if after.Hash() != discovery.Hash {
		t.Fatal("failed to verify exit block: hash=mismatch")
	}
	result := map[string]any{
		"pool": discovery.Pool.Hex(), "block_number": discovery.Block, "block_hash": discovery.Hash.Hex(), "custodian": discovery.Custodian.Hex(), "token_id": discovery.TokenID,
		"exit_checks": checks, "runtime_unchanged": true, "custody_unchanged": true,
		"incoming_existing_nft_reverted": true, "incoming_revert_reason": incomingReason,
		"direct_decrease_reverted": true, "direct_decrease_reason": directReason,
		"erc20_selector_reverted": true, "erc20_selector_reason": erc20Reason,
		"rpc_http_attempts": p.methods, "rate_limit_retries": p.retries, "wall_seconds": time.Since(started).Seconds(),
	}
	encoded, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "current-probes.json"), append(encoded, '\n'), 0600); err != nil {
		t.Fatal(err)
	}
	t.Log(string(encoded))
}
