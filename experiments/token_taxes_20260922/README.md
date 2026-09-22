# TokenTaxes 取得・解析方式の検証

2026-09-22 / Base mainnet / 調査専用。本番SDK・MarketHub・公開API・掲載条件は変更していない。

## 結果

**レビュー済み実装のruntime照合と、必要な状態の読取りに限定すれば、税率・税変更経路・免除機構を分けて返せる。**
実チェーンの3 ERC-20で税なしを確認し、ローカルEVMでは購入2%／売却5%、免除、現在0%でも変更可能な状態を確認した。

これは任意の公開ソースを自動で解釈する解析器の完成ではない。今回は人が転送・権限経路を確認したコードをモデル化した。
自動化した範囲は、取得、再コンパイル、runtime照合、モデルに必要な状態読取り、結果生成、キャッシュと計測。
ABI名・ownerの値だけから税なしや変更不可を推定する処理はない。

## 実データの入口と結果

Agentが2026-09-22 11:48:12 JSTに受信したNewPairイベントから、配列上で最初のUniswap V3とAerodromeを1件ずつ選択。
税の結果を見て選び直していない。[入力](evidence/inputs.json)にPool ID、Token ID、観測位置、元イベント時刻を保存した。
サンプル数は2 Pool・4 Token。市場全体の解析率や掲載数を推定する統計検証ではない。

- Uniswap V3: `0xc32d5c4a463593632b8d1f5472cbe06b671d1e5a`（WETH / LMPT）
- Aerodrome: `0x8bd619b0953725f2e7a8c3bf763fd8addac9250a`（TAOT / USDC）
- 固定観測: block **51626749**, `0x968b022a36e55f34a2b512d07ae82ca232975272428d3064254c12181aa2d96f`
- 別ブロック再評価: block **51635015**, `0xfc5717b55d4ec46b81ff4b402ab66ae4690e10cdf098d921f529292a7e951d60`

| Token | buyRate / sellRate | canChange / hasExemptions | 根拠・限界 |
| --- | --- | --- | --- |
| WETH `0x4200…0006` | `"0"` / `"0"` | false / false | WETH9の全転送経路。委譲・税設定経路なし |
| LMPT `0xaaeb…ee7e` | `"0"` / `"0"` | false / false | ERC20Permitを継承する実装。標準転送を変更する処理なし |
| TAOT `0x7f2f…ff3e` | `"0"` / `"0"` | false / false | 標準転送を変更する処理なし。ただし発行・ブリッジ管理権限はある |
| USDC `0x8335…2913` | tokenオブジェクトがnull | — | proxy外側のソースは取得済み。実装先・管理経路の解析は今回未対応 |

両ブロックで同じ税の結果を得た。TAOTのfalseは**税の変更経路**がないという意味であり、管理権限全般の不存在ではない。
USDCのnullは税があるという意味ではなく、解析未対応。同様にSDKの信頼Token定義を税率0の根拠にしていない。
実チェーンで非ゼロ税のTokenを解析できた実績はまだない。非ゼロ税の肯定例は下記ローカルfixture。

### コードとソースの対応

Sourcify v2からソース・コンパイラ設定を取得し、Solidity公式配布の該当版で**こちら側でも再コンパイル**した。
コンパイラは公式manifestのSHA-256を確認。既製のToken税判定APIは使用していない。
[照合結果](evidence/live-code-matches.json)にsource bundle・runtime・compilerのhashを記録。

- TAOT: runtime全体が完全一致。
- WETH9:末尾52 byteのCBORメタデータだけが異なる。既知の末尾境界と直前のINVALIDを確認し、実行部分は一致。
- LMPT: compilerが指定する7か所のimmutableを展開すると一致。全てEIP712の署名ドメイン関連値で、税率・課税先ではないことを確認。
- 認識時のキーは**展開後のruntime全体のhash**。任意の差分を無視する汎用マスクは使用しない。
- ソースbundleの未レビュー変更、実行部分の1 byte変更は受け入れない。

コールドコンパイルの再計測はWETH 493 ms、LMPT 729 ms、TAOT 585 ms（取得済みcompilerのロード込み、この環境の一回値）。
手動レビュー時間や初回compilerダウンロードはこの時間に含まれない。

## ローカルEVMでの照合

隔離したAnvilで7種類の検証コントラクトを配置。取引は全てlocalhost。公開チェーンに署名・送信していない。
実際の`transfer`と`transferFrom`双方で受取残高差を測った。

