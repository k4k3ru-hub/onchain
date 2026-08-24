package solana

import (
	"strings"
	"testing"

	"github.com/mr-tron/base58"
)

func TestParseSignature(t *testing.T) {
	bytes := make([]byte, signatureByteLength)
	for i := range bytes {
		bytes[i] = byte(i + 1)
	}
	wantText := base58.Encode(bytes)

	signature, err := ParseSignature("  " + wantText + "  ")
	if err != nil {
		t.Fatalf("ParseSignature() error = %v", err)
	}
	if got := signature.String(); got != wantText {
		t.Errorf("Signature.String() = %q, want %q", got, wantText)
	}
	if signature.IsZero() {
		t.Error("Signature.IsZero() = true, want false")
	}

	gotBytes := signature.Bytes()
	gotBytes[0] = 0
	if signature.Bytes()[0] != bytes[0] {
		t.Error("Signature.Bytes() returned mutable signature storage")
	}
}

func TestNormalizeSignature(t *testing.T) {
	value := base58.Encode(make([]byte, signatureByteLength))
	got, err := NormalizeSignature("\t" + value + "\n")
	if err != nil {
		t.Fatalf("NormalizeSignature() error = %v", err)
	}
	if got != value {
		t.Errorf("NormalizeSignature() = %q, want %q", got, value)
	}
}

func TestParseSignatureRejectsInvalidInput(t *testing.T) {
	tests := []struct {
		name  string
		value string
	}{
		{name: "empty"},
		{name: "invalid base58", value: "0OIl"},
		{name: "too short", value: base58.Encode(make([]byte, signatureByteLength-1))},
		{name: "too long decoded", value: base58.Encode(make([]byte, signatureByteLength+1))},
		{name: "too long input", value: strings.Repeat("1", signatureMaxInputLength+1)},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := ParseSignature(test.value); err == nil {
				t.Fatal("ParseSignature() error = nil, want error")
			}
		})
	}
}
