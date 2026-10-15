package sui

import (
	"bytes"
	"crypto/ed25519"
	"fmt"

	"golang.org/x/crypto/blake2b"
)

const maximumPersonalMessageLength = 1 << 20

// PersonalMessageDigest hashes the Sui PersonalMessage intent and BCS byte vector.
//
// Version:
//   - 2026-10-14: Added.
func PersonalMessageDigest(message []byte) ([32]byte, error) {
	if len(message) == 0 {
		return [32]byte{}, fmt.Errorf("failed to hash sui personal message: message=empty")
	}
	if len(message) > maximumPersonalMessageLength {
		return [32]byte{}, fmt.Errorf("failed to hash sui personal message: message=too_long max_length=%d", maximumPersonalMessageLength)
	}
	// IntentScope::PersonalMessage, IntentVersion::V0, AppId::Sui.
	material := make([]byte, 3, len(message)+8)
	material[0] = 3
	for n := uint32(len(message)); ; n >>= 7 {
		b := byte(n & 0x7f)
		if n >= 0x80 {
			b |= 0x80
		}
		material = append(material, b)
		if n < 0x80 {
			break
		}
	}
	material = append(material, message...)
	return blake2b.Sum256(material), nil
}

// SignPersonalMessage signs a message with the Sui PersonalMessage intent.
// The result is the Ed25519 scheme flag, signature and public key, without Base64 encoding.
//
// Version:
//   - 2026-10-14: Added.
func (k *KeyPair) SignPersonalMessage(message []byte) ([]byte, error) {
	if _, err := k.Address(); err != nil {
		return nil, fmt.Errorf("failed to sign sui personal message: %w", err)
	}
	digest, err := PersonalMessageDigest(message)
	if err != nil {
		return nil, fmt.Errorf("failed to sign sui personal message: %w", err)
	}
	result := []byte{ed25519SchemeFlag}
	result = append(result, ed25519.Sign(k.privateKey, digest[:])...)
	result = append(result, k.privateKey[ed25519.SeedSize:]...)
	return result, nil
}

// VerifyPersonalMessageSignature verifies an Ed25519 PersonalMessage for the expected Sui signer.
//
// Version:
//   - 2026-10-14: Added.
func VerifyPersonalMessageSignature(message, signature []byte, expectedSigner Address) error {
	if expectedSigner.IsZero() {
		return fmt.Errorf("failed to verify sui personal message: signer=empty")
	}
	if len(signature) != 1+ed25519.SignatureSize+ed25519.PublicKeySize || signature[0] != ed25519SchemeFlag {
		return fmt.Errorf("failed to verify sui personal message: signature=invalid")
	}
	publicKey := ed25519.PublicKey(signature[1+ed25519.SignatureSize:])
	address := blake2b.Sum256(append([]byte{ed25519SchemeFlag}, publicKey...))
	if !bytes.Equal(address[:], expectedSigner[:]) {
		return fmt.Errorf("failed to verify sui personal message: signer=mismatch")
	}
	digest, err := PersonalMessageDigest(message)
	if err != nil {
		return fmt.Errorf("failed to verify sui personal message: %w", err)
	}
	if !ed25519.Verify(publicKey, digest[:], signature[1:1+ed25519.SignatureSize]) {
		return fmt.Errorf("failed to verify sui personal message: signature=invalid")
	}
	return nil
}
