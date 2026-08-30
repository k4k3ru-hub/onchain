package sui

import (
	"fmt"
	"strings"
)

type InputKind uint8

const (
	InputKindPure InputKind = iota + 1
	InputKindImmutableOrOwned
	InputKindShared
	InputKindReceiving
)

type ObjectInput struct {
	Address Address
	Version uint64
	Digest  ObjectDigest
	Mutable bool
}

type ProgrammableTransactionInput struct {
	Kind   InputKind
	Pure   []byte
	Object ObjectInput
}

type ArgumentKind uint8

const (
	ArgumentKindGas ArgumentKind = iota + 1
	ArgumentKindInput
	ArgumentKindResult
)

type Argument struct {
	Kind      ArgumentKind
	Index     uint16
	Subresult *uint16
}

type MoveCall struct {
	Package       Address
	Module        string
	Function      string
	TypeArguments []string
	Arguments     []Argument
}

type ProgrammableTransaction struct {
	Inputs   []ProgrammableTransactionInput
	Commands []MoveCall
}

type ProgrammableTransactionBuilder struct {
	transaction ProgrammableTransaction
}

// NewProgrammableTransactionBuilder creates an empty Sui programmable transaction builder.
//
// Returns:
//   - Programmable transaction builder.
//
// Version:
//   - 2026-08-30: Added.
func NewProgrammableTransactionBuilder() *ProgrammableTransactionBuilder {
	return &ProgrammableTransactionBuilder{}
}

// Pure adds one BCS-encoded Move value input.
//
// Parameters:
//   - value: BCS-encoded Move value.
//
// Returns:
//   - Input argument.
//   - Validation error.
//
// Version:
//   - 2026-08-30: Added.
func (b *ProgrammableTransactionBuilder) Pure(value []byte) (Argument, error) {
	if b == nil {
		return Argument{}, fmt.Errorf("failed to add sui programmable transaction pure input: builder=null")
	}
	if value == nil {
		return Argument{}, fmt.Errorf("failed to add sui programmable transaction pure input: value=null")
	}
	return b.addInput(ProgrammableTransactionInput{Kind: InputKindPure, Pure: append([]byte(nil), value...)})
}

// Object adds one immutable, owned, shared, or receiving object input.
//
// Parameters:
//   - kind: Object input kind.
//   - object: Object reference.
//
// Returns:
//   - Input argument.
//   - Validation error.
//
// Version:
//   - 2026-08-30: Added.
func (b *ProgrammableTransactionBuilder) Object(kind InputKind, object ObjectInput) (Argument, error) {
	if b == nil {
		return Argument{}, fmt.Errorf("failed to add sui programmable transaction object input: builder=null")
	}
	input := ProgrammableTransactionInput{Kind: kind, Object: object}
	if err := input.validate(); err != nil {
		return Argument{}, fmt.Errorf("failed to add sui programmable transaction object input: %w", err)
	}
	return b.addInput(input)
}

// MoveCall appends one Move call command.
//
// Parameters:
//   - call: Move call.
//
// Returns:
//   - Command result argument.
//   - Validation error.
//
// Version:
//   - 2026-08-30: Added.
func (b *ProgrammableTransactionBuilder) MoveCall(call MoveCall) (Argument, error) {
	if b == nil {
		return Argument{}, fmt.Errorf("failed to add sui programmable transaction move call: builder=null")
	}
	if err := call.validate(len(b.transaction.Inputs), len(b.transaction.Commands)); err != nil {
		return Argument{}, fmt.Errorf("failed to add sui programmable transaction move call: %w", err)
	}
	call.TypeArguments = append([]string(nil), call.TypeArguments...)
	call.Arguments = append([]Argument(nil), call.Arguments...)
	b.transaction.Commands = append(b.transaction.Commands, call)
	return Argument{Kind: ArgumentKindResult, Index: uint16(len(b.transaction.Commands) - 1)}, nil
}

// NestedResult selects one return value from a command result.
//
// Parameters:
//   - result: Command result argument.
//   - index: Return-value index.
//
// Returns:
//   - Nested result argument.
//   - Validation error.
//
// Version:
//   - 2026-08-30: Added.
func NestedResult(result Argument, index uint16) (Argument, error) {
	if result.Kind != ArgumentKindResult || result.Subresult != nil {
		return Argument{}, fmt.Errorf("failed to select sui programmable transaction nested result: result=invalid")
	}
	return Argument{Kind: ArgumentKindResult, Index: result.Index, Subresult: &index}, nil
}

