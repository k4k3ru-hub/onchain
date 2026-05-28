//
// amount.go
//
package core

import (
    "fmt"
    "math/big"
    "strings"
)


const (
    AmountMaxAssetDecimals      = 77
    AmountMaxInputLength        = 128
    AmountMaxBaseUnitDigits     = 78
)


type DecimalAmount string
type BaseUnitAmount string


//
// Parse decimal amount to base unit interger.
//
// Version:
//   - 2026-05-28: Added.
//
func (a DecimalAmount) ToBaseUnitInt(decimals uint8) (*big.Int, error) {
	if decimals > AmountMaxAssetDecimals {
		return nil, fmt.Errorf("failed to parse decimal amount to base unit interger: invalid parameter: decimals=%d max_decimals=%d", decimals, AmountMaxAssetDecimals)
	}

	intPart, fracPart, err := parseDecimalAmountParts(string(a), decimals)
	if err != nil {
		return nil, fmt.Errorf("failed to parse decimal amount to base unit interger: %w", err)
	}

	fracPart += strings.Repeat("0", int(decimals)-len(fracPart))

	s := strings.TrimLeft(intPart+fracPart, "0")
	if s == "" {
		return nil, fmt.Errorf("failed to parse decimal amount to base unit interger: invalid parameter: amount must be greater than zero")
	}
	if len(s) > AmountMaxBaseUnitDigits {
		return nil, fmt.Errorf("failed to parse decimal amount to base unit interger: invalid parameter: max_digits=%d base_unit_amount=%q", AmountMaxBaseUnitDigits, "too long")
	}

	n, ok := new(big.Int).SetString(s, 10)
	if !ok || n.Sign() <= 0 {
		return nil, fmt.Errorf("failed to parse decimal amount to base unit interger: invalid parameter: decimal_amount=%q", string(a))
	}

	return n, nil
}


//
// Parse decimal amount to base unit amount.
//
// Version:
//   - 2026-05-28: Added.
//
func (a DecimalAmount) ToBaseUnits(decimals uint8) (BaseUnitAmount, error) {
	n, err := a.ToBaseUnitInt(decimals)
	if err != nil {
		return "", fmt.Errorf("failed to parse decimal amount to base unit amount: %w", err)
	}

	return BaseUnitAmount(n.String()), nil
}


//
// Parse base unit amount to integer.
//
// Version:
//   - 2026-05-28: Added.
//
func (a BaseUnitAmount) Int() (*big.Int, error) {
	s, err := normalizeBaseUnitAmountString(string(a))
	if err != nil {
		return nil, fmt.Errorf("failed to parse base unit amount to integer: %w", err)
	}

	n, ok := new(big.Int).SetString(s, 10)
	if !ok || n.Sign() <= 0 {
		return nil, fmt.Errorf("failed to parse base unit amount to integer: invalid parameter: base_unit_amount=%q", s)
	}

	return n, nil
}


//
// Format base unit amount to decimal amount.
//
// Version:
//   - 2026-05-28: Added.
//
func (a BaseUnitAmount) ToDecimal(decimals uint8) (DecimalAmount, error) {
	n, err := a.Int()
	if err != nil {
		return "", fmt.Errorf("failed to format base unit amount to decimal amount: %w", err)
	}

	return FormatBaseUnitInt(n, decimals)
}


//
// Format base unit integer to decimal amount.
//
// Version:
//   - 2026-05-28: Added.
//
func FormatBaseUnitInt(amount *big.Int, decimals uint8) (DecimalAmount, error) {
	if decimals > AmountMaxAssetDecimals {
		return "", fmt.Errorf("invalid parameter: decimals=%d", decimals)
	}
	if amount == nil {
		return "", fmt.Errorf("missing required parameter: base_unit_amount=null")
	}
	if amount.Sign() <= 0 {
		return "", fmt.Errorf("invalid parameter: amount must be greater than zero")
	}

	s := amount.String()
	if len(s) > AmountMaxBaseUnitDigits {
		return "", fmt.Errorf("invalid parameter: max_digits=%d base_unit_amount=%q", AmountMaxBaseUnitDigits, "too long")
	}

	d := int(decimals)
	if d == 0 {
		return DecimalAmount(s), nil
	}

	if len(s) <= d {
		s = strings.Repeat("0", d-len(s)+1) + s
	}

	intPart := s[:len(s)-d]
	fracPart := s[len(s)-d:]
	fracPart = strings.TrimRight(fracPart, "0")

	if fracPart == "" {
		return DecimalAmount(intPart), nil
	}

	return DecimalAmount(intPart + "." + fracPart), nil
}

func parseDecimalAmountParts(v string, decimals uint8) (string, string, error) {
	s := strings.TrimSpace(v)
	if s == "" {
		return "", "", fmt.Errorf("missing required parameter: decimal_amount=%q", "empty")
	}
	if len(s) > AmountMaxInputLength {
		return "", "", fmt.Errorf("invalid parameter: max_length=%d decimal_amount=%q", AmountMaxInputLength, "too long")
	}
	if strings.HasPrefix(s, "+") || strings.HasPrefix(s, "-") || strings.ContainsAny(s, "eE") {
		return "", "", fmt.Errorf("invalid parameter: decimal_amount=%q", s)
	}

	parts := strings.Split(s, ".")
	if len(parts) > 2 {
		return "", "", fmt.Errorf("invalid parameter: decimal_amount=%q", s)
	}

	intPart := parts[0]
	fracPart := ""

	if intPart == "" {
		return "", "", fmt.Errorf("invalid parameter: decimal_amount=%q", s)
	}

	if len(parts) == 2 {
		fracPart = parts[1]
		if fracPart == "" {
			return "", "", fmt.Errorf("invalid parameter: decimal_amount=%q", s)
		}
	}

	if len(fracPart) > int(decimals) {
		return "", "", fmt.Errorf("invalid parameter: amount decimals exceed asset decimals")
	}

	if !isAmountDigits(intPart + fracPart) {
		return "", "", fmt.Errorf("invalid parameter: decimal_amount=%q", s)
	}

	return intPart, fracPart, nil
}

func normalizeBaseUnitAmountString(v string) (string, error) {
	s := strings.TrimSpace(v)
	if s == "" {
		return "", fmt.Errorf("missing required parameter: base_unit_amount=%q", "empty")
	}
	if len(s) > AmountMaxInputLength {
		return "", fmt.Errorf("invalid parameter: max_length=%d base_unit_amount=%q", AmountMaxInputLength, "too long")
	}
	if strings.HasPrefix(s, "+") || strings.HasPrefix(s, "-") {
		return "", fmt.Errorf("invalid parameter: base_unit_amount=%q", s)
	}
	if !isAmountDigits(s) {
		return "", fmt.Errorf("invalid parameter: base_unit_amount=%q", s)
	}

	s = strings.TrimLeft(s, "0")
	if s == "" {
		return "", fmt.Errorf("invalid parameter: amount must be greater than zero")
	}
	if len(s) > AmountMaxBaseUnitDigits {
		return "", fmt.Errorf("invalid parameter: max_digits=%d base_unit_amount=%q", AmountMaxBaseUnitDigits, "too long")
	}

	return s, nil
}

func isAmountDigits(s string) bool {
	if s == "" {
		return false
	}

	for _, ch := range s {
		if ch < '0' || ch > '9' {
			return false
		}
	}

	return true
}




