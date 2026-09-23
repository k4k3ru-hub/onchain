package analysis

import (
	"encoding/hex"
	"encoding/json"
	"strings"
	"testing"
)

// TestPermitImmutablesCannotMaskExecutableChanges verifies the named analysis or transport invariant.
//
// Version:
//   - 2026-09-22: Added.
func TestPermitImmutablesCannotMaskExecutableChanges(t *testing.T) {
	f := loadFixture(t, "permit")
	artifact, sources, err := parseOutput(f.Output, f.Bundle)
	if err != nil {
		t.Fatal(err)
	}
	_, allowed, ok := recognizeModel(f.Bundle, sources)
	if !ok {
		t.Fatal("permit fixture model missing")
	}
	code, err := hex.DecodeString(strings.TrimPrefix(f.Code, "0x"))
	if err != nil {
		t.Fatal(err)
	}
	if !matchRuntime(artifact, code, allowed) {
		t.Fatal("original permit runtime not matched")
	}
	code[0] ^= 1
	if matchRuntime(artifact, code, allowed) {
		t.Fatal("executable change outside immutable ranges accepted")
	}
}

// TestRepeatedImmutablesMustAgree verifies the named analysis or transport invariant.
//
// Version:
//   - 2026-09-22: Added.
func TestRepeatedImmutablesMustAgree(t *testing.T) {
	var artifact runtimeArtifact
	raw := `{"object":"` + strings.Repeat("00", 64) + `","immutableReferences":{"1":[{"start":0,"length":32},{"start":32,"length":32}]}}`
	if err := json.Unmarshal([]byte(raw), &artifact); err != nil {
		t.Fatal(err)
	}
	actual := make([]byte, 64)
	actual[0], actual[32] = 1, 1
	if !matchRuntime(artifact, actual, map[string]bool{"1": true}) {
		t.Fatal("consistent values rejected")
	}
	actual[32] = 2
	if matchRuntime(artifact, actual, map[string]bool{"1": true}) {
		t.Fatal("inconsistent values accepted")
	}
}

// TestMetadataRelaxationRejectsCodeInspection verifies the named analysis or transport invariant.
//
// Version:
//   - 2026-09-22: Added.
func TestMetadataRelaxationRejectsCodeInspection(t *testing.T) {
	// A complete CBOR map with only solc. Different compiler metadata may be
	// tolerated for a reviewed body, but never for a body inspecting its code.
	metadata := []byte{0xa1, 0x64, 's', 'o', 'l', 'c', 0x43, 0, 8, 37, 0, 10}
	for _, opcode := range []byte{0x38, 0x39, 0x3b, 0x3c, 0x3f} {
		code := append([]byte{opcode, 0x00, 0xfe}, metadata...)
		actual := append([]byte(nil), code...)
		actual[len(actual)-3] = 34
		if matchRuntime(runtimeArtifact{Object: hex.EncodeToString(code)}, actual, nil) {
			t.Fatalf("code inspection accepted: opcode=%x", opcode)
		}
	}
}

// FuzzRuntimeComparison verifies the named analysis or transport invariant.
//
// Version:
//   - 2026-09-22: Added.
func FuzzRuntimeComparison(f *testing.F) {
	f.Add([]byte{0x60, 0x00, 0x00}, []byte{0x60, 0x00, 0x00})
	f.Add([]byte{0x60, 0x00, 0xff, 0xff}, []byte{0})
	f.Fuzz(func(t *testing.T, compiled, actual []byte) {
		if len(compiled) > 128*1024 || len(actual) > 128*1024 {
			t.Skip()
		}
		matchRuntime(runtimeArtifact{Object: hex.EncodeToString(compiled)}, actual, nil)
	})
}
