# 作成証拠・承認履歴のSDK型と限定ルール

2026-09-24。現行は`lp-protection-20260924-v5`。以下のCLルール・履歴要件は維持する。

現行v4形式は既存validatorを通してv5へ移行し、失敗回数・放棄済み範囲を維持する。
v3以前・不正な保存証拠は復元エラーとする。呼出し側で失敗回数をリセットして自動再取得しない。
V4の永久保管は[専用の作成・権限証拠](V4_PERMANENT.md)で扱い、CLルールの履歴を省略しない。

## 型と呼出し方

| 型・項目 | 用途 |
| --- | --- |
| `CreationRPC` | `TransactionByHash`・`NonceAtHash`を明示的に注入する追加依存。既存`RPC`は変更しない |
| `NewReaderWithCreationRPC` | 作成検証を有効にするconstructor。従来`NewReader`は互換維持し、作成候補だけで取得範囲を短縮しない |
| `CreationHint` / `Request.CreationHints` | Factory作成transactionの検索候補。外部のsource情報から得てもよいが、値自体は証拠として信頼しない |
| `CreationEvidence` / `Evidence.Creations()` | 作成rule、Factory・保管先・実装、作成tx/block、code hash、CREATE nonce、LockCreated。返却はコピー |
| `PermanentCustodyEvidence` / `Evidence.PermanentCustodies()` | V4永久保管の専用証拠。初回作成tx・署名者・生成nonce・runtime・initcodeを保持。返却はコピー。再利用時もReaderによる検証が必要 |
| `OperatorHistory` / `Evidence.Histories()` | manager・保管先・code hash・開始位置・連続取得末尾・発見した全operator・失敗範囲。返却はコピー |
| `HistoryFailure` | 固定の未取得範囲・実行回数・理由。観測位置の更新や保存復元で回数をリセットしない |
| `Evidence` / `Request.Evidence` / `Result.Evidence` | SDKが生成する変更不可の証拠。取得途中でも返し、次の評価へ渡せる |
| `RestoreEvidence(data, maxBytes)` | SDKが保存した内部JSON専用の復元。版・識別・形・サイズを検証する |

`complete=true`や任意の履歴開始blockを渡して短縮するAPIは用意しない。
`Evidence`の中身は非公開フィールドで、`json.Unmarshal`による直接復元を正式な経路にしない。
`MarshalJSON`→`RestoreEvidence`の経路を利用する。

```go
reader, err := lpprotection.NewReaderWithCreationRPC(rpcClient, creationClient, limits)
// errを処理する。どちらも同じchainへ接続し、各methodは1回だけ送信する。
request.CreationHints = []lpprotection.CreationHint{{
    Factory: factoryAddress, TransactionHash: deploymentCandidate,
}}
request.Evidence = previousEvidence
result, err := reader.Analyze(ctx, request)
// エラー時もresult.Evidence・Metrics・PositionIDsを保存できる。
// Observationがnilなら割合を公開しない。
```

候補の検索サービスはSDK内部で生成しない。初期の作成候補は呼出し側から渡す。
`LockCreated`は取得済みPool/Mint receiptや`Request.Receipts`から探す。
既に検証した同じ保管先は保存された作成候補とreceiptを再利用する。
作成tx候補の自動取得とPoolを跨ぐ共有キャッシュ・workerへの接続はMarketHub側の次工程。

## 限定ルール

Baseの現行Slipstream、レビュー済みCL locker/Factoryのruntime照合を通過した場合に限る。
lockerのアドレス一覧やUNCXを判定根拠にしない。

1. 成功したFactory作成tx/receiptを照合し、chain・署名・宛先・送金0を確認。
2. `deployCreate3(bytes32,bytes)`の正規ABIと、senderに結び付いたsaltの限定分岐を確認。
3. constructor bytecode 12,988 bytesのSHA-256を照合し、160 bytesの引数をruntimeのimmutable値へ結び付ける。
4. レビュー済みCreateX runtimeを作成blockとそのparent hashで照合。CREATE2 proxyアドレスとproxyのnonce 1からFactoryを導出し、対応する両作成イベントとproxy runtimeを確認。
5. Factoryの作成時・locker作成時・観測時runtimeの一致を確認。cloneと実装の作成時runtimeも現在値と一致させる。
6. `LockCreated`を対象NFT・保管先へ結び付け、作成blockの前後のFactory nonce範囲から実際のCREATEアドレスを導出する。
7. 作成blockを**含めて**観測位置まで`ApprovalForAll`を取得する。constructor内や同ブロック内の承認を除外しない。

レビュー済みCreateXの生成経路と、破棄・nonceリセットを持たない固定proxyにより、Factoryの同じアドレスの過去の生存期間を除外する。
Factoryは照合済みconstructor/runtimeの限定生成経路を持ち、任意delegatecallや自己破棄がない。
「直前blockでcodeがない」「runtime一致」「イベントがある」だけで初回作成とは判定しない。

