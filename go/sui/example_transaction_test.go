package sui_test

import (
	"fmt"

	"github.com/k4k3ru-hub/onchain/go/sui"
)

// ExampleTransactionData demonstrates explicit gas-coin funding without network access.
//
// Version:
//   - 2026-09-24: Added.
func ExampleTransactionData() {
	if err := exampleTransactionData(); err != nil {
		fmt.Println(err)
	}
	// Output: complete transaction: sender and gas budget preserved
}

func exampleTransactionData() error {
	// Synthetic references demonstrate encoding only. Obtain live references from
	// RPCClient.Coins / Coin and reserve them before preparing an actual transaction.
	owner, err := sui.ParseAddress("0x1")
	if err != nil {
		return err
	}
	gasAddress, err := sui.ParseAddress("0x99")
	if err != nil {
		return err
	}
	var digest sui.ObjectDigest
	digest[0] = 1
	builder := sui.NewProgrammableTransactionBuilder()
	amount, err := sui.PureUint64(builder, 1_000_000)
	if err != nil {
		return err
	}
	recipient, err := builder.Pure(owner.Bytes())
	if err != nil {
		return err
	}
	split, err := builder.SplitCoins(sui.SplitCoins{Coin: sui.Argument{Kind: sui.ArgumentKindGas}, Amounts: []sui.Argument{amount}})
	if err != nil {
		return err
	}
	coin, err := sui.NestedResult(split, 0)
	if err != nil {
		return err
	}
	if err := builder.TransferObjects(sui.TransferObjects{Objects: []sui.Argument{coin}, Address: recipient}); err != nil {
		return err
	}
	ptb, err := builder.Build()
	if err != nil {
		return err
	}
	// Applications obtain the current epoch and apply an explicit expiration policy.
	epoch := uint64(42)
	tx := sui.TransactionData{Transaction: ptb, Sender: owner, ExpirationEpoch: &epoch, GasData: sui.GasData{
		Payment: []sui.ObjectReference{{Address: gasAddress, Version: 1, Digest: digest}}, Owner: owner, Price: 1000, Budget: 10_000_000,
	}}
	raw, err := tx.MarshalBCS()
	if err != nil {
		return err
	}
	decoded, err := sui.ParseTransactionData(raw)
	if err != nil {
		return err
	}
	if decoded.Sender != owner || decoded.GasData.Budget != tx.GasData.Budget {
		return fmt.Errorf("failed to verify example transaction: transaction=mismatch")
	}
	fmt.Println("complete transaction: sender and gas budget preserved")
	return nil
}
