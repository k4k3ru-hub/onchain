package subscriptions_test

import (
	"context"
	"errors"
	"testing"

	"github.com/k4k3ru-hub/onchain/go/venues/arcus/websocket/subscriptions"
)

type sender func(context.Context, []byte) error

func (f sender) Send(c context.Context, b []byte) error { return f(c, b) }
func TestAggregationIdentityAndValidation(t *testing.T) {
	var frames []string
	c, err := subscriptions.NewClient(sender(func(_ context.Context, b []byte) error { frames = append(frames, string(b)); return nil }), "l2OrderbookUpdates")
	if err != nil {
		t.Fatal(err)
	}
	snapshot := false
	p := subscriptions.Params{Market: "BTC-USD", NLevels: 100, SigFigs: 5, RoundStep: 2, Snapshot: &snapshot}
	if err := c.Subscribe(context.Background(), p); err != nil {
		t.Fatal(err)
	}
	if err := c.Unsubscribe(context.Background(), p); err != nil {
		t.Fatal(err)
	}
	if frames[0] != `{"type":"subscribe","channel":"l2OrderbookUpdates","id":"BTC-USD","nLevels":100,"sigFigs":5,"roundStep":2,"snapshot":false}` || frames[1] != `{"type":"unsubscribe","channel":"l2OrderbookUpdates","id":"BTC-USD","sigFigs":5,"roundStep":2}` {
		t.Fatal(frames)
	}
	for _, p := range []subscriptions.Params{{}, {Market: "BTC-USD", NLevels: 101}, {Market: "BTC-USD", SigFigs: 1}, {Market: "BTC-USD", RoundStep: 3}} {
		if err := c.Subscribe(context.Background(), p); err == nil {
			t.Fatal("invalid options accepted")
		}
	}
	if len(frames) != 2 {
		t.Fatal("invalid request sent")
	}
	cause := errors.New("sender error")
	c, err = subscriptions.NewClient(sender(func(context.Context, []byte) error { return cause }), "markets")
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Subscribe(context.Background(), subscriptions.Params{}); !errors.Is(err, cause) {
		t.Fatal(err)
	}
	if err := c.Subscribe(context.Background(), subscriptions.Params{Market: "BTC-USD"}); err == nil || errors.Is(err, cause) {
		t.Fatal("global channel accepted market")
	}
	var nilSender sender
	if _, err := subscriptions.NewClient(nilSender, "markets"); err == nil {
		t.Fatal("typed nil accepted")
	}
}
