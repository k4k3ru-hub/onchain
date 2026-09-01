package clmm

import (
	"context"
	"encoding/binary"
	"math/big"
	"testing"

	onchainSolana "github.com/k4k3ru-hub/onchain/go/solana"
)

type stubAccounts struct {
	values        map[onchainSolana.Address]*onchainSolana.Account
	accountCalls  int
	snapshotCalls int
}

func (s *stubAccounts) Account(_ context.Context, address onchainSolana.Address) (*onchainSolana.Account, error) {
	s.accountCalls++
	return s.values[address], nil
}

func (s *stubAccounts) AccountSnapshot(_ context.Context, addresses []onchainSolana.Address) (*onchainSolana.AccountSnapshot, error) {
	s.snapshotCalls++
	accounts := make([]*onchainSolana.Account, len(addresses))
	for index, address := range addresses {
		accounts[index] = s.values[address]
	}
	return &onchainSolana.AccountSnapshot{Slot: 1, Accounts: accounts}, nil
}

func TestNewClientDiscoversConfiguredCLMMPool(t *testing.T) {
	poolAddress := testAddress(1)
	data := make([]byte, poolDataLength)
	copy(data[:8], poolDiscriminator[:])
	putAddress(data, 9, testAddress(2))
	putAddress(data, 41, testAddress(3))
	putAddress(data, 73, testAddress(4))
	putAddress(data, 105, testAddress(5))
	putAddress(data, 137, testAddress(6))
	putAddress(data, 169, testAddress(7))
	putAddress(data, 201, testAddress(8))
	data[233], data[234] = 9, 6
	binary.LittleEndian.PutUint16(data[235:237], 64)
	binary.LittleEndian.PutUint32(data[269:273], uint32(1234))
	configAddress := testAddress(2)

	accounts := &stubAccounts{values: map[onchainSolana.Address]*onchainSolana.Account{
		poolAddress:   {Address: poolAddress, Owner: mainnetProgramID, Data: data},
		configAddress: {Address: configAddress, Owner: mainnetProgramID, Data: testAMMConfigData(64)},
	}}
	client, err := NewClient(context.Background(), accounts, Config{Pools: []onchainSolana.Address{poolAddress}})
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	pools := client.Pools()
	if len(pools) != 1 || pools[0].TickSpacing != 64 || pools[0].CurrentTick != 1234 || pools[0].Token0Decimals != 9 || pools[0].Token1Decimals != 6 {
		t.Fatalf("Pools() = %+v", pools)
	}
}

func TestTickArraysForQuoteDiscoversBitmapArrays(t *testing.T) {
	poolAddress := testAddress(1)
	configAddress := testAddress(2)
	token0 := testAddress(4)
	poolData := make([]byte, poolDataLength)
	copy(poolData[:8], poolDiscriminator[:])
	putAddress(poolData, 9, configAddress)
	putAddress(poolData, 73, token0)
	putAddress(poolData, 105, testAddress(5))
	putAddress(poolData, 137, testAddress(6))
	putAddress(poolData, 169, testAddress(7))
	binary.LittleEndian.PutUint16(poolData[235:237], 10)
	binary.LittleEndian.PutUint32(poolData[269:273], uint32(650))
	// Current start is 600 (bitmap bit 513); zero-for-one next is 0 (bit 512).
	binary.LittleEndian.PutUint64(poolData[904+8*8:912+8*8], uint64(3))

	currentAddress, err := tickArrayAddress(mainnetProgramID, poolAddress, 600)
	if err != nil {
		t.Fatalf("tickArrayAddress() error = %v", err)
	}
	nextAddress, err := tickArrayAddress(mainnetProgramID, poolAddress, 0)
	if err != nil {
		t.Fatalf("tickArrayAddress() error = %v", err)
	}
	accounts := &stubAccounts{values: map[onchainSolana.Address]*onchainSolana.Account{
		poolAddress:    {Address: poolAddress, Owner: mainnetProgramID, Data: poolData},
		configAddress:  {Address: configAddress, Owner: mainnetProgramID, Data: testAMMConfigData(10)},
		currentAddress: {Address: currentAddress, Owner: mainnetProgramID, Data: testTickArrayData(poolAddress, 600)},
		nextAddress:    {Address: nextAddress, Owner: mainnetProgramID, Data: testTickArrayData(poolAddress, 0)},
	}}
	client, err := NewClient(context.Background(), accounts, Config{Pools: []onchainSolana.Address{poolAddress}})
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	arrays, err := client.TickArraysForQuote(context.Background(), poolAddress, token0, 2)
	if err != nil {
		t.Fatalf("TickArraysForQuote() error = %v", err)
	}
	if len(arrays) != 2 || arrays[0].StartTickIndex != 600 || arrays[1].StartTickIndex != 0 {
		t.Fatalf("TickArraysForQuote() = %+v", arrays)
	}
}

