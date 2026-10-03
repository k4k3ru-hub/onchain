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
	expected := map[bool]string{false: "7e34697575614f2dafa5657418f35f3acd4bcfc633065b0220393c101d94754b", true: "de35e1793b54126473562cd3c5e4c183e084e06b8a69bdf0e5dad00d963fc9b1"}
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
