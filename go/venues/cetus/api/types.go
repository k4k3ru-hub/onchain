package api

import (
	"encoding/json"
	"fmt"

	"github.com/k4k3ru-hub/onchain/go/sui"
)

type ListParams struct {
	Limit  int
	Offset int
}

type PoolPage struct {
	Total *int64 `json:"total"`
	Pools []Pool `json:"lp_list"`
}

// Pool retains provider statistics without converting APR units or assigning reward tokens.
// Numeric pointers preserve decimal text and distinguish missing/null from explicit zero.
type Pool struct {
	Address           string          `json:"address"`
	CoinAAddress      string          `json:"coin_a_address"`
	CoinBAddress      string          `json:"coin_b_address"`
	Symbol            string          `json:"symbol"`
	Name              string          `json:"name"`
	Fee               *json.Number    `json:"fee"`
	TickSpacing       *json.Number    `json:"tick_spacing"`
	PureTVLInUSD      *json.Number    `json:"pure_tvl_in_usd"`
	VolumeInUSD24h    *json.Number    `json:"vol_in_usd_24h"`
	Fee24h            *json.Number    `json:"fee_24_h"`
	TotalAPR          *json.Number    `json:"total_apr"` // Decimal ratio, not percentage points.
	APR               *APR            `json:"apr"`
	RewarderAPR       []*string       `json:"rewarder_apr"` // Percent-suffixed source strings; slots are not token mappings.
	RewarderUSD       []*json.Number  `json:"rewarder_usd"`
	IsClosed          *bool           `json:"is_closed"`
	IsDisplayRewarder *bool           `json:"is_display_rewarder"`
	RewarderDisplay1  *bool           `json:"rewarder_display1"`
	RewarderDisplay2  *bool           `json:"rewarder_display2"`
	RewarderDisplay3  *bool           `json:"rewarder_display3"`
	RewarderDisplay4  *bool           `json:"rewarder_display4"`
	RewarderDisplay5  *bool           `json:"rewarder_display5"`
	CoinA             *Coin           `json:"coin_a"`
	CoinB             *Coin           `json:"coin_b"`
	Object            *PoolObject     `json:"object"`
	StableFarming     json.RawMessage `json:"stable_farming"` // Active-farm schema is unverified.
	IsVaults          *bool           `json:"is_vaults"`
	ShowVaults        *bool           `json:"show_vaults"`
	Vaults            []string        `json:"vaults"`
}

type APR struct {
	FeeAPR24h *json.Number `json:"fee_apr_24h"` // Decimal ratio annualized from the provider's 24h basis.
}

type Coin struct {
	Address  string       `json:"address"`
	Name     string       `json:"name"`
	Symbol   string       `json:"symbol"`
	Decimals *uint8       `json:"decimals"`
	Balance  *json.Number `json:"balance"`
}

type PoolObject struct {
	IsPause          *bool           `json:"is_pause"`
	Liquidity        *json.Number    `json:"liquidity"`
	CurrentSqrtPrice *json.Number    `json:"current_sqrt_price"`
	FeeRate          *json.Number    `json:"fee_rate"`
	TickSpacing      *json.Number    `json:"tick_spacing"`
	RewarderManager  json.RawMessage `json:"rewarder_manager"` // Preserve emissions, coin identities and source update time verbatim.
}

func (p Pool) validate() error {
	id, err := sui.ParseAddress(p.Address)
	if err != nil {
		return fmt.Errorf("failed to validate cetus pool: %w", err)
	}
	if id.IsZero() {
		return fmt.Errorf("failed to validate cetus pool: address=empty")
	}
	return nil
}

type APIError struct {
	Code int
}

// Error describes an upstream application failure without exposing its message or payload.
//
// Version:
//   - 2026-09-14: Added.
func (e *APIError) Error() string {
	return fmt.Sprintf("failed to read cetus api response: code=%d", e.Code)
}

type detailError struct {
	operation string
	cause     error
}

// Error describes the operation without exposing underlying input values.
//
// Version:
//   - 2026-09-14: Added.
func (e *detailError) Error() string { return e.operation }

// Unwrap preserves the underlying error for inspection.
//
// Version:
//   - 2026-09-14: Added.
func (e *detailError) Unwrap() error { return e.cause }
