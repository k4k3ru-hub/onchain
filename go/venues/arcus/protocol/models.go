// Package protocol owns Arcus market data with upstream decimal and timestamp units.
package protocol

type Candle struct {
	MarketDisplayName      MarketDisplayName `json:"marketDisplayName"`
	MarketID               MarketID          `json:"marketId"`
	Timeframe              string            `json:"timeframe"`
	OpenTime               int64             `json:"openTime"`
	Open                   string            `json:"open"`
	High                   string            `json:"high"`
	Low                    string            `json:"low"`
	Close                  string            `json:"close"`
	Volume                 string            `json:"volume"`
	TakerBuyVolume         string            `json:"takerBuyVolume"`
	NotionalVolume         string            `json:"notionalVolume"`
	TakerBuyNotionalVolume string            `json:"takerBuyNotionalVolume"`
	TradeCount             int64             `json:"tradeCount"`
	IsFinal                bool              `json:"isFinal"`
}

type MarketDisplayName string

type MarketID int64

type MarketInfo struct {
	LastTradePrice    string            `json:"lastTradePrice"`
	MarketDisplayName MarketDisplayName `json:"marketDisplayName"`
	FullAssetName     string            `json:"fullAssetName"`
	MarketID          MarketID          `json:"marketId"`
	Status            string            `json:"status"`
	BaseAsset         string            `json:"baseAsset"`
	QuoteAsset        string            `json:"quoteAsset"`
	TickSize          string            `json:"tickSize"`
	StepSize          string            `json:"stepSize"`
	TickTiers         []struct {
		UpToPrice string `json:"upToPrice"`
		Tick      string `json:"tick"`
	} `json:"tickTiers"`
	MinOrderNotional              string               `json:"minOrderNotional"`
	MinOrderSize                  string               `json:"minOrderSize"`
	MaxOrderSize                  string               `json:"maxOrderSize"`
	OraclePrice                   string               `json:"oraclePrice"`
	MarkPrice                     string               `json:"markPrice"`
	FundingRate                   string               `json:"fundingRate"`
	NextFundingRate               string               `json:"nextFundingRate"`
	NextFundingAt                 int64                `json:"nextFundingAt"`
	PriceChange24h                string               `json:"priceChange24h"`
	Volume24h                     string               `json:"volume24h"`
	Volume24hNotional             string               `json:"volume24hNotional"`
	High24h                       string               `json:"high24h"`
	Low24h                        string               `json:"low24h"`
	Trades24h                     int64                `json:"trades24h"`
	OpenInterest                  string               `json:"openInterest"`
	OpenInterestCap               string               `json:"openInterestCap"`
	InitialMarginFraction         string               `json:"initialMarginFraction"`
	MaintenanceMarginFraction     string               `json:"maintenanceMarginFraction"`
	OffHoursInitialMarginFraction string               `json:"offHoursInitialMarginFraction"`
	RegularTradingHours           *RegularTradingHours `json:"regularTradingHours"`
	IsOutsideRTH                  *bool                `json:"isOutsideRth"`
	CurrentSettlementPrice        *string              `json:"currentSettlementPrice"`
	UpperTradingBound             *string              `json:"upperTradingBound"`
	LowerTradingBound             *string              `json:"lowerTradingBound"`
	NextUpperTradingBound         *string              `json:"nextUpperTradingBound"`
	NextLowerTradingBound         *string              `json:"nextLowerTradingBound"`
	IsUpperInExpansionZone        *bool                `json:"isUpperInExpansionZone"`
	IsLowerInExpansionZone        *bool                `json:"isLowerInExpansionZone"`
	UpperZoneEnteredAt            *int64               `json:"upperZoneEnteredAt"`
	UpperExpectedExpansionAt      *int64               `json:"upperExpectedExpansionAt"`
	LowerZoneEnteredAt            *int64               `json:"lowerZoneEnteredAt"`
	LowerExpectedExpansionAt      *int64               `json:"lowerExpectedExpansionAt"`
	Type                          MarketType           `json:"type"`
	Category                      MarketCategory       `json:"category"`
	AddedTimestamp                int64                `json:"addedTimestamp"`
	AssetResolution               string               `json:"assetResolution"`
	PythID                        string               `json:"pythId"`
}

type RegularTradingHours struct {
	StartSecondsOfDay int64  `json:"startSecondsOfDay"`
	EndSecondsOfDay   int64  `json:"endSecondsOfDay"`
	Timezone          string `json:"timezone"`
	IsOvernight       bool   `json:"isOvernight"`
}

type MarketType string

type MarketCategory string

type Trade struct {
	MarketID          int64  `json:"marketId"`
	MarketDisplayName string `json:"marketDisplayName"`
	Side              string `json:"side"`
	Price             string `json:"price"`
	Size              string `json:"size"`
	TradeID           string `json:"tradeId"`
	Timestamp         int64  `json:"timestamp"`
	TakerOrderID      string `json:"takerOrderId"`
	TakerAddress      string `json:"takerAddress"`
	MakerOrderID      string `json:"makerOrderId"`
	MakerAddress      string `json:"makerAddress"`
	SequenceNumber    uint64 `json:"sequenceNumber"`
}

type OrderbookSnapshot struct {
	Bids             []PriceLevel `json:"bids"`
	Asks             []PriceLevel `json:"asks"`
	LastSequenceID   uint64       `json:"lastSequenceId"`
	GlobalSequenceID uint64       `json:"globalSequenceId"`
	Timestamp        int64        `json:"timestamp"`
}

type BBO struct {
	BestBid          *BBOLevel `json:"bestBid"`
	BestAsk          *BBOLevel `json:"bestAsk"`
	LastSequenceID   uint64    `json:"lastSequenceId"`
	GlobalSequenceID uint64    `json:"globalSequenceId"`
	Timestamp        int64     `json:"timestamp"`
}

type BBOLevel struct {
	Price string `json:"price"`
	Size  string `json:"size"`
}

type MarketFundingRate struct {
	MarketID          MarketID          `json:"marketId"`
	MarketDisplayName MarketDisplayName `json:"marketDisplayName"`
	FundingRate       string            `json:"fundingRate"`
	Time              int64             `json:"time"`
}