| ケース | 解析結果 | 実測・補助検証 |
| --- | --- | --- |
| 税なしERC-20 | 0% / 0%、変更不可、免除なし | 10,000転送→10,000受取 |
| 比例課税 | 購入2% / 売却5%、変更可、免除あり | 10,000→9,800 / 9,500 |
| 免除対象 | 基本税率は2% / 5%のまま | 免除アドレスへの購入側転送は10,000→10,000 |
| ownerがzero、別controllerあり | 変更可、免除あり | owner=0を読み、別controllerから税率を3% / 7%へ変更成功 |
| 現在0%へ設定 | 0% / 0%、変更可 | 同じTokenの新ブロックを取得。以前の2% / 5%を再利用しない |
| 偽の0% getter | tokenがnull | getterは0、実転送は10,000→9,100。詐欺検出成功ではなく、未対応コードを0%にしない検証 |
| 固定額＋比例課税 | 両rateがnull、変更不可、免除なし | 10,000→9,899、100→98。単一率では表せない |
| EIP-1967 slotを使う既知proxy fixture | 現在0%、変更可 | 実装先の税なしコードを確認後、管理者のupgrade成功も確認 |
| 未知proxy | tokenがnull | 標準slotが空でも税なしにしない |
| 同一Token・別Pool | 元Pool 2% / 5%、非対象Pool 0% / 0% | Pool別の適用状態を読取り、結果を取り違えない |
| V4 Pool IDの条件付き課税 | rateはnull、確認済みフラグだけ返す | 32 byteのIDを送受信アドレスに変換しない |
| 状態取得失敗 | 失敗したrateだけnull | 初回＋3回再試行。成功済みの他データは再取得しない |

[EVM実行記録](evidence/local-transactions.json)、[解析・残高差・RPC計測](evidence/local-results.json)。
保存した最終シナリオには55件のローカル取引、計165件のセットアップ・残高確認等のローカルRPCがある。
解析用RPCの詳細は別ledger。これらは外部プロバイダーの消費量には含めない。

V4は**Pool条件付きモデルを未確定にする否定例まで**。実V4 Poolの送受信経路との対応を確認した肯定例は残課題。
また実DEXルーターのSwap可否、署名者ごとの売却可否は本検証の対象外。Token税の値から取引成功を保証しない。

## RPC・HTTPコスト

