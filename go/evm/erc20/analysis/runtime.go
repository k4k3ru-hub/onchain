package analysis

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
)

func matchRuntime(artifact runtimeArtifact, actual []byte, allowedImmutables map[string]bool) bool {
	compiled, err := hex.DecodeString(artifact.Object)
	if err != nil || len(artifact.LinkReferences) != 0 {
		return false
	}
	compiledBody, compiledMetadata := metadataBody(compiled)
	actualBody, actualMetadata := metadataBody(actual)
	if !compiledMetadata || !actualMetadata {
		compiledBody, actualBody = compiled, actual
	}
	if len(compiledBody) != len(actualBody) {
		return false
	}
	covered := make([]bool, len(compiledBody))
	for identifier, ranges := range artifact.ImmutableReferences {
		if !allowedImmutables[identifier] || len(ranges) == 0 {
			return false
		}
		var value []byte
		for _, span := range ranges {
			if span.Start < 0 || span.Length != 32 || span.Start > len(compiledBody)-32 {
				return false
			}
			actualValue := actualBody[span.Start : span.Start+32]
			if value != nil && !bytes.Equal(value, actualValue) {
				return false
			}
			value = actualValue
			for i := span.Start; i < span.Start+32; i++ {
				if covered[i] {
					return false
				}
				covered[i] = true
			}
			copy(compiledBody[span.Start:span.Start+32], actualValue)
		}
	}
	if !bytes.Equal(compiledBody, actualBody) {
		return false
	}
	if bytes.Equal(compiled, actual) {
		return true
	}
	// Metadata relaxation is limited to recognized standard models whose
	// executable body does not inspect code. PUSH immediate bytes are skipped.
	for i := 0; i < len(compiledBody); i++ {
		op := compiledBody[i]
		if op == 0x38 || op == 0x39 || op == 0x3b || op == 0x3c || op == 0x3f {
			return false
		}
		if op >= 0x60 && op <= 0x7f {
			i += int(op - 0x5f)
			if i >= len(compiledBody) {
				return false
			}
		}
	}
	return compiledMetadata && actualMetadata
}

// metadataBody accepts only complete, definite Solidity metadata maps with
// known byte-string entries and a compiler version. Unknown CBOR stays intact.
func metadataBody(code []byte) ([]byte, bool) {
	if len(code) < 3 {
		return code, false
	}
	n := int(binary.BigEndian.Uint16(code[len(code)-2:]))
	if n < 1 || n+2 >= len(code) {
		return code, false
	}
	start := len(code) - n - 2
	data := code[start : len(code)-2]
	if data[0] < 0xa1 || data[0] > 0xa4 {
		return code, false
	}
	count, offset := int(data[0]-0xa0), 1
	seen := make(map[string]bool)
	for range count {
		if offset >= len(data) || data[offset] < 0x61 || data[offset] > 0x77 {
			return code, false
		}
		length := int(data[offset] - 0x60)
		offset++
		if length > len(data)-offset {
			return code, false
		}
		key := string(data[offset : offset+length])
		offset += length
		if seen[key] || offset >= len(data) {
			return code, false
		}
		seen[key] = true
		expected := map[string]int{"ipfs": 34, "bzzr0": 32, "bzzr1": 32, "solc": 3}[key]
		if expected == 0 {
			return code, false
		}
		marker := data[offset]
		offset++
		if marker == 0x58 {
			if offset >= len(data) {
				return code, false
			}
			length = int(data[offset])
			offset++
		} else if marker >= 0x40 && marker <= 0x57 {
			length = int(marker - 0x40)
		} else {
			return code, false
		}
		if length != expected || length > len(data)-offset {
			return code, false
		}
		offset += length
	}
	if offset != len(data) || !seen["solc"] {
		return code, false
	}
	return code[:start], true
}
