package clmm

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	onchainSui "github.com/k4k3ru-hub/onchain/go/sui"
)

type Swap struct {
	Checkpoint      onchainSui.CheckpointSequenceNumber
	SequenceNumber  uint64
	Transaction     onchainSui.TransactionDigest
	EventIndex      uint32
	Timestamp       time.Time
	Pool            onchainSui.Address
	Sender          onchainSui.Address
	A2B             bool
	AmountIn        uint64
	AmountOut       uint64
	FeeAmount       uint64
	BeforeSqrtPrice string
	AfterSqrtPrice  string
}

// ParseSwapEvent parses a Cetus CLMM swap event.
//
// Parameters:
//   - event: Sui event.
//
// Returns:
//   - Parsed swap.
//   - Parse or validation error.
//
// Version:
//   - 2026-08-31: Supported the current atob field and optional sender in Cetus swap events.
//   - 2026-08-30: Added.
func ParseSwapEvent(event onchainSui.Event) (Swap, error) {
	swap, err := parseSwapEvent(event.Type, event.JSON)
	if err != nil {
		return Swap{}, err
	}
	swap.SequenceNumber = event.SequenceNumber
	swap.Checkpoint = event.Checkpoint
	swap.Transaction = event.Transaction
	swap.Timestamp = event.Timestamp
	return swap, nil
}

// ParseLiveSwapEvent parses a live Cetus CLMM swap event.
//
// Parameters:
//   - event: Sui live event.
//
// Returns:
//   - Parsed swap.
//   - Parse or validation error.
//
// Version:
//   - 2026-08-30: Added.
func ParseLiveSwapEvent(event onchainSui.LiveEvent) (Swap, error) {
	swap, err := parseSwapEvent(event.Type, event.JSON)
	if err != nil {
		return Swap{}, err
	}
	swap.Checkpoint = event.Checkpoint
	swap.Transaction = event.Transaction
	swap.EventIndex = event.EventIndex
	return swap, nil
}

func parseSwapEvent(eventType string, eventJSON json.RawMessage) (Swap, error) {
	if !strings.HasSuffix(eventType, "::pool::SwapEvent") {
		return Swap{}, fmt.Errorf("failed to parse cetus clmm swap event: event_type=invalid")
	}
	var value struct {
		Pool            string          `json:"pool"`
		Sender          string          `json:"sender"`
		AToB            *bool           `json:"atob"`
		A2B             *bool           `json:"a2b"`
		AmountIn        json.RawMessage `json:"amount_in"`
		AmountOut       json.RawMessage `json:"amount_out"`
		FeeAmount       json.RawMessage `json:"fee_amount"`
		BeforeSqrtPrice json.RawMessage `json:"before_sqrt_price"`
		AfterSqrtPrice  json.RawMessage `json:"after_sqrt_price"`
	}
	if err := json.Unmarshal(eventJSON, &value); err != nil {
		return Swap{}, fmt.Errorf("failed to parse cetus clmm swap event: failed to decode event: %w", err)
	}
	pool, err := onchainSui.ParseAddress(value.Pool)
	if err != nil {
		return Swap{}, fmt.Errorf("failed to parse cetus clmm swap event: pool=invalid: %w", err)
	}
	var sender onchainSui.Address
	if strings.TrimSpace(value.Sender) != "" {
		sender, err = onchainSui.ParseAddress(value.Sender)
		if err != nil {
			return Swap{}, fmt.Errorf("failed to parse cetus clmm swap event: sender=invalid: %w", err)
		}
	}
	a2b := value.AToB
	if a2b == nil {
		a2b = value.A2B
	}
	if a2b == nil {
		return Swap{}, fmt.Errorf("failed to parse cetus clmm swap event: atob=invalid")
	}
	amountIn, err := jsonUint64(value.AmountIn)
	if err != nil || amountIn == 0 {
		return Swap{}, fmt.Errorf("failed to parse cetus clmm swap event: amount_in=invalid")
	}
	amountOut, err := jsonUint64(value.AmountOut)
	if err != nil || amountOut == 0 {
		return Swap{}, fmt.Errorf("failed to parse cetus clmm swap event: amount_out=invalid")
	}
	fee, err := jsonUint64(value.FeeAmount)
	if err != nil {
		return Swap{}, fmt.Errorf("failed to parse cetus clmm swap event: fee_amount=invalid")
	}
	before, err := jsonUnsigned(value.BeforeSqrtPrice)
	if err != nil {
		return Swap{}, fmt.Errorf("failed to parse cetus clmm swap event: before_sqrt_price=invalid")
	}
	after, err := jsonUnsigned(value.AfterSqrtPrice)
	if err != nil {
		return Swap{}, fmt.Errorf("failed to parse cetus clmm swap event: after_sqrt_price=invalid")
	}
	return Swap{Pool: pool, Sender: sender, A2B: *a2b, AmountIn: amountIn, AmountOut: amountOut, FeeAmount: fee, BeforeSqrtPrice: before.String(), AfterSqrtPrice: after.String()}, nil
}
