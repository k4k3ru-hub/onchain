package solana

import (
	"crypto/ed25519"
	"crypto/rand"
	"fmt"

	"github.com/mr-tron/base58"
)

type KeyPair struct {
	privateKey ed25519.PrivateKey
}

// GenerateKeyPair generates an Ed25519 Solana key pair.
//
// Returns:
//   - Generated key pair.
//   - Generation error.
//
// Version:
//   - 2026-08-22: Added.
func GenerateKeyPair() (*KeyPair, error) {
	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("failed to generate solana key pair: %w", err)
	}
	return &KeyPair{privateKey: append(ed25519.PrivateKey(nil), privateKey...)}, nil
}

// Address returns the public address of the key pair.
func (k *KeyPair) Address() (Address, error) {
	if k == nil || len(k.privateKey) != ed25519.PrivateKeySize {
		return Address{}, fmt.Errorf("failed to get solana key pair address: private_key=invalid")
	}
	publicKey := k.privateKey.Public().(ed25519.PublicKey)
	var address Address
	copy(address[:], publicKey)
	return address, nil
}

// PrivateKeyBase58 returns the base58-encoded private key.
func (k *KeyPair) PrivateKeyBase58() (string, error) {
	if k == nil || len(k.privateKey) != ed25519.PrivateKeySize {
		return "", fmt.Errorf("failed to get solana private key: private_key=invalid")
	}
	return base58.Encode(k.privateKey), nil
}
