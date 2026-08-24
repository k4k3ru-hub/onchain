package solana

import (
	"context"
	"fmt"
)

type TransactionStatus struct {
	Signature          Signature
	Slot               Slot
	Confirmations      *uint64
	ConfirmationStatus Commitment
	Failed             bool
}

type rpcSignatureStatus struct {
	slot               Slot
	confirmations      *uint64
	confirmationStatus Commitment
	failed             bool
}

type signatureStatusProvider interface {
	getSignatureStatuses(ctx context.Context, signatures []Signature) ([]*rpcSignatureStatus, error)
}

// TransactionStatus returns the status of one Solana transaction signature.
//
// Parameters:
//   - ctx: request context; nil uses context.Background.
//   - signature: transaction signature.
//
// Returns:
//   - Transaction status; nil when the signature is not found.
//   - Retrieval error.
//
// Version:
//   - 2026-08-22: Added.
func (c *RPCClient) TransactionStatus(ctx context.Context, signature Signature) (*TransactionStatus, error) {
	statuses, err := c.TransactionStatuses(ctx, []Signature{signature})
	if err != nil {
		return nil, fmt.Errorf("failed to get solana transaction status: %w", err)
	}
	if len(statuses) != 1 {
		return nil, fmt.Errorf("failed to get solana transaction status: statuses=invalid actual_length=%d expected_length=1", len(statuses))
	}
	return statuses[0], nil
}

// TransactionStatuses returns statuses for Solana transaction signatures.
//
// Parameters:
//   - ctx: request context; nil uses context.Background.
//   - signatures: transaction signatures in requested result order.
//
// Returns:
//   - Transaction statuses; a nil element means the corresponding signature was not found.
//   - Retrieval error.
//
// Version:
//   - 2026-08-22: Added.
func (c *RPCClient) TransactionStatuses(ctx context.Context, signatures []Signature) ([]*TransactionStatus, error) {
	if c == nil {
		return nil, fmt.Errorf("failed to get solana transaction statuses: rpc_client=null")
	}
	if c.signatureStatusProvider == nil {
		return nil, fmt.Errorf("failed to get solana transaction statuses: signature_status_provider=null")
	}
	if len(signatures) == 0 {
		return nil, fmt.Errorf("failed to get solana transaction statuses: signatures=empty")
	}
	for _, signature := range signatures {
		if signature.IsZero() {
			return nil, fmt.Errorf("failed to get solana transaction statuses: signature=empty")
		}
	}
	if ctx == nil {
		ctx = context.Background()
	}

	rpcStatuses, err := c.signatureStatusProvider.getSignatureStatuses(ctx, signatures)
	if err != nil {
		return nil, fmt.Errorf("failed to get solana transaction statuses: %w", err)
	}
	if len(rpcStatuses) != len(signatures) {
		return nil, fmt.Errorf(
			"failed to get solana transaction statuses: statuses=invalid actual_length=%d expected_length=%d",
			len(rpcStatuses),
			len(signatures),
		)
	}

	statuses := make([]*TransactionStatus, len(signatures))
	for i, rpcStatus := range rpcStatuses {
		if rpcStatus == nil {
			continue
		}
		if err := rpcStatus.confirmationStatus.Validate(); err != nil {
			return nil, fmt.Errorf("failed to get solana transaction statuses: %w: status_index=%d", err, i)
		}
		if err := rpcStatus.slot.Validate(); err != nil {
			return nil, fmt.Errorf("failed to get solana transaction statuses: %w: status_index=%d", err, i)
		}
		confirmations := rpcStatus.confirmations
		if confirmations != nil {
			copiedConfirmations := *confirmations
			confirmations = &copiedConfirmations
		}
		statuses[i] = &TransactionStatus{
			Signature:          signatures[i],
			Slot:               rpcStatus.slot,
			Confirmations:      confirmations,
			ConfirmationStatus: rpcStatus.confirmationStatus,
			Failed:             rpcStatus.failed,
		}
	}
	return statuses, nil
}

// IsConfirmed reports whether the transaction succeeded at confirmed or finalized commitment.
//
// Returns:
//   - True when the transaction succeeded and is confirmed or finalized.
//
// Version:
//   - 2026-08-22: Added.
func (s *TransactionStatus) IsConfirmed() bool {
	if s == nil || s.Failed {
		return false
	}
	return s.ConfirmationStatus == CommitmentConfirmed || s.ConfirmationStatus == CommitmentFinalized
}

// IsFinalized reports whether the transaction succeeded at finalized commitment.
//
// Returns:
//   - True when the transaction succeeded and is finalized.
//
// Version:
//   - 2026-08-22: Added.
func (s *TransactionStatus) IsFinalized() bool {
	return s != nil && !s.Failed && s.ConfirmationStatus == CommitmentFinalized
}

// IsSuccessful reports whether the transaction status represents successful execution.
//
// Returns:
//   - True when the status exists and the transaction did not fail.
//
// Version:
//   - 2026-08-22: Added.
func (s *TransactionStatus) IsSuccessful() bool {
	return s != nil && !s.Failed
}
