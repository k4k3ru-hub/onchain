package dlmm

// Account layouts in this package conform to Meteora's public DLMM IDL v0.12.0
// and official SDK commit fb02e51ae677bbd18e76543f702dae40632426db.

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"math/big"

	solanaSDK "github.com/gagliardetto/solana-go"
	onchainSolana "github.com/k4k3ru-hub/onchain/go/solana"
)

const (
	lbPairDataLength   = 904
	binArrayDataLength = 10136
	binArraySize       = 70
	binDataLength      = 144
	bitmapCenter       = 512
	limitOrderFeeShare = 5000
)

var (
	mainnetProgramID      = mustAddress("LBUZKhRxPF3XUpBCjp4YzTKgLccjZhTSDM9YuVaPwxo")
	clockSysvarAddress    = mustAddress("SysvarC1ock11111111111111111111111111111111")
	lbPairDiscriminator   = [8]byte{33, 11, 49, 98, 181, 101, 177, 13}
	binArrayDiscriminator = [8]byte{92, 142, 92, 220, 5, 148, 70, 181}
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

type StaticParameters struct {
	BaseFactor               uint16
	FilterPeriod             uint16
	DecayPeriod              uint16
	ReductionFactor          uint16
	VariableFeeControl       uint32
	MaxVolatilityAccumulator uint32
	MinBinID                 int32
	MaxBinID                 int32
	ProtocolShare            uint16
	BaseFeePowerFactor       uint8
	FunctionType             uint8
	CollectFeeMode           uint8
}

type VariableParameters struct {
	VolatilityAccumulator uint32
	VolatilityReference   uint32
	IndexReference        int32
	LastUpdateTimestamp   int64
}

type Pool struct {
	Address               onchainSolana.Address
	Parameters            StaticParameters
	VariableParameters    VariableParameters
	PairType              uint8
	ActiveBinID           int32
	BinStep               uint16
	Status                uint8
	ActivationType        uint8
	TokenXMint            onchainSolana.Address
	TokenYMint            onchainSolana.Address
	ReserveX              onchainSolana.Address
	ReserveY              onchainSolana.Address
	Oracle                onchainSolana.Address
	RewardMints           [2]onchainSolana.Address
	BinArrayBitmap        [16]uint64
	LastUpdatedAt         int64
	ActivationPoint       uint64
	PreActivationDuration uint64
	Creator               onchainSolana.Address
	TokenXProgramFlag     uint8
	TokenYProgramFlag     uint8
	Version               uint8
}

type BinArray struct {
	Address onchainSolana.Address
	Index   int64
	Version uint8
	Pool    onchainSolana.Address
	Bins    [binArraySize]Bin
}

type Bin struct {
	AmountX                       uint64
	AmountY                       uint64
	Price                         [16]byte
	LiquiditySupply               [16]byte
	FulfilledOrderAmountX         uint64
	FulfilledOrderAmountY         uint64
	LimitOrderFeeAskSide          uint64
	LimitOrderFeeBidSide          uint64
	FeeAmountXPerTokenStored      [16]byte
	FeeAmountYPerTokenStored      [16]byte
	OpenOrderAmount               uint64
	TotalProcessingOrderAmount    uint64
	ProcessedOrderRemainingAmount uint64
	OrderAge                      uint32
	LimitOrderAskSide             uint8
}

type Quote struct {
	Pool          onchainSolana.Address
	InputMint     onchainSolana.Address
	OutputMint    onchainSolana.Address
	AmountIn      uint64
	AmountOut     uint64
	TradeFee      uint64
	ProtocolFee   uint64
	EndActiveBin  int32
	BinArraysUsed int
}

type ExactInputRequest struct {
	InputMint onchainSolana.Address
	AmountIn  uint64
}

type Client struct {
	accounts  accountProvider
	snapshots accountSnapshotProvider
	programID onchainSolana.Address
	pools     map[onchainSolana.Address]Pool
	poolOrder []onchainSolana.Address
}

// MainnetProgramAddress returns the Meteora DLMM mainnet program address.
//
// Returns:
//   - Meteora DLMM mainnet program address.
//
// Version:
//   - 2026-08-31: Added.
func MainnetProgramAddress() onchainSolana.Address { return mainnetProgramID }

// NewClient discovers and validates configured Meteora DLMM pools.
//
// Parameters:
//   - ctx: construction context; nil uses context.Background.
//   - accounts: Solana account provider.
//   - config: DLMM program and configured pool addresses.
//
// Returns:
//   - Meteora DLMM client.
//   - Client creation error.
//
// Version:
//   - 2026-08-31: Added.
func NewClient(ctx context.Context, accounts accountProvider, config Config) (*Client, error) {
	if accounts == nil {
		return nil, fmt.Errorf("failed to create meteora dlmm client: accounts=null")
	}
	if config.ProgramID.IsZero() {
		config.ProgramID = mainnetProgramID
	}
	if len(config.Pools) == 0 {
		return nil, fmt.Errorf("failed to create meteora dlmm client: pools=empty")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	client := &Client{accounts: accounts, programID: config.ProgramID, pools: make(map[onchainSolana.Address]Pool, len(config.Pools)), poolOrder: make([]onchainSolana.Address, 0, len(config.Pools))}
	client.snapshots, _ = accounts.(accountSnapshotProvider)
	for index, address := range config.Pools {
		if address.IsZero() {
			return nil, fmt.Errorf("failed to create meteora dlmm client: pool=empty pool_index=%d", index)
		}
		if _, exists := client.pools[address]; exists {
			return nil, fmt.Errorf("failed to create meteora dlmm client: pool=invalid duplicate=true pool_index=%d", index)
		}
		account, err := accounts.Account(ctx, address)
		if err != nil {
			return nil, fmt.Errorf("failed to create meteora dlmm client: failed to discover pool: %w: pool_address=%q", err, address.String())
		}
		if account == nil {
			return nil, fmt.Errorf("failed to create meteora dlmm client: pool_account=null pool_address=%q", address.String())
		}
		if account.Owner != config.ProgramID {
			return nil, fmt.Errorf("failed to create meteora dlmm client: pool_owner=invalid pool_address=%q", address.String())
		}
		pool, err := decodePool(address, account.Data)
		if err != nil {
			return nil, fmt.Errorf("failed to create meteora dlmm client: %w: pool_address=%q", err, address.String())
		}
		client.pools[address] = pool
		client.poolOrder = append(client.poolOrder, address)
	}
	return client, nil
}

// Pools returns detached metadata for configured DLMM pools.
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

// BinArraysForQuote discovers initialized standard-bitmap bin arrays in swap direction.
//
// Parameters:
//   - ctx: discovery context; nil uses context.Background.
//   - poolAddress: configured pool address.
//   - inputMint: token mint supplied by the swap.
//   - limit: maximum bin arrays to discover.
//
// Returns:
//   - Initialized bin arrays in traversal order.
//   - Discovery error.
//
// Version:
//   - 2026-08-31: Added.
func (c *Client) BinArraysForQuote(ctx context.Context, poolAddress, inputMint onchainSolana.Address, limit int) ([]BinArray, error) {
	if c == nil {
		return nil, fmt.Errorf("failed to discover meteora dlmm bin arrays: client=null")
	}
	configured, exists := c.pools[poolAddress]
	if !exists {
		return nil, fmt.Errorf("failed to discover meteora dlmm bin arrays: pool=invalid pool_address=%q", poolAddress.String())
	}
	if limit <= 0 {
		return nil, fmt.Errorf("failed to discover meteora dlmm bin arrays: limit=out_of_range min_value=1")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	pool, err := c.refreshPool(ctx, configured)
	if err != nil {
		return nil, fmt.Errorf("failed to discover meteora dlmm bin arrays: %w", err)
	}
	swapForY := inputMint == pool.TokenXMint
	if !swapForY && inputMint != pool.TokenYMint {
		return nil, fmt.Errorf("failed to discover meteora dlmm bin arrays: input_mint=invalid input_mint=%q", inputMint.String())
	}
	return c.fetchBinArrays(ctx, pool, swapForY, limit)
}

func (c *Client) fetchBinArrays(ctx context.Context, pool Pool, swapForY bool, limit int) ([]BinArray, error) {
	indexes := initializedBinArrayIndexes(pool, swapForY, limit)
	result := make([]BinArray, 0, len(indexes))
	for _, index := range indexes {
		address, addressErr := binArrayAddress(c.programID, pool.Address, int64(index))
		if addressErr != nil {
			return nil, fmt.Errorf("failed to discover meteora dlmm bin arrays: failed to derive bin array address: %w: bin_array_index=%d", addressErr, index)
		}
		binAccount, accountErr := c.accounts.Account(ctx, address)
		if accountErr != nil {
			return nil, fmt.Errorf("failed to discover meteora dlmm bin arrays: failed to fetch bin array: %w: bin_array_address=%q", accountErr, address.String())
		}
		if binAccount == nil || binAccount.Owner != c.programID {
			return nil, fmt.Errorf("failed to discover meteora dlmm bin arrays: bin_array_account=invalid bin_array_address=%q", address.String())
		}
		binArray, decodeErr := decodeBinArray(address, binAccount.Data)
		if decodeErr != nil {
			return nil, fmt.Errorf("failed to discover meteora dlmm bin arrays: %w: bin_array_address=%q", decodeErr, address.String())
		}
		if binArray.Pool != pool.Address || binArray.Index != int64(index) {
			return nil, fmt.Errorf("failed to discover meteora dlmm bin arrays: bin_array_identity=invalid bin_array_address=%q", address.String())
		}
		result = append(result, binArray)
	}
	return result, nil
}

func (c *Client) refreshPool(ctx context.Context, configured Pool) (Pool, error) {
	account, err := c.accounts.Account(ctx, configured.Address)
	if err != nil {
		return Pool{}, fmt.Errorf("failed to refresh meteora dlmm pool: %w: pool_address=%q", err, configured.Address.String())
	}
	if account == nil || account.Owner != c.programID {
		return Pool{}, fmt.Errorf("failed to refresh meteora dlmm pool: pool_account=invalid pool_address=%q", configured.Address.String())
	}
	pool, err := decodePool(configured.Address, account.Data)
	if err != nil {
		return Pool{}, fmt.Errorf("failed to refresh meteora dlmm pool: %w: pool_address=%q", err, configured.Address.String())
	}
	if pool.TokenXMint != configured.TokenXMint || pool.TokenYMint != configured.TokenYMint {
		return Pool{}, fmt.Errorf("failed to refresh meteora dlmm pool: pool_identity=invalid pool_address=%q", configured.Address.String())
	}
	return pool, nil
}

// QuoteExactInput returns an on-chain-state Meteora DLMM exact-input quote.
//
// Parameters:
//   - ctx: quote context; nil uses context.Background.
//   - poolAddress: configured pool address.
//   - inputMint: input token mint.
//   - amountIn: exact input amount in atomic units.
//
// Returns:
//   - Exact-input quote.
//   - Quote error, including unsupported Token-2022 or bitmap-extension state.
//
// Version:
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
//   - 2026-08-31: Added.
func (c *Client) QuoteExactInputs(ctx context.Context, poolAddress onchainSolana.Address, requests []ExactInputRequest) ([]Quote, error) {
	if c == nil || c.snapshots == nil {
		return nil, fmt.Errorf("failed to quote meteora dlmm exact inputs: account_snapshot_provider=null")
	}
	if len(requests) == 0 {
		return nil, fmt.Errorf("failed to quote meteora dlmm exact inputs: requests=empty")
	}
	configured, exists := c.pools[poolAddress]
	if !exists {
		return nil, fmt.Errorf("failed to quote meteora dlmm exact inputs: pool=invalid pool_address=%q", poolAddress.String())
	}
	if ctx == nil {
		ctx = context.Background()
	}
	pool, err := c.refreshPool(ctx, configured)
	if err != nil {
		return nil, fmt.Errorf("failed to quote meteora dlmm exact inputs: %w", err)
	}
	addresses := []onchainSolana.Address{pool.Address, clockSysvarAddress}
	seen := map[onchainSolana.Address]struct{}{pool.Address: {}, clockSysvarAddress: {}}
	for index, request := range requests {
		if request.AmountIn == 0 {
			return nil, fmt.Errorf("failed to quote meteora dlmm exact inputs: amount_in=empty request_index=%d", index)
		}
		swapForY := request.InputMint == pool.TokenXMint
		if !swapForY && request.InputMint != pool.TokenYMint {
			return nil, fmt.Errorf("failed to quote meteora dlmm exact inputs: input_mint=invalid request_index=%d", index)
		}
		for _, arrayIndex := range initializedBinArrayIndexes(pool, swapForY, 16) {
			address, addressErr := binArrayAddress(c.programID, pool.Address, int64(arrayIndex))
			if addressErr != nil {
				return nil, fmt.Errorf("failed to quote meteora dlmm exact inputs: failed to derive bin array address: %w: bin_array_index=%d", addressErr, arrayIndex)
			}
			if _, exists := seen[address]; !exists {
				seen[address] = struct{}{}
				addresses = append(addresses, address)
			}
		}
	}
	snapshot, err := c.snapshots.AccountSnapshot(ctx, addresses)
	if err != nil {
		return nil, fmt.Errorf("failed to quote meteora dlmm exact inputs: %w", err)
	}
	accounts, err := newSnapshotAccounts(addresses, snapshot)
	if err != nil {
		return nil, fmt.Errorf("failed to quote meteora dlmm exact inputs: %w", err)
	}
	local := &Client{accounts: accounts, programID: c.programID, pools: c.pools, poolOrder: c.poolOrder}
	quotes := make([]Quote, len(requests))
	for index, request := range requests {
		quotes[index], err = local.quoteExactInput(ctx, poolAddress, request.InputMint, request.AmountIn)
		if err != nil {
			return nil, fmt.Errorf("failed to quote meteora dlmm exact inputs: %w: request_index=%d snapshot_slot=%d", err, index, snapshot.Slot)
		}
	}
	return quotes, nil
}

func (c *Client) quoteExactInput(ctx context.Context, poolAddress, inputMint onchainSolana.Address, amountIn uint64) (Quote, error) {
	if c == nil {
		return Quote{}, fmt.Errorf("failed to quote meteora dlmm exact input: client=null")
	}
	if amountIn == 0 {
		return Quote{}, fmt.Errorf("failed to quote meteora dlmm exact input: amount_in=empty")
	}
	configured, exists := c.pools[poolAddress]
	if !exists {
		return Quote{}, fmt.Errorf("failed to quote meteora dlmm exact input: pool=invalid pool_address=%q", poolAddress.String())
	}
	if ctx == nil {
		ctx = context.Background()
	}
	pool, err := c.refreshPool(ctx, configured)
	if err != nil {
		return Quote{}, fmt.Errorf("failed to quote meteora dlmm exact input: %w", err)
	}
	if pool.Status != 0 {
		return Quote{}, fmt.Errorf("failed to quote meteora dlmm exact input: pool_status=invalid status=%d", pool.Status)
	}
	if pool.TokenXProgramFlag != 0 || pool.TokenYProgramFlag != 0 {
		return Quote{}, fmt.Errorf("failed to quote meteora dlmm exact input: token_2022=unsupported pool_address=%q", poolAddress.String())
	}
	if pool.Parameters.CollectFeeMode > 1 {
		return Quote{}, fmt.Errorf("failed to quote meteora dlmm exact input: collect_fee_mode=invalid value=%d", pool.Parameters.CollectFeeMode)
	}
	swapForY := inputMint == pool.TokenXMint
	if !swapForY && inputMint != pool.TokenYMint {
		return Quote{}, fmt.Errorf("failed to quote meteora dlmm exact input: input_mint=invalid input_mint=%q", inputMint.String())
	}
	clock, err := c.clock(ctx)
	if err != nil {
		return Quote{}, fmt.Errorf("failed to quote meteora dlmm exact input: %w", err)
	}
	if clock.timestamp < 0 {
		return Quote{}, fmt.Errorf("failed to quote meteora dlmm exact input: clock_timestamp=out_of_range min_value=0")
	}
	if (pool.PairType == 1 || pool.PairType == 2) && ((pool.ActivationType == 0 && clock.slot < pool.ActivationPoint) || (pool.ActivationType == 1 && uint64(clock.timestamp) < pool.ActivationPoint)) {
		return Quote{}, fmt.Errorf("failed to quote meteora dlmm exact input: pool_activation=pending pool_address=%q", poolAddress.String())
	}
	if pool.PairType > 3 || pool.ActivationType > 1 {
		return Quote{}, fmt.Errorf("failed to quote meteora dlmm exact input: pool_configuration=invalid pool_address=%q", poolAddress.String())
	}
	if err := updateFeeReferences(&pool, clock.timestamp); err != nil {
		return Quote{}, fmt.Errorf("failed to quote meteora dlmm exact input: %w", err)
	}
	arrays, err := c.fetchBinArrays(ctx, pool, swapForY, 16)
	if err != nil {
		return Quote{}, fmt.Errorf("failed to quote meteora dlmm exact input: %w", err)
	}
	feeOnInput := pool.Parameters.CollectFeeMode == 0 || !swapForY
	supportLimitOrder := poolSupportsLimitOrders(pool)
	remaining := amountIn
	var amountOut, tradeFee, protocolFee uint64
	arraysUsed := 0
	for arrayIndex := range arrays {
		array := arrays[arrayIndex]
		arraysUsed = arrayIndex + 1
		lowerBinID := int32(array.Index) * binArraySize
		upperBinID := lowerBinID + binArraySize - 1
		if pool.ActiveBinID < lowerBinID || pool.ActiveBinID > upperBinID {
			if swapForY {
				pool.ActiveBinID = upperBinID
			} else {
				pool.ActiveBinID = lowerBinID
			}
		}
		for pool.ActiveBinID >= lowerBinID && pool.ActiveBinID <= upperBinID && remaining > 0 {
			bin := array.Bins[pool.ActiveBinID-lowerBinID]
			maxOut := binMaxOutput(bin, swapForY, supportLimitOrder)
			if maxOut > 0 {
				price := uint128LE(bin.Price)
				if price.Sign() == 0 {
					return Quote{}, fmt.Errorf("failed to quote meteora dlmm exact input: stored_bin_price=empty active_bin_id=%d", pool.ActiveBinID)
				}
				updateVolatilityAccumulator(&pool)
				feeRate, feeErr := totalFeeRate(pool)
				if feeErr != nil {
					return Quote{}, fmt.Errorf("failed to quote meteora dlmm exact input: %w", feeErr)
				}
				consumed, output, fee, protocolDelta, stepErr := quoteBinExactInput(remaining, bin, price, feeRate, pool.Parameters.ProtocolShare, swapForY, supportLimitOrder, feeOnInput)
				if stepErr != nil {
					return Quote{}, fmt.Errorf("failed to quote meteora dlmm exact input: %w: active_bin_id=%d", stepErr, pool.ActiveBinID)
				}
				if consumed > remaining || amountOut > ^uint64(0)-output || tradeFee > ^uint64(0)-fee {
					return Quote{}, fmt.Errorf("failed to quote meteora dlmm exact input: amount=out_of_range")
				}
				if protocolFee > ^uint64(0)-protocolDelta {
					return Quote{}, fmt.Errorf("failed to quote meteora dlmm exact input: protocol_fee=out_of_range")
				}
				remaining -= consumed
				amountOut += output
				tradeFee += fee
				protocolFee += protocolDelta
			}
			if remaining > 0 {
				if swapForY {
					pool.ActiveBinID--
				} else {
					pool.ActiveBinID++
				}
			}
		}
		if remaining == 0 {
			outputMint := pool.TokenXMint
			if swapForY {
				outputMint = pool.TokenYMint
			}
			return Quote{Pool: poolAddress, InputMint: inputMint, OutputMint: outputMint, AmountIn: amountIn, AmountOut: amountOut, TradeFee: tradeFee, ProtocolFee: protocolFee, EndActiveBin: pool.ActiveBinID, BinArraysUsed: arraysUsed}, nil
		}
	}
	if (swapForY && floorDiv(pool.Parameters.MinBinID, binArraySize) < -bitmapCenter) || (!swapForY && floorDiv(pool.Parameters.MaxBinID, binArraySize) >= bitmapCenter) {
		return Quote{}, fmt.Errorf("failed to quote meteora dlmm exact input: bitmap_extension=required amount_remaining=%d", remaining)
	}
	return Quote{}, fmt.Errorf("failed to quote meteora dlmm exact input: liquidity=insufficient amount_remaining=%d", remaining)
}

type snapshotAccounts map[onchainSolana.Address]*onchainSolana.Account

func newSnapshotAccounts(addresses []onchainSolana.Address, snapshot *onchainSolana.AccountSnapshot) (snapshotAccounts, error) {
	if snapshot == nil || len(snapshot.Accounts) != len(addresses) {
		return nil, fmt.Errorf("failed to create meteora dlmm snapshot accounts: snapshot=invalid")
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

type clockState struct {
	slot      uint64
	timestamp int64
}

func (c *Client) clock(ctx context.Context) (clockState, error) {
	account, err := c.accounts.Account(ctx, clockSysvarAddress)
	if err != nil {
		return clockState{}, fmt.Errorf("failed to fetch solana clock sysvar: %w", err)
	}
	if account == nil {
		return clockState{}, fmt.Errorf("failed to fetch solana clock sysvar: clock_account=null")
	}
	if len(account.Data) < 40 {
		return clockState{}, fmt.Errorf("failed to decode solana clock sysvar: data_length=too_short actual_length=%d min_length=40", len(account.Data))
	}
	return clockState{slot: binary.LittleEndian.Uint64(account.Data[:8]), timestamp: int64(binary.LittleEndian.Uint64(account.Data[32:40]))}, nil
}

func quoteBinExactInput(amountIn uint64, bin Bin, price *big.Int, feeRate uint64, protocolShare uint16, swapForY, supportLimitOrder, feeOnInput bool) (uint64, uint64, uint64, uint64, error) {
	available := amountIn
	fee := uint64(0)
	var err error
	if feeOnInput {
		fee, err = feeFromIncludedAmount(amountIn, feeRate)
		if err != nil {
			return 0, 0, 0, 0, fmt.Errorf("failed to calculate meteora dlmm bin input fee: %w", err)
		}
		if fee > amountIn {
			return 0, 0, 0, 0, fmt.Errorf("failed to calculate meteora dlmm bin input fee: fee=out_of_range")
		}
		available -= fee
	}
	mmOut := bin.AmountX
	if swapForY {
		mmOut = bin.AmountY
	}
	remaining, mmAmountIn, grossOut, err := fillBinLayer(available, mmOut, price, swapForY)
	if err != nil {
		return 0, 0, 0, 0, err
	}
	consumedExcluded := mmAmountIn
	if supportLimitOrder && remaining > 0 {
		openOrder, processedOrder := limitOrderAmounts(bin, swapForY)
		var layerIn, layerOut uint64
		remaining, layerIn, layerOut, err = fillBinLayer(remaining, processedOrder, price, swapForY)
		if err != nil {
			return 0, 0, 0, 0, err
		}
		consumedExcluded, grossOut, err = addFillAmounts(consumedExcluded, grossOut, layerIn, layerOut)
		if err != nil {
			return 0, 0, 0, 0, err
		}
		if remaining > 0 {
			remaining, layerIn, layerOut, err = fillBinLayer(remaining, openOrder, price, swapForY)
			if err != nil {
				return 0, 0, 0, 0, err
			}
			consumedExcluded, grossOut, err = addFillAmounts(consumedExcluded, grossOut, layerIn, layerOut)
			if err != nil {
				return 0, 0, 0, 0, err
			}
		}
	}
	consumed := consumedExcluded
	if feeOnInput {
		fee, err = feeFromExcludedAmount(consumedExcluded, feeRate)
		if err != nil || consumed > ^uint64(0)-fee {
			return 0, 0, 0, 0, fmt.Errorf("failed to calculate meteora dlmm bin excluded fee: amount=out_of_range")
		}
		consumed += fee
	}
	if !feeOnInput {
		fee, err = feeFromIncludedAmount(grossOut, feeRate)
		if err != nil {
			return 0, 0, 0, 0, fmt.Errorf("failed to calculate meteora dlmm bin output fee: %w", err)
		}
		if fee > grossOut {
			return 0, 0, 0, 0, fmt.Errorf("failed to calculate meteora dlmm bin output fee: fee=out_of_range")
		}
		grossOut -= fee
	}
	protocolFee, err := splitProtocolFee(fee, protocolShare, mmAmountIn, consumedExcluded)
	if err != nil {
		return 0, 0, 0, 0, err
	}
	return consumed, grossOut, fee, protocolFee, nil
}

func fillBinLayer(amount, maxOut uint64, price *big.Int, swapForY bool) (uint64, uint64, uint64, error) {
	if maxOut == 0 {
		return amount, 0, 0, nil
	}
	maxIn, err := amountInAtBin(maxOut, price, swapForY)
	if err != nil {
		return 0, 0, 0, err
	}
	if amount >= maxIn {
		return amount - maxIn, maxIn, maxOut, nil
	}
	out, err := amountOutAtBin(amount, price, swapForY)
	return 0, amount, out, err
}

func addFillAmounts(amountIn, amountOut, deltaIn, deltaOut uint64) (uint64, uint64, error) {
	if amountIn > ^uint64(0)-deltaIn || amountOut > ^uint64(0)-deltaOut {
		return 0, 0, fmt.Errorf("failed to calculate meteora dlmm bin fill: amount=out_of_range")
	}
	return amountIn + deltaIn, amountOut + deltaOut, nil
}

func limitOrderAmounts(bin Bin, swapForY bool) (uint64, uint64) {
	isAskSide := bin.LimitOrderAskSide != 0
	if (swapForY && !isAskSide) || (!swapForY && isAskSide) {
		return bin.OpenOrderAmount, bin.ProcessedOrderRemainingAmount
	}
	return 0, 0
}

func binMaxOutput(bin Bin, swapForY, supportLimitOrder bool) uint64 {
	result := bin.AmountX
	if swapForY {
		result = bin.AmountY
	}
	if !supportLimitOrder {
		return result
	}
	openOrder, processedOrder := limitOrderAmounts(bin, swapForY)
	if result > ^uint64(0)-openOrder {
		return ^uint64(0)
	}
	result += openOrder
	if result > ^uint64(0)-processedOrder {
		return ^uint64(0)
	}
	return result + processedOrder
}

func poolSupportsLimitOrders(pool Pool) bool {
	switch pool.Parameters.FunctionType {
	case 2:
		return true
	case 1:
		return false
	default:
		return pool.RewardMints[0].IsZero() && pool.RewardMints[1].IsZero()
	}
}

func splitProtocolFee(tradingFee uint64, protocolShare uint16, mmAmountIn, totalAmountIn uint64) (uint64, error) {
	if totalAmountIn == 0 || tradingFee == 0 {
		return 0, nil
	}
	mmFeeValue := new(big.Int).Mul(new(big.Int).SetUint64(tradingFee), new(big.Int).SetUint64(mmAmountIn))
	mmFeeValue.Add(mmFeeValue, new(big.Int).SetUint64(totalAmountIn-1))
	mmFeeValue.Div(mmFeeValue, new(big.Int).SetUint64(totalAmountIn))
	if !mmFeeValue.IsUint64() {
		return 0, fmt.Errorf("failed to calculate meteora dlmm protocol fee: amount=out_of_range")
	}
	mmFee := mmFeeValue.Uint64()
	limitOrderFee := tradingFee - mmFee
	limitOrderUserFeeValue := new(big.Int).Mul(new(big.Int).SetUint64(limitOrderFee), new(big.Int).SetUint64(limitOrderFeeShare))
	limitOrderUserFeeValue.Div(limitOrderUserFeeValue, new(big.Int).SetUint64(basisPointMax))
	limitOrderProtocolFee := limitOrderFee - limitOrderUserFeeValue.Uint64()
	mmProtocolFeeValue := new(big.Int).Mul(new(big.Int).SetUint64(mmFee), new(big.Int).SetUint64(uint64(protocolShare)))
	mmProtocolFeeValue.Div(mmProtocolFeeValue, new(big.Int).SetUint64(basisPointMax))
	mmProtocolFee := mmProtocolFeeValue.Uint64()
	if limitOrderProtocolFee > ^uint64(0)-mmProtocolFee {
		return 0, fmt.Errorf("failed to calculate meteora dlmm protocol fee: amount=out_of_range")
	}
	return limitOrderProtocolFee + mmProtocolFee, nil
}

func decodePool(address onchainSolana.Address, data []byte) (Pool, error) {
	if len(data) != lbPairDataLength {
		return Pool{}, fmt.Errorf("failed to decode meteora dlmm pool: data_length=invalid actual_length=%d expected_length=%d", len(data), lbPairDataLength)
	}
	if !bytes.Equal(data[:8], lbPairDiscriminator[:]) {
		return Pool{}, fmt.Errorf("failed to decode meteora dlmm pool: discriminator=invalid")
	}
	pool := Pool{Address: address}
	p := &pool.Parameters
	p.BaseFactor = binary.LittleEndian.Uint16(data[8:10])
	p.FilterPeriod = binary.LittleEndian.Uint16(data[10:12])
	p.DecayPeriod = binary.LittleEndian.Uint16(data[12:14])
	p.ReductionFactor = binary.LittleEndian.Uint16(data[14:16])
	p.VariableFeeControl = binary.LittleEndian.Uint32(data[16:20])
	p.MaxVolatilityAccumulator = binary.LittleEndian.Uint32(data[20:24])
	p.MinBinID = int32(binary.LittleEndian.Uint32(data[24:28]))
	p.MaxBinID = int32(binary.LittleEndian.Uint32(data[28:32]))
	p.ProtocolShare = binary.LittleEndian.Uint16(data[32:34])
	p.BaseFeePowerFactor, p.FunctionType, p.CollectFeeMode = data[34], data[35], data[36]
	v := &pool.VariableParameters
	v.VolatilityAccumulator = binary.LittleEndian.Uint32(data[40:44])
	v.VolatilityReference = binary.LittleEndian.Uint32(data[44:48])
	v.IndexReference = int32(binary.LittleEndian.Uint32(data[48:52]))
	v.LastUpdateTimestamp = int64(binary.LittleEndian.Uint64(data[56:64]))
	pool.PairType = data[75]
	pool.ActiveBinID = int32(binary.LittleEndian.Uint32(data[76:80]))
	pool.BinStep = binary.LittleEndian.Uint16(data[80:82])
	pool.Status, pool.ActivationType = data[82], data[86]
	copy(pool.TokenXMint[:], data[88:120])
	copy(pool.TokenYMint[:], data[120:152])
	copy(pool.ReserveX[:], data[152:184])
	copy(pool.ReserveY[:], data[184:216])
	copy(pool.RewardMints[0][:], data[280:312])
	copy(pool.RewardMints[1][:], data[416:448])
	copy(pool.Oracle[:], data[552:584])
	for i := range pool.BinArrayBitmap {
		pool.BinArrayBitmap[i] = binary.LittleEndian.Uint64(data[584+i*8 : 592+i*8])
	}
	pool.LastUpdatedAt = int64(binary.LittleEndian.Uint64(data[712:720]))
	pool.ActivationPoint = binary.LittleEndian.Uint64(data[816:824])
	pool.PreActivationDuration = binary.LittleEndian.Uint64(data[824:832])
	copy(pool.Creator[:], data[848:880])
	pool.TokenXProgramFlag, pool.TokenYProgramFlag, pool.Version = data[880], data[881], data[882]
	if pool.TokenXMint.IsZero() || pool.TokenYMint.IsZero() || pool.ReserveX.IsZero() || pool.ReserveY.IsZero() {
		return Pool{}, fmt.Errorf("failed to decode meteora dlmm pool: required_address=empty")
	}
	if pool.BinStep == 0 {
		return Pool{}, fmt.Errorf("failed to decode meteora dlmm pool: bin_step=empty")
	}
	if pool.ActiveBinID < p.MinBinID || pool.ActiveBinID > p.MaxBinID {
		return Pool{}, fmt.Errorf("failed to decode meteora dlmm pool: active_bin_id=out_of_range")
	}
	if pool.Status > 1 || pool.PairType > 3 || pool.ActivationType > 1 || p.FunctionType > 2 || p.CollectFeeMode > 1 || pool.TokenXProgramFlag > 1 || pool.TokenYProgramFlag > 1 {
		return Pool{}, fmt.Errorf("failed to decode meteora dlmm pool: enum_value=invalid")
	}
	if p.ProtocolShare > 2500 {
		return Pool{}, fmt.Errorf("failed to decode meteora dlmm pool: protocol_share=out_of_range max_value=2500")
	}
	return pool, nil
}

func decodeBinArray(address onchainSolana.Address, data []byte) (BinArray, error) {
	if len(data) != binArrayDataLength {
		return BinArray{}, fmt.Errorf("failed to decode meteora dlmm bin array: data_length=invalid actual_length=%d expected_length=%d", len(data), binArrayDataLength)
	}
	if !bytes.Equal(data[:8], binArrayDiscriminator[:]) {
		return BinArray{}, fmt.Errorf("failed to decode meteora dlmm bin array: discriminator=invalid")
	}
	result := BinArray{Address: address, Index: int64(binary.LittleEndian.Uint64(data[8:16])), Version: data[16]}
	copy(result.Pool[:], data[24:56])
	for i := range result.Bins {
		offset := 56 + i*binDataLength
		bin := &result.Bins[i]
		bin.AmountX = binary.LittleEndian.Uint64(data[offset : offset+8])
		bin.AmountY = binary.LittleEndian.Uint64(data[offset+8 : offset+16])
		copy(bin.Price[:], data[offset+16:offset+32])
		copy(bin.LiquiditySupply[:], data[offset+32:offset+48])
		bin.FulfilledOrderAmountX = binary.LittleEndian.Uint64(data[offset+48 : offset+56])
		bin.FulfilledOrderAmountY = binary.LittleEndian.Uint64(data[offset+56 : offset+64])
		bin.LimitOrderFeeAskSide = binary.LittleEndian.Uint64(data[offset+64 : offset+72])
		bin.LimitOrderFeeBidSide = binary.LittleEndian.Uint64(data[offset+72 : offset+80])
		copy(bin.FeeAmountXPerTokenStored[:], data[offset+80:offset+96])
		copy(bin.FeeAmountYPerTokenStored[:], data[offset+96:offset+112])
		bin.OpenOrderAmount = binary.LittleEndian.Uint64(data[offset+112 : offset+120])
		bin.TotalProcessingOrderAmount = binary.LittleEndian.Uint64(data[offset+120 : offset+128])
		bin.ProcessedOrderRemainingAmount = binary.LittleEndian.Uint64(data[offset+128 : offset+136])
		bin.OrderAge = binary.LittleEndian.Uint32(data[offset+136 : offset+140])
		bin.LimitOrderAskSide = data[offset+140]
	}
	return result, nil
}

func initializedBinArrayIndexes(pool Pool, swapForY bool, limit int) []int32 {
	start := floorDiv(pool.ActiveBinID, binArraySize)
	position := start + bitmapCenter
	step := int32(1)
	if swapForY {
		step = -1
	}
	result := make([]int32, 0, limit)
	for position >= 0 && position < 1024 && len(result) < limit {
		word, bit := position/64, uint(position%64)
		if pool.BinArrayBitmap[word]&(uint64(1)<<bit) != 0 {
			result = append(result, position-bitmapCenter)
		}
		position += step
	}
	return result
}

func floorDiv(value int32, divisor int) int32 {
	divisor32 := int32(divisor)
	quotient := value / divisor32
	if value < 0 && value%divisor32 != 0 {
		quotient--
	}
	return quotient
}

func binArrayAddress(programID, poolAddress onchainSolana.Address, index int64) (onchainSolana.Address, error) {
	seed := make([]byte, 8)
	binary.LittleEndian.PutUint64(seed, uint64(index))
	address, _, err := solanaSDK.FindProgramAddress([][]byte{[]byte("bin_array"), poolAddress[:], seed}, solanaSDK.PublicKey(programID))
	if err != nil {
		return onchainSolana.Address{}, fmt.Errorf("failed to find solana program address: %w", err)
	}
	return onchainSolana.Address(address), nil
}

func mustAddress(value string) onchainSolana.Address {
	address, err := onchainSolana.ParseAddress(value)
	if err != nil {
		panic(err)
	}
	return address
}
