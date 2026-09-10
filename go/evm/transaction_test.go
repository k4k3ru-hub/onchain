package evm

import (
	"context"
	"errors"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
)

type transactionSenderStub struct {
	transaction *types.Transaction
	err         error
}

func (s *transactionSenderStub) SendTransaction(_ context.Context, transaction *types.Transaction) error {
	s.transaction = transaction
	return s.err
}

func TestHTTPClientSendTransaction(t *testing.T) {
	sender := new(transactionSenderStub)
	client := &HTTPClient{transactionSender: sender}
	transaction := signedDynamicFeeTransaction(t)
	hash, err := client.SendTransaction(t.Context(), transaction)
	if err != nil {
		t.Fatalf("SendTransaction() error = %v, want nil", err)
	}
	if hash != transaction.Hash() {
		t.Fatalf("SendTransaction() hash = %s, want %s", hash.Hex(), transaction.Hash().Hex())
	}
	if sender.transaction != transaction {
		t.Fatal("SendTransaction() did not pass the transaction to the provider")
	}
}

func TestHTTPClientSendTransactionValidatesInput(t *testing.T) {
	unsigned := types.NewTx(&types.DynamicFeeTx{
		ChainID: big.NewInt(84532), GasTipCap: big.NewInt(1), GasFeeCap: big.NewInt(2), Gas: 21_000,
		To: func() *common.Address { value := common.Address{1}; return &value }(), Value: new(big.Int),
	})
	tests := []struct {
		name        string
		client      *HTTPClient
		transaction *types.Transaction
	}{
		{name: "nil client", transaction: signedDynamicFeeTransaction(t)},
		{name: "missing provider", client: new(HTTPClient), transaction: signedDynamicFeeTransaction(t)},
		{name: "nil transaction", client: &HTTPClient{transactionSender: new(transactionSenderStub)}},
		{name: "unsigned transaction", client: &HTTPClient{transactionSender: new(transactionSenderStub)}, transaction: unsigned},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := test.client.SendTransaction(t.Context(), test.transaction); err == nil {
				t.Fatal("SendTransaction() error = nil")
			}
		})
	}
}

func TestHTTPClientSendTransactionPropagatesErrors(t *testing.T) {
	sendErr := errors.New("send failed")
	client := &HTTPClient{transactionSender: &transactionSenderStub{err: sendErr}}
	if _, err := client.SendTransaction(t.Context(), signedDynamicFeeTransaction(t)); !errors.Is(err, sendErr) {
		t.Fatalf("SendTransaction() error = %v, want wrapped send error", err)
	}
	canceledContext, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := client.SendTransaction(canceledContext, signedDynamicFeeTransaction(t)); !errors.Is(err, context.Canceled) {
		t.Fatalf("SendTransaction() canceled error = %v", err)
	}
}

func signedDynamicFeeTransaction(t *testing.T) *types.Transaction {
	t.Helper()
	privateKey, err := crypto.GenerateKey()
	if err != nil {
		t.Fatalf("GenerateKey() error = %v", err)
	}
	to := common.Address{1}
	unsigned := types.NewTx(&types.DynamicFeeTx{
		ChainID: big.NewInt(84532), Nonce: 1, GasTipCap: big.NewInt(1), GasFeeCap: big.NewInt(2),
		Gas: 21_000, To: &to, Value: new(big.Int),
	})
	transaction, err := types.SignTx(unsigned, types.LatestSignerForChainID(unsigned.ChainId()), privateKey)
	if err != nil {
		t.Fatalf("SignTx() error = %v", err)
	}
	return transaction
}
