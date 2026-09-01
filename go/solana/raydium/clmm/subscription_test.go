package clmm

import (
	"context"
	"testing"
	"time"

	onchainSolana "github.com/k4k3ru-hub/onchain/go/solana"
)

type swapTestSource struct{ transaction *onchainSolana.Transaction }

func (s swapTestSource) Transaction(context.Context, onchainSolana.Signature) (*onchainSolana.Transaction, error) {
	return s.transaction, nil
}

type swapTestLogs struct {
	values []*onchainSolana.Log
	closed bool
}

func (s *swapTestLogs) Recv(context.Context) (*onchainSolana.Log, error) {
	value := s.values[0]
	s.values = s.values[1:]
	return value, nil
}
func (s *swapTestLogs) Close() { s.closed = true }

func TestSwapSubscriberDerivesConfiguredPoolSwap(t *testing.T) {
	poolAddress, vault0, mint0, vault1, mint1 := testSwapAddress(1), testSwapAddress(2), testSwapAddress(3), testSwapAddress(4), testSwapAddress(5)
	pool := Pool{Address: poolAddress, Token0Vault: vault0, Token0Mint: mint0, Token1Vault: vault1, Token1Mint: mint1}
	timestamp := time.Date(2026, time.September, 1, 1, 2, 3, 0, time.UTC)
	transaction := &onchainSolana.Transaction{
		Signature: testSwapSignature(8), Slot: 99, Timestamp: &timestamp,
		AccountKeys:       []onchainSolana.Address{vault0, vault1},
		PreTokenBalances:  []onchainSolana.TokenBalance{{AccountIndex: 0, Mint: mint0, Amount: "100"}, {AccountIndex: 1, Mint: mint1, Amount: "200"}},
		PostTokenBalances: []onchainSolana.TokenBalance{{AccountIndex: 0, Mint: mint0, Amount: "110"}, {AccountIndex: 1, Mint: mint1, Amount: "180"}},
	}
	logs := &swapTestLogs{values: []*onchainSolana.Log{{Signature: transaction.Signature, Slot: transaction.Slot}}}
	subscriber, err := NewSwapSubscriber(&Client{pools: map[onchainSolana.Address]Pool{poolAddress: pool}}, swapTestSource{transaction: transaction}, func(address onchainSolana.Address) (LogSubscription, error) {
		if address != poolAddress {
			t.Fatalf("subscribe address = %s", address)
		}
		return logs, nil
	})
	if err != nil {
		t.Fatalf("NewSwapSubscriber() error = %v", err)
	}
	subscription, err := subscriber.SubscribeSwaps(context.Background(), poolAddress)
	if err != nil {
		t.Fatalf("SubscribeSwaps() error = %v", err)
	}
	event, err := subscription.Recv(context.Background())
	if err != nil {
		t.Fatalf("Recv() error = %v", err)
	}
	if event.Pool != poolAddress || event.InputMint != mint0 || event.OutputMint != mint1 || event.AmountIn != 10 || event.AmountOut != 20 || event.Slot != 99 || !event.Timestamp.Equal(timestamp) {
		t.Fatalf("Recv() = %+v", event)
	}
	resolved, err := subscriber.Swap(context.Background(), poolAddress, transaction.Signature)
	if err != nil {
		t.Fatalf("Swap() error = %v", err)
	}
	if resolved == nil || resolved.Signature != transaction.Signature || resolved.AmountIn != 10 || resolved.AmountOut != 20 {
		t.Fatalf("Swap() = %+v", resolved)
	}
	subscription.Close()
	if !logs.closed {
		t.Fatal("Close() did not close logs")
	}
}

func testSwapAddress(value byte) onchainSolana.Address {
	var result onchainSolana.Address
	result[0] = value
	return result
}
func testSwapSignature(value byte) onchainSolana.Signature {
	var result onchainSolana.Signature
	result[0] = value
	return result
}
