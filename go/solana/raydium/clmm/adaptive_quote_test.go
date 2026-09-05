package clmm

import (
	"context"
	"encoding/binary"
	"errors"
	onchainSolana "github.com/k4k3ru-hub/onchain/go/solana"
	"math/big"
	"testing"
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

	return &observingAccounts{stubAccounts: accounts}, poolAddress, token0
}

func TestAdaptiveQuoteExpandsAndMatchesFullSnapshot(t *testing.T) {
	source, pool, mint := adaptiveFixture(t)

	poolData := source.values[pool].Data
	// Current array 0 and the next initialized array -600.
	binary.LittleEndian.PutUint64(poolData[904+7*8:912+7*8], uint64(1)<<63)
	next, err := tickArrayAddress(mainnetProgramID, pool, -600)
	if err != nil {
		t.Fatal(err)
	}
	data := testTickArrayData(pool, -600)
	negativeTick := int32(-600)
	binary.LittleEndian.PutUint32(data[44:48], uint32(negativeTick))
	putUint128LE(data[44+20:44+36], big.NewInt(1))
	data[10124] = 1
	source.values[next] = &onchainSolana.Account{Address: next, Owner: mainnetProgramID, Data: data}

	client, err := NewClient(context.Background(), source, Config{Pools: []onchainSolana.Address{pool}, InitialArrayCount: 1, MaxArrayCount: 2})
	if err != nil {
		t.Fatal(err)
	}
	batch, err := client.QuoteExactInputsWithSlot(context.Background(), pool, []ExactInputRequest{{InputMint: mint, AmountIn: 1_000_000_000}})
	if err != nil {
		t.Fatal(err)
	}
	if len(source.sizes) != 2 || source.sizes[0] != 2 || source.sizes[1] != 3 || batch.Slot != 2 {
		t.Fatalf("sizes=%v slot=%d", source.sizes, batch.Slot)
	}
	full, err := NewClient(context.Background(), source, Config{Pools: []onchainSolana.Address{pool}, InitialArrayCount: 2, MaxArrayCount: 2})
	if err != nil {
		t.Fatal(err)
	}
	expected, err := full.QuoteExactInputsWithSlot(context.Background(), pool, []ExactInputRequest{{InputMint: mint, AmountIn: 1_000_000_000}})
	if err != nil {
		t.Fatal(err)
	}
	if batch.Quotes[0] != expected.Quotes[0] {
		t.Fatalf("adaptive=%+v full=%+v", batch.Quotes, expected.Quotes)
	}
}
func TestAdaptiveQuoteHonorsArrayLimit(t *testing.T) {
	source, pool, mint := adaptiveFixture(t)

	poolData := source.values[pool].Data
	// Current array 0 and the next initialized array -600.
	binary.LittleEndian.PutUint64(poolData[904+7*8:912+7*8], uint64(1)<<63)
	next, err := tickArrayAddress(mainnetProgramID, pool, -600)
	if err != nil {
		t.Fatal(err)
	}
	data := testTickArrayData(pool, -600)
	negativeTick := int32(-600)
	binary.LittleEndian.PutUint32(data[44:48], uint32(negativeTick))
	putUint128LE(data[44+20:44+36], big.NewInt(1))
	data[10124] = 1
	source.values[next] = &onchainSolana.Account{Address: next, Owner: mainnetProgramID, Data: data}

	client, err := NewClient(context.Background(), source, Config{Pools: []onchainSolana.Address{pool}, InitialArrayCount: 1, MaxArrayCount: 1})
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.QuoteExactInputsWithSlot(context.Background(), pool, []ExactInputRequest{{InputMint: mint, AmountIn: 1_000_000_000}})
	if !errors.Is(err, onchainSolana.ErrQuoteArrayLimit) || len(source.sizes) != 1 {
		t.Fatalf("sizes=%v error=%v", source.sizes, err)
	}
}
func TestAdaptiveQuoteDoesNotRetryRequestedNullAccount(t *testing.T) {
	source, pool, mint := adaptiveFixture(t)

	poolData := source.values[pool].Data
	// Current array 0 and the next initialized array -600.
	binary.LittleEndian.PutUint64(poolData[904+7*8:912+7*8], uint64(1)<<63)
	next, err := tickArrayAddress(mainnetProgramID, pool, -600)
	if err != nil {
		t.Fatal(err)
	}
	data := testTickArrayData(pool, -600)
	negativeTick := int32(-600)
	binary.LittleEndian.PutUint32(data[44:48], uint32(negativeTick))
	putUint128LE(data[44+20:44+36], big.NewInt(1))
	data[10124] = 1
	source.values[next] = &onchainSolana.Account{Address: next, Owner: mainnetProgramID, Data: data}

	source.values[next] = nil
	client, err := NewClient(context.Background(), source, Config{Pools: []onchainSolana.Address{pool}, InitialArrayCount: 2, MaxArrayCount: 2})
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.QuoteExactInputsWithSlot(context.Background(), pool, []ExactInputRequest{{InputMint: mint, AmountIn: 1_000_000_000}})
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
