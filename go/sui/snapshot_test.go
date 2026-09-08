package sui

import (
	"strings"
	"testing"
)

// TestSnapshotQueries verifies checkpoint bounds and rejection of missing dynamic values.
//
// Version:
//   - 2026-09-08: Added.
func TestSnapshotQueries(t *testing.T) {
	a, err := ParseAddress("0x6")
	if err != nil {
		t.Fatal(err)
	}
	f := &fakeCaller{response: map[string]any{"object": nil}}
	c := composeRPCClient(RPCConfig{URL: "https://example.com"}, f)
	if _, err := c.ObjectAtCheckpoint(nil, a, 123); err == nil {
		t.Fatal("accepted missing object")
	}
	if !strings.Contains(f.queryValue, "atCheckpoint: 123") {
		t.Fatal(f.queryValue)
	}
	f.response = map[string]any{"address": map[string]any{"dynamicFields": map[string]any{"nodes": []any{map[string]any{"value": map[string]any{"json": map[string]any{"value": "1"}}}}, "pageInfo": map[string]any{"hasNextPage": false}}}}
	p, err := c.DynamicValuesAtCheckpoint(nil, a, 123, 100, "")
	if err != nil || len(p.Values) != 1 {
		t.Fatalf("%+v %v", p, err)
	}
	if !strings.Contains(f.queryValue, "atCheckpoint: 123") {
		t.Fatal(f.queryValue)
	}
	f.response = map[string]any{"address": map[string]any{"dynamicFields": map[string]any{"nodes": []any{map[string]any{"value": nil}}, "pageInfo": map[string]any{"hasNextPage": false}}}}
	if _, err := c.DynamicValuesAtCheckpoint(nil, a, 123, 100, ""); err == nil {
		t.Fatal("accepted missing value")
	}
}

// TestKeyedSnapshotQueries checks key serialization, pinned scope and missing-entry preservation.
//
// Version:
//   - 2026-09-08: Added.
func TestKeyedSnapshotQueries(t *testing.T) {
	a, err := ParseAddress("0x6")
	if err != nil {
		t.Fatal(err)
	}
	f := &fakeCaller{response: map[string]any{"address": map[string]any{"multiGetDynamicFields": []any{nil, map[string]any{"value": map[string]any{"json": map[string]any{"score": "2"}}}}}}}
	c := composeRPCClient(RPCConfig{URL: "https://example.com"}, f)
	out, err := c.DynamicUint64ValuesAtCheckpoint(nil, a, 123, []uint64{1, 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 2 || out[0] != nil || len(out[1]) == 0 || !strings.Contains(f.queryValue, "AQAAAAAAAAA=") || !strings.Contains(f.queryValue, "atCheckpoint: 123") {
		t.Fatal("invalid keyed read")
	}
}
