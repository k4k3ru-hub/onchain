package sui

import (
	"fmt"
	"github.com/k4k3ru-hub/onchain/go/sui/internal/rpcv2"
)

type SimulationObjectReference struct {
	Address Address
	Version uint64
	Digest  ObjectDigest
}

func simulationInputObjects(e *rpcv2.TransactionEffects) ([]SimulationObjectReference, error) {
	var out []SimulationObjectReference
	add := func(id string, version uint64, digest string) error {
		a, err := ParseAddress(id)
		if err != nil {
			return fmt.Errorf("failed to parse simulation object: %w", err)
		}
		d, err := ParseObjectDigest(digest)
		if err != nil {
			return fmt.Errorf("failed to parse simulation object: %w", err)
		}
		if version == 0 {
			return fmt.Errorf("failed to parse simulation object: version=empty")
		}
		out = append(out, SimulationObjectReference{Address: a, Version: version, Digest: d})
		return nil
	}
	for _, o := range e.GetUnchangedConsensusObjects() {
		if o.GetKind() == rpcv2.UnchangedConsensusObject_READ_ONLY_ROOT {
			if err := add(o.GetObjectId(), o.GetVersion(), o.GetDigest()); err != nil {
				return nil, err
			}
		}
	}
	for _, o := range e.GetChangedObjects() {
		if o.InputVersion != nil && o.InputDigest != nil {
			if err := add(o.GetObjectId(), o.GetInputVersion(), o.GetInputDigest()); err != nil {
				return nil, err
			}
		}
	}
	return out, nil
}
