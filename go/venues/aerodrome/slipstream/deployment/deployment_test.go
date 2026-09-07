package deployment

import "testing"

// TestBaseMainnetDeployments verifies base mainnet deployments in the migrated SDK.
//
// Version:
//   - 2026-09-07: Migrated to onchain.
func TestBaseMainnetDeployments(t *testing.T) {
	t.Parallel()
	deployments := ListByChainID(8453)
	if len(deployments) != 2 {
		t.Fatalf("ListByChainID() length = %d, want 2", len(deployments))
	}
	for _, deployment := range deployments {
		if deployment.ChainID != 8453 || deployment.Venue != VenueAerodrome || deployment.Factory == ([20]byte{}) || deployment.QuoterV2 == ([20]byte{}) {
			t.Fatalf("ListByChainID() deployment = %+v", deployment)
		}
		resolved, err := ByID(deployment.ID)
		if err != nil || resolved != deployment {
			t.Fatalf("ByID(%q) = (%+v, %v)", deployment.ID, resolved, err)
		}
	}
}

// TestLatestByChainID verifies latest by chain id in the migrated SDK.
//
// Version:
//   - 2026-09-07: Migrated to onchain.
func TestLatestByChainID(t *testing.T) {
	t.Parallel()
	deployment, err := LatestByChainID(8453)
	if err != nil {
		t.Fatalf("LatestByChainID() error = %v", err)
	}
	if deployment.ID != IDAerodromeBaseMainnetCurrent {
		t.Fatalf("LatestByChainID() ID = %q", deployment.ID)
	}
}

// TestDeploymentLookupRejectsUnknownValues verifies deployment lookup rejects unknown values in the migrated SDK.
//
// Version:
//   - 2026-09-07: Migrated to onchain.
func TestDeploymentLookupRejectsUnknownValues(t *testing.T) {
	t.Parallel()
	if _, err := ByID("unknown"); err == nil {
		t.Fatal("ByID() error = nil")
	}
	if _, err := LatestByChainID(1); err == nil {
		t.Fatal("LatestByChainID() error = nil")
	}
	if deployments := ListByChainID(1); len(deployments) != 0 {
		t.Fatalf("ListByChainID() = %+v", deployments)
	}
}
