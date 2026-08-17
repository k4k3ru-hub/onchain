// hash_test.go
package evm

import (
	"encoding/hex"
	"errors"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/common"
)

func TestParseHash(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		value string
		want  common.Hash
	}{
		{
			name:  "zero hash",
			value: "0x" + strings.Repeat("0", 64),
			want:  common.Hash{},
		},
		{
			name:  "all ff hash",
			value: "0x" + strings.Repeat("ff", 32),
			want:  common.HexToHash("0x" + strings.Repeat("ff", 32)),
		},
		{
			name:  "lowercase hexadecimal",
			value: "0x0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
			want:  common.HexToHash("0x0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"),
		},
		{
			name:  "uppercase hexadecimal",
			value: "0xABCDEF0123456789ABCDEF0123456789ABCDEF0123456789ABCDEF0123456789",
			want:  common.HexToHash("0xabcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789"),
		},
		{
			name:  "surrounding whitespace",
			value: " \t0x0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef\n",
			want:  common.HexToHash("0x0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := ParseHash(tt.value)
			if err != nil {
				t.Fatalf("ParseHash() error = %v", err)
			}
			if got != tt.want {
				t.Errorf("ParseHash() = %s, want %s", got.Hex(), tt.want.Hex())
			}
			if got.Hex() != tt.want.Hex() {
				t.Errorf("ParseHash().Hex() = %q, want %q", got.Hex(), tt.want.Hex())
			}
		})
	}
}

func TestParseHashRejectsInvalidValues(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		value     string
		wantError string
	}{
		{name: "empty", value: "", wantError: "failed to parse evm hash: hash=empty"},
		{name: "whitespace only", value: " \t\n", wantError: "failed to parse evm hash: hash=empty"},
		{name: "missing prefix", value: strings.Repeat("0", 64), wantError: "failed to parse evm hash: hash=too_short actual_length=64 min_length=66"},
		{name: "uppercase prefix", value: "0X" + strings.Repeat("0", 64), wantError: "failed to parse evm hash: hash=invalid"},
		{name: "prefix only", value: "0x", wantError: "failed to parse evm hash: hash=too_short actual_length=2 min_length=66"},
		{name: "31 bytes", value: "0x" + strings.Repeat("00", 31), wantError: "failed to parse evm hash: hash=too_short actual_length=64 min_length=66"},
		{name: "33 bytes", value: "0x" + strings.Repeat("00", 33), wantError: "failed to parse evm hash: hash=too_long actual_length=68 max_length=66"},
		{name: "short hexadecimal", value: "0x" + strings.Repeat("a", 63), wantError: "failed to parse evm hash: hash=too_short actual_length=65 min_length=66"},
		{name: "long hexadecimal", value: "0x" + strings.Repeat("a", 65), wantError: "failed to parse evm hash: hash=too_long actual_length=67 max_length=66"},
		{name: "non hexadecimal", value: "0x" + strings.Repeat("0", 63) + "z", wantError: "failed to parse evm hash: hash=invalid: encoding/hex: invalid byte: U+007A 'z'"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := ParseHash(tt.value)
			if err == nil {
				t.Fatal("ParseHash() error = nil, want error")
			}
			if err.Error() != tt.wantError {
				t.Errorf("ParseHash() error = %q, want %q", err.Error(), tt.wantError)
			}
			if strings.Contains(err.Error(), strings.TrimSpace(tt.value)) && strings.TrimSpace(tt.value) != "" && len(strings.TrimSpace(tt.value)) > 2 {
				t.Errorf("ParseHash() error contains input hash: %q", err.Error())
			}
		})
	}
}

func TestParseHashWrapsDecodeError(t *testing.T) {
	t.Parallel()

	_, err := ParseHash("0x" + strings.Repeat("0", 63) + "z")
	if err == nil {
		t.Fatal("ParseHash() error = nil, want error")
	}

	var invalidByteError hex.InvalidByteError
	if !errors.As(err, &invalidByteError) {
		t.Fatalf("errors.As() = false, want wrapped hex.InvalidByteError: %v", err)
	}
}

func TestNormalizeHash(t *testing.T) {
	t.Parallel()

	value := "0xABCDEF0123456789ABCDEF0123456789ABCDEF0123456789ABCDEF0123456789"
	want := common.HexToHash(value).Hex()

	got, err := NormalizeHash(value)
	if err != nil {
		t.Fatalf("NormalizeHash() error = %v", err)
	}
	if got != want {
		t.Errorf("NormalizeHash() = %q, want %q", got, want)
	}

	parsed, err := ParseHash(got)
	if err != nil {
		t.Fatalf("ParseHash(NormalizeHash()) error = %v", err)
	}
	if parsed.Hex() != got {
		t.Errorf("ParseHash(NormalizeHash()).Hex() = %q, want %q", parsed.Hex(), got)
	}
}

func TestNormalizeHashWrapsParseError(t *testing.T) {
	t.Parallel()

	_, err := NormalizeHash("invalid")
	if err == nil {
		t.Fatal("NormalizeHash() error = nil, want error")
	}

	want := "failed to normalize evm hash: failed to parse evm hash: hash=too_short actual_length=7 min_length=66"
	if err.Error() != want {
		t.Errorf("NormalizeHash() error = %q, want %q", err.Error(), want)
	}
}
