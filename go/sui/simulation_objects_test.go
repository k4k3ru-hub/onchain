package sui

import (
	"github.com/k4k3ru-hub/onchain/go/sui/internal/rpcv2"
	"testing"
)

// TestSimulationObjectReferences verifies exact input versions are preserved independently of checkpoint height.
//
// Version:
//   - 2026-09-08: Added.
func TestSimulationObjectReferences(t *testing.T) {
	a, err := ParseAddress("0x9")
	if err != nil {
		t.Fatal(err)
	}
	id := a.String()
	version := uint64(123)
	digest := ObjectDigest{1}.String()
	kind := rpcv2.UnchangedConsensusObject_READ_ONLY_ROOT
	r, err := simulationInputObjects(&rpcv2.TransactionEffects{UnchangedConsensusObjects: []*rpcv2.UnchangedConsensusObject{{ObjectId: &id, Version: &version, Digest: &digest, Kind: &kind}}})
	if err != nil {
		t.Fatal(err)
	}
	if len(r) != 1 || r[0].Address != a || r[0].Version != 123 || r[0].Digest.String() != digest {
		t.Fatal(r)
	}
	bad := "invalid"
	if _, err = simulationInputObjects(&rpcv2.TransactionEffects{UnchangedConsensusObjects: []*rpcv2.UnchangedConsensusObject{{ObjectId: &id, Version: &version, Digest: &bad, Kind: &kind}}}); err == nil {
		t.Fatal("accepted malformed provenance")
	}
}
