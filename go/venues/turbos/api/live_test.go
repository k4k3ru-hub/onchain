package api

import (
	"context"
	"github.com/k4k3ru-hub/onchain/go/sui"
	"os"
	"testing"
	"time"
)

// TestLiveConfiguredPools checks public statistics only when explicitly enabled.
//
// Version:
//   - 2026-09-20: Added.
func TestLiveConfiguredPools(t *testing.T) {
	if os.Getenv("ONCHAIN_TURBOS_API_LIVE") != "1" {
		t.Skip("set ONCHAIN_TURBOS_API_LIVE=1")
	}
	client, err := NewClient(Config{})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	for _, raw := range []string{"0x0df4f02d0e210169cb6d5aabd03c3058328c06f2c4dbb0804faa041159c78443", "0xbca476e3c744648c65b1fae5551b86be8ad7f482ca9c2268dad1d6b4fd0e2635"} {
		id, err := sui.ParseAddress(raw)
		if err != nil {
			t.Fatal(err)
		}
		p, err := client.Pools.Get(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		if p.APR == nil || p.APR7d == nil || p.RewardAPR == nil || p.Volume24hUSD == nil || p.UpdatedAt == "" {
			t.Fatal("required sample statistics absent")
		}
		t.Logf("pool=%s apr_percent=%s reward_percent=%s rewards=%d updated_at=%s", p.PoolID, p.APR.String(), p.RewardAPR.String(), len(p.RewardInfos), p.UpdatedAt)
	}
}
