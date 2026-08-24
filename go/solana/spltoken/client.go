package spltoken

import (
	"context"
	"fmt"

	bin "github.com/gagliardetto/binary"
	solanaSDK "github.com/gagliardetto/solana-go"
	programToken "github.com/gagliardetto/solana-go/programs/token"
	onchainSolana "github.com/k4k3ru-hub/onchain/go/solana"
)

type accountProvider interface {
	Account(context.Context, onchainSolana.Address) (*onchainSolana.Account, error)
}

type standardRPCClient interface {
	accountProvider
	transactionSource
}

type Client struct {
	accountProvider    accountProvider
	transferProvider   TransferProvider
	transferSubscriber TransferSubscriber
	mints              []onchainSolana.Address
}

// WithTransferSubscriber configures real-time SPL Token transfer subscriptions.
//
// Parameters:
//   - subscriber: transfer subscription provider.
//
// Returns:
//   - Receiver for method chaining.
//
// Version:
//   - 2026-08-22: Added.
func (c *Client) WithTransferSubscriber(subscriber TransferSubscriber) *Client {
	if c != nil {
		c.transferSubscriber = subscriber
	}
	return c
}

// SubscribeTransfers subscribes to SPL Token transfers using the configured subscriber.
func (c *Client) SubscribeTransfers(ctx context.Context, filter TransferFilter) (*TransferSubscription, error) {
	if c == nil || c.transferSubscriber == nil {
		return nil, fmt.Errorf("failed to subscribe spl token transfers: transfer_subscriber=null")
	}
	return c.transferSubscriber.SubscribeTransfers(ctx, filter)
}

type MintMetadata struct {
	Address         onchainSolana.Address
	Decimals        uint8
	Supply          uint64
	MintAuthority   *onchainSolana.Address
	FreezeAuthority *onchainSolana.Address
}

// NewClient creates an SPL Token client.
//
// Parameters:
//   - provider: Solana account provider.
//   - mints: supported SPL Token mint addresses.
//
// Returns:
//   - SPL Token client.
//   - Construction error.
//
// Version:
//   - 2026-08-22: Added.
func NewClient(provider accountProvider, transferProvider TransferProvider, mints []onchainSolana.Address) (*Client, error) {
	if provider == nil {
		return nil, fmt.Errorf("failed to create spl token client: account_provider=null")
	}
	for _, mint := range mints {
		if mint.IsZero() {
			return nil, fmt.Errorf("failed to create spl token client: mint=empty")
		}
	}
	if transferProvider == nil {
		return nil, fmt.Errorf("failed to create spl token client: transfer_provider=null")
	}
	return &Client{accountProvider: provider, transferProvider: transferProvider, mints: append([]onchainSolana.Address(nil), mints...)}, nil
}

// NewDefaultClient creates an SPL Token client backed entirely by standard Solana RPC.
//
// Parameters:
//   - provider: standard Solana RPC client.
//   - mints: supported SPL Token mint addresses.
//
// Returns:
//   - SPL Token client.
//   - Construction error.
//
// Version:
//   - 2026-08-22: Added.
func NewDefaultClient(provider standardRPCClient, mints []onchainSolana.Address) (*Client, error) {
	transferProvider, err := NewStandardTransferProvider(provider)
	if err != nil {
		return nil, fmt.Errorf("failed to create default spl token client: %w", err)
	}
	client, err := NewClient(provider, transferProvider, mints)
	if err != nil {
		return nil, fmt.Errorf("failed to create default spl token client: %w", err)
	}
	return client, nil
}

// Mints returns configured SPL Token mint addresses.
func (c *Client) Mints() []onchainSolana.Address {
	if c == nil {
		return nil
	}
	return append([]onchainSolana.Address(nil), c.mints...)
}

// GetMintMetadata returns on-chain SPL Mint metadata.
func (c *Client) GetMintMetadata(ctx context.Context, mint onchainSolana.Address) (*MintMetadata, error) {
	if c == nil || c.accountProvider == nil {
		return nil, fmt.Errorf("failed to get spl token mint metadata: client=null")
	}
	if mint.IsZero() {
		return nil, fmt.Errorf("failed to get spl token mint metadata: mint=empty")
	}
	account, err := c.accountProvider.Account(ctx, mint)
	if err != nil {
		return nil, fmt.Errorf("failed to get spl token mint metadata: %w", err)
	}
	if account.Owner.String() != programToken.ProgramID.String() {
		return nil, fmt.Errorf("failed to get spl token mint metadata: account_owner=invalid")
	}
	var decoded programToken.Mint
	if err := bin.NewBinDecoder(account.Data).Decode(&decoded); err != nil {
		return nil, fmt.Errorf("failed to get spl token mint metadata: failed to decode mint: %w", err)
	}
	metadata := &MintMetadata{Address: mint, Decimals: decoded.Decimals, Supply: decoded.Supply}
	if decoded.MintAuthority != nil {
		value := addressFromSDK(*decoded.MintAuthority)
		metadata.MintAuthority = &value
	}
	if decoded.FreezeAuthority != nil {
		value := addressFromSDK(*decoded.FreezeAuthority)
		metadata.FreezeAuthority = &value
	}
	return metadata, nil
}

func addressFromSDK(value solanaSDK.PublicKey) onchainSolana.Address {
	var address onchainSolana.Address
	copy(address[:], value[:])
	return address
}
