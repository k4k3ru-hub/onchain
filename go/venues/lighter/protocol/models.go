// Package protocol defines Lighter market-data wire models without normalizing units.
package protocol

import "encoding/json"

type OrderBook struct {
	Symbol                 string `json:"symbol"`
	MarketID               int64  `json:"market_id"`
	MarketType             string `json:"market_type"`
	BaseAssetID            int64  `json:"base_asset_id"`
	QuoteAssetID           int64  `json:"quote_asset_id"`
	Status                 string `json:"status"`
	TakerFee               string `json:"taker_fee"`
	MakerFee               string `json:"maker_fee"`
	LiquidationFee         string `json:"liquidation_fee"`
	MinBaseAmount          string `json:"min_base_amount"`
	MinQuoteAmount         string `json:"min_quote_amount"`
	SupportedSizeDecimals  int64  `json:"supported_size_decimals"`
	SupportedPriceDecimals int64  `json:"supported_price_decimals"`
	SupportedQuoteDecimals int64  `json:"supported_quote_decimals"`
	OrderQuoteLimit        string `json:"order_quote_limit"`
	IsMakerFeeEnabled      bool   `json:"is_maker_fee_enabled"`
	IsTakerFeeEnabled      bool   `json:"is_taker_fee_enabled"`
	CreatedAt              string `json:"created_at"`
	Multiplier             string `json:"multiplier"`
}
type MarketConfig struct {
	MarketMarginMode           int64  `json:"market_margin_mode"`
	InsuranceFundAccountIndex  int64  `json:"insurance_fund_account_index"`
	LiquidationMode            int64  `json:"liquidation_mode"`
	ForceReduceOnly            bool   `json:"force_reduce_only"`
	FundingFeeDiscountsEnabled bool   `json:"funding_fee_discounts_enabled"`
	TradingHours               string `json:"trading_hours"`
	Hidden                     bool   `json:"hidden"`
	RFQEnabled                 bool   `json:"rfq_enabled"`
}
type PerpsOrderBookDetail struct {
	Symbol                       string          `json:"symbol"`
	MarketID                     int64           `json:"market_id"`
	MarketType                   string          `json:"market_type"`
	BaseAssetID                  int64           `json:"base_asset_id"`
	QuoteAssetID                 int64           `json:"quote_asset_id"`
	Status                       string          `json:"status"`
	TakerFee                     string          `json:"taker_fee"`
	MakerFee                     string          `json:"maker_fee"`
	LiquidationFee               string          `json:"liquidation_fee"`
	MinBaseAmount                string          `json:"min_base_amount"`
	MinQuoteAmount               string          `json:"min_quote_amount"`
	SupportedSizeDecimals        int64           `json:"supported_size_decimals"`
	SupportedPriceDecimals       int64           `json:"supported_price_decimals"`
	SupportedQuoteDecimals       int64           `json:"supported_quote_decimals"`
	OrderQuoteLimit              string          `json:"order_quote_limit"`
	SizeDecimals                 int64           `json:"size_decimals"`
	PriceDecimals                int64           `json:"price_decimals"`
	QuoteMultiplier              int64           `json:"quote_multiplier"`
	DefaultInitialMarginFraction int64           `json:"default_initial_margin_fraction"`
	MinInitialMarginFraction     int64           `json:"min_initial_margin_fraction"`
	MaintenanceMarginFraction    int64           `json:"maintenance_margin_fraction"`
	CloseoutMarginFraction       int64           `json:"closeout_margin_fraction"`
	LastTradePrice               json.Number     `json:"last_trade_price"`
	DailyTradesCount             int64           `json:"daily_trades_count"`
	DailyBaseTokenVolume         json.Number     `json:"daily_base_token_volume"`
	DailyQuoteTokenVolume        json.Number     `json:"daily_quote_token_volume"`
	DailyPriceLow                json.Number     `json:"daily_price_low"`
	DailyPriceHigh               json.Number     `json:"daily_price_high"`
	DailyPriceChange             json.Number     `json:"daily_price_change"`
	OpenInterest                 json.Number     `json:"open_interest"`
	DailyChart                   json.RawMessage `json:"daily_chart"`
	MarketConfig                 MarketConfig    `json:"market_config"`
	StrategyIndex                int64           `json:"strategy_index"`
	IsMakerFeeEnabled            bool            `json:"is_maker_fee_enabled"`
	IsTakerFeeEnabled            bool            `json:"is_taker_fee_enabled"`
	FundingClampSmall            string          `json:"funding_clamp_small"`
	FundingClampBig              string          `json:"funding_clamp_big"`
	BaseInterestRate             string          `json:"base_interest_rate"`
	CreatedAt                    string          `json:"created_at"`
	MarkPrice                    string          `json:"mark_price"`
	IndexPrice                   string          `json:"index_price"`
	Multiplier                   string          `json:"multiplier"`
	MarketFlags                  int64           `json:"market_flags"`
	FundingPremiumMultiplier     int64           `json:"funding_premium_multiplier"`
}
type SpotOrderBookDetail struct {
	Symbol                 string          `json:"symbol"`
	MarketID               int64           `json:"market_id"`
	MarketType             string          `json:"market_type"`
	BaseAssetID            int64           `json:"base_asset_id"`
	QuoteAssetID           int64           `json:"quote_asset_id"`
	Status                 string          `json:"status"`
	TakerFee               string          `json:"taker_fee"`
	MakerFee               string          `json:"maker_fee"`
	LiquidationFee         string          `json:"liquidation_fee"`
	MinBaseAmount          string          `json:"min_base_amount"`
	MinQuoteAmount         string          `json:"min_quote_amount"`
	OrderQuoteLimit        string          `json:"order_quote_limit"`
	SupportedSizeDecimals  int64           `json:"supported_size_decimals"`
	SupportedPriceDecimals int64           `json:"supported_price_decimals"`
	SupportedQuoteDecimals int64           `json:"supported_quote_decimals"`
	SizeDecimals           int64           `json:"size_decimals"`
	PriceDecimals          int64           `json:"price_decimals"`
	LastTradePrice         json.Number     `json:"last_trade_price"`
	DailyTradesCount       int64           `json:"daily_trades_count"`
	DailyBaseTokenVolume   json.Number     `json:"daily_base_token_volume"`
	DailyQuoteTokenVolume  json.Number     `json:"daily_quote_token_volume"`
	DailyPriceLow          json.Number     `json:"daily_price_low"`
	DailyPriceHigh         json.Number     `json:"daily_price_high"`
	DailyPriceChange       json.Number     `json:"daily_price_change"`
	DailyChart             json.RawMessage `json:"daily_chart"`
	IsMakerFeeEnabled      bool            `json:"is_maker_fee_enabled"`
	IsTakerFeeEnabled      bool            `json:"is_taker_fee_enabled"`
	CreatedAt              string          `json:"created_at"`
	Multiplier             string          `json:"multiplier"`
}
type SimpleOrder struct {
	OrderIndex          int64  `json:"order_index"`
	OrderID             string `json:"order_id"`
	OwnerAccountIndex   int64  `json:"owner_account_index"`
	InitialBaseAmount   string `json:"initial_base_amount"`
	RemainingBaseAmount string `json:"remaining_base_amount"`
	Price               string `json:"price"`
	OrderExpiry         int64  `json:"order_expiry"`
	TransactionTime     int64  `json:"transaction_time"`
}
type Trade struct {
	TradeID                          int64  `json:"trade_id"`
	TxHash                           string `json:"tx_hash"`
	Type                             string `json:"type"`
	MarketID                         int64  `json:"market_id"`
	Size                             string `json:"size"`
	Price                            string `json:"price"`
	USDAmount                        string `json:"usd_amount"`
	AskID                            int64  `json:"ask_id"`
	BidID                            int64  `json:"bid_id"`
	AskAccountID                     int64  `json:"ask_account_id"`
	BidAccountID                     int64  `json:"bid_account_id"`
	IsMakerAsk                       bool   `json:"is_maker_ask"`
	BlockHeight                      int64  `json:"block_height"`
	Timestamp                        int64  `json:"timestamp"`
	TakerFee                         int64  `json:"taker_fee"`
	TakerPositionSizeBefore          string `json:"taker_position_size_before"`
	TakerEntryQuoteBefore            string `json:"taker_entry_quote_before"`
	TakerInitialMarginFractionBefore int64  `json:"taker_initial_margin_fraction_before"`
	TakerPositionSignChanged         bool   `json:"taker_position_sign_changed"`
	MakerFee                         int64  `json:"maker_fee"`
	MakerPositionSizeBefore          string `json:"maker_position_size_before"`
	MakerEntryQuoteBefore            string `json:"maker_entry_quote_before"`
	MakerInitialMarginFractionBefore int64  `json:"maker_initial_margin_fraction_before"`
	MakerPositionSignChanged         bool   `json:"maker_position_sign_changed"`
	TransactionTime                  int64  `json:"transaction_time"`
	BidAccountPnL                    string `json:"bid_account_pnl"`
	AskAccountPnL                    string `json:"ask_account_pnl"`
	AskClientID                      int64  `json:"ask_client_id"`
	BidClientID                      int64  `json:"bid_client_id"`
	AskClientIDStr                   string `json:"ask_client_id_str"`
	BidClientIDStr                   string `json:"bid_client_id_str"`
	AskIDStr                         string `json:"ask_id_str"`
	BidIDStr                         string `json:"bid_id_str"`
	TradeIDStr                       string `json:"trade_id_str"`
	IntegratorMakerFee               int64  `json:"integrator_maker_fee"`
	IntegratorMakerFeeCollectorIndex int64  `json:"integrator_maker_fee_collector_index"`
	IntegratorTakerFee               int64  `json:"integrator_taker_fee"`
	IntegratorTakerFeeCollectorIndex int64  `json:"integrator_taker_fee_collector_index"`
	TakerAllocatedMarginUSDCBefore   int64  `json:"taker_allocated_margin_usdc_before"`
	TakerAllocatedMarginUSDCAfter    int64  `json:"taker_allocated_margin_usdc_after"`
	MakerAllocatedMarginUSDCBefore   int64  `json:"maker_allocated_margin_usdc_before"`
	MakerAllocatedMarginUSDCAfter    int64  `json:"maker_allocated_margin_usdc_after"`
	BidOrderVersion                  int64  `json:"bid_order_version"`
	AskOrderVersion                  int64  `json:"ask_order_version"`
}
type Funding struct {
	Timestamp int64  `json:"timestamp"`
	Value     string `json:"value"`
	Rate      string `json:"rate"`
	Direction string `json:"direction"`
}
type FundingRate struct {
	MarketID int64       `json:"market_id"`
	Exchange string      `json:"exchange"`
	Symbol   string      `json:"symbol"`
	Rate     json.Number `json:"rate"`
}
