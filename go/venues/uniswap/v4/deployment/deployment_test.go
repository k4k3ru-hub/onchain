package deployment

import (
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/common"
)

func TestByChainID(t *testing.T) {
	t.Parallel()

	tests := []struct {
		chainID     uint64
		poolManager string
		stateView   string
	}{
		{1, "0x000000000004444c5dc75cB358380D2e3dE08A90", "0x7ffe42c4a5deea5b0fec41c94c136cf115597227"},
		{10, "0x9a13f98cb987694c9f086b1f5eb990eea8264ec3", "0xc18a3169788f4f75a170290584eca6395c75ecdb"},
		{56, "0x28e2ea090877bf75740558f6bfb36a5ffee9e9df", "0xd13dd3d6e93f276fafc9db9e6bb47c1180aee0c4"},
		{130, "0x1F98400000000000000000000000000000000004", "0x86e8631a016f9068c3f085faf484ee3f5fdee8f2"},
		{137, "0x67366782805870060151383f4bbff9dab53e5cd6", "0x5ea1bd7974c8a611cbab0bdcafcb1d9cc9b3ba5a"},
		{143, "0x188d586ddcf52439676ca21a244753fa19f9ea8e", "0x77395f3b2e73ae90843717371294fa97cc419d64"},
		{196, "0x360e68faccca8ca495c1b759fd9eee466db9fb32", "0x76fd297e2d437cd7f76d50f01afe6160f86e9990"},
		{480, "0xb1860d529182ac3bc1f51fa2abd56662b7d13f33", "0x51d394718bc09297262e368c1a481217fdeb71eb"},
		{1301, "0x9cb26a7183b2f4515945dc52cb4195b0d2d06c95", "0x792d13207744f132943cdde4d37ec89f20ae3b0d"},
		{1868, "0x360e68faccca8ca495c1b759fd9eee466db9fb32", "0x76fd297e2d437cd7f76d50f01afe6160f86e9990"},
		{4217, "0x33620f62c5b9b2086dd6b62f4a297a9f30347029", "0x21b954fba3f5ddebe77ef2d47a3100c066908b2a"},
		{4326, "0xacb7e78fa05d562e0a5d3089ec896d57d057d38e", "0x726f84e1dfb8d375a365e0808282f40d52d3e4e8"},
		{4663, "0x8366a39cc670b4001a1121b8f6a443a643e40951", "0xf3334192d15450cdd385c8b70e03f9a6bd9e673b"},
		{5042, "0x8366a39cc670b4001a1121b8f6a443a643e40951", "0xf3334192d15450cdd385c8b70e03f9a6bd9e673b"},
		{8453, "0x498581ff718922c3f8e6a244956af099b2652b2b", "0xa3c0c9b65bad0b08107aa264b0f3db444b867a71"},
		{42161, "0x360e68faccca8ca495c1b759fd9eee466db9fb32", "0x76fd297e2d437cd7f76d50f01afe6160f86e9990"},
		{42220, "0x288dc841A52FCA2707c6947B3A777c5E56cd87BC", "0xbc21f8720babf4b20d195ee5c6e99c52b76f2bfb"},
		{57073, "0x360e68faccca8ca495c1b759fd9eee466db9fb32", "0x76fd297e2d437cd7f76d50f01afe6160f86e9990"},
		{59144, "0x248083fb965359d82b06c1f5322480dcfc1ad857", "0xe861de206e460a8b936b05ad3816520b58ccdf9b"},
		{81457, "0x1631559198a9e474033433b2958dabc135ab6446", "0x12a88ae16f46dce4e8b15368008ab3380885df30"},
		{11155111, "0xE03A1074c86CFeDd5C142C4F04F1a1536e203543", "0xE1Dd9c3fA50EDB962E442f60DfBc432e24537E4C"},
		{7777777, "0x0575338e4c17006ae181b47900a84404247CA30f", "0x385785af07d63b50d0a0ea57c4ff89d06adf7328"},
	}

	for _, test := range tests {
		test := test
		t.Run(test.poolManager, func(t *testing.T) {
			t.Parallel()

			got, err := ByChainID(test.chainID)
			if err != nil {
				t.Fatalf("ByChainID(%d) returned an error: %v", test.chainID, err)
			}
			if got.ChainID != test.chainID {
				t.Fatalf("ChainID = %d, want %d", got.ChainID, test.chainID)
			}
			wantPoolManager := common.HexToAddress(test.poolManager)
			if got.PoolManager != wantPoolManager {
				t.Fatalf("PoolManager = %s, want %s", got.PoolManager.Hex(), wantPoolManager.Hex())
			}
			wantStateView := common.HexToAddress(test.stateView)
			if got.StateView != wantStateView {
				t.Fatalf("StateView = %s, want %s", got.StateView.Hex(), wantStateView.Hex())
			}
		})
	}
}

