package suievent

import "testing"

// TestJSONRejectsUnknownLayout verifies unsupported field types fail closed.
//
// Version:
//   - 2026-09-11: Added.
func TestJSONRejectsUnknownLayout(t *testing.T) {
	if _, err := JSON([]byte{1}, []Field{{Name: "value", Type: "vector"}}); err == nil {
		t.Fatal("accepted unsupported layout")
	}
}
