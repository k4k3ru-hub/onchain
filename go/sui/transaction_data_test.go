package sui

import (
	"bytes"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"math"
	"os"
	"strings"
	"testing"
)

type transactionVector struct {
	Name, BCS, SigningDigest, TransactionDigest, Signature string
}
type transactionVectorFile struct {
	SDK, Sender, ObjectDigest string
	Vectors                   []transactionVector
}

func loadTransactionVectors(t testing.TB) transactionVectorFile {
	t.Helper()
	data, err := os.ReadFile("testdata/transaction_vectors.json")
	if err != nil {
		t.Fatal(err)
	}
	var result transactionVectorFile
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatal(err)
	}
	return result
}
func transactionTestAddress(t testing.TB, value string) Address {
	t.Helper()
	result, err := ParseAddress(value)
	if err != nil {
		t.Fatal(err)
	}
	return result
}
func transactionTestBytes(t testing.TB, value string) []byte {
	t.Helper()
	result, err := base64.StdEncoding.DecodeString(value)
	if err != nil {
		t.Fatal(err)
	}
	return result
}
func transactionTestKey(t testing.TB) *KeyPair {
	t.Helper()
	// Public deterministic fixture seed, not an operational wallet.
	value := make([]byte, 33)
	for i := range 32 {
		value[i+1] = byte(i)
	}
	key, err := ParseKeyPairBase64(base64.StdEncoding.EncodeToString(value))
	if err != nil {
		t.Fatal(err)
	}
	return key
}
func transactionTestData(t testing.TB) TransactionData {
	t.Helper()
	vectors := loadTransactionVectors(t)
	sender := transactionTestAddress(t, vectors.Sender)
	digest, err := ParseObjectDigest(vectors.ObjectDigest)
	if err != nil {
		t.Fatal(err)
	}
	builder := NewProgrammableTransactionBuilder()
	amount, err := PureUint64(builder, 1_000_000)
	if err != nil {
		t.Fatal(err)
	}
	address, err := builder.Pure(transactionTestAddress(t, "0x77").Bytes())
	if err != nil {
		t.Fatal(err)
	}
	split, err := builder.SplitCoins(SplitCoins{Coin: Argument{Kind: ArgumentKindGas}, Amounts: []Argument{amount}})
	if err != nil {
		t.Fatal(err)
	}
	coin, err := NestedResult(split, 0)
	if err != nil {
		t.Fatal(err)
	}
	if err := builder.TransferObjects(TransferObjects{Objects: []Argument{coin}, Address: address}); err != nil {
		t.Fatal(err)
	}
	ptb, err := builder.Build()
	if err != nil {
		t.Fatal(err)
	}
	epoch := uint64(42)
	return TransactionData{Transaction: ptb, Sender: sender, ExpirationEpoch: &epoch, GasData: GasData{
		Owner: sender, Price: 1000, Budget: 10_000_000,
		Payment: []ObjectReference{{Address: transactionTestAddress(t, "0xa1"), Version: 9007199254740993, Digest: digest}, {Address: transactionTestAddress(t, "0xa2"), Version: 99, Digest: digest}},
	}}
}

// TestTransactionDataMatchesOfficialSDK verifies independently generated wire and signature fixtures.
//
// Version:
//   - 2026-09-24: Added.
func TestTransactionDataMatchesOfficialSDK(t *testing.T) {
	vectors := loadTransactionVectors(t)
	key := transactionTestKey(t)
	constructed := transactionTestData(t)
	encoded, err := constructed.MarshalBCS()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(encoded, transactionTestBytes(t, vectors.Vectors[0].BCS)) {
		t.Fatal("builder encoding differs from official SDK")
	}
	for _, vector := range vectors.Vectors {
		t.Run(vector.Name, func(t *testing.T) {
			raw := transactionTestBytes(t, vector.BCS)
			transaction, err := ParseTransactionData(raw)
			if err != nil {
				t.Fatal(err)
			}
			encoded, err := transaction.MarshalBCS()
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(encoded, raw) {
				t.Fatal("BCS round trip differs from official SDK")
			}
			if transaction.GasData.Payment[0].Version != 9007199254740993 {
				t.Fatal("u64 precision lost")
			}
			signing, err := transaction.SigningDigest()
			if err != nil {
				t.Fatal(err)
			}
			if hex.EncodeToString(signing[:]) != vector.SigningDigest {
				t.Fatal("signing digest differs from official SDK")
			}
			digest, err := transaction.Digest()
			if err != nil {
				t.Fatal(err)
			}
			if digest.String() != vector.TransactionDigest {
				t.Fatal("transaction digest differs from official SDK")
			}
			signature, err := key.SignTransaction(raw)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(signature, transactionTestBytes(t, vector.Signature)) {
				t.Fatal("signature differs from official SDK")
			}
			if err := VerifyTransactionSignature(raw, signature, transaction.Sender); err != nil {
				t.Fatal(err)
			}
		})
	}
	complex, err := ParseTransactionData(transactionTestBytes(t, vectors.Vectors[1].BCS))
	if err != nil {
		t.Fatal(err)
	}
	if complex.ExpirationEpoch != nil || complex.Transaction.Inputs[3].Object.Version != math.MaxUint64 || complex.Transaction.Inputs[4].Pure == nil {
		t.Fatal("none expiration, receiving version or empty pure input was lost")
	}
	if len(complex.Transaction.Commands[1].MoveCall.TypeArguments) != 11 || complex.Transaction.Commands[2].Kind != CommandKindMakeMoveVec {
		t.Fatal("complex commands were not decoded")
	}
}

