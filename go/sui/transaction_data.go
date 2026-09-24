package sui

import (
	"fmt"

	"golang.org/x/crypto/blake2b"
)

type ObjectReference struct {
	Address Address
	Version uint64
	Digest  ObjectDigest
}

type GasData struct {
	Payment []ObjectReference
	Owner   Address
	Price   uint64
	Budget  uint64
}

// TransactionData models V1 programmable transactions with explicit gas coins.
// A nil ExpirationEpoch means no chain expiration, not an application TTL.
type TransactionData struct {
	Transaction     ProgrammableTransaction
	Sender          Address
	GasData         GasData
	ExpirationEpoch *uint64
}

// Validate validates an owned object reference without reading chain state.
//
// Version:
//   - 2026-09-24: Added.
func (r ObjectReference) Validate() error {
	if r.Address.IsZero() || r.Version == 0 || r.Digest.IsZero() {
		return fmt.Errorf("failed to validate sui object reference: object_reference=invalid")
	}
	return nil
}

// Validate validates the supported transaction structure, not onchain availability.
//
// Version:
//   - 2026-09-24: Added.
func (t TransactionData) Validate() error {
	if t.Sender.IsZero() || t.GasData.Owner.IsZero() {
		return fmt.Errorf("failed to validate sui transaction data: address=empty")
	}
	if t.GasData.Price == 0 || t.GasData.Budget == 0 {
		return fmt.Errorf("failed to validate sui transaction data: gas=empty")
	}
	if len(t.GasData.Payment) == 0 {
		return fmt.Errorf("failed to validate sui transaction data: gas_payment=empty")
	}
	if len(t.GasData.Payment) > maxTransactionElements {
		return fmt.Errorf("failed to validate sui transaction data: gas_payment=too_long")
	}
	if err := t.Transaction.Validate(); err != nil {
		return fmt.Errorf("failed to validate sui transaction data: %w", err)
	}
	objects := make(map[Address]struct{})
	for _, input := range t.Transaction.Inputs {
		if input.Kind == InputKindPure {
			if input.Object != (ObjectInput{}) || len(input.Pure) > maxTransactionDataSize {
				return fmt.Errorf("failed to validate sui transaction data: pure_input=invalid")
			}
			continue
		}
		if input.Pure != nil || (input.Kind != InputKindShared && input.Object.Mutable) || (input.Kind == InputKindShared && !input.Object.Digest.IsZero()) {
			return fmt.Errorf("failed to validate sui transaction data: object_input=invalid")
		}
		objects[input.Object.Address] = struct{}{}
	}
	for _, payment := range t.GasData.Payment {
		if err := payment.Validate(); err != nil {
			return fmt.Errorf("failed to validate sui transaction data: %w", err)
		}
		if _, exists := objects[payment.Address]; exists {
			return fmt.Errorf("failed to validate sui transaction data: gas_object=duplicate")
		}
		objects[payment.Address] = struct{}{}
	}
	for _, command := range t.Transaction.Commands {
		if command.MoveCall != nil {
			call := command.MoveCall
			if !validMoveIdentifier(call.Module) || !validMoveIdentifier(call.Function) {
				return fmt.Errorf("failed to validate sui transaction data: move_identifier=invalid")
			}
			if len(call.TypeArguments) > maxTransactionElements || len(call.Arguments) > maxTransactionElements {
				return fmt.Errorf("failed to validate sui transaction data: move_arguments=too_long")
			}
			for _, value := range call.TypeArguments {
				if _, err := parseTransactionTypeTag(value); err != nil {
					return fmt.Errorf("failed to validate sui transaction data: %w", err)
				}
			}
		}
		if command.MakeMoveVec != nil {
			if len(command.MakeMoveVec.Elements) > maxTransactionElements {
				return fmt.Errorf("failed to validate sui transaction data: vector_elements=too_long")
			}
			if _, err := parseTransactionTypeTag(command.MakeMoveVec.ElementType); err != nil {
				return fmt.Errorf("failed to validate sui transaction data: %w", err)
			}
		}
	}
	return nil
}

