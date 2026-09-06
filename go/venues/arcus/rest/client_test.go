package rest_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/k4k3ru-hub/onchain/go/venues/arcus/rest"
	"github.com/k4k3ru-hub/onchain/go/venues/arcus/rest/markets/bbo"
	"github.com/k4k3ru-hub/onchain/go/venues/arcus/rest/markets/candles"
	"github.com/k4k3ru-hub/onchain/go/venues/arcus/rest/markets/funding_rates"
	"github.com/k4k3ru-hub/onchain/go/venues/arcus/rest/markets/list"
	"github.com/k4k3ru-hub/onchain/go/venues/arcus/rest/markets/order_book"
	"github.com/k4k3ru-hub/onchain/go/venues/arcus/rest/markets/trades"
)

type httpFunc func(*http.Request) (*http.Response, error)

func (f httpFunc) Do(r *http.Request) (*http.Response, error) { return f(r) }
func response(body string) *http.Response {
	return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body))}
}

func TestCompositionAndWireContracts(t *testing.T) {
	ctx := context.Background()
	cases := []struct {
		path, query, body string
		call              func(*rest.Client) error
	}{
		{"markets", "market=0", `{"markets":[{"marketId":0,"marketDisplayName":"BTC-USD","tickSize":"0.000000000000000001","isOutsideRth":null}]}`, func(c *rest.Client) error {
			r, e := c.Markets.List.Send(ctx, list.Params{Market: "0"})
			if e == nil && (len(r.Markets) != 1 || r.Markets[0].TickSize != "0.000000000000000001" || r.Markets[0].IsOutsideRTH != nil) {
				t.Fatal(r)
			}
			return e
		}},
		{"l2OrderBook/BTC-USD", "nLevels=100&roundStep=2&sigFigs=5", `{"bids":[["1.000000000000000001","2"]],"asks":[],"lastSequenceId":18446744073709551615}`, func(c *rest.Client) error {
			r, e := c.Markets.OrderBook.Send(ctx, order_book.Params{Market: "BTC-USD", NLevels: 100, SigFigs: 5, RoundStep: 2})
			if e == nil && (r.LastSequenceID != 18446744073709551615 || r.Bids[0][0] != "1.000000000000000001") {
				t.Fatal(r)
			}
			return e
		}},
		{"bbo/BTC-USD", "", `{"bestBid":null,"bestAsk":{"price":"2","size":"1"},"lastSequenceId":9007199254740993}`, func(c *rest.Client) error {
			r, e := c.Markets.BBO.Send(ctx, bbo.Params{Market: "BTC-USD"})
			if e == nil && (r.BestBid != nil || r.BestAsk.Price != "2" || r.LastSequenceID != 9007199254740993) {
				t.Fatal(r)
			}
			return e
		}},
		{"trades", "limit=10&market=BTC-USD", `{"trades":[{"tradeId":"9007199254740993","timestamp":1788656400000000,"size":"0.001","sequenceNumber":18446744073709551615}]}`, func(c *rest.Client) error {
			r, e := c.Markets.Trades.Send(ctx, trades.Params{Market: "BTC-USD", Limit: 10})
			if e == nil && (r.Trades[0].SequenceNumber != 18446744073709551615 || r.Trades[0].Timestamp != 1788656400000000) {
				t.Fatal(r)
			}
			return e
		}},
		{"candles", "countback=5&market=BTC-USD&timeframe=1h&to=1788656400000000", `{"candles":[{"openTime":1788652800000000,"open":"1.234567890123456789","isFinal":true}]}`, func(c *rest.Client) error {
			r, e := c.Markets.Candles.Send(ctx, candles.Params{Market: "BTC-USD", Timeframe: "1h", To: 1788656400000000, CountBack: 5})
			if e == nil && (!r.Candles[0].IsFinal || r.Candles[0].Open != "1.234567890123456789") {
				t.Fatal(r)
			}
			return e
		}},
		{"fundingRates", "market=BTC-USD", `{"fundingRates":[{"marketId":1,"fundingRate":"-0.0000123456789012345","time":1788652800000000}]}`, func(c *rest.Client) error {
			r, e := c.Markets.FundingRates.Send(ctx, funding_rates.Params{Market: "BTC-USD"})
			if e == nil && r.FundingRates[0].FundingRate != "-0.0000123456789012345" {
				t.Fatal(r)
			}
			return e
		}},
	}
	for _, tt := range cases {
		t.Run(tt.path, func(t *testing.T) {
			count := 0
			c, e := rest.NewClient(rest.ClientParams{BaseURL: "https://example.com/prefix/", HTTPClient: httpFunc(func(r *http.Request) (*http.Response, error) {
				count++
				if r.Method != "GET" || r.URL.Path != "/prefix/v1/"+tt.path || r.URL.RawQuery != tt.query {
					t.Fatalf("unexpected request %s", r.URL)
				}
				if _, ok := r.Context().Deadline(); !ok {
					t.Fatal("unbounded request")
				}
				return response(tt.body), nil
			})})
			if e != nil {
				t.Fatal(e)
			}
			if c.Markets == nil || c.Markets.List == nil || c.Markets.OrderBook == nil || c.Markets.BBO == nil || c.Markets.Trades == nil || c.Markets.Candles == nil || c.Markets.FundingRates == nil {
				t.Fatal("incomplete composition")
			}
			if e := tt.call(c); e != nil {
				t.Fatal(e)
			}
			if count != 1 {
				t.Fatal(count)
			}
		})
	}
}

