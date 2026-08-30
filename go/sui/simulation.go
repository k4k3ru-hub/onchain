package sui

import (
	"context"
	"fmt"

	"github.com/k4k3ru-hub/onchain/go/sui/internal/rpcv2"
	"google.golang.org/protobuf/types/known/fieldmaskpb"
)

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

type SimulationResult struct {
	CommandResults    []SimulationCommandResult
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
			"transaction.effects", "command_outputs", "suggested_gas_price",
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
	if status == nil || !status.GetSuccess() {
		return nil, fmt.Errorf("failed to call sui transaction simulation: execution=failed")
	}
	result := &SimulationResult{SuggestedGasPrice: response.GetSuggestedGasPrice(), CommandResults: make([]SimulationCommandResult, len(response.CommandOutputs))}
	for i, command := range response.CommandOutputs {
		if command == nil {
			return nil, fmt.Errorf("failed to call sui transaction simulation: command_output=null command_index=%d", i)
		}
		result.CommandResults[i].ReturnValues = commandOutputs(command.ReturnValues)
		result.CommandResults[i].MutatedByRef = commandOutputs(command.MutatedByRef)
	}
	return result, nil
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
	for i, call := range request.Transaction.Commands {
		arguments := make([]*rpcv2.Argument, len(call.Arguments))
		for j, argument := range call.Arguments {
			arguments[j] = argumentToRPC(argument)
		}
		packageID, module, function := call.Package.String(), call.Module, call.Function
		commands[i] = &rpcv2.Command{Command: &rpcv2.Command_MoveCall{MoveCall: &rpcv2.MoveCall{
			Package: &packageID, Module: &module, Function: &function,
			TypeArguments: append([]string(nil), call.TypeArguments...), Arguments: arguments,
		}}}
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
