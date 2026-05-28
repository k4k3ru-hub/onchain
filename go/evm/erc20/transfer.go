//
// transfer.go
//
package erc20

import (
    "context"
    "fmt"
    "math/big"
    "sync"

    "github.com/ethereum/go-ethereum"
    "github.com/ethereum/go-ethereum/common"
    "github.com/ethereum/go-ethereum/core/types"
    "github.com/ethereum/go-ethereum/crypto"
)


var (
    transferEventSigHash = crypto.Keccak256Hash([]byte("Transfer(address,address,uint256)"))
)

type TransferWatchConfig struct {
    From          []common.Address
    To            []common.Address
    FromBlock     *big.Int
    ToBlock       *big.Int
    LogBufferSize int
}

type TransferEvent struct {
    from            common.Address
    to              common.Address
    amountBaseUnits *big.Int
    txHash          common.Hash
    blockNumber     uint64
    token           common.Address
}

type WatchTransferStopFunc func()

type TransferControl interface {
    Stop()
}

type TransferHandler func(event *TransferEvent, control TransferControl)

type transferControl struct {
    stop WatchTransferStopFunc
}

func (c *transferControl) Stop() {
    if c == nil || c.stop == nil {
        return
    }
    c.stop()
}


//
// Filter transfer logs.
//
// Version:
//   - 2026-05-22: Added.
//
func (c *Client) FilterTransferLogs(ctx context.Context, cfg *TransferWatchConfig) ([]*TransferEvent, error) {
    if c == nil {
        return nil, fmt.Errorf("failed to filter transfer logs: missing required parameter: client=null")
    }
    if ctx == nil {
        ctx = context.Background()
    }

    ec := c.HTTPETHClient()
    if ec == nil {
        return nil, fmt.Errorf("failed to filter transfer logs: missing required parameter: http_eth_client=null")
    }

    q, err := c.buildTransferFilterQuery(cfg)
    if err != nil {
        return nil, fmt.Errorf("failed to filter transfer logs: %w", err)
    }

    logs, err := ec.FilterLogs(ctx, q)
    if err != nil {
        return nil, fmt.Errorf("failed to filter transfer logs: %w", err)
    }

    events := make([]*TransferEvent, 0, len(logs))
    for _, eventLog := range logs {
        event, ok := parseTransferLog(eventLog)
        if !ok {
            continue
        }
        events = append(events, event)
    }

    return events, nil
}


//
// Watch transfer events.
//
// Return:
//   - WatchTransferStopFunc: Stop watching transfer events.
//   - func(): Stop watching transfer events.
//   - error: Failed to start watching transfer events.
//
// Version:
//   - 2026-05-27: Changed to pass transfer control to handler.
//   - 2026-05-26: Changed to return stop function.
//   - 2026-05-22: Added.
//
func (c *Client) WatchTransfer(ctx context.Context, cfg *TransferWatchConfig, handler TransferHandler) (WatchTransferStopFunc, error) {
    if c == nil {
        return nil, fmt.Errorf("failed to watch transfer:  missing required parameter: client=null")
    }
    if handler == nil {
        return nil, fmt.Errorf("failed to watch transfer: missing required parameter: handler=null")
    }
    if ctx == nil {
        ctx = context.Background()
    }

    ec := c.WSETHClient()
    if ec == nil {
        return nil, fmt.Errorf("failed to watch transfer: missing required parameter: ws_eth_client=null")
    }

    q, err := c.buildTransferFilterQuery(cfg)
    if err != nil {
        return nil, fmt.Errorf("failed to watch transfer: %w", err)
    }

    logBufferSize := 64
    if cfg != nil && cfg.LogBufferSize > 0 {
        logBufferSize = cfg.LogBufferSize
    }

    logsCh := make(chan types.Log, logBufferSize)

    watchCtx, cancel := context.WithCancel(ctx)

    sub, err := ec.SubscribeFilterLogs(watchCtx, q, logsCh)
    if err != nil {
        return nil, fmt.Errorf("failed to watch transfer: failed to subscribe transfer logs: %w", err)
    }

    var stopOnce sync.Once
    stop := WatchTransferStopFunc(func() {
        stopOnce.Do(func() {
            cancel()
            sub.Unsubscribe()
        })
    })

    control := &transferControl{
        stop: stop,
    }
    
    go func() {
        defer stop()

        for {
            select {
            case <-watchCtx.Done():
                return
            case <-sub.Err():
                return
            case eventLog := <-logsCh:
                event, ok := parseTransferLog(eventLog)
                if !ok {
                    continue
                }
                handler(event, control)
            }
        }
    }()

    return stop, nil
}



