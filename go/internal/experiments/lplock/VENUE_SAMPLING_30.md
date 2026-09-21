# 直近作成PoolのLP保護 — 4 Venue × 30件

検証日: 2026-09-21。各Venueの30件を保管先の調査前に固定し、合計120件を検証した。
失敗・未対応のPoolは別のPoolへ入れ替えていない。公開API、掲載条件、DBは変更していない。

**現在設定でロック100%を確認できたのは8件、0%は45件、割合未確定は67件だった。**
100%の8件にも管理者による移行設定の変更権限が残る。現在の割合と、将来の保護維持は別の評価である。

| Venue / Chain | 標本 | 現在設定で100% | 0%確認 | 割合未確定 |
| --- | ---: | ---: | ---: | ---: |
| Uniswap V3 / Base | 30 | 0 | 30 | 0 |
| Aerodrome Slipstream / Base | 30 | 8 | 15 | 7 |
| Meteora DLMM / Solana | 30 | 0 | 0 | 30 |
| Cetus CLMM / Sui | 30 | 0 | 0 | 30 |
| 合計 | **120** | **8** | **45** | **67** |

0%より大きく100%未満の値を確定できた例はない。未確定を0%として扱っていない。
この標本では「現在設定で100%と確認できる」という条件に残るのは**8/120 = 6.7%**。
残る112件の内訳は、実際に未保護だった45件と、割合を判定し切れていない67件である。

これはMarketHubの掲載実績から採った標本ではなく、作成されたPoolの標本である。
**「掲載数が93.3%減る」「市場の93.3%が保護されていない」という推定には使えない。**
8件も将来の権限変更を排除した掲載合格数ではない。残存期間・変更権限に関する掲載基準は今回追加していない。

## 作成期間と観測位置

ライブで一定時間待って30件を集める方式ではなく、確定済み履歴・公式インデックスから直近30件を遡った。
以下は30件のうち最古と最新の作成時刻。表の時刻はJST。

| Venue | 作成期間（JST） | 30件の時間幅 | 選定元 |
| --- | --- | --- | --- |
| Uniswap V3 | 9/21 17:16:19–17:44:07 | 27分48秒 | FactoryのPoolCreated |
| Aerodrome Slipstream | 9/19 04:51:57–9/21 17:04:23 | 60時間12分26秒 | 現行FactoryのPoolCreated |
| Meteora DLMM | 9/21 13:09:41–18:48:57 | 5時間39分16秒 | 公式Data APIの作成日時降順 |
| Cetus CLMM | 9/19 20:06:59–9/21 16:44:03 | 約44時間37分 | CreatePoolEventの最新30件 |

- Baseはblock **51596896**、**9/21 18:32:19 JST**に固定。全解析後にもblock hashを照合した。
  Uniswap V3 Factoryは`0x33128a8fc17869897dce68ed026d694621f6fdfd`。
  Aerodromeは現行Slipstream Factory `0xf8f2eb4940cfe7d13603dddd87f123820fc061ef`であり、Legacy AMMは含まない。
- Meteoraは最初の公式API応答で30件を固定。21件は作成取引のイベントとPoolをRPCで照合した。
  9件は各Poolの署名履歴3ページ・最大3,000件の予算を超え、作成取引の照合が未完了。
  その9件も標本に残し、追加でPoolとLPアカウントを取得した。30件のPoolアカウントのprogram owner・型を確認している。
  公式インデックスに未反映のPoolを含む全チェーンの「厳密な最新30件」は保証しない。
  LP観測slotは**449020851–449025061**。列挙と詳細取得は異なるslotになり得る。
- Cetusはcheckpoint **325178586**、**9/21 18:51:23.228 JST**に固定。
  30件すべての作成checkpointが観測位置以前であることを照合した。
- 全Venueで同時刻の観測ではない。Pool作成直後のLP状態を過去に再現した比較でもない。
  時間幅の違う直近30件から、Venue別の発生率や市場全体の割合は推定していない。

## Uniswap V3: Vaultに残っていても期限切れ

30件のうち2件は、全持分を説明できるNFT所有者による全量減少が`eth_call`で成功した。
28件は16個のVaultに保管されていたが、以下の追加調査により0%と判定した。

1. PoolのMint履歴・receipt・NFTから保管先を発見。最初の保管先にはSourcifyの公開ソースがなかった。
2. 実行コードの関数selector・エラー文字列から同構造の公開ソースを探し、`MultiVault`のソースを取得した。
   Solidity **0.8.4**で独立に再コンパイルした。
3. 16個すべての実行コードが、コンパイラの指定するimmutable挿入位置以外で一致した。
   同一immutableの複数挿入位置が同じ値であることも照合した。
4. 全16個で`unlockTimestamp - lockTimestamp = 300秒`。観測時点ではすべて期限切れだった。
   `isUnlocked=false`のままでも、期限による引き出し制限は既に終わっている。
