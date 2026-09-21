**現在状態からのLP保護確認（2026-09-21）**

前回の24時間調査で履歴欠落により未確定だったUniswap V4の1,575 Poolのうち、128 Poolについて、現在状態の照合でLP元本100%の永久保護を確認できた。残り1,447 Poolは未確定であり、ロック0%や不合格を意味しない。

今回はLP保護の検証。Tokenの税・売買制限・管理権限、売買需要、予定数量の往復売買は再評価していない。前回の総合条件の合格7件を137件へ更新した結果ではない。

**対象と評価時点**

- Chain / Network / Venue：Base Mainnet / Uniswap V4。
- 最新状態の評価ブロック：51,604,306。
- ブロック時刻：2026-09-21 22:39:19 JST / 13:39:19 UTC。
- Hash：`0xeaa77d3a8cce998496372ea11342eff355c61f8ea86a5def29adc7633b6e62fe`。
- 最初に取得した最新ブロックに全読み取りを固定し、各工程の終了時にhashを再確認した。実行中に変わる`latest`を混在させていない。
- 前回未確定だったV4の1,875 Poolと、前回合格7 Poolの計1,882 Poolを対象とした。V3・Aerodrome・Cetusの未確定30 Poolは今回の再検証対象に含めていない。

**結果**

| 対象 | 件数 | LP保護100%を確認 | LP保護は未確定 |
|---|---:|---:|---:|
| 前回、履歴欠落で未確定だったPool | 1,575 | 128 | 1,447 |
| 上記以外の、前回V4未確定Pool | 300 | 2 | 298 |
| 前回の総合条件合格Pool（対照群） | 7 | 7 | 0 |
| 合計 | 1,882 | 137 | 1,745 |

上記の2 Poolは前回もLP保護だけは確認済みで、対価Tokenの仕様とUSD評価が未確定だったもの。137件の内訳は、前回LP保護確認済み9件、履歴で持分を発見していたが全体を確定できなかった17件、今回現在状態から持分を発見して照合できた111件。

全1,882 Poolで現在の価格と有効な流動性を取得できた。有効な流動性が正だったPoolは897件、ゼロは985件。範囲外に元本が残る場合があるので、このゼロをPool全体の元本ゼロやロック0%とは扱っていない。

**確認方法**

1. 前回Poolの持分から発見・ソース確認したLaunchLockerの現在状態を再利用した。対象PoolのToken 1,797アドレスに対して`tokenIdOf`を読み、NFT 138件を取得した。PositionManagerのPool情報と照合し、今回のPoolに対応する137件を採用した。他PoolのNFT 1件は採用していない。
2. 同一ブロックでNFT所有者・個別承認・現在の持分数量を読み、137件すべてがLaunchLockerに保管され、個別承認ゼロ、正の数量であることを確認した。LockerとPositionManagerの実行コードは、前回公開ソースと照合した実行コードと完全一致した。Locker → PositionManager → PoolManager、およびStateView → PoolManagerの参照先も確認した。
3. 各Poolの全有効tick範囲を網羅するtick bitmapを読み、初期化済みtickを列挙した。137 Poolはすべてtick spacing=200で、1 Poolあたり36 bitmap word、計4,932 wordを読んだ。発見した全274 tickの`liquidityGross`と`liquidityNet`を取得した。
4. 保護済み持分の上下端・現在数量から求めたgross/netを、Poolの全tickと完全一致するか照合した。範囲外や別の所有者の持分が残れば、端点のgrossが増えるか別のtickが存在するため一致しない。さらにnetの累積がPoolの現在の有効な流動性と一致し、全tick通過後にゼロへ戻ることも確認した。
5. 137 Poolすべてで一致した。各Poolは正の持分1件で説明でき、hooksはzero addressだった。確認したLaunchLocker実装ではNFT移転・承認・元本を減らす公開経路やupgrade経路がなく、手数料回収時の流動性減少量は0に固定されるため、元本100%永久保護と判定した。

これは全持分の一致を証明するための照合であり、grossの単純な比率を「元本の保護割合」として返しているわけではない。全量の照合を完了できないPoolの割合はNULLとした。

