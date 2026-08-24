package sui

import (
	"encoding/base64"
	"testing"
)

func TestGenerateKeyPair(t *testing.T) {
	keyPair, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair() returned an unexpected error: %v", err)
	}
	address, err := keyPair.Address()
	if err != nil {
		t.Fatalf("KeyPair.Address() returned an unexpected error: %v", err)
	}
	if address.IsZero() {
		t.Fatal("KeyPair.Address() returned the zero address")
	}
	privateKey, err := keyPair.PrivateKeyBase64()
	if err != nil {
		t.Fatalf("KeyPair.PrivateKeyBase64() returned an unexpected error: %v", err)
	}
	decoded, err := base64.StdEncoding.DecodeString(privateKey)
	if err != nil {
		t.Fatalf("failed to decode private key: %v", err)
	}
	if len(decoded) != 33 || decoded[0] != ed25519SchemeFlag {
		t.Fatalf("decoded private key length/flag = %d/%d, want 33/0", len(decoded), decoded[0])
	}
}
