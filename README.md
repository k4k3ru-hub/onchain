# onchain

Go SDK primitives for EVM, Solana, and Sui integrations.

## Shared pool catalogs

Use `core.PoolReference` as the stable key shared by market evaluation and
trade execution. Chain-family catalogs own the metadata needed to resolve that
key without coupling applications to one another.

```go
reference := core.PoolReference{
	Chain:    core.ChainBase,
	Network:  core.NetworkMainnet,
	Protocol: core.Protocol("uniswap-v3"),
	PoolID:   poolAddress,
}

pool, ok := evmCatalog.Resolve(reference)
if !ok {
	return fmt.Errorf("failed to resolve evm pool metadata")
}
```

`evm.PoolMetadata.Address` is optional because protocols such as Uniswap v4
identify pools by a bytes32 pool ID under a shared pool manager rather than by
an individual pool contract address.

The `core` package does not import a chain-family package, and family catalogs
do not import protocol packages. Applications compose protocol entries into an
EVM or Solana catalog at their composition root, keeping the dependency graph
acyclic.

## Solana AMM SDKs

Raydium CPMM, Raydium CLMM, and Meteora DLMM clients discover explicitly
configured pools from Solana account state. Public pool and quote types remain
owned by each DEX package.

```go
rpc, err := solana.NewRPCClient(ctx, solana.RPCConfig{
	URL:        rpcURL,
	Commitment: solana.CommitmentConfirmed,
})
if err != nil {
	return err
}

pool, err := solana.ParseAddress(poolAddress)
if err != nil {
	return err
}

client, err := dlmm.NewClient(ctx, rpc, dlmm.Config{
	Pools: []solana.Address{pool},
})
if err != nil {
	return err
}
```

Use `raydium/cpmm`, `raydium/clmm`, or `meteora/dlmm` directly rather than a
root-level facade. Quote methods refresh mutable pool state before calculating.
