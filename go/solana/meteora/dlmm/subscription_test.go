package dlmm

import (
	"context"
	"testing"

	onchainSolana "github.com/k4k3ru-hub/onchain/go/solana"
)

type dlmmSwapTestSource struct{ transaction *onchainSolana.Transaction }

func (s dlmmSwapTestSource) Transaction(context.Context, onchainSolana.Signature) (*onchainSolana.Transaction, error) {
	return s.transaction, nil
}

type dlmmSwapTestLogs struct{ value *onchainSolana.Log }

func (s *dlmmSwapTestLogs) Recv(context.Context) (*onchainSolana.Log, error) { return s.value, nil }
func (*dlmmSwapTestLogs) Close()                                             {}

func TestSwapSubscriberDerivesConfiguredPoolSwap(t *testing.T) {
	poolAddress, reserveX, mintX, reserveY, mintY := dlmmSwapAddress(1), dlmmSwapAddress(2), dlmmSwapAddress(3), dlmmSwapAddress(4), dlmmSwapAddress(5)
	transaction := &onchainSolana.Transaction{
		Signature: dlmmSwapSignature(8), Slot: 99, AccountKeys: []onchainSolana.Address{reserveX, reserveY},
		PreTokenBalances:  []onchainSolana.TokenBalance{{AccountIndex: 0, Mint: mintX, Amount: "100"}, {AccountIndex: 1, Mint: mintY, Amount: "200"}},
		PostTokenBalances: []onchainSolana.TokenBalance{{AccountIndex: 0, Mint: mintX, Amount: "110"}, {AccountIndex: 1, Mint: mintY, Amount: "180"}},
	}
	client := &Client{pools: map[onchainSolana.Address]Pool{poolAddress: {Address: poolAddress, ReserveX: reserveX, TokenXMint: mintX, ReserveY: reserveY, TokenYMint: mintY}}}
	subscriber, err := NewSwapSubscriber(client, dlmmSwapTestSource{transaction}, func(onchainSolana.Address) (LogSubscription, error) {
		return &dlmmSwapTestLogs{value: &onchainSolana.Log{Signature: transaction.Signature}}, nil
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
	if event.InputMint != mintX || event.OutputMint != mintY || event.AmountIn != 10 || event.AmountOut != 20 || event.Slot != 99 {
		t.Fatalf("Recv() = %+v", event)
	}
}

func dlmmSwapAddress(value byte) onchainSolana.Address {
	var result onchainSolana.Address
	result[0] = value
	return result
}
func dlmmSwapSignature(value byte) onchainSolana.Signature {
	var result onchainSolana.Signature
	result[0] = value
	return result
}
