package core

import (
	"fmt"
	"strings"
)

const (
	maxProtocolLength = 64
	maxPoolIDLength   = 256
)

type Protocol string

// Normalize returns the canonical protocol identifier.
//
// Returns:
//   - Lowercase, trimmed protocol identifier.
//
// Version:
//   - 2026-08-31: Added.
func (p Protocol) Normalize() Protocol {
	return Protocol(strings.ToLower(strings.TrimSpace(string(p))))
}

// Validate validates the protocol identifier.
//
// Returns:
//   - Validation error.
//
// Version:
//   - 2026-08-31: Added.
func (p Protocol) Validate() error {
	normalized := p.Normalize()
	if normalized == "" {
		return fmt.Errorf("failed to validate protocol: protocol=empty")
	}
	if len(normalized) > maxProtocolLength {
		return fmt.Errorf("failed to validate protocol: protocol=too_long actual_length=%d max_length=%d", len(normalized), maxProtocolLength)
	}
	for _, character := range normalized {
		if (character < 'a' || character > 'z') && (character < '0' || character > '9') && character != '-' {
			return fmt.Errorf("failed to validate protocol: protocol=invalid")
		}
	}
	return nil
}

type PoolReference struct {
	Chain    Chain
	Network  Network
	Protocol Protocol
	PoolID   string
}

// Normalize returns a canonical pool reference.
//
// Returns:
//   - Normalized pool reference.
//
// Version:
//   - 2026-08-31: Added.
func (r PoolReference) Normalize() PoolReference {
	r.Protocol = r.Protocol.Normalize()
	r.PoolID = strings.TrimSpace(r.PoolID)
	return r
}

// Validate validates the pool reference.
//
// Returns:
//   - Validation error.
//
// Version:
//   - 2026-08-31: Added.
func (r PoolReference) Validate() error {
	r = r.Normalize()
	if err := r.Chain.Validate(); err != nil {
		return fmt.Errorf("failed to validate pool reference: %w", err)
	}
	if err := r.Network.Validate(); err != nil {
		return fmt.Errorf("failed to validate pool reference: %w", err)
	}
	if err := r.Protocol.Validate(); err != nil {
		return fmt.Errorf("failed to validate pool reference: %w", err)
	}
	if r.PoolID == "" {
		return fmt.Errorf("failed to validate pool reference: pool_id=empty")
	}
	if len(r.PoolID) > maxPoolIDLength {
		return fmt.Errorf("failed to validate pool reference: pool_id=too_long actual_length=%d max_length=%d", len(r.PoolID), maxPoolIDLength)
	}
	return nil
}
