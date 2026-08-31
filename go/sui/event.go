package sui

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	defaultEventPageSize = 50
	maxEventPageSize     = 100
)

type EventFilter struct {
	Sender           *Address
	Module           string
	Type             string
	AfterCheckpoint  *CheckpointSequenceNumber
	AtCheckpoint     *CheckpointSequenceNumber
	BeforeCheckpoint *CheckpointSequenceNumber
}

type EventQuery struct {
	Filter EventFilter
	First  int
	After  string
}

type Event struct {
	Checkpoint     CheckpointSequenceNumber
	SequenceNumber uint64
	Sender         Address
	Timestamp      time.Time
	Transaction    TransactionDigest
	Package        Address
	Module         string
	Type           string
	JSON           json.RawMessage
}

type EventPage struct {
	Events      []Event
	HasNextPage bool
	NextCursor  string
}

// Validate validates a Sui event filter.
func (f EventFilter) Validate() error {
	if f.Sender != nil && f.Sender.IsZero() {
		return fmt.Errorf("failed to validate sui event filter: sender=empty")
	}
	if utf8.RuneCountInString(f.Module) > 512 {
		return fmt.Errorf("failed to validate sui event filter: module=too_long actual_length=%d max_length=512", utf8.RuneCountInString(f.Module))
	}
	if utf8.RuneCountInString(f.Type) > 1024 {
		return fmt.Errorf("failed to validate sui event filter: type=too_long actual_length=%d max_length=1024", utf8.RuneCountInString(f.Type))
	}
	if f.AtCheckpoint != nil && (f.AfterCheckpoint != nil || f.BeforeCheckpoint != nil) {
		return fmt.Errorf("failed to validate sui event filter: checkpoint_range=invalid")
	}
	if f.AfterCheckpoint != nil && f.BeforeCheckpoint != nil && *f.AfterCheckpoint >= *f.BeforeCheckpoint {
		return fmt.Errorf("failed to validate sui event filter: checkpoint_range=invalid")
	}
	return nil
}

// Validate validates a Sui event query.
func (q EventQuery) Validate() error {
	if err := q.Filter.Validate(); err != nil {
		return fmt.Errorf("failed to validate sui event query: %w", err)
	}
	if q.First < 0 || q.First > maxEventPageSize {
		return fmt.Errorf("failed to validate sui event query: first=out_of_range min_value=0 max_value=%d", maxEventPageSize)
	}
	if utf8.RuneCountInString(q.After) > 4096 {
		return fmt.Errorf("failed to validate sui event query: after=too_long actual_length=%d max_length=4096", utf8.RuneCountInString(q.After))
	}
	return nil
}

// Events returns a page of Sui events matching a GraphQL event filter.
//
// Parameters:
//   - ctx: request context; nil uses context.Background.
//   - eventQuery: filter and forward-pagination parameters.
//
// Returns:
//   - SDK-owned event page.
//   - Retrieval or validation error.
//
// Version:
//   - 2026-08-23: Added.
func (c *RPCClient) Events(ctx context.Context, eventQuery EventQuery) (EventPage, error) {
	if c == nil {
		return EventPage{}, fmt.Errorf("failed to get sui events: rpc_client=null")
	}
	if c.caller == nil {
		return EventPage{}, fmt.Errorf("failed to get sui events: event_provider=null")
	}
	if err := eventQuery.Validate(); err != nil {
		return EventPage{}, fmt.Errorf("failed to get sui events: %w", err)
	}
	if ctx == nil {
		ctx = context.Background()
	}
	query := buildEventQuery(eventQuery)
	var result struct {
		Events struct {
			Nodes    []eventResponse `json:"nodes"`
			PageInfo struct {
				HasNextPage bool   `json:"hasNextPage"`
				EndCursor   string `json:"endCursor"`
			} `json:"pageInfo"`
		} `json:"events"`
	}
	if err := c.caller.query(ctx, query, &result); err != nil {
		return EventPage{}, fmt.Errorf("failed to get sui events: %w", err)
	}
	events := make([]Event, 0, len(result.Events.Nodes))
	for _, response := range result.Events.Nodes {
		event, err := response.event()
		if err != nil {
			return EventPage{}, fmt.Errorf("failed to get sui events: %w", err)
		}
		events = append(events, event)
	}
	if result.Events.PageInfo.HasNextPage && result.Events.PageInfo.EndCursor == "" {
		return EventPage{}, fmt.Errorf("failed to get sui events: next_cursor=empty")
	}
	return EventPage{Events: events, HasNextPage: result.Events.PageInfo.HasNextPage, NextCursor: result.Events.PageInfo.EndCursor}, nil
}

