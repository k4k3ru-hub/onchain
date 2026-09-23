# Base V4：LaunchLocker作成・承認証拠の先行検証

2026-09-23。承認済み範囲はBase mainnet・Hookなし・公式PositionManager。
今回は、長い承認履歴を全走査せずに永久保管を確認する限定ルールの成立性を検証した。
製品コード・公開API・依存・DDLは変更していない。調査コードはGo module外に隔離した。

## 結果

**固定サンプル1 Poolで、作成コード・実行コード・承認経路・全持分の照合を組み合わせた限定判定が成立した。**
既知の限定モデルと作成取引候補を用意した状態からのRPC取得は18回、45.530秒。
作成証拠を再利用した同一観測点の再読取りは9回、22.759秒だった。
どちらも128 RPC・2分・応答2 MiB以内。ただし、ソース候補取得・コンパイル・人手によるモデルレビューは、この時間に含めていない。
任意Poolからの自動ソース発見・自動モデル認識まで完了した結果ではない。

観測位置は過去調査と同じ **block 51,604,306 / 2026-09-21 13:39:19 UTC**。
今回そのブロックの状態を実RPCで再取得した。現在headの状態や、現在の掲載対象数を測定したものではない。
以前の137 Pool／128 Pool全件を、新しい基準で合格とした結果でもない。

| 項目 | 確認結果 |
| --- | --- |
| Pool | `0x2f20eec5945b32624fb6ccb8ba8716aabea99bc7d8e8590b5a863c283042601a` |
| PoolManager | `0x498581ff718922c3f8e6a244956af099b2652b2b` |
| PositionManager | `0x7c5f5a4bbd8fd63184577525326123b519429bdc` |
| NFT | `3073314`、正の持分1件 |
| 保管先 | `0xcd1680d26922fcd9cabfbb8a56ba40c333fd842a` |
| Hook | zero address |
| tick | lower `-887200`、upper `184400`、観測tick `184400` |
| 全持分 | 全36 bitmap word・全2 tickのgross/netと完全一致 |
| active liquidity | `0`。範囲外の持分も元本として集計した |
| Token0元本 | native ETH、base units `0` |
| Token1元本 | `0x3ee1a9806f4bf62bdd753b9907878f7f85c637f7`、base units `999999999999999999999994872`（切捨て） |
| 永久保護割合 | Token0は分母0のため`null`、Token1は`"100"` |
| 全持分保護 | 作成・権限証拠と併用した限定判定として`true` |
| 変更・解除権限 | レビュー済みのこの実装には元本解除へ至る経路を確認しなかった |

LP持分が永久保管されていても、この観測位置にはETH元本がない。売却の出口やToken自体の安全性を保証しない。
LP保護・元本構成・Token権限を別項目として返す設計を維持する。

## Pool作成通知から辿れた経路

作成receipt `0xfdbeb0af97ed9f7f0a5bf2a5e85a1affa3f7c9039bf013b0b9977b285d159d28`を取得し、次を照合した。

1. PoolManagerの`Initialize`でPool ID・Hookなしを確認。
2. 同receiptの`ModifyLiquidity`でsenderが公式PositionManager、saltがNFT ID `3073314`と確認。
3. PositionManagerのmint `Transfer`で、そのNFTの保管先を発見。
4. 固定観測hashでPoolKey・tick範囲・NFT liquidity・core liquidity・owner・getApprovedを照合。
5. 全tick gross/netを照合して、未確認の正の持分が残らないことを確認。

Pool IDはPoolKeyからKeccakで再計算した。PoolManagerのアドレスだけでPoolを識別していない。
旧調査の`tokenIdOf(token)`経由だけの発見から進み、今回は実receiptに発見経路を結び付けた。
ただし、調査入力には既知のサンプル・作成候補を使用している。探索一般化は本実装工程に残る。

## 承認履歴を省略できる限定根拠

通常の`getApproved=0`、runtime一致、任意署名1件のrevertだけでは判定しない。
今回の証明は、次の前提を全て必要とする。

### 初回作成

Sourcifyの候補を実transaction・receipt・blockで検証した。
`0xf58f481f38a00ec6db604d14a9b79c711fa4aac74dc4577072376eeecdac8a60`は、
EOA `0x63f300b7b21fba52a8d874d90f7e1b301ae858c4`のnonce 0・to=nullの署名付き作成取引。
作成先はLockerではなくFactory `0x815542e8b392389a1389e22e588e4b62a67ade72`だった。

Factory constructorがCREATE nonce 1でLockerを作成する。
署名からsenderを復元し、両CREATE addressを再計算した。
作成入力全体を、独立再コンパイルしたFactoryコード＋3個のconstructor引数へ一致させた。
ローカルEVMの再現では、Factory CREATEとLocker CREATEだけが発生し、外部CALL／STATICCALLは0。
生成した両runtimeが作成block末尾・観測block末尾の実コードと完全一致した。

したがって、この作成コードがconstructor内でPositionManagerへ包括承認を設定した可能性を除外できる。
first CREATEの特定には、通常の署名安全性とアドレス衝突困難性を前提とする。
将来の実装で任意Factory・再作成可能な仕組みへこの推論を広げない。

