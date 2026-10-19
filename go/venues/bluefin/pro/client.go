package pro

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Network   Network
	Transport http.RoundTripper
	Timeout   time.Duration
}

type Client struct {
	endpoints Endpoints
	http      *http.Client
	exchange  *ExchangeClient
	auth      *AuthClient
	account   *AccountClient
	trade     *TradeClient
}
type ExchangeClient struct{ client *Client }
type AuthClient struct{ client *Client }
type AccountClient struct{ client *Client }
type TradeClient struct{ client *Client }

// NewClient composes immutable REST API groups using official endpoints and an injectable transport.
// Redirects are disabled so login signatures and bearer tokens cannot be forwarded to another host.
//
// Version:
//   - 2026-10-14: Added.
func NewClient(config Config) (*Client, error) {
	endpoints, err := EndpointsFor(config.Network)
	if err != nil {
		return nil, fmt.Errorf("failed to create bluefin pro client: %w", err)
	}
	if config.Timeout < 0 {
		return nil, fmt.Errorf("failed to create bluefin pro client: timeout=out_of_range")
	}
	if config.Timeout == 0 {
		config.Timeout = 10 * time.Second
	}
	c := &Client{endpoints: endpoints, http: &http.Client{Transport: config.Transport, Timeout: config.Timeout, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}
	c.exchange = &ExchangeClient{c}
	c.auth = &AuthClient{c}
	c.account = &AccountClient{c}
	c.trade = &TradeClient{c}
	return c, nil
}

// Exchange returns the composed public exchange operations.
//
// Version:
//   - 2026-10-14: Added.
func (c *Client) Exchange() *ExchangeClient { return c.exchange }

// Auth returns the composed authentication operation.
//
// Version:
//   - 2026-10-14: Added.
func (c *Client) Auth() *AuthClient { return c.auth }

// Account returns the composed private account operations.
//
// Version:
//   - 2026-10-14: Added.
func (c *Client) Account() *AccountClient { return c.account }

// Trade returns the composed order operations.
//
// Version:
//   - 2026-10-14: Added.
func (c *Client) Trade() *TradeClient { return c.trade }

type HTTPError struct {
	StatusCode int
	RetryAfter time.Duration
}

// Error reports only response status and retry delay, excluding upstream payloads and credentials.
//
// Version:
//   - 2026-10-14: Added.
func (e *HTTPError) Error() string {
	return fmt.Sprintf("failed to call bluefin api: http_status=%d retry_after_ms=%d", e.StatusCode, e.RetryAfter.Milliseconds())
}

type requestError struct{ cause error }

// Error hides transport and parser payloads while preserving inspectable causes.
//
// Version:
//   - 2026-10-14: Added.
func (e *requestError) Error() string { return "failed to call bluefin api: upstream request failed" }

// Unwrap preserves cancellation and transport error inspection.
//
// Version:
//   - 2026-10-14: Added.
func (e *requestError) Unwrap() error { return e.cause }

func (c *Client) do(ctx context.Context, method, endpoint, path, token string, body any, headers http.Header, result any) (resultErr error) {
	if ctx == nil || c == nil {
		return fmt.Errorf("failed to call bluefin api: dependency=null")
	}
	if err := ctx.Err(); err != nil {
		return &requestError{err}
	}
	var encoded []byte
	if body != nil {
		var err error
		encoded, err = json.Marshal(body)
		if err != nil {
			return &requestError{err}
		}
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint+path, bytes.NewReader(encoded))
	if err != nil {
		return &requestError{err}
	}
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		if len(token) > 32<<10 || strings.ContainsAny(token, "\r\n") {
			return fmt.Errorf("failed to call bluefin api: token=invalid")
		}
		req.Header.Set("Authorization", "Bearer "+token)
	}
	for key, values := range headers {
		for _, value := range values {
			req.Header.Add(key, value)
		}
	}
	response, err := c.http.Do(req)
	if err != nil {
		return &requestError{err}
	}
	defer func() {
		if err := response.Body.Close(); err != nil {
			resultErr = errors.Join(resultErr, &requestError{err})
		}
	}()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		retry := time.Duration(0)
		if seconds, err := strconv.ParseInt(response.Header.Get("Retry-After"), 10, 64); err == nil && seconds > 0 && seconds <= 3600 {
			retry = time.Duration(seconds) * time.Second
		}
		return &HTTPError{StatusCode: response.StatusCode, RetryAfter: retry}
	}
	if result == nil {
		return nil
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, (4<<20)+1))
	if err != nil {
		return &requestError{err}
	}
	if len(data) > 4<<20 {
		return fmt.Errorf("failed to read bluefin response: body=too_long")
	}
	if err := json.Unmarshal(data, result); err != nil {
		return &requestError{err}
	}
	return nil
}