CreateXの他のsalt分岐・別deployment method・同一block内のFactory/locker作成・nonce差分1,024超・未知codeはこのルールの対象外。
対象外は従来の全履歴取得の予算判定へ戻り、取得できなければ未判定。
MultiVaultの作成証拠、V4、他Chainへこの生成ルールを流用しない。

本実装はレビュー済みfingerprintとアドレス計算で検証する。
調査時のローカルEVM再実行、source全文、compiler、debug trace RPC、研究用の偽の全履歴応答は本実装へ持ち込まない。
新しいproduction依存はない。

根拠:

- [CreateXの生成・salt処理](https://github.com/pcaversaccio/createx/blob/main/src/CreateX.sol)
- [独立コンパイル・ローカルEVMによる固定サンプル検証](../../../experiments/lp_operator_proof_20260923/README.md)
- Factory constructor SHA-256: `bbe406b3ec56ea8796f0ef27c923c1b79e907914098bf8b463cdf7edfca06c61`
- CreateX runtime Keccak-256: `bd8a7ea8cfca7b4e5f5041d7d4b17bc317c5ce42cfbc42066a00cf26b43eb53f`

これらは限定されたコード構造の検証であり、任意のcontractの完全な監査ではない。

## 履歴の再利用・欠落

- 連続成功末尾の次blockから差分取得。初回は検証済み作成block、根拠なしなら0。
- 各区間の末尾headerを取得前後で照合し、その間に取得済み末尾（初回は検証済み作成block）もRPCで再確認してからcheckpointを進める。1区間は通常4 RPC、genesis開始の最初の区間のみ3 RPCで、preflight予算にも含める。空の正常なログ応答も成功。
- 取消されたoperatorも集合に残す。取得済みログだけで現在値を決めず、観測hashで`isApprovedForAll`を読み直す。
- 失敗区間を飛ばさない。1回の`Analyze`で同じ区間を内部リトライしない。
- 初回+3回の失敗を保存すると、以後`ErrHistoryAbandoned`を返し、同区間へのRPCを送信しない。バックオフと再投入は呼出し側が担当。
- 予算preflightで送信しなかったものは失敗回数に含めない。
- reorgで開始根拠／保存末尾／最終観測hashが一致しなければ、保存証拠を破棄し、その評価では未判定を返す。次の評価で再構築できなければ未判定のまま。
- Live通知は再評価の契機。通知がなかったことだけでは履歴末尾を進めない。

歴史的なtx/receipt/code/nonce応答を証拠内に保持し、途中の取得失敗後にも再利用する。
入力イベントと保存receiptのblockが食い違う場合は、そのreceiptと依存する作成証拠・承認履歴を先に無効化し、同一予算内で再取得する。同一txへのreceipt RPCは1評価で最大1回。
再取得失敗・予算切れ・再取得結果の不一致では未判定とし、古い証拠を復活させない。無関係な保管先の失敗回数は維持する。
呼出し側の`Request.Receipts`は変更しないため、呼出し側でも古いキャッシュを置き換える。
保存JSONも`Limits.MaxResponseBytes`を上限とし、超過したデータを切り詰めて完全扱いしない。
解析RPCの`ResponseBytes`はデコード後の概算であり、保存JSONサイズ／HTTP転送量とは別。
長期キャッシュの共有・廃棄方針とDB保存上限はMarketHubへの接続時に扱う。

**信頼境界:** RPCと内部保存領域は信頼する。保存hashだけでは履歴に欠落がないことやDB改ざん耐性は証明できない。
`RestoreEvidence`は外部APIの任意JSONに使用しない。復元後もcanonicalityと可変状態はRPCで再確認する。

## 検証

- 通常のproduction Reader経由の割合算出、opaque型のコピー・保存復元・承認履歴の再利用。
- 偽constructor、異なるruntime、偽イベント、異なるtx、誤nonce、nonce範囲超過、候補不足の拒否。
- 作成block内の承認、取消済みoperator、現在の承認状態、部分進捗、失敗4回後の停止、reorg、旧版・サイズ超過の拒否。
- 区間間／作成anchor／最終観測でのreorg、末尾再確認の一時失敗、tx再収録時のreceipt再取得と予算・依存証拠の無効化。
- 旧constructor互換、追加依存のDI、外部呼出し回数と予算の照合。

`testdata/creation-rpc.json`は上記調査の公開RPC応答を必要部分だけ保存したfixture。
単体テストの不足する区間末尾header・manager code識別・実装の過去codeはテスト用に補い、そのことをテスト内に明記している。
実RPCテストはそれらもRPCから取得し、調査用adapterや単体fixtureの肯定判定を利用しない。