func (e *TransferEvent) From() common.Address {
    if e == nil {
        return common.Address{}
    }
    return e.from
}

func (e *TransferEvent) FromHex() string {
    if e == nil {
        return ""
    }
    return e.from.Hex()
}

func (e *TransferEvent) To() common.Address {
    if e == nil {
        return common.Address{}
    }
    return e.to
}

func (e *TransferEvent) ToHex() string {
    if e == nil {
        return ""
    }
    return e.to.Hex()
}

func (e *TransferEvent) AmountBaseUnits() *big.Int {
    if e == nil || e.amountBaseUnits == nil {
        return nil
    }
    return new(big.Int).Set(e.amountBaseUnits)
}

func (e *TransferEvent) AmountBaseUnitsString() string {
    if e == nil || e.amountBaseUnits == nil {
        return ""
    }
    return e.amountBaseUnits.String()
}

func (e *TransferEvent) TxHash() common.Hash {
    if e == nil {
        return common.Hash{}
    }
    return e.txHash
}

func (e *TransferEvent) TxHashHex() string {
    if e == nil {
        return ""
    }
    return e.txHash.Hex()
}

func (e *TransferEvent) BlockNumber() uint64 {
    if e == nil {
        return 0
    }
    return e.blockNumber
}

func (e *TransferEvent) Token() common.Address {
    if e == nil {
        return common.Address{}
    }
    return e.token
}

func (e *TransferEvent) TokenHex() string {
    if e == nil {
        return ""
    }
    return e.token.Hex()
}

//
// Build transfer filter query.
//
// Version:
//   - 2026-05-22: Added.
//
func (c *Client) buildTransferFilterQuery(cfg *TransferWatchConfig) (ethereum.FilterQuery, error) {
    if c == nil {
        return ethereum.FilterQuery{}, fmt.Errorf("missing required parameter: client=null")
    }

    tokens := c.Tokens()
    if len(tokens) == 0 {
        return ethereum.FilterQuery{}, fmt.Errorf("missing required parameter: tokens=%q", "empty")
    }

    q := ethereum.FilterQuery{
        Addresses: tokens,
        Topics: [][]common.Hash{
            {transferEventSigHash},
            nil,
            nil,
        },
    }

    if cfg == nil {
        return q, nil
    }

    if cfg.FromBlock != nil {
        q.FromBlock = cfg.FromBlock
    }
    if cfg.ToBlock != nil {
        q.ToBlock = cfg.ToBlock
    }

    fromTopics := buildAddressTopics(cfg.From)
    if len(fromTopics) > 0 {
        q.Topics[1] = fromTopics
    }

    toTopics := buildAddressTopics(cfg.To)
    if len(toTopics) > 0 {
        q.Topics[2] = toTopics
    }

    return q, nil
}


//
// Build address topics.
//
// Version:
//   - 2026-05-22: Added.
//
func buildAddressTopics(addrs []common.Address) []common.Hash {
    if len(addrs) == 0 {
        return nil
    }

    topics := make([]common.Hash, 0, len(addrs))
    for _, addr := range addrs {
        if addr == (common.Address{}) {
            continue
        }

        topics = append(topics, common.BytesToHash(addr.Bytes()))
    }

    return topics
}


//
// Parse transfer log.
//
// Version:
//   - 2026-05-22: Added.
//
func parseTransferLog(eventLog types.Log) (*TransferEvent, bool) {
    if len(eventLog.Topics) < 3 {
        return nil, false
    }
    if eventLog.Topics[0] != transferEventSigHash {
        return nil, false
    }
    if len(eventLog.Data) != 32 {
        return nil, false
    }

    from := common.BytesToAddress(eventLog.Topics[1].Bytes()[12:])
    to := common.BytesToAddress(eventLog.Topics[2].Bytes()[12:])
    amountBaseUnits := new(big.Int).SetBytes(eventLog.Data)

    return &TransferEvent{
        from:            from,
        to:              to,
        amountBaseUnits: amountBaseUnits,
        txHash:          eventLog.TxHash,
        blockNumber:     eventLog.BlockNumber,
        token:           eventLog.Address,
    }, true
}
