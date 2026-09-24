package sui

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

type coinCaller struct {
	queryText string
	response  any
	err       error
}

func (c *coinCaller) query(_ context.Context, query string, target any) error {
	c.queryText = query
	if c.err != nil {
		return c.err
	}
	data, err := json.Marshal(c.response)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, target)
}
func coinResponse(t testing.TB) map[string]any {
	t.Helper()
	return map[string]any{"address": "0x91", "version": uint64(9007199254740993), "digest": loadTransactionVectors(t).ObjectDigest,
		"owner":    map[string]any{"__typename": "AddressOwner", "address": map[string]any{"address": "0x1"}},
		"contents": map[string]any{"type": map[string]any{"repr": "0x2::coin::Coin<0x2::sui::SUI>"}, "json": map[string]any{"balance": "18446744073709551615"}},
	}
}
func coinPageResponse(nodes []any) map[string]any {
	return map[string]any{"address": map[string]any{"address": "0x1", "objects": map[string]any{"nodes": nodes, "pageInfo": map[string]any{"hasNextPage": true, "endCursor": "next"}}}}
}

// TestCoinsReadAndResolve verifies pagination, u64 precision and stale reference rejection.
//
// Version:
//   - 2026-09-24: Added.
func TestCoinsReadAndResolve(t *testing.T) {
	node := coinResponse(t)
	caller := &coinCaller{response: coinPageResponse([]any{node})}
	client := composeRPCClient(RPCConfig{}, caller)
	query := CoinQuery{Owner: transactionTestAddress(t, "0x1"), CoinType: "0x2::sui::SUI"}
	page, err := client.Coins(nil, query)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Coins) != 1 || !page.HasNextPage || page.NextCursor != "next" || page.Coins[0].Balance != ^uint64(0) {
		t.Fatal("unexpected coin page")
	}
	if !strings.Contains(caller.queryText, "first: 50") || strings.Contains(caller.queryText, "after:") {
		t.Fatal("unexpected initial query")
	}
	query.After = "previous"
	if _, err := client.Coins(nil, query); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(caller.queryText, `after: "previous"`) {
		t.Fatal("cursor was not forwarded")
	}
	node["asMoveObject"] = map[string]any{"contents": node["contents"]}
	delete(node, "contents")
	caller.response = map[string]any{"object": node}
	coin, err := client.Coin(nil, page.Coins[0].Reference)
	if err != nil {
		t.Fatal(err)
	}
	if coin.Reference.Version != 9007199254740993 {
		t.Fatal("object version lost precision")
	}
	stale := coin.Reference
	stale.Version--
	if _, err := client.Coin(nil, stale); err == nil {
		t.Fatal("accepted stale coin")
	}
	stale = coin.Reference
	stale.Digest[0] ^= 1
	if _, err := client.Coin(nil, stale); err == nil {
		t.Fatal("accepted wrong digest")
	}
}

// TestCoinsRejectInvalidResponses verifies untrusted owner, type and pagination data.
//
// Version:
//   - 2026-09-24: Added.
func TestCoinsRejectInvalidResponses(t *testing.T) {
	for name, mutate := range map[string]func(map[string]any){
		"wrong owner": func(n map[string]any) {
			n["owner"] = map[string]any{"__typename": "AddressOwner", "address": map[string]any{"address": "0x3"}}
		},
		"unsupported owner": func(n map[string]any) { n["owner"] = map[string]any{"__typename": "Shared"} },
		"wrong type": func(n map[string]any) {
			n["contents"].(map[string]any)["type"] = map[string]any{"repr": "0x2::coin::Coin<0x3::usdc::USDC>"}
		},
		"overflow balance": func(n map[string]any) {
			n["contents"].(map[string]any)["json"] = map[string]any{"balance": "18446744073709551616"}
		},
		"missing balance": func(n map[string]any) { n["contents"].(map[string]any)["json"] = map[string]any{} },
		"missing version": func(n map[string]any) { delete(n, "version") },
	} {
		t.Run(name, func(t *testing.T) {
			node := coinResponse(t)
			mutate(node)
			client := composeRPCClient(RPCConfig{}, &coinCaller{response: coinPageResponse([]any{node})})
			if _, err := client.Coins(nil, CoinQuery{Owner: transactionTestAddress(t, "0x1"), CoinType: "0x2::sui::SUI"}); err == nil {
				t.Fatal("accepted invalid coin")
			}
		})
	}
	query := CoinQuery{Owner: transactionTestAddress(t, "0x1"), CoinType: "0x2::sui::SUI", After: "next"}
	client := composeRPCClient(RPCConfig{}, &coinCaller{response: coinPageResponse(nil)})
	if _, err := client.Coins(nil, query); err == nil {
		t.Fatal("accepted repeated cursor")
	}
	client = composeRPCClient(RPCConfig{}, &coinCaller{err: context.DeadlineExceeded})
	if _, err := client.Coins(nil, query); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal("lost error chain")
	}
	query.First = 51
	if _, err := client.Coins(nil, query); err == nil {
		t.Fatal("accepted excessive page size")
	}
}
