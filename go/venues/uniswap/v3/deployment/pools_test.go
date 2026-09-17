package deployment

import (
	"github.com/ethereum/go-ethereum/common"
	"github.com/k4k3ru-hub/onchain/go/venues/uniswap/v3/protocol"
	"testing"
)

// TestPoolDefinitions verifies the trusted definitions against CREATE2 addresses.
//
// Version:
//   - 2026-09-17: Verify USDC/WETH and USDC/cbBTC definitions.
func TestPoolDefinitions(t *testing.T) {
	deployment, err := ByChainID(84532)
	if err != nil {
		t.Fatal(err)
	}
	for _, address := range []string{"0x94bfc0574ff48e92ce43d495376c477b1d0eeec0", "0x46880b404cd35c165eddeff7421019f8dd25f4ad", "0x859d61699ff73db220608389ce8a71497d66923e", "0x3804f1c3f7abf426111e48fc5f1014c37eeb2c71", "0x56baae1b305a47a500b1d2f784b28ae4cfb66450", "0x7ce8c98f23e8e994a9bd9c0e4cf8d7ead7fa8546"} {
		pool, ok := LookupPool(84532, common.HexToAddress(address))
		if !ok {
			t.Fatal("missing definition")
		}
		calculated, err := pool.Key.Address(pool.Factory, deployment.InitCodeHash)
		if err != nil {
			t.Fatal(err)
		}
		if calculated != pool.Address || pool.Factory != deployment.Factory {
			t.Fatal("definition does not match deployment")
		}
		pool.Key.Fee = 123
		again, _ := LookupPool(84532, common.HexToAddress(address))
		if again.Key.Fee == 123 {
			t.Fatal("lookup exposed mutable state")
		}
		if _, ok := LookupPool(8453, common.HexToAddress(address)); ok {
			t.Fatal("definition leaked across networks")
		}
	}
	if _, ok := LookupPool(84532, common.HexToAddress("0x1234")); ok {
		t.Fatal("unknown pool resolved")
	}
}

// TestLookupPoolPreservesExplicitTokens verifies lookup does not impose a USDC position.
// The fixtures are local definitions, not claims about deployed pools.
//
// Version:
//   - 2026-09-17: Added.
func TestLookupPoolPreservesExplicitTokens(t *testing.T) {
	usdc := common.HexToAddress("0x036CbD53842c5426634e7929541eC2318f3dCF7e")
	address := common.HexToAddress("0x1234")
	definitions := []PoolDefinition{
		{ChainID: 84532, Address: address, Key: protocol.PoolKey{
			Token0: protocol.NewCurrency(common.HexToAddress("0x01")), Token1: protocol.NewCurrency(usdc), Fee: 500,
		}},
		{ChainID: 8453, Address: address, Key: protocol.PoolKey{
			Token0: protocol.NewCurrency(common.HexToAddress("0x02")), Token1: protocol.NewCurrency(common.HexToAddress("0x03")), Fee: 3000,
		}},
	}
	for _, want := range definitions {
		if err := want.Key.Validate(); err != nil {
			t.Fatal(err)
		}
		got, ok := lookupPool(definitions, want.ChainID, address)
		if !ok || got != want {
			t.Fatalf("lookup = %+v, %t; want %+v", got, ok, want)
		}
		got.Key.Token0 = protocol.NewCurrency(usdc)
		again, _ := lookupPool(definitions, want.ChainID, address)
		if again != want {
			t.Fatal("lookup exposed mutable definition")
		}
	}
	if _, ok := lookupPool(definitions, 1, address); ok {
		t.Fatal("unexpected chain match")
	}
}
