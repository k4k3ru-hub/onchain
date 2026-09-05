package clmm

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"math/big"
	"slices"
	"sync"

	solanaSDK "github.com/gagliardetto/solana-go"
	onchainSolana "github.com/k4k3ru-hub/onchain/go/solana"
)

const (
	poolDataLength      = 1544
	ammConfigDataLength = 117
	tickArrayDataLength = 10240
	tickArraySize       = 60
	tickStateDataLength = 168
	bitmapCenter        = 512
)

var (
	mainnetProgramID       = mustAddress("CAMMCzo5YL8w4VFF8KVHrK22GGUsp5VTaW7grrKgrWqK")
	poolDiscriminator      = [8]byte{0xf7, 0xed, 0xe3, 0xf5, 0xd7, 0xc3, 0xde, 0x46}
	configDiscriminator    = [8]byte{0xda, 0xf4, 0x21, 0x68, 0xcb, 0xcb, 0x2b, 0x6f}
	tickArrayDiscriminator = [8]byte{0xc0, 0x9b, 0x55, 0xcd, 0x31, 0xf9, 0x81, 0x2a}
)

type accountProvider interface {
	Account(context.Context, onchainSolana.Address) (*onchainSolana.Account, error)
}

type accountSnapshotProvider interface {
	AccountSnapshot(context.Context, []onchainSolana.Address) (*onchainSolana.AccountSnapshot, error)
}

type Config struct {
	// Array counts are total unique arrays across requested directions; zero uses defaults 5/16.
	InitialArrayCount int
	MaxArrayCount     int
	ProgramID         onchainSolana.Address
	Pools             []onchainSolana.Address
}

type Client struct {
	initialArrayCount int
	maxArrayCount     int
	accounts          accountProvider
	snapshots         accountSnapshotProvider
	programID         onchainSolana.Address
	pools             map[onchainSolana.Address]Pool
	configs           map[onchainSolana.Address]AMMConfig
	poolOrder         []onchainSolana.Address
	quotePools        sync.Map
}

type Pool struct {
	Address         onchainSolana.Address
	AMMConfig       onchainSolana.Address
	Owner           onchainSolana.Address
	Token0Mint      onchainSolana.Address
	Token1Mint      onchainSolana.Address
	Token0Vault     onchainSolana.Address
	Token1Vault     onchainSolana.Address
	Observation     onchainSolana.Address
	Token0Decimals  uint8
	Token1Decimals  uint8
	TickSpacing     uint16
	Liquidity       [16]byte
	SqrtPriceX64    [16]byte
	CurrentTick     int32
	Status          uint8
	FeeOn           uint8
	OpenTime        uint64
	RecentEpoch     uint64
	TickArrayBitmap [16]uint64
	DynamicFee      bool
}

type AMMConfig struct {
	Address         onchainSolana.Address
	Bump            uint8
	Index           uint16
	Owner           onchainSolana.Address
	ProtocolFeeRate uint32
	TradeFeeRate    uint32
	TickSpacing     uint16
	FundFeeRate     uint32
	FundOwner       onchainSolana.Address
}

type TickArray struct {
	Address              onchainSolana.Address
	Pool                 onchainSolana.Address
	StartTickIndex       int32
	Ticks                [tickArraySize]Tick
	InitializedTickCount uint8
	RecentEpoch          uint64
}

type Tick struct {
	Index                     int32
	LiquidityNet              [16]byte
	LiquidityGross            [16]byte
	FeeGrowthOutside0X64      [16]byte
	FeeGrowthOutside1X64      [16]byte
	RewardGrowthsOutsideX64   [3][16]byte
	OrderPhase                uint64
	OrdersAmount              uint64
	PartFilledOrdersRemaining uint64
	UnfilledRatioX64          [16]byte
}

type Quote struct {
	Pool           onchainSolana.Address
	InputMint      onchainSolana.Address
	OutputMint     onchainSolana.Address
	AmountIn       uint64
	AmountOut      uint64
	TradeFee       uint64
	SqrtPriceX64   [16]byte
	EndTick        int32
	TickArraysUsed int
}

type ExactInputRequest struct {
	InputMint onchainSolana.Address
	AmountIn  uint64
}

type QuoteBatch struct {
	Slot   onchainSolana.Slot
	Quotes []Quote
}

// MainnetProgramAddress returns the Raydium CLMM mainnet program address.
//
// Returns:
//   - Raydium CLMM mainnet program address.
//
// Version:
//   - 2026-08-31: Added.
func MainnetProgramAddress() onchainSolana.Address { return mainnetProgramID }

