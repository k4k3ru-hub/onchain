// Package transport defines the consumer-owned market operation dependency.
package transport

import (
	"context"
	"net/url"
)

type Executor interface {
	Get(context.Context, string, url.Values, any) error
}
