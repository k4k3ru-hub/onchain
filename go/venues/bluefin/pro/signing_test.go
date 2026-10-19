package pro

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/k4k3ru-hub/onchain/go/sui"
)

// The payload follows fireflyprotocol/pro-sdk at 31943cfed2771647b247513596c367a748f5b058.
// Its independent signature and BCS hash were generated using @mysten/sui v2 and @mysten/bcs.
// The ephemeral signing key was not saved; only the public verification fixture is retained.
func officialOrder() OrderFields {
	return OrderFields{Symbol: "ETH-PERP", AccountAddress: "0x76a831f3a961579e5fd34b6cda412409dd41ba18fe0f10e149ecdad413af4050", PriceE9: "100000000000", QuantityE9: "100000000", Side: "LONG", LeverageE9: "0", Salt: "1739491998372", IDSID: "0xeb46bc9cd8cca03f9cabde46bf280a5aae264668877b7ee2f24cbaf0692089aa", ExpiresAtMS: 2037603360000, SignedAtMS: 1739492801736}
}

// TestOfficialTypeScriptOrderSignature verifies the named protocol behavior.
//
// Version:
//   - 2026-10-14: Added.
func TestOfficialTypeScriptOrderSignature(t *testing.T) {
	f := officialOrder()
	message, err := f.SigningBytes()
	if err != nil {
		t.Fatal(err)
	}
	encoded := "ADYs4BY/XrreDZ3O+trGwdzJSGqAiam8bnulVoivA4/HSN8ceCdf6KNSDJEjXSIa+kGCIomJgpfJK+QVZYIiOQo5pRDqaVKFSd/6xq7smEpcBjXoApUD2Q4xJDyjdCKAsA=="
	fixtureSigner := "0x2d50ce622721f84c0927fc6c87bbbfbc21844e9789fed91347aa896cc038199d"
	if err := VerifySignature(message, encoded, fixtureSigner); err != nil {
		t.Fatalf("official TS signature rejected: %v", err)
	}
	f.QuantityE9 = "200000000"
	changed, err := f.SigningBytes()
	if err != nil {
		t.Fatal(err)
	}
	if err := VerifySignature(changed, encoded, fixtureSigner); err == nil {
		t.Fatal("changed order accepted")
	}
}

// TestLoginCanonicalBytesAndSigner verifies the named protocol behavior.
//
// Version:
//   - 2026-10-14: Added.
func TestLoginCanonicalBytesAndSigner(t *testing.T) {
	key, err := sui.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	address, err := key.Address()
	if err != nil {
		t.Fatal(err)
	}
	p := LoginRequest{address.String(), 1791907200000, "api"}
	message, err := p.SigningBytes()
	if err != nil {
		t.Fatal(err)
	}
	expected := `{"accountAddress":"` + address.String() + `","signedAtMillis":1791907200000,"audience":"api"}`
	if string(message) != expected {
		t.Fatal("login field order or whitespace changed")
	}
	signature, err := SignMessage(context.Background(), key, message)
	if err != nil {
		t.Fatal(err)
	}
	if err := VerifySignature(message, signature, address.String()); err != nil {
		t.Fatal(err)
	}
	pretty, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := VerifySignature(pretty, signature, address.String()); err == nil {
		t.Fatal("changed serialization accepted")
	}
}

// TestExactDecimalBoundaries verifies the named protocol behavior.
//
// Version:
//   - 2026-10-14: Added.
func TestExactDecimalBoundaries(t *testing.T) {
	for value, want := range map[string]string{"0": "0", "1": "1000000000", "0.000000001": "1", "18446744073.709551615": "18446744073709551615"} {
		got, err := DecimalToE9(value)
		if err != nil || got != want {
			t.Fatalf("convert %q: %q %v", value, got, err)
		}
	}
	for _, value := range []string{"1e9", "-1", "0.0000000001", "18446744073.709551616", "1.", " 1"} {
		if _, err := DecimalToE9(value); err == nil {
			t.Fatalf("invalid value accepted: %q", value)
		}
	}
	for value, want := range map[string]string{"0": "0", "1000000000": "1", "1": "0.000000001", "-1500000": "-0.0015"} {
		got, err := FormatE9(value)
		if err != nil || got != want {
			t.Fatalf("format %q: %q %v", value, got, err)
		}
	}
	f := officialOrder()
	hash, err := f.OrderHash()
	if err != nil || len(hash) != 64 {
		t.Fatalf("hash: %q %v", hash, err)
	}
	if hash != "639d7caa7b7d056717845acd2054935137ef3e2dde084d36b0c9efec0715e1dc" {
		t.Fatal("order hash differs from the independent official BCS implementation")
	}
	f.Salt = "1739491998373"
	other, err := f.OrderHash()
	if err != nil {
		t.Fatal(err)
	}
	if strings.EqualFold(hash, other) {
		t.Fatal("salt did not bind order hash")
	}
}
