// Package lpprotection measures principal protection at one EVM block.
// It recognizes reviewed custody code, not locker names or third-party verdicts.
package lpprotection

import (
	"context"
	"errors"
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/k4k3ru-hub/onchain/go/evm/clliquidity"
)

const ModelVersion = "lp-protection-20260923-v4"

// RPC methods must make at most one outbound attempt each. Retry, transport
// cache and shared endpoint budgets belong to the composition boundary.
// ethclient.Client satisfies this interface without a library adapter.
type RPC interface {
	ChainID(context.Context) (*big.Int, error)
	HeaderByNumber(context.Context, *big.Int) (*types.Header, error)
	CallContractAtHash(context.Context, ethereum.CallMsg, common.Hash) ([]byte, error)
	CodeAtHash(context.Context, common.Address, common.Hash) ([]byte, error)
	FilterLogs(context.Context, ethereum.FilterQuery) ([]types.Log, error)
	TransactionReceipt(context.Context, common.Hash) (*types.Receipt, error)
}

// CreationRPC supplies the additional, single-attempt methods used by the
// optional creation rule. ethclient.Client implements both RPC interfaces.
type CreationRPC interface {
	TransactionByHash(context.Context, common.Hash) (*types.Transaction, bool, error)
	NonceAtHash(context.Context, common.Address, common.Hash) (uint64, error)
}

// CreationHintResolver supplies untrusted deployment candidates, never verdicts.
// Calls share Analyze's deadline; the application owns HTTP budgets and retries.
type CreationHintResolver interface {
	ResolveCreationHint(context.Context, uint64, common.Address) (CreationHint, error)
}

type Limits struct {
	Timeout                                 time.Duration
	MaxCalls, MaxReceipts                   int
	LogBlockRange                           uint64
	MaxLogs, MaxPositions, MaxResponseBytes int
}

var ErrBudget = errors.New("lp protection acquisition budget exceeded")

var ErrReorg = errors.New("lp protection canonical block changed")

type Reader struct {
	rpc         RPC
	creationRPC CreationRPC
	resolver    CreationHintResolver
	limits      Limits
}

type Principal struct {
	// Pool binds the complete tick snapshot to its originating pool. The caller
	// must preserve this provenance when restoring or incrementally updating it.
	// For V4, Pool is the PoolManager and PoolID is the full 32-byte Pool ID.
	Pool     common.Address
	PoolID   common.Hash
	Snapshot clliquidity.Snapshot
}

type Request struct {
	ChainID   uint64
	Protocol  clliquidity.Protocol
	Creation  types.Log
	Principal Principal
	// Previously discovered IDs are hints, never a completeness assertion.
	PositionIDs []*big.Int
	// Received Mint logs and receipts avoid fetching the same data again.
	MintLogs []types.Log
	// Received V4 ModifyLiquidity logs preserve Pool ID and token ID salt.
	ModifyLiquidityLogs []types.Log
	Receipts            map[common.Hash]*types.Receipt
	// Deployment hints are lookup candidates, never proof of a first creation.
	CreationHints []CreationHint
	// Evidence must originate from Analyze or trusted internal persistence.
	Evidence *Evidence
}

type TokenProtection struct {
	LockedPercentage, PermanentlyProtectedPercentage *string
}

type Observation struct {
	Token0, Token1        TokenProtection
	AllPositionsProtected bool
	EarliestUnlockAt      *time.Time
	CanWeakenProtection   *bool
}

type Position struct {
	ID              *big.Int
	Owner, Approved common.Address
	Lower, Upper    int32
	Liquidity       *big.Int
	CodeHash        common.Hash
	Model, Reason   string
	// Kind is withdrawable or locked; empty means unresolved.
	Kind                string
	UnlockAt            *time.Time
	CanWeakenProtection *bool
}

type Metrics struct {
	Calls, AdditionalReceipts, CacheHits, ResponseBytes int
	Methods                                             map[string]int
}

type ContractEvidence struct {
	Address  common.Address
	CodeHash common.Hash
}

type Result struct {
	ModelVersion  string
	Pool, Manager common.Address
	PoolID        common.Hash
	BlockNumber   uint64
	BlockHash     common.Hash
	BlockTime     time.Time
	Observation   *Observation
	Reason        string
	Positions     []Position
	Contracts     []ContractEvidence
	// Retain these IDs even if custody is unresolved; revalidate their state on
	// the next block instead of reacquiring historical receipts.
	PositionIDs []*big.Int
	Metrics     Metrics
	Evidence    *Evidence
}
