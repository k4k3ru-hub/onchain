# Poolを入口にした現設定のロック割合算出

2026-09-20。**対象の1例について、UNCX専用処理を使わない算出側で100%を得た。独立結果を保存した後、同一ブロックのUNCX専用getterを使う比較処理も100%となり、元本量と取得項目も一致した。**

公開ソースの構造解析・実RPCの照合に加えて、残る呼出経路の**手動ソースレビューを含む実現性検証**である。
任意の保管先を完全自動で判定できるSDKの完成を意味しない。

- 全結果・使用量・レビュー記録: [current_ratio_20260920.json](testdata/current_ratio_20260920.json)
- 前段の「値の抽出まで」の検証: [INDEPENDENT_CALCULATION.md](INDEPENDENT_CALCULATION.md)
- 算出処理: [current_state.py](current_state.py)
- RPCによる照合: [current_state_test.go](current_state_test.go)

## 結果

| 項目 | 値 |
| --- | --- |
| Chain / Network / Venue | Base / mainnet / Uniswap V3 |
| Pool | `0xd67f187eb06C51D3Ed02C73A14F82Ef36FeA4665` |
| 観測ブロック | `51559656` |
| 観測時刻 | 2026-09-20 12:50:59 UTC / 21:50:59 JST |
| ブロックhash | `0xe952ab612e84f8be7fc15b35139adc8a7e0b6b4598c302af116582afec7b55c5` |
| 独立側の現設定ロック割合 | **100%** |
| 比較専用処理の割合 | **100%** |
| Pool元本・token0 base units | `69651978657561984` |
| Pool元本・token1 base units | `243768074` |
| 現設定で拘束される元本 | 上記両トークンの全量 |
| 解除期限 | `1791648509` / 2026-10-10 16:08:29 UTC |
| 設定変更権限 | あり。割合とは別に保持 |

ここでの割合は、**観測ブロックの設定を維持した場合の、Pool内LP元本に占める拘束元本の割合**。
未回収手数料・直接送金された余剰残高は分母に含めない。
設定変更を伴う将来の移行、期限までの変更不能性、Poolやトークンの分散性はこの割合の意味に含めない。

## 入力と独立性

入力は [pool_created.json](testdata/pool_created.json) のPool作成イベントと、標準のUniswap V3 factory / NFT manager設定。
過去のNewPairイベントを再生したもので、未知Poolからの無作為サンプルではない。

1. 作成receiptとfactoryからPoolを照合し、LP NFTを発見する。
2. NFT / core position / 全tickのgross・netを照合する。この例では単一NFTがPool全元本を説明する。
3. NFTの現在の所有先をRPCで取得し、そのアドレスの公開ソース・ABIをSourcifyから取得する。
4. ソースをローカルでコンパイルする。RPCで照合済みのruntimeと、compiler出力をimmutable挿入領域以外の全バイトで比較する。繰り返されるimmutable値も一致を確認する。
5. ERC-721移転・承認とV3元本減額の呼出を起点に、保管先のレコード構造と制約を抽出する。
6. 発見したイベント内の整数候補からレコードを照合し、storage layoutから期限・所有者・設定値を読む。
7. 現設定での分岐と失敗する必須条件を評価し、ソースから生成したABI呼出を同一ブロックで照合する。
8. 残る呼出・storage更新・再入・承認経路を手動で確認し、取得ソースと解析計画に紐付けたレビュー記録を保存する。
9. 確認した持分の元本を集計し、独立側の `current-result.json` を保存する。
10. その後に比較用UNCX処理を実行する。

独立側は、UNCXの専用getter・既知の保管先アドレス・実装hashの事前登録・固定されたロックIDや期限・比較結果を入力にしない。
解析に現れる保管先の関数名は、取得ソースから観測した名称であり、サービス識別による分岐には使っていない。
手動レビュー記録のhashは**発見後に検査したソースの同一性を保持するもの**であり、事前の許可リストではない。

Sourcifyは公開ソースの取得に利用している。外部サービスを一切使用しない方式ではない。
UNCXのロック判定APIから割合を受け取った結果でもない。

