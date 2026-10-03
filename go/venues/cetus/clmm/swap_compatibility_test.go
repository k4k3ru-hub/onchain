package clmm

import (
	"crypto/sha256"
	"fmt"
	"testing"
)

// TestFundedSwapExactInputBytes detects changes to the transaction the Agent signs.
//
// Version:
//   - 2026-10-02: Added.
func TestFundedSwapExactInputBytes(t *testing.T) {
	expected := map[bool]string{false: "f18aaf24df2aed08a5ac06446b4cfaec6cd31312ed9d7213e00e504068632ade", true: "ed631ca23a4ccf483ebb74ac386d137ecf4881cc2d666b78e1ecb6f6254dd819"}
	for _, native := range []bool{false, true} {
		tx, err := BuildSwapTransaction(testDeployment(), fundedSwap(t, native))
		if err != nil {
			t.Fatal(err)
		}
		encoded, err := tx.MarshalBCS()
		if err != nil {
			t.Fatal(err)
		}
		if got := fmt.Sprintf("%x", sha256.Sum256(encoded)); got != expected[native] {
			t.Fatalf("exact-input transaction changed: native=%t got=%s want=%s", native, got, expected[native])
		}
	}
}
