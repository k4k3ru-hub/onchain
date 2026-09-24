package sui

import (
	"context"
	"fmt"
	"strconv"
	"strings"
)

type Coin struct {
	Reference ObjectReference
	Owner     Address
	CoinType  string
	Balance   uint64
}

type CoinQuery struct {
	Owner    Address
	CoinType string
	First    int
	After    string
}

type CoinPage struct {
	Coins       []Coin
	HasNextPage bool
	NextCursor  string
}

type coinContents struct {
	Type *struct {
		Repr string `json:"repr"`
	} `json:"type"`
	JSON *struct {
		Balance *string `json:"balance"`
	} `json:"json"`
}

type coinObject struct {
	Address string `json:"address"`
	Version uint64 `json:"version"`
	Digest  string `json:"digest"`
	Owner   *struct {
		Kind    string `json:"__typename"`
		Address *struct {
			Address string `json:"address"`
		} `json:"address"`
	} `json:"owner"`
	Contents *coinContents `json:"contents"`
}

const coinObjectFields = `address version digest owner { __typename ... on AddressOwner { address { address } } }`
const coinContentsFields = `contents { type { repr } json }`

// Coins returns one page of real address-owned Coin<T> objects of the requested type.
// It neither selects funds nor includes address-balance compatibility reservations.
// Re-read chosen references before signing; pagination is not a wallet lock.
//
// Version:
//   - 2026-09-24: Added.
func (c *RPCClient) Coins(ctx context.Context, query CoinQuery) (CoinPage, error) {
	if c == nil || c.caller == nil {
		return CoinPage{}, fmt.Errorf("failed to list sui coins: provider=null")
	}
	if query.Owner.IsZero() {
		return CoinPage{}, fmt.Errorf("failed to list sui coins: owner=empty")
	}
	if query.First < 0 || query.First > 50 {
		return CoinPage{}, fmt.Errorf("failed to list sui coins: first=out_of_range min_value=0 max_value=50")
	}
	if len(query.After) > 4096 {
		return CoinPage{}, fmt.Errorf("failed to list sui coins: cursor=too_long max_length=4096")
	}
	coinType, err := parseTransactionTypeTag(query.CoinType)
	if err != nil {
		return CoinPage{}, fmt.Errorf("failed to list sui coins: %w", err)
	}
	if coinType.kind != 7 {
		return CoinPage{}, fmt.Errorf("failed to list sui coins: coin_type=invalid")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	first := query.First
	if first == 0 {
		first = 50
	}
	var result struct {
		Address *struct {
			Address string `json:"address"`
			Objects *struct {
				Nodes    []*coinObject `json:"nodes"`
				PageInfo *struct {
					HasNextPage bool   `json:"hasNextPage"`
					EndCursor   string `json:"endCursor"`
				} `json:"pageInfo"`
			} `json:"objects"`
		} `json:"address"`
	}
	request := fmt.Sprintf(`query { address(address: %q) { address objects(first: %d, after: %q, filter: { type: %q }) { nodes { %s %s } pageInfo { hasNextPage endCursor } } } }`, query.Owner.String(), first, query.After, "0x2::coin::Coin<"+coinType.text()+">", coinObjectFields, coinContentsFields)
	// Omit an absent cursor rather than sending an empty opaque cursor.
	if query.After == "" {
		request = strings.Replace(request, `, after: ""`, "", 1)
	}
	if err := c.caller.query(ctx, request, &result); err != nil {
		return CoinPage{}, fmt.Errorf("failed to list sui coins: %w", err)
	}
	if result.Address == nil || result.Address.Objects == nil || result.Address.Objects.PageInfo == nil {
		return CoinPage{}, fmt.Errorf("failed to list sui coins: response=null")
	}
	owner, err := ParseAddress(result.Address.Address)
	if err != nil {
		return CoinPage{}, fmt.Errorf("failed to list sui coins: %w", err)
	}
	if owner != query.Owner {
		return CoinPage{}, fmt.Errorf("failed to list sui coins: owner=mismatch")
	}
	objects := result.Address.Objects
	if len(objects.Nodes) > first {
		return CoinPage{}, fmt.Errorf("failed to list sui coins: page=too_long")
	}
	page := CoinPage{Coins: make([]Coin, 0, len(objects.Nodes)), HasNextPage: objects.PageInfo.HasNextPage, NextCursor: objects.PageInfo.EndCursor}
	if page.HasNextPage && (page.NextCursor == "" || page.NextCursor == query.After) {
		return CoinPage{}, fmt.Errorf("failed to list sui coins: next_cursor=invalid")
	}
	seen := make(map[Address]struct{}, len(objects.Nodes))
	for _, object := range objects.Nodes {
		coin, err := parseCoinObject(object)
		if err != nil {
			return CoinPage{}, fmt.Errorf("failed to list sui coins: %w", err)
		}
		if coin.Owner != owner || coin.CoinType != coinType.text() {
			return CoinPage{}, fmt.Errorf("failed to list sui coins: coin=mismatch")
		}
		if _, exists := seen[coin.Reference.Address]; exists {
			return CoinPage{}, fmt.Errorf("failed to list sui coins: object=duplicate")
		}
		seen[coin.Reference.Address] = struct{}{}
		page.Coins = append(page.Coins, *coin)
	}
	return page, nil
}

// Coin reads a real address-owned coin and verifies its exact requested reference.
// A stale version or digest is rejected instead of silently selecting newer funds.
//
// Version:
//   - 2026-09-24: Added.
func (c *RPCClient) Coin(ctx context.Context, reference ObjectReference) (*Coin, error) {
	if c == nil || c.caller == nil {
		return nil, fmt.Errorf("failed to get sui coin: provider=null")
	}
	if err := reference.Validate(); err != nil {
		return nil, fmt.Errorf("failed to get sui coin: %w", err)
	}
	if ctx == nil {
		ctx = context.Background()
	}
	var result struct {
		Object *struct {
			coinObject
			Move *struct {
				Contents *coinContents `json:"contents"`
			} `json:"asMoveObject"`
		} `json:"object"`
	}
	request := fmt.Sprintf(`query { object(address: %q) { %s asMoveObject { %s } } }`, reference.Address.String(), coinObjectFields, coinContentsFields)
	if err := c.caller.query(ctx, request, &result); err != nil {
		return nil, fmt.Errorf("failed to get sui coin: %w", err)
	}
	if result.Object == nil || result.Object.Move == nil {
		return nil, fmt.Errorf("failed to get sui coin: object=null")
	}
	object := result.Object.coinObject
	object.Contents = result.Object.Move.Contents
	coin, err := parseCoinObject(&object)
	if err != nil {
		return nil, fmt.Errorf("failed to get sui coin: %w", err)
	}
	if coin.Reference != reference {
		return nil, fmt.Errorf("failed to get sui coin: object_reference=mismatch")
	}
	return coin, nil
}

func parseCoinObject(value *coinObject) (*Coin, error) {
	if value == nil || value.Contents == nil || value.Contents.Type == nil || value.Contents.JSON == nil || value.Contents.JSON.Balance == nil {
		return nil, fmt.Errorf("failed to parse sui coin: contents=null")
	}
	if value.Owner == nil || value.Owner.Kind != "AddressOwner" || value.Owner.Address == nil {
		return nil, fmt.Errorf("failed to parse sui coin: owner=unsupported")
	}
	address, err := ParseAddress(value.Address)
	if err != nil {
		return nil, fmt.Errorf("failed to parse sui coin: %w", err)
	}
	digest, err := ParseObjectDigest(value.Digest)
	if err != nil {
		return nil, fmt.Errorf("failed to parse sui coin: %w", err)
	}
	owner, err := ParseAddress(value.Owner.Address.Address)
	if err != nil {
		return nil, fmt.Errorf("failed to parse sui coin: %w", err)
	}
	tag, err := parseTransactionTypeTag(value.Contents.Type.Repr)
	if err != nil {
		return nil, fmt.Errorf("failed to parse sui coin: %w", err)
	}
	framework := Address{}
	framework[31] = 2
	if tag.kind != 7 || tag.address != framework || tag.module != "coin" || tag.name != "Coin" || len(tag.parameters) != 1 || tag.parameters[0].kind != 7 {
		return nil, fmt.Errorf("failed to parse sui coin: coin_type=invalid")
	}
	balance, err := strconv.ParseUint(*value.Contents.JSON.Balance, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("failed to parse sui coin: balance=invalid")
	}
	coin := &Coin{Reference: ObjectReference{Address: address, Version: value.Version, Digest: digest}, Owner: owner, CoinType: tag.parameters[0].text(), Balance: balance}
	if err := coin.Reference.Validate(); err != nil {
		return nil, fmt.Errorf("failed to parse sui coin: %w", err)
	}
	if owner.IsZero() {
		return nil, fmt.Errorf("failed to parse sui coin: owner=empty")
	}
	return coin, nil
}