独立結果のSHA-256: `f2f67cbb11bf5c724ab43744ebfcbf9287e90d6c6858fd59d79d0fb567127af8`。
比較では割合に加えて、レコードID・期限・所有者・移行設定・設定変更者・両トークン元本の7項目が一致した。

## 出金条件の確認

| 経路 | 独立側の確認 |
| --- | --- |
| レコードに紐付くNFT移転 | 分岐のどちらも現在は通過不能。RPCはソースの期限条件と一致する `NOT YET` |
| レコードに紐付く元本減額 | 同様に期限前。RPCは `NOT YET` |
| 別経路へのNFT承認 | 必須の設定先がzero address。RPCはソースの条件と一致する `NOT SET` |
| 新規預入後の手数料用元本減額 | 同じmanager / NFTを呼出者から受け取る処理が先行する。保管済みNFTではfromが現在の所有者と一致せず、実managerの呼出も拒否 |
| NFT managerへ直接元本減額 | ロック所有者による呼出は `Not approved` |
| ERC20返金helperのselectorをNFT managerへ送る | 保管先を送信元とした呼出も空のrevert。ERC721移転とは異なるselector |

NFTの所有先と個別承認も再確認した。手動レビューでは、手数料回収・元本追加・内部helper・再入防止・レコード書換え・operator承認経路を確認した。
レビューの理由と限界は結果JSONの `source_review` に記録している。

「RPCがrevertした」という観測だけで100%と判定していない。
ただし、この構造解析と手動レビューは任意の外部コントラクトの脆弱性まで証明する形式検証ではない。

## 割合の計算

この例では、全tickとpositionの照合により対象NFTが全元本を持つことを確認できた。
対象持分が現設定で拘束されるため、各トークンについて以下になる。

- token0: `69651978657561984 / 69651978657561984 × 100 = 100%`
- token1: `243768074 / 243768074 × 100 = 100%`

両トークンで同率なので、USD価格や異なるレンジのraw liquidityを足す計算を使わず、全体も100%となる。
複数NFTや異なる拘束率の集計は今回の算出コードの対象外。未確認部分を0%や100%で埋めない。

## RPC使用量

正常完了した一連の実行を集計した。RPC件数はJSON-RPC操作数、HTTP回数はbatch送信と再試行を反映した通信回数。

| 段階 | RPC操作数・再試行除外 | HTTP RPC試行 | 429再試行 | ソース取得HTTP | 通信処理の実測秒 |
| --- | ---: | ---: | ---: | ---: | ---: |
| Pool・全持分・保管先の探索 | 13 | 13 | 0 | 1 | 18.57 |
| ソース由来のstorage取得 | 16 | 5 | 0 | 0 | 6.23 |
| 出金経路・runtime・所有関係の照合 | 11 | 13 | 2 | 0 | 18.20 |
| **独立算出合計** | **40** | **31** | **2** | **1** | **42.99** |
| 比較専用UNCX処理 | 23 | 24 | 1 | 1 | 35.87 |

- 独立側の再試行を含むJSON-RPC試行数は42。storage 14件は3回のbatchで送った。
- 上記時間は制限回避の1.5秒間隔を含む。ローカルcompile・解析・手動レビューの時間は含めない。
- sourceの取得・compiler処理はオンチェーンRPC数に含めない。プロバイダの課金単位とも同一ではない。
- 上記以外に、sandboxのDNS拒否で失敗した探索1回と、空revertのデコード修正前に失敗した出金検証1回がある。
  後者は失敗箇所まで通常経路10 RPC。初回版は失敗時の再試行数を保存していなかったため、正確なHTTP試行数は不明。
  この追加コストは上表から除外し、結果JSONの `rpc_failed_attempts` に区別している。

## 検証と再現

Goの対象packageテスト・vet、Pythonの16テストを実施。
否定例は期限切れ・有効な別経路・条件付き期限チェック・別レコードの期限参照・追加の無条件出金・チェック前の書換え・持分不足・異なるブロック・欠けたRPC証拠・比較値の不一致を含む。
成功した処理だけを返すためにネットワークエラーをrevert扱いにはしない。
Goの実験packageの単体テスト5件はPASS。公開RPCの探索・storage読取り・現設定照合・比較の4段階も最終実行はPASS。
追加で、保存した公開ソースのruntime命令1byteを改変した入力と、同じimmutableの片方だけを改変した入力が、compiler照合で拒否されることを確認した。