// Info reads the environment's current contracts, collateral and market constraints.
//
// Version:
//   - 2026-10-14: Added.
func (c *ExchangeClient) Info(ctx context.Context) (*ExchangeInfo, error) {
	var r ExchangeInfo
	err := c.client.do(ctx, http.MethodGet, c.client.endpoints.API, "/v1/exchange/info", "", nil, nil, &r)
	if err != nil {
		return nil, err
	}
	return &r, nil
}

type Depth struct {
	Symbol      string     `json:"symbol"`
	Bids        [][]string `json:"bidsE9"`
	Asks        [][]string `json:"asksE9"`
	UpdateID    uint64     `json:"lastUpdateId"`
	UpdatedAtMS int64      `json:"updatedAtMillis"`
}

// Depth reads a bounded snapshot, preserving zero upstream timestamps as unavailable.
//
// Version:
//   - 2026-10-14: Added.
func (c *ExchangeClient) Depth(ctx context.Context, symbol string, limit int) (*Depth, error) {
	if symbol != "SUI-PERP" || limit < 1 || limit > 1000 {
		return nil, fmt.Errorf("failed to read bluefin depth: parameters=invalid")
	}
	var r Depth
	err := c.client.do(ctx, http.MethodGet, c.client.endpoints.API, "/v1/exchange/depth?symbol="+url.QueryEscape(symbol)+"&limit="+strconv.Itoa(limit), "", nil, nil, &r)
	if err != nil {
		return nil, err
	}
	return &r, nil
}

type Ticker struct {
	Symbol                 string `json:"symbol"`
	MarkPriceE9            string `json:"markPriceE9"`
	OraclePriceE9          string `json:"oraclePriceE9"`
	EstimatedFundingRateE9 string `json:"estimatedFundingRateE9"`
	LastFundingRateE9      string `json:"lastFundingRateE9"`
	NextFundingTimeMS      int64  `json:"nextFundingTimeAtMillis"`
	OpenInterestE9         string `json:"openInterestE9"`
	UpdatedAtMS            int64  `json:"updatedAtMillis"`
}

// Ticker reads current SUI market observations without inferring unspecified units or periods.
//
// Version:
//   - 2026-10-14: Added.
func (c *ExchangeClient) Ticker(ctx context.Context) (*Ticker, error) {
	var r Ticker
	err := c.client.do(ctx, http.MethodGet, c.client.endpoints.API, "/v1/exchange/ticker?symbol=SUI-PERP", "", nil, nil, &r)
	if err != nil {
		return nil, err
	}
	return &r, nil
}

type LoginResult struct {
	AccessToken string        `json:"-"`
	ValidFor    time.Duration `json:"-"`
}

