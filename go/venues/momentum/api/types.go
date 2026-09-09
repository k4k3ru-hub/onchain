package api

import (
	"encoding/json"
	"fmt"

	"github.com/k4k3ru-hub/onchain/go/sui"
)

// Numeric values retain upstream decimal text, without float64 conversion or annualization.
// Nil denotes missing/null, not zero. APR/APY units and denominators are not normalized here.
type Pool struct {
	PoolID              string        `json:"poolId"`
	TokenXType          string        `json:"tokenXType"`
	TokenYType          string        `json:"tokenYType"`
	TVL                 *json.Number  `json:"tvl"`
	Volume24h           *json.Number  `json:"volume24h"`
	Fees24h             *json.Number  `json:"fees24h"`
	LPFeesPercent       *json.Number  `json:"lpFeesPercent"`
	ProtocolFeesPercent *json.Number  `json:"protocolFeesPercent"`
	Timestamp           string        `json:"timestamp"`
	APRBreakdown        *APRBreakdown `json:"aprBreakdown"`
	LegacyAPY           *json.Number  `json:"apy"`
	Rewarders           []Rewarder    `json:"rewarders"`
	TokenX              *Token        `json:"tokenX"`
	TokenY              *Token        `json:"tokenY"`
	FarmID              *string       `json:"farm_id"`
	FarmSource          *string       `json:"farm_source"`
	IsDeprecated        *bool         `json:"isDeprecated"`
}
type APRBreakdown struct {
	Total   *json.Number `json:"total"`
	Fee     *json.Number `json:"fee"`
	Rewards []RewardAPR  `json:"rewards"`
}
type RewardAPR struct {
	CoinType     string       `json:"coinType"`
	APR          *json.Number `json:"apr"`
	AmountPerDay *json.Number `json:"amountPerDay"`
}
type Rewarder struct {
	CoinType         string       `json:"coin_type"`
	FlowRate         *json.Number `json:"flow_rate"`
	RewardAmount     *json.Number `json:"reward_amount"`
	RewardsAllocated *json.Number `json:"rewards_allocated"`
	HasEnded         *bool        `json:"hasEnded"`
}
type Token struct {
	CoinType string       `json:"coinType"`
	Ticker   string       `json:"ticker"`
	Decimals *uint8       `json:"decimals"`
	Price    *json.Number `json:"price"`
}
type RewardsAPY struct {
	PoolID    string       `json:"pool_id"`
	Rewarders []Rewarder   `json:"rewarders"`
	APY       *json.Number `json:"apy"`
}

func (p Pool) validate() error {
	id, err := sui.ParseAddress(p.PoolID)
	if err != nil {
		return fmt.Errorf("failed to validate momentum pool: %w", err)
	}
	if id.IsZero() {
		return fmt.Errorf("failed to validate momentum pool: pool_id=empty")
	}
	return nil
}
