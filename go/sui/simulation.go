package sui

import (
	"context"
	"fmt"

	"github.com/k4k3ru-hub/onchain/go/sui/internal/rpcv2"
	"google.golang.org/protobuf/types/known/fieldmaskpb"
)

// SimulationExecutionError describes a transaction execution failure returned by Sui simulation.
type SimulationExecutionError struct {
	Description      string
	Kind             string
	CommandIndex     uint64
	HasCommandIndex  bool
	MoveAbortCode    uint64
	HasMoveAbortCode bool
	MovePackage      string
	MoveModule       string
	MoveFunction     string
}

// Error formats the Sui simulation execution failure.
//
// Returns:
//   - Execution failure description with available Sui diagnostics.
//
// Version:
//   - 2026-09-01: Added.
func (e *SimulationExecutionError) Error() string {
	if e == nil {
		return "failed to execute sui transaction simulation: execution_error=null"
	}
	message := fmt.Sprintf("failed to execute sui transaction simulation: execution=failed kind=%q", e.Kind)
	if e.Description != "" {
		message += fmt.Sprintf(" description=%q", e.Description)
	}
	if e.HasCommandIndex {
		message += fmt.Sprintf(" command_index=%d", e.CommandIndex)
	}
	if e.HasMoveAbortCode {
		message += fmt.Sprintf(" move_abort_code=%d", e.MoveAbortCode)
	}
	if e.MovePackage != "" {
		message += fmt.Sprintf(" move_package=%q", e.MovePackage)
	}
	if e.MoveModule != "" {
		message += fmt.Sprintf(" move_module=%q", e.MoveModule)
	}
	if e.MoveFunction != "" {
		message += fmt.Sprintf(" move_function=%q", e.MoveFunction)
	}
	return message
}

type SimulationRequest struct {
	Sender         Address
	Transaction    ProgrammableTransaction
	ChecksEnabled  bool
	DoGasSelection bool
}

type CommandOutput struct {
	BCS  []byte
	JSON any
}

type SimulationCommandResult struct {
	ReturnValues []CommandOutput
	MutatedByRef []CommandOutput
}

type SimulationEvent struct {
	Package Address
	Module  string
	Type    string
	BCS     []byte
	JSON    any
}

type SimulationResult struct {
	CommandResults    []SimulationCommandResult
	Events            []SimulationEvent
	Checkpoint        CheckpointSequenceNumber
	SuggestedGasPrice uint64
}

type transactionSimulationProvider interface {
	simulateTransaction(context.Context, SimulationRequest) (*SimulationResult, error)
}

// SimulateTransaction simulates a programmable transaction without executing it.
//
// Parameters:
//   - ctx: Request context; nil uses context.Background.
//   - request: Simulation request.
//
// Returns:
//   - Simulation result.
//   - Simulation or validation error.
//
// Version:
//   - 2026-09-01: Returned the checkpoint used by simulation.
//   - 2026-08-30: Added.
func (c *GRPCClient) SimulateTransaction(ctx context.Context, request SimulationRequest) (*SimulationResult, error) {
	if c == nil {
		return nil, fmt.Errorf("failed to simulate sui transaction: grpc_client=null")
	}
	if c.simulationProvider == nil {
		return nil, fmt.Errorf("failed to simulate sui transaction: simulation_provider=null")
	}
	if request.Sender.IsZero() {
		return nil, fmt.Errorf("failed to simulate sui transaction: sender=empty")
	}
	if err := request.Transaction.Validate(); err != nil {
		return nil, fmt.Errorf("failed to simulate sui transaction: %w", err)
	}
	if ctx == nil {
		ctx = context.Background()
	}
	result, err := c.simulationProvider.simulateTransaction(ctx, request)
	if err != nil {
		return nil, fmt.Errorf("failed to simulate sui transaction: %w", err)
	}
	if result == nil {
		return nil, fmt.Errorf("failed to simulate sui transaction: result=null")
	}
	return result, nil
}

