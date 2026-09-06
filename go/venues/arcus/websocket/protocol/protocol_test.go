package protocol_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/k4k3ru-hub/onchain/go/venues/arcus/websocket/protocol"
)

func TestPublicChannels(t *testing.T) {
	m, err := protocol.Decode([]byte(`{"type":"subscribed","channel":"l2OrderbookUpdates","id":"BTC-USD","sigFigs":5,"roundStep":2,"contents":{"bids":[["1.000000000000000001","2"]],"asks":[],"lastSequenceId":18446744073709551614}}`))
	if err != nil {
		t.Fatal(err)
	}
	if m.OrderBook.LastSequenceID != 18446744073709551614 || m.OrderBook.Bids[0][0] != "1.000000000000000001" || m.SigFigs != 5 || m.RoundStep != 2 {
		t.Fatal(m)
	}
	d, err := protocol.Decode([]byte(`{"type":"channel_data","channel":"l2OrderbookUpdates","id":"BTC-USD","contents":{"bids":[["1.000000000000000001","0"]],"asks":[],"lastSequenceId":18446744073709551615}}`))
	if err != nil || d.OrderBook.LastSequenceID != m.OrderBook.LastSequenceID+1 || d.OrderBook.Bids[0][1] != "0" {
		t.Fatalf("delta: %v", err)
	}
	m, err = protocol.Decode([]byte(`{"type":"channel_data","channel":"bbo","id":"BTC-USD","contents":{"bestBid":null,"bestAsk":{"price":"2","size":"1"},"timestamp":1788656400123456}}`))
	if err != nil || m.BBO.BestBid != nil || m.BBO.BestAsk.Price != "2" {
		t.Fatalf("bbo: %v", err)
	}
	m, err = protocol.Decode([]byte(`{"type":"channel_data","channel":"trades","id":"BTC-USD","contents":[{"tradeId":"a","sequenceNumber":9007199254740993},{"tradeId":"b","sequenceNumber":9007199254740993}]}`))
	if err != nil || len(m.Trades) != 2 || m.Trades[1].SequenceNumber != 9007199254740993 {
		t.Fatalf("trade batch: %v", err)
	}
	m, err = protocol.Decode([]byte(`{"type":"subscribed","channel":"markets","contents":{"isSnapshot":true,"markets":{"0":{"marketId":0,"lastTradePrice":"2","isOutsideRth":false,"upperTradingBound":null}}}}`))
	if err != nil || !m.Markets.IsSnapshot || m.Markets.Markets["0"].LastTradePrice != "2" || m.Markets.Markets["0"].IsOutsideRTH == nil || *m.Markets.Markets["0"].IsOutsideRTH {
		t.Fatalf("markets: %v", err)
	}
	m, err = protocol.Decode([]byte(`{"type":"channel_data","channel":"oraclePrices","id":"BTC-USD","contents":{"epoch":1788656400123456789,"prices":[{"marketId":1,"price":"10","markPrice":"0","markEpochNanos":1788656400123456788}]}}`))
	if err != nil || m.OraclePrices.Epoch != 1788656400123456789 || m.OraclePrices.Prices[0].MarkPrice != "0" {
		t.Fatalf("oracle: %v", err)
	}
	m, err = protocol.Decode([]byte(`{"type":"channel_data","channel":"predictedFunding","id":"BTC-USD","contents":{"market":"BTC-USD","rate1h":"-0.0000123456789012345"}}`))
	if err != nil || m.PredictedFunding.Rate1h != "-0.0000123456789012345" {
		t.Fatalf("prediction: %v", err)
	}
	for _, data := range []string{`{"type":"subscribed","channel":"predictedFunding","contents":{}}`, `{"type":"subscribed","channel":"predictedFunding"}`, `{"type":"subscribed","channel":"trades"}`, `{"type":"subscribed","channel":"trades","contents":{}}`, `{"type":"unsubscribed","channel":"markets"}`, `{"type":"degraded","channel":"markets","reason":"snapshot_unavailable","retryAfterMs":5000}`, `{"type":"channel_data","channel":"future","contents":{"value":1}}`} {
		if _, err := protocol.Decode([]byte(data)); err != nil {
			t.Fatal(err)
		}
	}
}
func TestMalformedDataAndRemoteErrors(t *testing.T) {
	for _, data := range []string{`null`, `{}`, `{"type":"channel_data"}`, `{"type":"channel_data","channel":"markets","contents":null}`, `{"type":"channel_data","channel":"l2Orderbook","contents":{"bids":[],"asks":[]}}`, `{"type":"channel_data","channel":"l2Orderbook","contents":{"bids":[["1"]],"asks":[],"lastSequenceId":1}}`, `{"type":"channel_data","channel":"l2Orderbook","contents":{"bids":[["1","2","3"]],"asks":[],"lastSequenceId":1}}`, `{"type":"channel_data","channel":"oraclePrices","contents":{"epoch":"secret"}}`} {
		if _, err := protocol.Decode([]byte(data)); err == nil || strings.Contains(err.Error(), "secret") {
			t.Fatalf("invalid payload accepted or leaked: %v", err)
		}
	}
	_, err := protocol.Decode([]byte(`{"id":1,"status":429,"error":{"message":"secret"}}`))
	var remote *protocol.ResponseError
	if !errors.As(err, &remote) || remote.Status != 429 || strings.Contains(err.Error(), "secret") {
		t.Fatal(err)
	}
}
