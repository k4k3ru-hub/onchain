package sui

import (
	"strings"
	"testing"

	"github.com/mr-tron/base58"
)

func TestCoinMetadata(t *testing.T) {
	metadataAddress, _ := ParseAddress("0x123")
	caller := &fakeCaller{response: map[string]any{"coinMetadata": map[string]any{
		"address": metadataAddress.String(), "decimals": 9, "name": "Sui", "symbol": "SUI",
		"description": "", "iconUrl": "", "supply": "10000000000000000000",
		"regulatedState": "UNREGULATED", "allowGlobalPause": false, "supplyState": "FIXED",
	}}}
	client := composeRPCClient(RPCConfig{URL: "https://example.com/graphql"}, caller)
	metadata, err := client.CoinMetadata(nil, "0x2::sui::SUI")
	if err != nil {
		t.Fatalf("CoinMetadata() returned an unexpected error: %v", err)
	}
	if metadata.Symbol != "SUI" || metadata.Decimals != 9 || metadata.Supply.String() != "10000000000000000000" || metadata.MetadataAddress != metadataAddress {
		t.Fatalf("CoinMetadata() = %+v, want matching metadata", metadata)
	}
	if !strings.Contains(caller.queryValue, "0000000000000000000000000000000000000000000000000000000000000002::sui::SUI") {
		t.Fatalf("CoinMetadata() query = %q, want canonical coin type", caller.queryValue)
	}
}

func TestTransactionDigests(t *testing.T) {
	address, _ := ParseAddress("0x123")
	digestValue := base58.Encode(bytesWithFirstByte(20))
	caller := &fakeCaller{response: map[string]any{"transactions": map[string]any{
		"nodes":    []any{map[string]any{"digest": digestValue}},
		"pageInfo": map[string]any{"hasNextPage": true, "endCursor": "next"},
	}}}
	client := composeRPCClient(RPCConfig{URL: "https://example.com/graphql"}, caller)
	page, err := client.TransactionDigests(nil, TransactionQuery{AffectedAddress: address, First: 10})
	if err != nil {
		t.Fatalf("TransactionDigests() returned an unexpected error: %v", err)
	}
	if len(page.Digests) != 1 || page.Digests[0].String() != digestValue || !page.HasNextPage || page.NextCursor != "next" {
		t.Fatalf("TransactionDigests() = %+v, want matching page", page)
	}
	if !strings.Contains(caller.queryValue, "affectedAddress") || !strings.Contains(caller.queryValue, address.String()) {
		t.Fatalf("TransactionDigests() query = %q", caller.queryValue)
	}
}
