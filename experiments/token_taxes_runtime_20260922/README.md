# TokenTaxes compiler実行環境の検証

2026-09-22。実装プラン工程1の確認用。この配備検証では正式SDK・MarketHub・依存・本番Dockerfileを変更していない。
その後、[onchainの正式な解析器](../../go/evm/erc20/tax/README.md)を実装した。正式runner・本番イメージへの組込みは後続工程。
保存済みの[TokenTaxes検証資料](../token_taxes_20260922/README.md)を再利用し、追加のオンチェーンRPCは **0回**。

## 結果

**固定native compilerを既存MarketHubイメージへ同梱し、Goから別プロセスで実行する方式が動作した。**
native版とNode.js版を合計24回実行し、生成artifactが以前の照合済みartifactと一致した。
制限・失敗・復旧の7項目、import制御の3項目、同梱イメージの2件もPASS。
[集計](evidence/summary.json)、[全比較](evidence/matrix.json)、[制限](evidence/controls.json)、[import制御](evidence/imports.json)、[イメージ確認](evidence/image-smoke.json)。

| 実行方式 | Token | compiler | コンパイル中央値 | 最大常駐メモリ |
| --- | --- | --- | --- | --- |
| native / arm64 | LMPT | 0.8.37 | 117.7 ms | 41.5 MiB |
| native / arm64 | TAOT | 0.8.34 | 91.9 ms | 20.3 MiB |
| Node.js / arm64 | WETH9 | 0.5.17 | 1,065.9 ms | 181.5 MiB |
| Node.js / arm64 | LMPT | 0.8.37 | 1,339.9 ms | 265.2 MiB |
| Node.js / arm64 | TAOT | 0.8.34 | 1,236.3 ms | 286.8 MiB |
| native / amd64エミュレーション | WETH9 | 0.5.17 | 118.4 ms | 14.9 MiB |
| native / amd64エミュレーション | LMPT | 0.8.37 | 337.8 ms | 54.1 MiB |
| native / amd64エミュレーション | TAOT | 0.8.34 | 339.1 ms | 40.4 MiB |

各組合せ3回。毎回新しいcompilerプロセスを起動。時間はcompiler子プロセスの開始から終了までで、
ソースHTTP・RPC・compilerダウンロード・workerキュー待ち・Docker起動・結果のJSON再解析は含まない。
RSSは子プロセスのLinux `getrusage`最大値。別欄の`containerWallMs`はDocker起動等を含む。
親Goプロセス・MarketHub・OSを含めた全体のメモリ量ではない。
Node.jsはキャッシュ済みimageのv22.23.2、solc wrapper 0.8.30、公式固定compiler 3版を使用。

ホストはApple SiliconのDocker Desktop、Linux VMはarm64。
amd64の値は同VM上のエミュレーションであり、実amd64本番機の性能値ではない。
既存サービス等が動く開発機上の小規模比較で、任意のソースや高負荷時の性能保証・掲載対象カバー率は測っていない。

## 対応版と配布物

