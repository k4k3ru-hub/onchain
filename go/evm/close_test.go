package evm

import "testing"

type fakeClientCloser struct {
	closeCalls int
}

func (f *fakeClientCloser) Close() {
	f.closeCalls++
}

func TestHTTPClientClose(t *testing.T) {
	t.Parallel()

	closer := &fakeClientCloser{}
	client := &HTTPClient{clientCloser: closer}

	client.Close()
	if closer.closeCalls != 1 {
		t.Errorf("Close() calls = %d, want 1", closer.closeCalls)
	}

	client.Close()
	if closer.closeCalls != 1 {
		t.Errorf("Close() calls after repeated close = %d, want 1", closer.closeCalls)
	}
}

func TestHTTPClientCloseHandlesNilState(t *testing.T) {
	t.Parallel()

	var nilClient *HTTPClient
	nilClient.Close()

	client := &HTTPClient{}
	client.Close()
}

func TestWSClientClose(t *testing.T) {
	t.Parallel()

	closer := &fakeClientCloser{}
	client := &WSClient{clientCloser: closer}

	client.Close()
	client.Close()
	if closer.closeCalls != 1 {
		t.Errorf("Close() calls = %d, want 1", closer.closeCalls)
	}
}

func TestWSClientCloseHandlesNilState(t *testing.T) {
	t.Parallel()

	var nilClient *WSClient
	nilClient.Close()

	client := &WSClient{}
	client.Close()
}
