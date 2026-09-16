package v3

import (
	"github.com/ethereum/go-ethereum/common"
	"math/big"
	"testing"
	"time"
)

func TestInitialPriceIsDetachedAndNotQuoteReady(t *testing.T) {
	c := &StateCache{}
	s := InitialPriceSnapshot{BlockNumber: 7, BlockHash: common.HexToHash("0x1234"), CapturedAt: time.Now(), SqrtPriceX96: new(big.Int).Lsh(big.NewInt(1), 96)}
	if err := c.StoreInitialPrice(s); err != nil {
		t.Fatal(err)
	}
	s.SqrtPriceX96.SetInt64(0)
	got := c.InitialPrice()
	if got.SqrtPriceX96.Sign() <= 0 || got.BlockNumber != 7 {
		t.Fatal("input aliased")
	}
	got.SqrtPriceX96.SetInt64(0)
	if c.InitialPrice().SqrtPriceX96.Sign() <= 0 {
		t.Fatal("output aliased")
	}
	if c.CaptureQuoteSnapshot() != nil {
		t.Fatal("partial state treated as quote ready")
	}
	if err := c.StoreInitialPrice(s); err == nil {
		t.Fatal("invalid state accepted")
	}
}
