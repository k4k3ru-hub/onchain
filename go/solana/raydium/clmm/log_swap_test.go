package clmm

import (
	"encoding/base64"
	"encoding/binary"
	solana "github.com/k4k3ru-hub/onchain/go/solana"
	"testing"
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
