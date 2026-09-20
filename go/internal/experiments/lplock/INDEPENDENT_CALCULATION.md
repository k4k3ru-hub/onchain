# UNCX非依存の算出と比較検証

追記: 後続の[現設定割合の検証](CURRENT_RATIO.md)では、構造解析・RPC照合・手動ソースレビューを組み合わせ、この1例で独立側100%の算出と比較を完了した。以下は、値の抽出までを実装した前段の記録として保持する。

2026-09-20。公開ソース・ABIの汎用解析を含む検証についてユーザー承認を受け、実行した。

**専用getter・固定の保管先・実装hashの事前登録なしで、期限・所有者・例外経路の設定値を取得できた。比較用UNCXの同一blockの値と5項目が一致した。**
**ただし、lock割合の独立算出は未達。汎用解析PoCは全引出経路を証明する段階まで実装しておらず、割合は`null`としている。**
これは汎用解析が不可能という結論ではなく、この検証で確認できた範囲の上限である。

証拠・解析計画・比較結果: [testdata/independent_analysis_20260920.json](testdata/independent_analysis_20260920.json)。

## 要件

- 算出側はPool作成通知を入口にし、オンチェーンの持分・所有関係・引出条件から独立して割合を求める。
- UNCXは比較用にのみ利用する。UNCXから得たlock ID、lock判定、期限、割合を独立算出側の入力にしない。
- UNCX専用のABI・runtime hashへの一致・固定された引出関数名を使う既存PoCは、独立算出の証明にならない。
- 算出結果と比較値は、同じPool・block・元本範囲・権限の解釈で比較する。

## 確認済みと未確認

| 処理 | 現状 |
| --- | --- |
| Poolの全持分から元本量を取得 | 既存`evm/clliquidity`でUNCX非依存に取得できる |
| PoolイベントからNFTと現在の所有先を発見 | Uniswap V3／ERC-721の情報で取得できる |
| 公開ソースからNFT移転・時刻条件・保存位置を特定 | 構文木とstorage layoutから抽出できた |
| 期限・所有者・例外経路設定の取得とUNCX比較 | 専用getterを使わず取得し、同一blockで5項目一致 |
| 任意の保管先の全引出経路をUNCX専用処理なしで判定 | このPoCでは未実装 |
| 独立算出したlock割合とUNCX側の値を比較 | 未達。独立側`null`、参照側`100%` |
| 既存UNCX PoCの100% | 比較用の参照値。独立算出の結果とは扱わない |

## 標準仕様を確認した結果