使用した標準仕様は[StateLibrary](https://github.com/Uniswap/v4-core/blob/main/src/libraries/StateLibrary.sol)と[Poolの流動性更新処理](https://github.com/Uniswap/v4-core/blob/main/src/libraries/Pool.sol)。公開ソースの証跡は前回の[LaunchLocker](../newpair_quality_24h_20260921/evidence/sources/0xcd1680d26922fcd9cabfbb8a56ba40c333fd842a.json.gz)と[PositionManager](../newpair_quality_24h_20260921/evidence/sources/0x7c5f5a4bbd8fd63184577525326123b519429bdc.json.gz)を再利用した。今回、独立再コンパイルや形式検証はしていない。

**前回と同じ時点での対照確認**

時間経過だけで結果が変わったのかを区別するため、前回と同じブロック51,596,896（18:32:19 JST）でも、持分情報を保存済みだった26 Poolを全tickと照合した。

- 過去の持分数量・所有者・承認・価格・有効な流動性は取得済みデータを再利用した。
- 過去のイベント履歴の再取得は行わず、未取得だったbitmap 936 wordと52 tickの状態を読んだ。
- 26 Poolすべてで全持分が一致し、100%永久保護を確認できた。
- この26件には、前回履歴欠落で確定できなかった17件が含まれる。少なくともこの17件は、同じ時点でも今回の方法で確定できる。
- 残る111件について、前回と同一時点での照合は行っていない。128件すべてが前回時点でも100%だったとは主張しない。

**未確定の理由と適用範囲**

今回、全体との照合を完了した保管実装は前回発見したLaunchLockerに限定している。他の保管先の解除・移行・変更権限や、対応する全持分の確認は完了していない。最新のPool状態が読めたことだけで安全とは判定していない。

tickによる全持分照合は、イベント履歴が欠けても100%保護を確認する手段になった。一方、任意の保管先のコードを自動解析してロックの意味を判定する汎用機能を完成させたわけではない。UNCXや外部の安全性判定APIは使用していない。

元本保護と、手数料の受け取り権限・Tokenの所有権・需要・収益性は別項目。保護割合は評価時点のもので、後から別の引き出し可能な持分が追加されればPool全体の割合は変わり得る。

**RPCと証跡**

- 保存済み台帳に記録したRPC HTTP試行：160回。最新状態と同時点対照の両方を含む。
- 内訳：成功137回、HTTP 429が23回。再試行で回復し、今回の必要な読み取りに未回復の取得失敗はなかった。
- メソッド：`eth_getBlockByNumber` 7回、`eth_call` 151回、`eth_getCode` 2回。
- 別途、開始時の疎通確認2試行（sandbox接続失敗1回、HTTP 403が1回）。合計162試行。接続失敗の1回はRPCサーバーへ到達していない。
- Multicallを使用しており、HTTP試行数は内部の呼び出し数やプロバイダーの課金単位とは異なる。
- RPCは読み取りのみ。`eth_getLogs`、署名、トランザクション送信は今回使用していない。

[summary.json](evidence/summary.json)に全対象の分類、[ticks.json](evidence/ticks.json)に現在の照合結果、[historical_control/ticks.json](evidence/historical_control/ticks.json)に同時点対照を保存した。`evidence/rpc/`と`evidence/historical_control/rpc/`にRPC payloadと成功応答、各`requests.json`に試行台帳を保存した。

**再集計と検証**

```sh
python3 experiments/newpair_quality_lateststate_20260921/probe.py --stage summary
python3 experiments/newpair_quality_lateststate_20260921/verify.py
python3 -m unittest discover -s experiments/newpair_quality_lateststate_20260921 -p 'test_*.py' -v
```

ネットワーク不要の6テストが成功。範囲外の追加持分、同じ価格帯の別持分、空Pool、状態不整合、負のtickと全範囲境界、3回の再試行後に再取得を止める処理を確認した。Pythonの構文、変更ファイルの空白・改行、保存応答からの照合再現も確認した。

追加コードは`probe.py`、`verify.py`、`test_probe.py`。本ディレクトリにレポートと約4.4 MBの証跡を保存した。本番API・SDK・DB・掲載条件は変更していない。Goのテスト・buildは対象外。commit / pushは実施していない。
