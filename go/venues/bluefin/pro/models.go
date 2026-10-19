// Package pro implements the Bluefin Pro perpetual protocol independently of Spot.
package pro

import (
	"fmt"
	"math/big"
	"strconv"
	"strings"

	"github.com/k4k3ru-hub/onchain/go/sui"
)

type Network string

const (
	Mainnet Network = "mainnet"
	Testnet Network = "testnet"
)

type Endpoints struct{ API, Auth, Trade, MarketStream, AccountStream string }

// EndpointsFor resolves only official Bluefin Pro environments.
//
// Version:
//   - 2026-10-14: Added.
func EndpointsFor(network Network) (Endpoints, error) {
	environment := ""
	switch network {
	case Mainnet:
		environment = "sui-prod"
	case Testnet:
		environment = "sui-staging"
	default:
		return Endpoints{}, fmt.Errorf("failed to resolve bluefin endpoints: network=invalid")
	}
	return Endpoints{
		API:           "https://api." + environment + ".bluefin.io",
		Auth:          "https://auth.api." + environment + ".bluefin.io",
		Trade:         "https://trade.api." + environment + ".bluefin.io",
		MarketStream:  "wss://stream.api." + environment + ".bluefin.io/ws/market",
		AccountStream: "wss://stream.api." + environment + ".bluefin.io/ws/account",
	}, nil
}

type Contracts struct {
	IDSID   string  `json:"idsId"`
	EDSID   string  `json:"edsId"`
	Network Network `json:"network"`
}
type Asset struct {
	Symbol          string `json:"symbol"`
	AssetType       string `json:"assetType"`
	Decimals        uint8  `json:"decimals"`
	MarginAvailable bool   `json:"marginAvailable"`
}
type ExchangeInfo struct {
	Assets          []Asset   `json:"assets"`
	Contracts       Contracts `json:"contractsConfig"`
	Markets         []Market  `json:"markets"`
	ServerTimeMS    int64     `json:"serverTimeAtMillis"`
	TradingGasFeeE9 string    `json:"tradingGasFeeE9"`
}
type Market struct {
	Symbol              string   `json:"symbol"`
	Status              string   `json:"status"`
	BaseAssetSymbol     string   `json:"baseAssetSymbol"`
	BaseAssetDecimals   uint8    `json:"baseAssetDecimals"`
	StepSizeE9          string   `json:"stepSizeE9"`
	TickSizeE9          string   `json:"tickSizeE9"`
	MinimumQuantityE9   string   `json:"minOrderQuantityE9"`
	MaximumQuantityE9   string   `json:"maxLimitOrderQuantityE9"`
	MinimumPriceE9      string   `json:"minOrderPriceE9"`
	MaximumPriceE9      string   `json:"maxOrderPriceE9"`
	DefaultLeverageE9   string   `json:"defaultLeverageE9"`
	MaxNotionalAtOpenE9 []string `json:"maxNotionalAtOpenE9"`
	MakerFeeE9          string   `json:"defaultMakerFeeE9"`
	TakerFeeE9          string   `json:"defaultTakerFeeE9"`
	IsolatedOnly        *bool    `json:"isolatedOnly"`
}

// ResolveSUI validates the current SUI market and environment-specific USDC collateral.
//
// Version:
//   - 2026-10-14: Added.
func (i ExchangeInfo) ResolveSUI(network Network) (Market, Asset, error) {
	if _, err := EndpointsFor(network); err != nil {
		return Market{}, Asset{}, err
	}
	if i.Contracts.Network != network || i.ServerTimeMS <= 0 {
		return Market{}, Asset{}, fmt.Errorf("failed to resolve bluefin sui market: environment=invalid")
	}
	if err := address(i.Contracts.IDSID); err != nil {
		return Market{}, Asset{}, fmt.Errorf("failed to resolve bluefin sui market: %w", err)
	}
	var selected Market
	for _, m := range i.Markets {
		if m.Symbol == "SUI-PERP" {
			if selected.Symbol != "" {
				return Market{}, Asset{}, fmt.Errorf("failed to resolve bluefin sui market: market=invalid")
			}
			selected = m
		}
	}
	if err := selected.Validate(); err != nil {
		return Market{}, Asset{}, fmt.Errorf("failed to resolve bluefin sui market: %w", err)
	}
	var collateral Asset
	for _, a := range i.Assets {
		if a.Symbol == "USDC" {
			if collateral.Symbol != "" || !a.MarginAvailable || a.Decimals != 6 {
				return Market{}, Asset{}, fmt.Errorf("failed to resolve bluefin sui market: collateral=invalid")
			}
			collateral = a
		}
	}
	if collateral.Symbol == "" || collateral.AssetType == "" {
		return Market{}, Asset{}, fmt.Errorf("failed to resolve bluefin sui market: collateral=empty")
	}
	parts := strings.Split(collateral.AssetType, "::")
	if len(parts) != 3 || address(parts[0]) != nil || !moveIdentifier(parts[1]) || !moveIdentifier(parts[2]) {
		return Market{}, Asset{}, fmt.Errorf("failed to resolve bluefin sui market: collateral_type=invalid")
	}
	if network == Mainnet && collateral.AssetType != "0xdba34672e30cb065b1f93e3ab55318768fd6fef66c15942c9f7cb846e2f900e7::usdc::USDC" {
		return Market{}, Asset{}, fmt.Errorf("failed to resolve bluefin sui market: native_usdc=invalid")
	}
	return selected, collateral, nil
}

