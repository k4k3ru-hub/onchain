package api

import "encoding/json"

// Pool preserves provider statistics and optional values without APR normalization.
type Pool struct {
	ID                     string       `json:"id"`
	ProgramID              string       `json:"programId"`
	Type                   string       `json:"type"`
	MintA                  *Token       `json:"mintA"`
	MintB                  *Token       `json:"mintB"`
	TVL                    *json.Number `json:"tvl"`
	FeeRate                *json.Number `json:"feeRate"`
	Day                    *Statistics  `json:"day"`
	Week                   *Statistics  `json:"week"`
	Month                  *Statistics  `json:"month"`
	RewardDefaultInfos     []Reward     `json:"rewardDefaultInfos"`
	RewardDefaultPoolInfos string       `json:"rewardDefaultPoolInfos"`
	FarmUpcomingCount      *uint64      `json:"farmUpcomingCount"`
	FarmOngoingCount       *uint64      `json:"farmOngoingCount"`
	FarmFinishedCount      *uint64      `json:"farmFinishedCount"`
}

type Statistics struct {
	Volume      *json.Number   `json:"volume"`
	VolumeQuote *json.Number   `json:"volumeQuote"`
	VolumeFee   *json.Number   `json:"volumeFee"`
	APR         *json.Number   `json:"apr"`
	FeeAPR      *json.Number   `json:"feeApr"`
	RewardAPR   []*json.Number `json:"rewardApr"`
}

type Token struct {
	Address   string `json:"address"`
	ProgramID string `json:"programId"`
	Symbol    string `json:"symbol"`
	Decimals  *uint8 `json:"decimals"`
}

type Reward struct {
	Mint      *Token       `json:"mint"`
	PerSecond *json.Number `json:"perSecond"`
	StartTime *uint64      `json:"startTime"`
	EndTime   *uint64      `json:"endTime"`
}
