package spltoken

import (
	"context"
	"encoding/binary"
	"testing"

	programToken "github.com/gagliardetto/solana-go/programs/token"
	onchainSolana "github.com/k4k3ru-hub/onchain/go/solana"
)

type stubAccountProvider struct{ account *onchainSolana.Account }

func (p *stubAccountProvider) Account(context.Context, onchainSolana.Address) (*onchainSolana.Account, error) {
	return p.account, nil
}

func TestGetMintMetadata(t *testing.T) {
	data := make([]byte, 82)
	binary.LittleEndian.PutUint64(data[36:44], 1_000_000)
	data[44] = 6
	data[45] = 1
	var owner onchainSolana.Address
	copy(owner[:], programToken.ProgramID[:])
	var mint onchainSolana.Address
	mint[0] = 1
	client, err := NewClient(&stubAccountProvider{account: &onchainSolana.Account{Owner: owner, Data: data}}, stubTransferProvider{}, []onchainSolana.Address{mint})
	if err != nil {
		t.Fatalf("NewClient() returned an unexpected error: %v", err)
	}
	metadata, err := client.GetMintMetadata(nil, mint)
	if err != nil {
		t.Fatalf("GetMintMetadata() returned an unexpected error: %v", err)
	}
	if metadata.Decimals != 6 || metadata.Supply != 1_000_000 {
		t.Fatalf("GetMintMetadata() = %+v, want decimals=6 supply=1000000", metadata)
	}
}

type stubTransferProvider struct{}

func (stubTransferProvider) TransferEvents(context.Context, TransferFilter) ([]TransferEvent, error) {
	return nil, nil
}