type eventResponse struct {
	SequenceNumber uint64 `json:"sequenceNumber"`
	Sender         *struct {
		Address string `json:"address"`
	} `json:"sender"`
	Timestamp   string `json:"timestamp"`
	Transaction *struct {
		Digest  string `json:"digest"`
		Effects *struct {
			Checkpoint *struct {
				SequenceNumber CheckpointSequenceNumber `json:"sequenceNumber"`
			} `json:"checkpoint"`
		} `json:"effects"`
	} `json:"transaction"`
	TransactionModule *struct {
		Package *struct {
			Address string `json:"address"`
		} `json:"package"`
		Name string `json:"name"`
	} `json:"transactionModule"`
	Contents *struct {
		Type *struct {
			Representation string `json:"repr"`
		} `json:"type"`
		JSON json.RawMessage `json:"json"`
	} `json:"contents"`
}

func (r eventResponse) event() (Event, error) {
	if r.Sender == nil || r.Transaction == nil || r.Transaction.Effects == nil || r.Transaction.Effects.Checkpoint == nil || r.TransactionModule == nil || r.TransactionModule.Package == nil || r.Contents == nil || r.Contents.Type == nil {
		return Event{}, fmt.Errorf("failed to parse sui event: event_fields=null")
	}
	sender, err := ParseAddress(r.Sender.Address)
	if err != nil {
		return Event{}, fmt.Errorf("failed to parse sui event: %w", err)
	}
	transaction, err := ParseTransactionDigest(r.Transaction.Digest)
	if err != nil {
		return Event{}, fmt.Errorf("failed to parse sui event: %w", err)
	}
	packageAddress, err := ParseAddress(r.TransactionModule.Package.Address)
	if err != nil {
		return Event{}, fmt.Errorf("failed to parse sui event: %w", err)
	}
	timestamp, err := time.Parse(time.RFC3339Nano, r.Timestamp)
	if err != nil {
		return Event{}, fmt.Errorf("failed to parse sui event: timestamp=invalid: %w", err)
	}
	if strings.TrimSpace(r.TransactionModule.Name) == "" {
		return Event{}, fmt.Errorf("failed to parse sui event: module=empty")
	}
	if strings.TrimSpace(r.Contents.Type.Representation) == "" {
		return Event{}, fmt.Errorf("failed to parse sui event: type=empty")
	}
	return Event{
		Checkpoint:     r.Transaction.Effects.Checkpoint.SequenceNumber,
		SequenceNumber: r.SequenceNumber,
		Sender:         sender, Timestamp: timestamp, Transaction: transaction,
		Package: packageAddress, Module: r.TransactionModule.Name,
		Type: r.Contents.Type.Representation, JSON: append(json.RawMessage(nil), r.Contents.JSON...),
	}, nil
}

func buildEventQuery(eventQuery EventQuery) string {
	first := eventQuery.First
	if first == 0 {
		first = defaultEventPageSize
	}
	arguments := []string{fmt.Sprintf("first: %d", first)}
	if eventQuery.After != "" {
		arguments = append(arguments, fmt.Sprintf("after: %q", eventQuery.After))
	}
	filterFields := make([]string, 0, 6)
	filter := eventQuery.Filter
	if filter.Sender != nil {
		filterFields = append(filterFields, fmt.Sprintf("sender: %q", filter.Sender.String()))
	}
	if filter.Module != "" {
		filterFields = append(filterFields, fmt.Sprintf("module: %q", filter.Module))
	}
	if filter.Type != "" {
		filterFields = append(filterFields, fmt.Sprintf("type: %q", filter.Type))
	}
	if filter.AfterCheckpoint != nil {
		filterFields = append(filterFields, fmt.Sprintf("afterCheckpoint: %d", *filter.AfterCheckpoint))
	}
	if filter.AtCheckpoint != nil {
		filterFields = append(filterFields, fmt.Sprintf("atCheckpoint: %d", *filter.AtCheckpoint))
	}
	if filter.BeforeCheckpoint != nil {
		filterFields = append(filterFields, fmt.Sprintf("beforeCheckpoint: %d", *filter.BeforeCheckpoint))
	}
	if len(filterFields) > 0 {
		arguments = append(arguments, "filter: { "+strings.Join(filterFields, " ")+" }")
	}
	return "query { events(" + strings.Join(arguments, " ") + ") { nodes { sequenceNumber sender { address } timestamp transaction { digest effects { checkpoint { sequenceNumber } } } transactionModule { package { address } name } contents { type { repr } json } } pageInfo { hasNextPage endCursor } } }"
}
