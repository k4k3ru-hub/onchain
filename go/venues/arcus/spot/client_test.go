package spot_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/k4k3ru-hub/onchain/go/venues/arcus/spot"
	"github.com/k4k3ru-hub/onchain/go/venues/arcus/spot/health"
	"github.com/k4k3ru-hub/onchain/go/venues/arcus/spot/price"
	"github.com/k4k3ru-hub/onchain/go/venues/arcus/spot/tokens"
)

type httpFunc func(*http.Request) (*http.Response, error)

func (f httpFunc) Do(r *http.Request) (*http.Response, error) { return f(r) }
func response(status int, body string) *http.Response {
	return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(body))}
}
func params() price.Params {
	return price.Params{ChainID: 4663, SellToken: "0x39dBED3a2bd333467115dE45665cC57F813C4571", BuyToken: "0x5fc5360D0400a0Fd4f2af552ADD042D716F1d168", SellAmount: "1000000000000000000"}
}

func TestCompositionAndWireContracts(t *testing.T) {
	for _, base := range []string{"https://example.com/prefix", "https://example.com/prefix/v1/"} {
		c, err := spot.NewClient(spot.ClientParams{BaseURL: base, HTTPClient: httpFunc(func(r *http.Request) (*http.Response, error) {
			if r.Method != "GET" {
				t.Fatal(r.Method)
			}
			if _, ok := r.Context().Deadline(); !ok {
				t.Fatal("missing timeout")
			}
			switch r.URL.Path {
			case "/prefix/v1/price":
				q := r.URL.Query()
				p := params()
				if q.Get("chainId") != "4663" || q.Get("sellToken") != p.SellToken || q.Get("buyToken") != p.BuyToken || q.Get("sellAmount") != p.SellAmount {
					t.Fatal(q)
				}
				return response(200, `{"recommended":"future-venue","all":[{"venue":"future-venue","sellAmount":"1000000000000000000","buyAmount":"99999999999999999999999","raw":{"amount":99999999999999999999999}}],"errors":[{"venue":"arcus","error":{"kind":"http_4xx","status":422,"message":"no quote","code":"NO_QUOTES"}}]}`), nil
			case "/prefix/v1/tokens":
				return response(200, `[{"chainId":4663,"symbol":"PONS","address":"0x39dBED3a2bd333467115dE45665cC57F813C4571","decimals":18,"verified":false}]`), nil
			case "/prefix/health":
				return response(200, `{"ok":true,"chainId":4663}`), nil
			default:
				t.Fatal(r.URL.Path)
				return nil, errors.New("unexpected path")
			}
		})})
		if err != nil {
			t.Fatal(err)
		}
		if c.Price == nil || c.Tokens == nil || c.Health == nil {
			t.Fatal("incomplete composition")
		}
		p, err := c.Price.Send(context.Background(), params())
		if err != nil {
			t.Fatal(err)
		}
		if p.All[0].BuyAmount != "99999999999999999999999" || p.Recommended != "future-venue" || p.Errors[0].Error.Code != "NO_QUOTES" {
			t.Fatal(p)
		}
		ts, err := c.Tokens.Send(context.Background(), tokens.Params{})
		if err != nil {
			t.Fatal(err)
		}
		if len(*ts) != 1 || (*ts)[0].Decimals != 18 || (*ts)[0].WrappedTokenAddress != nil {
			t.Fatal(ts)
		}
		h, err := c.Health.Send(context.Background(), health.Params{})
		if err != nil || !h.OK || h.ChainID != 4663 {
			t.Fatal(h, err)
		}
	}
}

func TestHTTPFailuresAndBounds(t *testing.T) {
	for _, tc := range []struct {
		status int
		body   string
		limit  int64
	}{
		{422, `{"code":"NO_QUOTES","private":"secret"}`, 1024}, {404, `missing`, 1024}, {429, `secret`, 1024}, {200, `null`, 1024}, {200, `{} {}`, 1024}, {200, `invalid`, 1024}, {200, `{"ok":true}`, 2},
	} {
		c, err := spot.NewClient(spot.ClientParams{MaxResponseBytes: tc.limit, HTTPClient: httpFunc(func(*http.Request) (*http.Response, error) { return response(tc.status, tc.body), nil })})
		if err != nil {
			t.Fatal(err)
		}
		_, err = c.Health.Send(context.Background(), health.Params{})
		if err == nil {
			t.Fatalf("accepted %v", tc)
		}
		if strings.Contains(err.Error(), "secret") {
			t.Fatal("remote body exposed")
		}
		if tc.status != 200 {
			var e *spot.ResponseError
			if !errors.As(err, &e) || e.StatusCode != tc.status {
				t.Fatal(err)
			}
			if tc.status == 422 && e.Code != "NO_QUOTES" {
				t.Fatal("missing response code")
			}
		}
	}
}

func TestTransportCancellationPreservesCause(t *testing.T) {
	c, err := spot.NewClient(spot.ClientParams{Timeout: time.Millisecond, HTTPClient: httpFunc(func(r *http.Request) (*http.Response, error) {
		<-r.Context().Done()
		return nil, &url.Error{Op: "Get", URL: "https://secret.invalid", Err: r.Context().Err()}
	})})
	if err != nil {
		t.Fatal(err)
	}
	_, err = c.Health.Send(context.Background(), health.Params{})
	if !errors.Is(err, context.DeadlineExceeded) || strings.Contains(err.Error(), "secret") {
		t.Fatal(err)
	}
}

type executor struct {
	called bool
	err    error
}

func (e *executor) Get(context.Context, string, url.Values, any) error { e.called = true; return e.err }
func TestPriceValidationAndErrorChain(t *testing.T) {
	for _, value := range []string{"", "0", "-1", "1.5", "1e18", strings.Repeat("9", 79), strings.Repeat("9", 78)} {
		e := &executor{}
		c, err := price.NewClient(e)
		if err != nil {
			t.Fatal(err)
		}
		p := params()
		p.SellAmount = value
		if _, err := c.Send(context.Background(), p); err == nil || e.called {
			t.Fatalf("accepted amount %q", value)
		}
	}
	cause := errors.New("unavailable")
	e := &executor{err: cause}
	c, err := price.NewClient(e)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.Send(context.Background(), params()); !errors.Is(err, cause) {
		t.Fatal(err)
	}
	var nilExecutor *executor
	if _, err := price.NewClient(nilExecutor); err == nil {
		t.Fatal("accepted nil executor")
	}
	if _, err := tokens.NewClient(nilExecutor); err == nil {
		t.Fatal("accepted nil executor")
	}
	if _, err := health.NewClient(nilExecutor); err == nil {
		t.Fatal("accepted nil executor")
	}
}