// Login exchanges a locally signed login for a short-lived token without retaining the refresh token.
// The caller must verify the expected signer and login age before invoking this operation.
//
// Version:
//   - 2026-10-14: Added.
func (c *AuthClient) Login(ctx context.Context, p LoginRequest, signature string) (*LoginResult, error) {
	if _, err := p.SigningBytes(); err != nil {
		return nil, err
	}
	if err := verifyLoginSignature(p, signature); err != nil {
		return nil, err
	}
	var r struct {
		AccessToken string `json:"accessToken"`
		ValidFor    int64  `json:"accessTokenValidForSeconds"`
	}
	err := c.client.do(ctx, http.MethodPost, c.client.endpoints.Auth, "/auth/v2/token?readOnly=false", "", p, http.Header{"payloadSignature": {signature}}, &r)
	if err != nil {
		return nil, err
	}
	if r.AccessToken == "" || len(r.AccessToken) > 32<<10 || strings.ContainsAny(r.AccessToken, "\r\n") || r.ValidFor <= 0 || r.ValidFor > 3600 {
		return nil, fmt.Errorf("failed to login bluefin account: response=invalid")
	}
	return &LoginResult{AccessToken: r.AccessToken, ValidFor: time.Duration(r.ValidFor) * time.Second}, nil
}

func verifyLoginSignature(p LoginRequest, signature string) error {
	message, err := p.SigningBytes()
	if err != nil {
		return err
	}
	return VerifySignature(message, signature, p.AccountAddress)
}

type TradingFees struct {
	MakerE9   string `json:"makerFeeE9"`
	TakerE9   string `json:"takerFeeE9"`
	IsApplied *bool  `json:"isApplied"`
}
type Balance struct {
	Symbol             string `json:"symbol"`
	QuantityE9         string `json:"quantityE9"`
	EffectiveBalanceE9 string `json:"effectiveBalanceE9"`
}
type Position struct {
	Symbol              string `json:"symbol"`
	Side                string `json:"side"`
	SizeE9              string `json:"sizeE9"`
	AverageEntryPriceE9 string `json:"avgEntryPriceE9"`
	LiquidationPriceE9  string `json:"liquidationPriceE9"`
	LeverageE9          string `json:"clientSetLeverageE9"`
	MarginRequiredE9    string `json:"marginRequiredE9"`
	UnrealizedPnLE9     string `json:"unrealizedPnlE9"`
	IsIsolated          *bool  `json:"isIsolated"`
	UpdatedAtMS         int64  `json:"updatedAtMillis"`
}
type Account struct {
	Address               string      `json:"accountAddress"`
	CanTrade              *bool       `json:"canTrade"`
	MarginAvailableE9     string      `json:"marginAvailableE9"`
	OrderMarginRequiredE9 string      `json:"totalOrderMarginRequiredE9"`
	AccountValueE9        string      `json:"totalAccountValueE9"`
	UpdatedAtMS           int64       `json:"updatedAtMillis"`
	Assets                []Balance   `json:"assets"`
	Positions             []Position  `json:"positions"`
	Fees                  TradingFees `json:"tradingFees"`
}

// Get reads the authenticated trading account without changing collateral or leverage.
//
// Version:
//   - 2026-10-14: Added.
func (c *AccountClient) Get(ctx context.Context, token string) (*Account, error) {
	if token == "" {
		return nil, fmt.Errorf("failed to read bluefin account: token=empty")
	}
	var r Account
	err := c.client.do(ctx, http.MethodGet, c.client.endpoints.API, "/api/v1/account", token, nil, nil, &r)
	if err != nil {
		return nil, err
	}
	return &r, nil
}

type Trade struct {
	ID              string `json:"id"`
	Symbol          string `json:"symbol"`
	OrderHash       string `json:"orderHash"`
	ClientOrderID   string `json:"clientOrderId"`
	PriceE9         string `json:"priceE9"`
	QuantityE9      string `json:"quantityE9"`
	QuoteQuantityE9 string `json:"quoteQuantityE9"`
	Side            string `json:"side"`
	IsMaker         *bool  `json:"isMaker"`
	TradingFeeE9    string `json:"tradingFeeE9"`
	TradingFeeAsset string `json:"tradingFeeAsset"`
	GasFeeE9        string `json:"gasFeeE9"`
	GasFeeAsset     string `json:"gasFeeAsset"`
	ExecutedAtMS    int64  `json:"executedAtMillis"`
}
type TradeQuery struct {
	StartAtMS, EndAtMS int64
	Page, Limit        int
}

