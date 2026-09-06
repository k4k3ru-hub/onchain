# Contract ABI

This directory contains the canonical ABI generated from contracts in this repository.
Do not edit generated ABI or Go bindings manually.

Generate the SpreadExecutor ABI and Go binding from the repository root:

```sh
./scripts/generate-spread-executor.sh
```

The generated files are:

- `abi/evm/spread/SpreadExecutor.json`
- `go/evm/spreadexecutor/spread_executor.go`

Contract build artifacts under `out/` and `cache/` are local-only and are not source artifacts.
