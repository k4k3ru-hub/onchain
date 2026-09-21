# Base ERC-20の購入後制限・許可アドレス例外の検証

2026-09-21。検証用実装。公開API・SDKの正式な機能ではありません。

## 結果

**Base上の実例1 Tokenについて、購入による自動制限と管理者による許可設定での回避を、過去状態のローカルforkで再現・検出できました。**

| ケース | 購入 | 購入ブロック内の売却 `eth_call` | 次のブロックの売却 | その後の売却トランザクション |
| --- | --- | --- | --- | --- |
| 通常の新規アドレス | 成功 | 成功 | Token内部でrevert | 失敗 |
| 同じ通常アドレスに管理者が許可を設定 | 設定前に購入済み | 対象外 | 許可設定後は成功 | 成功 |
| 元の状態の管理者アドレス | 成功 | 成功 | 成功 | 成功 |

- 通常アドレスは購入から65ブロック後も売却できませんでした。
- Tokenの直接 `transfer` も次のブロックでは失敗しました。売却経路だけを制限する仕組みではありません。
- 購入で `0 → 1` になったToken内の1スロットだけを、別のローカル比較で `0` に戻すと、残高を変えずに売却できました。比較後はsnapshotを復元しました。
- 通常アドレスによる許可設定はrevertし、管理者による同じ設定は成功しました。
- 比較対象のBase WETHはdeposit後、ブロックを進めてもtransferが成功しました。これは送金の比較であり、WETHのDEX往復検証ではありません。
- 初めに試したブロック2,032,747ではDEXが停止しており、Token送金に到達する前に購入が失敗しました。このケースは `inconclusive` とし、Tokenの売却制限と判定しません。

同じブロック内の往復だけでは、この例を見逃します。購入による状態変化を保持してブロックを進める検証が必要です。

## 対象・再現条件

| 項目 | 値 |
| --- | --- |
| Chain / network | Base mainnet / chainId 8453 |
| 起点ブロック | 2,032,290。対象PoolのSwap成立イベントから選定 |
| 起点ブロックhash | `0x59b04b383ee85ed69b367a224c9d9b7fed67d04a68d2c4f717975f3b02be273a` |
| Token | ZEBRA / `0xac6b1693f547a6235a40c1559b287eab9ee4e167` |
| Pool | `0x123bffbacb6fce5e1696bcc1492f2c5e2bee4f09` |
| Router | `0xfcd3842f85ed87ba2889b4d35893403796e67ff1` |
| `owner()` | `0xf01e51504a9f5020f6f46cc0bc76c1d2efb40ec0` |
| 通常アドレス | `0x1000000000000000000000000000000000000001` |
| 購入額 | 0.0001 ETH。ローカルfork内のテスト資金 |
| 受取Token数量 | `17181139658566881547260` raw units |
| 失敗するToken呼び出し | Routerからの `transferFrom(通常アドレス, Pool, 数量)` |
| 制限に関与するスロット | `0xadf025fb6952a0c381ff3fd1479429b2aa16b97d1a333da509756f4447b3a429` |
| 実行環境 | Anvil 1.5.1、固定Docker image digest。詳細はsummary JSON |

既存コードとToken・Poolの残高をBase RPCから取得し、ローカルAnvilで実行しています。通常アドレスと管理者の経路は同じ起点snapshotから比較します。変更したnative ETH残高はローカルのテスト用です。Token残高やPool流動性を捏造して購入を成立させていません。

売却条件は、実際に購入できた数量、十分なallowance、十分なgas、期限内、minimum output 0に固定しています。minimum output 0は原因を切り分ける検証用であり、実取引の推奨値ではありません。

公開RPCは読み取り専用allowlistで制限しています。送信・impersonation・許可設定・storage変更は固定のlocalhost宛だけです。証拠中のSwap等のトランザクションhashは**ローカルで生成したもの**であり、Baseに実送信したhashではありません。

## 何を実装したか

