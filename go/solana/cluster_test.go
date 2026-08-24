package solana

import "testing"

func TestResolveCluster(t *testing.T) {
	for want, value := range clusterGenesisHashes {
		hash, err := ParseHash(value)
		if err != nil {
			t.Fatalf("ParseHash(%q) returned an unexpected error: %v", value, err)
		}
		got, err := ResolveCluster(hash)
		if err != nil {
			t.Fatalf("ResolveCluster(%q) returned an unexpected error: %v", value, err)
		}
		if got != want {
			t.Fatalf("ResolveCluster(%q) = %q, want %q", value, got, want)
		}
	}
}

func TestGenerateKeyPair(t *testing.T) {
	keyPair, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair() returned an unexpected error: %v", err)
	}
	address, err := keyPair.Address()
	if err != nil || address.IsZero() {
		t.Fatalf("KeyPair.Address() = %q error=%v", address, err)
	}
	privateKey, err := keyPair.PrivateKeyBase58()
	if err != nil || privateKey == "" {
		t.Fatalf("KeyPair.PrivateKeyBase58() empty or error=%v", err)
	}
}
