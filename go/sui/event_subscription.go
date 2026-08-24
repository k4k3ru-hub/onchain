package sui

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/k4k3ru-hub/onchain/go/sui/internal/rpcv2"
	"google.golang.org/protobuf/types/known/fieldmaskpb"
)

type LiveEventFilter struct {
	Sender *Address
	Module string
	Type   string
}

type LiveEvent struct {
	Checkpoint       CheckpointSequenceNumber
	Transaction      TransactionDigest
	TransactionIndex uint64
	EventIndex       uint32
	Sender           Address
	Package          Address
	Module           string
	Type             string
	BCS              []byte
	JSON             json.RawMessage
}

type EventWatermark struct {
	Cursor     []byte
	Checkpoint *CheckpointSequenceNumber
}

type EventNotification struct {
	Event     *LiveEvent
	Watermark EventWatermark
}

type EventSubscription struct {
	receiver  liveEventReceiver
	closeOnce sync.Once
}

type grpcEventReceiver struct {
	stream interface {
		Recv() (*rpcv2.SubscribeEventsResponse, error)
	}
	cancel    context.CancelFunc
	closeOnce sync.Once
}

// Validate validates a Sui live event filter.
func (f LiveEventFilter) Validate() error {
	if f.Sender != nil && f.Sender.IsZero() {
		return fmt.Errorf("failed to validate sui live event filter: sender=empty")
	}
	if utf8.RuneCountInString(f.Module) > 512 {
		return fmt.Errorf("failed to validate sui live event filter: module=too_long actual_length=%d max_length=512", utf8.RuneCountInString(f.Module))
	}
	if utf8.RuneCountInString(f.Type) > 1024 {
		return fmt.Errorf("failed to validate sui live event filter: type=too_long actual_length=%d max_length=1024", utf8.RuneCountInString(f.Type))
	}
	if f.Module != "" {
		if _, err := normalizeMovePath(f.Module); err != nil {
			return fmt.Errorf("failed to validate sui live event filter: module=invalid: %w", err)
		}
	}
	if f.Type != "" {
		if _, err := normalizeMovePath(f.Type); err != nil {
			return fmt.Errorf("failed to validate sui live event filter: type=invalid: %w", err)
		}
	}
	return nil
}

// SubscribeEvents subscribes to live Sui events using gRPC.
//
// The subscription starts at the current node tip. Its lifetime is bound to
// ctx and can also be ended explicitly with EventSubscription.Close.
//
// Version:
//   - 2026-08-23: Added.
func (c *GRPCClient) SubscribeEvents(ctx context.Context, filter LiveEventFilter) (*EventSubscription, error) {
	if c == nil || c.provider == nil {
		return nil, fmt.Errorf("failed to subscribe sui events: event_provider=null")
	}
	if err := filter.Validate(); err != nil {
		return nil, fmt.Errorf("failed to subscribe sui events: %w", err)
	}
	if ctx == nil {
		ctx = context.Background()
	}
	receiver, err := c.provider.subscribeEvents(ctx, filter)
	if err != nil {
		return nil, fmt.Errorf("failed to subscribe sui events: %w", err)
	}
	if receiver == nil {
		return nil, fmt.Errorf("failed to subscribe sui events: event_receiver=null")
	}
	return &EventSubscription{receiver: receiver}, nil
}

