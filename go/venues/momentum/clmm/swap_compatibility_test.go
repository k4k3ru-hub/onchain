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
	expected := map[bool]string{false: "aed06e7b274911d3a6085765e8328242640cc2874788cdb5c00a26d55cec9b7f", true: "7a0b51bf048d9455b950168dcd28889420989803acb49c4bbf33317c5d034144"}
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
