// Package sourcify retrieves untrusted contract deployment metadata through API v2.
package sourcify

import (
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

	"github.com/ethereum/go-ethereum/common"
)

type HTTPClient interface {
	Do(*http.Request) (*http.Response, error)
}
type Client struct {
	http    HTTPClient
	baseURL string
}
type Deployment struct {
	ChainID         uint64
	Address         common.Address
	TransactionHash common.Hash
}
type HTTPError struct{ StatusCode int }

// Error returns the HTTP status without response payloads or credentials.
//
// Version:
//   - 2026-09-23: Added.
func (e *HTTPError) Error() string {
	return fmt.Sprintf("failed to fetch contract deployment: http_status=%d", e.StatusCode)
}

// NewClient composes deployment lookup with an injected HTTP transport.
//
// Version:
//   - 2026-09-23: Added.
func NewClient(client HTTPClient, baseURL string) (*Client, error) {
	if client == nil {
		return nil, fmt.Errorf("failed to create deployment client: http_client=null")
	}
	u, err := url.Parse(baseURL)
	if err != nil || u.Host == "" || u.Scheme != "https" && u.Scheme != "http" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return nil, fmt.Errorf("failed to create deployment client: base_url=invalid")
	}
	return &Client{http: client, baseURL: strings.TrimRight(baseURL, "/")}, nil
}

// Deployment fetches at most 64 KiB in one HTTP attempt under a 20-second timeout.
// A zero transaction hash means metadata did not contain a deployment candidate.
// Returned metadata requires independent onchain verification.
//
// Version:
//   - 2026-09-23: Added.
func (c *Client) Deployment(ctx context.Context, chainID uint64, address common.Address) (Deployment, error) {
	result := Deployment{ChainID: chainID, Address: address}
	if c == nil || c.http == nil || ctx == nil {
		return result, fmt.Errorf("failed to fetch contract deployment: dependency=null")
	}
	if chainID == 0 || address == (common.Address{}) {
		return result, fmt.Errorf("failed to fetch contract deployment: identity=empty")
	}
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	endpoint := c.baseURL + "/v2/contract/" + strconv.FormatUint(chainID, 10) + "/" + address.Hex() + "?fields=deployment"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return result, fmt.Errorf("failed to create deployment request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "K4K3RU-onchain")
	response, err := c.http.Do(req)
	if err != nil {
		return result, fmt.Errorf("failed to fetch contract deployment: %w", err)
	}
	if response == nil || response.Body == nil {
		return result, fmt.Errorf("failed to fetch contract deployment: response=null")
	}
	data, readErr := io.ReadAll(io.LimitReader(response.Body, (64<<10)+1))
	closeErr := response.Body.Close()
	if readErr != nil || closeErr != nil {
		return result, fmt.Errorf("failed to read contract deployment: %w", errors.Join(readErr, closeErr))
	}
	if response.StatusCode != http.StatusOK {
		return result, &HTTPError{StatusCode: response.StatusCode}
	}
	if len(data) > 64<<10 {
		return result, fmt.Errorf("failed to read contract deployment: response=too_long")
	}
	var decoded struct {
		ChainID    string `json:"chainId"`
		Address    string `json:"address"`
		Deployment *struct {
			TransactionHash string `json:"transactionHash"`
		} `json:"deployment"`
	}
	if err := json.Unmarshal(data, &decoded); err != nil {
		return result, fmt.Errorf("failed to decode contract deployment: %w", err)
	}
	if decoded.ChainID != strconv.FormatUint(chainID, 10) || !common.IsHexAddress(decoded.Address) || common.HexToAddress(decoded.Address) != address {
		return result, fmt.Errorf("failed to verify contract deployment: identity=invalid")
	}
	if decoded.Deployment == nil || decoded.Deployment.TransactionHash == "" {
		return result, nil
	}
	h := decoded.Deployment.TransactionHash
	if len(h) != 66 || !strings.EqualFold(common.HexToHash(h).Hex(), h) || common.HexToHash(h) == (common.Hash{}) {
		return result, fmt.Errorf("failed to verify contract deployment: transaction_hash=invalid")
	}
	result.TransactionHash = common.HexToHash(h)
	return result, nil
}
