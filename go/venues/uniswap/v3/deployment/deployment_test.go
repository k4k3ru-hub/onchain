package deployment

import "testing"

func TestByChainID(t *testing.T) {
	t.Parallel()
	for _, chainID := range []uint64{1, 56, 8453, 84532, 4663} {
		deployment, err := ByChainID(chainID)
		if err != nil {
			t.Fatalf("ByChainID(%d) error = %v", chainID, err)
		}
		if deployment.ChainID != chainID || deployment.Factory == ([20]byte{}) || deployment.QuoterV2 == ([20]byte{}) || deployment.InitCodeHash == ([32]byte{}) {
			t.Fatalf("ByChainID(%d) = %+v", chainID, deployment)
		}
		if (chainID == 8453 || chainID == 84532) && deployment.SwapRouter02 == ([20]byte{}) {
			t.Fatalf("ByChainID(%d).SwapRouter02 is empty", chainID)
		}
	}
}

func TestByChainIDRejectsUnsupportedChain(t *testing.T) {
	t.Parallel()
	if _, err := ByChainID(10); err == nil {
		t.Fatal("ByChainID() error = nil")
	}
}
