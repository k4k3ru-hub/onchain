# Pool作成イベントを起点にした4 Venue・40 Poolの検証

観測日: 2026-09-20 UTC。レポート作成: 2026-09-21 JST。

**40 Poolを採取し、16 Poolは自動処理で現在の割合を0%、2 Poolはソースの人手確認とRPC検証を加えて100%と算出した。残る22 Poolは未確定。任意の保管先を自動判定できる段階には達していない。**

今回の入力にはUNCXの一覧・API・専用アダプター・既知のlocker登録を使っていない。前回のUNCXとの比較事例は今回の40件に含めない。公開ソースの配布元としてSourcifyを利用し、取得したソースは別途コンパイルしてオンチェーンの実行コードと照合した。

| Venue / deployment | Pool数 | 自動算出 | 人手のソース確認を加えた算出 | 未確定 |
|---|---:|---:|---:|---:|
| Uniswap V3 / Base mainnet | 10 | 9件: 0% | 0 | 1 |
| Aerodrome Slipstream / Base mainnet・現行factory | 10 | 7件: 0% | 2件: 100% | 1 |
| Meteora DLMM / Solana mainnet | 10 | 0 | 0 | 10 |
| Cetus CLMM / Sui mainnet | 10 | 0 | 0 | 10 |

算出値は観測時点の設定・持分に対する値。0%は検証した所有者の全量減少経路が利用可能という意味であり、売却可能性やトークンの健全性の評価ではない。100%の2件も将来の設定変更を含む保証ではない。LPロック割合、設定変更権限、保有集中度、トークン側権限は別々の評価項目として維持する。

## サンプリングと入口

