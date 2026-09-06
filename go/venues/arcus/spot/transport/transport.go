// Package transport defines the Spot operation executor contract.
package transport

import (
	"context"
	"net/url"
)

type Executor interface {
	Get(context.Context, string, url.Values, any) error
}
