package sui

import (
	"strings"
	"testing"

	"github.com/mr-tron/base58"
)

func TestObject(t *testing.T) {
	address, err := ParseAddress("0x6")
	if err != nil {
		t.Fatalf("ParseAddress() returned an unexpected error: %v", err)
	}
	digest := base58.Encode(bytesWithFirstByte(10))
	caller := &fakeCaller{response: map[string]any{"object": map[string]any{
		"address": address.String(), "version": 12, "digest": digest, "objectBcs": "AA==",
		"asMoveObject": map[string]any{"contents": map[string]any{
			"type": map[string]any{"repr": "0x2::clock::Clock"}, "json": map[string]any{"id": address.String()},
		}},
	}}}
	client := composeRPCClient(RPCConfig{URL: "https://example.com/graphql"}, caller)
	object, err := client.Object(nil, address)
	if err != nil {
		t.Fatalf("Object() returned an unexpected error: %v", err)
	}
	if object.Address != address || object.Version != 12 || object.Digest.String() != digest || object.Move == nil || object.Move.Type != "0x2::clock::Clock" {
		t.Fatalf("Object() = %+v, want matching Move object", object)
	}
}

func TestMovePackage(t *testing.T) {
	address, err := ParseAddress("0x2")
	if err != nil {
		t.Fatalf("ParseAddress() returned an unexpected error: %v", err)
	}
	digest := base58.Encode(bytesWithFirstByte(11))
	caller := &fakeCaller{response: map[string]any{"package": map[string]any{
		"address": address.String(), "version": 58, "digest": digest,
		"modules": map[string]any{"nodes": []any{map[string]any{"name": "coin"}, map[string]any{"name": "transfer"}}, "pageInfo": map[string]any{"hasNextPage": false}},
	}}}
	client := composeRPCClient(RPCConfig{URL: "https://example.com/graphql"}, caller)
	movePackage, err := client.MovePackage(nil, address)
	if err != nil {
		t.Fatalf("MovePackage() returned an unexpected error: %v", err)
	}
	if movePackage.Address != address || movePackage.Digest.String() != digest || len(movePackage.Modules) != 2 || movePackage.Modules[0] != "coin" {
		t.Fatalf("MovePackage() = %+v, want matching package", movePackage)
	}
}

func TestEvents(t *testing.T) {
	sender, err := ParseAddress("0x123")
	if err != nil {
		t.Fatalf("ParseAddress() returned an unexpected error: %v", err)
	}
	packageAddress, err := ParseAddress("0x2")
	if err != nil {
		t.Fatalf("ParseAddress() returned an unexpected error: %v", err)
	}
	transaction := base58.Encode(bytesWithFirstByte(12))
	caller := &fakeCaller{response: map[string]any{"events": map[string]any{
		"nodes": []any{map[string]any{
			"sequenceNumber": 0, "sender": map[string]any{"address": sender.String()}, "timestamp": "2026-08-23T01:02:03Z",
			"transaction":       map[string]any{"digest": transaction, "effects": map[string]any{"checkpoint": map[string]any{"sequenceNumber": 100}}},
			"transactionModule": map[string]any{"package": map[string]any{"address": packageAddress.String()}, "name": "coin"},
			"contents":          map[string]any{"type": map[string]any{"repr": "0x2::coin::Transfer"}, "json": map[string]any{"amount": "100"}},
		}},
		"pageInfo": map[string]any{"hasNextPage": true, "endCursor": "next-cursor"},
	}}}
	client := composeRPCClient(RPCConfig{URL: "https://example.com/graphql"}, caller)
	atCheckpoint := CheckpointSequenceNumber(100)
	page, err := client.Events(nil, EventQuery{First: 10, Filter: EventFilter{Sender: &sender, AtCheckpoint: &atCheckpoint}})
	if err != nil {
		t.Fatalf("Events() returned an unexpected error: %v", err)
	}
	if len(page.Events) != 1 || page.Events[0].Checkpoint != 100 || page.Events[0].Sender != sender || page.Events[0].Transaction.String() != transaction || !page.HasNextPage || page.NextCursor != "next-cursor" {
		t.Fatalf("Events() = %+v, want matching event page", page)
	}
	if !strings.Contains(caller.queryValue, "first: 10") || !strings.Contains(caller.queryValue, "atCheckpoint: 100") || !strings.Contains(caller.queryValue, "effects { checkpoint { sequenceNumber } }") || !strings.Contains(caller.queryValue, sender.String()) {
		t.Fatalf("Events() query = %q", caller.queryValue)
	}
}

func TestEventQueryValidateRejectsInvalidRange(t *testing.T) {
	after := CheckpointSequenceNumber(20)
	before := CheckpointSequenceNumber(10)
	err := (EventQuery{Filter: EventFilter{AfterCheckpoint: &after, BeforeCheckpoint: &before}}).Validate()
	if err == nil || !strings.Contains(err.Error(), "checkpoint_range=invalid") {
		t.Fatalf("EventQuery.Validate() error = %v, want invalid checkpoint range", err)
	}
}
