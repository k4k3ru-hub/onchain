package solana

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	solanaSDK "github.com/gagliardetto/solana-go"
	solanaRPC "github.com/gagliardetto/solana-go/rpc"
)

const rpcURLMaxLength = 2048

type RPCConfig struct {
	URL        string
	Commitment Commitment
}

type RPCClient struct {
	config                    RPCConfig
	blockHeightProvider       blockHeightProvider
	blockProvider             blockProvider
	accountProvider           accountProvider
	accountSnapshotProvider   accountSnapshotProvider
	transactionProvider       transactionProvider
	addressSignaturesProvider addressSignaturesProvider
	genesisHashProvider       genesisHashProvider
	slotProvider              slotProvider
	signatureStatusProvider   signatureStatusProvider
}

type sdkRPCAdapter struct {
	client   *solanaRPC.Client
	endpoint string
}

type sanitizedRPCError struct {
	err     error
	message string
}

func (e *sanitizedRPCError) Error() string { return e.message }
func (e *sanitizedRPCError) Unwrap() error { return e.err }

// NewRPCClient creates a Solana JSON-RPC client.
//
// Parameters:
//   - ctx: construction context; nil uses context.Background.
//   - config: Solana RPC configuration.
//
// Returns:
//   - Solana RPC client.
//   - Client creation error.
//
// Version:
//   - 2026-08-22: Added.
func NewRPCClient(ctx context.Context, config RPCConfig) (*RPCClient, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("failed to create solana rpc client: %w", err)
	}

	endpoint := strings.TrimSpace(config.URL)
	adapter := &sdkRPCAdapter{client: solanaRPC.New(endpoint), endpoint: endpoint}
	return composeRPCClient(config, adapter), nil
}

func (a *sdkRPCAdapter) sanitizeError(err error) error {
	if err == nil || a == nil || a.endpoint == "" {
		return err
	}
	redacted := "[redacted_rpc_url]"
	if parsed, parseErr := url.Parse(a.endpoint); parseErr == nil && parsed.Scheme != "" && parsed.Host != "" {
		parsed.User = nil
		parsed.RawQuery = ""
		parsed.Fragment = ""
		redacted = parsed.String()
	}
	return &sanitizedRPCError{err: err, message: strings.ReplaceAll(err.Error(), a.endpoint, redacted)}
}

// Validate validates the Solana RPC configuration.
//
// Returns:
//   - Validation error.
//
// Version:
//   - 2026-08-22: Added.
func (c RPCConfig) Validate() error {
	trimmedURL := strings.TrimSpace(c.URL)
	if trimmedURL == "" {
		return fmt.Errorf("failed to validate solana rpc config: rpc_url=empty")
	}
	actualLength := utf8.RuneCountInString(trimmedURL)
	if actualLength > rpcURLMaxLength {
		return fmt.Errorf(
			"failed to validate solana rpc config: rpc_url=too_long actual_length=%d max_length=%d",
			actualLength,
			rpcURLMaxLength,
		)
	}
	if err := c.Commitment.Validate(); err != nil {
		return fmt.Errorf("failed to validate solana rpc config: %w", err)
	}
	return nil
}

func composeRPCClient(config RPCConfig, dependency interface {
	blockHeightProvider
	blockProvider
	accountProvider
	accountSnapshotProvider
	transactionProvider
	addressSignaturesProvider
	genesisHashProvider
	slotProvider
	signatureStatusProvider
}) *RPCClient {
	client := &RPCClient{config: config}
	if dependency != nil {
		client.blockHeightProvider = dependency
		client.blockProvider = dependency
		client.accountProvider = dependency
		client.accountSnapshotProvider = dependency
		client.transactionProvider = dependency
		client.addressSignaturesProvider = dependency
		client.genesisHashProvider = dependency
		client.slotProvider = dependency
		client.signatureStatusProvider = dependency
	}
	return client
}

func (a *sdkRPCAdapter) getAddressSignatures(ctx context.Context, address Address, commitment Commitment, limit int) ([]addressSignature, error) {
	var publicKey solanaSDK.PublicKey
	copy(publicKey[:], address[:])
	result, err := a.client.GetSignaturesForAddressWithOpts(ctx, publicKey, &solanaRPC.GetSignaturesForAddressOpts{Commitment: solanaRPC.CommitmentType(commitment), Limit: &limit})
	if err != nil {
		return nil, fmt.Errorf("failed to get solana rpc signatures for address: %w", a.sanitizeError(err))
	}
	values := make([]addressSignature, len(result))
	for i, value := range result {
		copy(values[i].signature[:], value.Signature[:])
		values[i].slot = Slot(value.Slot)
		values[i].failed = value.Err != nil
	}
	return values, nil
}

