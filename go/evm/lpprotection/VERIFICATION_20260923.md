# LP保護解析の初期実装・検証結果

実装日: 2026-09-23。対象: `onchain` の許可済み `main`。
commit / pushは実施していない。

## レビュー修正

解析モデルを `lp-protection-20260923-v2` に更新した。**v1で返していたSlipstreamの「100%・全持分保護true」は撤回する。**
runtime一致と個別NFT承認ゼロだけでは、constructor等によって過去に設定された包括承認を否定できなかったため。

- 保護済み判定の前に、canonical NFT managerの保管先別`ApprovalForAll`履歴をblock 0から観測位置まで調べ、発見した各operatorの現在値を観測hash固定で取得する。Pool作成より前の承認も対象。
- 有効な包括承認は`operator_approval_active`として未判定。履歴取得エラー・上限不足は`operator_history_unresolved`として未判定にし、Pool全体の`Observation`を返さない。管理者権限なしとも断定しない。
- 全履歴の走査が残りRPC上限に収まらない場合は、走査開始前に`ErrBudget`を返す。1,000ブロック単位・128回上限では、実Baseの包括承認履歴を網羅できない。検証済み作成経路または再利用できる完全な承認索引の導入が今後必要。
- Pool作成確認で取得したreceiptと入力済みreceiptからMint / IncreaseLiquidityを先に再利用する。全tickの持分を説明できればMint履歴RPCは不要。不足時も、履歴の途中で全持分が揃えばそこで止める。
- 持分の再取得に失敗しても、発見済みNFT IDは返却し次回の再利用に残す。

定義済みTokenを信頼する処理とは独立している。USDC / WETHを含むPoolでも、LP持分の保管先・承認・保護を省略しない。

## 実装

- `evm/lpprotection/`: Pool作成イベントから持分を列挙し、Pool / manager / NFT / 全tick / 保管先コードを同一観測位置で照合するreader。
- `evm/clliquidity/position.go`: 価格帯ごとの元本を整数・有理数で算出する共通処理。割合では小数点以下18桁で切り捨て、全持分保護は丸め前の持分分類から独立して判定する。
- `evm/lpprotection/testdata/`: 照合用runtimeと実Poolの作成イベント。productionは調査コードやAgentをimportしない。
- 新規の本番依存、既存公開APIの破壊的変更、DB変更、MarketHub掲載条件変更はない。

詳細なモデル条件・コード根拠・取得責務は [README](README.md) を参照。

## 実Pool照合

新規の直近Poolサンプリングではなく、既存調査の2 Poolを固定した過去ブロックで正式readerから再検証した。
観測位置はBase mainnetの **51596896**、`2026-09-21T09:32:19Z`。
block hash: `0xacc268320c2198258d4b13b4fb4f90bb810d626b9329fe58ac60e52c55796ba2`。

入力にはPool作成イベントとLP状態を渡した。保管先・NFT ID・外部サービスのロック判定は渡していない。
NFT IDはPoolのMint receiptから発見し、NFT状態・owner・承認先・runtime・期限・移行設定などを独自に取得した。

| 項目 | Uniswap V3 | Aerodrome Slipstream（current deployment） |
| --- | --- | --- |
| Pool | `0x81f3bD2fF7147a6Fd779fe749f4Bcc82C802C060` | `0x197A0913aCC071cc6D0B5611A72b65F5838797eE` |
| 発見したNFT ID | `6059166` | `6628901` |
| 保管形態 | 通常EOAの直接保有 | CL locker cloneのruntimeは一致、包括承認の根拠不足 |
| Token0期限付き保護割合 | `0` | 未判定（Observationなし） |
| Token1期限付き保護割合 | `0` | 未判定（Observationなし） |
| 全持分保護 | `false` | 未判定（Observationなし） |
| 最早解除日時 | なし | 未判定（Observationなし） |
| 保護を弱める変更権限 | `false` | 未判定（Observationなし） |
| 初回の追加RPC | **15回** | **25回**（未判定まで） |
| 索引再利用時の追加RPC | **15回** | 未判定例の反復は省略 |
| 初回の追加receipt | 1件 | 1件 |
| 初回の履歴RPC | **0回** | **0回** |
| 索引再利用時の履歴RPC | **0回** | 未判定例の反復は省略 |
| 初回の解析時間 | 約37.9秒 | 約62.5秒 |
| 共用LP状態の準備RPC（上記に含めない） | 5回 | 5回 |

Slipstreamの元本・保管先・期限などが取得できても、包括承認の証拠が不足するため割合を公開しない。
従来確認した`4294967295`の期限は永久ではない。期限や管理者の情報だけでも、保護済み割合の根拠にはならない。

