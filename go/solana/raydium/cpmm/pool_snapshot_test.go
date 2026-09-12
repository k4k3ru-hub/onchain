package cpmm

import (
	"context"
	"encoding/binary"
	"math/big"
	"reflect"
	"testing"
	"time"
)

// TestPoolSnapshotUsesFrozenPriceInputs verifies local extraction, detached fields and price-specific freshness.
//
// Version:
//   - 2026-09-12: Added.
func TestPoolSnapshotUsesFrozenPriceInputs(t *testing.T) {
	c, _ := cacheFixture(t)
	requests := []ExactInputRequest{{InputMint: testAddress(5), AmountIn: 100000}}
	if _, err := c.QuoteExactInputs(context.Background(), requests); err != nil {
		t.Fatal(err)
	}
	c.connected = len(c.addresses)
	var snap *QuoteSnapshot
	c.SetQuoteSnapshotObserver(func(s *QuoteSnapshot) { snap = s })
	got, err := snap.PoolSnapshot()
	if err != nil {
		t.Fatal(err)
	}
	ratio, ok := new(big.Rat).SetString(got.RawPrice)
	if !ok || ratio.Sign() <= 0 {
		t.Fatalf("invalid price: %+v", got)
	}
	if got.Protocol != "cpmm" || len(got.PriceInputs) == 0 || got.PriceInputs[0].ReceivedAt.IsZero() {
		t.Fatalf("missing metadata: %+v", got)
	}
	before := got.PriceInputs[0].ReceivedAt
	for address, a := range snap.accounts.Accounts {
		if address != snap.pool {
			a.ObservedAt = before.Add(time.Hour)
			snap.accounts.Accounts[address] = a
		}
	}
	after, err := snap.PoolSnapshot()
	if err != nil {
		t.Fatal(err)
	}
	if after.PriceInputs[0].ReceivedAt != before || got.RawPrice != after.RawPrice {
		t.Fatal("unrelated account changed price provenance")
	}
	if len(got.PriceInputs) != 3 || after.PriceInputs[1].ReceivedAt != before.Add(time.Hour) {
		t.Fatal("vault provenance omitted")
	}
	got.Components["pool"].Fields["corruption"] = "value"
	again, err := snap.PoolSnapshot()
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := again.Components["pool"].Fields["corruption"]; ok {
		t.Fatal("mutable alias")
	}
	if !reflect.DeepEqual(after, again) {
		t.Fatal("repeated local capture changed")
	}
	if _, err := (*QuoteSnapshot)(nil).PoolSnapshot(); err == nil {
		t.Fatal("nil accepted")
	}
	binary.LittleEndian.PutUint64(snap.accounts.Accounts[snap.pool].Account.Data[341:349], 100000)
	numeric, err := snap.PoolSnapshot()
	if err != nil {
		t.Fatal(err)
	}
	value, ok := new(big.Rat).SetString(numeric.RawPrice)
	if !ok {
		t.Fatal("invalid ratio")
	}
	if value.FloatString(18) != "2.222222222222222222" {
		t.Fatalf("wrong protocol price: %s", value.FloatString(18))
	}

}
