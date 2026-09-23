# LP包括承認：作成経路を確認したSlipstream 1例の検証

検証日: 2026-09-23。設計レビューで合意した実Pool検証。
**この1例では、作成経路を確認することで承認履歴を7 RPCに絞り、Token1の期限付き保護割合100%まで算出できた。**

今回は調査用の取得・証拠再利用処理を追加した。productionの`lpprotection`コード、公開API、DB、go.mod / go.sumは変更していない。
通常のSDKは作成根拠の自動取得・検証をまだ持たず、このPoolに対する従来の未判定処理は維持される。
過去の根拠不足だった100%をそのまま再採用した結果ではない。

## 対象と結果

| 項目 | 結果 |
| --- | --- |
| Chain / Network / Venue | Base mainnet / current Aerodrome Slipstream |
| Pool | `0x197A0913aCC071cc6D0B5611A72b65F5838797eE` |
| NFT ID | `6628901` |
| locker | `0xdbf7a3d301e39871e6644a17fb1af1a3889078af` |
| locker作成ブロック | `51590853` |
| 観測ブロック | `51596896` / `2026-09-21T09:32:19Z` |
| 観測hash | `0xacc268320c2198258d4b13b4fb4f90bb810d626b9329fe58ac60e52c55796ba2` |
| 包括承認 | 作成ブロックを含む6,044ブロックを連続取得。7区間すべて正常応答、イベント0件 |
| 個別NFT承認 | zero address |
| Token0期限付き保護割合 | `null`（Token0元本0） |
| Token1期限付き保護割合 | `100` |
| Token1永久保護割合 | `0` |
| 全持分保護 | `true` |
| 最早解除日時 | `2106-02-07T06:28:15Z` |
| 保護を弱める変更権限 | `true` |

100%は観測位置の現在設定における期限付き保護であり、変更権限や永久保護の判定とは分けている。
LP元本・全tickとの網羅性・core position・runtime・移行先・gauge等は既存readerが同じ観測hashで再確認した。
Tokenをonchain定義で信頼する判定は用いていない。資産の売買判断や、新規掲載中PoolのE2E結果ではない。

取得結果は[result.json](result.json)、作成根拠は[proof.json](proof.json)に保存した。

## 作成位置を短縮できた根拠

Pool入口から取得済みのPool作成receiptを使用した。このreceiptにPoolCreated、Mint、IncreaseLiquidity、NFT移転、lockerの初期化、FactoryのLockCreatedが含まれていた。
Poolの作成とlockerの作成が同じtransactionだったことは今回のサンプルの性質であり、全Poolに一般化しない。

### Factoryの作成まで遡る

- NFT所有者のclone runtimeから実装・Factoryへ辿る既存の情報と、保存ソース内のdeployment候補を照合した。
- Factoryは`0x932d0b4c00a2a33ef1ec5fe0aa981bd1a00a6f5c`。
- 作成txは`0xb498956df7c6d233f510db3298adba98aab1d13f7b70079e6324af699ac93701`、block `44515546`。
- 作成txの入力にあるinit codeは、保存されたCLLockerFactoryソースを**solc 0.8.30で独立再コンパイル**した12,988 bytesとconstructor引数160 bytesに一致した。
- runtimeもcompilerのimmutable挿入位置だけを正規化して一致し、反復immutableの値も一致した。作成時とPool作成時のruntimeは同一だった。
- ソースのdeployment宣言や`instances=true`だけを証拠にしていない。

### CreateX → 固定proxy → Factoryを確認

FactoryはCreateXの`deployCreate3(bytes32,bytes)`経由だった。
取得した作成時のCreateX runtimeと実際のtransaction入力を、**ローカルのgeth EVMで限定再実行**した。
RPCの`debug_traceTransaction`は使っていない。

確認した実行経路:

1. 元transaction sender → CreateX。
2. CreateXが固定16-byte init codeをCREATE2で実行し、proxy `0x6b9d4aaccc4d8b924a86c07b3b7129ab1b922ca1`を生成。
3. proxyがnonce 1のCREATEでFactoryを生成。
4. Factory constructorがcanonical NFT managerの`factory()`をSTATICCALL。
5. 得られたproxy runtime / Factory runtime / アドレスが取得したオンチェーン値と一致。

