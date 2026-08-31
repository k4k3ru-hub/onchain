package dlmm

import (
	"context"
	"encoding/binary"
	"testing"

	onchainSolana "github.com/k4k3ru-hub/onchain/go/solana"
)

type stubAccounts struct {
	values        map[onchainSolana.Address]*onchainSolana.Account
	snapshotCalls int
}

func (s *stubAccounts) Account(_ context.Context, address onchainSolana.Address) (*onchainSolana.Account, error) {
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

func TestNewClientDiscoversConfiguredPool(t *testing.T) {
	poolAddress := testAddress(1)
	poolData := testPoolData(75, 25)
	accounts := &stubAccounts{values: map[onchainSolana.Address]*onchainSolana.Account{
		poolAddress: {Address: poolAddress, Owner: mainnetProgramID, Data: poolData},
	}}
	client, err := NewClient(context.Background(), accounts, Config{Pools: []onchainSolana.Address{poolAddress}})
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	pools := client.Pools()
	if len(pools) != 1 || pools[0].ActiveBinID != 75 || pools[0].BinStep != 25 || pools[0].TokenXMint != testAddress(2) {
		t.Fatalf("Pools() = %+v", pools)
	}
}

func TestBinArraysForQuoteDiscoversBitmapArrays(t *testing.T) {
	poolAddress := testAddress(1)
	poolData := testPoolData(75, 25)
	// Active array index is 1 (bitmap bit 513); X -> Y traverses toward index 0.
	binary.LittleEndian.PutUint64(poolData[584+8*8:592+8*8], 3)
	arrayOneAddress, err := binArrayAddress(mainnetProgramID, poolAddress, 1)
	if err != nil {
		t.Fatalf("binArrayAddress() error = %v", err)
	}
	arrayZeroAddress, err := binArrayAddress(mainnetProgramID, poolAddress, 0)
	if err != nil {
		t.Fatalf("binArrayAddress() error = %v", err)
	}
	accounts := &stubAccounts{values: map[onchainSolana.Address]*onchainSolana.Account{
		poolAddress:      {Address: poolAddress, Owner: mainnetProgramID, Data: poolData},
		arrayOneAddress:  {Address: arrayOneAddress, Owner: mainnetProgramID, Data: testBinArrayData(poolAddress, 1)},
		arrayZeroAddress: {Address: arrayZeroAddress, Owner: mainnetProgramID, Data: testBinArrayData(poolAddress, 0)},
	}}
	client, err := NewClient(context.Background(), accounts, Config{Pools: []onchainSolana.Address{poolAddress}})
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	arrays, err := client.BinArraysForQuote(context.Background(), poolAddress, testAddress(2), 2)
	if err != nil {
		t.Fatalf("BinArraysForQuote() error = %v", err)
	}
	if len(arrays) != 2 || arrays[0].Index != 1 || arrays[1].Index != 0 {
		t.Fatalf("BinArraysForQuote() = %+v", arrays)
	}
}

func TestQuoteExactInputUsesStoredBinPriceAndDynamicFeeState(t *testing.T) {
	poolAddress := testAddress(1)
	poolData := testPoolData(75, 25)
	binary.LittleEndian.PutUint16(poolData[8:10], 100)
	binary.LittleEndian.PutUint64(poolData[584+8*8:592+8*8], 2)
	arrayAddress, err := binArrayAddress(mainnetProgramID, poolAddress, 1)
	if err != nil {
		t.Fatalf("binArrayAddress() error = %v", err)
	}
	arrayData := testBinArrayData(poolAddress, 1)
	binOffset := 56 + 5*binDataLength
	binary.LittleEndian.PutUint64(arrayData[binOffset+8:binOffset+16], 10_000_000)
	// Q64 price 1.0.
	binary.LittleEndian.PutUint64(arrayData[binOffset+24:binOffset+32], 1)
	clockData := make([]byte, 40)
	binary.LittleEndian.PutUint64(clockData[:8], 1_000)
	binary.LittleEndian.PutUint64(clockData[32:40], 1_000)
	accounts := &stubAccounts{values: map[onchainSolana.Address]*onchainSolana.Account{
		poolAddress:        {Address: poolAddress, Owner: mainnetProgramID, Data: poolData},
		arrayAddress:       {Address: arrayAddress, Owner: mainnetProgramID, Data: arrayData},
		clockSysvarAddress: {Address: clockSysvarAddress, Data: clockData},
	}}
	client, err := NewClient(context.Background(), accounts, Config{Pools: []onchainSolana.Address{poolAddress}})
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	quote, err := client.QuoteExactInput(context.Background(), poolAddress, testAddress(2), 1_000_000)
	if err != nil {
		t.Fatalf("QuoteExactInput() error = %v", err)
	}
	if quote.AmountIn != 1_000_000 || quote.AmountOut == 0 || quote.AmountOut >= quote.AmountIn || quote.TradeFee == 0 || quote.OutputMint != testAddress(3) {
		t.Fatalf("QuoteExactInput() = %+v", quote)
	}
	if accounts.snapshotCalls != 1 {
		t.Fatalf("AccountSnapshot() calls = %d, want 1", accounts.snapshotCalls)
	}
}

func TestQuoteExactInputsSharesOneAccountSnapshot(t *testing.T) {
	poolAddress := testAddress(1)
	poolData := testPoolData(75, 25)
	binary.LittleEndian.PutUint16(poolData[8:10], 100)
	binary.LittleEndian.PutUint64(poolData[584+8*8:592+8*8], 2)
	arrayAddress, err := binArrayAddress(mainnetProgramID, poolAddress, 1)
	if err != nil {
		t.Fatalf("binArrayAddress() error = %v", err)
	}
	arrayData := testBinArrayData(poolAddress, 1)
	binOffset := 56 + 5*binDataLength
	binary.LittleEndian.PutUint64(arrayData[binOffset+8:binOffset+16], 10_000_000)
	binary.LittleEndian.PutUint64(arrayData[binOffset+24:binOffset+32], 1)
	clockData := make([]byte, 40)
	binary.LittleEndian.PutUint64(clockData[:8], 1_000)
	binary.LittleEndian.PutUint64(clockData[32:40], 1_000)
	accounts := &stubAccounts{values: map[onchainSolana.Address]*onchainSolana.Account{
		poolAddress:        {Address: poolAddress, Owner: mainnetProgramID, Data: poolData},
		arrayAddress:       {Address: arrayAddress, Owner: mainnetProgramID, Data: arrayData},
		clockSysvarAddress: {Address: clockSysvarAddress, Data: clockData},
	}}
	client, err := NewClient(context.Background(), accounts, Config{Pools: []onchainSolana.Address{poolAddress}})
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	quotes, err := client.QuoteExactInputs(context.Background(), poolAddress, []ExactInputRequest{
		{InputMint: testAddress(2), AmountIn: 1_000_000},
		{InputMint: testAddress(2), AmountIn: 2_000_000},
	})
	if err != nil {
		t.Fatalf("QuoteExactInputs() error = %v", err)
	}
	if len(quotes) != 2 || quotes[0].AmountOut == 0 || quotes[1].AmountOut <= quotes[0].AmountOut {
		t.Fatalf("QuoteExactInputs() = %+v", quotes)
	}
	if accounts.snapshotCalls != 1 {
		t.Fatalf("AccountSnapshot() calls = %d, want 1", accounts.snapshotCalls)
	}
}

func TestQuoteExactInputConsumesMatchingLimitOrderLiquidity(t *testing.T) {
	poolAddress := testAddress(1)
	poolData := testPoolData(75, 25)
	poolData[35] = 2 // FunctionType::LimitOrder.
	binary.LittleEndian.PutUint16(poolData[8:10], 100)
	binary.LittleEndian.PutUint64(poolData[584+8*8:592+8*8], 2)
	arrayAddress, err := binArrayAddress(mainnetProgramID, poolAddress, 1)
	if err != nil {
		t.Fatalf("binArrayAddress() error = %v", err)
	}
	arrayData := testBinArrayData(poolAddress, 1)
	binOffset := 56 + 5*binDataLength
	binary.LittleEndian.PutUint64(arrayData[binOffset+8:binOffset+16], 400_000)
	binary.LittleEndian.PutUint64(arrayData[binOffset+24:binOffset+32], 1) // Q64 price 1.0.
	binary.LittleEndian.PutUint64(arrayData[binOffset+112:binOffset+120], 400_000)
	binary.LittleEndian.PutUint64(arrayData[binOffset+128:binOffset+136], 300_000)
	arrayData[binOffset+140] = 0 // Bid-side orders are fillable when swapping X for Y.
	clockData := make([]byte, 40)
	binary.LittleEndian.PutUint64(clockData[:8], 1_000)
	binary.LittleEndian.PutUint64(clockData[32:40], 1_000)
	accounts := &stubAccounts{values: map[onchainSolana.Address]*onchainSolana.Account{
		poolAddress:        {Address: poolAddress, Owner: mainnetProgramID, Data: poolData},
		arrayAddress:       {Address: arrayAddress, Owner: mainnetProgramID, Data: arrayData},
		clockSysvarAddress: {Address: clockSysvarAddress, Data: clockData},
	}}
	client, err := NewClient(context.Background(), accounts, Config{Pools: []onchainSolana.Address{poolAddress}})
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	quote, err := client.QuoteExactInput(context.Background(), poolAddress, testAddress(2), 1_000_000)
	if err != nil {
		t.Fatalf("QuoteExactInput() error = %v", err)
	}
	if quote.AmountOut <= 400_000 {
		t.Fatalf("QuoteExactInput() AmountOut = %d, want limit-order liquidity consumed", quote.AmountOut)
	}
	if quote.TradeFee == 0 || quote.ProtocolFee == 0 {
		t.Fatalf("QuoteExactInput() fees = trade:%d protocol:%d", quote.TradeFee, quote.ProtocolFee)
	}
}

func TestNewClientRejectsInvalidDiscriminator(t *testing.T) {
	poolAddress := testAddress(1)
	accounts := &stubAccounts{values: map[onchainSolana.Address]*onchainSolana.Account{
		poolAddress: {Address: poolAddress, Owner: mainnetProgramID, Data: make([]byte, lbPairDataLength)},
	}}
	if _, err := NewClient(context.Background(), accounts, Config{Pools: []onchainSolana.Address{poolAddress}}); err == nil {
		t.Fatal("NewClient() error = nil")
	}
}

func TestBinArrayAddressMatchesOfficialFixtureVectors(t *testing.T) {
	pool, err := onchainSolana.ParseAddress("EtAdVRLFH22rjWh3mcUasKFF27WtHhsaCvK27tPFFWig")
	if err != nil {
		t.Fatalf("ParseAddress() error = %v", err)
	}
	tests := []struct {
		index int64
		want  string
	}{
		{index: 0, want: "5Sm2ecMeqohRkNpFJPWSqHL1BkA7AEW4ck8TmdF1gD4t"},
		{index: -1, want: "E6gur9Jw8675DCR7GpJVhoSrkruRgt8EdEVqLAc5RLUt"},
	}
	for _, test := range tests {
		address, addressErr := binArrayAddress(mainnetProgramID, pool, test.index)
		if addressErr != nil {
			t.Fatalf("binArrayAddress(%d) error = %v", test.index, addressErr)
		}
		if address.String() != test.want {
			t.Fatalf("binArrayAddress(%d) = %q, want %q", test.index, address.String(), test.want)
		}
	}
}

func testPoolData(activeBinID int32, binStep uint16) []byte {
	data := make([]byte, lbPairDataLength)
	copy(data[:8], lbPairDiscriminator[:])
	minBinID := int32(-443636)
	binary.LittleEndian.PutUint32(data[24:28], uint32(minBinID))
	binary.LittleEndian.PutUint32(data[28:32], uint32(443636))
	binary.LittleEndian.PutUint32(data[76:80], uint32(activeBinID))
	binary.LittleEndian.PutUint16(data[80:82], binStep)
	putAddress(data, 88, testAddress(2))
	putAddress(data, 120, testAddress(3))
	putAddress(data, 152, testAddress(4))
	putAddress(data, 184, testAddress(5))
	return data
}

func testBinArrayData(poolAddress onchainSolana.Address, index int64) []byte {
	data := make([]byte, binArrayDataLength)
	copy(data[:8], binArrayDiscriminator[:])
	binary.LittleEndian.PutUint64(data[8:16], uint64(index))
	putAddress(data, 24, poolAddress)
	return data
}

func putAddress(data []byte, offset int, address onchainSolana.Address) {
	copy(data[offset:offset+32], address[:])
}

func testAddress(value byte) onchainSolana.Address {
	var address onchainSolana.Address
	address[0] = value
	return address
}
