package cpmm

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"math/big"

	onchainSolana "github.com/k4k3ru-hub/onchain/go/solana"
)

const (
	poolDataLength      = 637
	ammConfigDataLength = 236
	tokenAmountOffset   = 64
	tokenAccountMinSize = 72
	feeDenominator      = uint64(1_000_000)
)

var (
	mainnetProgramID    = mustAddress("CPMMoo8L3F4NbTegBCKVNunggL7H1ZpdTHKxQB5qKP1C")
	splTokenProgramID   = mustAddress("TokenkegQfeZyiNwAJbNbGKPFXCWuBvf9Ss623VQ5DA")
	poolDiscriminator   = [8]byte{0xf7, 0xed, 0xe3, 0xf5, 0xd7, 0xc3, 0xde, 0x46}
	configDiscriminator = [8]byte{0xda, 0xf4, 0x21, 0x68, 0xcb, 0xcb, 0x2b, 0x6f}
)

type accountProvider interface {
	Account(context.Context, onchainSolana.Address) (*onchainSolana.Account, error)
}

type accountSnapshotProvider interface {
	AccountSnapshot(context.Context, []onchainSolana.Address) (*onchainSolana.AccountSnapshot, error)
}

type Config struct {
	ProgramID onchainSolana.Address
	Pools     []onchainSolana.Address
}

type Client struct {
	accounts  accountProvider
	snapshots accountSnapshotProvider
	programID onchainSolana.Address
	pools     map[onchainSolana.Address]Pool
	poolOrder []onchainSolana.Address
}

type Pool struct {
	Address           onchainSolana.Address
	AMMConfig         onchainSolana.Address
	Token0Vault       onchainSolana.Address
	Token1Vault       onchainSolana.Address
	Token0Mint        onchainSolana.Address
	Token1Mint        onchainSolana.Address
	Token0Program     onchainSolana.Address
	Token1Program     onchainSolana.Address
	Status            uint8
	Token0Decimals    uint8
	Token1Decimals    uint8
	ProtocolFees0     uint64
	ProtocolFees1     uint64
	FundFees0         uint64
	FundFees1         uint64
	CreatorFeeOn      uint8
	CreatorFeeEnabled bool
	CreatorFees0      uint64
	CreatorFees1      uint64
}

type Quote struct {
	PoolAddress onchainSolana.Address
	InputMint   onchainSolana.Address
	OutputMint  onchainSolana.Address
	AmountIn    uint64
	AmountOut   uint64
	TradeFee    uint64
	CreatorFee  uint64
}

type ExactInputRequest struct {
	InputMint onchainSolana.Address
	AmountIn  uint64
}

type QuoteBatch struct {
	Slot   onchainSolana.Slot
	Quotes []Quote
}

type ammConfig struct {
	tradeFeeRate    uint64
	protocolFeeRate uint64
	fundFeeRate     uint64
	creatorFeeRate  uint64
}

// MainnetProgramAddress returns the Raydium CPMM mainnet program address.
//
// Returns:
//   - Raydium CPMM mainnet program address.
//
// Version:
//   - 2026-08-31: Added.
func MainnetProgramAddress() onchainSolana.Address { return mainnetProgramID }

