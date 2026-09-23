# V4第1段階のレビュー・LaunchLocker永久保護の実装

2026-09-24。承認済み計画のonchain第2段階。作業repositoryは`onchain/main`。

## 第1段階の差分レビュー

対象は公開済みcommit `7af45af`のV4 deployment・Pool識別・持分探索・通常EOA引き出し。
Pool IDの32 bytes保持、salt別core照合、全tick gross/net、burnと取得失敗の区別、native通貨、
固定block hash、受信receiptの再利用、予算、V3 / Slipstreamへの分岐を確認した。
対象testを再実行し、必須修正の指摘はなかった。実RPC・最新掲載Poolの成功まで保証するレビューではない。

## 今回の実装

`v4-launch-factory-create-locker-v1`を追加した。名前や保管先アドレスのallowlistでは判定しない。
初期対象はBase・Hookなし・公式PositionManagerのまま。

1. 実ownerのLaunchLocker runtimeを照合し、全immutableのmanager / factoryを確認する。
2. Factory runtimeとPoolManager / PositionManager / Permit2 / lockerを照合する。
3. 注入済み`CreationHintResolver`、明示候補、保存済み証拠からFactory作成tx候補を得る。
4. 署名付きtype 2、Base chain ID、to=null、nonce=0、value=0を検証し、署名者からFactoryを導出する。
5. initcode 19,996 bytesのfingerprintと末尾96 bytesの3引数を完全一致させる。末尾余分dataも拒否する。
6. FactoryのCREATE nonce 1から実Lockerを導出し、成功receipt・contractAddress・作成blockを照合する。
7. 作成blockと観測blockのFactory / Locker runtimeを完全一致させる。
8. 現在のNFT owner・approval、提供済みreceiptの権限イベント、保存された矛盾を確認する。
9. 作成block・Pool作成blockを最後にも再取得し、観測block確認後に割合を返す。

code hashだけではconstructorで残された承認を排除できないため、作成入力と生成経路も必須。
検証済みコードには元本減少・NFT移転・承認・proxy / delegatecall・自己破棄・ERC-1271の経路がなく、
手数料回収のliquidity減少量は常に0という限定規則を用いる。
既知の公式PMと署名・アドレス衝突困難性、RPC・内部保存領域を信頼境界とする。
任意Solidityの自動監査や、有限テストによる完全な形式検証とは扱わない。

V3 / Slipstreamの承認履歴は従来どおり必要。
このV4モデルが成立しない場合に、長い承認履歴の全走査へ自動fallbackしない。
本番でのコンパイル・ローカルEVM実行・debug trace・新しいproduction依存は追加しない。

## 証拠・矛盾・移行

- `PermanentCustodyEvidence` / `Evidence.PermanentCustodies()`を追加。読み取りコピーのみを公開する。
- Manager / Factory / Custodian / 署名者、作成tx / block、initcode / runtime hash、生成nonceを保持する。
- 現在の個別approvalが非zero、または確認済みreceiptでLockerからのApproval / ApprovalForAll / Transferの
  矛盾を見つけた場合は未判定。矛盾をblockに結び付けて内部保存し、approvalが後から0になっても肯定へ戻さない。
- 権限イベントは`Request.Receipts`または取得・保存済みreceiptから確認する。権限専用購読・定期RPCは作らない。
- 別owner / managerのイベントは混同しない。removed・取得不能・canonicality不一致から肯定しない。
- 矛盾自身のblockがorphanedだった場合のみ、その矛盾を除去する。その評価はreorgエラーで終了し、次回再構築する。
  無関係なreorgではcanonicalな矛盾を消さない。
- modelは`lp-protection-20260924-v5`。旧`lp-protection-20260923-v4`は既存validatorを通して移行する。
  Creations / Histories / Acquired、失敗回数4・放棄済み範囲を保持し、永久証拠は空から始める。
- 旧版JSONに新しい証拠fieldを入れても移行で受け入れない。v3以前・不正データは復元エラー。
- SDKの移行だけでは、MarketHub外側のModelVersion / Failures / Hints / StopReason処理は更新されない。
  それらの保持、旧観測をstaleにする処理、停止レコードの移行はサーバー接続工程で対応する。

## 返却

`Position.Kind="permanent"`を追加し、期限付き`locked`と別集計する。
Token別の元本に対して永久割合を算出し、丸める前の全持分分類から`AllPositionsProtected`を決める。
永久保護には架空の解除日を入れない。`EarliestUnlockAt`は期限付き持分だけから集計する。
変更権限は保護持分のみを集約し、未判定の有効持分が1件でもあればPool全体の割合は返さない。
NewPairの掲載条件・公開JSON型・DDLは今回変更していない。

## 保存済み実データでの再現

Pool `0x2f20eec5945b32624fb6ccb8ba8716aabea99bc7d8e8590b5a863c283042601a`、NFT `3073314`。
観測block `51,604,306`、hash `0xeaa77d3a8cce998496372ea11342eff355c61f8ea86a5def29adc7633b6e62fe`。
**過去に取得したRPC応答をfake transportから本番Readerへ入力した再現テスト**。今回新たな実RPCは実行していない。

| 項目 | 結果 |
| --- | --- |
| Token0元本 | 0。locked / permanent割合ともnull |
| Token1 | locked `0`、permanent `100` |
| 全持分保護 | true |
| 解除期限 | null |
| 変更・解除可能 | false（限定モデルの確認範囲） |
| OperatorHistory | 生成せず、履歴RPC 0回 |
| 保存Evidence | 78,614 bytes（512 KiB以内） |

| Reader計測 | 呼出し回数 | 追加receipt | 応答bytes概算 |
| --- | ---: | ---: | ---: |
| 初回 | 24 | 2 | 89,946 |
| 証拠再利用 | 19 | 0 | 46,323 |

