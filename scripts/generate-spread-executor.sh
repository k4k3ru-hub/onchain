#!/usr/bin/env bash

set -euo pipefail

repository_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
artifact_path="${repository_root}/out/SpreadExecutor.sol/SpreadExecutor.json"
abi_path="${repository_root}/abi/evm/spread/SpreadExecutor.json"
binding_path="${repository_root}/go/evm/spreadexecutor/spread_executor.go"

mkdir -p "$(dirname "${abi_path}")" "$(dirname "${binding_path}")"

cd "${repository_root}"
forge build
jq --sort-keys '.abi' "${artifact_path}" > "${abi_path}"

cd "${repository_root}/go"
GOPROXY="${GOPROXY:-https://proxy.golang.org}" go run github.com/ethereum/go-ethereum/cmd/abigen@v1.17.3 \
  --abi "${abi_path}" \
  --pkg spreadexecutor \
  --type SpreadExecutor \
  --out "${binding_path}"
