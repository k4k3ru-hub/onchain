# LP保護 v3：作成証拠・承認履歴の本実装と検証

2026-09-23。onchainの許可済みmainブランチで実施。commit / pushなし。

## 実装

- `types.go`・`reader.go`: 既存constructor/RPC互換を保ち、追加の取得依存を明示注入する`NewReaderWithCreationRPC`、入力候補、内部証拠の入出力を追加。
- `creation.go`・`creation_rpc.go`: CL Factory / CreateX / CREATE cloneの限定生成ルール。実transaction/receipt・constructor fingerprint・引数・runtime・アドレス導出・nonce・canonicalityを検証。
- `evidence.go`: SDK内部証拠、保存復元、コピーでの参照、取得済み応答の再利用、保存サイズ制限。
- `approvals.go`・`custody.go`・`rpc.go`: 作成blockを含む履歴取得、連続checkpoint、現在のoperator承認、固定失敗区間、初回+3回後の打切り、receipt再利用。
- `creation_test.go`・`evidence_test.go`・`creation_live_test.go`・既存fake: 生成経路・再利用・異常系・実RPC検証。fixture `testdata/creation-rpc.json`は公開RPC応答。
- `README.md`・`EVIDENCE.md`: 公開型、信頼境界、非対応経路、MarketHubへ残す責務を記録。

通常コードから調査用packageやAgentへのimportはない。go.mod / go.sumは変更していない。
詳細なAPIとルールは[設計・利用方法](EVIDENCE.md)を参照。

## オフライン検証

すべてPASS:

```sh
go test ./evm/lpprotection ./evm/clliquidity
go test ./...
go test -race ./evm/lpprotection ./evm/clliquidity
go vet ./evm/lpprotection ./evm/clliquidity
go vet ./...
go build ./...
GOWORK=off go test ./evm/lpprotection ./evm/clliquidity
```

workspaceのgeth 1.17.5と、onchain単独go.modの1.17.3で確認。
`go list -deps ./evm/lpprotection`でも研究用package・Agent依存がないことを確認。

主な異常系:

- 現在runtimeだけを再現する異なるconstructor、未知deployer、偽イベント、異なるtransaction、誤ったnonce、過大nonce範囲。
- 作成block内のoperator承認、Pool作成前の承認、取消済みoperatorの保持と現在値照合。
- 途中失敗→保存復元→同じ未取得区間からの再開。観測blockを進めても失敗回数を維持。
- 4回失敗後は取得を停止。後続区間へ飛ばして割合を返さない。
- 作成根拠／履歴末尾のreorg、旧版・不正形・サイズ超過。
- 追加依存のDI、旧constructor互換、RPC予算、入力証拠を変更しないこと。

通常fixtureによるcold完遂は63 method呼出し。これはRPC失敗・待ち時間を持たないテスト上の回数であり、公開RPCで2分以内にcold完遂したことを意味しない。

## 実RPC検証

コマンド:

```sh
ONCHAIN_LP_CREATION_LIVE=1 go test ./evm/lpprotection -run '^TestCreationRuleLive$' -count=1 -v
```

Base公開RPCへの読み取りのみ。実際のbounded `eth_getLogs`を使用し、研究用の全履歴応答adapterを使わない。

- Pool: `0x197A0913aCC071cc6D0B5611A72b65F5838797eE`
- 観測: Base mainnet block **51,596,896** / `2026-09-21T09:32:19Z`
- NFT: **6,628,901**
- 検証できた保管先の初回作成: block **51,590,853**
- 作成候補transactionは入力hint。値だけを信用せず、productionルールで検証。
- 元本snapshotの準備: 5 RPC。既存のprincipal readerを使用。

最初の1秒間隔の試行はLP評価9呼出し目でHTTP 429となり停止。割合は返さなかった。
再検証は2.5秒間隔。SDKに隠れたリトライを追加せず、各評価の上限は2分・128呼出し・追加receipt64件を維持。

| 評価 | 所要時間 | SDK method呼出し | eth_getLogs | 追加receipt | 結果 |
| --- | ---: | ---: | ---: | ---: | --- |
| cold | 120.00秒 | 48 | 2 | 2 | deadlineで未判定。作成証拠と2区間のcheckpointを保持 |
| 次の独立した評価 | 115.08秒 | 46 | 5 | 0 | 残り5区間のみ取得して割合確定 |
| JSON保存・復元後、同じ観測block | 77.53秒 | 31 | 0 | 0 | 履歴・作成tx・nonceを再取得せず同じ割合 |

成功した後半2評価はHTTP送信数もそれぞれ46・31で一致。
coldの48はRPCインターフェースへの呼出し数で、deadline時はtransportの送信待機中に止まる可能性があるためHTTP実送信数と同一とは断定しない。
上記3評価の呼出しを足して「1評価で2分以内」とは扱わない。**coldからの完遂は今回2分に収まらなかった。**
全テスト所要約322.83秒。各評価の上限が機能し、進捗再利用により後続評価で完了したことを確認した。

保存証拠JSON: **128,900 bytes**。今回の2 MiB上限以内。
追加のsourceダウンロード・compile・debug traceは0。

結果:

| 項目 | 値 |
| --- | --- |
| Token0ロック割合 | null（元本0） |
| Token1ロック割合 | 100% |
| Token1永久保護割合 | 0% |
| allPositionsProtected | true |
| 最短解除日時 | 2106-02-07 06:28:15 UTC |
| canWeakenProtection | true |

これは**現在設定での期限付き保護**であり、変更権限がないという判定でも永久保護でもない。
定義済みTokenの信頼判定とも独立している。

## 残る範囲

- 同じPoolの固定過去blockでのSDK検証。現在のNewPair掲載・Agent E2Eはまだこの単位の対象外。
- MarketHubのworker-pool、Factory作成候補の検索、共通cache、DB保存、世代管理、イベントによる無効化は未接続。
- 広範な保管先、MultiVaultの作成経路、V4は未対応。未知を0%や100%へ置き換えない。
- RPC制限下でcold完遂を速くする運用値は未調整。再投入とバックオフはアプリケーションで制御し、同じ失敗区間の回数をリセットしない。
- SDK snapshotは信頼した内部保存専用。hash照合だけで改ざんやRPCの不完全応答を暗号学的に排除するものではない。
