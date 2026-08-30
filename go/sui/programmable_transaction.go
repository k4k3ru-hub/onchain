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

type MakeMoveVec struct {
	ElementType string
	Elements    []Argument
}

type CommandKind uint8

const (
	CommandKindMoveCall CommandKind = iota + 1
	CommandKindMakeMoveVec
)

type Command struct {
	Kind        CommandKind
	MoveCall    *MoveCall
	MakeMoveVec *MakeMoveVec
}

type ProgrammableTransaction struct {
	Inputs   []ProgrammableTransactionInput
	Commands []Command
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
	b.transaction.Commands = append(b.transaction.Commands, Command{Kind: CommandKindMoveCall, MoveCall: &call})
	return Argument{Kind: ArgumentKindResult, Index: uint16(len(b.transaction.Commands) - 1)}, nil
}

// MakeMoveVec appends one command that constructs a Move vector.
//
// ElementType may be empty when Move can infer an object element type. It is
// required for an empty vector or when the elements are pure values.
//
// Parameters:
//   - vector: Move vector definition.
//
// Returns:
//   - Command result argument.
//   - Validation error.
//
// Version:
//   - 2026-08-31: Added.
func (b *ProgrammableTransactionBuilder) MakeMoveVec(vector MakeMoveVec) (Argument, error) {
	if b == nil {
		return Argument{}, fmt.Errorf("failed to add sui programmable transaction move vector: builder=null")
	}
	if err := vector.validate(len(b.transaction.Inputs), len(b.transaction.Commands)); err != nil {
		return Argument{}, fmt.Errorf("failed to add sui programmable transaction move vector: %w", err)
	}
	vector.Elements = append([]Argument(nil), vector.Elements...)
	b.transaction.Commands = append(b.transaction.Commands, Command{Kind: CommandKindMakeMoveVec, MakeMoveVec: &vector})
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

func (c Command) validate(inputCount, commandCount int) error {
	switch c.Kind {
	case CommandKindMoveCall:
		if c.MoveCall == nil || c.MakeMoveVec != nil {
			return fmt.Errorf("failed to validate sui programmable transaction command: move_call=invalid")
		}
		if err := c.MoveCall.validate(inputCount, commandCount); err != nil {
			return fmt.Errorf("failed to validate sui programmable transaction command: %w", err)
		}
	case CommandKindMakeMoveVec:
		if c.MakeMoveVec == nil || c.MoveCall != nil {
			return fmt.Errorf("failed to validate sui programmable transaction command: make_move_vec=invalid")
		}
		if err := c.MakeMoveVec.validate(inputCount, commandCount); err != nil {
			return fmt.Errorf("failed to validate sui programmable transaction command: %w", err)
		}
	default:
		return fmt.Errorf("failed to validate sui programmable transaction command: kind=invalid")
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

func (v MakeMoveVec) validate(inputCount, commandCount int) error {
	if strings.TrimSpace(v.ElementType) == "" {
		return fmt.Errorf("failed to validate sui move vector: element_type=empty")
	}
	for i, element := range v.Elements {
		if err := element.validate(inputCount, commandCount); err != nil {
			return fmt.Errorf("failed to validate sui move vector: %w: element_index=%d", err, i)
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
	result := ProgrammableTransaction{Inputs: make([]ProgrammableTransactionInput, len(t.Inputs)), Commands: make([]Command, len(t.Commands))}
	copy(result.Inputs, t.Inputs)
	for i := range result.Inputs {
		result.Inputs[i].Pure = append([]byte(nil), t.Inputs[i].Pure...)
	}
	for i, command := range t.Commands {
		result.Commands[i].Kind = command.Kind
		if command.MoveCall != nil {
			call := *command.MoveCall
			call.TypeArguments = append([]string(nil), command.MoveCall.TypeArguments...)
			call.Arguments = append([]Argument(nil), command.MoveCall.Arguments...)
			result.Commands[i].MoveCall = &call
		}
		if command.MakeMoveVec != nil {
			vector := *command.MakeMoveVec
			vector.Elements = append([]Argument(nil), command.MakeMoveVec.Elements...)
			result.Commands[i].MakeMoveVec = &vector
		}
	}
	return result
}
