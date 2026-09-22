// Package sourcify retrieves public compilation inputs through Sourcify v2.
// It does not use external tax verdicts or submit source verification requests.
package sourcify

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/k4k3ru-hub/onchain/go/evm/erc20/tax"
)

type HTTPClient interface {
	Do(*http.Request) (*http.Response, error)
}

type Client struct {
	http    HTTPClient
	baseURL string
}
type HTTPError struct{ StatusCode int }

// Error formats an HTTP failure without exposing response bodies or credentials.
//
// Version:
//   - 2026-09-22: Added.
func (e *HTTPError) Error() string {
	return fmt.Sprintf("failed to fetch token source: http_status=%d", e.StatusCode)
}

// NewClient composes a public-source client, normally using https://sourcify.dev/server.
// The supplied HTTP client owns transport policy, including permitted redirects.
//
// Version:
//   - 2026-09-22: Added.
func NewClient(client HTTPClient, baseURL string) (*Client, error) {
	if client == nil {
		return nil, fmt.Errorf("failed to create sourcify client: http_client=null")
	}
	parsed, err := url.Parse(baseURL)
	if err != nil {
		return nil, fmt.Errorf("failed to create sourcify client: base_url=invalid")
	}
	if (parsed.Scheme != "https" && parsed.Scheme != "http") || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return nil, fmt.Errorf("failed to create sourcify client: base_url=invalid")
	}
	return &Client{http: client, baseURL: strings.TrimRight(baseURL, "/")}, nil
}

// Source fetches one bounded v2 compilation input without retries or caching.
// Callers share requests and apply the agreed initial attempt plus three retries,
// including HTTP 4xx failures. Returned inputs still require local verification.
//
// Version:
//   - 2026-09-22: Added.
func (c *Client) Source(ctx context.Context, request tax.SourceRequest) (tax.SourceBundle, error) {
	if c == nil || c.http == nil {
		return tax.SourceBundle{}, fmt.Errorf("failed to fetch token source: client=null")
	}
	if request.ChainID == 0 || request.Token == (common.Address{}) {
		return tax.SourceBundle{}, fmt.Errorf("failed to fetch token source: identity=empty")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	endpoint := c.baseURL + "/v2/contract/" + request.ChainID.String() + "/" + request.Token.Hex() + "?fields=compilation,stdJsonInput"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return tax.SourceBundle{}, fmt.Errorf("failed to create token source request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "K4K3RU-onchain")
	response, err := c.http.Do(req)
	if err != nil {
		return tax.SourceBundle{}, fmt.Errorf("failed to fetch token source: %w", err)
	}
	if response == nil || response.Body == nil {
		return tax.SourceBundle{}, fmt.Errorf("failed to fetch token source: response=null")
	}
	if response.StatusCode != http.StatusOK {
		httpErr := &HTTPError{StatusCode: response.StatusCode}
		if closeErr := response.Body.Close(); closeErr != nil {
			return tax.SourceBundle{}, fmt.Errorf("failed to close token source response: %w", errors.Join(httpErr, closeErr))
		}
		return tax.SourceBundle{}, httpErr
	}
	data, readErr := io.ReadAll(io.LimitReader(response.Body, 4*1024*1024+1))
	closeErr := response.Body.Close()
	if readErr != nil || closeErr != nil {
		return tax.SourceBundle{}, fmt.Errorf("failed to read token source response: %w", errors.Join(readErr, closeErr))
	}
	if len(data) > 4*1024*1024 {
		return tax.SourceBundle{}, fmt.Errorf("failed to read token source response: response=too_long")
	}
	var decoded struct {
		ChainID     string `json:"chainId"`
		Address     string `json:"address"`
		Compilation struct {
			Language           string `json:"language"`
			Compiler           string `json:"compiler"`
			CompilerVersion    string `json:"compilerVersion"`
			FullyQualifiedName string `json:"fullyQualifiedName"`
		} `json:"compilation"`
		Input json.RawMessage `json:"stdJsonInput"`
	}
	if err := json.Unmarshal(data, &decoded); err != nil {
		return tax.SourceBundle{}, fmt.Errorf("failed to decode token source: %w", err)
	}
	if decoded.ChainID != request.ChainID.String() || !common.IsHexAddress(decoded.Address) || common.HexToAddress(decoded.Address) != request.Token {
		return tax.SourceBundle{}, fmt.Errorf("failed to verify token source response: identity=invalid")
	}
	qualified := decoded.Compilation.FullyQualifiedName
	separator := strings.LastIndexByte(qualified, ':')
	if decoded.Compilation.Language != "Solidity" || decoded.Compilation.Compiler != "solc" || separator <= 0 || separator == len(qualified)-1 || len(decoded.Input) == 0 || string(decoded.Input) == "null" {
		return tax.SourceBundle{}, fmt.Errorf("failed to resolve token source: %w", tax.ErrUnsupported)
	}
	return tax.SourceBundle{CompilerVersion: decoded.Compilation.CompilerVersion, ContractFile: qualified[:separator], ContractName: qualified[separator+1:], Input: decoded.Input}, nil
}