func (a *grpcAdapter) simulateTransaction(ctx context.Context, request SimulationRequest) (*SimulationResult, error) {
	if a == nil || a.executionClient == nil {
		return nil, fmt.Errorf("failed to call sui transaction simulation: execution_client=null")
	}
	transaction, err := transactionToRPC(request)
	if err != nil {
		return nil, fmt.Errorf("failed to call sui transaction simulation: %w", err)
	}
	checks := rpcv2.SimulateTransactionRequest_DISABLED
	if request.ChecksEnabled {
		checks = rpcv2.SimulateTransactionRequest_ENABLED
	}
	response, err := a.executionClient.SimulateTransaction(ctx, &rpcv2.SimulateTransactionRequest{
		Transaction: transaction,
		ReadMask: &fieldmaskpb.FieldMask{Paths: []string{
			"transaction.effects", "transaction.events", "transaction.checkpoint", "command_outputs", "suggested_gas_price",
		}},
		Checks:         checks.Enum(),
		DoGasSelection: &request.DoGasSelection,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to call sui transaction simulation: %w", err)
	}
	if response == nil || response.Transaction == nil || response.Transaction.Effects == nil {
		return nil, fmt.Errorf("failed to call sui transaction simulation: response=invalid")
	}
	status := response.Transaction.Effects.Status
	if status == nil {
		return nil, fmt.Errorf("failed to call sui transaction simulation: execution_status=null")
	}
	if !status.GetSuccess() {
		return nil, fmt.Errorf("failed to call sui transaction simulation: %w", simulationExecutionError(status.GetError()))
	}
	if response.Transaction.Checkpoint == nil {
		return nil, fmt.Errorf("failed to call sui transaction simulation: checkpoint=null")
	}
	checkpoint := CheckpointSequenceNumber(response.Transaction.GetCheckpoint())
	if err := checkpoint.Validate(); err != nil {
		return nil, fmt.Errorf("failed to call sui transaction simulation: %w", err)
	}
	result := &SimulationResult{Checkpoint: checkpoint, SuggestedGasPrice: response.GetSuggestedGasPrice(), CommandResults: make([]SimulationCommandResult, len(response.CommandOutputs))}
	for i, command := range response.CommandOutputs {
		if command == nil {
			return nil, fmt.Errorf("failed to call sui transaction simulation: command_output=null command_index=%d", i)
		}
		result.CommandResults[i].ReturnValues = commandOutputs(command.ReturnValues)
		result.CommandResults[i].MutatedByRef = commandOutputs(command.MutatedByRef)
	}
	if response.Transaction.Events != nil {
		result.Events = make([]SimulationEvent, len(response.Transaction.Events.Events))
		for i, event := range response.Transaction.Events.Events {
			if event == nil {
				return nil, fmt.Errorf("failed to call sui transaction simulation: event=null event_index=%d", i)
			}
			packageAddress, err := ParseAddress(event.GetPackageId())
			if err != nil {
				return nil, fmt.Errorf("failed to call sui transaction simulation: event_package=invalid: %w: event_index=%d", err, i)
			}
			result.Events[i] = SimulationEvent{Package: packageAddress, Module: event.GetModule(), Type: event.GetEventType()}
			if event.Contents != nil {
				result.Events[i].BCS = append([]byte(nil), event.Contents.Value...)
			}
			if event.Json != nil {
				result.Events[i].JSON = event.Json.AsInterface()
			}
		}
	}
	return result, nil
}

func simulationExecutionError(value *rpcv2.ExecutionError) *SimulationExecutionError {
	if value == nil {
		return &SimulationExecutionError{Kind: rpcv2.ExecutionError_EXECUTION_ERROR_KIND_UNKNOWN.String()}
	}
	result := &SimulationExecutionError{
		Description:     value.GetDescription(),
		Kind:            value.GetKind().String(),
		HasCommandIndex: value.Command != nil,
		CommandIndex:    value.GetCommand(),
	}
	if abort := value.GetAbort(); abort != nil {
		result.HasMoveAbortCode = abort.AbortCode != nil
		result.MoveAbortCode = abort.GetAbortCode()
		if location := abort.GetLocation(); location != nil {
			result.MovePackage = location.GetPackage()
			result.MoveModule = location.GetModule()
			result.MoveFunction = location.GetFunctionName()
		}
	}
	return result
}

func transactionToRPC(request SimulationRequest) (*rpcv2.Transaction, error) {
	inputs := make([]*rpcv2.Input, len(request.Transaction.Inputs))
	for i, input := range request.Transaction.Inputs {
		converted, err := inputToRPC(input)
		if err != nil {
			return nil, fmt.Errorf("failed to convert sui transaction input: %w: input_index=%d", err, i)
		}
		inputs[i] = converted
	}
	commands := make([]*rpcv2.Command, len(request.Transaction.Commands))
	for i, command := range request.Transaction.Commands {
		switch command.Kind {
		case CommandKindMoveCall:
			call := command.MoveCall
			arguments := make([]*rpcv2.Argument, len(call.Arguments))
			for j, argument := range call.Arguments {
				arguments[j] = argumentToRPC(argument)
			}
			packageID, module, function := call.Package.String(), call.Module, call.Function
			commands[i] = &rpcv2.Command{Command: &rpcv2.Command_MoveCall{MoveCall: &rpcv2.MoveCall{
				Package: &packageID, Module: &module, Function: &function,
				TypeArguments: append([]string(nil), call.TypeArguments...), Arguments: arguments,
			}}}
		case CommandKindMakeMoveVec:
			vector := command.MakeMoveVec
			elements := make([]*rpcv2.Argument, len(vector.Elements))
			for j, element := range vector.Elements {
				elements[j] = argumentToRPC(element)
			}
			elementType := vector.ElementType
			commands[i] = &rpcv2.Command{Command: &rpcv2.Command_MakeMoveVector{MakeMoveVector: &rpcv2.MakeMoveVector{ElementType: &elementType, Elements: elements}}}
		default:
			return nil, fmt.Errorf("failed to convert sui transaction command: command_kind=invalid command_index=%d", i)
		}
	}
	kind := rpcv2.TransactionKind_PROGRAMMABLE_TRANSACTION
	sender := request.Sender.String()
	return &rpcv2.Transaction{Sender: &sender, Kind: &rpcv2.TransactionKind{
		Kind: &kind,
		Data: &rpcv2.TransactionKind_ProgrammableTransaction{
			ProgrammableTransaction: &rpcv2.ProgrammableTransaction{Inputs: inputs, Commands: commands},
		},
	}}, nil
}

func inputToRPC(input ProgrammableTransactionInput) (*rpcv2.Input, error) {
	converted := &rpcv2.Input{}
	switch input.Kind {
	case InputKindPure:
		kind := rpcv2.Input_PURE
		converted.Kind, converted.Pure = &kind, append([]byte(nil), input.Pure...)
	case InputKindImmutableOrOwned, InputKindReceiving:
		kind := rpcv2.Input_IMMUTABLE_OR_OWNED
		if input.Kind == InputKindReceiving {
			kind = rpcv2.Input_RECEIVING
		}
		address, digest, version := input.Object.Address.String(), input.Object.Digest.String(), input.Object.Version
		converted.Kind, converted.ObjectId, converted.Digest, converted.Version = &kind, &address, &digest, &version
	case InputKindShared:
		kind := rpcv2.Input_SHARED
		address, version, mutable := input.Object.Address.String(), input.Object.Version, input.Object.Mutable
		mutability := rpcv2.Input_IMMUTABLE
		if mutable {
			mutability = rpcv2.Input_MUTABLE
		}
		converted.Kind, converted.ObjectId, converted.Version, converted.Mutable, converted.Mutability = &kind, &address, &version, &mutable, &mutability
	default:
		return nil, fmt.Errorf("input_kind=invalid")
	}
	return converted, nil
}

func argumentToRPC(argument Argument) *rpcv2.Argument {
	converted := &rpcv2.Argument{}
	switch argument.Kind {
	case ArgumentKindGas:
		kind := rpcv2.Argument_GAS
		converted.Kind = &kind
	case ArgumentKindInput:
		kind, index := rpcv2.Argument_INPUT, uint32(argument.Index)
		converted.Kind, converted.Input = &kind, &index
	case ArgumentKindResult:
		kind, index := rpcv2.Argument_RESULT, uint32(argument.Index)
		converted.Kind, converted.Result = &kind, &index
		if argument.Subresult != nil {
			subresult := uint32(*argument.Subresult)
			converted.Subresult = &subresult
		}
	}
	return converted
}

func commandOutputs(values []*rpcv2.CommandOutput) []CommandOutput {
	outputs := make([]CommandOutput, len(values))
	for i, value := range values {
		if value == nil {
			continue
		}
		if value.Value != nil {
			outputs[i].BCS = append([]byte(nil), value.Value.Value...)
		}
		if value.Json != nil {
			outputs[i].JSON = value.Json.AsInterface()
		}
	}
	return outputs
}
