package protocol

import (
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/common"
)

func TestPoolKeyAddressMatchesKnownPool(t *testing.T) {
	t.Parallel()
	key := PoolKey{Token0: NewCurrency(common.HexToAddress("0xA0b86991c6218b36c1d19d4a2e9eb0ce3606eb48")), Token1: NewCurrency(common.HexToAddress("0xC02aaA39b223FE8D0A0e5C4F27eAD9083C756Cc2")), Fee: 3000}
	address, err := key.Address(common.HexToAddress("0x1F98431c8aD98523631AE4a59f267346ea31F984"), common.HexToHash("0xe34f199b19b2b4f47f68442619d555527d244f78a3297ea89325f843f87b8b54"))
	if err != nil {
		t.Fatalf("Address() error = %v", err)
	}
	if want := common.HexToAddress("0x8ad599c3a0ff1de082011efddc58f1908eb6e6d8"); address != want {
		t.Fatalf("Address() = %s, want %s", address.Hex(), want.Hex())
	}
}

func TestPoolKeyValidate(t *testing.T) {
	t.Parallel()
	token0 := NewCurrency(common.HexToAddress("0x01"))
	token1 := NewCurrency(common.HexToAddress("0x02"))
	for _, test := range []struct {
		name string
		key  PoolKey
		want string
	}{
		{"valid arbitrary fee", PoolKey{Token0: token0, Token1: token1, Fee: 42}, ""},
		{"zero token", PoolKey{Token1: token1, Fee: 1}, "token0=empty"},
		{"order", PoolKey{Token0: token1, Token1: token0, Fee: 1}, "token_order=invalid"},
		{"zero fee", PoolKey{Token0: token0, Token1: token1}, "fee=out_of_range"},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := test.key.Validate()
			if test.want == "" && err != nil {
				t.Fatalf("Validate() error = %v", err)
			}
			if test.want != "" && (err == nil || !strings.Contains(err.Error(), test.want)) {
				t.Fatalf("Validate() error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestParseCurrencyRejectsZero(t *testing.T) {
	t.Parallel()
	if _, err := ParseCurrency(common.Address{}.Hex()); err == nil || !strings.Contains(err.Error(), "address=empty") {
		t.Fatalf("ParseCurrency() error = %v", err)
	}
}