初回のmethod別RPC件数:

| method | V3 | Slipstream |
| --- | ---: | ---: |
| `eth_chainId` | 1 | 1 |
| `eth_getBlockByNumber` | 2 | 1 |
| `eth_getTransactionReceipt` | 1 | 1 |
| `eth_getLogs` | 0 | 0 |
| `eth_getCode` | 1 | 3 |
| `eth_call` | 10 | 19 |

初回解析ではHTTP transportの実送信回数とreaderのカウントが一致することもassertした。
解析上限は1回2分・128送信・追加receipt64件。取得済みLP状態は別費用として計測した。
追加のMulticallは使用しておらず、LP状態の準備のみ既存のMulticall経路を使った。
decoded payloadはV3が3,488 bytes、Slipstreamが31,524 bytes。HTTP/JSONのwire bytesではない。
Slipstreamの25回は128回を使い切ったという意味ではなく、必要な包括承認履歴の取得が残り予算に収まらないと判明した時点までの消費。履歴走査を始めずに未判定を返した。

v1の公開RPC検証では429と履歴範囲上限の413が発生したため、今回は2.5秒間隔・1,000ブロック単位に設定した。
上表は最終テストの各解析1回の値であり、それ以前の失敗した検証実行の消費を含まない。Slipstreamは保護率の算出成功ではなく、期待する未判定結果への一致を確認した。
SDKは自動再試行せず、取得エラーをwrapped errorと未判定で返す。
初回＋3回のバックオフ、再投入を跨ぐ失敗回数の保持、実送信の共通上限は次のMarketHub接続工程で実装する。

## 実行した検証

| コマンド・確認 | 結果 |
| --- | --- |
| 変更Goファイルへの `gofmt -w` | 完了 |
| `go test ./evm/lpprotection ./evm/clliquidity` | PASS |
| `GOWORK=off go test ./evm/lpprotection ./evm/clliquidity -race` | PASS |
| `go vet ./evm/lpprotection ./evm/clliquidity` | PASS |
| `GOWORK=off go vet ./evm/lpprotection ./evm/clliquidity` | PASS |
| `GOWORK=off go test ./...` | PASS |
| `ONCHAIN_LP_PROTECTION_LIVE=1 go test ./evm/lpprotection -run TestLPProtectionLive -count=1 -v` | PASS。V3は0%とreceipt再利用、Slipstreamは包括承認の根拠不足による未判定を確認。sandbox内のDNS失敗後、許可を得て外部通信可能な環境で実行 |
| `go list -deps ./evm/lpprotection` | 自モジュール依存は `clliquidity` と `lpprotection` のみ。調査package / Agent依存なし |
| 差分・新規ファイルの空白・文書リンク確認 | PASS |

`GOWORK=off`でも実行し、workspaceで上書きされる依存ではなくonchain自身のgo.mod（go-ethereum v1.17.3）でのテスト・vet通過を確認した。

固定fixtureでは、期限前・期限切れ、90%/10%のToken別分布、丸めで消えるほど微小な未保護持分、未知コード、個別承認、追加operator経路を持つ不一致コード、移行設定あり、gaugeあり、コード不一致、持分網羅不足、core position不一致、観測位置不一致、期限・RPC・receipt・応答量の上限を確認した。

修正の回帰テストでは、runtimeが同一でも有効な包括承認があるケース、承認取消後の現在値、履歴取得失敗・予算超過・不正イベント、Pool作成前を含む全範囲照会を確認した。作成receipt・入力済みreceiptの再利用、不要なMint履歴RPCの省略、途中で全持分が揃う履歴の打切り、取得エラー後のID保持、removedログの拒否も確認した。

## 残る範囲

- V4、Legacy Slipstream、永久保護の肯定判定は初期モデルの対象外。
- Vault key発行後の権限、smart wallet / EIP-7702、未検証のmigration / gauge / operator経路は未判定。
- 128回上限での実Baseの肯定的なlock判定には、包括承認について少ないRPCで網羅性を証明できる仕組みが必要。今回の修正は根拠不足で100%を返す経路を閉じたもので、実Poolで肯定判定できる範囲を広げるものではない。
- 履歴中のNFTが読めなくなった場合、任意のrevertをburn扱いして除外せず未判定にする。burnを証明した索引の整備は別途拡張する。
- 期限付きVaultの未来期限の肯定例・期限切れ例は固定fixtureで検証した。今回の実RPC照合は通常EOAとCL locker cloneの2例。
- この検証は現在掲載中のNewPairのAgent E2Eではない。SDK公開型、MarketHubのworker / 保存 / 更新 / 通知、Agent / Consoleへの接続は次工程。
