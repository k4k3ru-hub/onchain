package sui

import (
	"context"
	"fmt"
	"strings"
	"unicode/utf8"
)

type TransactionQuery struct {
	AffectedAddress  Address
	First            int
	After            string
	AfterCheckpoint  *CheckpointSequenceNumber
	AtCheckpoint     *CheckpointSequenceNumber
	BeforeCheckpoint *CheckpointSequenceNumber
}

type TransactionDigestPage struct {
	Digests     []TransactionDigest
	HasNextPage bool
	NextCursor  string
}

// Validate validates a Sui transaction query.
func (q TransactionQuery) Validate() error {
	if q.AffectedAddress.IsZero() {
		return fmt.Errorf("failed to validate sui transaction query: affected_address=empty")
	}
	if q.First < 0 || q.First > 100 {
		return fmt.Errorf("failed to validate sui transaction query: first=out_of_range min_value=0 max_value=100")
	}
	if utf8.RuneCountInString(q.After) > 4096 {
		return fmt.Errorf("failed to validate sui transaction query: after=too_long actual_length=%d max_length=4096", utf8.RuneCountInString(q.After))
	}
	if q.AtCheckpoint != nil && (q.AfterCheckpoint != nil || q.BeforeCheckpoint != nil) {
		return fmt.Errorf("failed to validate sui transaction query: checkpoint_range=invalid")
	}
	if q.AfterCheckpoint != nil && q.BeforeCheckpoint != nil && *q.AfterCheckpoint >= *q.BeforeCheckpoint {
		return fmt.Errorf("failed to validate sui transaction query: checkpoint_range=invalid")
	}
	return nil
}

// TransactionDigests returns transactions affecting an address.
//
// Version:
//   - 2026-08-23: Added.
func (c *RPCClient) TransactionDigests(ctx context.Context, transactionQuery TransactionQuery) (TransactionDigestPage, error) {
	if c == nil {
		return TransactionDigestPage{}, fmt.Errorf("failed to get sui transaction digests: rpc_client=null")
	}
	if c.caller == nil {
		return TransactionDigestPage{}, fmt.Errorf("failed to get sui transaction digests: transaction_provider=null")
	}
	if err := transactionQuery.Validate(); err != nil {
		return TransactionDigestPage{}, fmt.Errorf("failed to get sui transaction digests: %w", err)
	}
	if ctx == nil {
		ctx = context.Background()
	}
	first := transactionQuery.First
	if first == 0 {
		first = 50
	}
	arguments := []string{fmt.Sprintf("first: %d", first)}
	if transactionQuery.After != "" {
		arguments = append(arguments, fmt.Sprintf("after: %q", transactionQuery.After))
	}
	filter := []string{fmt.Sprintf("affectedAddress: %q", transactionQuery.AffectedAddress.String())}
	if transactionQuery.AfterCheckpoint != nil {
		filter = append(filter, fmt.Sprintf("afterCheckpoint: %d", *transactionQuery.AfterCheckpoint))
	}
	if transactionQuery.AtCheckpoint != nil {
		filter = append(filter, fmt.Sprintf("atCheckpoint: %d", *transactionQuery.AtCheckpoint))
	}
	if transactionQuery.BeforeCheckpoint != nil {
		filter = append(filter, fmt.Sprintf("beforeCheckpoint: %d", *transactionQuery.BeforeCheckpoint))
	}
	arguments = append(arguments, "filter: { "+strings.Join(filter, " ")+" }")
	query := "query { transactions(" + strings.Join(arguments, " ") + ") { nodes { digest } pageInfo { hasNextPage endCursor } } }"
	var result struct {
		Transactions struct {
			Nodes []struct {
				Digest string `json:"digest"`
			} `json:"nodes"`
			PageInfo struct {
				HasNextPage bool   `json:"hasNextPage"`
				EndCursor   string `json:"endCursor"`
			} `json:"pageInfo"`
		} `json:"transactions"`
	}
	if err := c.caller.query(ctx, query, &result); err != nil {
		return TransactionDigestPage{}, fmt.Errorf("failed to get sui transaction digests: %w", err)
	}
	digests := make([]TransactionDigest, 0, len(result.Transactions.Nodes))
	for _, node := range result.Transactions.Nodes {
		digest, err := ParseTransactionDigest(node.Digest)
		if err != nil {
			return TransactionDigestPage{}, fmt.Errorf("failed to get sui transaction digests: %w", err)
		}
		digests = append(digests, digest)
	}
	if result.Transactions.PageInfo.HasNextPage && result.Transactions.PageInfo.EndCursor == "" {
		return TransactionDigestPage{}, fmt.Errorf("failed to get sui transaction digests: next_cursor=empty")
	}
	return TransactionDigestPage{Digests: digests, HasNextPage: result.Transactions.PageInfo.HasNextPage, NextCursor: result.Transactions.PageInfo.EndCursor}, nil
}
