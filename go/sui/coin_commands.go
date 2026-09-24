package sui

import "fmt"

type SplitCoins struct {
	Coin    Argument
	Amounts []Argument
}

type MergeCoins struct {
	Destination Argument
	Sources     []Argument
}

type TransferObjects struct {
	Objects []Argument
	Address Argument
}

// SplitCoins appends a native coin split and returns its command result.
// Select each output coin with NestedResult(result, index).
//
// Version:
//   - 2026-09-24: Added.
func (b *ProgrammableTransactionBuilder) SplitCoins(split SplitCoins) (Argument, error) {
	if b == nil {
		return Argument{}, fmt.Errorf("failed to append sui coin split: builder=null")
	}
	split.Coin, split.Amounts = cloneArgument(split.Coin), cloneArguments(split.Amounts)
	command := Command{Kind: CommandKindSplitCoins, SplitCoins: &split}
	if err := command.validate(len(b.transaction.Inputs), len(b.transaction.Commands)); err != nil {
		return Argument{}, fmt.Errorf("failed to append sui coin split: %w", err)
	}
	return b.addCommand(command)
}

// MergeCoins appends a native coin merge that consumes the source coins.
//
// Version:
//   - 2026-09-24: Added.
func (b *ProgrammableTransactionBuilder) MergeCoins(merge MergeCoins) error {
	if b == nil {
		return fmt.Errorf("failed to append sui coin merge: builder=null")
	}
	merge.Destination, merge.Sources = cloneArgument(merge.Destination), cloneArguments(merge.Sources)
	command := Command{Kind: CommandKindMergeCoins, MergeCoins: &merge}
	if err := command.validate(len(b.transaction.Inputs), len(b.transaction.Commands)); err != nil {
		return fmt.Errorf("failed to append sui coin merge: %w", err)
	}
	_, err := b.addCommand(command)
	return err
}

// TransferObjects appends a native transfer to a BCS address argument.
//
// Version:
//   - 2026-09-24: Added.
func (b *ProgrammableTransactionBuilder) TransferObjects(transfer TransferObjects) error {
	if b == nil {
		return fmt.Errorf("failed to append sui object transfer: builder=null")
	}
	transfer.Address, transfer.Objects = cloneArgument(transfer.Address), cloneArguments(transfer.Objects)
	command := Command{Kind: CommandKindTransferObjects, TransferObjects: &transfer}
	if err := command.validate(len(b.transaction.Inputs), len(b.transaction.Commands)); err != nil {
		return fmt.Errorf("failed to append sui object transfer: %w", err)
	}
	_, err := b.addCommand(command)
	return err
}

func (c Command) validateCoinCommand(inputCount, commandCount int) error {
	var primary Argument
	var arguments []Argument
	switch {
	case c.Kind == CommandKindSplitCoins && c.SplitCoins != nil:
		primary, arguments = c.SplitCoins.Coin, c.SplitCoins.Amounts
	case c.Kind == CommandKindMergeCoins && c.MergeCoins != nil:
		primary, arguments = c.MergeCoins.Destination, c.MergeCoins.Sources
	case c.Kind == CommandKindTransferObjects && c.TransferObjects != nil:
		primary, arguments = c.TransferObjects.Address, c.TransferObjects.Objects
	default:
		return fmt.Errorf("failed to validate sui coin command: variant=invalid")
	}
	if len(arguments) == 0 {
		return fmt.Errorf("failed to validate sui coin command: arguments=empty")
	}
	if len(arguments) > 1<<16 {
		return fmt.Errorf("failed to validate sui coin command: arguments=too_long max_length=%d", 1<<16)
	}
	if err := primary.validate(inputCount, commandCount); err != nil {
		return fmt.Errorf("failed to validate sui coin command: %w", err)
	}
	for i, argument := range arguments {
		if err := argument.validate(inputCount, commandCount); err != nil {
			return fmt.Errorf("failed to validate sui coin command: %w: argument_index=%d", err, i)
		}
	}
	return nil
}

// AppendCoinIntoBalance appends a call consuming Coin<T> and returning Balance<T>.
//
// Version:
//   - 2026-09-24: Added.
func AppendCoinIntoBalance(builder *ProgrammableTransactionBuilder, coinType string, coin Argument) (Argument, error) {
	return appendFrameworkMoveCall(builder, "coin", "into_balance", coinType, []Argument{coin}, "failed to append sui coin into balance")
}

// AppendDestroyZeroBalance appends a call that aborts unless the balance is zero.
//
// Version:
//   - 2026-09-24: Added.
func AppendDestroyZeroBalance(builder *ProgrammableTransactionBuilder, coinType string, balance Argument) error {
	_, err := appendFrameworkMoveCall(builder, "balance", "destroy_zero", coinType, []Argument{balance}, "failed to append sui zero balance destruction")
	return err
}