func TestQuoteExactInputWithinCurrentLiquidityRange(t *testing.T) {
	poolAddress := testAddress(1)
	configAddress := testAddress(2)
	token0, token1 := testAddress(4), testAddress(5)
	poolData := make([]byte, poolDataLength)
	copy(poolData[:8], poolDiscriminator[:])
	putAddress(poolData, 9, configAddress)
	putAddress(poolData, 73, token0)
	putAddress(poolData, 105, token1)
	putAddress(poolData, 137, testAddress(6))
	putAddress(poolData, 169, testAddress(7))
	binary.LittleEndian.PutUint16(poolData[235:237], 10)
	putUint128LE(poolData[237:253], big.NewInt(1_000_000_000_000))
	price, err := sqrtPriceAtTick(5)
	if err != nil {
		t.Fatalf("sqrtPriceAtTick() error = %v", err)
	}
	putUint128LE(poolData[253:269], price)
	binary.LittleEndian.PutUint32(poolData[269:273], 5)
	binary.LittleEndian.PutUint64(poolData[904+8*8:912+8*8], 1)

	tickAddress, err := tickArrayAddress(mainnetProgramID, poolAddress, 0)
	if err != nil {
		t.Fatalf("tickArrayAddress() error = %v", err)
	}
	tickData := testTickArrayData(poolAddress, 0)
	putUint128LE(tickData[44+20:44+36], big.NewInt(1))
	tickData[10124] = 1
	configData := testAMMConfigData(10)
	binary.LittleEndian.PutUint32(configData[47:51], 2500)
	accounts := &stubAccounts{values: map[onchainSolana.Address]*onchainSolana.Account{
		poolAddress:   {Address: poolAddress, Owner: mainnetProgramID, Data: poolData},
		configAddress: {Address: configAddress, Owner: mainnetProgramID, Data: configData},
		tickAddress:   {Address: tickAddress, Owner: mainnetProgramID, Data: tickData},
	}}
	client, err := NewClient(context.Background(), accounts, Config{Pools: []onchainSolana.Address{poolAddress}})
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	quote, err := client.QuoteExactInput(context.Background(), poolAddress, token0, 1_000_000)
	if err != nil {
		t.Fatalf("QuoteExactInput() error = %v", err)
	}
	if quote.AmountIn != 1_000_000 || quote.AmountOut == 0 || quote.TradeFee == 0 || quote.OutputMint != token1 || quote.TickArraysUsed != 1 {
		t.Fatalf("QuoteExactInput() = %+v", quote)
	}
	if accounts.snapshotCalls != 1 {
		t.Fatalf("AccountSnapshot() calls = %d, want 1", accounts.snapshotCalls)
	}
}

func TestQuoteExactInputsSharesOneAccountSnapshot(t *testing.T) {
	poolAddress := testAddress(1)
	configAddress := testAddress(2)
	token0, token1 := testAddress(4), testAddress(5)
	poolData := make([]byte, poolDataLength)
	copy(poolData[:8], poolDiscriminator[:])
	putAddress(poolData, 9, configAddress)
	putAddress(poolData, 73, token0)
	putAddress(poolData, 105, token1)
	putAddress(poolData, 137, testAddress(6))
	putAddress(poolData, 169, testAddress(7))
	binary.LittleEndian.PutUint16(poolData[235:237], 10)
	putUint128LE(poolData[237:253], big.NewInt(1_000_000_000_000))
	price, err := sqrtPriceAtTick(5)
	if err != nil {
		t.Fatalf("sqrtPriceAtTick() error = %v", err)
	}
	putUint128LE(poolData[253:269], price)
	binary.LittleEndian.PutUint32(poolData[269:273], 5)
	binary.LittleEndian.PutUint64(poolData[904+8*8:912+8*8], 1)
	tickAddress, err := tickArrayAddress(mainnetProgramID, poolAddress, 0)
	if err != nil {
		t.Fatalf("tickArrayAddress() error = %v", err)
	}
	tickData := testTickArrayData(poolAddress, 0)
	putUint128LE(tickData[44+20:44+36], big.NewInt(1))
	tickData[10124] = 1
	configData := testAMMConfigData(10)
	binary.LittleEndian.PutUint32(configData[47:51], 2500)
	accounts := &stubAccounts{values: map[onchainSolana.Address]*onchainSolana.Account{
		poolAddress:   {Address: poolAddress, Owner: mainnetProgramID, Data: poolData},
		configAddress: {Address: configAddress, Owner: mainnetProgramID, Data: configData},
		tickAddress:   {Address: tickAddress, Owner: mainnetProgramID, Data: tickData},
	}}
	client, err := NewClient(context.Background(), accounts, Config{Pools: []onchainSolana.Address{poolAddress}})
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	accounts.accountCalls = 0
	batch, err := client.QuoteExactInputsWithSlot(context.Background(), poolAddress, []ExactInputRequest{
		{InputMint: token0, AmountIn: 1_000_000},
		{InputMint: token0, AmountIn: 2_000_000},
	})
	if err != nil {
		t.Fatalf("QuoteExactInputsWithSlot() error = %v", err)
	}
	quotes := batch.Quotes
	if batch.Slot != 1 || len(quotes) != 2 || quotes[0].AmountOut == 0 || quotes[1].AmountOut <= quotes[0].AmountOut {
		t.Fatalf("QuoteExactInputsWithSlot() = %+v", batch)
	}
	if accounts.snapshotCalls != 1 {
		t.Fatalf("AccountSnapshot() calls = %d, want 1", accounts.snapshotCalls)
	}
	if accounts.accountCalls != 0 {
		t.Fatalf("Account() calls = %d, want 0", accounts.accountCalls)
	}
}

