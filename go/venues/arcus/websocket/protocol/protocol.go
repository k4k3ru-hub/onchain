// Package protocol owns Arcus public WebSocket envelopes and typed channel data.
package protocol

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/k4k3ru-hub/onchain/go/venues/arcus/internal/safety"
	market "github.com/k4k3ru-hub/onchain/go/venues/arcus/protocol"
)

type Markets struct {
	IsSnapshot bool                         `json:"isSnapshot"`
	Markets    map[string]market.MarketInfo `json:"markets"`
}
type OraclePrice struct {
	MarketDisplayName string          `json:"marketDisplayName"`
	MarketID          market.MarketID `json:"marketId"`
	Price             string          `json:"price"`
	MarkPrice         string          `json:"markPrice"`
	MarkEpochNanos    int64           `json:"markEpochNanos"`
}
type OraclePrices struct {
	Epoch  int64         `json:"epoch"`
	Prices []OraclePrice `json:"prices"`
}
type PredictedFunding struct {
	Market string `json:"market"`
	Rate1h string `json:"rate1h"`
}
type Message struct {
	Type             string                    `json:"type"`
	Channel          string                    `json:"channel"`
	ID               string                    `json:"id"`
	SigFigs          int64                     `json:"sigFigs"`
	RoundStep        int64                     `json:"roundStep"`
	Reason           string                    `json:"reason"`
	RetryAfterMS     int64                     `json:"retryAfterMs"`
	Contents         json.RawMessage           `json:"contents"`
	OrderBook        *market.OrderbookSnapshot `json:"-"`
	BBO              *market.BBO               `json:"-"`
	Trades           []market.Trade            `json:"-"`
	Markets          *Markets                  `json:"-"`
	OraclePrices     *OraclePrices             `json:"-"`
	PredictedFunding *PredictedFunding         `json:"-"`
}
type ResponseError struct{ Status int }

// Error describes a server error without echoing its untrusted payload.
//
// Version:
//   - 2026-09-06: Added.
func (e *ResponseError) Error() string {
	return fmt.Sprintf("failed to receive arcus message: status=%d", e.Status)
}

// Decode decodes subscriptions, updates and control notices without reconstructing books.
// Unknown channels retain Contents. Degraded notices are returned for caller handling.
//
// Version:
//   - 2026-09-06: Added.
func Decode(data []byte) (*Message, error) {
	var control struct {
		Status int             `json:"status"`
		Error  json.RawMessage `json:"error"`
		Type   string          `json:"type"`
	}
	if err := json.Unmarshal(data, &control); err != nil {
		return nil, fmt.Errorf("failed to decode arcus envelope: %w", safety.Redact(err))
	}
	if control.Status >= 400 || control.Type == "error" || len(control.Error) > 0 && !bytes.Equal(bytes.TrimSpace(control.Error), []byte("null")) {
		return nil, &ResponseError{Status: control.Status}
	}
	var m Message
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("failed to decode arcus message: %w", safety.Redact(err))
	}
	if m.Type == "" {
		return nil, fmt.Errorf("failed to decode arcus message: type=empty")
	}
	if m.Type != "subscribed" && m.Type != "channel_data" {
		return &m, nil
	}
	if m.Channel == "" {
		return nil, fmt.Errorf("failed to decode arcus message: channel=empty")
	}
	hasContents := len(m.Contents) > 0 && !bytes.Equal(bytes.TrimSpace(m.Contents), []byte("null"))
	// Trades has no subscribe-time snapshot; predicted funding can lack cached data.
	if !hasContents && m.Type == "subscribed" && (m.Channel == "trades" || m.Channel == "predictedFunding" || m.Channel == "oraclePrices") {
		return &m, nil
	}
	if m.Type == "subscribed" && m.Channel == "trades" {
		// Mainnet acknowledges this live-only channel with contents: {}.
		var acknowledgement map[string]json.RawMessage
		if err := json.Unmarshal(m.Contents, &acknowledgement); err == nil && len(acknowledgement) == 0 {
			return &m, nil
		}
	}
	var target any
	switch m.Channel {
	case "l2Orderbook", "l2OrderbookUpdates":
		target = &m.OrderBook
	case "bbo":
		target = &m.BBO
	case "trades":
		target = &m.Trades
	case "markets":
		target = &m.Markets
	case "oraclePrices":
		target = &m.OraclePrices
	case "predictedFunding":
		target = &m.PredictedFunding
	default:
		return &m, nil
	}
	if !hasContents {
		return nil, fmt.Errorf("failed to decode arcus channel: contents=null")
	}
	if err := json.Unmarshal(m.Contents, target); err != nil {
		return nil, fmt.Errorf("failed to decode arcus channel: %w", safety.Redact(err))
	}
	if m.OrderBook != nil {
		var presence struct {
			LastSequenceID *uint64 `json:"lastSequenceId"`
		}
		if err := json.Unmarshal(m.Contents, &presence); err != nil {
			return nil, fmt.Errorf("failed to decode book sequence: %w", safety.Redact(err))
		}
		if presence.LastSequenceID == nil || m.OrderBook.Bids == nil || m.OrderBook.Asks == nil {
			return nil, fmt.Errorf("failed to decode arcus book: snapshot_or_delta=invalid")
		}
	}
	if m.Markets != nil && m.Markets.Markets == nil {
		return nil, fmt.Errorf("failed to decode arcus markets: markets=null")
	}
	return &m, nil
}
