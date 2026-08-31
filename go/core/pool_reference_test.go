package core

import "testing"

func TestPoolReferenceNormalizeAndValidate(t *testing.T) {
	reference := PoolReference{Chain: ChainBase, Network: NetworkMainnet, Protocol: " Uniswap-V3 ", PoolID: " 0xabc "}.Normalize()
	if reference.Protocol != "uniswap-v3" || reference.PoolID != "0xabc" {
		t.Fatalf("Normalize() = %+v", reference)
	}
	if err := reference.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestPoolReferenceValidateRejectsInvalidValues(t *testing.T) {
	tests := []PoolReference{
		{Network: NetworkMainnet, Protocol: "uniswap-v3", PoolID: "pool"},
		{Chain: ChainBase, Protocol: "uniswap-v3", PoolID: "pool"},
		{Chain: ChainBase, Network: NetworkMainnet, PoolID: "pool"},
		{Chain: ChainBase, Network: NetworkMainnet, Protocol: "uniswap_v3", PoolID: "pool"},
		{Chain: ChainBase, Network: NetworkMainnet, Protocol: "uniswap-v3"},
	}
	for _, reference := range tests {
		if err := reference.Validate(); err == nil {
			t.Fatalf("Validate() reference=%+v error=nil", reference)
		}
	}
}
