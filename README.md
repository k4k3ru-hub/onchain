# onchain

Go SDK primitives for EVM, Solana, and Sui integrations.

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