### 作成後の全権限経路（限定ソースレビュー）

| 経路 | このコードの確認内容 |
| --- | --- |
| NFT transfer／approve／setApprovalForAll | Lockerの到達可能な外部呼出しに存在しない |
| constructorでの承認 | 作成入力照合＋実行再現で外部呼出し0 |
| permit／permitForAll | 実PMはcontract ownerの場合ERC-1271を要求。Lockerに該当selectorとfallbackがなく拒否される |
| 作成前・constructor中の署名 | codeが空の期間はEOA署名検証が必要。このCREATEアドレスの秘密鍵を得られないという通常の暗号前提。constructorから署名・承認処理への呼出しもない |
| collect／collectMany | 固定のDECREASE_LIQUIDITY＋TAKE_PAIR。減少量は常に0。元本を減らす可変引数がない |
| register | immutable factoryのみ。NFT所有確認、Token／quote／配当先の登録に限定される |
| claim／claimFor | 手数料のcreditを同じ受取人へ払い出す。NFT承認へ変換する経路がない |
| 外部Token／配当先呼出し | ERC20 balanceOf／transferまたは空calldataのnative送金。PMの権限変更selectorへの衝突なし |
| 再入 | collect／claimはReentrancyGuard、registerはfactoryのみ。再入で公開関数を呼んでも引き出し・任意calldata実行経路を作れない |
| コード差替え | このLockerにproxy／upgrade／delegatecall／selfdestructの実装がない |

公式PMを既知protocolとして扱い、実runtimeと再コンパイル済みソースを照合した。
PMの包括承認変更は所有者自身のsetApprovalForAllまたは有効なpermitForAllを通る。
直接承認・permit・移転時の承認消去もソース上で確認した。
個別承認の観測値は0。第三者が包括承認を得る初期経路と将来経路が、この限定モデルでは閉じている。

これはレビュー済みコードへの限定的な帰納的根拠であり、任意Solidityの形式検証器ではない。
有限件のテストだけで全入力の安全性を証明したとは扱わない。
名前がLaunchLockerでも、コード・constructor・immutable・生成経路が異なれば対象外。
UNCX・locker事前allowlist・外部の安全判定APIは使っていない。

### 反例テスト

同じLocker runtimeを返すが、constructor内で実PMへ`setApprovalForAll`する偽作成コードをローカルEVMで実行した。
実際に包括承認が保存されることをgetterで確認した。
runtime比較だけなら一致するが、今回のconstructor照合は拒否する。
この反例により、runtime照合だけで永久保護へ進めないことを確認した。

署名長0／64／65／96のpermitForAllは、実PMからLockerのERC-1271へ進んで拒否されることを再現した。
実Lockerのcollectを、managerの注入fixtureで実行し、NFT IDとliquidity減少量0を確認した。

## RPC・保存量

実測は[metrics.json](metrics.json)、各応答と試行は`*-rpc.json`へ保存した。
全state readはEIP-1898のblockHash＋requireCanonicalで固定。
作成・Pool作成・観測headerを取得後に再確認した。

| 測定 | 実送信RPC | 内訳 | Multicall内read | 応答bytes | 経過 |
| --- | ---: | --- | ---: | ---: | ---: |
| 事前調査 | 5 | chain 1、tx 1、receipt 1、header 2 | 0 | 77,125 | 12.866秒 |
| 初回証拠取得 | 18 | chain 1、tx 1、receipt 2、header 6、code 5、eth_call 3 | 49 | 306,029 | 45.530秒 |
| 作成証拠再利用 | 9 | chain 1、header 2、code 3、eth_call 3 | 49 | 140,561 | 22.759秒 |

- 実ネットワークRPCは合計32回。事前調査開始時にsandbox DNS失敗が1回あり、台帳上は合計33試行。冷温測定内のretryは0。
- coldはRPC応答cacheを使わず取得。warmは作成transaction／receipt／作成時codeなどを共有し、同じ観測blockの可変stateを再取得した。warm collector自身のcache hitは0。
- 49 readはNFT／binding／slot0等11、bitmap36、tick2。全bitmap・tickの再読取りもこの数に含む。
- 製品では既存の同一観測hashのLP状態を再利用する。専用に全bitmapを毎回読み直す方針ではない。
- wire RPC回数と料金計算上のcompute unitは別。Multicallがprovider料金も49分の1にするとは解釈しない。
- 初回RPC証跡JSONは360,204 bytes、compact JSONは336,683 bytes。両方512 KiB内だが、将来の製品Evidence形式の実測値ではない。
- ソースとcompiler成果物はモデル共通の調査資産。Pool別保存の512 KiBへ複製する設計にはしない。
- 作成候補のSourcify HTTP取得1回成功とcompiler binary取得1回成功はRPC回数に含めない。Sourcify取得前にもsandbox DNS失敗が1回あった。HTTP取得と独立compileの経過時間は今回のRPC計測に含めていない。
- 既存の1,000 block／通常4 RPCの履歴方式なら、このLockerの664,177 blockに665区間・約2,660 RPCを要する見積もり。今回は限定した作成・権限証明を使うため、承認ログ全走査を実行しなかった。

