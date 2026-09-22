package tax

// Reviewed complete runtime hashes and exact OpenZeppelin dependency contents.
// Provenance and accepted behavior are documented in README.md.
var reviewedRuntimes = map[string]string{
	"667c900c2c6da80d452501a9c6332e046384a0c438c3334ce6f71c86dd7b8735": "weth9-reviewed-runtime-v1",
	"9825b6b2c5680fc87cfb9926a36b1b8eefc51eb5143d65f335a961e3561017e2": "taot-reviewed-runtime-v1",
}

var reviewedSources = map[string]string{
	"@openzeppelin/contracts/access/AccessControl.sol":                  "97abba9ae4ee97f16f17b7b688d90f6f1195b79edfba344eacc58406e088b51e",
	"@openzeppelin/contracts/access/IAccessControl.sol":                 "140304b7e99d517c57a13f391dfd3b550ca633a507f7b2af130f391b6cd89cc5",
	"@openzeppelin/contracts/interfaces/IERC5267.sol":                   "f89772d2d4fafcbef8b73bb10ee9a3373edec2eedb2f1b302320915b351dc3d1",
	"@openzeppelin/contracts/interfaces/draft-IERC6093.sol":             "fa7068f56bc180571a6095be6eaef06d51dbe81e30cb128397d993c9599ecc30",
	"@openzeppelin/contracts/token/ERC20/ERC20.sol":                     "50f34ae16a067a41c2c1091445d11d63788e54c717677cb6d6b0e4cdea2ad21d",
	"@openzeppelin/contracts/token/ERC20/IERC20.sol":                    "01b6f5c4fa45fd38822b286ecef6daf983d27306dd6362496fa71b3e4600b72c",
	"@openzeppelin/contracts/token/ERC20/extensions/ERC20Permit.sol":    "53584e84cd4851447b3149219f8c1b41397f7c753f85da07eda7f17ac6846e47",
	"@openzeppelin/contracts/token/ERC20/extensions/IERC20Metadata.sol": "0b7132f17d14d1d84b41b0bb429be62dafdff00fd3470f68724d8018eb07f57a",
	"@openzeppelin/contracts/token/ERC20/extensions/IERC20Permit.sol":   "c302edfbf7f396e1130be81278c5c994543868f0f5bf0c0fb3f557e166d8d3f2",
	"@openzeppelin/contracts/utils/Bytes.sol":                           "3ce9120a27934c75a074cb785d3a88868069ef6529dbe4dc6b845af0836f3c70",
	"@openzeppelin/contracts/utils/Context.sol":                         "847fda5460fee70f56f4200f59b82ae622bb03c79c77e67af010e31b7e2cc5b6",
	"@openzeppelin/contracts/utils/Nonces.sol":                          "9b4cbb85d1f5053c744e83302538eb643a713ffd14bc37665b224f1c66529339",
	"@openzeppelin/contracts/utils/Panic.sol":                           "270fc8401c1a13fae6a7a4a2dd6e381b95d658896701e51f0d3e2688acab3dec",
	"@openzeppelin/contracts/utils/ShortStrings.sol":                    "1cfa08d3c551e9ec0b9cd249b09987f04e7e90f352e1a5e3b0d6a3bc426b3de7",
	"@openzeppelin/contracts/utils/StorageSlot.sol":                     "75704538dcb223239280c6726d9a31cf769a7816718517c997fc7d63bdb70778",
	"@openzeppelin/contracts/utils/Strings.sol":                         "e318709bbe73a7831d64c1351e8bdc43780b245f25191234e361e6ab9b98c67b",
	"@openzeppelin/contracts/utils/cryptography/ECDSA.sol":              "ba7c2d314fcd61c9783e0b3f0c664d004b76b408fda6a84edc8267d17c5012d6",
	"@openzeppelin/contracts/utils/cryptography/EIP712.sol":             "2845caaaf70757b6cce612ae4ffbec2c382cb3d1cb9765c98fa649c0f5f1c794",
	"@openzeppelin/contracts/utils/cryptography/MessageHashUtils.sol":   "538a0d7c4f0ec5a8562f58561890f7b49d14580674ecf58bc5f505b45c73df65",
	"@openzeppelin/contracts/utils/introspection/ERC165.sol":            "61a2b12c96857aaa35f48aeda67a4281637a42494c0b3795be0ae976aa7b3793",
	"@openzeppelin/contracts/utils/introspection/IERC165.sol":           "9055c2994b37dea1a41b7b7926dcb510f05dbe2540b0aafc5fbee9558fffd0ca",
	"@openzeppelin/contracts/utils/math/Math.sol":                       "bdfdbe133991c0c78042957ff5cd97167926fcaf6d15664f3835a076cb066457",
	"@openzeppelin/contracts/utils/math/SafeCast.sol":                   "5779bc848bde39f1ad7bc02b4f708a0040888e0083b1a33f119cd94639350134",
	"@openzeppelin/contracts/utils/math/SignedMath.sol":                 "1ed50b1056af886752f0fb48a0165d381e69bb4a4b18b893b066dc144a7e08d7",
}