`onchain/go`から実行:

```sh
ONCHAIN_INDEPENDENT_DISCOVERY=1 ONCHAIN_ANALYSIS_DIR=/tmp/onchain-current-lock go test ./internal/experiments/lplock -run '^TestIndependentDiscoveryLive$' -count=1 -v
```

`onchain`からローカルcompileと構造抽出:

```sh
node go/internal/experiments/lplock/compile_source.cjs /tmp/onchain-lock-analysis-solc/node_modules/solc /tmp/onchain-current-lock/source.json /tmp/onchain-current-lock/compiled.json
python3 go/internal/experiments/lplock/analyze_source.py /tmp/onchain-current-lock
```

`onchain/go`からstorageを読む:

```sh
ONCHAIN_INDEPENDENT_STORAGE=1 ONCHAIN_ANALYSIS_DIR=/tmp/onchain-current-lock go test ./internal/experiments/lplock -run '^TestIndependentStorageLive$' -count=1 -v
```

`onchain`から現設定を評価:

```sh
python3 go/internal/experiments/lplock/current_state.py /tmp/onchain-current-lock
```

取得した公開ソースについて、結果JSONの `source_review` にある5種類の確認を実施し、
その時点の `source.json` / `current-plan.json` のhashを含む `source-review.json` を保存する。
異なるソースに今回のレビューを流用しない。手動レビューが欠ける場合、最終算出コマンドは失敗する。

`onchain/go`から出金を照合:

```sh
ONCHAIN_INDEPENDENT_CURRENT=1 ONCHAIN_ANALYSIS_DIR=/tmp/onchain-current-lock go test ./internal/experiments/lplock -run '^TestIndependentCurrentStateLive$' -count=1 -v
```

`onchain`から独立結果を確定:

```sh
python3 go/internal/experiments/lplock/current_state.py /tmp/onchain-current-lock --finish
```

この保存後にだけ、`onchain/go`から比較処理を実行する。blockは新しい `discovery.json` の値に置き換える。

```sh
ONCHAIN_UNCX_LIVE=1 ONCHAIN_REFERENCE_BLOCK=51559656 ONCHAIN_UNCX_REPORT=/tmp/onchain-current-lock/reference-result.json go test ./internal/experiments/lplock -run '^TestUNCXPoolFirstLive$' -count=1 -v
```

`onchain`から比較とPythonテスト:

```sh
python3 go/internal/experiments/lplock/compare_results.py /tmp/onchain-current-lock --independent current-result.json
ONCHAIN_SOLC_MODULE=/tmp/onchain-lock-analysis-solc/node_modules/solc python3 -m unittest discover -s go/internal/experiments/lplock -p 'test_*.py' -v
```

`onchain/go`からGo検証:

```sh
go test ./internal/experiments/lplock ./evm/clliquidity -count=1
go vet ./internal/experiments/lplock ./evm/clliquidity
```

署名・transaction送信は行っていない。実験コードのみで、MarketHubの公開APIやAgent経由のE2Eへの組込みは対象外。

## 今回変更したファイル

- 追加: `current_state.py`、`current_state_test.go`、`test_current_state.py`、このレポート、`testdata/current_ratio_20260920.json`。
- 更新: `independent_discovery_test.go`（NFTのliquidityを保存）、`compile_source.cjs`（実runtimeとの照合）、`compare_results.py`（割合と元本量の一致判定）、`test_analyze_source.py`（比較の否定例）、`README.md`、`INDEPENDENT_CALCULATION.md`。
- 検証範囲外: Go SDK全package、公開APIのbuild、Agent経由E2E、複数NFT・proxy・他Venue / 他Chainの実RPC。今回の1例からそれらへの適用可否は判断していない。

## 今後の評価項目

ユーザーと合意した評価軸は以下の4項目として保持する。

| 評価軸 | 今回 |
| --- | --- |
| LPの現設定ロック割合 | 100%を算出 |
| ロック設定の変更権限 | 管理者にあり。割合とは別項目 |
| 保有の集中度 | この検証では未評価 |
| トークン側の権限 | この検証では未評価 |

100%という数値を「完全に分散している」「安全なトークン」と解釈しない。
