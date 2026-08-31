package cpmm

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

func (s *stubAccounts) AccountSnapshot(_ context.Context, addresses []onchainSolana.Address) (*onchainSolana.AccountSnapshot, error) {
	s.snapshotCalls++
	accounts := make([]*onchainSolana.Account, len(addresses))
	for index, address := range addresses {
		accounts[index] = s.values[address]
	}
	return &onchainSolana.AccountSnapshot{Slot: 42, Accounts: accounts}, nil
}

func (s *stubAccounts) Account(_ context.Context, address onchainSolana.Address) (*onchainSolana.Account, error) {
	return s.values[address], nil
}

func TestNewClientDiscoversConfiguredPoolAndQuotesExactInput(t *testing.T) {
	poolAddress := testAddress(1)
	configAddress := testAddress(2)
	vault0 := testAddress(3)
	vault1 := testAddress(4)
	mint0 := testAddress(5)
	mint1 := testAddress(6)

	poolData := make([]byte, poolDataLength)
	copy(poolData[:8], poolDiscriminator[:])
	putAddress(poolData, 8, configAddress)
	putAddress(poolData, 72, vault0)
	putAddress(poolData, 104, vault1)
	putAddress(poolData, 168, mint0)
	putAddress(poolData, 200, mint1)
	putAddress(poolData, 232, splTokenProgramID)
	putAddress(poolData, 264, splTokenProgramID)
	poolData[331] = 6
	poolData[332] = 6

	configData := make([]byte, ammConfigDataLength)
	copy(configData[:8], configDiscriminator[:])
	binary.LittleEndian.PutUint64(configData[12:20], 2_500)

	accounts := &stubAccounts{values: map[onchainSolana.Address]*onchainSolana.Account{
		poolAddress:   {Address: poolAddress, Owner: mainnetProgramID, Data: poolData},
		configAddress: {Address: configAddress, Owner: mainnetProgramID, Data: configData},
		vault0:        tokenAccount(vault0, 1_000_000),
		vault1:        tokenAccount(vault1, 2_000_000),
	}}

	client, err := NewClient(context.Background(), accounts, Config{Pools: []onchainSolana.Address{poolAddress}})
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	quote, err := client.QuoteExactInput(context.Background(), poolAddress, mint0, 100_000)
	if err != nil {
		t.Fatalf("QuoteExactInput() error = %v", err)
	}
	if quote.AmountOut != 181_404 || quote.TradeFee != 250 || quote.OutputMint != mint1 {
		t.Fatalf("QuoteExactInput() = %+v", quote)
	}
	batch, err := client.QuoteExactInputsWithSlot(context.Background(), poolAddress, []ExactInputRequest{
		{InputMint: mint0, AmountIn: 100_000},
		{InputMint: mint1, AmountIn: 100_000},
	})
	if err != nil {
		t.Fatalf("QuoteExactInputsWithSlot() error = %v", err)
	}
	if batch.Slot != 42 || len(batch.Quotes) != 2 || batch.Quotes[0].AmountOut == 0 || batch.Quotes[1].AmountOut == 0 {
		t.Fatalf("QuoteExactInputsWithSlot() = %+v", batch)
	}
	if accounts.snapshotCalls != 2 {
		t.Fatalf("AccountSnapshot() calls = %d, want 2", accounts.snapshotCalls)
	}
}

func TestNewClientRejectsPoolOwnedByAnotherProgram(t *testing.T) {
	poolAddress := testAddress(1)
	accounts := &stubAccounts{values: map[onchainSolana.Address]*onchainSolana.Account{
		poolAddress: {Address: poolAddress, Owner: testAddress(2), Data: make([]byte, poolDataLength)},
	}}
	if _, err := NewClient(context.Background(), accounts, Config{Pools: []onchainSolana.Address{poolAddress}}); err == nil {
		t.Fatal("NewClient() error = nil")
	}
}

func TestQuoteAppliesCreatorFeeOnOutput(t *testing.T) {
	amountOut, tradeFee, creatorFee, err := quote(100_000, 1_000_000, 2_000_000, 2_500, 1_000, false)
	if err != nil {
		t.Fatalf("quote() error = %v", err)
	}
	if amountOut != 181_222 || tradeFee != 250 || creatorFee != 182 {
		t.Fatalf("quote() = amount_out=%d trade_fee=%d creator_fee=%d", amountOut, tradeFee, creatorFee)
	}
}

func tokenAccount(address onchainSolana.Address, amount uint64) *onchainSolana.Account {
	data := make([]byte, tokenAccountMinSize)
	binary.LittleEndian.PutUint64(data[tokenAmountOffset:tokenAmountOffset+8], amount)
	return &onchainSolana.Account{Address: address, Data: data}
}

func putAddress(data []byte, offset int, address onchainSolana.Address) {
	copy(data[offset:offset+32], address[:])
}

func testAddress(value byte) onchainSolana.Address {
	var address onchainSolana.Address
	address[0] = value
	return address
}
