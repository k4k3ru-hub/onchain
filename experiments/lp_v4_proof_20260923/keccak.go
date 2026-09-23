//go:build ignore

package main

import (
	"bufio"
	"encoding/hex"
	"fmt"
	"log"
	"os"

	"github.com/ethereum/go-ethereum/crypto"
)

func main() {
	if err := run(); err != nil {
		log.Print(err)
		os.Exit(1)
	}
}

func run() error {
	s := bufio.NewScanner(os.Stdin)
	for s.Scan() {
		b, err := hex.DecodeString(s.Text())
		if err != nil {
			return fmt.Errorf("failed to decode keccak input: %w", err)
		}
		if _, err := fmt.Printf("%x\n", crypto.Keccak256(b)); err != nil {
			return fmt.Errorf("failed to write keccak output: %w", err)
		}
	}
	if err := s.Err(); err != nil {
		return fmt.Errorf("failed to read keccak input: %w", err)
	}
	return nil
}
