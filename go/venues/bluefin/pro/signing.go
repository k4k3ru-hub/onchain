package pro

import (
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/k4k3ru-hub/onchain/go/sui"
	"golang.org/x/crypto/blake2b"
)

type PersonalMessageSigner interface{ SignPersonalMessage([]byte) ([]byte, error) }

type LoginRequest struct {
	AccountAddress string `json:"accountAddress"`
	SignedAtMS     int64  `json:"signedAtMillis"`
	Audience       string `json:"audience"`
}

// SigningBytes serializes the official compact login JSON with its defined field order.
//
// Version:
//   - 2026-10-14: Added.
func (r LoginRequest) SigningBytes() ([]byte, error) {
	if err := address(r.AccountAddress); err != nil {
		return nil, fmt.Errorf("failed to encode bluefin login: %w", err)
	}
	if r.Audience != "api" || r.SignedAtMS <= 0 {
		return nil, fmt.Errorf("failed to encode bluefin login: payload=invalid")
	}
	data, err := json.Marshal(r)
	if err != nil {
		return nil, fmt.Errorf("failed to encode bluefin login: %w", err)
	}
	return data, nil
}

type OrderFields struct {
	Symbol         string `json:"symbol"`
	AccountAddress string `json:"accountAddress"`
	PriceE9        string `json:"priceE9"`
	QuantityE9     string `json:"quantityE9"`
	Side           string `json:"side"`
	LeverageE9     string `json:"leverageE9"`
	IsIsolated     bool   `json:"isIsolated"`
	Salt           string `json:"salt"`
	IDSID          string `json:"idsId"`
	ExpiresAtMS    int64  `json:"expiresAtMillis"`
	SignedAtMS     int64  `json:"signedAtMillis"`
}

// Validate checks the protocol's bounded scalar and address representation.
// Market-specific quantity, price and leverage policies belong to the consumer.
//
// Version:
//   - 2026-10-14: Added.
func (f OrderFields) Validate() error {
	if len(f.Symbol) < 6 || len(f.Symbol) > 32 {
		return fmt.Errorf("failed to validate bluefin order fields: symbol=invalid")
	}
	for _, c := range f.Symbol {
		if (c < 'A' || c > 'Z') && (c < '0' || c > '9') && c != '-' {
			return fmt.Errorf("failed to validate bluefin order fields: symbol=invalid")
		}
	}
	if f.Side != "LONG" && f.Side != "SHORT" {
		return fmt.Errorf("failed to validate bluefin order fields: side=invalid")
	}
	for _, value := range []string{f.AccountAddress, f.IDSID} {
		if err := address(value); err != nil {
			return fmt.Errorf("failed to validate bluefin order fields: %w", err)
		}
	}
	for _, value := range []string{f.PriceE9, f.QuantityE9, f.Salt} {
		if _, err := integer(value, false); err != nil {
			return fmt.Errorf("failed to validate bluefin order fields: %w", err)
		}
	}
	if _, err := integer(f.LeverageE9, true); err != nil {
		return fmt.Errorf("failed to validate bluefin order fields: %w", err)
	}
	if f.SignedAtMS <= 0 || f.ExpiresAtMS <= f.SignedAtMS {
		return fmt.Errorf("failed to validate bluefin order fields: lifetime=invalid")
	}
	return nil
}

// SigningBytes reproduces Bluefin's two-space-indented UI order JSON.
// Order flags and client IDs are intentionally not in the upstream signing format.
//
// Version:
//   - 2026-10-14: Added.
func (f OrderFields) SigningBytes() ([]byte, error) {
	if err := f.Validate(); err != nil {
		return nil, err
	}
	positionType := "CROSS"
	if f.IsIsolated {
		positionType = "ISOLATED"
	}
	if f.Side != "LONG" && f.Side != "SHORT" {
		return nil, fmt.Errorf("failed to encode bluefin order: side=invalid")
	}
	value := struct {
		Type         string `json:"type"`
		IDS          string `json:"ids"`
		Account      string `json:"account"`
		Market       string `json:"market"`
		Price        string `json:"price"`
		Quantity     string `json:"quantity"`
		Leverage     string `json:"leverage"`
		Side         string `json:"side"`
		PositionType string `json:"positionType"`
		Expiration   string `json:"expiration"`
		Salt         string `json:"salt"`
		SignedAt     string `json:"signedAt"`
	}{"Bluefin Pro Order", f.IDSID, f.AccountAddress, f.Symbol, f.PriceE9, f.QuantityE9, f.LeverageE9, f.Side, positionType, strconv.FormatInt(f.ExpiresAtMS, 10), f.Salt, strconv.FormatInt(f.SignedAtMS, 10)}
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to encode bluefin order: %w", err)
	}
	return data, nil
}