- `probe.py`: 公開RPC・公開ソースの調査、Pool履歴取得、検証の入口。
- `fork_probe.py`: 固定ブロックのfork、通常/管理者の購入・売却、時間差、許可設定、制限フラグの比較、WETH比較、RPC計測。
- `assess.py`: 証拠から確認できた挙動だけを返す実験用判定。RPC障害、DEX停止、残高/allowance不足を売却制限の証拠にしない。
- `test_probe.py`: 記録した実例と失敗ケース、公開RPCへの書き込み拒否、キャッシュ、取得上限、3回までのバックオフ再試行を検証。
- `evidence/`: 判定に必要な記録。巨大なbytecodeと無関係の状態は削減。

これは、対象Token・Pool・Router・管理者・許可設定のABIを指定した検証です。任意のNewPairから未知の許可関数を自動発見する解析器ではありません。関数名の存在だけで危険判定もしていません。

## RPC使用量

| 条件 | 外部Base RPC | RPCエラー | 全体時間 |
| --- | ---: | ---: | ---: |
| 最終シナリオ、取得キャッシュなし | 104回 | 0回 | 約38.1秒 |
| 同じ過去状態のキャッシュを再利用 | 0回 | 0回 | 約2.0秒 |

初回104回の内訳:

| メソッド | 回数 |
| --- | ---: |
| `eth_getStorageAt` | 65 |
| `eth_getCode` | 12 |
| `eth_getBalance` | 12 |
| `eth_getTransactionCount` | 12 |
| `eth_chainId` | 1 |
| `eth_getBlockByNumber` | 1 |
| `eth_getBlockByHash` | 1 |

通常・管理者・許可後・制限フラグ比較・WETH比較を含む、検証全体の値です。単純な1回のquoteに必要な回数ではありません。Base RPC待ち時間の合計は約27.1秒。初回forkのキャッシュ応答は83回でした。

上表には、対象探索・公開ソース取得・失敗原因の調査を含めていません。今回のプローブに記録した探索・開発中の試行も含めるとBase RPCは530回でした。ほかにプローブの公開ソースHTTP要求6回があります。ブラウザ調査等はこの計測外です。

事前のメタデータ調査では429が8回発生しました。fork用取得では、取得失敗時は初回に加えて最大3回、2秒・4秒・8秒のバックオフで再試行し、それでも失敗したら判定を進めず取得を停止します。上限は1実行250回。失敗を「売却不可」とみなしません。

RPC事業者の課金単位とRPC回数は同一とは限りません。過去状態・traceの量によってコストも変わります。また、追加0回という結果は同一ブロックの再利用に限ります。新しいブロックのToken状態を、古いstorageのキャッシュで代用する設計ではありません。

## 基本情報・owner・proxyスロットの意味

| 情報 | 内容 | これだけでは分からないこと |
| --- | --- | --- |
| `name()` / `symbol()` / `decimals()` | 名称、略号、小数桁。例: decimals=6ならrawの1,000,000が1 Token | 売却可否、税率、許可リスト |
| `owner()` | その実装が管理者として返すアドレス。保有者一覧ではない | 全ての管理権限。ゼロでも他のroleや外部管理契約が残る場合がある |
| EIP-1967 implementation slot | proxyから処理を委譲する実装アドレス | 実装内部の制限・更新権限の全体 |
| EIP-1967 admin slot | この方式で使われるproxy更新管理者のアドレス | UUPS等の別方式の認可条件 |
| EIP-1967 beacon slot | 実装アドレスを管理するbeaconのアドレス。beaconの`implementation()`を辿る | beaconの更新権限・将来の実装 |

slotはJSONメタデータではなく、コントラクトstorage内の32バイトの保存場所です。EIP-1967の決まった位置を`eth_getStorageAt`で読みます。空でも独自proxyを否定できません。`owner()`はERC-20の必須項目ではなく、name/symbol/decimalsもERC-20仕様では任意です。

現行MarketHubの取得箇所は `apps/markethub/internal/evmpair/evm.go` の `AssessMetadata`。基本情報・owner・3種類のスロットを観測します。この取得完了は安全性の判定ではありません。

