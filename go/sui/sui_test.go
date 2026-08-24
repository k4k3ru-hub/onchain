package sui

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/mr-tron/base58"
)

type fakeCaller struct {
	queryValue string
	response   any
}

func (f *fakeCaller) query(_ context.Context, query string, result any) error {
	f.queryValue = query
	encoded, err := json.Marshal(f.response)
	if err != nil {
		return err
	}
	return json.Unmarshal(encoded, result)
}

func TestParseAddress(t *testing.T) {
	address, err := ParseAddress("0x2")
	if err != nil {
		t.Fatalf("ParseAddress() returned an unexpected error: %v", err)
	}
	if !strings.HasSuffix(address.String(), "02") || len(address.String()) != 66 {
		t.Fatalf("ParseAddress() = %q, want canonical 32-byte address", address.String())
	}
}

func TestParseTransactionDigest(t *testing.T) {
	value := base58.Encode(make([]byte, digestByteLength))
	digest, err := ParseTransactionDigest(value)
	if err != nil {
		t.Fatalf("ParseTransactionDigest() returned an unexpected error: %v", err)
	}
	if digest.String() != value {
		t.Fatalf("ParseTransactionDigest() = %q, want %q", digest.String(), value)
	}
}

func TestLatestCheckpointSequenceNumber(t *testing.T) {
	caller := &fakeCaller{response: map[string]any{"checkpoint": map[string]any{"sequenceNumber": 123456789}}}
	client := composeRPCClient(RPCConfig{URL: "https://example.com/graphql"}, caller)
	number, err := client.LatestCheckpointSequenceNumber(nil)
	if err != nil {
		t.Fatalf("LatestCheckpointSequenceNumber() returned an unexpected error: %v", err)
	}
	if number != 123456789 || !strings.Contains(caller.queryValue, "checkpoint") {
		t.Fatalf("LatestCheckpointSequenceNumber() = %d query=%q", number, caller.queryValue)
	}
}

func TestRPCConfigValidate(t *testing.T) {
	if err := (RPCConfig{URL: "https://example.com/graphql"}).Validate(); err != nil {
		t.Fatalf("RPCConfig.Validate() returned an unexpected error: %v", err)
	}
	if err := (RPCConfig{}).Validate(); err == nil {
		t.Fatal("RPCConfig.Validate() error = nil, want error")
	}
}

func TestTransactionStatus(t *testing.T) {
	var digest TransactionDigest
	digest[0] = 1
	caller := &fakeCaller{response: map[string]any{
		"transaction": map[string]any{
			"digest": digest.String(),
			"effects": map[string]any{
				"status":     "SUCCESS",
				"checkpoint": map[string]any{"sequenceNumber": 99},
			},
		},
	}}
	client := composeRPCClient(RPCConfig{URL: "https://example.com/graphql"}, caller)
	status, err := client.TransactionStatus(nil, digest)
	if err != nil {
		t.Fatalf("TransactionStatus() returned an unexpected error: %v", err)
	}
	if !status.IsSuccessful() || !status.IsFinalized() || *status.Checkpoint != 99 {
		t.Fatalf("TransactionStatus() = %+v, want successful finalized status", status)
	}
}