5. 受益者がEOAであることを確認し、そのcallerから28 NFTすべての
   `partialNonFungibleTokenUnlock`が成功した。実際の送信やNFT移動は行っていない。

**「ロッカーにLPがある」だけでは不十分で、現在時刻に対する残存期限の確認が必要**という結果である。
28 Poolの受益者は同じ1アドレスだった。30件は独立した30組織の標本ではなく、同一主体の連続作成に強く影響されている。
この観測だけで詐欺や悪意を認定していない。

## Aerodrome: 8件の100%と、変更権限を分離

8件は各Poolの全持分を説明できるNFTが、EIP-1167 cloneのLockerに保管されていた。
前回Poolから発見・コンパイルした公開ソースを再利用し、今回の固定blockで実装コードを再照合した。

- NFT ID、PositionManager、Pool、保管先の対応と、全tickのgross/net・元本を照合した。
- `lockedUntil = 4294967295`、**2106-02-07 06:28:15 UTC**。本レポートでは無期限の放棄とせず、期限値を記録した。
- `staked=false`、NFT個別approvalなし。保存されたgaugeも、Voterから取得した現在のgaugeもzero。
- 通常解除は期限のエラー、移行は移行停止のエラー、stakeはgauge不在のエラー、直接解除はFactory認可のエラーになった。
  通信失敗や任意のrevertを「ロック成功」と見なさず、ソースで特定したエラーselectorと一致することを確認した。
- Factoryの`newLockerFactory=zero`。ただしFactory ownerは非zeroで、移行先変更の認可が残る。
  そのownerをcallerとしたsetterの`eth_call`は成功した。管理者コントラクト自体の署名条件まで検証した意味ではない。

したがって**現在設定での割合は100%、設定変更権限あり**と記録した。
期限まで引き出し不可であることを、将来にわたって保証した判定にはしていない。
仮に必要残存期間を1日・7日・30日と置けば、期限だけでは8件とも残るが、権限変更の問題は別に残る。

15件の0%のうち14件は所有者からの全量減少を確認した。
残る1件は2 NFTのうち一方がGaugeに保管されていた。発見した実装の検証済み公開ソースと実行コードを照合し、
Pool・NFT管理コントラクトの対応、EOAのstake登録、`withdraw`成功を確認した。他方のNFTは全量減少に成功した。
**ステーキングGaugeへの預け入れを、期限付きロックとして数えていない。**

未確定7件の内訳:

- 5件: Mint取引が98・188・188・189・96件。1 Poolあたり64 receiptの調査予算を超えた。
- 2件: 発見できたNFTだけでは全tickの持分を説明し切れなかった。確認できた部分だけで0%や100%にしていない。

## Meteora / Cetus: 30件ずつ調べたが、割合は未確定

**取得対象120件と、割合を確定できた53件は区別する必要がある。**
追加サンプリングで、Solana/Suiの解析未対応が解消したわけではない。

Meteoraでは30 PoolのLPアカウントを調べ、15 Poolから計31 Positionを観測した。
残る15 Poolでは対象Position列挙が空だった。観測した31件の`lock_release_point`はいずれも0。
しかし、全binの元本・持分の網羅性、保管先PDAの権限、引き出し経路を検証していないため、
これをPool全体の0%へ置き換えていない。空の列挙も、元本ゼロやバーンの証明ではない。

Cetusでは32 Positionオブジェクトを取得でき、AddressOwnerが20、ObjectOwnerが12だった。
別に4 Positionオブジェクトが返らず、12 Poolでは親オブジェクトを最後まで取得できなかった。
AddressOwnerという形式だけで引き出し可能と断定せず、Moveの所有・引き出し条件と全元本の照合を未完了としている。

これら60件を「100%保護ではない」と事実認定することはできない。
未確定を掲載しない場合は解析能力による除外になるため、保護基準の厳しさによる除外とは別に集計する。

## RPC利用量

保存した測定対象は**RPC / GraphQL HTTP試行1,832回 + 公開HTTP GET 7回**。
リトライを含む。JSON-RPCのbatch送信は使っていない。EVMのMulticall内部read数やプロバイダーの課金単位とは異なる。

| Venue | 発見・基本解析 | ソース照合・追加確認 | RPC / GraphQL合計 | 公開GET |
| --- | ---: | ---: | ---: | ---: |
| Uniswap V3 | 438 | 130 | **568** | 4 |
| Aerodrome | 848 | 159 | **1,007** | 1 |
| Meteora DLMM | 117 | 18 | **135** | 2 |
| Cetus CLMM | 122 | 0 | **122** | 0 |
| 合計 | 1,525 | 307 | **1,832** | **7** |

- Base基本解析の429再試行はUniswap 97回、Aerodrome 145回。上表に含む。
  コアの取得処理は初回＋最大3回、2/4/8秒のバックオフ。非429の取得失敗も対象とし、期待するEVM revertは通信リトライにしていない。
- Uniswap基本解析は約13分02秒、Aerodromeは約23分02秒。発見、追加ソース調査、手動レビューの時間は別。
  Meteoraの最初の30件取得は約3分17秒、Cetusは約1分35秒。その後の補助解析を除く。