[Solidity公式の配布方法](https://docs.soliditylang.org/en/latest/installing-solidity.html#static-binaries)と
[公式solc-bin](https://github.com/argotorg/solc-bin)を確認した。

| compiler | linux-amd64 | linux-arm64 |
| --- | --- | --- |
| 0.5.17+commit.d19bba13 | 配布あり・実行確認 | 今回取得した公式manifestに該当版なし |
| 0.8.34+commit.80d5c536 | 配布あり・実行確認 | 配布あり・実行確認 |
| 0.8.37+commit.f401782d | 配布あり・実行確認 | 配布あり・実行確認 |

両architectureのファイルはstatic ELFだった。0.8.34／0.8.37は既存distrolessのarm64イメージ内で動作。
[取得版・SHA-256](evidence/native-builds.json)の全5バイナリを公式manifestと照合した。
この24回の比較では元と同じcompiler版を用いた。別版の比較結果は後述し、利用した版を区別して記録する。

自動取得の[HTTP記録](evidence/downloads.json)はmanifest 2回＋binary 5回。
予備調査では`binaries.soliditylang.org`がHTTP 403だったため公式GitHubミラーを使用した。
これはcompiler配備のための取得であり、Pool単位で繰り返す本番処理ではない。
Sourcifyへの新規照会は行わず、保存済みsource bundleを再利用した。

## 制限と終了動作

検証コンテナはnetworkなし、read-only、nonroot、capabilityなし、CPU 1、memory/swap 512 MiB、pids 32。
native子プロセスには独立してアドレス空間・CPU時間上限を設定した。

| 確認 | 実測結果 |
| --- | --- |
| wall timeout 100 ms | 約101 msで子プロセスをkillし、Wait完了 |
| 明示キャンセル100 ms | 約101 msでkillし、Wait完了 |
| CPU時間1秒 | 約1.01秒でCPU上限による終了。wall timeoutではない |
| アドレス空間24 MiB | 同じLMPT入力がInternalCompilerErrorになり、artifactを採用しない |
| stdout上限1 KiB | 過大出力を検出してcancelし、上限以上を保存しない |
| 不正Solidity | ParserErrorを検出してartifactを採用しない |
| 失敗後の正常処理 | LMPTを再実行し、元artifactと一致 |

メモリ不足・構文エラーでもcompilerの終了コードは0だった。終了コードだけで成功判定せず、
JSONの`errors[].severity`と必要なartifactを確認する。
検証helperの出力制限は実行テストで修正済み。`bytes.Buffer`の埋込みによる`ReadFrom`が`Write`制限を迂回しない構造にした。

0.8.34／0.8.37とも、`--standard-json --no-import-callback`で全ソース同梱の入力を処理できた。
実在する追加ファイルのURLを渡した場合はIOErrorとなり、そのファイルを取り込まなかった。
本実装では入力側でも`urls`を許可せず、ソースcontentが揃う入力だけを渡す。

検証用コンテナ全体のnetwork制限を、そのまま本番MarketHub内の子プロセスのnetwork隔離と呼ばない。
今回実証した本番候補は「別プロセス＋資源制限＋固定入出力」。独立したnetwork namespaceや常駐compilerサービスは未実装。

## 本番候補（レビュー対象）

compiler版の一致を必須にせず、別版でも必要データの解析と対象Tokenとの対応確認が成立すれば採用する方針はユーザー了承済み。
初期はnative compiler方式を先行し、実行条件を満たせない旧版はnullとすることも了承済み。Node.js追加は初期対象外。
正式runnerと配備の実装・統合負荷検証は後続工程とする。

1. 同梱compilerは固定hashで用意する。0.8.34／0.8.37は今回の検証対象であり、本番の対応版を2つに限定する根拠にはしない。
   ユーザー了承に従い、未取得の元compilerをダウンロードする前に、互換性のある同梱版で照合を試す。
2. 既知コード・保存済みの解析結果を優先する。元のcompilerが既に使えるならそれを使う。
   元の版がなければ、元ソース・pragma・設定を変更せず扱える同梱版でコンパイルする。
3. コンパイル成功だけで税観測を採用しない。オンチェーンの実行コードとの照合と対応税モデルの解析を必要とする。
   metadataやimmutableの差分は、意味に影響しないと対応モデルで確認した範囲だけ許容する。
   同梱版で必要な確認が成立すれば元のcompilerは取得しない。確認済みの項目を採用し、不明な項目はnullとする。
   コンパイルできない場合や説明できないコード差分がある場合に元のcompiler取得・再コンパイルへ進む。公式配布・hash照合・キャッシュを用いる。
   解析モデル自体が未対応の場合は、compiler取得を繰り返す理由にはせずnullとする。
   取得・実行・照合ができない場合もnull。初期の旧版対応は実行条件を満たすnative配布物がある場合に限る。
   信頼対象USDCの解析省略、WETH9のレビュー済みコード照合は維持する。
4. 初期の1ジョブ上限案はwall 15秒、CPU時間5秒、仮想アドレス空間512 MiB、入力2 MiB、stdout 8 MiB、stderr 64 KiB。
   これは今回の入力が通る保守的な上限案で、全Tokenを解析できる保証ではない。
   CPU時間5秒はCPU使用率の制限ではなく、512 MiBも常駐メモリの計測値ではない。
5. `./worker-pool`はMarketHubが保持し、compiler同時実行数を制限する。worker数・queue容量は統合負荷試験で決める。
6. nativeを使える版では今回の実行方式を再利用できる。nativeがない旧版への対応では、今回動いたNode.js＋solc-jsも候補とする。
   最終的なruntime・配備方式は、同梱版を先行する順序と分けて確定する。

native方式の先行は確定済み。本番imageには現段階で適用していない。

## 別compiler版を先に試す追加検証

ユーザーの「元の版を追加取得する前に、同梱の最新版で試したい」という提案に合わせ、保存済み3 Tokenを別版でコンパイルした。
ソース・pragma・compiler設定は変更していない。追加RPC・compilerダウンロードは0回。
[実行スクリプト](cross_version.py)、[結果](evidence/cross-version.json)。

| Token | 元の版→試した版 | コンパイル | コード比較 |
| --- | --- | --- | --- |
| TAOT | 0.8.34→0.8.37 | 成功 | 全体は不一致。末尾CBOR metadata以外の3,511 bytesは保存済みオンチェーンコードと一致 |
| LMPT | 0.8.37→0.8.34 | 成功 | metadata以外のコンパイル時テンプレート6,944 bytesが一致。オンチェーンとの比較にはimmutable実値の確認も必要 |
| WETH9 | 0.5.17→0.8.37 | 失敗 | pragmaの許容範囲外 |

比較では末尾の長さ情報だけで削除せず、対象のCBOR mapを解析しcompiler版フィールドを確認した。
ただし、これは差分の診断であり、metadataが実行結果へ影響しないことまで一般的に証明するmatcherではない。
metadataを読むコードや、説明できないimmutable・library・状態依存があれば、その意味を別途検証する必要がある。
今回の3件から、他Tokenの一致率やcompiler取得削減率は推定しない。税観測の自動採用はまだ行っていない。

**結論:** 同梱版を先に試す順序には実例上の見込みがある。「コンパイル成功」を「このソースが実際のTokenに対応する」と同一視せず、
検証済みのコード照合・税モデルを通った結果だけ採用する構成にする。

```sh
python3 experiments/token_taxes_runtime_20260922/cross_version.py
```

## 配備レビューのための旧版オプション確認

0.5.17のnative amd64版を、保存済みWETH9入力に`--no-import-callback`を付けて実行した。
[結果](evidence/legacy-import-option.json)はexit code 1、`unrecognised option '--no-import-callback'`。
従って、前述の通常コンパイル成功だけでは、外部importを無効化する初期runnerで旧版を扱える根拠にならない。
オプションを黙って外して実行を続ける方針にはしない。レビュー済みコードの照合によるWETH9解析は引き続き可能。

追加のonchain RPC・Sourcify照会・compiler取得は0回。コンテナは`--rm`で終了し、既存サービスを変更していない。
同梱候補4ファイルのSHA-256も再確認した。2版の合計ファイルサイズはarm64で27.91 MiB、amd64で30.84 MiB。
image全体や圧縮後の転送量を表す数値ではない。

再現コマンドは次のとおり。入力とimage IDは本検証で準備済みのものを使う。

```sh
docker run --rm -i --network none --read-only --cap-drop ALL --security-opt no-new-privileges --user 65532:65532 --cpus 1 --memory 512m --memory-swap 512m --pids-limit 32 --mount type=bind,src=/private/tmp/token-taxes-runtime-20260922,dst=/probe,readonly --entrypoint /probe/supervise-arm64 sha256:129d54b986d38751e010fa0277891e3d0516774516c95c6367374f6c7b057fc9 -memory-mib 0 -- /probe/linux-amd64/solc-linux-amd64-v0.5.17+commit.d19bba13 --standard-json --no-import-callback < /private/tmp/token-taxes-runtime-20260922/WETH9.json
```

amd64はARM上のエミュレーション。compilerの仮想メモリ上限はこの試行では無効、コンテナのmemory上限は512 MiB。
旧版の本番利用を承認・実装した検証ではない。
[初期配備のレビュー案](../../../k4k3ru/docs/internal/NEW_PAIR_TOKEN_TAXES_COMPILER_DEPLOYMENT_REVIEW.md)へ選択肢を整理した。

## 解析器との境界

- `onchain`: 固定観測位置でのcode読取り、Sourcifyからのソース資料、モデル解析、コード照合、税の観測結果を所有する。
- 税解析器が所有する小さな`Reader`／`SourceProvider`／`Compiler`境界をconstructorで受け取る。
- `Compiler`は版・全ソース・設定を受け、artifact／AST／diagnosticsを返す。DB・pool・process-globalな実行環境を参照しない。
- MarketHub: 固定したcompilerの配備・processの資源制限・キュー・cache・保存・通知を所有し、正式なrunnerを注入する。
- productionは、この検証ディレクトリのGo／Python／Nodeスクリプトをimport・呼出ししない。

## 再現手順

入力は保存済みの3 Token。USDCは対象外。コンパイル用全ソースとバイナリは一時ディレクトリへ置く。
次はonchainリポジトリ直下で実行する。`probe.py`のimage ID・既存solc-js cacheのパスは検証環境の値。
別環境では固定image・compiler hash・wrapper依存を準備し、対応するパスを明示して再現する。

```sh
python3 experiments/token_taxes_runtime_20260922/prepare.py --cache /private/tmp/token-taxes-runtime-20260922 --fetch-native
env GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -o /private/tmp/token-taxes-runtime-20260922/supervise-arm64 experiments/token_taxes_runtime_20260922/supervise.go
env GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o /private/tmp/token-taxes-runtime-20260922/supervise-amd64 experiments/token_taxes_runtime_20260922/supervise.go
python3 experiments/token_taxes_runtime_20260922/probe.py matrix
python3 experiments/token_taxes_runtime_20260922/probe.py controls
python3 experiments/token_taxes_runtime_20260922/probe.py imports
docker build --pull=false --tag k4k3ru-token-tax-runtime:research --file experiments/token_taxes_runtime_20260922/Dockerfile /private/tmp/token-taxes-runtime-20260922
```

イメージ確認は次のように行う（LMPT）。TAOTは`0.8.34`と`TAOT.json`、保存名`image-taot.json`へ置き換える。

```sh
docker run --rm -i --network none --read-only --cap-drop ALL --security-opt no-new-privileges --cpus 1 --memory 512m --memory-swap 512m --pids-limit 32 k4k3ru-token-tax-runtime:research -- /opt/solc/0.8.37 --standard-json < /private/tmp/token-taxes-runtime-20260922/ShinyLIMPET.json > /private/tmp/token-taxes-runtime-20260922/image-lmpt.json
python3 experiments/token_taxes_runtime_20260922/verify.py
env GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go vet experiments/token_taxes_runtime_20260922/supervise.go
```

Go helperは`//go:build ignore`で明示実行専用。gofmt・Linux arm64/amd64 build・対象go vetを完了。
Pythonの記録検証は24比較＋7制限＋3import＋2imageを確認した。
本番API未実装のため、Agent E2E・本番サーバー全体のbuild／testは今回の対象外。
実amd64環境の資源制限・本番負荷・任意Tokenへのモデル認識は後続工程で確認する。

検証用image `k4k3ru-token-tax-runtime:research`をローカルへ作成した。
常駐コンテナは起動せず、各probeは`--rm`で終了後に削除される。既存MarketHubの起動・再起動・deployは行っていない。