func TestResponseFailures(t *testing.T) {
	for _, tt := range []struct {
		name, body string
		status     int
		limit      int64
		wantCode   int
		want       string
	}{

		{"http status", `private-payload`, 429, 0, 429, "status_code=429"},

		{"null", `null`, 200, 0, 0, "body=null"},
		{"invalid json", `{"code":200,"private-payload":`, 200, 0, 0, "failed to decode"},
		{"oversized", `{"code":200}`, 200, 3, 0, "body=too_long"},
		{"wrong type", `{"code":200,"markets":"private-payload"}`, 200, 0, 0, "failed to decode"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			c, err := rest.NewClient(rest.ClientParams{MaxResponseBytes: tt.limit, HTTPClient: httpFunc(func(*http.Request) (*http.Response, error) {
				r := response(tt.body)
				r.StatusCode = tt.status
				return r, nil
			})})
			if err != nil {
				t.Fatal(err)
			}
			_, err = c.Markets.List.Send(context.Background(), list.Params{})
			if err == nil || !strings.Contains(err.Error(), tt.want) || strings.Contains(err.Error(), "private-payload") {
				t.Fatal(err)
			}
			if tt.wantCode != 0 {
				var re *rest.ResponseError
				if !errors.As(err, &re) || re.StatusCode != tt.wantCode {
					t.Fatal(err)
				}
			}
		})
	}
}

type brokenBody struct{ readErr, closeErr error }

func (b *brokenBody) Read([]byte) (int, error) { return 0, b.readErr }
func (b *brokenBody) Close() error             { return b.closeErr }
func TestErrorsRemainInspectable(t *testing.T) {
	root := errors.New("private-payload")
	for _, tt := range []struct {
		name string
		http httpFunc
	}{
		{"transport", func(*http.Request) (*http.Response, error) {
			return nil, &url.Error{Op: "Get", URL: "https://private-payload", Err: root}
		}},
		{"body", func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: 200, Body: &brokenBody{readErr: root, closeErr: root}}, nil
		}},
		{"close", func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: 200, Body: &closeBody{Reader: strings.NewReader(`{"code":200}`), err: root}}, nil
		}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			c, e := rest.NewClient(rest.ClientParams{HTTPClient: tt.http})
			if e != nil {
				t.Fatal(e)
			}
			_, e = c.Markets.List.Send(context.Background(), list.Params{})
			if !errors.Is(e, root) || strings.Contains(e.Error(), "private-payload") {
				t.Fatal(e)
			}
		})
	}
}

type closeBody struct {
	io.Reader
	err error
}

func (b *closeBody) Close() error { return b.err }

func TestCancellationAndValidation(t *testing.T) {
	c, err := rest.NewClient(rest.ClientParams{Timeout: time.Millisecond, HTTPClient: httpFunc(func(r *http.Request) (*http.Response, error) { <-r.Context().Done(); return nil, r.Context().Err() })})
	if err != nil {
		t.Fatal(err)
	}
	_, err = c.Markets.List.Send(context.Background(), list.Params{})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal(err)
	}
	for _, raw := range []string{"https://user:secret@example.com", "https://example.com?token=secret", "file:///tmp/data", ":bad", "https://example.com/#secret"} {
		if _, err := rest.NewClient(rest.ClientParams{BaseURL: raw}); err == nil || strings.Contains(err.Error(), "secret") {
			t.Fatal(raw, err)
		}
	}
	var typedNil httpFunc
	if _, err := rest.NewClient(rest.ClientParams{HTTPClient: typedNil}); err == nil {
		t.Fatal("typed nil accepted")
	}
	if _, err := rest.NewClient(rest.ClientParams{Timeout: -1}); err == nil {
		t.Fatal("negative timeout accepted")
	}
}

func TestSyntaxErrorInspectable(t *testing.T) {
	c, e := rest.NewClient(rest.ClientParams{HTTPClient: httpFunc(func(*http.Request) (*http.Response, error) { return response("{"), nil })})
	if e != nil {
		t.Fatal(e)
	}
	_, e = c.Markets.List.Send(context.Background(), list.Params{})
	var syntax *json.SyntaxError
	if !errors.As(e, &syntax) {
		t.Fatal(e)
	}
}
