package protocol

import (
	"testing"

	"github.com/ethereum/go-ethereum/common"
)

// TestNewPoolKeyOrdersCurrencies verifies new pool key orders currencies in the migrated SDK.
//
// Version:
//   - 2026-09-07: Migrated to onchain.
func TestNewPoolKeyOrdersCurrencies(t *testing.T) {
	t.Parallel()
	low := NewCurrency(common.HexToAddress("0x0000000000000000000000000000000000000001"))
	high := NewCurrency(common.HexToAddress("0x0000000000000000000000000000000000000002"))
	key, err := NewPoolKey(high, low, 100)
	if err != nil {
		t.Fatalf("NewPoolKey() error = %v", err)
	}
	if key.Token0 != low || key.Token1 != high || key.TickSpacing != 100 {
		t.Fatalf("NewPoolKey() = %+v", key)
	}
}

// TestPoolKeyValidateRejectsInvalidValues verifies pool key validate rejects invalid values in the migrated SDK.
//
// Version:
//   - 2026-09-07: Migrated to onchain.
func TestPoolKeyValidateRejectsInvalidValues(t *testing.T) {
	t.Parallel()
	low := NewCurrency(common.HexToAddress("0x0000000000000000000000000000000000000001"))
	high := NewCurrency(common.HexToAddress("0x0000000000000000000000000000000000000002"))
	tests := []PoolKey{
		{Token1: high, TickSpacing: 1},
		{Token0: low, TickSpacing: 1},
		{Token0: high, Token1: low, TickSpacing: 1},
		{Token0: low, Token1: high},
		{Token0: low, Token1: high, TickSpacing: MaxTickSpacing + 1},
	}
	for _, key := range tests {
		if err := key.Validate(); err == nil {
			t.Fatalf("Validate(%+v) error = nil", key)
		}
	}
}