- 同一runのreceipt・ownerコードをキャッシュし、Meteoraの作成取引再読込21回をキャッシュで省いた。
  Aerodromeの既存ソース・コンパイル結果も再利用した。
- 調査段階をまたぐコード再取得、固定block headerの反復照合は残っている。
  したがって上表は今回の実測であり、最適化後の常時監視コストやNewPair 1件の固定単価ではない。
- 公開GETはソース取得・公式API・IDL。失敗したソース照会2回も含む。
  sandboxで遮断された非EVM接続8試行、Web資料検索、検証用コンパイラ取得は上の測定対象に含めない。

取得上限を設けた結果、Aerodrome 5件とMeteoraの作成照合9件に未完了が残る。
上限を無視して「失敗なし」「全120件の割合を算出済み」とはしていない。

## 証跡と再現

- [120件の個別結果・RPC内訳](testdata/venue_samples_20260921_30/summary.json)
- [120件のCSV](testdata/venue_samples_20260921_30/pools.csv)
- [証跡ディレクトリ](testdata/venue_samples_20260921_30/)
- [公開ソースの保存先・ハッシュ・コンパイラ](testdata/venue_samples_20260921_30/source-index.json)
- [ネットワーク不要の再集計](summarize_sampling30.py)

EVMの元データ`*-samples.json`は汎用調査段階の未確定を保持している。
`summary.json`は、公開ソースの人手レビューと追加probeを照合して結果を更新する。
任意のコードを自動認定する本番実装が完成した、という意味ではない。

UNCXのAPI・一覧・専用判定ロジックは使用していない。
公開ソースの照会先はSourcify等であり、ロック割合を外部評価APIから取得したものではない。
ソースはPoolから発見した保管先を入口に探した。名前・ブランド・既知アドレスだけでは肯定していない。

主要コマンド（`onchain`直下、Goコマンドのみ`onchain/go`）:

```sh
# 保存された120件をネットワークなしで再集計
PYTHONPYCACHEPREFIX=/tmp/lp-sampling-pycache python3 go/internal/experiments/lplock/summarize_sampling30.py
PYTHONPYCACHEPREFIX=/tmp/lp-sampling-pycache python3 -m unittest discover -s go/internal/experiments/lplock -p 'test_sampling.py' -v

# Goの検証
GOWORK=off GOCACHE=/tmp/k4k3ru-go-build go test ./internal/experiments/lplock ./evm/clliquidity
GOWORK=off GOCACHE=/tmp/k4k3ru-go-build go vet ./internal/experiments/lplock

# Baseのライブ再実行例。取得済み証跡を上書きしない出力先を使う。
GOWORK=off GOCACHE=/tmp/k4k3ru-go-build ONCHAIN_SAMPLE_VENUE=uniswap-v3 ONCHAIN_SAMPLE_COUNT=30 ONCHAIN_SAMPLE_BLOCK=51596896 ONCHAIN_SAMPLE_MANIFEST=internal/experiments/lplock/testdata/venue_samples_20260921_30/uniswap-v3-manifest.json ONCHAIN_SAMPLE_DIR=/tmp/lp-reproduction-uniswap go test ./internal/experiments/lplock -run '^TestLPVenueSamplesLive$' -count=1 -timeout 30m -v
```

非EVMの収集は`sampling_non_evm.py --count 30`、Meteoraの履歴未照合分は`sampling_meteora_accounts.py`。
保管先のコード照合は`sampling_runtime_review.py`、具体的な呼出は保存済み`*-plan.json`を
`sampling_plan_probe.py`へ渡した。外部通信はすべてread RPC / GET / GraphQL / eth_callで、署名・送信していない。

今回の変更は実験用Go/Python、検証、レポート、証跡のみ。
共通RPC測定ヘルパーが従来から置かれている`uncx_test.go`も取得リトライの変更対象だが、UNCX固有のlive testは実行していない。
Solidity 0.8.4のコンパイラは`/tmp`へ導入し、本番依存には追加していない。
Go test / vet、Pythonの6テスト、構文確認、gofmt確認、差分の空白確認は成功。
サービス全体のbuild・Swap E2Eは実験コードだけの変更のため実施していない。commit / pushも行っていない。

## この検証から判断できること

BaseではPoolを入口に、外部ロック評価APIなしで割合を確定できる例を増やせた。
一方、4 Venue横断の「100%確認済みだけ掲載」を今の解析能力で適用すると、
実際の未保護に加えて多数の解析未完了も除外する。100%保護が市場にほぼ存在しないという結論にはならない。

掲載条件を確定する前に必要なのは、期限の残存期間と変更権限の扱いを決めること、
Solana/Suiの全元本・引き出し条件の解析範囲を増やすこと、実際のMarketHub掲載対象でも同じ評価を行うことである。
LP保護が確認できても、Token税・売却制限・保有集中・十分な売却深度は別の評価項目として残る。
