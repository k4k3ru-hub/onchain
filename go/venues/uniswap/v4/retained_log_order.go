package v4

import (
	"bytes"
	"slices"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
)

type retainedLogKey struct {
	block   uint64
	index   uint
	hash    common.Hash
	removed bool
}
type retainedLogOrder struct {
	seen  map[retainedLogKey]types.Log
	queue []retainedLogKey
}

// accept de-duplicates exact notifications without imposing ledger order on
// live observations. Cancellation and replacement notifications have distinct keys.
func (o *retainedLogOrder) accept(log types.Log) (bool, error) {
	if log.BlockHash == (common.Hash{}) {
		return false, retainedReinitializationError(log, "missing block hash")
	}
	if len(log.Topics) == 0 {
		return false, retainedReinitializationError(log, "missing event topics")
	}
	key := retainedLogKey{block: log.BlockNumber, index: log.Index, hash: log.BlockHash, removed: log.Removed}
	if previous, ok := o.seen[key]; ok {
		if previous.Address == log.Address && previous.TxHash == log.TxHash && previous.TxIndex == log.TxIndex && slices.Equal(previous.Topics, log.Topics) && bytes.Equal(previous.Data, log.Data) {
			return true, nil
		}
		return false, retainedReinitializationError(log, "log content conflict")
	}
	if o.seen == nil {
		o.seen = make(map[retainedLogKey]types.Log)
	}
	if len(o.queue) == retainedReplayLimit {
		delete(o.seen, o.queue[0])
		o.queue = o.queue[1:]
	}
	log.Topics = slices.Clone(log.Topics)
	log.Data = bytes.Clone(log.Data)
	o.seen[key] = log
	o.queue = append(o.queue, key)
	return false, nil
}
