package sui

import (
	"bytes"
	"testing"

	"golang.org/x/crypto/blake2b"
)

// TestPersonalMessageIntentAndLength verifies the named protocol behavior.
//
// Version:
//   - 2026-10-14: Added.
func TestPersonalMessageIntentAndLength(t *testing.T) {
	for _, size := range []int{1, 127, 128, 16383, 16384} {
		message := bytes.Repeat([]byte{'a'}, size)
		var prefix []byte
		switch size {
		case 1:
			prefix = []byte{1}
		case 127:
			prefix = []byte{127}
		case 128:
			prefix = []byte{128, 1}
		case 16383:
			prefix = []byte{255, 127}
		case 16384:
			prefix = []byte{128, 128, 1}
		}
		expected := blake2b.Sum256(append(append([]byte{3, 0, 0}, prefix...), message...))
		actual, err := PersonalMessageDigest(message)
		if err != nil || actual != expected {
			t.Fatalf("size=%d digest mismatch: %v", size, err)
		}
	}
	key, err := GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	account, err := key.Address()
	if err != nil {
		t.Fatal(err)
	}
	message := []byte("Bluefin test login")
	signature, err := key.SignPersonalMessage(message)
	if err != nil {
		t.Fatal(err)
	}
	if err := VerifyPersonalMessageSignature(message, signature, account); err != nil {
		t.Fatal(err)
	}
	if err := VerifyPersonalMessageSignature([]byte("changed"), signature, account); err == nil {
		t.Fatal("changed message accepted")
	}
	other := account
	other[0] ^= 1
	if err := VerifyPersonalMessageSignature(message, signature, other); err == nil {
		t.Fatal("other signer accepted")
	}
	if _, err := PersonalMessageDigest(nil); err == nil {
		t.Fatal("empty message accepted")
	}
	if _, err := PersonalMessageDigest(make([]byte, maximumPersonalMessageLength+1)); err == nil {
		t.Fatal("oversized message accepted")
	}
	if err := VerifyPersonalMessageSignature(message, []byte{0}, account); err == nil {
		t.Fatal("short signature accepted")
	}
}
