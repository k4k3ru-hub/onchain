# HTTP client injection

`NewRPCClient(ctx, RPCConfig{URL: endpoint, Commitment: CommitmentConfirmed,
HTTPClient: sharedHTTPClient})` composes all RPC providers using the supplied
`*http.Client`. Applications may share it across clients to coordinate endpoint
pacing and cooldowns through a custom RoundTripper. It remains caller-owned.
Omitting HTTPClient retains the existing SDK defaults. WebSocket construction
and lifecycles remain separate. The SDK adds no global limiter or transport logs.
