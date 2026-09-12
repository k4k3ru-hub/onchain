package dlmm

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
	c, _, requests := newCacheFixture(t)
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
	if got.Protocol != "dlmm" || len(got.PriceInputs) == 0 || got.PriceInputs[0].ReceivedAt.IsZero() {
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
	if len(got.Coverage) == 0 {
		t.Fatal("missing loaded arrays")
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
	binary.LittleEndian.PutUint32(snap.accounts.Accounts[snap.pool].Account.Data[76:80], 1)
	binary.LittleEndian.PutUint16(snap.accounts.Accounts[snap.pool].Account.Data[80:82], 100)
	numeric, err := snap.PoolSnapshot()
	if err != nil {
		t.Fatal(err)
	}
	value, ok := new(big.Rat).SetString(numeric.RawPrice)
	if !ok {
		t.Fatal("invalid ratio")
	}
	if value.FloatString(18) != "1.010000000000000000" {
		t.Fatalf("wrong protocol price: %s", value.FloatString(18))
	}

}
