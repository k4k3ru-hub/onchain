package v4

import (
	"bytes"
	"slices"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
)

type retainedLogKey struct {
	block uint64
	index uint
}
type retainedLogOrder struct {
	seen      map[retainedLogKey]types.Log
	queue     []retainedLogKey
	latest    types.Log
	hasLatest bool
}

// accept remembers a bounded set of full log contents, independently of the
// asynchronously captured quote baseline. Old unrecognized logs are not guessed
// to be duplicates. Removed/hash-conflicting blocks require a new stream epoch.
func (o *retainedLogOrder) accept(log types.Log) (bool, error) {
	if log.Removed {
		*o = retainedLogOrder{}
		return false, retainedReinitializationError(log, "removed log")
	}
	if log.BlockHash == (common.Hash{}) {
		return false, retainedReinitializationError(log, "missing block hash")
	}
	if len(log.Topics) == 0 {
		return false, retainedReinitializationError(log, "missing event topics")
	}
	key := retainedLogKey{log.BlockNumber, log.Index}
	if previous, ok := o.seen[key]; ok {
		if previous.BlockHash != log.BlockHash {
			*o = retainedLogOrder{}
			return false, retainedReinitializationError(log, "stream block hash mismatch")
		}
		if previous.Address == log.Address && previous.TxHash == log.TxHash && previous.TxIndex == log.TxIndex && slices.Equal(previous.Topics, log.Topics) && bytes.Equal(previous.Data, log.Data) {
			return true, nil
		}
		return false, retainedReinitializationError(log, "log content conflict")
	}
	if o.hasLatest {
		if log.BlockNumber == o.latest.BlockNumber && log.BlockHash != o.latest.BlockHash {
			*o = retainedLogOrder{}
			return false, retainedReinitializationError(log, "stream block hash mismatch")
		}
		if log.BlockNumber < o.latest.BlockNumber || (log.BlockNumber == o.latest.BlockNumber && log.Index < o.latest.Index) {
			return false, retainedReinitializationError(log, "out of order log")
		}
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
	o.latest, o.hasLatest = log, true
	return false, nil
}