// Recv waits for the next Sui event or progress-only notification.
//
// Cancelling ctx closes the subscription because a gRPC stream receive cannot
// be cancelled independently from its stream.
func (s *EventSubscription) Recv(ctx context.Context) (*EventNotification, error) {
	if s == nil || s.receiver == nil {
		return nil, fmt.Errorf("failed to receive sui event: subscription=null")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	type result struct {
		notification *EventNotification
		err          error
	}
	resultChannel := make(chan result, 1)
	go func() {
		notification, err := s.receiver.Recv()
		resultChannel <- result{notification: notification, err: err}
	}()
	select {
	case <-ctx.Done():
		s.Close()
		return nil, fmt.Errorf("failed to receive sui event: %w", ctx.Err())
	case received := <-resultChannel:
		if received.err != nil {
			return nil, fmt.Errorf("failed to receive sui event: %w", received.err)
		}
		if received.notification == nil {
			return nil, fmt.Errorf("failed to receive sui event: notification=null")
		}
		return received.notification, nil
	}
}

// Close closes the Sui event subscription.
func (s *EventSubscription) Close() {
	if s != nil {
		s.closeOnce.Do(func() {
			if s.receiver != nil {
				s.receiver.Close()
			}
		})
	}
}

func (a *grpcAdapter) subscribeEvents(ctx context.Context, filter LiveEventFilter) (liveEventReceiver, error) {
	streamContext, cancel := context.WithCancel(ctx)
	grpcFilter, err := makeGRPCEventFilter(filter)
	if err != nil {
		cancel()
		return nil, err
	}
	request := &rpcv2.SubscribeEventsRequest{
		ReadMask: &fieldmaskpb.FieldMask{Paths: []string{"package_id", "module", "sender", "event_type", "contents", "json", "checkpoint", "transaction_digest", "transaction_index", "event_index"}},
		Filter:   grpcFilter,
	}
	stream, err := a.client.SubscribeEvents(streamContext, request)
	if err != nil {
		cancel()
		return nil, err
	}
	return &grpcEventReceiver{stream: stream, cancel: cancel}, nil
}

func (r *grpcEventReceiver) Recv() (*EventNotification, error) {
	response, err := r.stream.Recv()
	if err != nil {
		return nil, err
	}
	if response == nil || response.Watermark == nil || len(response.Watermark.Cursor) == 0 {
		return nil, fmt.Errorf("failed to decode sui grpc event: watermark=invalid")
	}
	notification := &EventNotification{Watermark: EventWatermark{Cursor: append([]byte(nil), response.Watermark.Cursor...)}}
	if response.Watermark.Checkpoint != nil {
		checkpoint := CheckpointSequenceNumber(*response.Watermark.Checkpoint)
		notification.Watermark.Checkpoint = &checkpoint
	}
	if response.Event != nil {
		event, err := decodeGRPCEvent(response.Event)
		if err != nil {
			return nil, err
		}
		notification.Event = event
	}
	return notification, nil
}

func (r *grpcEventReceiver) Close() {
	if r != nil {
		r.closeOnce.Do(func() {
			if r.cancel != nil {
				r.cancel()
			}
		})
	}
}

func makeGRPCEventFilter(filter LiveEventFilter) (*rpcv2.EventFilter, error) {
	literals := make([]*rpcv2.EventLiteral, 0, 3)
	if filter.Sender != nil {
		value := filter.Sender.String()
		literals = append(literals, &rpcv2.EventLiteral{Predicate: &rpcv2.EventLiteral_Sender{Sender: &rpcv2.SenderFilter{Address: &value}}})
	}
	if strings.TrimSpace(filter.Module) != "" {
		value, err := normalizeMovePath(filter.Module)
		if err != nil {
			return nil, fmt.Errorf("failed to create sui grpc event filter: %w", err)
		}
		literals = append(literals, &rpcv2.EventLiteral{Predicate: &rpcv2.EventLiteral_EmitModule{EmitModule: &rpcv2.EmitModuleFilter{Module: &value}}})
	}
	if strings.TrimSpace(filter.Type) != "" {
		value, err := normalizeMovePath(filter.Type)
		if err != nil {
			return nil, fmt.Errorf("failed to create sui grpc event filter: %w", err)
		}
		literals = append(literals, &rpcv2.EventLiteral{Predicate: &rpcv2.EventLiteral_EventType{EventType: &rpcv2.EventTypeFilter{EventType: &value}}})
	}
	if len(literals) == 0 {
		return nil, nil
	}
	return &rpcv2.EventFilter{Terms: []*rpcv2.EventTerm{{Literals: literals}}}, nil
}

func normalizeMovePath(value string) (string, error) {
	trimmed := strings.TrimSpace(value)
	addressText := trimmed
	suffix := ""
	if separator := strings.Index(trimmed, "::"); separator >= 0 {
		addressText = trimmed[:separator]
		suffix = trimmed[separator:]
	}
	address, err := ParseAddress(addressText)
	if err != nil {
		return "", err
	}
	return address.String() + suffix, nil
}

// NormalizeMoveType normalizes the leading address of a Sui Move type.
//
// Version:
//   - 2026-08-23: Added.
func NormalizeMoveType(value string) (string, error) {
	normalized, err := normalizeMovePath(value)
	if err != nil || !strings.Contains(normalized, "::") {
		return "", fmt.Errorf("failed to normalize sui move type: move_type=invalid")
	}
	return normalized, nil
}

func decodeGRPCEvent(value *rpcv2.Event) (*LiveEvent, error) {
	if value.Checkpoint == nil || value.TransactionDigest == nil || value.TransactionIndex == nil || value.EventIndex == nil {
		return nil, fmt.Errorf("failed to decode sui grpc event: ledger_position=null")
	}
	sender, err := ParseAddress(value.GetSender())
	if err != nil {
		return nil, fmt.Errorf("failed to decode sui grpc event: %w", err)
	}
	packageAddress, err := ParseAddress(value.GetPackageId())
	if err != nil {
		return nil, fmt.Errorf("failed to decode sui grpc event: %w", err)
	}
	transaction, err := ParseTransactionDigest(value.GetTransactionDigest())
	if err != nil {
		return nil, fmt.Errorf("failed to decode sui grpc event: %w", err)
	}
	if strings.TrimSpace(value.GetModule()) == "" || strings.TrimSpace(value.GetEventType()) == "" {
		return nil, fmt.Errorf("failed to decode sui grpc event: event_identity=invalid")
	}
	var jsonValue json.RawMessage
	if value.Json != nil {
		encoded, err := json.Marshal(value.Json.AsInterface())
		if err != nil {
			return nil, fmt.Errorf("failed to decode sui grpc event: failed to encode json: %w", err)
		}
		jsonValue = encoded
	}
	var bcs []byte
	if value.Contents != nil {
		bcs = append([]byte(nil), value.Contents.Value...)
	}
	return &LiveEvent{
		Checkpoint: CheckpointSequenceNumber(value.GetCheckpoint()), Transaction: transaction,
		TransactionIndex: value.GetTransactionIndex(), EventIndex: value.GetEventIndex(),
		Sender: sender, Package: packageAddress, Module: value.GetModule(), Type: value.GetEventType(),
		BCS: bcs, JSON: jsonValue,
	}, nil
}
