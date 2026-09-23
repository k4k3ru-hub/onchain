package tax

import (
	"encoding/json"
	"os"
	"testing"
)

// TestOniAgentRemainsControlsOnly prevents an implicit expansion of zero-tax claims.
//
// Version:
//   - 2026-09-23: Added.
func TestOniAgentRemainsControlsOnly(t *testing.T) {
	raw, err := os.ReadFile("../analysis/testdata/oni-agent.json")
	if err != nil {
		t.Fatal(err)
	}
	f := &fakeDependencies{}
	if err := json.Unmarshal(raw, &f.fixture); err != nil {
		t.Fatal(err)
	}
	r, err := analyzeFixture(t, f)
	if err != nil {
		t.Fatal(err)
	}
	if r.Model != "" || r.CodeSHA256 == "" || r.Observation != nil || r.Reason != "unsupported_model" || f.sourceCalls != 0 || len(f.compiled) != 0 {
		t.Fatal("Controls runtime implicitly expanded tax claims", r)
	}
}
