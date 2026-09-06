// Package markets composes Lighter public market-data operations.
package markets

import (
	"fmt"

	"github.com/k4k3ru-hub/onchain/go/venues/lighter/internal/safety"
	"github.com/k4k3ru-hub/onchain/go/venues/lighter/rest/markets/funding_rates"
	"github.com/k4k3ru-hub/onchain/go/venues/lighter/rest/markets/fundings"
	"github.com/k4k3ru-hub/onchain/go/venues/lighter/rest/markets/order_book_details"
	"github.com/k4k3ru-hub/onchain/go/venues/lighter/rest/markets/order_book_orders"
	"github.com/k4k3ru-hub/onchain/go/venues/lighter/rest/markets/order_books"
	"github.com/k4k3ru-hub/onchain/go/venues/lighter/rest/markets/recent_trades"
	"github.com/k4k3ru-hub/onchain/go/venues/lighter/rest/transport"
)

type Client struct {
	OrderBooks       *order_books.Client
	OrderBookDetails *order_book_details.Client
	OrderBookOrders  *order_book_orders.Client
	RecentTrades     *recent_trades.Client
	Fundings         *fundings.Client
	FundingRates     *funding_rates.Client
}

// NewClient composes all public market-data operation clients.
//
// Version:
//   - 2026-09-06: Added.
func NewClient(executor transport.Executor) (*Client, error) {
	if safety.IsNil(executor) {
		return nil, fmt.Errorf("failed to create markets client: executor=null")
	}
	c := &Client{}
	var err error
	c.OrderBooks, err = order_books.NewClient(executor)
	if err != nil {
		return nil, fmt.Errorf("failed to create markets client: %w", err)
	}
	c.OrderBookDetails, err = order_book_details.NewClient(executor)
	if err != nil {
		return nil, fmt.Errorf("failed to create markets client: %w", err)
	}
	c.OrderBookOrders, err = order_book_orders.NewClient(executor)
	if err != nil {
		return nil, fmt.Errorf("failed to create markets client: %w", err)
	}
	c.RecentTrades, err = recent_trades.NewClient(executor)
	if err != nil {
		return nil, fmt.Errorf("failed to create markets client: %w", err)
	}
	c.Fundings, err = fundings.NewClient(executor)
	if err != nil {
		return nil, fmt.Errorf("failed to create markets client: %w", err)
	}
	c.FundingRates, err = funding_rates.NewClient(executor)
	if err != nil {
		return nil, fmt.Errorf("failed to create markets client: %w", err)
	}
	return c, nil
}