128 RPCに近い複数NFT Pool・混在保管先・別Poolでの共有効率は、未実測。
初回2分の本番deadlineには候補取得等も含め、完了できなければ未判定にする必要がある。

## 再現とチェック

既存go moduleのgo-ethereumだけを利用。新しいproduction依存は追加していない。

```sh
# onchain/go
GOWORK=off go build -o /private/tmp/lp-v4-keccak ../experiments/lp_v4_proof_20260923/keccak.go
GOWORK=off go test -tags lp_v4_research -v ../experiments/lp_v4_proof_20260923/proof_test.go
GOWORK=off go vet -tags lp_v4_research ../experiments/lp_v4_proof_20260923/proof_test.go
GOWORK=off go vet ../experiments/lp_v4_proof_20260923/keccak.go

# onchain
python3 -B -m unittest discover -s experiments/lp_v4_proof_20260923 -p 'test_*.py' -v
python3 -B experiments/lp_v4_proof_20260923/prepare.py
```

Goは6件（署名長の4 subtestを含む）、Pythonは3件成功。gofmt・個別go vet成功。
初回vetは異なるpackageの2ファイルを一緒に指定したため失敗し、上記の個別指定へ修正した。
製品変更がないため全module test／service build／Agent E2Eは今回対象外。

独立compileは公式Linux amd64 native solc `0.8.26+commit.8a97fa7a`をSHA-256検証後に使用。
期待値は`d5f23436f443edb85d8e76906d12f0a86ce0490e7663a9e608efeb7a93f149ef`。
Factory・Locker・PMのcreation/runtimeが保存された対応ソースのコンパイル結果と一致した。
compiler本体は一時ディレクトリのみ。`compilation.py prepare <tmpdir>`で標準JSON入力を再生成できる。
出力名を`factory-output.json`／`manager-output.json`として、`compilation.py verify <tmpdir>`で成果物を照合する。

実際のコンパイルコマンド（入力名・出力名をFactoryとPMで切替）：

```sh
docker run --rm --network none --read-only --cap-drop ALL \
  --security-opt no-new-privileges --memory 1g --cpus 1 --pids-limit 64 \
  --platform linux/amd64 -v /private/tmp/lp-v4-compile:/work:ro \
  --entrypoint /work/solc -i debian:bookworm-slim --standard-json \
  < /private/tmp/lp-v4-compile/factory-input.json \
  > /private/tmp/lp-v4-compile/factory-output.json
```

RPC再収集は`probe.py <plan.json> <new-output.json>`で実行する。同じoutputを指定すると成功結果を再利用し、失敗4回の取得対象は再開しない。
任意トランザクション送信はallowlist外。署名・ガス消費・サービス再起動は行っていない。
全ての研究資産はこのディレクトリにあり、製品からimportしない。

## 本実装レビューへ渡す結論

1. 新しい限定証拠種別を追加する案：初回作成入力の一致＋生成経路の一意性＋runtime一致＋レビュー済み権限閉包。
2. 既存V3／Slipstreamの承認履歴要件は維持する。今回の根拠が成立するV4モデルにだけ新種別を適用する。
3. protocolのPM、Pool ID、core position salt、永久保護の集計をonchainへ実装し、既存APIには追加型で接続する。
4. 原本ソース／コンパイルはモデル検証用。既知モデルの本番判定は照合済みfingerprintを利用し、Poolごとの再コンパイルを要求しない。
5. EOAの実Poolでの全量減少eth_call、複数持分、未確認持分、Hookあり、reorg・stale・予算超過は本実装テストへ追加する。EOA実Poolの成功例は今回は未確認。
6. MarketHubは既存LP状態・receipt・作成証拠を優先利用し、未知モデルはnull・掲載継続。最後にAgentで実E2Eする。

実装前レビューの中心は、**この限定モデルで承認履歴全走査を置き換える証拠種別を採用するか**。
ここで得た結果だけで製品の証拠要件を変更してはいない。

## 一次資料

- [公式V4 deployments](https://developers.uniswap.org/docs/protocols/v4/deployments)
- [ERC721Permit_v4の仕様参照](https://raw.githubusercontent.com/Uniswap/v4-periphery/main/src/base/ERC721Permit_v4.sol)
- [署名検証ライブラリの仕様参照](https://raw.githubusercontent.com/Uniswap/permit2/main/src/libraries/SignatureVerification.sol)
- [EIP-6780](https://eips.ethereum.org/EIPS/eip-6780)
- [Sourcify Factory公開ソース](https://sourcify.dev/server/v2/contract/8453/0x815542e8b392389a1389e22e588e4b62a67ade72?fields=all)

仕様参照のmainソースをdeployment一致の証拠にはしていない。実deploymentのSourcify応答・コンパイル成果物と実RPCコードで照合した。
公開ソースにはMIT／GPL等の元のライセンス表記を保持している。
