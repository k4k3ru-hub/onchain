package query

import (
	"fmt"
	"net/url"
	"strconv"
)

type MetadataParams struct {
	MarketID *int64
	Filter   string
}
type LimitParams struct {
	MarketID int64
	Limit    int64
	Maximum  int64
}
type FundingParams struct {
	MarketID       int64
	Resolution     string
	StartTimestamp int64
	EndTimestamp   int64
	CountBack      int64
}
type EmptyParams struct{}

// MarketID validates the documented nonnegative int16 market identifier; zero is valid.
//
// Version:
//   - 2026-09-06: Added.
func MarketID(id int64) error {
	if id < 0 || id > 32767 {
		return fmt.Errorf("failed to validate market identifier: market_id=out_of_range min_value=0 max_value=32767")
	}
	return nil
}

// Values builds metadata filters, preserving an explicitly selected market zero.
//
// Version:
//   - 2026-09-06: Added.
func (p MetadataParams) Values() (url.Values, error) {
	q := make(url.Values)
	if p.MarketID != nil {
		if err := MarketID(*p.MarketID); err != nil {
			return nil, err
		}
		q.Set("market_id", strconv.FormatInt(*p.MarketID, 10))
	}
	switch p.Filter {
	case "":
	case "all", "spot", "perp":
		q.Set("filter", p.Filter)
	default:
		return nil, fmt.Errorf("failed to validate metadata parameters: filter=invalid")
	}
	return q, nil
}

// Values builds a bounded market query.
//
// Version:
//   - 2026-09-06: Added.
func (p LimitParams) Values() (url.Values, error) {
	if err := MarketID(p.MarketID); err != nil {
		return nil, err
	}
	if p.Limit < 1 || p.Limit > p.Maximum {
		return nil, fmt.Errorf("failed to validate market query: limit=out_of_range min_value=1 max_value=%d", p.Maximum)
	}
	return url.Values{"market_id": {strconv.FormatInt(p.MarketID, 10)}, "limit": {strconv.FormatInt(p.Limit, 10)}}, nil
}

// Values builds a funding history query without changing timestamp units.
//
// Version:
//   - 2026-09-06: Added.
func (p FundingParams) Values() (url.Values, error) {
	if err := MarketID(p.MarketID); err != nil {
		return nil, err
	}
	if p.Resolution != "1h" && p.Resolution != "1d" {
		return nil, fmt.Errorf("failed to validate funding parameters: resolution=invalid")
	}
	if p.StartTimestamp < 0 || p.EndTimestamp < p.StartTimestamp || p.EndTimestamp > 5000000000000 {
		return nil, fmt.Errorf("failed to validate funding parameters: time_range=out_of_range")
	}
	if p.CountBack < 0 {
		return nil, fmt.Errorf("failed to validate funding parameters: count_back=out_of_range")
	}
	return url.Values{"market_id": {strconv.FormatInt(p.MarketID, 10)}, "resolution": {p.Resolution}, "start_timestamp": {strconv.FormatInt(p.StartTimestamp, 10)}, "end_timestamp": {strconv.FormatInt(p.EndTimestamp, 10)}, "count_back": {strconv.FormatInt(p.CountBack, 10)}}, nil
}

// Values returns an empty query for an unfiltered operation.
//
// Version:
//   - 2026-09-06: Added.
func (EmptyParams) Values() (url.Values, error) { return make(url.Values), nil }