外部getterは同じ作成位置で実際にRPC取得した戻り値を与え、予想外の呼出しがあればテストを失敗させた。
これは取引全体のstate witnessを取得したフル再実行ではなく、依存するgetterを限定した生成処理の検証である。
CreateX全体のソースを再コンパイルした検証でもない。取得runtimeの限定経路を実行した結果と、[公式の生成処理](https://github.com/pcaversaccio/createx/blob/main/src/CreateX.sol)を照合した。

proxy runtimeは`363d3d37363d34f0`の8 bytesで、SELFDESTRUCT / DELEGATECALL / CALLCODEがない。
CREATE2のアドレス計算に結び付くinit codeも固定の返却処理で、外部呼出しを持たない。
proxyのnonce 1は再利用できず、同じsaltで作成をやり直す再実行も失敗した。
Factory runtimeにもSELFDESTRUCT / DELEGATECALL / CALLCODEがないことを、PUSHデータとmetadataを除いたopcode列で確認した。
CreateXの同じruntimeがFactory作成の直前ブロックにも存在し、CreateX runtimeにSLOAD / SSTOREもないことを確認した。

この検証は、通常のアドレスハッシュの衝突困難性、canonical managerの既知の承認仕様、RPCの正しい応答を前提にする。
任意のCREATE2 / CREATE3 / proxyを自動的に安全と認めるルールではない。

### locker自身の最初の生成を確認

Factoryのソースと照合済みruntimeでは、対象のlock経路がCREATEでcloneを生成する。
Pool作成ブロック前後のFactory nonceは**86 → 87**で、`CREATE(factory, 86)`から得られるアドレスが対象lockerと一致した。
同じreceipt内のLockCreatedはlockerとNFT `6628901`を示し、NFT移転も一致した。
これにより、任意のlockTimestampや「直前のコードが空だった」という情報だけに依存せず、このlockerの最初の生成位置を確定した。

Factoryのnonceを巻き戻して同じ子アドレスを作り直す経路も、上記の作成・runtimeの根拠で除外した。
同じ生成モデルでも根拠が揃わない別deploymentには、この結果を適用しない。

## 承認履歴の取得と割合の再計算

範囲`51590853..51596896`を1,000ブロックずつ7区間で取得し、NFT managerとlocker ownerでApprovalForAllを絞った。
全区間が正常に空を返した。失敗区間を飛ばして0件にした結果ではない。
発見したoperatorが0件なので、`isApprovedForAll(owner, operator)`の追加読取りは0回。
作成ブロックを含めており、constructor内や同ブロックの後続処理の承認も対象になる。

`proof_test.go`で作成根拠・全区間・観測hashを検証した後、調査用RPC adapterが以下を再利用した。

- 作成前にはその保管先が存在しなかった根拠。
- 作成から観測位置までの7区間の承認履歴。
- 取得済みchain IDとPool作成receipt。

adapterはこの特定manager / owner / 観測ブロック以外を拒否する。汎用的に空のログを返すものではない。
現行SDKへ完全な空履歴を渡すため、調査内だけで全範囲の論理的な問合せをcacheから満たしている。
**その大きなLogBlockRangeを公開RPCへ送ることや、本番へ設定することは提案していない。**
正式実装では検証済みの開始位置と取得済み範囲をreaderが直接扱える形にする必要がある。

このadapterを通して既存v2 readerの割合計算を実行し、所有者・コード・承認・期限・元本などの可変状態を再読取りした。
`result.json`の内部`ModelVersion`は呼び出した既存readerのv2であるが、外側にresearch-onlyのscopeを付けた。これを通常SDKが自動判定した結果として保存・公開してはならない。

## RPC実測

| 工程 | RPC実送信 | 補足 |
| --- | ---: | --- |
| 作成根拠・承認履歴・canonical headerの取得 | **29** | うち承認履歴7回、receipt 2件。複数段階で実施 |
| 共用LP snapshotの準備 | **5** | 全tick等の既存取得。保護専用取得と分離 |
| 証拠再利用後の保護評価 | **24** | 約**60.8秒**、追加receipt 0件、履歴の実送信0回 |
| 今回の外部RPC合計 | **58** | すべて読み取り。外部RPCの失敗・再試行0回 |

保護評価24回の内訳: `eth_call=19`、`eth_getCode=3`、`eth_getBlockByNumber=2`。
readerのinterface呼出しは26回だが、chain IDと承認履歴の2回をadapterのcacheが処理した。実送信との差を検証した。

全58回のmethod別内訳:

| method | 回数 |
| --- | ---: |
| eth_chainId | 1 |
| eth_getBlockByNumber | 12 |
| eth_getTransactionByHash | 2 |
| eth_getTransactionReceipt | 2 |
| eth_getCode | 9 |
| eth_getLogs | 7 |
| eth_getTransactionCount | 2 |
| eth_call | 23 |

外部RPC応答のbody合計は661,754 bytes。HTTP headerやTLS等の通信量、providerの課金単位とは別である。
本番の128送信・追加receipt64件より回数は少ないが、**初回の全工程を単一の2分以内に完了することは確認できていない**。
公開RPCの2.5秒間隔を含め、分割実行した取得と再計算の所要時間の合計は約145秒で、compile・手動解析・操作待ちは含まない。
したがって初回の全工程が2分以内に収まったとは報告しない。証拠再利用後の保護評価単体は2分以内だった。

本番では共用のFactory証拠とPoolの承認履歴を再利用し、初回に上限で終わらなければ未判定を返す方針を維持する。
調査を分割した実行方式を、同じ取得対象の予算や再試行回数をリセットする実装にしない。

sandbox内では接続エラーが初回＋3回発生した。これは外部通信可能な環境での58回とは別に[rpc-evidence-sandbox.json](rpc-evidence-sandbox.json)へ記録した。
合計の呼出し試行は62回、外部応答を取得した送信は58回。環境を切り替えた後の取得にHTTP / JSON-RPCエラーはなかった。
既存ソースとローカルcompilerを再利用し、新しいソース取得やcompilerのdownloadは行っていない。公式仕様のWeb閲覧はRPC数に含めない。

## 再現手順と実行結果

`probe.py`は固定のpublic Base endpoint、読取りmethodのallowlist、永続化した最大4試行、全体128試行で動作する。
成功した同一要求は再取得しない。保存済み失敗の回数も引き継ぐ。1回の起動は取得開始判定100秒＋通信timeout20秒で制限し、待機と通信timeoutが残り120秒の予算に入る場合だけ次の取得を開始する。
保存済み証跡を維持したまま実行するとcache確認になる。新しいネットワーク検証を行う場合は、別の出力ファイルへ記録して両方の消費を報告する。

onchain rootから:

```sh
python3 experiments/lp_operator_proof_20260923/probe.py experiments/lp_operator_proof_20260923/plan.json experiments/lp_operator_proof_20260923/rpc-evidence.json
node experiments/lp_operator_proof_20260923/compile.cjs /private/tmp/lp-lock-solc-0830/node_modules/solc go/internal/experiments/lplock/testdata/venue_samples_20260921_30/sources/dependency-source-0x932d0b4c00a2a33ef1ec5fe0aa981bd1a00a6f5c.json.gz experiments/lp_operator_proof_20260923/factory-compiled.json
```

compilerの第1引数は検証環境に存在したsolc 0.8.30の場所。別環境では同じ版のローカル配置を指定する。
Node / compilerは調査用であり、本番依存には追加していない。

onchain/goから:

```sh
go test -tags lp_operator_research ../experiments/lp_operator_proof_20260923/proof_test.go ../experiments/lp_operator_proof_20260923/reader_test.go -count=1
go vet -tags lp_operator_research ../experiments/lp_operator_proof_20260923/proof_test.go ../experiments/lp_operator_proof_20260923/reader_test.go
ONCHAIN_LP_OPERATOR_RESEARCH_LIVE=1 go test -tags lp_operator_research ../experiments/lp_operator_proof_20260923/proof_test.go ../experiments/lp_operator_proof_20260923/reader_test.go -run TestProtectedReader -count=1 -v
```

- 保存済み作成根拠の検証・同salt再作成の拒否: PASS。
- creation / runtimeの独立再コンパイル: PASS。
- 実RPCでの割合再計算: PASS。
- 調査Goコードのgofmt / vet: PASS。
- `GOWORK=off go test -tags lp_operator_research ...`: PASS。onchain自身のgo-ethereum v1.17.3でも保存済み証跡の検証が通過。sandboxのGo cacheアクセス拒否後、許可された環境で実行した。
- Python構文・文書リンク・空白確認: PASS。

Goファイルは`lp_operator_research` build tag付きでgo moduleの外に配置し、明示指定でのみ実行する。通常SDKのbuild / testから読み込まれない。
productionコードを変更していないため、サービスbuild・通常の全体テスト・Agent E2Eは今回の対象外。commit / pushは行っていない。

## 次に実装できる範囲

1. この生成経路の検証を限定ルールとしてonchainへ実装する。creation / runtime照合、CreateX経路、FactoryのCREATE nonce、receiptの関連を含め、任意の開始ブロックや自己申告の完了フラグで代用しない。
2. Factory証拠と保管先別承認履歴を保存・再利用する型を確定する。cold時の2分上限、reorg、保存失敗、取得打切りを明示する。
3. 別の保管先でも同じルールが成立することを検証してからMarketHubへ接続する。MultiVault / V3 lockerや他の作成経路は今回未検証。

この1例は実装を進める根拠になるが、任意Poolの完全自動解析や常時低RPCを確認したものではない。
UNCX APIや外部のlock割合は使用していない。