- EVM: factoryの`PoolCreated`ログを新しい順に10件固定し、その後に保管先を調査。作成receipt、factoryから得たPool、Mintログ、NFT、現在の所有者へ進む。Uniswap V3はblock `51561375`、Aerodromeは`51561574`で固定し、最後にblock hashを再照合した。
- Aerodromeは[公式deployment一覧](https://raw.githubusercontent.com/aerodrome-finance/slipstream/main/README.md)のfactory `0xf8f2eB4940CFE7d13603DDDD87f123820Fc061Ef`、NPM `0xe1f8cd9AC4e4A65F54f38a5CdAfCA44f6dD68b53`が対象。旧factoryのPoolや他のAMM形式はこの10件に含まない。
- Meteora: 公式Data APIの新規Pool一覧を候補発見に利用し、各Poolの最古のトランザクションをRPCで取得して`LbPairCreate`を照合した。作成イベントを10件固定した後、PoolをキーにPositionV2を検索した。**作成イベント自体を入口とする処理は検証したが、連続稼働するNewPair SubscribeのE2Eではない。候補一覧の網羅性も検証していない。**
- Cetus: GraphQLの`CreatePoolEvent`から10件を固定し、checkpoint `324867755`でPool・持分・所有者を照会。照会形式の修正後も同じmanifest・checkpointを再利用した。
- 比率や保管先を見てサンプルを交換していない。取得失敗、予算上限、未対応のPoolも残した。

全40件のアドレス、イベント、判定理由は[summary.json](testdata/venue_samples_20260920/summary.json)、各Venueのmanifest・samplesに保存した。

## 自動化できた部分

EVMでは既存の`evm/clliquidity`を使い、全tickのgross/net、現在価格、active liquidityを照合し、発見したNFT群で全持分が説明できることを確認した。そのうえで全所有者から全量の`decreaseLiquidity`を`eth_call`で実行し、成功したPoolだけ0%とした。未確定の保管先が1つでも残る場合や全持分を説明できない場合はnullを維持する。NFT本数だけで割合は算出していない。

Uniswap V3の1 Poolにはコード長23 byteの所有者が2人いた。[EIP-7702](https://eips.ethereum.org/EIPS/eip-7702)の委譲形式を厳密に識別し、同規格が許す所有者からの直接トランザクション経路で全量減少が成功したため、このPoolも0%と算出できた。単にコードがあるだけではlockerとは判定できない。

Uniswap V3では11 NFT・5所有者、Aerodromeでは解析できた9 Poolで10 NFT・7所有者を観測。Aerodromeの5 Poolは同じEOAが所有していた。Pool数に比べて実装・所有者の多様性は小さい。

## UNCX以外のロック実装で確認したこと

Aerodromeの次の2 Poolから、NFTの所有者としてERC-1167 proxyを発見した。

| Pool | NFT | 保管先 | token0 / token1のロック割合 |
|---|---:|---|---|
| `0xb0Bc332F85Ab53F3A74A0e5cD713E373528eBB2e` | 6577077 | `0x29f4dc9fd61a76f848bb607d2862fef11ba85b7a` | 100% / 100% |
| `0x39F197dc4c4ADEA97A4fd478e2015930AC2Efbf1` | 6577053 | `0x2b7e3decd50dbfe052d5a21ca8640ec202392485` | 100% / 100% |

proxyのコードから実装`0xfe678bffc3c1c8d1de4478cc5c3e1b93ee4638ae`を発見し、[公開ソース](https://sourcify.dev/server/v2/contract/8453/0xfe678bffc3c1c8d1de4478cc5c3e1b93ee4638ae?fields=all)の`CLLocker`、そのgetterから[Factory](https://sourcify.dev/server/v2/contract/8453/0x932d0b4c00a2a33ef1ec5fe0aa981bd1a00a6f5c?fields=all)へ進んだ。名前・アドレスを事前登録して発見したものではない。同じ1種類の実装の2 instanceであり、独立した2種類のlockerを検証したわけではない。

ソースと状態を確認した結果:

1. 各Poolの全持分は1 NFTで説明でき、そのNFT・NPM・Poolが保管先のgetterと一致した。
2. 保管先の`lockedUntil`は両方とも`4294967295`。この値だけでは認定せず、NFTの出口を追跡した。
3. 保管先の直接unlockはFactoryだけが呼び出せる。期限チェックはFactory側に存在する。
4. 別の出口であるmigrationは、観測時点の`newLockerFactory = zero address`で停止していた。stakeによるNFT移動も、観測時点で対応gaugeがなく停止していた。
5. 所有者をcallerに指定したunlock・migrate・stake・直接unlockは、それぞれソースで特定したcustom errorでrevertした。RPCの通信失敗をrevertとして扱っていない。状態確認・前後のblock hashを含む15確認すべてが期待値に一致した。
6. 残る公開関数、初期化保護、ERC20操作とNFT操作の区別を人手で確認した。現在の設定を保ったまま対象NFTを引き出す別経路は、この確認範囲では見つからなかった。

**ここには人手のソース確認が含まれる。** 既存のAST解析器にも同じソースを入力したが、record shapeが0件となり、Factoryをまたぐ制御フローの判定は未対応だった。自動処理の出力はnullのまま残し、100%の結果を[別の検証記録](testdata/venue_samples_20260920/aerodrome-manual-reviewed-result.json)へ保存した。

Factoryにはownerだけがmigration先を設定できる経路がある。現在のowner権限の有効性までは評価していない。`4294967295`も、現在のコード・期限判定とともに扱い、無期限・変更不能という保証には置き換えない。

## 未確定の理由

- **Uniswap V3: 1 Pool。** 130 byteの保管先コードと、その公開ソース取得失敗を記録。実装先・権限・解除経路の検証が未完了。コード長からロック判定しない。
- **Aerodrome: 1 Pool。** `0x2a7bc428a1b986Ed5938a11878224DdB52039Ac0`には188件のMintトランザクションがあり、1 Poolあたり64 receiptの調査予算を超えた。別Poolには置換せず、全持分未確認として保持。差分索引や一括receipt取得が次の改善点。
- **Meteora: 10 Pool。** 13 PositionV2・9所有者を発見し、全件で`lock_release_point = 0`を取得。ただし、全Binの持分とPositionV2拡張部分の照合、所有者/PDAの署名権限、プログラムの解除処理を検証しきれていない。列挙slotと本体snapshot slotにも差がある。0%とはしていない。
- **Cetus: 10 Pool。** 9件は持分あり、1件はposition一覧とtick一覧のサイズがともに0で、割合の分母がない。持分のある3 Poolで親オブジェクトを最後まで取得できず、そのうち1 Poolでは持分オブジェクト2つも取得できなかった。取れないことを焼却・永久ロックと解釈しない。残る6 Poolでも全tickとの照合と解除処理の検証が必要。

Meteoraの[公式SDK](https://raw.githubusercontent.com/MeteoraAg/dlmm-sdk/main/ts-client/src/dlmm/index.ts)には、activation typeに応じたrelease pointとBin情報からロック持分を扱う処理がある。ただし、SDKが値を読む処理と、配備済みプログラムが全出口で制約を守ることの検証は分ける。Cetusの[公開interface](https://raw.githubusercontent.com/CetusProtocol/cetus-clmm-interface/main/sui/cetus_clmm/sources/pool.move)は関数本体がstubであり、そのファイルだけでは配備済みMoveコードの解除条件を検証できない。

## RPC利用量

下表の単位は**外部RPC/GraphQLリクエストの試行回数**。multicall内部のコントラクトread数やproviderの課金単位とは異なる。再試行も含む。取得済みデータのローカル解析、コンパイル、人手レビュー時間は含めない。

| Venue | 採取・基本解析 | 成功した追加解析フロー | 公開情報のGET | 基本解析の実測時間 |
|---|---:|---:|---:|---:|
| Uniswap V3 | 133 RPC | 7 RPC | 1 GET、ソース未取得 | 約198秒 |
| Aerodrome | 212 RPC | 59 RPC | 2 GET | 約317秒 |
| Meteora DLMM | 50 RPC | 0 | 2 GET（公式一覧・IDL） | 約88秒 |
| Cetus CLMM | 61 GraphQL、初回と再照会の合計 | 0 | 0 | 初回とは別に再照会約30秒 |

- Uniswapの追加7 RPC: コード再取得、EIP-7702所有者の全量減少、前後block照合。
- Aerodromeの追加59 RPC: コード/実装照合5、ABIからのgetter38、Factoryコード取得1、状態・解除経路・前後block照合15。
- EVM基本解析のHTTP 429再試行はUniswap 2回、Aerodrome 13回で、基本解析の数字に含まれる。
- Cetusは初回22回、修正後39回。最初の2回がmanifest作成で、照会上限を50件へ直した経路だけなら41回。今回実際に保存した両実行は61回である。
- 別途記録した開発中の失敗: AerodromeのUser-Agentが拒否された13 HTTP試行。これを追加すると、保存済みの全フローで**535 RPC/GraphQL HTTP試行、公開GET 5回**。
- 上記535にはCetusの単発エラー診断1回、最初のsandbox通信失敗1回、途中停止して明細を保存できなかった最初のgetter照会は含めない。したがって、セッション全体の全通信回数ではない。初回getter照会の回数は復元できず、総通信量を正確に断定しない。

将来のNewPair Subscribeでは作成イベントを既に受信しているため、今回の過去イベント探索を毎回繰り返す必要はない。一方、持分やlockの追加・移転・期限経過・設定変更を追う再評価コストは別途発生する。

## 実装方針へ反映する内容

- `onchain`の責務: 作成イベントからの持分発見、全持分のカバー率、所有関係、コード・公開ソースの検証、現在状態に対する解除経路の証拠。
- `MarketHub`の責務: 取得時点・未確定理由・対応範囲を含む観測結果の公開、更新通知。
- `TradeHub`の責務: ユーザーの条件に対し、未確定を許容するかも含めて取引可否を判断すること。
- 次のEVM対応単位は、特定locker名の追加ではなく、**固定proxy → scalar状態 → 外部Factoryの条件 → 設定変更可能なgate**という今回発見した構造。
- Solanaは全Bin・Position拡張・PDA権限、Suiは持分とtickの照合・ラップされた所有関係・Moveの解除経路をそれぞれ実装する。異なる環境でも「全持分・出口・観測時点が揃わなければnull」という結果の意味を揃える。
- 各10件は今回の採取条件を満たすが、非UNCXの肯定例は実装1種類。追加のロック実装、解除済み、部分ロック、設定変更直後などを別の固定cohortで増やす必要がある。

公開API、status enum、production依存、DBは変更していない。研究コードは`go/internal/experiments/lplock/`に隔離し、mainでの作業許可範囲内で追加した。commit/pushは行っていない。

## 再実行と確認

以下は`onchain`ディレクトリで実行する。ネットワーク照会はすべて読み取り専用。`sampling_non_evm.py`等の`--legacy-dir`は既存の検証用Readerを明示的に利用するために必要で、production SDKの依存ではない。Meteora/Suiの過去状態を一般的なRPCから常に再取得できるとは限らないため、保存済みJSONを証拠として残している。

```sh
(cd go && ONCHAIN_SAMPLE_VENUE=uniswap-v3 ONCHAIN_SAMPLE_DIR=/tmp/lp-venue-samples go test ./internal/experiments/lplock -run '^TestLPVenueSamplesLive$' -count=1 -timeout 30m -v)
(cd go && ONCHAIN_SAMPLE_VENUE=aerodrome ONCHAIN_SAMPLE_DIR=/tmp/lp-venue-samples go test ./internal/experiments/lplock -run '^TestLPVenueSamplesLive$' -count=1 -timeout 30m -v)
python3 go/internal/experiments/lplock/sampling_non_evm.py meteora-dlmm --legacy-dir ../k4k3ru/docs/internal/experiments/newpair_lp_lock_20260920 --env-file ../k4k3ru/.env.markethub.local --output /tmp/lp-venue-samples
python3 go/internal/experiments/lplock/sampling_non_evm.py cetus-clmm --legacy-dir ../k4k3ru/docs/internal/experiments/newpair_lp_lock_20260920 --env-file ../k4k3ru/.env.markethub.local --output /tmp/lp-venue-samples
# 同じCetus cohortを再照会する場合は、上のコマンドに次を追加する。
# --manifest /tmp/lp-venue-samples/cetus-clmm-manifest.json
python3 go/internal/experiments/lplock/sampling_custody.py --legacy-dir ../k4k3ru/docs/internal/experiments/newpair_lp_lock_20260920 --samples /tmp/lp-venue-samples/uniswap-v3-samples.json --output /tmp/lp-venue-samples
python3 go/internal/experiments/lplock/sampling_custody.py --legacy-dir ../k4k3ru/docs/internal/experiments/newpair_lp_lock_20260920 --samples /tmp/lp-venue-samples/aerodrome-samples.json --output /tmp/lp-venue-samples
python3 go/internal/experiments/lplock/sampling_getters.py --legacy-dir ../k4k3ru/docs/internal/experiments/newpair_lp_lock_20260920 --custody /tmp/lp-venue-samples/aerodrome-custody.json --output /tmp/lp-venue-samples
python3 go/internal/experiments/lplock/sampling_dependency.py --legacy-dir ../k4k3ru/docs/internal/experiments/newpair_lp_lock_20260920 --address 0x932d0b4c00a2a33ef1ec5fe0aa981bd1a00a6f5c --block 51561574 --output /tmp/lp-venue-samples
# このplanは上記の固定blockに対する人手レビューの成果物。
python3 go/internal/experiments/lplock/sampling_plan_probe.py go/internal/experiments/lplock/testdata/venue_samples_20260920/aerodrome-manual-review-plan.json /tmp/aerodrome-review-probes.json
(cd go && go test ./internal/experiments/lplock ./evm/clliquidity)
(cd go && go vet ./internal/experiments/lplock)
ONCHAIN_SOLC_MODULE=/tmp/onchain-lock-analysis-solc/node_modules/solc python3 -m unittest discover -s go/internal/experiments/lplock -p 'test_*.py' -v
```

実行した追加のコンパイル照合コマンド:

```sh
npm install --prefix /tmp/lp-lock-solc-0830 --no-package-lock --no-save solc@0.8.30
node go/internal/experiments/lplock/compile_source.cjs /tmp/lp-lock-solc-0830/node_modules/solc /tmp/lp-venue-samples/aerodrome-source-0xfe678bffc3c1c8d1de4478cc5c3e1b93ee4638ae.json /tmp/lp-venue-samples/aerodrome-compiled.json
node go/internal/experiments/lplock/compile_source.cjs /tmp/lp-lock-solc-0830/node_modules/solc /tmp/lp-venue-samples/dependency-source-0x932d0b4c00a2a33ef1ec5fe0aa981bd1a00a6f5c.json /tmp/lp-venue-samples/aerodrome-factory-compiled.json
```

Go関連テスト・vet成功。PythonはSolidityコンパイラを設定して21テスト成功。追加Pythonスクリプト5本の`py_compile`も成功した（最初のキャッシュ書込制限を解消するため`PYTHONPYCACHEPREFIX=/tmp/lp-sampling-pycache`で再実行）。2つの実行コード照合には、一時ディレクトリの`solc@0.8.30`と`compile_source.cjs`を使用した。Goファイルはgofmt済み。productionコード変更がないため全サービスのbuild、全Go packageのtest、Agentからの取引E2Eは今回の検証対象外。

追加ファイル: `sampling_evm_test.go`、`sampling_non_evm.py`、`sampling_custody.py`、`sampling_getters.py`、`sampling_dependency.py`、`sampling_plan_probe.py`、`test_sampling.py`、本レポートと`testdata/venue_samples_20260920/`。既存の研究コードを利用し、READMEにレポートへのリンクを追加した。
