// Package query validates public Arcus request parameters.
package query

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

// OptionalMarket builds an optional market filter without normalizing it.
//
// Version:
//   - 2026-09-06: Added.
func OptionalMarket(m string) (url.Values, error) {
	if m == "" {
		return make(url.Values), nil
	}
	return RequiredMarket(m)
}

// RequiredMarket validates a market name or ID safe for one URL segment.
//
// Version:
//   - 2026-09-06: Added.
func RequiredMarket(m string) (url.Values, error) {
	if m == "" {
		return nil, fmt.Errorf("failed to validate market: market=empty")
	}
	if len(m) > 128 {
		return nil, fmt.Errorf("failed to validate market: market=too_long max_length=128")
	}
	for _, r := range m {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_') {
			return nil, fmt.Errorf("failed to validate market: market=invalid")
		}
	}
	return url.Values{"market": {m}}, nil
}

// Book builds a book request; zero options select server defaults.
//
// Version:
//   - 2026-09-06: Added.
func Book(m string, levels, sigFigs, roundStep int64) (url.Values, error) {
	q, err := RequiredMarket(m)
	if err != nil {
		return nil, err
	}
	if levels < 0 || levels > 100 {
		return nil, fmt.Errorf("failed to validate book: n_levels=out_of_range max_value=100")
	}
	if sigFigs != 0 && (sigFigs < 2 || sigFigs > 5) {
		return nil, fmt.Errorf("failed to validate book: sig_figs=invalid")
	}
	if roundStep != 0 && roundStep != 1 && roundStep != 2 && roundStep != 5 {
		return nil, fmt.Errorf("failed to validate book: round_step=invalid")
	}
	for k, v := range map[string]int64{"nLevels": levels, "sigFigs": sigFigs, "roundStep": roundStep} {
		if v != 0 {
			q.Set(k, strconv.FormatInt(v, 10))
		}
	}
	return q, nil
}
func bounds(q url.Values, from, to *int64) error {
	for k, v := range map[string]*int64{"from": from, "to": to} {
		if v != nil {
			if *v < 100000000000000 {
				return fmt.Errorf("failed to validate time window: timestamp=out_of_range min_value=100000000000000")
			}
			q.Set(k, strconv.FormatInt(*v, 10))
		}
	}
	if from != nil && to != nil && *from > *to {
		return fmt.Errorf("failed to validate time window: range=invalid")
	}
	return nil
}

// History builds a history query using epoch microseconds and an optional limit.
//
// Version:
//   - 2026-09-06: Added.
func History(m string, limit int64, from, to *int64) (url.Values, error) {
	q, err := RequiredMarket(m)
	if err != nil {
		return nil, err
	}
	if limit < 0 || limit > 1000 {
		return nil, fmt.Errorf("failed to validate history: limit=out_of_range max_value=1000")
	}
	if limit != 0 {
		q.Set("limit", strconv.FormatInt(limit, 10))
	}
	if err := bounds(q, from, to); err != nil {
		return nil, err
	}
	return q, nil
}

// Candles builds a candle query with epoch microseconds and exclusive pagination options.
//
// Version:
//   - 2026-09-06: Added.
func Candles(m, timeframe string, to int64, from *int64, countBack int64) (url.Values, error) {
	q, err := RequiredMarket(m)
	if err != nil {
		return nil, err
	}
	valid := false
	for _, f := range strings.Fields("1m 3m 5m 15m 30m 1h 2h 4h 8h 12h 1d 3d 1w") {
		if f == timeframe {
			valid = true
			break
		}
	}
	if !valid {
		return nil, fmt.Errorf("failed to validate candles: timeframe=invalid")
	}
	if countBack < 0 || countBack > 1500 {
		return nil, fmt.Errorf("failed to validate candles: count_back=out_of_range max_value=1500")
	}
	if from != nil && countBack != 0 {
		return nil, fmt.Errorf("failed to validate candles: from_and_count_back=invalid")
	}
	if err := bounds(q, from, &to); err != nil {
		return nil, err
	}
	q.Set("timeframe", timeframe)
	if countBack != 0 {
		q.Set("countback", strconv.FormatInt(countBack, 10))
	}
	return q, nil
}