// OrderHash computes the official BCS order hash independently of its signature and HTTP acknowledgment.
//
// Version:
//   - 2026-10-14: Added.
func (f OrderFields) OrderHash() (string, error) {
	if _, err := f.SigningBytes(); err != nil {
		return "", err
	}
	ids, err := sui.ParseAddress(f.IDSID)
	if err != nil {
		return "", fmt.Errorf("failed to hash bluefin order: %w", err)
	}
	account, err := sui.ParseAddress(f.AccountAddress)
	if err != nil {
		return "", fmt.Errorf("failed to hash bluefin order: %w", err)
	}
	data := append(append([]byte(nil), ids[:]...), account[:]...)
	text := func(value string) {
		for n := uint64(len(value)); ; n >>= 7 {
			b := byte(n & 127)
			if n >= 128 {
				b |= 128
			}
			data = append(data, b)
			if n < 128 {
				break
			}
		}
		data = append(data, value...)
	}
	text(f.Symbol)
	for _, value := range []string{f.PriceE9, f.QuantityE9, f.LeverageE9} {
		n, err := integer(value, true)
		if err != nil {
			return "", err
		}
		data = binary.LittleEndian.AppendUint64(data, n)
	}
	text(f.Side)
	positionType := "CROSS"
	if f.IsIsolated {
		positionType = "ISOLATED"
	}
	text(positionType)
	data = binary.LittleEndian.AppendUint64(data, uint64(f.ExpiresAtMS))
	salt, err := integer(f.Salt, false)
	if err != nil {
		return "", err
	}
	data = binary.LittleEndian.AppendUint64(data, salt)
	data = binary.LittleEndian.AppendUint64(data, uint64(f.SignedAtMS))
	digest := blake2b.Sum256(data)
	return hex.EncodeToString(digest[:]), nil
}

// SignMessage signs protocol bytes without retaining private-key material.
//
// Version:
//   - 2026-10-14: Added.
func SignMessage(ctx context.Context, signer PersonalMessageSigner, message []byte) (string, error) {
	if ctx == nil || signer == nil {
		return "", fmt.Errorf("failed to sign bluefin message: dependency=null")
	}
	if err := ctx.Err(); err != nil {
		return "", fmt.Errorf("failed to sign bluefin message: %w", err)
	}
	signature, err := signer.SignPersonalMessage(message)
	if err != nil {
		return "", fmt.Errorf("failed to sign bluefin message: %w", err)
	}
	return base64.StdEncoding.EncodeToString(signature), nil
}

// VerifySignature verifies a serialized Sui Ed25519 PersonalMessage for the expected account.
//
// Version:
//   - 2026-10-14: Added.
func VerifySignature(message []byte, encoded, account string) error {
	if len(encoded) != base64.StdEncoding.EncodedLen(97) {
		return fmt.Errorf("failed to verify bluefin signature: signature=invalid")
	}
	signature, err := base64.StdEncoding.Strict().DecodeString(encoded)
	if err != nil {
		return fmt.Errorf("failed to verify bluefin signature: signature=invalid")
	}
	signer, err := sui.ParseAddress(account)
	if err != nil {
		return fmt.Errorf("failed to verify bluefin signature: signer=invalid")
	}
	if err := sui.VerifyPersonalMessageSignature(message, signature, signer); err != nil {
		return fmt.Errorf("failed to verify bluefin signature: %w", err)
	}
	return nil
}
