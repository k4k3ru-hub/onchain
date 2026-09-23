package analysis

import (
	"encoding/json"
	"os"
	"testing"
)

type fixture struct {
	Bundle SourceBundle    `json:"bundle"`
	Code   string          `json:"code"`
	Output json.RawMessage `json:"output"`
}

func loadFixture(t *testing.T, name string) fixture {
	t.Helper()
	raw, err := os.ReadFile("../tax/testdata/" + name + ".json")
	if err != nil {
		t.Fatal(err)
	}
	var f fixture
	if err := json.Unmarshal(raw, &f); err != nil {
		t.Fatal(err)
	}
	return f
}
