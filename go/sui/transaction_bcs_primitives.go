package sui

import (
	"encoding/binary"
	"fmt"
	"unicode/utf8"
)

// Local decoding bounds, not claims about a network's protocol limits.
const maxTransactionDataSize = 1 << 20
const maxTransactionElements = 1 << 16
const maxMoveTypeDepth = 64

type transactionBCSWriter struct {
	data []byte
	err  error
}

func (w *transactionBCSWriter) raw(value []byte) {
	if w.err != nil {
		return
	}
	if len(value) > maxTransactionDataSize-len(w.data) {
		w.err = fmt.Errorf("failed to encode sui transaction bcs: data=too_long max_length=%d", maxTransactionDataSize)
		return
	}
	w.data = append(w.data, value...)
}

func (w *transactionBCSWriter) uleb(value uint32) {
	for value >= 128 {
		w.raw([]byte{byte(value) | 128})
		value >>= 7
	}
	w.raw([]byte{byte(value)})
}

func (w *transactionBCSWriter) u16(value uint16) {
	var raw [2]byte
	binary.LittleEndian.PutUint16(raw[:], value)
	w.raw(raw[:])
}
func (w *transactionBCSWriter) u64(value uint64) {
	var raw [8]byte
	binary.LittleEndian.PutUint64(raw[:], value)
	w.raw(raw[:])
}
func (w *transactionBCSWriter) bytes(value []byte) {
	w.uleb(uint32(len(value)))
	w.raw(value)
}
func (w *transactionBCSWriter) text(value string)     { w.bytes([]byte(value)) }
func (w *transactionBCSWriter) address(value Address) { w.raw(value[:]) }
func (w *transactionBCSWriter) object(value ObjectReference) {
	w.address(value.Address)
	w.u64(value.Version)
	w.bytes(value.Digest[:])
}

type transactionBCSReader struct {
	data     []byte
	position int
	err      error
}

func (r *transactionBCSReader) fail(reason string) {
	if r.err == nil {
		r.err = fmt.Errorf("failed to decode sui transaction bcs: %s: byte_offset=%d", reason, r.position)
	}
}

func (r *transactionBCSReader) read(length int) []byte {
	if r.err != nil {
		return nil
	}
	if length < 0 || length > len(r.data)-r.position {
		r.fail("data=too_short")
		return nil
	}
	value := r.data[r.position : r.position+length]
	r.position += length
	return value
}

func (r *transactionBCSReader) byte() byte {
	value := r.read(1)
	if value == nil {
		return 0
	}
	return value[0]
}

func (r *transactionBCSReader) uleb() uint32 {
	var value uint32
	for i := 0; i < 5; i++ {
		b := r.byte()
		if r.err != nil {
			return 0
		}
		if i == 4 && b > 15 {
			r.fail("uleb128=out_of_range")
			return 0
		}
		value |= uint32(b&127) << (7 * i)
		if b&128 == 0 {
			if i > 0 && b == 0 {
				r.fail("uleb128=non_canonical")
				return 0
			}
			return value
		}
	}
	r.fail("uleb128=invalid")
	return 0
}

func (r *transactionBCSReader) length(limit int) int {
	value := uint64(r.uleb())
	if value > uint64(limit) || value > uint64(len(r.data)-r.position) {
		r.fail("length=out_of_range")
		return 0
	}
	return int(value)
}

func (r *transactionBCSReader) u16() uint16 {
	value := r.read(2)
	if value == nil {
		return 0
	}
	return binary.LittleEndian.Uint16(value)
}
func (r *transactionBCSReader) u64() uint64 {
	value := r.read(8)
	if value == nil {
		return 0
	}
	return binary.LittleEndian.Uint64(value)
}
func (r *transactionBCSReader) bytes() []byte {
	value := r.read(r.length(maxTransactionDataSize))
	if r.err != nil {
		return nil
	}
	return append([]byte{}, value...)
}
func (r *transactionBCSReader) text() string {
	value := r.bytes()
	if !utf8.Valid(value) {
		r.fail("string=invalid")
	}
	return string(value)
}
func (r *transactionBCSReader) address() Address {
	var value Address
	copy(value[:], r.read(32))
	return value
}
func (r *transactionBCSReader) object() ObjectReference {
	value := ObjectReference{Address: r.address(), Version: r.u64()}
	digest := r.bytes()
	if len(digest) != 32 {
		r.fail("object_digest=invalid")
	}
	copy(value.Digest[:], digest)
	return value
}
