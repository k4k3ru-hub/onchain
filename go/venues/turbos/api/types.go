package api

import "encoding/json"

// Numeric pointers preserve source decimal text and distinguish missing from zero.
type Pool struct {
	PoolID                  string       `json:"pool_id"`
	CoinTypeA               string       `json:"coin_type_a"`
	CoinTypeB               string       `json:"coin_type_b"`
	Fee                     *json.Number `json:"fee"`
	FeeProtocol             *json.Number `json:"fee_protocol"`
	Unlocked                *bool        `json:"unlocked"`
	LiquidityUSD            *json.Number `json:"liquidity_usd"`
	Volume24hUSD            *json.Number `json:"volume_24h_usd"`
	Volume7dUSD             *json.Number `json:"volume_7d_usd"`
	Volume30dUSD            *json.Number `json:"volume_30d_usd"`
	Fee24hUSD               *json.Number `json:"fee_24h_usd"`
	Fee7dUSD                *json.Number `json:"fee_7d_usd"`
	APR                     *json.Number `json:"apr"`
	APR7d                   *json.Number `json:"apr_7d"`
	FeeAPR                  *json.Number `json:"fee_apr"`
	Fee7dAPR                *json.Number `json:"fee_7d_apr"`
	RewardAPR               *json.Number `json:"reward_apr"`
	Reward7dAPR             *json.Number `json:"reward_7d_apr"`
	RewardInfos             []RewardInfo `json:"reward_infos"`
	RewardLastUpdatedTimeMS *json.Number `json:"reward_last_updated_time_ms"`
	UpdatedAt               string       `json:"updated_at"`
	IsVault                 *bool        `json:"is_vault"`
}

type RewardInfo struct {
	Vault              string       `json:"vault"`
	VaultCoinType      string       `json:"vault_coin_type"`
	EmissionsPerSecond *json.Number `json:"emissions_per_second"`
}
