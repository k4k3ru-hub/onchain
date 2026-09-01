package solana

import (
	"context"
	"testing"
)

type wsTestProvider struct {
	address    Address
	commitment Commitment
}

func (p *wsTestProvider) subscribeLogs(address Address, commitment Commitment) (logReceiver, error) {
	p.address, p.commitment = address, commitment
	return wsTestReceiver{}, nil
}

type wsTestReceiver struct{}

func (wsTestReceiver) Recv(context.Context) (*Log, error) { return &Log{}, nil }
func (wsTestReceiver) Unsubscribe()                       {}

func TestSubscribeLogsUsesConfiguredCommitment(t *testing.T) {
	provider := &wsTestProvider{}
	client := &WSClient{provider: provider, commitment: CommitmentConfirmed}
	address := Address{1}

	subscription, err := client.SubscribeLogs(address)
	if err != nil {
		t.Fatalf("SubscribeLogs() error = %v", err)
	}
	defer subscription.Close()
	if provider.address != address || provider.commitment != CommitmentConfirmed {
		t.Fatalf("subscribeLogs() address = %s commitment = %q", provider.address, provider.commitment)
	}
}
