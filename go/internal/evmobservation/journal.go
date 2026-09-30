package evmobservation

import (
	"crypto/sha256"
	"encoding/binary"
	"maps"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
)

type logKey struct {
	block uint64
	hash  common.Hash
	index uint
}
type Evidence map[logKey][32]byte
type Journal struct {
	seen  Evidence
	queue []logKey
}

func fingerprint(log types.Log) [32]byte {
	data := append([]byte{}, log.Address[:]...)
	data = append(data, log.TxHash[:]...)
	data = binary.BigEndian.AppendUint64(data, uint64(log.TxIndex))
	data = binary.BigEndian.AppendUint64(data, uint64(len(log.Topics)))
	for _, topic := range log.Topics {
		data = append(data, topic[:]...)
	}
	data = append(data, log.Data...)
	return sha256.Sum256(data)
}

// Record remembers a successfully applied log, bounded to 4096 identities.
// Missing or evicted evidence prevents confirmation; it never implies application.
//
// Version:
//   - 2026-10-01: Added.
func (j *Journal) Record(log types.Log) {
	if log.Removed {
		return
	}
	key := logKey{log.BlockNumber, log.BlockHash, log.Index}
	if j.seen == nil {
		j.seen = make(Evidence)
	}
	if _, ok := j.seen[key]; !ok {
		if len(j.queue) == 4096 {
			delete(j.seen, j.queue[0])
			j.queue = j.queue[1:]
		}
		j.queue = append(j.queue, key)
	}
	j.seen[key] = fingerprint(log)
}

// Snapshot copies applied evidence for a detached background proof.
//
// Version:
//   - 2026-10-01: Added.
func (j *Journal) Snapshot() Evidence { return maps.Clone(j.seen) }

func (e Evidence) contains(log types.Log) bool {
	digest, ok := e[logKey{log.BlockNumber, log.BlockHash, log.Index}]
	return ok && digest == fingerprint(log)
}
