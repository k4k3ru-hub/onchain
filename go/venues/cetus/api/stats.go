package api

import (
	"encoding/json"
	"errors"
)

var ErrPoolNotFound = errors.New("failed to find cetus pool: no indexed statistics")

// PoolStats preserves the v3 response independently of the v2 Pool schema.
// APR fields in observed v3 responses are decimal ratios, including mining rewards.
// Nil numeric pointers mean missing/null, never zero. Raw retains all provider fields,
// including unknown farming metadata, without interpreting eligibility or units.
type PoolStats struct {
	Pool            string           `json:"pool"`
	FeeRate         *json.Number     `json:"feeRate"`
	ShowReverse     *bool            `json:"showReverse"`
	CoinA           *StatsCoin       `json:"coinA"`
	CoinB           *StatsCoin       `json:"coinB"`
	TVL             *json.Number     `json:"tvl"`
	TotalAPR        *json.Number     `json:"totalApr"`
	Stats           []PeriodStats    `json:"stats"`
	MiningRewarders []MiningRewarder `json:"miningRewarders"`
	Vault           json.RawMessage  `json:"vault"`
	Extensions      json.RawMessage  `json:"extensions"`
	Raw             json.RawMessage  `json:"-"`
}

type StatsCoin struct {
	CoinType   string `json:"coinType"`
	Symbol     string `json:"symbol"`
	Decimals   *uint8 `json:"decimals"`
	IsVerified *bool  `json:"isVerified"`
	LogoURL    string `json:"logoURL"`
}

type PeriodStats struct {
	DateType string       `json:"dateType"` // Source labels include 24H, 7D and 30D.
	Volume   *json.Number `json:"vol"`
	Fee      *json.Number `json:"fee"`
	APR      *json.Number `json:"apr"` // Provider fee APR; no fallback across periods.
}

type MiningRewarder struct {
	CoinType           string       `json:"coinType"`
	Symbol             string       `json:"symbol"`
	Decimals           *uint8       `json:"decimals"`
	LogoURL            string       `json:"logoURL"`
	Display            *bool        `json:"display"`
	APR                *json.Number `json:"apr"`
	EmissionsPerSecond *json.Number `json:"emissionsPerSecond"`
}

// UnmarshalJSON decodes typed statistics and retains an independent copy of all source fields.
//
// Version:
//   - 2026-09-14: Added.
func (p *PoolStats) UnmarshalJSON(data []byte) error {
	type plain PoolStats
	var decoded plain
	if err := json.Unmarshal(data, &decoded); err != nil {
		return &detailError{operation: "failed to decode cetus pool statistics: response=invalid", cause: err}
	}
	decoded.Raw = append(json.RawMessage(nil), data...)
	*p = PoolStats(decoded)
	return nil
}
