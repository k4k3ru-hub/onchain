package dlmm

import (
	"context"
	"encoding/binary"
	"errors"
	"testing"

	onchainSolana "github.com/k4k3ru-hub/onchain/go/solana"
)

type observingAccounts struct {
	*stubAccounts
	sizes  []int
	mutate func(int)
}

func (o *observingAccounts) AccountSnapshot(ctx context.Context, addresses []onchainSolana.Address) (*onchainSolana.AccountSnapshot, error) {
	o.sizes = append(o.sizes, len(addresses))
	if o.mutate != nil {
		o.mutate(len(o.sizes))
	}
	snapshot, err := o.stubAccounts.AccountSnapshot(ctx, addresses)
	if err == nil {
		snapshot.Slot = onchainSolana.Slot(len(o.sizes))
	}
	return snapshot, err
}
func adaptiveFixture(t *testing.T) (*observingAccounts, onchainSolana.Address, onchainSolana.Address) {
	t.Helper()
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

	return &observingAccounts{stubAccounts: accounts}, poolAddress, testAddress(2)
}

func TestAdaptiveQuoteExpandsAndMatchesFullSnapshot(t *testing.T) {
	source, pool, mint := adaptiveFixture(t)

	// Include array 0 below the active array 1. Reduce array 1 reserves so the quote must expand.
	poolData := source.values[pool].Data
	binary.LittleEndian.PutUint64(poolData[584+8*8:592+8*8], 3)
	first, err := binArrayAddress(mainnetProgramID, pool, 1)
	if err != nil {
		t.Fatal(err)
	}
	binary.LittleEndian.PutUint64(source.values[first].Data[56+5*binDataLength+8:], 100)
	next, err := binArrayAddress(mainnetProgramID, pool, 0)
	if err != nil {
		t.Fatal(err)
	}
	data := testBinArrayData(pool, 0)
	binary.LittleEndian.PutUint64(data[56+69*binDataLength+8:], 10_000_000)
	binary.LittleEndian.PutUint64(data[56+69*binDataLength+24:], 1)
	source.values[next] = &onchainSolana.Account{Address: next, Owner: mainnetProgramID, Data: data}

	client, err := NewClient(context.Background(), source, Config{Pools: []onchainSolana.Address{pool}, InitialArrayCount: 1, MaxArrayCount: 2})
	if err != nil {
		t.Fatal(err)
	}
	source.mutate = func(attempt int) {
		if attempt == 2 {
			binary.LittleEndian.PutUint64(source.values[first].Data[56+5*binDataLength+8:], 500_000)
		}
	}
	batch, err := client.QuoteExactInputsWithSlot(context.Background(), pool, []ExactInputRequest{{InputMint: mint, AmountIn: 1_000_000}})
	if err != nil {
		t.Fatal(err)
	}
	if len(source.sizes) != 2 || source.sizes[0] != 3 || source.sizes[1] != 4 || batch.Slot != 2 {
		t.Fatalf("sizes=%v slot=%d", source.sizes, batch.Slot)
	}
	full, err := NewClient(context.Background(), source, Config{Pools: []onchainSolana.Address{pool}, InitialArrayCount: 2, MaxArrayCount: 2})
	if err != nil {
		t.Fatal(err)
	}
	expected, err := full.QuoteExactInputsWithSlot(context.Background(), pool, []ExactInputRequest{{InputMint: mint, AmountIn: 1_000_000}})
	if err != nil {
		t.Fatal(err)
	}
	if batch.Quotes[0] != expected.Quotes[0] {
		t.Fatalf("adaptive=%+v full=%+v", batch.Quotes, expected.Quotes)
	}
}
func TestAdaptiveQuoteHonorsArrayLimit(t *testing.T) {
	source, pool, mint := adaptiveFixture(t)

	// Include array 0 below the active array 1. Reduce array 1 reserves so the quote must expand.
	poolData := source.values[pool].Data
	binary.LittleEndian.PutUint64(poolData[584+8*8:592+8*8], 3)
	first, err := binArrayAddress(mainnetProgramID, pool, 1)
	if err != nil {
		t.Fatal(err)
	}
	binary.LittleEndian.PutUint64(source.values[first].Data[56+5*binDataLength+8:], 100)
	next, err := binArrayAddress(mainnetProgramID, pool, 0)
	if err != nil {
		t.Fatal(err)
	}
	data := testBinArrayData(pool, 0)
	binary.LittleEndian.PutUint64(data[56+69*binDataLength+8:], 10_000_000)
	binary.LittleEndian.PutUint64(data[56+69*binDataLength+24:], 1)
	source.values[next] = &onchainSolana.Account{Address: next, Owner: mainnetProgramID, Data: data}

	client, err := NewClient(context.Background(), source, Config{Pools: []onchainSolana.Address{pool}, InitialArrayCount: 1, MaxArrayCount: 1})
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.QuoteExactInputsWithSlot(context.Background(), pool, []ExactInputRequest{{InputMint: mint, AmountIn: 1_000_000}})
	if !errors.Is(err, onchainSolana.ErrQuoteArrayLimit) || len(source.sizes) != 1 {
		t.Fatalf("sizes=%v error=%v", source.sizes, err)
	}
}
func TestAdaptiveQuoteDoesNotRetryRequestedNullAccount(t *testing.T) {
	source, pool, mint := adaptiveFixture(t)

	// Include array 0 below the active array 1. Reduce array 1 reserves so the quote must expand.
	poolData := source.values[pool].Data
	binary.LittleEndian.PutUint64(poolData[584+8*8:592+8*8], 3)
	first, err := binArrayAddress(mainnetProgramID, pool, 1)
	if err != nil {
		t.Fatal(err)
	}
	binary.LittleEndian.PutUint64(source.values[first].Data[56+5*binDataLength+8:], 100)
	next, err := binArrayAddress(mainnetProgramID, pool, 0)
	if err != nil {
		t.Fatal(err)
	}
	data := testBinArrayData(pool, 0)
	binary.LittleEndian.PutUint64(data[56+69*binDataLength+8:], 10_000_000)
	binary.LittleEndian.PutUint64(data[56+69*binDataLength+24:], 1)
	source.values[next] = &onchainSolana.Account{Address: next, Owner: mainnetProgramID, Data: data}

	source.values[next] = nil
	client, err := NewClient(context.Background(), source, Config{Pools: []onchainSolana.Address{pool}, InitialArrayCount: 2, MaxArrayCount: 2})
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.QuoteExactInputsWithSlot(context.Background(), pool, []ExactInputRequest{{InputMint: mint, AmountIn: 1_000_000}})
	if err == nil || errors.Is(err, onchainSolana.ErrQuoteArrayLimit) || len(source.sizes) != 1 {
		t.Fatalf("sizes=%v error=%v", source.sizes, err)
	}
}
func TestAdaptiveQuoteStopsOnCancelledContext(t *testing.T) {
	source, pool, mint := adaptiveFixture(t)
	client, err := NewClient(context.Background(), source, Config{Pools: []onchainSolana.Address{pool}})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = client.QuoteExactInputsWithSlot(ctx, pool, []ExactInputRequest{{InputMint: mint, AmountIn: 1}})
	if !errors.Is(err, context.Canceled) || len(source.sizes) != 0 {
		t.Fatalf("sizes=%v error=%v", source.sizes, err)
	}
}