[ERC-721](https://eips.ethereum.org/EIPS/eip-721)は所有者・承認・移転等を定めるが、
NFTの保管先に対する解除期限・管理者の例外経路を取得する標準APIは定めていない。
[Uniswap V3のNFT manager](https://github.com/Uniswap/v3-periphery/blob/main/contracts/interfaces/INonfungiblePositionManager.sol)と
[Pool状態](https://github.com/Uniswap/v3-core/blob/main/contracts/interfaces/pool/IUniswapV3PoolState.sol)にも、
外部保管先のロックを説明する共通項目はない。

この仕様から、LP量とコントラクト保有を確認しただけでは、期限付きlock・いつでも解除可能なvault・その他の保管を区別できないと判断する。
「コントラクトが保有するLPの割合」を「lockedLiquidityPercentage」として返すことはしない。
また、一つの呼出がrevertしても、別の引出経路が存在しないことの証明にはならない。

## 独立側の処理

1. Go SDKで作成イベント・receipt・Pool addressを照合し、LP NFTと現在の保管先を発見する。
2. 既存`clliquidity`で全tickのgross／net・NFT・core持分を照合する。この実例では単一NFTが全持分を説明できた。
3. 発見したaddressを使ってSourcifyからソース・ABIを取得する。保管先の名前・address・code hashの許可リストは使わない。
4. Sourcifyの検証対象runtimeと実RPCのcodeを比較する。取得したcompiler versionとsettingsで再コンパイルし、recompiled bytecodeも照合する。
5. 構文木の参照IDから、ERC-721の移転・承認、およびUniswap V3のliquidity減額に関係する関数を抽出する。
6. NFT移転で使うmanager／token IDがどのmappingの構造体に由来するか、時刻条件がどのfieldを参照するかを追う。
7. compilerのstorage layoutからmappingのslot・field位置・詰め込みoffsetを導出する。
8. 保管先が出したイベントを取得ABIでデコードし、整数値9候補からmapping keyを探索する。保存されたNFT IDとmanagerが一致するものを採用する。
9. `eth_getStorageAt`で期限・所有者・例外経路の設定値・設定変更者のaddressを直接取得する。
10. 独立結果を保存した後、同じblockを指定して別のUNCX専用テストを実行し、比較する。

独立側で固定しているselectorはERC-721／Uniswap V3の操作だけ。
`getLock`、lockイベント名、保管先のstorage slot、期限field名、レコードIDなどは固定していない。
ソースから取得した名称が解析結果に表示されることはあるが、サービス識別の条件には使わない。
公開ソース取得・再コンパイルはオフチェーン処理であり、オンチェーンRPCだけで完結する手法ではない。

## 比較結果

- Pool: `0xd67f187eb06C51D3Ed02C73A14F82Ef36FeA4665`
- block: `51557927`
- hash: `0xa817722482859dcd437a0e12c020e99401d0282685af33b40c6165c5f0f60382`
- 入力は以前のPool作成イベントの再生。未知のPoolからの無作為な成功例ではない。

| 比較項目 | 独立側の取得結果 | UNCX専用処理との比較 |
| --- | --- | --- |
| レコードID | `1158` | 一致 |
| 時刻制約の値 | `1791648509`（2026-10-10 16:08:29 UTC） | unlock日時と一致 |
| レコード所有者 | `0x7618283E4D27C1CdCA572B791B499e4DE9d9954A` | 一致 |
| 別経路の設定先 | zero address | MIGRATORと一致 |
| 設定変更者 | `0x31c44A17aa2E639B40f33DA805CB1DB55d969693` | 管理者と一致 |
| lock割合 | **未確定** | 参照側は100%。割合の一致検証には到達していない |

独立取得では、5件のasset操作に関係するcall、期限fieldを持つレコード構造1件、
NFT承認を通じた別経路の設定変数1件とその変更関数を検出した。
現在の時刻条件は成立せず、別経路の設定先は0であることを読み取った。
比較結果を独立側の未確定値の穴埋めに使わない。独立結果のSHA-256も比較記録に保存した。

## なぜ割合を確定しなかったか

今回実装したのは、構文木から条件と状態の依存関係を抽出する処理である。
例えば`if (条件) { require(期限条件); }`では、期限条件を通らない分岐が存在し得る。
そのため、同じ関数に期限チェックとNFT移転があるだけで「期限前は必ず移転不能」と判定しない。

以下はこのPoCに未実装であり、残る実装・検証課題である。

- 全分岐で期限・認可チェックを必ず通過することの確認。
- 内部関数・library・外部call・callbackを横断する資産移動の解析。
- 期限・所有関係・承認・移行設定を変更する経路の到達可能性の評価。
- proxyや更新可能な実装、複数NFT、後日のlock追加、履歴不足への対応。

現在の解析結果は常に割合を未確定に保つ設計であり、完全な汎用lock判定器ではない。
特に、単に移転がrevertしたこと、未来のtimestampが保存されていること、コントラクトがNFTを持つことを100%の根拠にしていない。
参照側の100%は、そのUNCX実装の既知の条件を評価した結果なので、独立側の上記未実装を補完する証拠にはならない。

## RPC使用量

| 段階 | 再試行を除くJSON-RPC件数 | HTTP RPC試行 | 別途ソースGET | 所要時間 |
| --- | ---: | ---: | ---: | ---: |
| 独立側・Poolからの探索／全持分／ソース照合 | 13 | 14（429再試行1回） | 1 | 21.16秒 |
| 独立側・storageから値の取得 | 16 | 5（storage 14件を3 batch＋header 2回） | 0 | 6.39秒 |
| 合計・独立側 | **29** | **19** | **1** | **約27.55秒＋ローカル解析時間** |
| 比較用UNCX・最終実行 | 23 | 24（429再試行1回） | 1 | 35.80秒 |

初回の比較用テストは所有者・管理者の値を出力していなかったため、出力を追加して再実行した。
初回分として別途26 HTTP RPC試行（再試行3回）とソースGET 1回を消費した。
公開RPC向けの1.5秒間隔を含む計測であり、運用時の最低コストや料金ではない。
compiler取得のnpm通信・Web仕様確認はRPC件数に含めていない。

## ファイルと検証

- [independent_discovery_test.go](independent_discovery_test.go): Pool起点の独立探索・全持分照合・ソース取得。
- [compile_source.cjs](compile_source.cjs): 指定compilerで構文木とstorage layoutを生成し、再コンパイルしたruntimeを照合。
- [analyze_source.py](analyze_source.py): サービス固有名に依存しない構造解析。
- [independent_storage_test.go](independent_storage_test.go): 解析から導出した保存位置の実RPC読取り。
- [compare_results.py](compare_results.py): 同じblockの独立結果と参照結果の比較。
- [test_analyze_source.py](test_analyze_source.py)、[checks_test.go](checks_test.go): 別名の合成コントラクト、packed field、条件付きチェック、異なるblockの比較拒否等のテスト。
- [uncx_test.go](uncx_test.go): 比較専用。今回、block固定と比較用の出力項目を追加した。

compilerは`/tmp/onchain-lock-analysis-solc`へ取得した検証用の`solc@0.8.19`。
Goモジュール・本番依存・公開SDK APIは変更していない。
取得した第三者のソース全文・compiler生成物は`/tmp`に置き、リポジトリには再配布しない。

`onchain`から:

```sh
node go/internal/experiments/lplock/compile_source.cjs /tmp/onchain-lock-analysis-solc/node_modules/solc /tmp/onchain-independent-lock/source.json /tmp/onchain-independent-lock/compiled.json
python3 go/internal/experiments/lplock/analyze_source.py /tmp/onchain-independent-lock
python3 go/internal/experiments/lplock/compare_results.py /tmp/onchain-independent-lock
ONCHAIN_SOLC_MODULE=/tmp/onchain-lock-analysis-solc/node_modules/solc python3 -m unittest discover -s go/internal/experiments/lplock -p 'test_analyze_source.py' -v
```

`onchain/go`から、探索→ローカルcompile／解析→storage読取り→参照→比較の順で実行:

```sh
ONCHAIN_INDEPENDENT_DISCOVERY=1 ONCHAIN_ANALYSIS_DIR=/tmp/onchain-independent-lock go test ./internal/experiments/lplock -run '^TestIndependentDiscoveryLive$' -count=1 -v
ONCHAIN_INDEPENDENT_STORAGE=1 ONCHAIN_ANALYSIS_DIR=/tmp/onchain-independent-lock go test ./internal/experiments/lplock -run '^TestIndependentStorageLive$' -count=1 -v
ONCHAIN_UNCX_LIVE=1 ONCHAIN_REFERENCE_BLOCK=51557927 ONCHAIN_UNCX_REPORT=/tmp/onchain-independent-lock/reference-result.json go test ./internal/experiments/lplock -run '^TestUNCXPoolFirstLive$' -count=1 -v
go test ./internal/experiments/lplock ./evm/clliquidity -count=1
go vet ./internal/experiments/lplock
```

参照blockは再実行で新たに生成された`discovery.json`の値を指定する。
Goの単体テスト4件・Pythonのテスト6件・実RPCの3段階はPASS。ここでPASSは読取り・照合等の成功を指し、lock割合の独立算出成功を意味しない。
`gofmt`と対象packageの`go vet`も確認。本番APIへの組込みとAgent経由のlock割合E2Eは未実施。

storage位置の計算は[Solidityのstorage layout仕様](https://docs.soliditylang.org/en/latest/internals/layout_in_storage.html)、
構文木の生成は[Solidity compilerのJSON入出力](https://docs.soliditylang.org/en/latest/using-the-compiler.html)、
公開ソースの取得は[Sourcify API](https://docs.sourcify.dev/docs/api/)に基づく。