// MarshalBCS encodes complete V1 TransactionData with explicit gas and expiration.
//
// Version:
//   - 2026-09-24: Added.
func (t TransactionData) MarshalBCS() ([]byte, error) {
	if err := t.Validate(); err != nil {
		return nil, fmt.Errorf("failed to encode sui transaction data: %w", err)
	}
	w := transactionBCSWriter{}
	w.uleb(0) // TransactionData::V1
	w.uleb(0) // TransactionKind::ProgrammableTransaction
	if err := w.programmable(t.Transaction); err != nil {
		return nil, fmt.Errorf("failed to encode sui transaction data: %w", err)
	}
	w.address(t.Sender)
	w.uleb(uint32(len(t.GasData.Payment)))
	for _, payment := range t.GasData.Payment {
		w.object(payment)
	}
	w.address(t.GasData.Owner)
	w.u64(t.GasData.Price)
	w.u64(t.GasData.Budget)
	if t.ExpirationEpoch == nil {
		w.uleb(0)
	} else {
		w.uleb(1)
		w.u64(*t.ExpirationEpoch)
	}
	if w.err != nil {
		return nil, fmt.Errorf("failed to encode sui transaction data: %w", w.err)
	}
	return w.data, nil
}

// ParseTransactionData decodes supported canonical BCS and rejects trailing data.
// Publish, Upgrade, address-balance withdrawals and newer expiration variants are
// deliberately unsupported. Decoding is bounded to 1 MiB and 64 nested type tags.
//
// Version:
//   - 2026-09-24: Added.
func ParseTransactionData(data []byte) (TransactionData, error) {
	if len(data) == 0 || len(data) > maxTransactionDataSize {
		return TransactionData{}, fmt.Errorf("failed to parse sui transaction data: data=out_of_range max_length=%d", maxTransactionDataSize)
	}
	r := transactionBCSReader{data: data}
	if r.uleb() != 0 {
		r.fail("transaction_version=unsupported")
	}
	if r.uleb() != 0 {
		r.fail("transaction_kind=unsupported")
	}
	t := TransactionData{Transaction: r.programmable(), Sender: r.address()}
	t.GasData.Payment = make([]ObjectReference, r.length(maxTransactionElements))
	for i := range t.GasData.Payment {
		t.GasData.Payment[i] = r.object()
	}
	t.GasData.Owner, t.GasData.Price, t.GasData.Budget = r.address(), r.u64(), r.u64()
	switch r.uleb() {
	case 0:
	case 1:
		epoch := r.u64()
		t.ExpirationEpoch = &epoch
	default:
		r.fail("expiration=unsupported")
	}
	if r.err != nil {
		return TransactionData{}, fmt.Errorf("failed to parse sui transaction data: %w", r.err)
	}
	if r.position != len(data) {
		return TransactionData{}, fmt.Errorf("failed to parse sui transaction data: trailing_data=invalid")
	}
	if err := t.Validate(); err != nil {
		return TransactionData{}, fmt.Errorf("failed to parse sui transaction data: %w", err)
	}
	return t, nil
}

// SigningDigest returns Blake2b-256 of the Sui TransactionData intent and BCS.
// This is the user-signature prehash, not the onchain transaction digest.
//
// Version:
//   - 2026-09-24: Added.
func (t TransactionData) SigningDigest() ([32]byte, error) {
	data, err := t.MarshalBCS()
	if err != nil {
		return [32]byte{}, fmt.Errorf("failed to hash sui transaction intent: %w", err)
	}
	return transactionIntentDigest(data), nil
}

// Digest returns the onchain transaction identifier using the TransactionData tag.
//
// Version:
//   - 2026-09-24: Added.
func (t TransactionData) Digest() (TransactionDigest, error) {
	data, err := t.MarshalBCS()
	if err != nil {
		return TransactionDigest{}, fmt.Errorf("failed to hash sui transaction data: %w", err)
	}
	return TransactionDigest(blake2b.Sum256(append([]byte("TransactionData::"), data...))), nil
}

func transactionIntentDigest(data []byte) [32]byte {
	return blake2b.Sum256(append([]byte{0, 0, 0}, data...))
}
