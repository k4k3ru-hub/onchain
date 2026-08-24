package sui

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"fmt"

	"golang.org/x/crypto/blake2b"
)

const ed25519SchemeFlag byte = 0x00

type KeyPair struct {
	privateKey ed25519.PrivateKey
}

// GenerateKeyPair generates an Ed25519 Sui key pair.
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
		return nil, fmt.Errorf("failed to generate sui key pair: %w", err)
	}
	return &KeyPair{privateKey: append(ed25519.PrivateKey(nil), privateKey...)}, nil
}

// Address returns the Sui address derived from the Ed25519 public key.
func (k *KeyPair) Address() (Address, error) {
	if k == nil || len(k.privateKey) != ed25519.PrivateKeySize {
		return Address{}, fmt.Errorf("failed to get sui key pair address: private_key=invalid")
	}
	publicKey := k.privateKey.Public().(ed25519.PublicKey)
	material := make([]byte, 1+len(publicKey))
	material[0] = ed25519SchemeFlag
	copy(material[1:], publicKey)
	digest := blake2b.Sum256(material)
	var address Address
	copy(address[:], digest[:])
	return address, nil
}

// PrivateKeyBase64 returns the Sui keystore-compatible base64 key bytes.
//
// The encoded bytes contain the Ed25519 scheme flag followed by the 32-byte
// private-key seed.
func (k *KeyPair) PrivateKeyBase64() (string, error) {
	if k == nil || len(k.privateKey) != ed25519.PrivateKeySize {
		return "", fmt.Errorf("failed to get sui private key: private_key=invalid")
	}
	encoded := make([]byte, 1+ed25519.SeedSize)
	encoded[0] = ed25519SchemeFlag
	copy(encoded[1:], k.privateKey.Seed())
	return base64.StdEncoding.EncodeToString(encoded), nil
}
