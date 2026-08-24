package solana

import (
	"context"
	"errors"
	"testing"
)

func TestRPCClientSlot(t *testing.T) {
	dependency := &fakeRPCDependency{slot: 12345}
	client := composeRPCClient(RPCConfig{Commitment: CommitmentFinalized}, dependency)

	slot, err := client.Slot(nil)
	if err != nil {
		t.Fatalf("Slot() error = %v", err)
	}
	if slot != 12345 {
		t.Errorf("Slot() = %d, want 12345", slot)
	}
	if dependency.slotContext == nil {
		t.Error("Slot() passed nil context")
	}
	if dependency.commitment != CommitmentFinalized {
		t.Errorf("Slot() commitment = %q, want %q", dependency.commitment, CommitmentFinalized)
	}
	if slot.Uint64() != 12345 || slot.String() != "12345" {
		t.Errorf("Slot helpers returned unexpected values for %d", slot)
	}
}

func TestRPCClientSlotWrapsError(t *testing.T) {
	slotErr := errors.New("rpc unavailable")
	client := &RPCClient{slotProvider: &fakeRPCDependency{slotErr: slotErr}}

	_, err := client.Slot(context.Background())
	if !errors.Is(err, slotErr) {
		t.Fatalf("Slot() error = %v, want wrapped RPC error", err)
	}
}

func TestRPCClientSlotRejectsInvalidState(t *testing.T) {
	for _, client := range []*RPCClient{nil, {}, {slotProvider: &fakeRPCDependency{}}} {
		if _, err := client.Slot(context.Background()); err == nil {
			t.Fatal("Slot() error = nil, want error")
		}
	}
}
