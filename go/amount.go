//
// amount.go
//
package onchain

import (
    "fmt"
    "strings"
)

type DecimalAmount string
type BaseUnitAmount string

//
// Parse decimal amount to base units.
//
// Version:
//   - 2026-05-17: Added.
//
func (a DecimalAmount) ParseToBaseUnits(decimals uint8) (BaseUnitAmount, error) {
    if decimals > 77 {
        return "", fmt.Errorf("invalid parameter: decimals=%d", decimals)
    }

    s := strings.TrimSpace(string(a))
    if s == "" {
        return "", fmt.Errorf("missing required parameter: amount=%q", "empty")
    }
    if s != string(a) {
        return "", fmt.Errorf("invalid parameter: amount contains whitespace")
    }
    if strings.HasPrefix(s, "+") || strings.HasPrefix(s, "-") {
        return "", fmt.Errorf("invalid parameter: amount=%q", s)
    }
    if strings.ContainsAny(s, "eE") {
        return "", fmt.Errorf("invalid parameter: amount must not use exponent notation")
    }

    parts := strings.Split(s, ".")
    if len(parts) > 2 {
        return "", fmt.Errorf("invalid parameter: amount=%q", s)
    }

    intPart := parts[0]
    fracPart := ""

    if len(parts) == 2 {
        fracPart = parts[1]
        if fracPart == "" {
            return "", fmt.Errorf("invalid parameter: amount=%q", s)
        }
    }

    if intPart == "" {
        return "", fmt.Errorf("invalid parameter: amount=%q", s)
    }

    if len(fracPart) > int(decimals) {
        return "", fmt.Errorf("invalid parameter: amount decimals exceed asset decimals")
    }

    for _, ch := range intPart + fracPart {
        if ch < '0' || ch > '9' {
            return "", fmt.Errorf("invalid parameter: amount=%q", s)
        }
    }

    fracPart += strings.Repeat("0", int(decimals)-len(fracPart))

    base := strings.TrimLeft(intPart+fracPart, "0")
    if base == "" {
        return "", fmt.Errorf("invalid parameter: amount must be greater than zero")
    }
    if len(base) > 78 {
        return "", fmt.Errorf("invalid parameter: max_length=78 amount=%q", "too long")
    }

    return BaseUnitAmount(base), nil
}

//
// Format base units to decimal amount.
//
// Version:
//   - 2026-05-17: Added.
//
func (a BaseUnitAmount) FormatToDecimal(decimals uint8) (DecimalAmount, error) {
    if decimals > 77 {
		return "", fmt.Errorf("invalid parameter: decimals=%d", decimals)
	}

	s := strings.TrimSpace(string(a))
	if s == "" {
		return "", fmt.Errorf("missing required parameter: amount=%q", "empty")
	}
	if s != string(a) {
		return "", fmt.Errorf("invalid parameter: amount contains whitespace")
	}
	if strings.HasPrefix(s, "+") || strings.HasPrefix(s, "-") {
		return "", fmt.Errorf("invalid parameter: amount=%q", s)
	}

	for _, ch := range s {
		if ch < '0' || ch > '9' {
			return "", fmt.Errorf("invalid parameter: amount=%q", s)
		}
	}

	s = strings.TrimLeft(s, "0")
	if s == "" {
		return "", fmt.Errorf("invalid parameter: amount must be greater than zero")
	}

	if len(s) > 78 {
		return "", fmt.Errorf("invalid parameter: max_length=78 amount=%q", "too long")
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