## 公開ソースとの対応・限界

[CertiKの実例報告](https://www.certik.com/blog/VRiiQwezlkHMeJO4fK5lz-the-proliferation-of-honeypot-contracts-in-web3)を対象探索に利用しました。今回の結果は、同社APIの判定値の転載ではなく、Baseの実データをローカルで実行した結果です。

比較調査した次の2コントラクトでは、BaseScan掲載の同一ソースに、自動greylist登録、購入ブロックの記録、block数による拒否、whitelist例外、ownerのみが設定できる関数を確認しました。

- [0xdb72d11dbc54f24c9e61ee420d3635b5bcb712c7](https://basescan.org/address/0xdb72d11dbc54f24c9e61ee420d3635b5bcb712c7#code)
- [0x7387856052de6414ef805c0487281d9b2725e844](https://basescan.org/address/0x7387856052de6414ef805c0487281d9b2725e844#code)

取得ソースのSHA-256は両方 `08079a4fea5f958ff6c75c52f018ea29dfb27aba9ae09de26a9addaf01527ed5`。これはソースの比較であり、この2 Tokenの現在の制限が有効であることを実証したものではありません。

ZEBRA自体の公開検証済みソースは今回取得できていません。比較用ソースをそのままZEBRAのソースと断定していません。ZEBRAの判定根拠は購入・売却trace、状態差分、1スロットの比較、管理者による許可設定の実行です。

サンプル数は動作検証1 Token + WETH送金比較、ソース比較2件です。一般的な検出率や誤判定率は測っていません。今回のforkは現行AnvilのOptimism実行環境で過去のstorage/codeを利用しており、過去の全L2プロトコル条件を厳密に再現したチェーン検証器ではありません。gas課金の厳密な再現も目的外です。

将来の設定変更、proxy更新、外部コントラクト依存、数量や時刻・呼出者による別分岐まで「安全」と保証するものではありません。成功した経路についても `safe` という結果は返しません。

## 責務と次の実装判断

- **onchain**: 固定ブロックでのcode/storage取得、実装先の解決、呼び出し・状態変化の分析、検証可能な証拠の生成。
- **MarketHub**: NewPairと結び付けた観測結果の保存・配信、再評価のスケジュール。
- **TradeHub**: 条件での候補選別と実行直前の最新状態・実際の署名アドレス/数量/経路による再評価。

現行の基本情報取得だけでは検出できない制限を、追加の動作検証で検出できることは確認できました。本番化は、少数の対応パターンから始め、解析不能を未確定のまま返す方式が妥当です。公開APIの返却項目・実行対象の選別・RPC予算は、本番実装前に設計レビューが必要です。

## 再現・チェック

`onchain`ディレクトリで実行します。Python標準ライブラリとDockerのみを使います。初回は固定imageの取得と公開Base RPCへのアクセスが必要です。

```sh
python3 -m unittest discover -s experiments/token_restrictions_20260921 -p 'test_*.py' -v
python3 experiments/token_restrictions_20260921/probe.py --mode fork --output /private/tmp/token-restrictions-reproduce/run
```

同じ親ディレクトリにあるRPCキャッシュを再利用します。キャッシュなしを計測する場合は新しい親ディレクトリを指定します。固定localhost port 18545/18546と専用Docker container名を使うため、同時に複数実行しません。

実行結果は指定ディレクトリ内の `assessment.json`、`ordinary.json`、`owner.json`、`weth_control.json`、`requests.json`、`local_calls.json`、`fork_summary.json` に保存します。プローブは終了時に自分が起動したcontainerだけを停止します。

検証結果: 回帰テスト15件PASS、forkで全シナリオの期待した結果を確認。変更は実験用Python・JSON・文書のみのため、Goのbuild/test、Gateway/MarketHub/TradeHubの起動、Agent E2Eは対象外です。

仕様の出典: [ERC-20](https://eips.ethereum.org/EIPS/eip-20)、[ERC-1967](https://eips.ethereum.org/EIPS/eip-1967)。