// NewClient discovers and validates configured Raydium CPMM pools.
//
// Parameters:
//   - ctx: construction context; nil uses context.Background.
//   - accounts: Solana account provider.
//   - config: CPMM program and configured pool addresses.
//
// Returns:
//   - Raydium CPMM client.
//   - Client creation error.
//
// Version:
//   - 2026-08-31: Added.
func NewClient(ctx context.Context, accounts accountProvider, config Config) (*Client, error) {
	if accounts == nil {
		return nil, fmt.Errorf("failed to create raydium cpmm client: accounts=null")
	}
	if config.ProgramID.IsZero() {
		config.ProgramID = mainnetProgramID
	}
	if len(config.Pools) == 0 {
		return nil, fmt.Errorf("failed to create raydium cpmm client: pools=empty")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	client := &Client{accounts: accounts, programID: config.ProgramID, pools: make(map[onchainSolana.Address]Pool, len(config.Pools)), poolOrder: make([]onchainSolana.Address, 0, len(config.Pools))}
	client.snapshots, _ = accounts.(accountSnapshotProvider)
	for i, address := range config.Pools {
		if address.IsZero() {
			return nil, fmt.Errorf("failed to create raydium cpmm client: pool=empty pool_index=%d", i)
		}
		if _, exists := client.pools[address]; exists {
			return nil, fmt.Errorf("failed to create raydium cpmm client: pool=invalid duplicate=true pool_index=%d", i)
		}
		account, err := accounts.Account(ctx, address)
		if err != nil {
			return nil, fmt.Errorf("failed to create raydium cpmm client: failed to discover pool: %w: pool_address=%q", err, address.String())
		}
		if account == nil {
			return nil, fmt.Errorf("failed to create raydium cpmm client: pool_account=null pool_address=%q", address.String())
		}
		if account.Owner != config.ProgramID {
			return nil, fmt.Errorf("failed to create raydium cpmm client: pool_owner=invalid pool_address=%q", address.String())
		}
		pool, err := decodePool(address, account.Data)
		if err != nil {
			return nil, fmt.Errorf("failed to create raydium cpmm client: %w: pool_address=%q", err, address.String())
		}
		if pool.Token0Program != splTokenProgramID || pool.Token1Program != splTokenProgramID {
			return nil, fmt.Errorf("failed to create raydium cpmm client: token_program=unsupported pool_address=%q", address.String())
		}
		client.pools[address] = pool
		client.poolOrder = append(client.poolOrder, address)
	}
	return client, nil
}

// Pools returns detached metadata for the configured CPMM pools.
//
// Returns:
//   - Configured pools.
//
// Version:
//   - 2026-08-31: Added.
func (c *Client) Pools() []Pool {
	if c == nil {
		return nil
	}
	result := make([]Pool, 0, len(c.poolOrder))
	for _, address := range c.poolOrder {
		result = append(result, c.pools[address])
	}
	return result
}

// QuoteExactInput returns an on-chain-state CPMM quote for one configured pool.
//
// Parameters:
//   - ctx: quote context; nil uses context.Background.
//   - poolAddress: configured pool address.
//   - inputMint: input token mint.
//   - amountIn: input amount in atomic token units.
//
// Returns:
//   - Exact-input quote for SPL Token pools.
//   - Quote error.
//
// Version:
//   - 2026-09-01: Used a batched account snapshot when supported.
//   - 2026-08-31: Added.
func (c *Client) QuoteExactInput(ctx context.Context, poolAddress, inputMint onchainSolana.Address, amountIn uint64) (Quote, error) {
	if c != nil && c.snapshots != nil {
		batch, err := c.QuoteExactInputsWithSlot(ctx, poolAddress, []ExactInputRequest{{InputMint: inputMint, AmountIn: amountIn}})
		if err != nil {
			return Quote{}, err
		}
		return batch.Quotes[0], nil
	}
	return c.quoteExactInput(ctx, poolAddress, inputMint, amountIn)
}

// QuoteExactInputs returns exact-input quotes from one batched account snapshot.
//
// Parameters:
//   - ctx: quote context; nil uses context.Background.
//   - poolAddress: configured pool address.
//   - requests: exact-input requests evaluated against the same account snapshot.
//
// Returns:
//   - Quotes in request order.
//   - Snapshot or quote error.
//
// Version:
//   - 2026-09-01: Added.
func (c *Client) QuoteExactInputs(ctx context.Context, poolAddress onchainSolana.Address, requests []ExactInputRequest) ([]Quote, error) {
	batch, err := c.QuoteExactInputsWithSlot(ctx, poolAddress, requests)
	if err != nil {
		return nil, err
	}
	return batch.Quotes, nil
}

// QuoteExactInputsWithSlot returns exact-input quotes and their shared RPC context slot.
//
// Parameters:
//   - ctx: quote context; nil uses context.Background.
//   - poolAddress: configured pool address.
//   - requests: exact-input requests evaluated against the same account snapshot.
//
// Returns:
//   - Quote batch containing the shared slot and quotes in request order.
//   - Snapshot or quote error.
//
// Version:
//   - 2026-09-01: Added.
func (c *Client) QuoteExactInputsWithSlot(ctx context.Context, poolAddress onchainSolana.Address, requests []ExactInputRequest) (QuoteBatch, error) {
	if c == nil || c.snapshots == nil {
		return QuoteBatch{}, fmt.Errorf("failed to quote raydium cpmm exact inputs with slot: account_snapshot_provider=null")
	}
	if len(requests) == 0 {
		return QuoteBatch{}, fmt.Errorf("failed to quote raydium cpmm exact inputs with slot: requests=empty")
	}
	pool, exists := c.pools[poolAddress]
	if !exists {
		return QuoteBatch{}, fmt.Errorf("failed to quote raydium cpmm exact inputs with slot: pool=invalid pool_address=%q", poolAddress.String())
	}
	for index, request := range requests {
		if request.AmountIn == 0 {
			return QuoteBatch{}, fmt.Errorf("failed to quote raydium cpmm exact inputs with slot: amount_in=empty request_index=%d", index)
		}
		if request.InputMint != pool.Token0Mint && request.InputMint != pool.Token1Mint {
			return QuoteBatch{}, fmt.Errorf("failed to quote raydium cpmm exact inputs with slot: input_mint=invalid request_index=%d", index)
		}
	}
	if ctx == nil {
		ctx = context.Background()
	}
	addresses := []onchainSolana.Address{pool.Address, pool.AMMConfig, pool.Token0Vault, pool.Token1Vault}
	snapshot, err := c.snapshots.AccountSnapshot(ctx, addresses)
	if err != nil {
		return QuoteBatch{}, fmt.Errorf("failed to quote raydium cpmm exact inputs with slot: %w", err)
	}
	accounts, err := newSnapshotAccounts(addresses, snapshot)
	if err != nil {
		return QuoteBatch{}, fmt.Errorf("failed to quote raydium cpmm exact inputs with slot: %w", err)
	}
	local := &Client{accounts: accounts, programID: c.programID, pools: c.pools, poolOrder: c.poolOrder}
	quotes := make([]Quote, len(requests))
	for index, request := range requests {
		quotes[index], err = local.quoteExactInput(ctx, poolAddress, request.InputMint, request.AmountIn)
		if err != nil {
			return QuoteBatch{}, fmt.Errorf("failed to quote raydium cpmm exact inputs with slot: %w: request_index=%d snapshot_slot=%d", err, index, snapshot.Slot)
		}
	}
	return QuoteBatch{Slot: snapshot.Slot, Quotes: quotes}, nil
}

func (c *Client) quoteExactInput(ctx context.Context, poolAddress, inputMint onchainSolana.Address, amountIn uint64) (Quote, error) {
	if c == nil || c.accounts == nil {
		return Quote{}, fmt.Errorf("failed to quote raydium cpmm exact input: client=null")
	}
	if amountIn == 0 {
		return Quote{}, fmt.Errorf("failed to quote raydium cpmm exact input: amount_in=empty")
	}
	_, ok := c.pools[poolAddress]
	if !ok {
		return Quote{}, fmt.Errorf("failed to quote raydium cpmm exact input: pool=not_found pool_address=%q", poolAddress.String())
	}
	if ctx == nil {
		ctx = context.Background()
	}
	poolAccount, err := c.accounts.Account(ctx, poolAddress)
	if err != nil {
		return Quote{}, fmt.Errorf("failed to quote raydium cpmm exact input: failed to refresh pool: %w", err)
	}
	if poolAccount == nil {
		return Quote{}, fmt.Errorf("failed to quote raydium cpmm exact input: pool_account=null")
	}
	if poolAccount.Owner != c.programID {
		return Quote{}, fmt.Errorf("failed to quote raydium cpmm exact input: pool_owner=invalid")
	}
	pool, err := decodePool(poolAddress, poolAccount.Data)
	if err != nil {
		return Quote{}, fmt.Errorf("failed to quote raydium cpmm exact input: %w", err)
	}
	if pool.Status&4 != 0 {
		return Quote{}, fmt.Errorf("failed to quote raydium cpmm exact input: pool swap disabled: pool_address=%q", poolAddress.String())
	}
	configAccount, err := c.accounts.Account(ctx, pool.AMMConfig)
	if err != nil {
		return Quote{}, fmt.Errorf("failed to quote raydium cpmm exact input: failed to get amm config: %w", err)
	}
	if configAccount == nil {
		return Quote{}, fmt.Errorf("failed to quote raydium cpmm exact input: amm_config_account=null")
	}
	if configAccount.Owner != c.programID {
		return Quote{}, fmt.Errorf("failed to quote raydium cpmm exact input: amm_config_owner=invalid")
	}
	config, err := decodeAMMConfig(configAccount)
	if err != nil {
		return Quote{}, fmt.Errorf("failed to quote raydium cpmm exact input: %w", err)
	}
	zeroForOne := inputMint == pool.Token0Mint
	if !zeroForOne && inputMint != pool.Token1Mint {
		return Quote{}, fmt.Errorf("failed to quote raydium cpmm exact input: input_mint=invalid")
	}
	inputVault, outputVault, outputMint := pool.Token0Vault, pool.Token1Vault, pool.Token1Mint
	fees0, ok := checkedSum(pool.ProtocolFees0, pool.FundFees0, pool.CreatorFees0)
	if !ok {
		return Quote{}, fmt.Errorf("failed to quote raydium cpmm exact input: token_0_fees=out_of_range")
	}
	fees1, ok := checkedSum(pool.ProtocolFees1, pool.FundFees1, pool.CreatorFees1)
	if !ok {
		return Quote{}, fmt.Errorf("failed to quote raydium cpmm exact input: token_1_fees=out_of_range")
	}
	feesIn, feesOut := fees0, fees1
	if !zeroForOne {
		inputVault, outputVault, outputMint, feesIn, feesOut = pool.Token1Vault, pool.Token0Vault, pool.Token0Mint, feesOut, feesIn
	}
	inAccount, err := c.accounts.Account(ctx, inputVault)
	if err != nil {
		return Quote{}, fmt.Errorf("failed to quote raydium cpmm exact input: failed to get input vault: %w", err)
	}
	outAccount, err := c.accounts.Account(ctx, outputVault)
	if err != nil {
		return Quote{}, fmt.Errorf("failed to quote raydium cpmm exact input: failed to get output vault: %w", err)
	}
	inAmount, err := decodeTokenAmount(inAccount)
	if err != nil {
		return Quote{}, fmt.Errorf("failed to quote raydium cpmm exact input: %w", err)
	}
	outAmount, err := decodeTokenAmount(outAccount)
	if err != nil {
		return Quote{}, fmt.Errorf("failed to quote raydium cpmm exact input: %w", err)
	}
	if inAmount < feesIn || outAmount < feesOut {
		return Quote{}, fmt.Errorf("failed to quote raydium cpmm exact input: vault_amount=invalid")
	}
	creatorRate := uint64(0)
	if pool.CreatorFeeEnabled {
		creatorRate = config.creatorFeeRate
	}
	creatorOnInput := pool.CreatorFeeOn == 0 || (pool.CreatorFeeOn == 1 && zeroForOne) || (pool.CreatorFeeOn == 2 && !zeroForOne)
	amountOut, tradeFee, creatorFee, err := quote(amountIn, inAmount-feesIn, outAmount-feesOut, config.tradeFeeRate, creatorRate, creatorOnInput)
	if err != nil {
		return Quote{}, fmt.Errorf("failed to quote raydium cpmm exact input: %w", err)
	}
	return Quote{PoolAddress: poolAddress, InputMint: inputMint, OutputMint: outputMint, AmountIn: amountIn, AmountOut: amountOut, TradeFee: tradeFee, CreatorFee: creatorFee}, nil
}

type snapshotAccounts map[onchainSolana.Address]*onchainSolana.Account

func newSnapshotAccounts(addresses []onchainSolana.Address, snapshot *onchainSolana.AccountSnapshot) (snapshotAccounts, error) {
	if snapshot == nil || len(snapshot.Accounts) != len(addresses) {
		return nil, fmt.Errorf("failed to create raydium cpmm snapshot accounts: snapshot=invalid")
	}
	result := make(snapshotAccounts, len(addresses))
	for index, account := range snapshot.Accounts {
		if account != nil {
			result[addresses[index]] = account
		}
	}
	return result, nil
}

func (s snapshotAccounts) Account(_ context.Context, address onchainSolana.Address) (*onchainSolana.Account, error) {
	return s[address], nil
}

func quote(amountIn, reserveIn, reserveOut, tradeRate, creatorRate uint64, creatorOnInput bool) (uint64, uint64, uint64, error) {
	if reserveIn == 0 || reserveOut == 0 || tradeRate >= feeDenominator || creatorRate >= feeDenominator || tradeRate > feeDenominator-creatorRate {
		return 0, 0, 0, fmt.Errorf("failed to calculate raydium cpmm quote: reserves_or_fees=invalid")
	}
	totalRate := tradeRate
	if creatorOnInput {
		totalRate += creatorRate
	}
	totalFee, ok := ceilMulDiv(amountIn, totalRate, feeDenominator)
	if !ok || totalFee >= amountIn {
		return 0, 0, 0, fmt.Errorf("failed to calculate raydium cpmm quote: amount_after_fees=empty")
	}
	creatorFee := uint64(0)
	tradeFee := totalFee
	if creatorOnInput && creatorRate > 0 {
		creatorFee = uint64(new(big.Int).Div(new(big.Int).Mul(new(big.Int).SetUint64(totalFee), new(big.Int).SetUint64(creatorRate)), new(big.Int).SetUint64(totalRate)).Uint64())
		tradeFee -= creatorFee
	}
	effective := amountIn - totalFee
	n := new(big.Int).Mul(new(big.Int).SetUint64(effective), new(big.Int).SetUint64(reserveOut))
	d := new(big.Int).Add(new(big.Int).SetUint64(reserveIn), new(big.Int).SetUint64(effective))
	raw := new(big.Int).Div(n, d)
	if !raw.IsUint64() || raw.Sign() == 0 {
		return 0, 0, 0, fmt.Errorf("failed to calculate raydium cpmm quote: amount_out=invalid")
	}
	amountOut := raw.Uint64()
	if !creatorOnInput && creatorRate > 0 {
		creatorFee, ok = ceilMulDiv(amountOut, creatorRate, feeDenominator)
		if !ok || creatorFee >= amountOut {
			return 0, 0, 0, fmt.Errorf("failed to calculate raydium cpmm quote: output_after_creator_fee=empty")
		}
		amountOut -= creatorFee
	}
	return amountOut, tradeFee, creatorFee, nil
}

func ceilMulDiv(value, numerator, denominator uint64) (uint64, bool) {
	if denominator == 0 {
		return 0, false
	}
	n := new(big.Int).Mul(new(big.Int).SetUint64(value), new(big.Int).SetUint64(numerator))
	n.Add(n, new(big.Int).SetUint64(denominator-1))
	n.Div(n, new(big.Int).SetUint64(denominator))
	return n.Uint64(), n.IsUint64()
}

func checkedSum(values ...uint64) (uint64, bool) {
	var result uint64
	for _, value := range values {
		if value > ^uint64(0)-result {
			return 0, false
		}
		result += value
	}
	return result, true
}

func decodePool(address onchainSolana.Address, data []byte) (Pool, error) {
	if len(data) != poolDataLength {
		return Pool{}, fmt.Errorf("failed to decode raydium cpmm pool: data_length=invalid actual_length=%d expected_length=%d", len(data), poolDataLength)
	}
	if !bytes.Equal(data[:8], poolDiscriminator[:]) {
		return Pool{}, fmt.Errorf("failed to decode raydium cpmm pool: discriminator=invalid")
	}
	p := Pool{Address: address}
	o := 8
	readAddress := func() onchainSolana.Address { var a onchainSolana.Address; copy(a[:], data[o:o+32]); o += 32; return a }
	p.AMMConfig = readAddress()
	_ = readAddress()
	p.Token0Vault = readAddress()
	p.Token1Vault = readAddress()
	_ = readAddress()
	p.Token0Mint = readAddress()
	p.Token1Mint = readAddress()
	p.Token0Program = readAddress()
	p.Token1Program = readAddress()
	_ = readAddress()
	o++ // auth bump
	p.Status = data[o]
	o++
	o++ // lp decimals
	p.Token0Decimals = data[o]
	o++
	p.Token1Decimals = data[o]
	o++
	o += 8 // lp supply
	readU64 := func() uint64 { v := binary.LittleEndian.Uint64(data[o : o+8]); o += 8; return v }
	p.ProtocolFees0 = readU64()
	p.ProtocolFees1 = readU64()
	p.FundFees0 = readU64()
	p.FundFees1 = readU64()
	o += 16
	p.CreatorFeeOn = data[o]
	o++
	p.CreatorFeeEnabled = data[o] != 0
	o += 7
	p.CreatorFees0 = readU64()
	p.CreatorFees1 = readU64()
	if p.AMMConfig.IsZero() || p.Token0Vault.IsZero() || p.Token1Vault.IsZero() || p.Token0Mint.IsZero() || p.Token1Mint.IsZero() {
		return Pool{}, fmt.Errorf("failed to decode raydium cpmm pool: required_address=empty")
	}
	if p.CreatorFeeOn > 2 {
		return Pool{}, fmt.Errorf("failed to decode raydium cpmm pool: creator_fee_on=invalid")
	}
	return p, nil
}

func decodeAMMConfig(account *onchainSolana.Account) (ammConfig, error) {
	if account == nil {
		return ammConfig{}, fmt.Errorf("failed to decode raydium cpmm amm config: account=null")
	}
	if len(account.Data) != ammConfigDataLength {
		return ammConfig{}, fmt.Errorf("failed to decode raydium cpmm amm config: data_length=invalid actual_length=%d expected_length=%d", len(account.Data), ammConfigDataLength)
	}
	if !bytes.Equal(account.Data[:8], configDiscriminator[:]) {
		return ammConfig{}, fmt.Errorf("failed to decode raydium cpmm amm config: discriminator=invalid")
	}
	d := account.Data
	o := 12
	c := ammConfig{tradeFeeRate: binary.LittleEndian.Uint64(d[o : o+8]), protocolFeeRate: binary.LittleEndian.Uint64(d[o+8 : o+16]), fundFeeRate: binary.LittleEndian.Uint64(d[o+16 : o+24]), creatorFeeRate: binary.LittleEndian.Uint64(d[108:116])}
	return c, nil
}
func decodeTokenAmount(account *onchainSolana.Account) (uint64, error) {
	if account == nil {
		return 0, fmt.Errorf("failed to decode spl token account: account=null")
	}
	if len(account.Data) < tokenAccountMinSize {
		return 0, fmt.Errorf("failed to decode spl token account: data_length=too_short actual_length=%d min_length=%d", len(account.Data), tokenAccountMinSize)
	}
	return binary.LittleEndian.Uint64(account.Data[tokenAmountOffset : tokenAmountOffset+8]), nil
}
func mustAddress(value string) onchainSolana.Address {
	address, err := onchainSolana.ParseAddress(value)
	if err != nil {
		panic(err)
	}
	return address
}
