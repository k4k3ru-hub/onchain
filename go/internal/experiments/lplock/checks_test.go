package lplock

import (
	"errors"
	"fmt"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/k4k3ru-hub/onchain/go/evm/clliquidity"
)

// TestCoverageRejectsUnexplainedPrincipal rejects extra liquidity with cancelling net changes.
//
// Version:
//   - 2026-09-20: Added.
func TestCoverageRejectsUnexplainedPrincipal(t *testing.T) {
	s := clliquidity.State{Complete: true, Spacing: 10, SqrtPriceX96: new(big.Int).Lsh(big.NewInt(1), 96), Tick: 0, ActiveLiquidity: big.NewInt(5), Ticks: []clliquidity.Tick{
		{Index: -10, Gross: big.NewInt(5), Net: big.NewInt(5)},
		{Index: 10, Gross: big.NewInt(5), Net: big.NewInt(-5)},
	}}
	if !fullSinglePosition(s, -10, 10, big.NewInt(5), big.NewInt(5)) {
		t.Fatal("failed to verify full coverage: result=invalid")
	}
	if fullSinglePosition(s, -10, 10, big.NewInt(5), big.NewInt(6)) {
		t.Fatal("failed to reject extra core liquidity: result=invalid")
	}
	s.Ticks = append(s.Ticks[:1], clliquidity.Tick{Index: 0, Gross: big.NewInt(2), Net: new(big.Int)}, s.Ticks[1])
	if fullSinglePosition(s, -10, 10, big.NewInt(5), big.NewInt(5)) {
		t.Fatal("failed to reject unexplained tick: result=invalid")
	}
	s.Complete = false
	if fullSinglePosition(s, -10, 10, big.NewInt(5), big.NewInt(5)) {
		t.Fatal("failed to reject incomplete snapshot: result=invalid")
	}
}

// TestDiscoveryRequiresMatchingPoolAndNFT binds lock records to discovered ownership.
//
// Version:
//   - 2026-09-20: Added.
func TestDiscoveryRequiresMatchingPoolAndNFT(t *testing.T) {
	owner, manager, pool := common.HexToAddress("0x1"), common.HexToAddress("0x2"), common.HexToAddress("0x3")
	words := make([]*big.Int, 22)
	for i := range words {
		words[i] = new(big.Int)
	}
	words[0], words[1], words[2], words[9] = big.NewInt(987), manager.Big(), big.NewInt(88), pool.Big()
	var data []byte
	for _, w := range words {
		data = append(data, word(w)...)
	}
	log := &types.Log{Address: owner, Topics: []common.Hash{crypto.Keccak256Hash([]byte(lockSignature))}, Data: data}
	id, err := discoverLock([]*types.Log{log}, owner, manager, pool, big.NewInt(88))
	if err != nil || id.Cmp(big.NewInt(987)) != 0 {
		t.Fatalf("failed to discover fixture lock: err=%v", err)
	}
	if _, err := discoverLock([]*types.Log{log}, owner, manager, pool, big.NewInt(89)); err == nil {
		t.Fatal("failed to reject different nft: error=empty")
	}
	if _, err := discoverLock([]*types.Log{log}, owner, manager, owner, big.NewInt(88)); err == nil {
		t.Fatal("failed to reject different pool: error=empty")
	}
	if _, err := discoverLock([]*types.Log{log, log}, owner, manager, pool, big.NewInt(88)); err == nil {
		t.Fatal("failed to reject ambiguous events: error=empty")
	}
	log.Removed = true
	if _, err := discoverLock([]*types.Log{log}, owner, manager, pool, big.NewInt(88)); err == nil {
		t.Fatal("failed to reject removed event: error=empty")
	}
}

type revertError struct{ data string }

// Error describes the fake JSON-RPC failure.
//
// Version:
//   - 2026-09-20: Added.
func (e revertError) Error() string { return "execution reverted" }

// ErrorData exposes ABI revert data as the live provider does.
//
// Version:
//   - 2026-09-20: Added.
func (e revertError) ErrorData() any { return e.data }

// TestRevertRequiresABIProof distinguishes contract rejection from transport errors.
//
// Version:
//   - 2026-09-20: Added.
func TestRevertRequiresABIProof(t *testing.T) {
	data := append(crypto.Keccak256([]byte("Error(string)"))[:4], word(big.NewInt(32))...)
	data = append(data, word(big.NewInt(7))...)
	data = append(data, append([]byte("NOT YET"), make([]byte, 25)...)...)
	err := fmt.Errorf("failed to call evm contract: %w", revertError{hexutil.Encode(data)})
	reason, decodeErr := decodeRevert(err)
	if decodeErr != nil || reason != "NOT YET" {
		t.Fatalf("failed to decode fixture revert: err=%v", decodeErr)
	}
	transportErr := errors.New("connection failed")
	if _, err := decodeRevert(transportErr); !errors.Is(err, transportErr) {
		t.Fatal("failed to preserve transport error: error_chain=invalid")
	}
	if _, err := decodeRevert(revertError{"0x00"}); err == nil {
		t.Fatal("failed to reject malformed revert: error=empty")
	}
}

// TestPackedStorageField verifies layout offsets independently of record names.
//
// Version:
//   - 2026-09-20: Added.
func TestPackedStorageField(t *testing.T) {
	f := storageField{Mapping: "17", Slot: "2", Offset: 20, Bytes: 6}
	key := big.NewInt(73)
	location, err := f.location(key)
	if err != nil {
		t.Fatal(err)
	}
	expected := common.BigToHash(new(big.Int).Add(crypto.Keccak256Hash(word(key), word(big.NewInt(17))).Big(), big.NewInt(2)))
	if location != expected {
		t.Fatal("failed to compute mapping member: slot=mismatch")
	}
	packed := new(big.Int).Lsh(big.NewInt(1234567), 160)
	packed.Or(packed, new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 160), big.NewInt(1)))
	if f.extract(word(packed)).Cmp(big.NewInt(1234567)) != 0 {
		t.Fatal("failed to decode packed field: value=mismatch")
	}
	f.Offset = 30
	if _, err := f.location(key); err == nil {
		t.Fatal("failed to reject invalid layout: error=empty")
	}
}
