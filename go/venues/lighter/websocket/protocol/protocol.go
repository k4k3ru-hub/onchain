// Package protocol defines Lighter public WebSocket messages.
package protocol

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/k4k3ru-hub/onchain/go/venues/lighter/internal/safety"
	market "github.com/k4k3ru-hub/onchain/go/venues/lighter/protocol"
)

type Request struct {
	Type    string `json:"type"`
	Channel string `json:"channel"`
}
type PriceLevel struct {
	Price string `json:"price"`
	Size  string `json:"size"`
}
type OrderBook struct {
	Code          int          `json:"code"`
	Asks          []PriceLevel `json:"asks"`
	Bids          []PriceLevel `json:"bids"`
	Offset        int64        `json:"offset"`
	Nonce         int64        `json:"nonce"`
	BeginNonce    *int64       `json:"begin_nonce"`
	LastUpdatedAt int64        `json:"last_updated_at"`
}
type Ticker struct {
	Symbol        string     `json:"s"`
	Ask           PriceLevel `json:"a"`
	Bid           PriceLevel `json:"b"`
	LastUpdatedAt int64      `json:"last_updated_at"`
}
type MarketStats struct {
	Symbol                string      `json:"symbol"`
	MarketID              int64       `json:"market_id"`
	IndexPrice            string      `json:"index_price"`
	MarkPrice             string      `json:"mark_price"`
	MidPrice              string      `json:"mid_price"`
	BestAskPrice          string      `json:"best_ask_price"`
	BestBidPrice          string      `json:"best_bid_price"`
	OpenInterest          string      `json:"open_interest"`
	OpenInterestLimit     string      `json:"open_interest_limit"`
	FundingClampSmall     string      `json:"funding_clamp_small"`
	FundingClampBig       string      `json:"funding_clamp_big"`
	BaseInterestRate      string      `json:"base_interest_rate"`
	LastTradePrice        string      `json:"last_trade_price"`
	CurrentFundingRate    string      `json:"current_funding_rate"`
	FundingRate           string      `json:"funding_rate"`
	FundingTimestamp      int64       `json:"funding_timestamp"`
	DailyBaseTokenVolume  json.Number `json:"daily_base_token_volume"`
	DailyQuoteTokenVolume json.Number `json:"daily_quote_token_volume"`
	DailyPriceLow         json.Number `json:"daily_price_low"`
	DailyPriceHigh        json.Number `json:"daily_price_high"`
	DailyPriceChange      json.Number `json:"daily_price_change"`
}
type SpotMarketStats struct {
	Symbol                string      `json:"symbol"`
	MarketID              int64       `json:"market_id"`
	IndexPrice            string      `json:"index_price"`
	MidPrice              string      `json:"mid_price"`
	LastTradePrice        string      `json:"last_trade_price"`
	DailyBaseTokenVolume  json.Number `json:"daily_base_token_volume"`
	DailyQuoteTokenVolume json.Number `json:"daily_quote_token_volume"`
	DailyPriceLow         json.Number `json:"daily_price_low"`
	DailyPriceHigh        json.Number `json:"daily_price_high"`
	DailyPriceChange      json.Number `json:"daily_price_change"`
}
type Message struct {
	Type              string                     `json:"type"`
	Channel           string                     `json:"channel"`
	Timestamp         int64                      `json:"timestamp"`
	LastUpdatedAt     int64                      `json:"last_updated_at"`
	Offset            int64                      `json:"offset"`
	Nonce             int64                      `json:"nonce"`
	OrderBook         *OrderBook                 `json:"order_book"`
	Ticker            *Ticker                    `json:"ticker"`
	Trades            []market.Trade             `json:"trades"`
	LiquidationTrades []market.Trade             `json:"liquidation_trades"`
	MarketStats       map[string]MarketStats     `json:"-"`
	SpotMarketStats   map[string]SpotMarketStats `json:"-"`
}
type ResponseError struct{ Code int }

// Error describes a server error without echoing the remote message.
//
// Version:
//   - 2026-09-06: Added.
func (e *ResponseError) Error() string {
	return fmt.Sprintf("failed to receive lighter message: code=%d", e.Code)
}

// Decode decodes public messages, retaining control messages and separating liquidations.
// Stats for one market or all markets are keyed by the server market identifier.
// Unknown message types preserve their type and channel for forward compatibility.
//
// Version:
//   - 2026-09-06: Added.
func Decode(data []byte) (*Message, error) {
	type message Message
	var raw struct {
		message
		MarketStats     json.RawMessage `json:"market_stats"`
		SpotMarketStats json.RawMessage `json:"spot_market_stats"`
		Error           *struct {
			Code int `json:"code"`
		} `json:"error"`
		Code int `json:"code"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("failed to decode lighter message: %w", safety.Redact(err))
	}
	if raw.Type == "" {
		return nil, fmt.Errorf("failed to decode lighter message: type=empty")
	}
	if raw.Error != nil {
		return nil, &ResponseError{Code: raw.Error.Code}
	}
	if raw.Type == "error" {
		return nil, &ResponseError{Code: raw.Code}
	}
	m := Message(raw.message)
	if raw.MarketStats != nil {
		stats, err := decodeStats[MarketStats](raw.Channel, "market_stats", raw.MarketStats)
		if err != nil {
			return nil, err
		}
		m.MarketStats = stats
	}
	if raw.SpotMarketStats != nil {
		stats, err := decodeStats[SpotMarketStats](raw.Channel, "spot_market_stats", raw.SpotMarketStats)
		if err != nil {
			return nil, err
		}
		m.SpotMarketStats = stats
	}
	if (m.Type == "subscribed/order_book" || m.Type == "update/order_book") && m.OrderBook == nil {
		return nil, fmt.Errorf("failed to decode lighter message: order_book=null")
	}
	if m.OrderBook != nil && m.OrderBook.Code != 0 && m.OrderBook.Code != 200 {
		return nil, &ResponseError{Code: m.OrderBook.Code}
	}
	return &m, nil
}

func decodeStats[T any](channel, prefix string, data []byte) (map[string]T, error) {
	key, ok := strings.CutPrefix(channel, prefix+":")
	if !ok || key == "" || string(data) == "null" {
		return nil, fmt.Errorf("failed to decode market statistics: channel_or_payload=invalid")
	}
	values := make(map[string]T)
	if key == "all" {
		if err := json.Unmarshal(data, &values); err != nil {
			return nil, fmt.Errorf("failed to decode market statistics: %w", safety.Redact(err))
		}
	} else {
		var value T
		if err := json.Unmarshal(data, &value); err != nil {
			return nil, fmt.Errorf("failed to decode market statistics: %w", safety.Redact(err))
		}
		values[key] = value
	}
	return values, nil
}
