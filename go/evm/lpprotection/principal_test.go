package lpprotection

import (
	"math/big"
	"testing"
	"time"
)

// TestProtectionRatios verifies token-specific denominators and tiny open positions.
//
// Version:
//   - 2026-09-23: Added.
func TestProtectionRatios(t *testing.T) {
	price := new(big.Int).Lsh(big.NewInt(1), 96)
	until := time.Unix(1800000000, 0)
	pos := func(lower, upper int32, liquidity int64, kind string) Position {
		return Position{Lower: lower, Upper: upper, Liquidity: big.NewInt(liquidity), Kind: kind, UnlockAt: &until, CanWeakenProtection: flag(false)}
	}
	t.Run("independent denominators", func(t *testing.T) {
		p := []Position{pos(60, 120, 9, "locked"), pos(60, 120, 1, "withdrawable"), pos(-120, -60, 1, "locked"), pos(-120, -60, 9, "withdrawable")}
		v, reason, err := aggregate(price, p)
		if err != nil || reason != "" || v == nil || *v.Token0.LockedPercentage != "90" || *v.Token1.LockedPercentage != "10" || v.AllPositionsProtected {
			t.Fatalf("%+v %s %v", v, reason, err)
		}
	})
	t.Run("tiny unprotected principal", func(t *testing.T) {
		locked := pos(-60, 60, 1, "locked")
		locked.Liquidity = new(big.Int).Lsh(big.NewInt(1), 127)
		v, reason, err := aggregate(price, []Position{locked, pos(-60, 60, 1, "withdrawable")})
		if err != nil || reason != "" || v.AllPositionsProtected || *v.Token0.LockedPercentage != "99.999999999999999999" {
			t.Fatalf("%+v %s %v", v, reason, err)
		}
	})
	t.Run("zero token denominator", func(t *testing.T) {
		v, _, err := aggregate(price, []Position{pos(60, 120, 1, "locked")})
		if err != nil || v.Token1.LockedPercentage != nil || v.Token1.PermanentlyProtectedPercentage != nil || !v.AllPositionsProtected {
			t.Fatalf("%+v %v", v, err)
		}
	})
	t.Run("unknown discards entire observation", func(t *testing.T) {
		v, reason, err := aggregate(price, []Position{pos(-60, 60, 100, "locked"), pos(-60, 60, 1, "")})
		if err != nil || v != nil || reason != "unsupported_custody" {
			t.Fatalf("%+v %s %v", v, reason, err)
		}
	})
	t.Run("no principal", func(t *testing.T) {
		v, reason, err := aggregate(price, nil)
		if err != nil || v != nil || reason != "no_principal" {
			t.Fatalf("%+v %s %v", v, reason, err)
		}
	})
}

// TestRuntimeMatching verifies immutable masking cannot hide other bytecode changes.
//
// Version:
//   - 2026-09-23: Added.
func TestRuntimeMatching(t *testing.T) {
	for _, name := range []string{"vault", "clLocker", "clFactory"} {
		t.Run(name, func(t *testing.T) {
			code := runtime(t, name)
			if _, ok := match(code, name); !ok {
				t.Fatal("reviewed runtime not recognized")
			}
			code[100] ^= 1
			if _, ok := match(code, name); ok {
				t.Fatal("modified code recognized")
			}
			code[100] ^= 1
			for _, sites := range template(name).fields {
				if len(sites) > 1 {
					code[sites[0]+31] ^= 1
					if _, ok := match(code, name); ok {
						t.Fatal("inconsistent immutable recognized")
					}
					return
				}
			}
			t.Fatal("fixture missing repeated immutable")
		})
	}
}