func (a *sdkRPCAdapter) getBlock(ctx context.Context, slot Slot, commitment Commitment) (*Block, error) {
	version := uint64(0)
	result, err := a.client.GetBlockWithOpts(ctx, slot.Uint64(), &solanaRPC.GetBlockOpts{Commitment: solanaRPC.CommitmentType(commitment), TransactionDetails: solanaRPC.TransactionDetailsSignatures, Rewards: pointer(false), MaxSupportedTransactionVersion: &version})
	if err != nil {
		return nil, fmt.Errorf("failed to get solana rpc block: %w", a.sanitizeError(err))
	}
	block := &Block{Slot: slot, ParentSlot: Slot(result.ParentSlot)}
	copy(block.Hash[:], result.Blockhash[:])
	copy(block.PreviousHash[:], result.PreviousBlockhash[:])
	if result.BlockHeight != nil {
		value := BlockHeight(*result.BlockHeight)
		block.Height = &value
	}
	if result.BlockTime != nil {
		value := time.Unix(int64(*result.BlockTime), 0).UTC()
		block.Timestamp = &value
	}
	for _, value := range result.Signatures {
		var signature Signature
		copy(signature[:], value[:])
		block.Signatures = append(block.Signatures, signature)
	}
	return block, nil
}

func (a *sdkRPCAdapter) getAccount(ctx context.Context, address Address, commitment Commitment) (*Account, error) {
	var publicKey solanaSDK.PublicKey
	copy(publicKey[:], address[:])
	result, err := a.client.GetAccountInfoWithOpts(ctx, publicKey, &solanaRPC.GetAccountInfoOpts{Encoding: solanaSDK.EncodingBase64, Commitment: solanaRPC.CommitmentType(commitment)})
	if err != nil {
		return nil, fmt.Errorf("failed to get solana rpc account: %w", a.sanitizeError(err))
	}
	account := &Account{Address: address, Lamports: result.Value.Lamports, Executable: result.Value.Executable, Data: append([]byte(nil), result.GetBinary()...)}
	copy(account.Owner[:], result.Value.Owner[:])
	return account, nil
}

func (a *sdkRPCAdapter) getAccountSnapshot(ctx context.Context, addresses []Address, commitment Commitment) (*AccountSnapshot, error) {
	publicKeys := make([]solanaSDK.PublicKey, len(addresses))
	for i, address := range addresses {
		copy(publicKeys[i][:], address[:])
	}
	result, err := a.client.GetMultipleAccountsWithOpts(ctx, publicKeys, &solanaRPC.GetMultipleAccountsOpts{Encoding: solanaSDK.EncodingBase64, Commitment: solanaRPC.CommitmentType(commitment)})
	if err != nil {
		return nil, fmt.Errorf("failed to get solana rpc account snapshot: %w", a.sanitizeError(err))
	}
	if result == nil {
		return nil, fmt.Errorf("failed to get solana rpc account snapshot: result=null")
	}
	if len(result.Value) != len(addresses) {
		return nil, fmt.Errorf("failed to get solana rpc account snapshot: account_count=invalid actual_length=%d expected_length=%d", len(result.Value), len(addresses))
	}
	accounts := make([]*Account, len(result.Value))
	for i, value := range result.Value {
		if value == nil {
			continue
		}
		account := &Account{Address: addresses[i], Lamports: value.Lamports, Executable: value.Executable, Data: append([]byte(nil), value.Data.GetBinary()...)}
		copy(account.Owner[:], value.Owner[:])
		accounts[i] = account
	}
	return &AccountSnapshot{Slot: Slot(result.Context.Slot), Accounts: accounts}, nil
}

func (a *sdkRPCAdapter) getTransaction(ctx context.Context, signature Signature, commitment Commitment) (*Transaction, error) {
	var sdkSignature solanaSDK.Signature
	copy(sdkSignature[:], signature[:])
	version := uint64(0)
	result, err := a.client.GetTransaction(ctx, sdkSignature, &solanaRPC.GetTransactionOpts{Encoding: solanaSDK.EncodingBase64, Commitment: solanaRPC.CommitmentType(commitment), MaxSupportedTransactionVersion: &version})
	if err != nil {
		return nil, fmt.Errorf("failed to get solana rpc transaction: %w", a.sanitizeError(err))
	}
	if result == nil || result.Transaction == nil {
		return nil, fmt.Errorf("failed to get solana rpc transaction: result=null")
	}
	sdkTransaction, err := result.Transaction.GetTransaction()
	if err != nil {
		return nil, fmt.Errorf("failed to get solana rpc transaction: failed to decode transaction: %w", err)
	}
	transaction := &Transaction{Signature: signature, Slot: Slot(result.Slot), AccountKeys: convertAccountKeys(sdkTransaction, result.Meta)}
	if result.BlockTime != nil {
		value := time.Unix(int64(*result.BlockTime), 0).UTC()
		transaction.Timestamp = &value
	}
	if result.Meta != nil {
		transaction.Fee = result.Meta.Fee
		transaction.Failed = result.Meta.Err != nil
		transaction.Logs = append([]string(nil), result.Meta.LogMessages...)
		transaction.PreTokenBalances = convertTokenBalances(result.Meta.PreTokenBalances)
		transaction.PostTokenBalances = convertTokenBalances(result.Meta.PostTokenBalances)
	}
	return transaction, nil
}