バッチ送信なし。下表のRPC 1回はHTTPリクエスト1回。共通ブロック確認はグループ全体の前後2回。
状態は[EIP-1898](https://eips.ethereum.org/EIPS/eip-1898)のblockHash指定で読み、キャッシュにもblockHashを含めた。
同じブロック番号のhashが変わった場合は再利用しない。warmでも前後のcanonical block確認は省略しない。

### 実Base：4 Tokenをまとめて再評価

| 条件 | logical read | 外部RPC実行 | cache hit | 経過時間 |
| --- | ---: | ---: | ---: | ---: |
| 初回・モデル準備済み | 6 | 6 | 0 | 3.95秒 |
| 同じブロック | 6 | 2 | 4 | 1.29秒 |
| 新しいブロック | 6 | 6 | 0 | 3.72秒 |

初回6回はcode 4回＋共通block 2回。別途、セッション開始のchainId確認1回。
時間にはRPC間の350 ms間隔を含む。warmは実測しており、RPC全体が0になるという意味ではない。
モデル・ソース取得はこの表の外。[計測ledger](evidence/live-results.json)。

### ローカル比例課税モデル：1 Token

| 条件 | RPC実行 | 内訳 |
| --- | ---: | --- |
| cold | 7 | code 1＋controller／Pool適用／購入率／売却率の4 reads＋block 2 |
| 同じブロック | 2 | state 5件を再利用、block 2 |
| 同じToken・別Pool | 3 | 別Poolの適用状態1＋block 2。他Poolで無課税と確定したためrate読取り不要 |
| 新ブロック | 7 | 最新の適用状態・税率を再取得 |
| 既知proxy＋税なし実装のcold | 6 | proxy code＋実装slot＋admin slot＋実装code＋block 2 |

これは本番Tokenの平均値ではなく、対応したfixtureモデルのread数。ABI/getter数・proxy経路によって増える。
1 readが失敗した場合は最大4試行、バックオフ1/2/4秒。リトライ・失敗はキャッシュしない。

### 今回の外部通信合計

- 調査開始時: RPC 4試行が403で失敗。初回＋3回再試行で停止した。[記録](evidence/live-rpc-initial-403.json)
- User-Agentを既存調査と同じにした再取得: RPC **7回**、Sourcify HTTP **4回**。成功の原因をヘッダーだけとは断定しない。
- cold/warm/new block比較: RPC **15回**（chainIdの1回を含む）。
- 公式compiler取得: HTTP **4回**（manifest 1＋3版）。開発時の取得でありPoolごとのコストではない。
- 合計: 外部RPCのHTTP試行 **26回**（成功22・失敗4）、その他HTTP **8回**。
- サンドボックス内でのDNS失敗4試行とlocalhost接続拒否は別。プロバイダーから応答を得た回数には含めない。

初回ソース取得4回は同一モデルの再評価では再取得していない。検証用の取得工程とcold比較を別セッションで実行したため、code読取りの一部は意図的に重複している。

## 本番実装に向けた案（未確定）

1. **onchain**: 所有パッケージにToken税の結果・観測位置・解析モデルを置く。Reader、モデル照合器、Poolの送受信コンテキストを明示注入する。rate／変更可否／免除は部分的なnullを許容する。
2. 初期認識はレビュー済みruntimeと単純なERC-20モデルから始める。新規Tokenへの対応率を上げるには、継承・override・proxy・外部呼出しまで検証する厳格なソース構造の認識を別途実装する。今回のfixture用hashを本番にコピーしても一般Tokenへの対応にはならない。
3. **MarketHub**: NewPair受信処理と解析workerを分離。OnchainRegistry等の取得済みcode・header・状態を同一観測位置で再利用し、不足分だけRPC。Token単位で取得を共有し、Pool条件付きの解析結果はPool単位で分離する。
4. 公開ソース取得・compiler実行はPoolイベントの同期処理に入れない。取得元は今回Sourcify v2で実証したが、productionで外部サービスを必須にするか、事前解析済みcatalogを使うかはレビューが必要。compilerは隔離worker・上限付きで扱う。
5. RPC予算のたたき台は、直接Token **8 unique reads**、proxy **16 unique reads**、共通headerは別枠。今回の直接5／proxy4 state readsを収め、委譲は深さ2まで。各失敗の初回＋3リトライも総attempt予算へ計上する。平均コストや最終上限の確定値ではない。
6. **SDK → MarketHub → Agent**へ承認済みのnullableな`tokenTaxes`を追加する。未取得を0%へ補完しない。source/positionを保ち、税専用pollingや新しい掲載フィルターはこの工程で導入しない。
7. onchainのfake注入テスト、SDK JSON互換性、MarketHubの同一Token共有・別Pool分離・reorg/失敗時テストを経て、`./k4k3ru-agent`で新しい通知のE2Eを行う。実行経路の確認はTradeHub側。

初期モデル、USDCのproxy対応範囲、外部ソースの運用方式を確定してから本実装へ進む。
未知コードを0%にせずnullで返すルールは、承認済みの返却設計を維持する。
掲載除外は引き続き別レビュー。今回の3/4という件数から掲載への影響は判断できない。

## 再現とチェック

Python標準ライブラリ、既存のsolcjs 0.8.30ラッパー、検証用Anvilを使用。go.mod/package.jsonは変更なし。
以下はこのディレクトリから実行した主要コマンド。実Base取得以外は保存済みevidenceで再検証可能。

```sh
node compile.cjs /private/tmp/lp-lock-solc-0830/node_modules/solc
python3 acquire.py --event /private/tmp/k4k3ru-lp-principal-retry-20260922/principal-event.json --output evidence
python3 get_compilers.py /private/tmp/token-taxes-solc-20260922
node compile_live.cjs /private/tmp/lp-lock-solc-0830/node_modules/solc /private/tmp/token-taxes-solc-20260922
python3 verify_live.py
python3 local_probe.py
python3 live_probe.py
python3 -m unittest -v test_validation.py
```

ローカルEVMの起動（この検証で作った専用コンテナのみ終了する）:

```sh
docker run --rm -d --name k4k3ru-token-taxes-20260922 \
  -p 127.0.0.1:19545:8545 --entrypoint anvil \
  ghcr.io/foundry-rs/foundry@sha256:043752653d5be351c71709091b3db97c4421c907eb40ea294195e7f532aadf46 \
  --host 0.0.0.0 --silent
docker stop k4k3ru-token-taxes-20260922
```

**17 tests PASS**。fixtureのSolidityコンパイル、実Token 3件の再コンパイル・runtime照合、ローカルEVM実行、Base読み取りを完了。
Python AST／Node構文チェック、文書のローカルリンク検証、`git diff --check`も成功。
本番Goディレクトリから本experimentsへの参照はない。検証専用Anvilコンテナは終了済み。
Go本番コードの変更がないため、Go全体テスト・サービス再ビルド・新規Agent E2Eは実行していない。
既存Agent受信データを解析の入口として再利用したもので、新しいAPIのE2E完了とは扱わない。

参考: [Sourcify v2 API](https://docs.sourcify.dev/docs/api/)、[Solidityのcompiler出力](https://docs.soliditylang.org/en/latest/using-the-compiler.html)、[immutable](https://docs.soliditylang.org/en/latest/contracts.html#immutable)。