// Trades reads bounded account trade history; a page alone does not prove complete order history.
//
// Version:
//   - 2026-10-14: Added.
func (c *AccountClient) Trades(ctx context.Context, token string, p TradeQuery) ([]Trade, error) {
	if token == "" || p.StartAtMS < 0 || p.EndAtMS <= p.StartAtMS || p.EndAtMS-p.StartAtMS > int64(7*24*time.Hour/time.Millisecond) || p.Page < 1 || p.Limit < 1 || p.Limit > 1000 {
		return nil, fmt.Errorf("failed to read bluefin trades: parameters=invalid")
	}
	query := url.Values{"symbol": {"SUI-PERP"}, "startTimeAtMillis": {strconv.FormatInt(p.StartAtMS, 10)}, "endTimeAtMillis": {strconv.FormatInt(p.EndAtMS, 10)}, "page": {strconv.Itoa(p.Page)}, "limit": {strconv.Itoa(p.Limit)}}
	var r []Trade
	err := c.client.do(ctx, http.MethodGet, c.client.endpoints.API, "/api/v1/account/trades?"+query.Encode(), token, nil, nil, &r)
	return r, err
}

type OrderRequest struct {
	Fields        OrderFields `json:"signedFields"`
	Signature     string      `json:"signature"`
	ClientOrderID string      `json:"clientOrderId"`
	Type          string      `json:"type"`
	ReduceOnly    bool        `json:"reduceOnly"`
	PostOnly      bool        `json:"postOnly"`
	TimeInForce   string      `json:"timeInForce"`
}
type OrderResponse struct {
	OrderHash string `json:"orderHash"`
}
type OpenOrder struct {
	Type             string `json:"type"`
	TimeInForce      string `json:"timeInForce"`
	ReduceOnly       *bool  `json:"reduceOnly"`
	Side             string `json:"side"`
	OrderHash        string `json:"orderHash"`
	ClientOrderID    string `json:"clientOrderId"`
	Symbol           string `json:"symbol"`
	AccountAddress   string `json:"accountAddress"`
	FilledQuantityE9 string `json:"filledQuantityE9"`
	Status           string `json:"status"`
	UpdatedAtMS      int64  `json:"updatedAtMillis"`
}

// CreateOrder sends one signed IOC limit order exactly once and returns only its acknowledgment.
// Transport failures have unknown outcomes; this SDK never retries an order.
//
// Version:
//   - 2026-10-14: Added.
func (c *TradeClient) CreateOrder(ctx context.Context, token string, p OrderRequest) (*OrderResponse, error) {
	message, err := p.Fields.SigningBytes()
	if err != nil {
		return nil, err
	}
	if token == "" || p.Type != "LIMIT" || p.TimeInForce != "IOC" || p.PostOnly || p.ClientOrderID == "" || len(p.ClientOrderID) > 128 {
		return nil, fmt.Errorf("failed to create bluefin order: parameters=invalid")
	}
	if err := VerifySignature(message, p.Signature, p.Fields.AccountAddress); err != nil {
		return nil, err
	}
	var r OrderResponse
	err = c.client.do(ctx, http.MethodPost, c.client.endpoints.Trade, "/api/v1/trade/orders", token, p, nil, &r)
	if err != nil {
		return nil, err
	}
	if r.OrderHash == "" {
		return nil, fmt.Errorf("failed to create bluefin order: order_hash=empty")
	}
	return &r, nil
}

// OpenOrders reads currently open SUI orders without treating absence as terminal evidence.
//
// Version:
//   - 2026-10-14: Added.
func (c *TradeClient) OpenOrders(ctx context.Context, token string) ([]OpenOrder, error) {
	if token == "" {
		return nil, fmt.Errorf("failed to read bluefin open orders: token=empty")
	}
	var r []OpenOrder
	err := c.client.do(ctx, http.MethodGet, c.client.endpoints.Trade, "/api/v1/trade/openOrders?symbol=SUI-PERP", token, nil, nil, &r)
	return r, err
}
