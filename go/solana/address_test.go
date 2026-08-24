package solana

import (
	"strings"
	"testing"

	"github.com/mr-tron/base58"
)

func TestParseAddress(t *testing.T) {
	bytes := make([]byte, addressByteLength)
	for i := range bytes {
		bytes[i] = byte(i + 1)
	}
	wantText := base58.Encode(bytes)

	address, err := ParseAddress("  " + wantText + "  ")
	if err != nil {
		t.Fatalf("ParseAddress() error = %v", err)
	}
	if got := address.String(); got != wantText {
		t.Errorf("Address.String() = %q, want %q", got, wantText)
	}
	if address.IsZero() {
		t.Error("Address.IsZero() = true, want false")
	}

	gotBytes := address.Bytes()
	gotBytes[0] = 0
	if address.Bytes()[0] != bytes[0] {
		t.Error("Address.Bytes() returned mutable address storage")
	}
}

func TestParseAddressAcceptsSystemProgram(t *testing.T) {
	address, err := ParseAddress("11111111111111111111111111111111")
	if err != nil {
		t.Fatalf("ParseAddress() error = %v", err)
	}
	if !address.IsZero() {
		t.Error("System Program address IsZero() = false, want true")
	}
}

func TestNormalizeAddress(t *testing.T) {
	value := base58.Encode(make([]byte, addressByteLength))
	got, err := NormalizeAddress("\t" + value + "\n")
	if err != nil {
		t.Fatalf("NormalizeAddress() error = %v", err)
	}
	if got != value {
		t.Errorf("NormalizeAddress() = %q, want %q", got, value)
	}
}

func TestParseAddressRejectsInvalidInput(t *testing.T) {
	tests := []struct {
		name  string
		value string
	}{
		{name: "empty"},
		{name: "invalid base58", value: "0OIl"},
		{name: "too short", value: base58.Encode(make([]byte, addressByteLength-1))},
		{name: "too long decoded", value: base58.Encode(make([]byte, addressByteLength+1))},
		{name: "too long input", value: strings.Repeat("1", addressMaxInputLength+1)},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := ParseAddress(test.value); err == nil {
				t.Fatal("ParseAddress() error = nil, want error")
			}
		})
	}
}