func moveIdentifier(value string) bool {
	if value == "" || len(value) > 128 {
		return false
	}
	for i, c := range value {
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || c == '_' || i > 0 && c >= '0' && c <= '9' {
			continue
		}
		return false
	}
	return true
}

// Validate verifies supported SUI metadata before it can authorize an order.
//
// Version:
//   - 2026-10-14: Added.
func (m Market) Validate() error {
	if m.Symbol != "SUI-PERP" || m.BaseAssetSymbol != "SUI" || m.Status != "ACTIVE" || m.IsolatedOnly == nil || m.BaseAssetDecimals != 9 {
		return fmt.Errorf("failed to validate bluefin market: market=invalid")
	}
	for _, value := range []string{m.StepSizeE9, m.TickSizeE9, m.MinimumQuantityE9, m.MaximumQuantityE9, m.MinimumPriceE9, m.MaximumPriceE9, m.DefaultLeverageE9} {
		if _, err := integer(value, false); err != nil {
			return fmt.Errorf("failed to validate bluefin market: %w", err)
		}
	}
	for _, value := range []string{m.MakerFeeE9, m.TakerFeeE9} {
		if _, err := FormatE9(value); err != nil {
			return fmt.Errorf("failed to validate bluefin market: %w", err)
		}
	}
	return nil
}

// ValidateOrder enforces exact price and quantity increments and current market bounds.
//
// Version:
//   - 2026-10-14: Added.
func (m Market) ValidateOrder(f OrderFields) error {
	if err := m.Validate(); err != nil {
		return err
	}
	if err := f.Validate(); err != nil {
		return err
	}
	if f.Symbol != m.Symbol || (*m.IsolatedOnly && !f.IsIsolated) {
		return fmt.Errorf("failed to validate bluefin order: market=invalid")
	}
	for _, rangeCheck := range []struct{ value, minimum, maximum, step string }{
		{f.PriceE9, m.MinimumPriceE9, m.MaximumPriceE9, m.TickSizeE9},
		{f.QuantityE9, m.MinimumQuantityE9, m.MaximumQuantityE9, m.StepSizeE9},
	} {
		value, err := integer(rangeCheck.value, false)
		if err != nil {
			return err
		}
		minimum, err := integer(rangeCheck.minimum, false)
		if err != nil {
			return err
		}
		maximum, err := integer(rangeCheck.maximum, false)
		if err != nil {
			return err
		}
		step, err := integer(rangeCheck.step, false)
		if err != nil {
			return err
		}
		if value < minimum || value > maximum || value%step != 0 {
			return fmt.Errorf("failed to validate bluefin order: price_or_quantity=out_of_range")
		}
	}
	return nil
}

func address(value string) error {
	a, err := sui.ParseAddress(value)
	if err != nil || a.IsZero() || a.String() != value {
		return fmt.Errorf("failed to validate bluefin address: address=invalid")
	}
	return nil
}

func integer(value string, allowZero bool) (uint64, error) {
	if value == "" || len(value) > 20 || (len(value) > 1 && value[0] == '0') {
		return 0, fmt.Errorf("failed to parse bluefin integer: value=invalid")
	}
	for _, c := range value {
		if c < '0' || c > '9' {
			return 0, fmt.Errorf("failed to parse bluefin integer: value=invalid")
		}
	}
	n, err := strconv.ParseUint(value, 10, 64)
	if err != nil || (!allowZero && n == 0) {
		return 0, fmt.Errorf("failed to parse bluefin integer: value=out_of_range")
	}
	return n, nil
}

// DecimalToE9 converts unsigned decimal notation to a canonical uint64 E9 string without rounding.
//
// Version:
//   - 2026-10-14: Added.
func DecimalToE9(value string) (string, error) {
	if value == "" || len(value) > 40 {
		return "", fmt.Errorf("failed to convert bluefin decimal: value=invalid")
	}
	parts := strings.Split(value, ".")
	if len(parts) > 2 || parts[0] == "" {
		return "", fmt.Errorf("failed to convert bluefin decimal: value=invalid")
	}
	fraction := ""
	if len(parts) == 2 {
		fraction = parts[1]
		if fraction == "" || len(fraction) > 9 {
			return "", fmt.Errorf("failed to convert bluefin decimal: precision=invalid")
		}
	}
	for _, c := range parts[0] + fraction {
		if c < '0' || c > '9' {
			return "", fmt.Errorf("failed to convert bluefin decimal: value=invalid")
		}
	}
	n, ok := new(big.Int).SetString(parts[0]+fraction+strings.Repeat("0", 9-len(fraction)), 10)
	if !ok || !n.IsUint64() {
		return "", fmt.Errorf("failed to convert bluefin decimal: value=out_of_range")
	}
	return n.String(), nil
}

// FormatE9 converts a bounded signed E9 integer to exact decimal notation.
//
// Version:
//   - 2026-10-14: Added.
func FormatE9(value string) (string, error) {
	negative := strings.HasPrefix(value, "-")
	unsigned := strings.TrimPrefix(value, "-")
	if _, err := integer(unsigned, true); err != nil {
		return "", fmt.Errorf("failed to format bluefin decimal: %w", err)
	}
	unsigned = strings.Repeat("0", max(0, 10-len(unsigned))) + unsigned
	whole, fraction := unsigned[:len(unsigned)-9], strings.TrimRight(unsigned[len(unsigned)-9:], "0")
	result := whole
	if fraction != "" {
		result += "." + fraction
	}
	if negative && result != "0" {
		result = "-" + result
	}
	return result, nil
}
