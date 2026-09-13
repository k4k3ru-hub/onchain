package clmm

import (
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"testing"

	solana "github.com/k4k3ru-hub/onchain/go/solana"
)

func TestDecodeSwapLogWithoutRPC(t *testing.T) {
	pool := Pool{Address: solana.Address{1}, Token0Mint: solana.Address{2}, Token1Mint: solana.Address{3}}
	s := &SwapSubscriber{pools: map[solana.Address]Pool{pool.Address: pool}}
	for _, reverse := range []bool{false, true} {
		b := make([]byte, 205)
		copy(b, []byte{64, 198, 205, 232, 38, 8, 113, 226})
		copy(b[8:40], pool.Address[:])
		if !reverse {
			b[168] = 1
			binary.LittleEndian.PutUint64(b[136:144], 100)
			binary.LittleEndian.PutUint64(b[152:160], 195)
			binary.LittleEndian.PutUint64(b[160:168], 5)
		} else {
			binary.LittleEndian.PutUint64(b[152:160], 100)
			binary.LittleEndian.PutUint64(b[136:144], 195)
			binary.LittleEndian.PutUint64(b[144:152], 5)
		}
		program := MainnetProgramAddress().String()
		log := &solana.Log{Signature: solana.Signature{1}, Slot: 1, Messages: []string{"Program " + program + " invoke [1]", "Program data: " + base64.StdEncoding.EncodeToString(b), "Program data: " + base64.StdEncoding.EncodeToString(b), "Program " + program + " success"}}
		events, err := s.DecodeSwapLog(pool.Address, solana.Address{}, log)
		if err != nil || len(events) != 2 {
			t.Fatal(events, err)
		}
		for i, e := range events {
			if e.AmountIn != 100 || e.AmountOut != 200 || e.EventIndex != uint32(i+1) {
				t.Fatal(e)
			}
			expected := pool.Token0Mint
			if reverse {
				expected = pool.Token1Mint
			}
			if e.InputMint != expected {
				t.Fatal(e)
			}
		}
		log.Failed = true
		events, err = s.DecodeSwapLog(pool.Address, solana.Address{}, log)
		if err != nil || len(events) != 0 {
			t.Fatal(events, err)
		}
		log.Failed = false
		log.Messages[1] = "Program data: " + base64.StdEncoding.EncodeToString(b[:45])
		if _, err = s.DecodeSwapLog(pool.Address, solana.Address{}, log); err == nil {
			t.Fatal("accepted truncated event")
		}
	}
}

// TestDecodeSwapLogTruncatedPrefix checks that truncation preserves only validated committed swaps.
//
// Version:
//   - 2026-09-13: Added.
func TestDecodeSwapLogTruncatedPrefix(t *testing.T) {
	pool := Pool{Address: solana.Address{1}, Token0Mint: solana.Address{2}, Token1Mint: solana.Address{3}}
	s := &SwapSubscriber{pools: map[solana.Address]Pool{pool.Address: pool}}
	p := MainnetProgramAddress().String()
	for _, size := range []int{205, 221} {
		t.Run(fmt.Sprint(size), func(t *testing.T) {
			payload := make([]byte, size)
			copy(payload, []byte{64, 198, 205, 232, 38, 8, 113, 226})
			copy(payload[8:40], pool.Address[:])
			payload[168] = 1
			binary.LittleEndian.PutUint64(payload[136:144], 100)
			binary.LittleEndian.PutUint64(payload[152:160], 195)
			binary.LittleEndian.PutUint64(payload[160:168], 5)
			data := "Program data: " + base64.StdEncoding.EncodeToString(payload)
			log := &solana.Log{Signature: solana.Signature{1}, Slot: 42, Messages: []string{
				"Program " + p + " invoke [1]", data, data, "Program " + p + " success",
				"Program " + p + " invoke [1]", data, "Log truncated", "Program " + p + " success",
			}}
			events, err := s.DecodeSwapLog(pool.Address, solana.Address{}, log)
			if !errors.Is(err, solana.ErrExecutionLogsTruncated) || len(events) != 2 {
				t.Fatalf("events = %v, error = %v", events, err)
			}
			for i, event := range events {
				if event.AmountIn != 100 || event.AmountOut != 200 || event.EventIndex != uint32(i+1) || event.Signature != log.Signature || event.Slot != log.Slot || event.Pool != pool.Address || event.InputMint != pool.Token0Mint || event.OutputMint != pool.Token1Mint {
					t.Fatalf("incorrect recovered swap: %+v", event)
				}
			}
			// A valid event must not hide a malformed matching event before the marker.
			log.Messages[2] = "Program data: " + base64.StdEncoding.EncodeToString(payload[:45])
			events, err = s.DecodeSwapLog(pool.Address, solana.Address{}, log)
			if err == nil || errors.Is(err, solana.ErrExecutionLogsTruncated) || len(events) != 0 {
				t.Fatalf("malformed event admitted: events = %v, error = %v", events, err)
			}
		})
	}
}