// NewClient discovers and validates configured Raydium CLMM pools.
//
// Parameters:
//   - ctx: construction context; nil uses context.Background.
//   - accounts: Solana account provider.
//   - config: CLMM program and configured pool addresses.
//
// Returns:
//   - Raydium CLMM client.
//   - Client creation error.
//
// Version:
//   - 2026-09-05: Added bounded adaptive array snapshots.
//   - 2026-08-31: Added.
func NewClient(ctx context.Context, accounts accountProvider, config Config) (*Client, error) {
	if accounts == nil {
		return nil, fmt.Errorf("failed to create raydium clmm client: accounts=null")
	}
	initial, maximum, err := onchainSolana.NormalizeQuoteArrayCounts(config.InitialArrayCount, config.MaxArrayCount)
	if err != nil {
		return nil, fmt.Errorf("failed to create raydium clmm client: %w", err)
	}
	if config.ProgramID.IsZero() {
		config.ProgramID = mainnetProgramID
	}
	if len(config.Pools) == 0 {
		return nil, fmt.Errorf("failed to create raydium clmm client: pools=empty")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	client := &Client{initialArrayCount: initial, maxArrayCount: maximum,
		accounts: accounts, programID: config.ProgramID,
		pools:     make(map[onchainSolana.Address]Pool, len(config.Pools)),
		configs:   make(map[onchainSolana.Address]AMMConfig),
		poolOrder: make([]onchainSolana.Address, 0, len(config.Pools)),
	}
	client.snapshots, _ = accounts.(accountSnapshotProvider)
	for i, address := range config.Pools {
		if address.IsZero() {
			return nil, fmt.Errorf("failed to create raydium clmm client: pool=empty pool_index=%d", i)
		}
		if _, exists := client.pools[address]; exists {
			return nil, fmt.Errorf("failed to create raydium clmm client: pool=invalid duplicate=true pool_index=%d", i)
		}
		account, err := accounts.Account(ctx, address)
		if err != nil {
			return nil, fmt.Errorf("failed to create raydium clmm client: failed to discover pool: %w: pool_address=%q", err, address.String())
		}
		if account == nil {
			return nil, fmt.Errorf("failed to create raydium clmm client: pool_account=null pool_address=%q", address.String())
		}
		if account.Owner != config.ProgramID {
			return nil, fmt.Errorf("failed to create raydium clmm client: pool_owner=invalid pool_address=%q", address.String())
		}
		pool, err := decodePool(address, account.Data)
		if err != nil {
			return nil, fmt.Errorf("failed to create raydium clmm client: %w: pool_address=%q", err, address.String())
		}
		ammConfig, exists := client.configs[pool.AMMConfig]
		if !exists {
			configAccount, accountErr := accounts.Account(ctx, pool.AMMConfig)
			if accountErr != nil {
				return nil, fmt.Errorf("failed to create raydium clmm client: failed to discover amm config: %w: config_address=%q", accountErr, pool.AMMConfig.String())
			}
			if configAccount == nil {
				return nil, fmt.Errorf("failed to create raydium clmm client: config_account=null config_address=%q", pool.AMMConfig.String())
			}
			if configAccount.Owner != config.ProgramID {
				return nil, fmt.Errorf("failed to create raydium clmm client: config_owner=invalid config_address=%q", pool.AMMConfig.String())
			}
			ammConfig, err = decodeAMMConfig(pool.AMMConfig, configAccount.Data)
			if err != nil {
				return nil, fmt.Errorf("failed to create raydium clmm client: %w: config_address=%q", err, pool.AMMConfig.String())
			}
			client.configs[pool.AMMConfig] = ammConfig
		}
		if ammConfig.TickSpacing != pool.TickSpacing {
			return nil, fmt.Errorf("failed to create raydium clmm client: tick_spacing=invalid pool_address=%q", address.String())
		}
		client.pools[address] = pool
		client.quotePools.Store(address, pool)
		client.poolOrder = append(client.poolOrder, address)
	}
	return client, nil
}

// AMMConfig returns detached metadata for a discovered CLMM AMM configuration.
//
// Parameters:
//   - address: AMM configuration account address.
//
// Returns:
//   - AMM configuration and whether it was discovered.
//
// Version:
//   - 2026-08-31: Added.
func (c *Client) AMMConfig(address onchainSolana.Address) (AMMConfig, bool) {
	if c == nil {
		return AMMConfig{}, false
	}
	config, exists := c.configs[address]
	return config, exists
}

// TickArraysForQuote discovers initialized standard-bitmap tick arrays in swap direction.
//
// Parameters:
//   - ctx: discovery context; nil uses context.Background.
//   - poolAddress: configured pool address.
//   - inputMint: token mint supplied by the swap.
//   - limit: maximum tick arrays to discover.
//
// Returns:
//   - Initialized tick arrays in traversal order.
//   - Discovery error.
//
// Version:
//   - 2026-08-31: Added.
func (c *Client) TickArraysForQuote(ctx context.Context, poolAddress, inputMint onchainSolana.Address, limit int) ([]TickArray, error) {
	if c == nil {
		return nil, fmt.Errorf("failed to discover raydium clmm tick arrays: client=null")
	}
	pool, exists := c.pools[poolAddress]
	if !exists {
		return nil, fmt.Errorf("failed to discover raydium clmm tick arrays: pool=invalid pool_address=%q", poolAddress.String())
	}
	if limit <= 0 {
		return nil, fmt.Errorf("failed to discover raydium clmm tick arrays: limit=out_of_range min_value=1")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	refreshedPool, err := c.refreshPool(ctx, pool)
	if err != nil {
		return nil, fmt.Errorf("failed to discover raydium clmm tick arrays: %w", err)
	}
	pool = refreshedPool
	zeroForOne := inputMint == pool.Token0Mint
	if !zeroForOne && inputMint != pool.Token1Mint {
		return nil, fmt.Errorf("failed to discover raydium clmm tick arrays: input_mint=invalid input_mint=%q", inputMint.String())
	}
	return c.fetchTickArrays(ctx, pool, zeroForOne, limit)
}

func (c *Client) fetchTickArrays(ctx context.Context, pool Pool, zeroForOne bool, limit int) ([]TickArray, error) {
	starts, _ := initializedTickArrayStarts(pool, zeroForOne, limit)
	result := make([]TickArray, 0, len(starts))
	for _, start := range starts {
		array, err := c.readTickArray(ctx, pool, start)
		if err != nil {
			return nil, err
		}
		result = append(result, array)
	}
	return result, nil
}

func (c *Client) readTickArray(ctx context.Context, pool Pool, start int32) (TickArray, error) {
	address, err := tickArrayAddress(c.programID, pool.Address, start)
	if err != nil {
		return TickArray{}, fmt.Errorf("failed to discover raydium clmm tick arrays: failed to derive tick array address: %w: start_tick_index=%d", err, start)
	}
	account, err := c.accounts.Account(ctx, address)
	if err != nil {
		return TickArray{}, fmt.Errorf("failed to discover raydium clmm tick arrays: failed to fetch tick array: %w: tick_array_address=%q", err, address.String())
	}
	if account == nil {
		return TickArray{}, fmt.Errorf("failed to discover raydium clmm tick arrays: tick_array_account=null tick_array_address=%q", address.String())
	}
	if account.Owner != c.programID {
		return TickArray{}, fmt.Errorf("failed to discover raydium clmm tick arrays: tick_array_owner=invalid tick_array_address=%q", address.String())
	}
	tickArray, err := decodeTickArray(address, account.Data)
	if err != nil {
		return TickArray{}, fmt.Errorf("failed to discover raydium clmm tick arrays: %w: tick_array_address=%q", err, address.String())
	}
	if tickArray.Pool != pool.Address || tickArray.StartTickIndex != start {
		return TickArray{}, fmt.Errorf("failed to discover raydium clmm tick arrays: tick_array_identity=invalid tick_array_address=%q", address.String())
	}
	return tickArray, nil
}

func (c *Client) refreshPool(ctx context.Context, configured Pool) (Pool, error) {
	account, err := c.accounts.Account(ctx, configured.Address)
	if err != nil {
		return Pool{}, fmt.Errorf("failed to refresh raydium clmm pool: %w: pool_address=%q", err, configured.Address.String())
	}
	if account == nil {
		return Pool{}, fmt.Errorf("failed to refresh raydium clmm pool: pool_account=null pool_address=%q", configured.Address.String())
	}
	if account.Owner != c.programID {
		return Pool{}, fmt.Errorf("failed to refresh raydium clmm pool: pool_owner=invalid pool_address=%q", configured.Address.String())
	}
	pool, err := decodePool(configured.Address, account.Data)
	if err != nil {
		return Pool{}, fmt.Errorf("failed to refresh raydium clmm pool: %w: pool_address=%q", err, configured.Address.String())
	}
	if pool.AMMConfig != configured.AMMConfig || pool.Token0Mint != configured.Token0Mint || pool.Token1Mint != configured.Token1Mint {
		return Pool{}, fmt.Errorf("failed to refresh raydium clmm pool: pool_identity=invalid pool_address=%q", configured.Address.String())
	}
	return pool, nil
}

// QuoteExactInput returns a static-fee CLMM quote from validated on-chain state.
//
// Parameters:
//   - ctx: quote context; nil uses context.Background.
//   - poolAddress: configured pool address.
//   - inputMint: input token mint.
//   - amountIn: exact input amount in atomic units.
//
// Returns:
//   - Exact-input quote.
//   - Quote error, including unsupported dynamic-fee or limit-order state.
//
// Version:
//   - 2026-09-05: Added bounded adaptive array snapshots.
//   - 2026-08-31: Added batched account snapshots when supported.
func (c *Client) QuoteExactInput(ctx context.Context, poolAddress, inputMint onchainSolana.Address, amountIn uint64) (Quote, error) {
	if c != nil && c.snapshots != nil {
		quotes, err := c.QuoteExactInputs(ctx, poolAddress, []ExactInputRequest{{InputMint: inputMint, AmountIn: amountIn}})
		if err != nil {
			return Quote{}, err
		}
		return quotes[0], nil
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
//   - 2026-09-05: Added bounded adaptive array snapshots.
//   - 2026-09-01: Delegated to the slot-aware quote batch API.
//   - 2026-08-31: Added.
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
//   - 2026-09-05: Added bounded adaptive array snapshots.
//   - 2026-09-01: Included pool refresh in the shared account snapshot.
//   - 2026-09-01: Added.
func (c *Client) QuoteExactInputsWithSlot(ctx context.Context, poolAddress onchainSolana.Address, requests []ExactInputRequest) (QuoteBatch, error) {
	if c == nil || c.snapshots == nil {
		return QuoteBatch{}, fmt.Errorf("failed to quote raydium clmm exact inputs with slot: account_snapshot_provider=null")
	}
	if len(requests) == 0 {
		return QuoteBatch{}, fmt.Errorf("failed to quote raydium clmm exact inputs with slot: requests=empty")
	}
	configured, exists := c.pools[poolAddress]
	if !exists {
		return QuoteBatch{}, fmt.Errorf("failed to quote raydium clmm exact inputs with slot: pool=invalid pool_address=%q", poolAddress.String())
	}
	if ctx == nil {
		ctx = context.Background()
	}
	initial, maximum, err := onchainSolana.NormalizeQuoteArrayCounts(c.initialArrayCount, c.maxArrayCount)
	if err != nil {
		return QuoteBatch{}, err
	}
	pool := configured
	if value, ok := c.quotePools.Load(poolAddress); ok {
		pool = value.(Pool)
	}
	var selected []onchainSolana.Address
	for attempt := 0; attempt < 5; attempt++ {
		if err := ctx.Err(); err != nil {
			return QuoteBatch{}, fmt.Errorf("failed to quote raydium clmm exact inputs with slot: %w", err)
		}
		base, candidates, err := c.quoteArrayCandidates(pool, requests)
		if err != nil {
			return QuoteBatch{}, err
		}
		selected = onchainSolana.SelectQuoteArrays(candidates, selected, initial)
		if len(selected) > maximum {
			return QuoteBatch{}, fmt.Errorf("failed to quote raydium clmm exact inputs with slot: %w: max_array_count=%d", onchainSolana.ErrQuoteArrayLimit, maximum)
		}
		addresses := append(base, selected...)
		snapshot, err := c.snapshots.AccountSnapshot(ctx, addresses)
		if err != nil {
			return QuoteBatch{}, fmt.Errorf("failed to quote raydium clmm exact inputs with slot: %w", err)
		}
		accounts, err := newSnapshotAccounts(addresses, snapshot)
		if err != nil {
			return QuoteBatch{}, err
		}
		local := &Client{accounts: accounts, programID: c.programID, pools: c.pools, configs: c.configs, poolOrder: c.poolOrder}
		refreshed, err := local.refreshPool(ctx, configured)
		if err != nil {
			return QuoteBatch{}, err
		}
		pool = refreshed
		quotes := make([]Quote, len(requests))
		var needed []onchainSolana.Address
		for index, request := range requests {
			quotes[index], err = local.quoteExactInput(ctx, poolAddress, request.InputMint, request.AmountIn)
			if err != nil {
				var missing *onchainSolana.QuoteArrayRequiredError
				if !errors.As(err, &missing) {
					return QuoteBatch{}, fmt.Errorf("failed to quote raydium clmm exact inputs with slot: %w: request_index=%d snapshot_slot=%d", err, index, snapshot.Slot)
				}
				needed = append(needed, missing.Address)
			}
		}
		if len(needed) == 0 {
			c.quotePools.Store(poolAddress, refreshed)
			return QuoteBatch{Slot: snapshot.Slot, Quotes: quotes}, nil
		}
		// Recenter on refreshed state. Old initial-window arrays are not expansion hints.
		retained := make([]onchainSolana.Address, 0, len(selected))
		for _, address := range selected {
			if !slices.Contains(candidates[:min(initial, len(candidates))], address) {
				retained = append(retained, address)
			}
		}
		_, candidates, err = c.quoteArrayCandidates(pool, requests)
		if err != nil {
			return QuoteBatch{}, err
		}
		selected = onchainSolana.SelectQuoteArrays(candidates, retained, initial)
		for _, address := range needed {
			if !slices.Contains(selected, address) {
				selected = append(selected, address)
			}
		}
		if len(selected) > maximum {
			return QuoteBatch{}, fmt.Errorf("failed to quote raydium clmm exact inputs with slot: %w: max_array_count=%d", onchainSolana.ErrQuoteArrayLimit, maximum)
		}
	}
	return QuoteBatch{}, fmt.Errorf("failed to quote raydium clmm exact inputs with slot: %w: max_attempts=5", onchainSolana.ErrQuoteSnapshotLimit)
}

// quoteArrayCandidates interleaves directions so the initial budget covers both sides.
func (c *Client) quoteArrayCandidates(pool Pool, requests []ExactInputRequest) ([]onchainSolana.Address, []onchainSolana.Address, error) {
	base := []onchainSolana.Address{pool.Address}
	groups := make([][]onchainSolana.Address, len(requests))
	for i, request := range requests {
		addresses, err := c.quoteSnapshotAddresses(pool, []ExactInputRequest{request})
		if err != nil {
			return nil, nil, err
		}
		groups[i] = addresses[len(base):]
	}
	var candidates []onchainSolana.Address
	for depth := 0; depth < 16; depth++ {
		for _, group := range groups {
			if depth < len(group) && !slices.Contains(candidates, group[depth]) {
				candidates = append(candidates, group[depth])
			}
		}
	}
	return base, candidates, nil
}

func (c *Client) quoteSnapshotAddresses(pool Pool, requests []ExactInputRequest) ([]onchainSolana.Address, error) {
	addresses := []onchainSolana.Address{pool.Address}
	seen := map[onchainSolana.Address]struct{}{pool.Address: {}}
	for index, request := range requests {
		if request.AmountIn == 0 {
			return nil, fmt.Errorf("failed to quote raydium clmm exact inputs with slot: amount_in=empty request_index=%d", index)
		}
		zeroForOne := request.InputMint == pool.Token0Mint
		if !zeroForOne && request.InputMint != pool.Token1Mint {
			return nil, fmt.Errorf("failed to quote raydium clmm exact inputs with slot: input_mint=invalid request_index=%d", index)
		}
		starts, _ := initializedTickArrayStarts(pool, zeroForOne, 16)
		for _, start := range starts {
			address, addressErr := tickArrayAddress(c.programID, pool.Address, start)
			if addressErr != nil {
				return nil, fmt.Errorf("failed to quote raydium clmm exact inputs with slot: failed to derive tick array address: %w: start_tick_index=%d", addressErr, start)
			}
			if _, exists := seen[address]; !exists {
				seen[address] = struct{}{}
				addresses = append(addresses, address)
			}
		}
	}
	return addresses, nil
}

func (c *Client) quoteExactInput(ctx context.Context, poolAddress, inputMint onchainSolana.Address, amountIn uint64) (Quote, error) {
	if c == nil {
		return Quote{}, fmt.Errorf("failed to quote raydium clmm exact input: client=null")
	}
	if amountIn == 0 {
		return Quote{}, fmt.Errorf("failed to quote raydium clmm exact input: amount_in=empty")
	}
	configured, exists := c.pools[poolAddress]
	if !exists {
		return Quote{}, fmt.Errorf("failed to quote raydium clmm exact input: pool=invalid pool_address=%q", poolAddress.String())
	}
	if ctx == nil {
		ctx = context.Background()
	}
	pool, err := c.refreshPool(ctx, configured)
	if err != nil {
		return Quote{}, fmt.Errorf("failed to quote raydium clmm exact input: %w", err)
	}
	zeroForOne := inputMint == pool.Token0Mint
	if !zeroForOne && inputMint != pool.Token1Mint {
		return Quote{}, fmt.Errorf("failed to quote raydium clmm exact input: input_mint=invalid input_mint=%q", inputMint.String())
	}
	starts, _ := initializedTickArrayStarts(pool, zeroForOne, 16)
	if pool.DynamicFee {
		return Quote{}, fmt.Errorf("failed to quote raydium clmm exact input: dynamic_fee=unsupported pool_address=%q", poolAddress.String())
	}
	config := c.configs[pool.AMMConfig]
	outputMint := pool.Token0Mint
	if zeroForOne {
		outputMint = pool.Token1Mint
	}
	feeOnInput := pool.FeeOn == 0 || (pool.FeeOn == 1 && zeroForOne) || (pool.FeeOn == 2 && !zeroForOne)
	liquidity := unsigned128LE(pool.Liquidity)
	price := unsigned128LE(pool.SqrtPriceX64)
	currentTick := pool.CurrentTick
	remaining := amountIn
	var amountOut, tradeFee uint64
	arraysUsed := 0
	for arrayIndex, start := range starts {
		array, err := c.readTickArray(ctx, pool, start)
		if err != nil {
			return Quote{}, fmt.Errorf("failed to quote raydium clmm exact input: %w", err)
		}
		arraysUsed = arrayIndex + 1
		indices := initializedTicks(array, currentTick, zeroForOne)
		for _, tick := range indices {
			target, targetErr := sqrtPriceAtTick(tick.Index)
			if targetErr != nil {
				return Quote{}, fmt.Errorf("failed to quote raydium clmm exact input: %w", targetErr)
			}
			step, stepErr := computeExactInputStep(price, target, liquidity, remaining, config.TradeFeeRate, zeroForOne, feeOnInput)
			if stepErr != nil {
				return Quote{}, fmt.Errorf("failed to quote raydium clmm exact input: %w", stepErr)
			}
			consumed := step.amountIn
			if feeOnInput {
				if consumed > ^uint64(0)-step.feeAmount {
					return Quote{}, fmt.Errorf("failed to quote raydium clmm exact input: amount=out_of_range")
				}
				consumed += step.feeAmount
			}
			if consumed > remaining || amountOut > ^uint64(0)-step.amountOut || tradeFee > ^uint64(0)-step.feeAmount {
				return Quote{}, fmt.Errorf("failed to quote raydium clmm exact input: amount=out_of_range")
			}
			remaining -= consumed
			amountOut += step.amountOut
			tradeFee += step.feeAmount
			price = step.sqrtPriceNext
			if remaining == 0 && price.Cmp(target) != 0 {
				endTick, tickErr := tickAtSqrtPrice(price)
				if tickErr != nil {
					return Quote{}, fmt.Errorf("failed to quote raydium clmm exact input: %w", tickErr)
				}
				return makeQuote(poolAddress, inputMint, outputMint, amountIn, amountOut, tradeFee, price, endTick, arraysUsed), nil
			}
			if price.Cmp(target) != 0 {
				return Quote{}, fmt.Errorf("failed to quote raydium clmm exact input: swap_progress=invalid")
			}
			hasLimitOrders := tick.OrdersAmount != 0 || tick.PartFilledOrdersRemaining != 0
			if hasLimitOrders {
				match, matchErr := computeExactInputLimitOrderMatch(target, remaining, tick.OrdersAmount, tick.PartFilledOrdersRemaining, config.TradeFeeRate, zeroForOne, feeOnInput)
				if matchErr != nil {
					return Quote{}, fmt.Errorf("failed to quote raydium clmm exact input: %w: tick=%d", matchErr, tick.Index)
				}
				limitConsumed := match.amountIn
				if feeOnInput {
					if limitConsumed > ^uint64(0)-match.feeAmount {
						return Quote{}, fmt.Errorf("failed to quote raydium clmm exact input: amount=out_of_range")
					}
					limitConsumed += match.feeAmount
				}
				if limitConsumed > remaining || amountOut > ^uint64(0)-match.amountOut || tradeFee > ^uint64(0)-match.feeAmount {
					return Quote{}, fmt.Errorf("failed to quote raydium clmm exact input: amount=out_of_range")
				}
				remaining -= limitConsumed
				amountOut += match.amountOut
				tradeFee += match.feeAmount
				if match.ordersRemain {
					if zeroForOne {
						currentTick = tick.Index
					} else {
						currentTick = tick.Index - 1
					}
					if remaining != 0 {
						return Quote{}, fmt.Errorf("failed to quote raydium clmm exact input: limit_order_progress=invalid tick=%d", tick.Index)
					}
					return makeQuote(poolAddress, inputMint, outputMint, amountIn, amountOut, tradeFee, price, currentTick, arraysUsed), nil
				}
			}
			delta := signed128LE(tick.LiquidityNet)
			if zeroForOne {
				delta.Neg(delta)
			}
			liquidity.Add(liquidity, delta)
			if liquidity.Sign() < 0 || liquidity.BitLen() > 128 {
				return Quote{}, fmt.Errorf("failed to quote raydium clmm exact input: liquidity=out_of_range tick=%d", tick.Index)
			}
			if zeroForOne {
				currentTick = tick.Index - 1
			} else {
				currentTick = tick.Index
			}
			if remaining == 0 {
				return makeQuote(poolAddress, inputMint, outputMint, amountIn, amountOut, tradeFee, price, currentTick, arraysUsed), nil
			}
		}
	}
	return Quote{}, fmt.Errorf("failed to quote raydium clmm exact input: %w: amount_remaining=%d", onchainSolana.ErrQuoteArrayRange, remaining)
}

type snapshotAccounts map[onchainSolana.Address]*onchainSolana.Account

func newSnapshotAccounts(addresses []onchainSolana.Address, snapshot *onchainSolana.AccountSnapshot) (snapshotAccounts, error) {
	if snapshot == nil || len(snapshot.Accounts) != len(addresses) {
		return nil, fmt.Errorf("failed to create raydium clmm snapshot accounts: snapshot=invalid")
	}
	result := make(snapshotAccounts, len(addresses))
	for index, account := range snapshot.Accounts {
		result[addresses[index]] = account
	}
	return result, nil
}

func (s snapshotAccounts) Account(_ context.Context, address onchainSolana.Address) (*onchainSolana.Account, error) {
	account, exists := s[address]
	if !exists {
		return nil, &onchainSolana.QuoteArrayRequiredError{Address: address}
	}
	return account, nil
}

func accountsContain(accounts snapshotAccounts, addresses []onchainSolana.Address) bool {
	for _, address := range addresses {
		if accounts[address] == nil {
			return false
		}
	}
	return true
}

// Pools returns detached metadata for the configured CLMM pools.
//
// Returns:
//   - Configured pools in configuration order.
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

func decodePool(address onchainSolana.Address, data []byte) (Pool, error) {
	if len(data) != poolDataLength {
		return Pool{}, fmt.Errorf("failed to decode raydium clmm pool: data_length=invalid actual_length=%d expected_length=%d", len(data), poolDataLength)
	}
	if !bytes.Equal(data[:8], poolDiscriminator[:]) {
		return Pool{}, fmt.Errorf("failed to decode raydium clmm pool: discriminator=invalid")
	}
	pool := Pool{Address: address}
	copy(pool.AMMConfig[:], data[9:41])
	copy(pool.Owner[:], data[41:73])
	copy(pool.Token0Mint[:], data[73:105])
	copy(pool.Token1Mint[:], data[105:137])
	copy(pool.Token0Vault[:], data[137:169])
	copy(pool.Token1Vault[:], data[169:201])
	copy(pool.Observation[:], data[201:233])
	pool.Token0Decimals = data[233]
	pool.Token1Decimals = data[234]
	pool.TickSpacing = binary.LittleEndian.Uint16(data[235:237])
	copy(pool.Liquidity[:], data[237:253])
	copy(pool.SqrtPriceX64[:], data[253:269])
	pool.CurrentTick = int32(binary.LittleEndian.Uint32(data[269:273]))
	pool.Status = data[389]
	pool.FeeOn = data[390]
	pool.OpenTime = binary.LittleEndian.Uint64(data[1080:1088])
	pool.RecentEpoch = binary.LittleEndian.Uint64(data[1088:1096])
	pool.DynamicFee = !allZero(data[1096:1176])
	for i := range pool.TickArrayBitmap {
		pool.TickArrayBitmap[i] = binary.LittleEndian.Uint64(data[904+i*8 : 912+i*8])
	}
	if pool.AMMConfig.IsZero() || pool.Token0Mint.IsZero() || pool.Token1Mint.IsZero() || pool.Token0Vault.IsZero() || pool.Token1Vault.IsZero() {
		return Pool{}, fmt.Errorf("failed to decode raydium clmm pool: required_address=empty")
	}
	if pool.TickSpacing == 0 {
		return Pool{}, fmt.Errorf("failed to decode raydium clmm pool: tick_spacing=empty")
	}
	if pool.FeeOn > 2 {
		return Pool{}, fmt.Errorf("failed to decode raydium clmm pool: fee_on=invalid")
	}
	return pool, nil
}

func initializedTicks(array TickArray, currentTick int32, zeroForOne bool) []Tick {
	result := make([]Tick, 0, array.InitializedTickCount)
	if zeroForOne {
		for i := len(array.Ticks) - 1; i >= 0; i-- {
			tick := array.Ticks[i]
			if tick.Index <= currentTick && (!allZero(tick.LiquidityGross[:]) || tick.OrdersAmount != 0 || tick.PartFilledOrdersRemaining != 0) {
				result = append(result, tick)
			}
		}
		return result
	}
	for _, tick := range array.Ticks {
		if tick.Index > currentTick && (!allZero(tick.LiquidityGross[:]) || tick.OrdersAmount != 0 || tick.PartFilledOrdersRemaining != 0) {
			result = append(result, tick)
		}
	}
	return result
}

func makeQuote(pool, inputMint, outputMint onchainSolana.Address, amountIn, amountOut, tradeFee uint64, price *big.Int, tick int32, arraysUsed int) Quote {
	quote := Quote{Pool: pool, InputMint: inputMint, OutputMint: outputMint, AmountIn: amountIn, AmountOut: amountOut, TradeFee: tradeFee, EndTick: tick, TickArraysUsed: arraysUsed}
	bigEndian := make([]byte, 16)
	price.FillBytes(bigEndian)
	for i := range bigEndian {
		quote.SqrtPriceX64[len(bigEndian)-1-i] = bigEndian[i]
	}
	return quote
}

func unsigned128LE(value [16]byte) *big.Int { return new(big.Int).SetBytes(reverse16(value[:])) }

func signed128LE(value [16]byte) *big.Int {
	result := unsigned128LE(value)
	if value[15]&0x80 != 0 {
		result.Sub(result, new(big.Int).Lsh(big.NewInt(1), 128))
	}
	return result
}

func reverse16(value []byte) []byte {
	result := make([]byte, len(value))
	for i := range value {
		result[len(value)-1-i] = value[i]
	}
	return result
}

func allZero(value []byte) bool {
	for _, item := range value {
		if item != 0 {
			return false
		}
	}
	return true
}

func decodeAMMConfig(address onchainSolana.Address, data []byte) (AMMConfig, error) {
	if len(data) != ammConfigDataLength {
		return AMMConfig{}, fmt.Errorf("failed to decode raydium clmm amm config: data_length=invalid actual_length=%d expected_length=%d", len(data), ammConfigDataLength)
	}
	if !bytes.Equal(data[:8], configDiscriminator[:]) {
		return AMMConfig{}, fmt.Errorf("failed to decode raydium clmm amm config: discriminator=invalid")
	}
	config := AMMConfig{Address: address, Bump: data[8], Index: binary.LittleEndian.Uint16(data[9:11])}
	copy(config.Owner[:], data[11:43])
	config.ProtocolFeeRate = binary.LittleEndian.Uint32(data[43:47])
	config.TradeFeeRate = binary.LittleEndian.Uint32(data[47:51])
	config.TickSpacing = binary.LittleEndian.Uint16(data[51:53])
	config.FundFeeRate = binary.LittleEndian.Uint32(data[53:57])
	copy(config.FundOwner[:], data[61:93])
	if config.TickSpacing == 0 {
		return AMMConfig{}, fmt.Errorf("failed to decode raydium clmm amm config: tick_spacing=empty")
	}
	return config, nil
}

func initializedTickArrayStarts(pool Pool, zeroForOne bool, limit int) ([]int32, bool) {
	tickCount := int32(pool.TickSpacing) * tickArraySize
	current := floorDiv(pool.CurrentTick, tickCount)
	position := current + bitmapCenter
	step := int32(1)
	if zeroForOne {
		step = -1
	}
	result := make([]int32, 0, limit)
	for position >= 0 && position < 1024 && len(result) < limit {
		word, bit := position/64, uint(position%64)
		if pool.TickArrayBitmap[word]&(uint64(1)<<bit) != 0 {
			result = append(result, (position-bitmapCenter)*tickCount)
		}
		position += step
	}
	return result, position < 0 || position >= 1024
}

func floorDiv(value, divisor int32) int32 {
	quotient := value / divisor
	if value < 0 && value%divisor != 0 {
		quotient--
	}
	return quotient
}

func tickArrayAddress(programID, poolAddress onchainSolana.Address, startTickIndex int32) (onchainSolana.Address, error) {
	seed := make([]byte, 4)
	binary.BigEndian.PutUint32(seed, uint32(startTickIndex))
	address, _, err := solanaSDK.FindProgramAddress(
		[][]byte{[]byte("tick_array"), poolAddress[:], seed},
		solanaSDK.PublicKey(programID),
	)
	if err != nil {
		return onchainSolana.Address{}, fmt.Errorf("failed to find solana program address: %w", err)
	}
	return onchainSolana.Address(address), nil
}

func decodeTickArray(address onchainSolana.Address, data []byte) (TickArray, error) {
	if len(data) != tickArrayDataLength {
		return TickArray{}, fmt.Errorf("failed to decode raydium clmm tick array: data_length=invalid actual_length=%d expected_length=%d", len(data), tickArrayDataLength)
	}
	if !bytes.Equal(data[:8], tickArrayDiscriminator[:]) {
		return TickArray{}, fmt.Errorf("failed to decode raydium clmm tick array: discriminator=invalid")
	}
	result := TickArray{Address: address}
	copy(result.Pool[:], data[8:40])
	result.StartTickIndex = int32(binary.LittleEndian.Uint32(data[40:44]))
	for i := range result.Ticks {
		offset := 44 + i*tickStateDataLength
		tick := &result.Ticks[i]
		tick.Index = int32(binary.LittleEndian.Uint32(data[offset : offset+4]))
		copy(tick.LiquidityNet[:], data[offset+4:offset+20])
		copy(tick.LiquidityGross[:], data[offset+20:offset+36])
		copy(tick.FeeGrowthOutside0X64[:], data[offset+36:offset+52])
		copy(tick.FeeGrowthOutside1X64[:], data[offset+52:offset+68])
		for reward := range tick.RewardGrowthsOutsideX64 {
			copy(tick.RewardGrowthsOutsideX64[reward][:], data[offset+68+reward*16:offset+84+reward*16])
		}
		tick.OrderPhase = binary.LittleEndian.Uint64(data[offset+116 : offset+124])
		tick.OrdersAmount = binary.LittleEndian.Uint64(data[offset+124 : offset+132])
		tick.PartFilledOrdersRemaining = binary.LittleEndian.Uint64(data[offset+132 : offset+140])
		copy(tick.UnfilledRatioX64[:], data[offset+140:offset+156])
	}
	result.InitializedTickCount = data[10124]
	result.RecentEpoch = binary.LittleEndian.Uint64(data[10125:10133])
	return result, nil
}

func mustAddress(value string) onchainSolana.Address {
	address, err := onchainSolana.ParseAddress(value)
	if err != nil {
		panic(err)
	}
	return address
}
