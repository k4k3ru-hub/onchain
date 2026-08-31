package sui

import (
	"encoding/binary"
	"fmt"
	"strings"
)

// AppendBalanceValue appends a call that reads a Balance<T> value.
//
// Parameters:
//   - builder: Programmable transaction builder.
//   - coinType: Balance coin type.
//   - balance: Balance argument.
//
// Returns:
//   - u64 value result argument.
//   - Validation error.
//
// Version:
//   - 2026-08-31: Added.
func AppendBalanceValue(builder *ProgrammableTransactionBuilder, coinType string, balance Argument) (Argument, error) {
	return appendFrameworkMoveCall(builder, "balance", "value", coinType, []Argument{balance}, "failed to append sui balance value")
}

// AppendBalanceSplit appends a call that splits an exact amount from a Balance<T>.
//
// Parameters:
//   - builder: Programmable transaction builder.
//   - coinType: Balance coin type.
//   - balance: Mutable source balance argument.
//   - amount: u64 amount argument.
//
// Returns:
//   - Split Balance<T> result argument.
//   - Validation error.
//
// Version:
//   - 2026-08-31: Added.
func AppendBalanceSplit(builder *ProgrammableTransactionBuilder, coinType string, balance, amount Argument) (Argument, error) {
	return appendFrameworkMoveCall(builder, "balance", "split", coinType, []Argument{balance, amount}, "failed to append sui balance split")
}

// AppendBalanceJoin appends a call that joins one Balance<T> into another.
//
// Parameters:
//   - builder: Programmable transaction builder.
//   - coinType: Balance coin type.
//   - destination: Mutable destination balance argument.
//   - source: Consumed source balance argument.
//
// Returns:
//   - Validation error.
//
// Version:
//   - 2026-08-31: Added.
func AppendBalanceJoin(builder *ProgrammableTransactionBuilder, coinType string, destination, source Argument) error {
	_, err := appendFrameworkMoveCall(builder, "balance", "join", coinType, []Argument{destination, source}, "failed to append sui balance join")
	return err
}

// AppendZeroBalance appends a call that creates an empty Balance<T>.
//
// Parameters:
//   - builder: Programmable transaction builder.
//   - coinType: Balance coin type.
//
// Returns:
//   - Empty Balance<T> result argument.
//   - Validation error.
//
// Version:
//   - 2026-08-31: Added.
func AppendZeroBalance(builder *ProgrammableTransactionBuilder, coinType string) (Argument, error) {
	return appendFrameworkMoveCall(builder, "balance", "zero", coinType, nil, "failed to append sui zero balance")
}

// AppendTransferBalance appends calls that convert a Balance<T> to Coin<T> and transfer it.
//
// Parameters:
//   - builder: Programmable transaction builder.
//   - coinType: Balance coin type.
//   - balance: Consumed balance argument.
//   - recipient: Transfer recipient.
//
// Returns:
//   - Validation error.
//
// Version:
//   - 2026-08-31: Added.
func AppendTransferBalance(builder *ProgrammableTransactionBuilder, coinType string, balance Argument, recipient Address) error {
	if recipient.IsZero() {
		return fmt.Errorf("failed to append sui balance transfer: recipient=empty")
	}
	coin, err := appendFrameworkMoveCall(builder, "coin", "from_balance", coinType, []Argument{balance}, "failed to append sui balance transfer")
	if err != nil {
		return err
	}
	recipientArgument, err := builder.Pure(recipient.Bytes())
	if err != nil {
		return fmt.Errorf("failed to append sui balance transfer: %w", err)
	}
	_, err = appendFrameworkMoveCall(builder, "transfer", "public_transfer", "0x2::coin::Coin<"+strings.TrimSpace(coinType)+">", []Argument{coin, recipientArgument}, "failed to append sui balance transfer")
	return err
}

// PureUint64 adds one BCS-encoded u64 input.
//
// Parameters:
//   - builder: Programmable transaction builder.
//   - value: Unsigned value.
//
// Returns:
//   - Input argument.
//   - Validation error.
//
// Version:
//   - 2026-08-31: Added.
func PureUint64(builder *ProgrammableTransactionBuilder, value uint64) (Argument, error) {
	if builder == nil {
		return Argument{}, fmt.Errorf("failed to add sui programmable transaction u64 input: builder=null")
	}
	encoded := make([]byte, 8)
	binary.LittleEndian.PutUint64(encoded, value)
	argument, err := builder.Pure(encoded)
	if err != nil {
		return Argument{}, fmt.Errorf("failed to add sui programmable transaction u64 input: %w", err)
	}
	return argument, nil
}

func appendFrameworkMoveCall(builder *ProgrammableTransactionBuilder, module, function, typeArgument string, arguments []Argument, operation string) (Argument, error) {
	if builder == nil {
		return Argument{}, fmt.Errorf("%s: builder=null", operation)
	}
	typeArgument = strings.TrimSpace(typeArgument)
	if typeArgument == "" {
		return Argument{}, fmt.Errorf("%s: coin_type=empty", operation)
	}
	framework, err := ParseAddress("0x2")
	if err != nil {
		return Argument{}, fmt.Errorf("%s: %w", operation, err)
	}
	result, err := builder.MoveCall(MoveCall{Package: framework, Module: module, Function: function, TypeArguments: []string{typeArgument}, Arguments: arguments})
	if err != nil {
		return Argument{}, fmt.Errorf("%s: %w", operation, err)
	}
	return result, nil
}
