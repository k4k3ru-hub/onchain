package helius

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	onchainSolana "github.com/k4k3ru-hub/onchain/go/solana"
	"github.com/k4k3ru-hub/onchain/go/solana/spltoken"
)

type stubDoer struct{ body string }

func (s stubDoer) Do(*http.Request) (*http.Response, error) {
	return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(s.body))}, nil
}

type stubTransactionSource struct{ transaction *onchainSolana.Transaction }

func (s stubTransactionSource) Transaction(context.Context, onchainSolana.Signature) (*onchainSolana.Transaction, error) {
	return s.transaction, nil
}

func TestTransferEvents(t *testing.T) {
	var address, from, to, mint onchainSolana.Address
	address[0] = 1
	from[0] = 2
	to[0] = 3
	mint[0] = 4
	address = from
	var signature onchainSolana.Signature
	signature[0] = 1
	body := `{"jsonrpc":"2.0","id":"1","result":{"data":[{"signature":"` + signature.String() + `"}],"paginationToken":"10:1"}}`
	transaction := &onchainSolana.Transaction{Signature: signature, Slot: 10,
		PreTokenBalances:  []onchainSolana.TokenBalance{{AccountIndex: 1, Owner: &from, Mint: mint, Amount: "100", Decimals: 6}, {AccountIndex: 2, Owner: &to, Mint: mint, Amount: "0", Decimals: 6}},
		PostTokenBalances: []onchainSolana.TokenBalance{{AccountIndex: 1, Owner: &from, Mint: mint, Amount: "60", Decimals: 6}, {AccountIndex: 2, Owner: &to, Mint: mint, Amount: "40", Decimals: 6}},
	}
	client := &RPCClient{config: RPCConfig{BaseURL: "https://example.com", APIKey: "secret", Commitment: onchainSolana.CommitmentFinalized}, client: stubDoer{body: body}, transactions: stubTransactionSource{transaction: transaction}}
	events, err := client.TransferEvents(context.Background(), spltoken.TransferFilter{Address: address, Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].Amount != "40" || events[0].From != from || events[0].To != to {
		t.Fatalf("TransferEvents() = %+v", events)
	}
	page, err := client.TransferEventPage(context.Background(), spltoken.TransferFilter{Address: address, Limit: 10})
	if err != nil || page.NextCursor != "10:1" {
		t.Fatalf("TransferEventPage() page=%+v error=%v, want next cursor", page, err)
	}
}
