package protocol_test

import (
	"errors"
	"testing"

	"github.com/k4k3ru-hub/onchain/go/venues/lighter/websocket/protocol"
)

func TestDecodePublicMessages(t *testing.T) {
	snapshot, err := protocol.Decode([]byte(`{"type":"subscribed/order_book","channel":"order_book:0","order_book":{"code":200,"nonce":9007199254740993,"asks":[{"price":"123.000000000000001","size":"2"}],"bids":[]}}`))
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.OrderBook.BeginNonce != nil || snapshot.OrderBook.Nonce != 9007199254740993 || snapshot.OrderBook.Asks[0].Price != "123.000000000000001" {
		t.Fatalf("lost snapshot precision: %+v", snapshot)
	}
	delta, err := protocol.Decode([]byte(`{"type":"update/order_book","channel":"order_book:0","order_book":{"nonce":9007199254740994,"begin_nonce":9007199254740993,"asks":[{"price":"123","size":"0"}]}}`))
	if err != nil {
		t.Fatal(err)
	}
	if *delta.OrderBook.BeginNonce != snapshot.OrderBook.Nonce || delta.OrderBook.Asks[0].Size != "0" {
		t.Fatal("lost delta semantics")
	}
	for _, payload := range []string{
		`{"type":"update/market_stats","channel":"market_stats:0","market_stats":{"market_id":0,"current_funding_rate":"0.001","daily_quote_token_volume":9007199254740993.123}}`,
		`{"type":"update/market_stats","channel":"market_stats:all","market_stats":{"0":{"market_id":0,"current_funding_rate":"0.001","daily_quote_token_volume":9007199254740993.123}}}`,
	} {
		m, err := protocol.Decode([]byte(payload))
		if err != nil {
			t.Fatal(err)
		}
		if m.MarketStats["0"].DailyQuoteTokenVolume.String() != "9007199254740993.123" {
			t.Fatal("lost stats precision")
		}
	}
	m, err := protocol.Decode([]byte(`{"type":"update/trade","channel":"trade:0","trades":[{"trade_id":1}],"liquidation_trades":[{"trade_id":2}]}`))
	if err != nil || len(m.Trades) != 1 || len(m.LiquidationTrades) != 1 {
		t.Fatalf("trade decode: %v %+v", err, m)
	}
	m, err = protocol.Decode([]byte(`{"type":"future/control","channel":"future:0"}`))
	if err != nil || m.Type != "future/control" {
		t.Fatal("unknown control discarded")
	}
}

func TestDecodeErrors(t *testing.T) {
	for _, payload := range []string{`null`, `{}`, `{"type":"update/order_book"}`, `{"type":"update/market_stats","channel":"market_stats:all","market_stats":null}`} {
		if _, err := protocol.Decode([]byte(payload)); err == nil {
			t.Fatalf("accepted %s", payload)
		}
	}
	_, err := protocol.Decode([]byte(`{"type":"error","error":{"code":42,"message":"private payload"}}`))
	var remote *protocol.ResponseError
	if !errors.As(err, &remote) || remote.Code != 42 {
		t.Fatalf("error chain: %v", err)
	}
}
