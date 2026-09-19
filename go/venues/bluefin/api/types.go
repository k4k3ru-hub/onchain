package api

import "encoding/json"

// Numeric pointers retain missing/null values and exact provider decimal precision.
type Pool struct {
	Address string       `json:"address"`
	FeeRate *json.Number `json:"feeRate"`
	TVL     *json.Number `json:"tvl"`
	Paused  *bool        `json:"is_paused"`
	Day     *Period      `json:"day"`
	Week    *Period      `json:"week"`
	Month   *Period      `json:"month"`
	Rewards *[]Reward    `json:"rewards"`
	TokenA  PoolToken    `json:"tokenA"`
	TokenB  PoolToken    `json:"tokenB"`
}
type PoolToken struct {
	Info Token `json:"info"`
}
type Token struct {
	Address  string `json:"address"`
	Decimals *uint8 `json:"decimals"`
	Symbol   string `json:"symbol"`
}
type Period struct {
	APR    *APR         `json:"apr"`
	Fee    *json.Number `json:"fee"`
	Volume *json.Number `json:"volume"`
}
type APR struct {
	Fee    *json.Number `json:"feeApr"`
	Reward *json.Number `json:"rewardApr"`
	Total  *json.Number `json:"total"`
}
type Reward struct {
	DailyAmount *json.Number `json:"dailyRewards"`
	DailyUSD    *json.Number `json:"dailyRewardsUsd"`
	EndTime     *string      `json:"endTime"`
	Token       Token        `json:"token"`
}
