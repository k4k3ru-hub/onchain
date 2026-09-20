# UNCXの実装に基づくGo SDK検証

最新の4 Venue・各10 Poolの検証は[VENUE_SAMPLING.md](VENUE_SAMPLING.md)を参照。UNCX専用処理を使わず、自動算出16件・人手のソース確認を加えた算出2件・未確定22件を記録している。

検証日: 2026-09-20。Base mainnet / Uniswap V3 / UNCX。

> 要件の訂正: 求められているのは、UNCXに依存しない算出処理と、その結果をUNCXで答え合わせする検証。
> 以下のテストはUNCX固有のABI・runtime hash・引出条件に依存するため、**比較用の参照検証**に位置づける。
> UNCX非依存の算出が成功した証拠としては扱わない。最新の独立算出と比較結果は[CURRENT_RATIO.md](CURRENT_RATIO.md)、前段の抽出検証は[INDEPENDENT_CALCULATION.md](INDEPENDENT_CALCULATION.md)を参照。

**UNCXの公開されたコントラクト実装から引出条件を解析し、既存の`onchain` Go SDKを使った読取り検証で、Pool作成イベントから現在のlock割合100%まで到達した。**
検証用コードは`_test.go`に限定し、公開SDKのAPIや本番依存は変更していない。

## 実装を読んで確認した条件

対象の実装は`UNCX_LiquidityLocker_UniV3`。次を確認した。

| 実装 | 確認内容 |
| --- | --- |
| `lock` | 指定NFTを保管し、manager・Pool・NFT・所有者・解除日時を記録する |
| `getLock` | lock IDから現在の記録を取得できる |
| `withdraw` | lock所有者だけが実行でき、通常lockは`unlockDate < block.timestamp`が必要 |
| `decreaseLiquidity` | lock所有者・NFT ID・同じ期限条件を検証してから元本を減らす |
| `relock` | 期限の延長だけが可能 |
| `collect` | 所有者／追加collector／botがfeeを回収する経路。保有中liquidityの減額とは区別する |
| `migrate` | MIGRATORが必要だが、通常の解除期限チェックがない移行経路 |
| `setMigrator` | コントラクト管理者が移行先を変更できる |
| `adminRefundERC20` | ERC20のtransferを使う。対象のUniswap NFTをこの経路では送れない |

元ソースの対応箇所は150、239、309、327、341、372、441、460、468、540行付近。
ソース全文は再配布せず、照合用SHA-256を記録した。

