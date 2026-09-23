# Contract deployment metadata

This package retrieves deployment transaction **candidates**, not security or
liquidity-lock verdicts. Construct `NewClient(httpClient, baseURL)` at the
application composition boundary and call `Deployment(ctx, chainID, address)`.

- Uses `GET /v2/contract/{chainID}/{address}?fields=deployment`.
- Checks the returned chain/address and transaction-hash encoding.
- Missing deployment metadata returns a zero hash; callers decide whether to retry.
- One HTTP attempt per call, at most 20 seconds and 64 KiB.
- Inject a transport with redirects disabled when the application requires a
  strict outbound-attempt budget. MarketHub supplies this configuration.
- Retries, shared caching and persistent attempt limits belong to the application.
- All candidates require independent onchain verification.

The package has no dependency on the ERC-20 tax compiler or LP custody models.
Protocol reference: [Sourcify API v2 lookup endpoints](https://docs.sourcify.dev/blog/apiv2-lookup-endpoints/).