func convertAccountKeys(transaction *solanaSDK.Transaction, meta *solanaRPC.TransactionMeta) []Address {
	if transaction == nil {
		return nil
	}
	publicKeys := append(solanaSDK.PublicKeySlice(nil), transaction.Message.AccountKeys...)
	if meta != nil {
		publicKeys = append(publicKeys, meta.LoadedAddresses.Writable...)
		publicKeys = append(publicKeys, meta.LoadedAddresses.ReadOnly...)
	}
	result := make([]Address, len(publicKeys))
	for i, publicKey := range publicKeys {
		copy(result[i][:], publicKey[:])
	}
	return result
}

func convertTokenBalances(values []solanaRPC.TokenBalance) []TokenBalance {
	result := make([]TokenBalance, 0, len(values))
	for _, value := range values {
		if value.UiTokenAmount == nil {
			continue
		}
		balance := TokenBalance{AccountIndex: value.AccountIndex, Amount: value.UiTokenAmount.Amount, Decimals: value.UiTokenAmount.Decimals}
		copy(balance.Mint[:], value.Mint[:])
		if value.Owner != nil {
			owner := Address{}
			copy(owner[:], value.Owner[:])
			balance.Owner = &owner
		}
		result = append(result, balance)
	}
	return result
}

func pointer[T any](value T) *T { return &value }

func (a *sdkRPCAdapter) getGenesisHash(ctx context.Context) (Hash, error) {
	if a == nil || a.client == nil {
		return Hash{}, fmt.Errorf("failed to get solana rpc genesis hash: sdk_rpc_client=null")
	}
	hash, err := a.client.GetGenesisHash(ctx)
	if err != nil {
		return Hash{}, fmt.Errorf("failed to get solana rpc genesis hash: %w", a.sanitizeError(err))
	}
	var result Hash
	copy(result[:], hash[:])
	return result, nil
}

func (a *sdkRPCAdapter) getBlockHeight(ctx context.Context, commitment Commitment) (BlockHeight, error) {
	if a == nil || a.client == nil {
		return 0, fmt.Errorf("failed to get solana rpc block height: sdk_rpc_client=null")
	}

	blockHeight, err := a.client.GetBlockHeight(ctx, solanaRPC.CommitmentType(commitment))
	if err != nil {
		return 0, fmt.Errorf("failed to get solana rpc block height: %w", a.sanitizeError(err))
	}
	return BlockHeight(blockHeight), nil
}

func (a *sdkRPCAdapter) getSlot(ctx context.Context, commitment Commitment) (Slot, error) {
	if a == nil || a.client == nil {
		return 0, fmt.Errorf("failed to get solana rpc slot: sdk_rpc_client=null")
	}

	slot, err := a.client.GetSlot(ctx, solanaRPC.CommitmentType(commitment))
	if err != nil {
		return 0, fmt.Errorf("failed to get solana rpc slot: %w", a.sanitizeError(err))
	}
	return Slot(slot), nil
}

func (a *sdkRPCAdapter) getSignatureStatuses(ctx context.Context, signatures []Signature) ([]*rpcSignatureStatus, error) {
	if a == nil || a.client == nil {
		return nil, fmt.Errorf("failed to get solana rpc signature statuses: sdk_rpc_client=null")
	}

	sdkSignatures := make([]solanaSDK.Signature, len(signatures))
	for i, signature := range signatures {
		copy(sdkSignatures[i][:], signature[:])
	}

	result, err := a.client.GetSignatureStatuses(ctx, true, sdkSignatures...)
	if err != nil {
		return nil, fmt.Errorf("failed to get solana rpc signature statuses: %w", a.sanitizeError(err))
	}
	if result == nil {
		return nil, fmt.Errorf("failed to get solana rpc signature statuses: response=null")
	}

	statuses := make([]*rpcSignatureStatus, len(result.Value))
	for i, status := range result.Value {
		if status == nil {
			continue
		}
		statuses[i] = &rpcSignatureStatus{
			slot:               Slot(status.Slot),
			confirmations:      status.Confirmations,
			confirmationStatus: Commitment(status.ConfirmationStatus),
			failed:             status.Err != nil,
		}
	}
	return statuses, nil
}
