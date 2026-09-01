package cpmm

import (
	"context"
	"testing"

	onchainSolana "github.com/k4k3ru-hub/onchain/go/solana"
)

type cpmmSwapTestSource struct{ transaction *onchainSolana.Transaction }

func (s cpmmSwapTestSource) Transaction(context.Context, onchainSolana.Signature) (*onchainSolana.Transaction, error) {
	return s.transaction, nil
}

type cpmmSwapTestLogs struct{ value *onchainSolana.Log }

func (s *cpmmSwapTestLogs) Recv(context.Context) (*onchainSolana.Log, error) { return s.value, nil }
func (*cpmmSwapTestLogs) Close()                                             {}

func TestSwapSubscriberDerivesConfiguredPoolSwap(t *testing.T) {
	poolAddress, vault0, mint0, vault1, mint1 := cpmmSwapAddress(1), cpmmSwapAddress(2), cpmmSwapAddress(3), cpmmSwapAddress(4), cpmmSwapAddress(5)
	transaction := &onchainSolana.Transaction{
		Signature: cpmmSwapSignature(8), Slot: 99, AccountKeys: []onchainSolana.Address{vault0, vault1},
		PreTokenBalances:  []onchainSolana.TokenBalance{{AccountIndex: 0, Mint: mint0, Amount: "100"}, {AccountIndex: 1, Mint: mint1, Amount: "200"}},
		PostTokenBalances: []onchainSolana.TokenBalance{{AccountIndex: 0, Mint: mint0, Amount: "90"}, {AccountIndex: 1, Mint: mint1, Amount: "220"}},
	}
	client := &Client{pools: map[onchainSolana.Address]Pool{poolAddress: {Address: poolAddress, Token0Vault: vault0, Token0Mint: mint0, Token1Vault: vault1, Token1Mint: mint1}}}
	subscriber, err := NewSwapSubscriber(client, cpmmSwapTestSource{transaction}, func(onchainSolana.Address) (LogSubscription, error) {
		return &cpmmSwapTestLogs{value: &onchainSolana.Log{Signature: transaction.Signature}}, nil
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
	if event.InputMint != mint1 || event.OutputMint != mint0 || event.AmountIn != 20 || event.AmountOut != 10 || event.Slot != 99 {
		t.Fatalf("Recv() = %+v", event)
	}
}

func cpmmSwapAddress(value byte) onchainSolana.Address {
	var result onchainSolana.Address
	result[0] = value
	return result
}
func cpmmSwapSignature(value byte) onchainSolana.Signature {
	var result onchainSolana.Signature
	result[0] = value
	return result
}
