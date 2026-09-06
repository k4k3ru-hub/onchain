package safety

import (
	"fmt"
	"net/url"
	"reflect"
	"strings"
)

// IsNil reports nil interfaces and typed nil dependencies.
//
// Version:
//   - 2026-09-06: Added.
func IsNil(value any) bool {
	if value == nil {
		return true
	}
	v := reflect.ValueOf(value)
	switch v.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return v.IsNil()
	}
	return false
}

// Endpoint validates a credential-free endpoint without exposing its contents.
//
// Version:
//   - 2026-09-06: Added.
func Endpoint(raw string, websocket bool) (*url.URL, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("failed to validate endpoint: url=invalid")
	}
	secure, plain := "https", "http"
	if websocket {
		secure, plain = "wss", "ws"
	}
	if u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || u.Opaque != "" || (u.Scheme != secure && u.Scheme != plain) {
		return nil, fmt.Errorf("failed to validate endpoint: url=invalid")
	}
	u.Path = strings.TrimRight(u.Path, "/")
	return u, nil
}

type redactedError struct{ cause error }

// Error returns a safe error description while Unwrap preserves inspection.
//
// Version:
//   - 2026-09-06: Added.
func (e redactedError) Error() string { return "failed to perform transport operation" }

// Unwrap returns the original error for errors.Is and errors.As.
//
// Version:
//   - 2026-09-06: Added.
func (e redactedError) Unwrap() error { return e.cause }

// Redact preserves an underlying error without printing URLs or response payloads.
//
// Version:
//   - 2026-09-06: Added.
func Redact(err error) error { return redactedError{cause: err} }
