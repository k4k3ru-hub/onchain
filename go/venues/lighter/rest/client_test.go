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

	"github.com/k4k3ru-hub/onchain/go/venues/lighter/rest"
	"github.com/k4k3ru-hub/onchain/go/venues/lighter/rest/markets/funding_rates"
	"github.com/k4k3ru-hub/onchain/go/venues/lighter/rest/markets/fundings"
	"github.com/k4k3ru-hub/onchain/go/venues/lighter/rest/markets/order_book_details"
	"github.com/k4k3ru-hub/onchain/go/venues/lighter/rest/markets/order_book_orders"
	"github.com/k4k3ru-hub/onchain/go/venues/lighter/rest/markets/order_books"
	"github.com/k4k3ru-hub/onchain/go/venues/lighter/rest/markets/recent_trades"
)

type httpFunc func(*http.Request) (*http.Response, error)

func (f httpFunc) Do(r *http.Request) (*http.Response, error) { return f(r) }
func response(body string) *http.Response {
	return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body))}
}

func TestCompositionAndWireContracts(t *testing.T) {
	zero := int64(0)
	cases := []struct {
		path, query, body string
		call              func(*rest.Client) error
	}{
		{"orderBooks", "filter=spot&market_id=0", `{"code":200,"order_books":[{"market_id":0,"symbol":"ETH","min_base_amount":"0.000000000000000001"}]}`, func(c *rest.Client) error {
			r, e := c.Markets.OrderBooks.Send(context.Background(), order_books.Params{MarketID: &zero, Filter: "spot"})
			if e == nil && (len(r.OrderBooks) != 1 || r.OrderBooks[0].MinBaseAmount != "0.000000000000000001") {
				t.Fatal(r)
			}
			return e
		}},
		{"orderBookDetails", "filter=all", `{"code":200,"order_book_details":[{"market_id":0,"open_interest":123456789012345678.12345}],"spot_order_book_details":[{"market_id":2048,"symbol":"ETH/USDG"}]}`, func(c *rest.Client) error {
			r, e := c.Markets.OrderBookDetails.Send(context.Background(), order_book_details.Params{Filter: "all"})
			if e == nil && (r.OrderBookDetails[0].OpenInterest.String() != "123456789012345678.12345" || len(r.SpotOrderBookDetails) != 1) {
				t.Fatal(r)
			}
			return e
		}},
		{"orderBookOrders", "limit=250&market_id=0", `{"code":200,"total_asks":1,"asks":[{"order_index":9007199254740993,"price":"1.23","remaining_base_amount":"2"}],"total_bids":0,"bids":[]}`, func(c *rest.Client) error {
			r, e := c.Markets.OrderBookOrders.Send(context.Background(), order_book_orders.Params{MarketID: 0, Limit: 250})
			if e == nil && r.Asks[0].OrderIndex != 9007199254740993 {
				t.Fatal(r)
			}
			return e
		}},
		{"recentTrades", "limit=100&market_id=2048", `{"code":200,"trades":[{"trade_id":9007199254740993,"trade_id_str":"9007199254740993","market_id":2048,"size":"0.001","is_maker_ask":true}],"next_cursor":"next"}`, func(c *rest.Client) error {
			r, e := c.Markets.RecentTrades.Send(context.Background(), recent_trades.Params{MarketID: 2048, Limit: 100})
			if e == nil && (r.Trades[0].TradeID != 9007199254740993 || !r.Trades[0].IsMakerAsk || r.NextCursor != "next") {
				t.Fatal(r)
			}
			return e
		}},
		{"fundings", "count_back=0&end_timestamp=1788656400&market_id=0&resolution=1h&start_timestamp=1788652800", `{"code":200,"resolution":"1h","fundings":[{"timestamp":1788652800,"rate":"0.001","value":"0.01","direction":"long"}]}`, func(c *rest.Client) error {
			r, e := c.Markets.Fundings.Send(context.Background(), fundings.Params{MarketID: 0, Resolution: "1h", StartTimestamp: 1788652800, EndTimestamp: 1788656400})
			if e == nil && r.Fundings[0].Rate != "0.001" {
				t.Fatal(r)
			}
			return e
		}},
		{"funding-rates", "", `{"code":200,"funding_rates":[{"market_id":0,"exchange":"lighter","symbol":"ETH","rate":0.0000123456789012345}]}`, func(c *rest.Client) error {
			r, e := c.Markets.FundingRates.Send(context.Background(), funding_rates.Params{})
			if e == nil && r.FundingRates[0].Rate.String() != "0.0000123456789012345" {
				t.Fatal(r)
			}
			return e
		}},
	}
	for _, tt := range cases {
		t.Run(tt.path, func(t *testing.T) {
			count := 0
			c, err := rest.NewClient(rest.ClientParams{BaseURL: "https://example.com/prefix/", HTTPClient: httpFunc(func(r *http.Request) (*http.Response, error) {
				count++
				if r.Method != "GET" || r.URL.Path != "/prefix/api/v1/"+tt.path || r.URL.RawQuery != tt.query {
					t.Fatalf("unexpected request: %s %s", r.Method, r.URL)
				}
				if _, ok := r.Context().Deadline(); !ok {
					t.Fatal("missing timeout")
				}
				return response(tt.body), nil
			})})
			if err != nil {
				t.Fatal(err)
			}
			if c.Markets == nil || c.Markets.OrderBooks == nil || c.Markets.OrderBookDetails == nil || c.Markets.OrderBookOrders == nil || c.Markets.RecentTrades == nil || c.Markets.Fundings == nil || c.Markets.FundingRates == nil {
				t.Fatal("incomplete composition")
			}
			if err := tt.call(c); err != nil {
				t.Fatal(err)
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
		{"remote code", `{"code":400,"message":"private-payload"}`, 200, 0, 400, "code=400"},
		{"http status", `private-payload`, 429, 0, 0, "status_code=429"},
		{"missing code", `{}`, 200, 0, 0, "code=null"},
		{"null", `null`, 200, 0, 0, "code=null"},
		{"invalid json", `{"code":200,"private-payload":`, 200, 0, 0, "failed to decode"},
		{"oversized", `{"code":200}`, 200, 3, 0, "body=too_long"},
		{"wrong type", `{"code":200,"order_books":"private-payload"}`, 200, 0, 0, "failed to decode"},
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
			_, err = c.Markets.OrderBooks.Send(context.Background(), order_books.Params{})
			if err == nil || !strings.Contains(err.Error(), tt.want) || strings.Contains(err.Error(), "private-payload") {
				t.Fatal(err)
			}
			if tt.wantCode != 0 {
				var re *rest.ResponseError
				if !errors.As(err, &re) || re.Code != tt.wantCode {
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
			_, e = c.Markets.OrderBooks.Send(context.Background(), order_books.Params{})
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
	_, err = c.Markets.OrderBooks.Send(context.Background(), order_books.Params{})
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
	_, e = c.Markets.OrderBooks.Send(context.Background(), order_books.Params{})
	var syntax *json.SyntaxError
	if !errors.As(e, &syntax) {
		t.Fatal(e)
	}
}
