package clmm

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/k4k3ru-hub/onchain/go/sui"
)

// TestPoolRewards checks reward reads through the composed client.
//
// Version:
//   - 2026-09-09: Added.
func TestPoolRewards(t *testing.T) {
	address, err := sui.ParseAddress("0x1")
	if err != nil {
		t.Fatal(err)
	}
	for _, body := range []string{
		`{"rewarder_manager":{"last_updated_time":"1","rewarders":[{"reward_coin":{"name":"0x2::sui::SUI"},"emissions_per_second":"18446744073709551617","growth_global":"0"}]}}`,
		`{"fields":{"rewarder_manager":{"fields":{"last_updated_time":"1","rewarders":[{"fields":{"reward_coin":{"fields":{"name":"0x2::sui::SUI"}},"emissions_per_second":"18446744073709551617","growth_global":"0"}}]}}}}`,
	} {
		provider := &clientTestProvider{object: &sui.Object{Address: address, Version: 7, Move: &sui.MoveObject{Type: "0x1::pool::Pool<0x2::sui::SUI,0x3::usdc::USDC>", JSON: json.RawMessage(body)}}}
		c, err := composeClient(testDeployment(), provider, provider, provider, &quoteTestSimulator{})
		if err != nil {
			t.Fatal(err)
		}
		state, err := c.PoolRewards(context.Background(), address)
		if err != nil {
			t.Fatal(err)
		}
		if state.ObjectVersion != 7 || state.Rewarders[0].EmissionsPerSecondX64.String() != "18446744073709551617" {
			t.Fatalf("incorrect rewards: %+v", state)
		}
	}
}

// TestPoolRewardsMissingAndEmpty distinguishes unavailable and empty rewards.
//
// Version:
//   - 2026-09-09: Added.
func TestPoolRewardsMissingAndEmpty(t *testing.T) {
	for _, tc := range []struct {
		body  string
		valid bool
	}{
		{`{}`, false}, {`{"rewarder_manager":{"last_updated_time":"1","rewarders":null}}`, false},
		{`{"rewarder_manager":{"last_updated_time":"1","rewarders":[]}}`, true},
		{`{"rewarder_manager":{"last_updated_time":"1","rewarders":[null]}}`, false},
	} {
		o := &sui.Object{Move: &sui.MoveObject{Type: "0x1::pool::Pool<0x2::sui::SUI,0x3::usdc::USDC>", JSON: json.RawMessage(tc.body)}}
		got, err := ParsePoolRewards(o)
		if (err == nil) != tc.valid {
			t.Fatalf("%s: %v", tc.body, err)
		}
		if tc.valid && (got.Rewarders == nil || len(got.Rewarders) != 0) {
			t.Fatal("empty reward list not preserved")
		}
	}
}