func TestByChainIDRejectsUnsupportedChain(t *testing.T) {
	t.Parallel()

	got, err := ByChainID(0)
	if err == nil {
		t.Fatal("ByChainID(0) returned nil error")
	}
	if got != (Deployment{}) {
		t.Fatalf("deployment = %+v, want zero value", got)
	}
	if !strings.Contains(err.Error(), "chain_id=0") {
		t.Fatalf("error = %q, want chain_id context", err)
	}
}

func TestQuoterByChainID(t *testing.T) {
	t.Parallel()

	tests := []struct {
		chainID uint64
		quoter  string
	}{
		{1, "0x52f0e24d1c21c8a0cb1e5a5dd6198556bd9e1203"},
		{56, "0x9f75dd27d6664c475b90e105573e550ff69437b0"},
		{8453, "0x0d5e0f971ed27fbff6c2837bf31316121532048d"},
		{4663, "0x8dc178efb8111bb0973dd9d722ebeff267c98f94"},
	}
	for _, test := range tests {
		test := test
		t.Run(test.quoter, func(t *testing.T) {
			t.Parallel()
			got, err := QuoterByChainID(test.chainID)
			if err != nil {
				t.Fatalf("QuoterByChainID(%d) error = %v", test.chainID, err)
			}
			want := common.HexToAddress(test.quoter)
			if got == (common.Address{}) || got != want {
				t.Fatalf("quoter = %s, want %s", got.Hex(), want.Hex())
			}
			deployment, err := ByChainID(test.chainID)
			if err != nil {
				t.Fatalf("ByChainID(%d) error = %v", test.chainID, err)
			}
			if deployment.Quoter != want {
				t.Fatalf("Deployment.Quoter = %s, want %s", deployment.Quoter.Hex(), want.Hex())
			}
		})
	}
}

func TestQuoterByChainIDRejectsUnsupportedChain(t *testing.T) {
	t.Parallel()

	got, err := QuoterByChainID(0)
	if err == nil || got != (common.Address{}) || !strings.Contains(err.Error(), "chain_id=0") {
		t.Fatalf("QuoterByChainID(0) = (%s, %v), want zero address and chain error", got.Hex(), err)
	}
}

// TestDeploymentWithoutQuoter preserves discovery support when quoting is unavailable.
//
// Version:
//   - 2026-09-16: Added.
func TestDeploymentWithoutQuoter(t *testing.T) {
	d, err := ByChainID(10)
	if err != nil || d.Quoter != (common.Address{}) {
		t.Fatalf("deployment=%+v error=%v", d, err)
	}
	if _, err := QuoterByChainID(10); err == nil {
		t.Fatal("missing quoter accepted")
	}
}

// TestPositionManagerDeployment limits LP analysis to the reviewed Base deployment.
//
// Version:
//   - 2026-09-24: Added.
func TestPositionManagerDeployment(t *testing.T) {
	for _, chain := range []uint64{8453, 1, 10, 56} {
		d, err := ByChainID(chain)
		if err != nil {
			t.Fatal(err)
		}
		var want common.Address
		if chain == 8453 {
			want = common.HexToAddress("0x7c5f5a4bbd8fd63184577525326123b519429bdc")
		}
		if d.PositionManager != want {
			t.Fatalf("chain=%d manager=%s want=%s", chain, d.PositionManager, want)
		}
	}
}