func TestQuoteExactInputMatchesLimitOrderAtCurrentTick(t *testing.T) {
	poolAddress := testAddress(1)
	configAddress := testAddress(2)
	token0, token1 := testAddress(4), testAddress(5)
	poolData := make([]byte, poolDataLength)
	copy(poolData[:8], poolDiscriminator[:])
	putAddress(poolData, 9, configAddress)
	putAddress(poolData, 73, token0)
	putAddress(poolData, 105, token1)
	putAddress(poolData, 137, testAddress(6))
	putAddress(poolData, 169, testAddress(7))
	binary.LittleEndian.PutUint16(poolData[235:237], 1)
	putUint128LE(poolData[237:253], big.NewInt(1_000_000_000_000))
	price, err := sqrtPriceAtTick(0)
	if err != nil {
		t.Fatalf("sqrtPriceAtTick() error = %v", err)
	}
	putUint128LE(poolData[253:269], price)
	binary.LittleEndian.PutUint64(poolData[904+8*8:912+8*8], 1)
	tickAddress, err := tickArrayAddress(mainnetProgramID, poolAddress, 0)
	if err != nil {
		t.Fatalf("tickArrayAddress() error = %v", err)
	}
	tickData := testTickArrayData(poolAddress, 0)
	putUint128LE(tickData[44+20:44+36], big.NewInt(1))
	binary.LittleEndian.PutUint64(tickData[44+124:44+132], 2_000)
	binary.LittleEndian.PutUint64(tickData[44+132:44+140], 500)
	tickData[10124] = 1
	configData := testAMMConfigData(1)
	binary.LittleEndian.PutUint32(configData[47:51], 2_500)
	accounts := &stubAccounts{values: map[onchainSolana.Address]*onchainSolana.Account{
		poolAddress:   {Address: poolAddress, Owner: mainnetProgramID, Data: poolData},
		configAddress: {Address: configAddress, Owner: mainnetProgramID, Data: configData},
		tickAddress:   {Address: tickAddress, Owner: mainnetProgramID, Data: tickData},
	}}
	client, err := NewClient(context.Background(), accounts, Config{Pools: []onchainSolana.Address{poolAddress}})
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	quote, err := client.QuoteExactInput(context.Background(), poolAddress, token0, 1_000)
	if err != nil {
		t.Fatalf("QuoteExactInput() error = %v", err)
	}
	if quote.AmountOut != 997 || quote.TradeFee != 3 || quote.EndTick != 0 {
		t.Fatalf("QuoteExactInput() = %+v", quote)
	}
}

func TestNewClientRejectsInvalidCLMMDiscriminator(t *testing.T) {
	poolAddress := testAddress(1)
	accounts := &stubAccounts{values: map[onchainSolana.Address]*onchainSolana.Account{
		poolAddress: {Address: poolAddress, Owner: mainnetProgramID, Data: make([]byte, poolDataLength)},
	}}
	if _, err := NewClient(context.Background(), accounts, Config{Pools: []onchainSolana.Address{poolAddress}}); err == nil {
		t.Fatal("NewClient() error = nil")
	}
}

func putAddress(data []byte, offset int, address onchainSolana.Address) {
	copy(data[offset:offset+32], address[:])
}

func testAddress(value byte) onchainSolana.Address {
	var address onchainSolana.Address
	address[0] = value
	return address
}

func testAMMConfigData(tickSpacing uint16) []byte {
	data := make([]byte, ammConfigDataLength)
	copy(data[:8], configDiscriminator[:])
	binary.LittleEndian.PutUint16(data[51:53], tickSpacing)
	return data
}

func testTickArrayData(poolAddress onchainSolana.Address, startTickIndex int32) []byte {
	data := make([]byte, tickArrayDataLength)
	copy(data[:8], tickArrayDiscriminator[:])
	putAddress(data, 8, poolAddress)
	binary.LittleEndian.PutUint32(data[40:44], uint32(startTickIndex))
	return data
}

func putUint128LE(destination []byte, value *big.Int) {
	bytes := make([]byte, 16)
	value.FillBytes(bytes)
	for i := range bytes {
		destination[len(bytes)-1-i] = bytes[i]
	}
}
