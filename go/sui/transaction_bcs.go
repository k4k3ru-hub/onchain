package sui

import "fmt"

func (w *transactionBCSWriter) argument(value Argument) {
	switch value.Kind {
	case ArgumentKindGas:
		w.uleb(0)
	case ArgumentKindInput:
		w.uleb(1)
		w.u16(value.Index)
	case ArgumentKindResult:
		if value.Subresult == nil {
			w.uleb(2)
			w.u16(value.Index)
		} else {
			w.uleb(3)
			w.u16(value.Index)
			w.u16(*value.Subresult)
		}
	}
}
func (w *transactionBCSWriter) arguments(values []Argument) {
	w.uleb(uint32(len(values)))
	for _, value := range values {
		w.argument(value)
	}
}
func (w *transactionBCSWriter) programmable(t ProgrammableTransaction) error {
	w.uleb(uint32(len(t.Inputs)))
	for _, input := range t.Inputs {
		if input.Kind == InputKindPure {
			w.uleb(0)
			w.bytes(input.Pure)
			continue
		}
		w.uleb(1)
		switch input.Kind {
		case InputKindImmutableOrOwned:
			w.uleb(0)
		case InputKindShared:
			w.uleb(1)
		case InputKindReceiving:
			w.uleb(2)
		}
		if input.Kind == InputKindShared {
			w.address(input.Object.Address)
			w.u64(input.Object.Version)
			if input.Object.Mutable {
				w.raw([]byte{1})
			} else {
				w.raw([]byte{0})
			}
		} else {
			w.object(ObjectReference{Address: input.Object.Address, Version: input.Object.Version, Digest: input.Object.Digest})
		}
	}
	w.uleb(uint32(len(t.Commands)))
	for _, command := range t.Commands {
		switch command.Kind {
		case CommandKindMoveCall:
			w.uleb(0)
			call := command.MoveCall
			w.address(call.Package)
			w.text(call.Module)
			w.text(call.Function)
			w.uleb(uint32(len(call.TypeArguments)))
			for _, value := range call.TypeArguments {
				tag, err := parseTransactionTypeTag(value)
				if err != nil {
					return err
				}
				w.typeTag(tag)
			}
			w.arguments(call.Arguments)
		case CommandKindTransferObjects:
			w.uleb(1)
			w.arguments(command.TransferObjects.Objects)
			w.argument(command.TransferObjects.Address)
		case CommandKindSplitCoins:
			w.uleb(2)
			w.argument(command.SplitCoins.Coin)
			w.arguments(command.SplitCoins.Amounts)
		case CommandKindMergeCoins:
			w.uleb(3)
			w.argument(command.MergeCoins.Destination)
			w.arguments(command.MergeCoins.Sources)
		case CommandKindMakeMoveVec:
			w.uleb(5)
			w.uleb(1) // Some(TypeTag); the current builder requires an explicit type.
			tag, err := parseTransactionTypeTag(command.MakeMoveVec.ElementType)
			if err != nil {
				return err
			}
			w.typeTag(tag)
			w.arguments(command.MakeMoveVec.Elements)
		default:
			return fmt.Errorf("failed to encode sui programmable transaction: command=unsupported")
		}
	}
	return nil
}

func (r *transactionBCSReader) argument() Argument {
	switch r.uleb() {
	case 0:
		return Argument{Kind: ArgumentKindGas}
	case 1:
		return Argument{Kind: ArgumentKindInput, Index: r.u16()}
	case 2:
		return Argument{Kind: ArgumentKindResult, Index: r.u16()}
	case 3:
		index, subresult := r.u16(), r.u16()
		return Argument{Kind: ArgumentKindResult, Index: index, Subresult: &subresult}
	default:
		r.fail("argument=unsupported")
		return Argument{}
	}
}
func (r *transactionBCSReader) arguments() []Argument {
	values := make([]Argument, r.length(maxTransactionElements))
	for i := range values {
		values[i] = r.argument()
	}
	return values
}
func (r *transactionBCSReader) programmable() ProgrammableTransaction {
	t := ProgrammableTransaction{Inputs: make([]ProgrammableTransactionInput, r.length(maxTransactionElements))}
	for i := range t.Inputs {
		if r.err != nil {
			break
		}
		input := &t.Inputs[i]
		switch r.uleb() {
		case 0:
			input.Kind, input.Pure = InputKindPure, r.bytes()
		case 1:
			switch r.uleb() {
			case 0:
				input.Kind = InputKindImmutableOrOwned
			case 1:
				input.Kind = InputKindShared
			case 2:
				input.Kind = InputKindReceiving
			default:
				r.fail("object_variant=unsupported")
			}
			if input.Kind == InputKindShared {
				input.Object.Address, input.Object.Version = r.address(), r.u64()
				mutable := r.byte()
				if mutable > 1 {
					r.fail("bool=invalid")
				}
				input.Object.Mutable = mutable == 1
			} else {
				object := r.object()
				input.Object = ObjectInput{Address: object.Address, Version: object.Version, Digest: object.Digest}
			}
		default:
			r.fail("input_variant=unsupported")
		}
	}
	t.Commands = make([]Command, r.length(maxTransactionElements))
	for i := range t.Commands {
		if r.err != nil {
			break
		}
		command := &t.Commands[i]
		switch r.uleb() {
		case 0:
			call := &MoveCall{Package: r.address(), Module: r.text(), Function: r.text()}
			call.TypeArguments = make([]string, r.length(maxTransactionElements))
			for j := range call.TypeArguments {
				tag := r.typeTag(0)
				if r.err != nil {
					break
				}
				call.TypeArguments[j] = tag.text()
			}
			call.Arguments = r.arguments()
			command.Kind, command.MoveCall = CommandKindMoveCall, call
		case 1:
			command.Kind, command.TransferObjects = CommandKindTransferObjects, &TransferObjects{Objects: r.arguments(), Address: r.argument()}
		case 2:
			command.Kind, command.SplitCoins = CommandKindSplitCoins, &SplitCoins{Coin: r.argument(), Amounts: r.arguments()}
		case 3:
			command.Kind, command.MergeCoins = CommandKindMergeCoins, &MergeCoins{Destination: r.argument(), Sources: r.arguments()}
		case 5:
			if r.uleb() != 1 {
				r.fail("vector_type=unsupported")
				break
			}
			tag := r.typeTag(0)
			if r.err != nil {
				break
			}
			command.Kind, command.MakeMoveVec = CommandKindMakeMoveVec, &MakeMoveVec{ElementType: tag.text(), Elements: r.arguments()}
		default:
			r.fail("command_variant=unsupported")
		}
	}
	return t
}