// Build validates and returns an immutable programmable transaction snapshot.
//
// Returns:
//   - Programmable transaction.
//   - Validation error.
//
// Version:
//   - 2026-08-30: Added.
func (b *ProgrammableTransactionBuilder) Build() (ProgrammableTransaction, error) {
	if b == nil {
		return ProgrammableTransaction{}, fmt.Errorf("failed to build sui programmable transaction: builder=null")
	}
	if err := b.transaction.Validate(); err != nil {
		return ProgrammableTransaction{}, fmt.Errorf("failed to build sui programmable transaction: %w", err)
	}
	return b.transaction.clone(), nil
}

// Validate validates a programmable transaction.
//
// Returns:
//   - Validation error.
//
// Version:
//   - 2026-08-30: Added.
func (t ProgrammableTransaction) Validate() error {
	if len(t.Commands) == 0 {
		return fmt.Errorf("failed to validate sui programmable transaction: commands=empty")
	}
	for i, input := range t.Inputs {
		if err := input.validate(); err != nil {
			return fmt.Errorf("failed to validate sui programmable transaction: %w: input_index=%d", err, i)
		}
	}
	for i, command := range t.Commands {
		if err := command.validate(len(t.Inputs), i); err != nil {
			return fmt.Errorf("failed to validate sui programmable transaction: %w: command_index=%d", err, i)
		}
	}
	return nil
}

func (b *ProgrammableTransactionBuilder) addInput(input ProgrammableTransactionInput) (Argument, error) {
	if len(b.transaction.Inputs) >= 1<<16 {
		return Argument{}, fmt.Errorf("failed to add sui programmable transaction input: inputs=too_long max_length=%d", 1<<16-1)
	}
	b.transaction.Inputs = append(b.transaction.Inputs, input)
	return Argument{Kind: ArgumentKindInput, Index: uint16(len(b.transaction.Inputs) - 1)}, nil
}

func (i ProgrammableTransactionInput) validate() error {
	switch i.Kind {
	case InputKindPure:
		if i.Pure == nil {
			return fmt.Errorf("failed to validate sui programmable transaction input: pure=null")
		}
	case InputKindImmutableOrOwned, InputKindReceiving:
		if i.Object.Address.IsZero() || i.Object.Version == 0 || i.Object.Digest.IsZero() {
			return fmt.Errorf("failed to validate sui programmable transaction input: object=invalid")
		}
	case InputKindShared:
		if i.Object.Address.IsZero() || i.Object.Version == 0 {
			return fmt.Errorf("failed to validate sui programmable transaction input: shared_object=invalid")
		}
	default:
		return fmt.Errorf("failed to validate sui programmable transaction input: kind=invalid")
	}
	return nil
}

func (c MoveCall) validate(inputCount, commandCount int) error {
	if c.Package.IsZero() {
		return fmt.Errorf("failed to validate sui move call: package=empty")
	}
	if strings.TrimSpace(c.Module) == "" {
		return fmt.Errorf("failed to validate sui move call: module=empty")
	}
	if strings.TrimSpace(c.Function) == "" {
		return fmt.Errorf("failed to validate sui move call: function=empty")
	}
	for i, value := range c.TypeArguments {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("failed to validate sui move call: type_argument=empty type_argument_index=%d", i)
		}
	}
	for i, argument := range c.Arguments {
		if err := argument.validate(inputCount, commandCount); err != nil {
			return fmt.Errorf("failed to validate sui move call: %w: argument_index=%d", err, i)
		}
	}
	return nil
}

func (a Argument) validate(inputCount, commandCount int) error {
	switch a.Kind {
	case ArgumentKindGas:
		if a.Subresult != nil {
			return fmt.Errorf("failed to validate sui programmable transaction argument: gas=invalid")
		}
	case ArgumentKindInput:
		if int(a.Index) >= inputCount || a.Subresult != nil {
			return fmt.Errorf("failed to validate sui programmable transaction argument: input=invalid")
		}
	case ArgumentKindResult:
		if int(a.Index) >= commandCount {
			return fmt.Errorf("failed to validate sui programmable transaction argument: result=invalid")
		}
	default:
		return fmt.Errorf("failed to validate sui programmable transaction argument: kind=invalid")
	}
	return nil
}

func (t ProgrammableTransaction) clone() ProgrammableTransaction {
	result := ProgrammableTransaction{Inputs: make([]ProgrammableTransactionInput, len(t.Inputs)), Commands: make([]MoveCall, len(t.Commands))}
	copy(result.Inputs, t.Inputs)
	for i := range result.Inputs {
		result.Inputs[i].Pure = append([]byte(nil), t.Inputs[i].Pure...)
	}
	for i, command := range t.Commands {
		result.Commands[i] = command
		result.Commands[i].TypeArguments = append([]string(nil), command.TypeArguments...)
		result.Commands[i].Arguments = append([]Argument(nil), command.Arguments...)
	}
	return result
}