これらはfake transportへの呼出し件数。実ネットワーク遅延、provider料金、Registry cache効果、
Sourcify HTTP回数は実測していない。既存元本snapshotの取得コストも含まない。
Token0元本0なので、永久保護100%でも売却の出口があるという意味ではない。

## 検証

- 本番Readerで肯定 / 未判定を確認。欠落候補、誤tx、runtime / immutable差替え、作成時code不一致を拒否。
- 正しく署名された別constructor、誤nonce / chain / value / 引数・余分data・署名なしを拒否。
- 正規constructorをローカルEVMで再現し、Factory / Locker CREATEの2回、外部承認呼出し0を確認。
- 別constructorがLockerの包括承認を残し、同じFactory / Locker runtimeと整合するimmutableを返す反例を実行。
  実PMのgetterで承認成立を確認したうえで、本実装の作成入力検証が拒否することを確認。
- Authorityイベントの保存 / 復元、現在approvalの取消後の未判定維持、orphanedとcanonicalの矛盾の分離。
- 初回・最終確認時の作成 / Pool block reorg、RPCエラーの原因保持、RPC / receipt / bytes上限・context取消。
- v4の失敗4回を保持する移行、旧schema偽装・不正データ・サイズ超過の拒否。
- 期限付き / 永久 / 引出可能 / 未判定の混在集計、片側元本0、微小な未保護持分。

通常の確認（`onchain/go`）:

```sh
GOWORK=off go test ./evm/lpprotection ./evm/clliquidity ./venues/uniswap/v4/...
GOWORK=off go test ./...
GOWORK=off go vet ./evm/lpprotection ./evm/clliquidity ./venues/uniswap/v4/...
```

ローカルEVMのモデル検証は`lp_v4_model`タグを明示する。EVM用の間接依存をproductionのgo.mod / go.sumへ
追加せず、一時modfileを使用する。今回の実行先は`/private/tmp/onchain-v4-model.TXiHRD`。

```sh
LP_V4_MODEL_DIR="$(mktemp -d /private/tmp/onchain-v4-model.XXXXXX)"
cp go.mod go.sum "$LP_V4_MODEL_DIR/"
GOWORK=off go test -mod=mod -modfile="$LP_V4_MODEL_DIR/go.mod" -tags lp_v4_model ./evm/lpprotection -run '^TestPermanent(ConstructorReplay|FakeConstructor)$' -v
GOWORK=off go vet -modfile="$LP_V4_MODEL_DIR/go.mod" -tags lp_v4_model ./evm/lpprotection
```

通常・モデル検証のtest / vetはすべて成功。変更Goファイルのgofmt、差分の空白、文書のローカルリンクも確認済み。
repositoryのgo.mod / go.sumは変更していない。

テストはローカルの合成鍵で検証用txに署名するが、送信しない。サービスbuild・実RPC・Agent E2Eは今回未実施。
実Poolの現在状態での再確認、共有保管先を含む実RPC費用計測は後続工程。

## 変更ファイルと根拠

- [v4_permanent.go](v4_permanent.go)、[permanent_authority.go](permanent_authority.go)、
  [permanent_evidence.go](permanent_evidence.go)：限定規則・矛盾保持・専用証拠。
- [evidence.go](evidence.go)、[types.go](types.go)、[templates.go](templates.go)、[principal.go](principal.go)：移行・モデル版・fingerprint・集計。
- [v4.go](v4.go)、[v4_positions.go](v4_positions.go)、[reader.go](reader.go)、[rpc.go](rpc.go)、
  [creation_rpc.go](creation_rpc.go)、[receipt_cache.go](receipt_cache.go)：既存Readerへの接続と失効。
- [v4_permanent_test.go](v4_permanent_test.go)、[permanent_evidence_test.go](permanent_evidence_test.go)、
  [permanent_limits_test.go](permanent_limits_test.go)、[permanent_constructor_test.go](permanent_constructor_test.go)、
  [principal_test.go](principal_test.go)：再現・反例・回帰テスト。
- `testdata/v4-permanent-rpc.json`、`v4LaunchFactory.hex`、`v4LaunchLocker.hex`：公開RPCの必要部分とruntime。
  元データは[隔離済み調査](../../../experiments/lp_v4_proof_20260923/README.md)。production / testから研究コードはimportしない。
- この文書、[README](README.md)、[V4](V4.md)、[EVIDENCE](EVIDENCE.md)：対応範囲と移行手順。

照合したfingerprint:

| 対象 | bytes | SHA-256 |
| --- | ---: | --- |
| Factory initcode | 19,996 | `2acc2c3d3f93674e48f1be9de3c7b5025d4cfdc1b7399fd3a3c03f4265088603` |
| Factory runtime（immutable正規化） | 13,681 | `f194573d2926cf2826ccdada4eb8b6a136844c3dcca58802862b4cfeb6cde8be` |
| Locker runtime（immutable正規化） | 5,757 | `32562f683e5e70e56c01b78668929412d5fb9d7d9dc644f3fc4821bccab978f0` |

Solidity `0.8.26+commit.8a97fa7a`による独立コンパイル結果と、実runtimeの全immutable位置を照合した。
公開ソースのMITおよび依存ソースのlicense情報は元の調査archiveに保持されている。
[署名経路の仕様](https://raw.githubusercontent.com/Uniswap/v4-periphery/main/src/base/ERC721Permit_v4.sol)と
[自己破棄の仕様](https://eips.ethereum.org/EIPS/eip-6780)も参照したが、upstream mainだけを実deploymentの証拠にはしない。

次は第2段階の差分レビューと、公開後の依存反映。その後にMarketHubのV4接続・保存移行・Agent E2Eを行う。