func TestAdaptiveQuoteStopsAfterFiveSnapshots(t *testing.T) {
	source, pool, mint := adaptiveFixture(t)
	poolData := source.values[pool].Data
	for index := int32(1); index >= -9; index-- {
		position := index + bitmapCenter
		offset := 584 + int(position/64)*8
		bits := binary.LittleEndian.Uint64(poolData[offset : offset+8])
		bits |= uint64(1) << uint(position%64)
		binary.LittleEndian.PutUint64(poolData[offset:offset+8], bits)
		address, err := binArrayAddress(mainnetProgramID, pool, int64(index))
		if err != nil {
			t.Fatal(err)
		}
		source.values[address] = &onchainSolana.Account{Address: address, Owner: mainnetProgramID, Data: testBinArrayData(pool, int64(index))}
	}
	client, err := NewClient(context.Background(), source, Config{Pools: []onchainSolana.Address{pool}, InitialArrayCount: 1, MaxArrayCount: 16})
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.QuoteExactInputsWithSlot(context.Background(), pool, []ExactInputRequest{{InputMint: mint, AmountIn: 100}})
	if !errors.Is(err, onchainSolana.ErrQuoteSnapshotLimit) || len(source.sizes) != 5 {
		t.Fatalf("sizes=%v error=%v", source.sizes, err)
	}
}

func TestAdaptiveQuoteRecentersAfterPoolMoves(t *testing.T) {
	source, pool, mint := adaptiveFixture(t)
	// Only array 1 exists during construction. The first snapshot moves into array 2.
	next, err := binArrayAddress(mainnetProgramID, pool, 2)
	if err != nil {
		t.Fatal(err)
	}
	data := testBinArrayData(pool, 2)
	binary.LittleEndian.PutUint64(data[56+8:], 10_000_000)
	binary.LittleEndian.PutUint64(data[56+24:], 2)
	source.values[next] = &onchainSolana.Account{Address: next, Owner: mainnetProgramID, Data: data}
	client, err := NewClient(context.Background(), source, Config{Pools: []onchainSolana.Address{pool}, InitialArrayCount: 1, MaxArrayCount: 1})
	if err != nil {
		t.Fatal(err)
	}
	source.mutate = func(attempt int) {
		if attempt == 1 {
			binary.LittleEndian.PutUint32(source.values[pool].Data[76:80], 140)
			binary.LittleEndian.PutUint64(source.values[pool].Data[584+8*8:592+8*8], 4)
		}
	}
	batch, err := client.QuoteExactInputsWithSlot(context.Background(), pool, []ExactInputRequest{{InputMint: mint, AmountIn: 100}})
	if err != nil {
		t.Fatal(err)
	}
	if len(source.sizes) != 2 || source.sizes[0] != 3 || source.sizes[1] != 3 || batch.Slot != 2 {
		t.Fatalf("sizes=%v slot=%d", source.sizes, batch.Slot)
	}
	if batch.Quotes[0].AmountOut <= 100 {
		t.Fatalf("quote did not use refreshed price: %+v", batch.Quotes[0])
	}
}
