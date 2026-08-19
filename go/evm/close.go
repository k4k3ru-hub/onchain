package evm

type clientCloser interface {
	Close()
}

// Close closes the underlying EVM HTTP RPC connection.
//
// Version:
//   - 2026-08-20: Added.
func (c *HTTPClient) Close() {
	if c == nil || c.clientCloser == nil {
		return
	}

	c.closeOnce.Do(c.clientCloser.Close)
}
