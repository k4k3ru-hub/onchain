package sui

import (
	"bytes"
	"crypto/ed25519"
	"encoding/base64"
	"fmt"

	"golang.org/x/crypto/blake2b"
)

// ParseKeyPairBase64 imports an Ed25519 key encoded by PrivateKeyBase64.
// It accepts only the 33-byte Sui keystore format, not arbitrary private-key data.
//
// Version:
//   - 2026-09-24: Added.
func ParseKeyPairBase64(value string) (*KeyPair, error) {
	if len(value) != base64.StdEncoding.EncodedLen(1+ed25519.SeedSize) {
		return nil, fmt.Errorf("failed to parse sui key pair: private_key=invalid")
	}
	raw, err := base64.StdEncoding.Strict().DecodeString(value)
	if err != nil || len(raw) != 1+ed25519.SeedSize || raw[0] != ed25519SchemeFlag {
		return nil, fmt.Errorf("failed to parse sui key pair: private_key=invalid")
	}
	return &KeyPair{privateKey: ed25519.NewKeyFromSeed(raw[1:])}, nil
}

// SignTransaction signs supported canonical TransactionData and returns the Sui
// flag || signature || public-key bytes. It never sends a transaction.
// The key must identify the sender or gas owner; callers must enforce their policy.
//
// Version:
//   - 2026-09-24: Added.
func (k *KeyPair) SignTransaction(data []byte) ([]byte, error) {
	address, err := k.Address()
	if err != nil {
		return nil, fmt.Errorf("failed to sign sui transaction: %w", err)
	}
	t, err := ParseTransactionData(data)
	if err != nil {
		return nil, fmt.Errorf("failed to sign sui transaction: %w", err)
	}
	if address != t.Sender && address != t.GasData.Owner {
		return nil, fmt.Errorf("failed to sign sui transaction: signer=mismatch")
	}
	digest := transactionIntentDigest(data)
	result := make([]byte, 1, 1+ed25519.SignatureSize+ed25519.PublicKeySize)
	result[0] = ed25519SchemeFlag
	result = append(result, ed25519.Sign(k.privateKey, digest[:])...)
	result = append(result, k.privateKey[ed25519.SeedSize:]...)
	return result, nil
}

// VerifyTransactionSignature verifies one Ed25519 transaction signature for an
// expected sender or gas owner. It does not assert that all required signatures
// for a sponsored transaction have been supplied.
//
// Version:
//   - 2026-09-24: Added.
func VerifyTransactionSignature(data, signature []byte, expectedSigner Address) error {
	t, err := ParseTransactionData(data)
	if err != nil {
		return fmt.Errorf("failed to verify sui transaction signature: %w", err)
	}
	if expectedSigner.IsZero() || (expectedSigner != t.Sender && expectedSigner != t.GasData.Owner) {
		return fmt.Errorf("failed to verify sui transaction signature: signer=mismatch")
	}
	if len(signature) != 1+ed25519.SignatureSize+ed25519.PublicKeySize || signature[0] != ed25519SchemeFlag {
		return fmt.Errorf("failed to verify sui transaction signature: signature=invalid")
	}
	publicKey := ed25519.PublicKey(signature[1+ed25519.SignatureSize:])
	address := blake2b.Sum256(append([]byte{ed25519SchemeFlag}, publicKey...))
	if !bytes.Equal(address[:], expectedSigner[:]) {
		return fmt.Errorf("failed to verify sui transaction signature: public_key=mismatch")
	}
	digest := transactionIntentDigest(data)
	if !ed25519.Verify(publicKey, digest[:], signature[1:1+ed25519.SignatureSize]) {
		return fmt.Errorf("failed to verify sui transaction signature: signature=invalid")
	}
	return nil
}
