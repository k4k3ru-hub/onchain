// call_test.go
package evm

import (
	"context"
	"errors"
	"math/big"
	"reflect"
	"testing"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
)

var errTestContractCall = errors.New("test contract call error")

type fakeContractCaller struct {
	ctx         context.Context
	call        ethereum.CallMsg
	blockNumber *big.Int
	result      []byte
	err         error
}

func (f *fakeContractCaller) CallContract(ctx context.Context, call ethereum.CallMsg, blockNumber *big.Int) ([]byte, error) {
	f.ctx = ctx
	f.call = call
	f.blockNumber = blockNumber
	return f.result, f.err
}

func TestCallContractDelegatesCall(t *testing.T) {
	t.Parallel()

	type contextKey string
	ctx := context.WithValue(context.Background(), contextKey("key"), "value")
	to := common.HexToAddress("0x0000000000000000000000000000000000000001")
	call := ethereum.CallMsg{
		From:       common.HexToAddress("0x0000000000000000000000000000000000000002"),
		To:         &to,
		Gas:        12345,
		GasPrice:   big.NewInt(2),
		GasFeeCap:  big.NewInt(3),
		GasTipCap:  big.NewInt(4),
		Value:      big.NewInt(5),
		Data:       []byte{0xde, 0xad, 0xbe, 0xef},
		AccessList: nil,
	}
	blockNumber := big.NewInt(123)
	raw := []byte{0x01, 0x02, 0x03}
	fake := &fakeContractCaller{result: raw}
	client := &HTTPClient{contractCaller: fake}

	got, err := client.CallContract(ctx, call, blockNumber)
	if err != nil {
		t.Fatalf("CallContract() error = %v", err)
	}
	if fake.ctx != ctx {
		t.Fatal("CallContract() delegated context differs from input")
	}
	if !reflect.DeepEqual(fake.call, call) {
		t.Errorf("CallContract() delegated call = %#v, want %#v", fake.call, call)
	}
	if fake.blockNumber != blockNumber {
		t.Fatal("CallContract() delegated block number differs from input")
	}
	if !reflect.DeepEqual(got, raw) {
		t.Errorf("CallContract() result = %x, want %x", got, raw)
	}
	if len(got) > 0 && &got[0] != &raw[0] {
		t.Fatal("CallContract() copied or replaced raw response bytes")
	}
}

func TestCallContractAllowsNilBlockNumber(t *testing.T) {
	t.Parallel()

	fake := &fakeContractCaller{result: []byte{0x01}}
	client := &HTTPClient{contractCaller: fake}

	_, err := client.CallContract(context.Background(), ethereum.CallMsg{}, nil)
	if err != nil {
		t.Fatalf("CallContract() error = %v", err)
	}
	if fake.blockNumber != nil {
		t.Errorf("CallContract() delegated block number = %v, want nil", fake.blockNumber)
	}
}

func TestCallContractHandlesNilContext(t *testing.T) {
	t.Parallel()

	fake := &fakeContractCaller{}
	client := &HTTPClient{contractCaller: fake}

	_, err := client.CallContract(nil, ethereum.CallMsg{}, nil)
	if err != nil {
		t.Fatalf("CallContract() error = %v", err)
	}
	if fake.ctx == nil {
		t.Fatal("CallContract() delegated context = nil, want context.Background()")
	}
}

func TestCallContractRejectsNilReceiver(t *testing.T) {
	t.Parallel()

	var client *HTTPClient
	_, err := client.CallContract(context.Background(), ethereum.CallMsg{}, nil)
	if err == nil {
		t.Fatal("CallContract() error = nil, want error")
	}
	if err.Error() != "failed to call evm contract: evm_http_client=null" {
		t.Errorf("CallContract() error = %q", err.Error())
	}
}

func TestCallContractRejectsMissingContractCaller(t *testing.T) {
	t.Parallel()

	client := &HTTPClient{}
	_, err := client.CallContract(context.Background(), ethereum.CallMsg{}, nil)
	if err == nil {
		t.Fatal("CallContract() error = nil, want error")
	}
	if err.Error() != "failed to call evm contract: http_eth_client=null" {
		t.Errorf("CallContract() error = %q", err.Error())
	}
}

func TestCallContractWrapsRPCError(t *testing.T) {
	t.Parallel()

	client := &HTTPClient{contractCaller: &fakeContractCaller{err: errTestContractCall}}
	_, err := client.CallContract(context.Background(), ethereum.CallMsg{}, nil)
	if err == nil {
		t.Fatal("CallContract() error = nil, want error")
	}
	if err.Error() != "failed to call evm contract: test contract call error" {
		t.Errorf("CallContract() error = %q", err.Error())
	}
	if !errors.Is(err, errTestContractCall) {
		t.Fatalf("errors.Is() = false, want wrapped error: %v", err)
	}
}

func TestComposeHTTPClientComposesDependencies(t *testing.T) {
	t.Parallel()

	ethClient := new(ethclient.Client)
	client := composeHTTPClient(HTTPConfig{}, ethClient)

	if client.contractCaller != ethClient {
		t.Fatal("HTTP contract-call dependency was not composed")
	}
	if client.gasEstimator != ethClient {
		t.Fatal("HTTP gas-estimator dependency was not composed")
	}
	if client.pendingNonceProvider != ethClient {
		t.Fatal("HTTP pending-nonce dependency was not composed")
	}
	if client.gasTipCapSuggester != ethClient {
		t.Fatal("HTTP gas-tip-cap dependency was not composed")
	}
	if client.blockHeaderByHasher != ethClient {
		t.Fatal("HTTP block-header dependency was not composed")
	}
	if client.blockNumberer != ethClient {
		t.Fatal("HTTP block-number dependency was not composed")
	}
	if client.logFilterer != ethClient {
		t.Fatal("HTTP log-filter dependency was not composed")
	}
	if client.receiptProvider != ethClient {
		t.Fatal("HTTP transaction-receipt dependency was not composed")
	}
	if client.chainIDProvider != ethClient {
		t.Fatal("HTTP chain-ID dependency was not composed")
	}
	if client.clientCloser != ethClient {
		t.Fatal("HTTP close dependency was not composed")
	}
}
