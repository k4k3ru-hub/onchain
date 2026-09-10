# Sui Swap ordering

Bluefin Spot, Cetus CLMM, Momentum CLMM and Turbos CLMM expose TransactionIndex (*uint64), the transaction position within Checkpoint. Live parsers preserve the existing Sui LiveEvent index, including zero. No additional RPC is required.

Compare checkpoint first, then TransactionIndex, then EventIndex (live) or SequenceNumber (historical transaction-local event index). Transaction digest identifies the transaction; do not sort by its text. Different events within one transaction must retain the same digest.

TransactionIndex is nil for the existing GraphQL event history transport, which does not provide that position. Nil must never be interpreted as zero. Historical ParseSwapEvent preserves optional index metadata when a caller supplies it, and copies its value so the result does not alias the input pointer.

Example:

    if swap.TransactionIndex == nil {
        // Store history, but do not use this event to replace ordered current state.
        return
    }
    transactionOffset := *swap.TransactionIndex

Protocol reference: https://github.com/MystenLabs/sui-apis/blob/main/proto/sui/rpc/v2/event.proto
The bundled rpcv2 Event schema documents transaction_index as a zero-based checkpoint-local position.