- ソースSHA-256: `12116e83ad4a3a53946d65ea783fac4d337b5b23e955525ffd6c71733ebf2f80`
- runtime keccak256: `0x865de3c33580ca7144492890feee24ef985af705497e92e1dea0ef11a8be0eb8`
- [Sourcifyの照合情報](https://sourcify.dev/server/v2/contract/8453/0x231278eDd38B00B07fBd52120CEf685b9BaEBCC1?fields=all)
- [デプロイされたコントラクト](https://base.blockscout.com/address/0x231278eDd38B00B07fBd52120CEf685B9BaEBCC1?tab=contract)

ライブテストでは発見した保管先の`eth_getCode`を読み、review済みruntime hash、Sourcifyの`exact_match`、照合先runtime bytecode、ソースhashを確認した。
名称やABIが似ているだけの別コントラクトは採用しない。
実装と主要経路の確認であり、形式検証や第三者セキュリティ監査ではない。

## Poolを入口とする実行

入力は[testdata/pool_created.json](testdata/pool_created.json)の実際のPool作成イベントだけ。
この例は以前の検証対象の再生であり、新規Poolから無作為に選択した標本ではない。
コードにfactory／manager／protocol ABI／review済みcode hashは与えるが、対象のNFT ID、保管先address、lock IDは与えない。

1. 作成イベントを成功receipt・canonicalな作成blockと照合する。
2. 既存`v3/protocol.PoolKey.Address`でPool addressを検算する。
3. 作成receiptのPool `Mint`とmanager `IncreaseLiquidity`からNFT候補を発見する。
4. 既存`evm/clliquidity.Reader.Capture`で全bitmap・全tick・現在元本を取得する。
5. 対象Poolに一致するNFT持分が全tickのgross／netとmanagerのcore持分を説明することを確認する。
6. `ownerOf`から保管先を取得し、そのruntimeと実装を照合する。
7. 発見した保管先の`onLock`からlock IDを取得し、現在の`getLock`と突き合わせる。
8. 引出経路・例外権限を同じblockの`eth_call`で確認し、最後にblock hashを再確認する。

結果は[testdata/live_result_20260920.json](testdata/live_result_20260920.json)。

| 項目 | 観測値 |
| --- | --- |
| Pool | `0xd67f187eb06C51D3Ed02C73A14F82Ef36FeA4665` |
| 評価block | `51557560` |
| block hash | `0x387c18cd66b280ee8d059eb1e22b3d0d626a950584e2378fd6326e3fa7e46b95` |
| 発見したNFT | `5951125` |
| 発見したlock ID | `1158` |
| 現在の解除日時 | `1791648509` = 2026-10-10 16:08:29 UTC |
| 元本base units | token0: `69651978657561984`、token1: `243768074` |
| 現在の設定で拘束される元本割合 | `100%` |
| 将来の移行設定変更権限 | 管理者にあり |

## 実コードに対する読取りシミュレーション

| 呼出 | 送信元として指定した役割 | 結果 |
| --- | --- | --- |
| `withdraw` | lock所有者 | `NOT YET`でrevert |
| `decreaseLiquidity`（全量） | lock所有者 | `NOT YET`でrevert |
| managerの`decreaseLiquidity`を直接呼ぶ | lock所有者 | `Not approved`でrevert |
| `relock`で期限を短縮 | lock所有者 | `DATE`でrevert |
| `migrate` | lock所有者 | 現設定のMIGRATORが0なので`NOT SET`でrevert |
| `adminRefundERC20`にNFTを指定 | 管理者 | `ST`でrevert |
| `setMigrator` | 管理者 | 成功。変更は保存されない |

すべて`eth_call`であり、署名もtransaction送信もしていない。
revertはRPC接続エラーと区別し、ABIのエラーデータから理由を復号している。
管理者が将来MIGRATORを設定できるため、100%という結果は現在の設定での拘束割合。
「解除日時まで管理者を含め誰も引き出せない」という意味ではない。
期限後に解除できること、設定変更と移行を連続実行した場合の結果は今回シミュレーションしていない。

## onchainに実装できる範囲

既存SDKで再利用できたもの:

- `evm.HTTPClient`: receipt取得、contractのread／シミュレーション。
- `evm/clliquidity`: 全tick取得、全体状態の整合性確認、元本計算。
- `venues/uniswap/v3/protocol`: Poolの識別・address検算。

追加する必要がある処理:

- NFTの発見・所有先追跡、Pool全体との持分照合。
- 対応lockerの実装識別・状態取得・引出条件・例外権限の判定。
- 結果に観測block、確認できた持分、未対応理由、管理者権限を含めること。

PoCでは`eth_getCode`と`clliquidity.RPC`形式のheader取得に既存依存のgo-ethereumを明示的に組み合わせた。
`evm.HTTPClient`には現在`CodeAt`がなく、headerの戻り値も`clliquidity.RPC`と異なるため、
公開SDKへの実装時にはこの依存の組立てを整理する必要がある。
新しい依存追加や既存API変更はこの検証では行っていない。

このGo検証は単一NFTが全持分を説明する、対応runtimeの期限付きlockに限定する。
複数NFT、後日lockの履歴探索、別locker、expired／eternal lockの統一出力、未知の実装の自動解析は未実装。
検証対象外や不完全な持分を0%として扱わず、テストを失敗させる。
したがって、**UNCXを確認用にした実装可能性は確認できたが、汎用SDK機能の完成を意味しない。**

## 使用量と検証コマンド

- 論理RPC: **23回**。
- HTTP RPC試行: **26回**（429再試行3回を含む）。
- 別途Sourcifyのsource照合GET: **1回**。
- 所要時間: **39.65秒**。公開RPC向けの1.5秒間隔を含む。
- 単体テスト3件PASS、実RPCテスト1件PASS、対象packageの`go vet` PASS。

前回の割合計算12 RPCに比べ、今回は複数の引出経路の検証・作成block照合を追加し、getterの一部も個別に呼んでいる。
この23回は確認用テストの費用であり、運用時に毎回必要な最低回数ではない。

`onchain/go`から実行:

```sh
go test ./internal/experiments/lplock -count=1 -v
ONCHAIN_UNCX_LIVE=1 ONCHAIN_UNCX_REPORT=/tmp/onchain-uncx-go-result.json go test ./internal/experiments/lplock -run '^TestUNCXPoolFirstLive$' -count=1 -v
go vet ./internal/experiments/lplock
```

通常のテストではライブ検証をskipする。ライブ結果は現時点の状態に依存し、lock期限後や設定変更後は成功を期待しない。
本番API・MarketHubへの組込みとAgent経由のlock指標E2Eは未実施。