// TestTransactionDataRejectsMalformedBCS verifies canonical encoding and bounded decoding.
//
// Version:
//   - 2026-09-24: Added.
func TestTransactionDataRejectsMalformedBCS(t *testing.T) {
	raw := transactionTestBytes(t, loadTransactionVectors(t).Vectors[0].BCS)
	for i := 0; i < len(raw); i++ {
		if _, err := ParseTransactionData(raw[:i]); err == nil {
			t.Fatalf("accepted truncation at %d", i)
		}
	}
	for name, data := range map[string][]byte{
		"trailing":             append(append([]byte{}, raw...), 0),
		"noncanonical version": append([]byte{0x80, 0}, raw[1:]...),
		"unknown version":      append([]byte{1}, raw[1:]...),
		"unknown kind":         append([]byte{0, 1}, raw[2:]...),
		"length overflow":      {0, 0, 255, 255, 255, 255, 127},
		"allocation attack":    {0, 0, 255, 255, 255, 127},
		"unknown expiration":   append(append([]byte{}, raw[:len(raw)-9]...), 2),
		"oversized":            make([]byte, maxTransactionDataSize+1),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := ParseTransactionData(data); err == nil {
				t.Fatal("accepted malformed BCS")
			}
		})
	}
}

// TestTransactionDataRejectsConflictingFunds verifies explicit gas and input invariants.
//
// Version:
//   - 2026-09-24: Added.
func TestTransactionDataRejectsConflictingFunds(t *testing.T) {
	for name, mutate := range map[string]func(*TransactionData){
		"empty gas":     func(v *TransactionData) { v.GasData.Payment = nil },
		"duplicate gas": func(v *TransactionData) { v.GasData.Payment[1] = v.GasData.Payment[0] },
		"input gas overlap": func(v *TransactionData) {
			r := v.GasData.Payment[0]
			v.Transaction.Inputs = append(v.Transaction.Inputs, ProgrammableTransactionInput{Kind: InputKindImmutableOrOwned, Object: ObjectInput{Address: r.Address, Version: r.Version, Digest: r.Digest}})
		},
		"empty budget":      func(v *TransactionData) { v.GasData.Budget = 0 },
		"ambiguous command": func(v *TransactionData) { v.Transaction.Commands[0].MoveCall = &MoveCall{} },
		"invalid argument":  func(v *TransactionData) { v.Transaction.Commands[0].SplitCoins.Amounts[0].Index = 100 },
		"future result": func(v *TransactionData) {
			v.Transaction.Commands[0].SplitCoins.Coin = Argument{Kind: ArgumentKindResult, Index: 1}
		},
	} {
		t.Run(name, func(t *testing.T) {
			value := transactionTestData(t)
			mutate(&value)
			if _, err := value.MarshalBCS(); err == nil {
				t.Fatal("accepted invalid transaction")
			}
		})
	}
}

// TestTransactionSignatureRejectsTampering verifies identity and payload binding.
//
// Version:
//   - 2026-09-24: Added.
func TestTransactionSignatureRejectsTampering(t *testing.T) {
	data := transactionTestData(t)
	raw, err := data.MarshalBCS()
	if err != nil {
		t.Fatal(err)
	}
	key := transactionTestKey(t)
	signature, err := key.SignTransaction(raw)
	if err != nil {
		t.Fatal(err)
	}
	data.GasData.Budget++
	changed, err := data.MarshalBCS()
	if err != nil {
		t.Fatal(err)
	}
	if err := VerifyTransactionSignature(changed, signature, data.Sender); err == nil {
		t.Fatal("accepted changed budget")
	}
	if err := VerifyTransactionSignature(raw, signature, transactionTestAddress(t, "0x999")); err == nil {
		t.Fatal("accepted wrong signer")
	}
	for _, index := range []int{0, 1, 65} {
		changedSignature := append([]byte{}, signature...)
		changedSignature[index] ^= 1
		if err := VerifyTransactionSignature(raw, changedSignature, data.Sender); err == nil {
			t.Fatalf("accepted signature mutation at %d", index)
		}
	}
	data.Sender, data.GasData.Owner = transactionTestAddress(t, "0x1"), transactionTestAddress(t, "0x1")
	changed, err = data.MarshalBCS()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := key.SignTransaction(changed); err == nil {
		t.Fatal("signed unrelated transaction")
	}
	if _, err := (*KeyPair)(nil).SignTransaction(raw); err == nil {
		t.Fatal("nil key was accepted")
	}
	secret := "invalid-confidential-value"
	if _, err := ParseKeyPairBase64(secret); err == nil || strings.Contains(err.Error(), secret) {
		t.Fatal("invalid private key was accepted or exposed")
	}
}

// FuzzParseTransactionData checks that supported BCS has one canonical encoding.
//
// Version:
//   - 2026-09-24: Added.
func FuzzParseTransactionData(f *testing.F) {
	for _, vector := range loadTransactionVectors(f).Vectors {
		f.Add(transactionTestBytes(f, vector.BCS))
	}
	f.Add([]byte{0, 0, 255, 255, 255, 255, 127})
	f.Fuzz(func(t *testing.T, data []byte) {
		value, err := ParseTransactionData(data)
		if err != nil {
			return
		}
		encoded, err := value.MarshalBCS()
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(encoded, data) {
			t.Fatal("accepted noncanonical BCS")
		}
	})
}
