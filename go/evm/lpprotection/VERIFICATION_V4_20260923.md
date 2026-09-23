# LP保護：reorg・receipt再収録の修正検証

2026-09-23。対象は `lp-protection-20260923-v4`。`onchain/main` の `29c1d41` に対する修正。

## 修正結果

1. **承認履歴の異なるfork間での混在を防止。** 新しい区間の末尾headerを取得前後で照合する間に、取得済み区間の末尾（最初は検証済み作成block）もRPCで再確認する。不一致なら作成証拠・承認履歴・保存済み取得応答を無効化する。最終観測hashの不一致でも同様に破棄する。
2. **別blockへ再収録された取引のreceiptを再取得。** 入力イベントとcacheのblockが食い違う場合、そのreceiptと依存する作成証拠・承認履歴を無効化してから同じ取得予算内で取り直す。取得失敗・予算切れ・新しい応答の不一致では未判定を維持する。無関係な承認履歴の失敗回数は保持する。

公開の型・フィールド・constructor・RPC interfaceの変更、production依存の追加はない。
内部保存証拠のモデル版をv4へ更新した。v3以前の証拠は再構築し、版文字列だけを書き換えて再利用しない。
入力の`Request.Receipts`や`Evidence`は変更しない。呼出し側が持つ古いreceipt cacheは呼出し側でも破棄・置換する。

## 変更ファイル

同じ `go/evm/lpprotection/` 配下のみ。

- `approvals.go`、`creation_rpc.go`：保存済み区間・作成blockの再確認。
- `rpc.go`、新規`receipt_cache.go`：receipt取得の共通化、再取得・証拠無効化、最終観測のreorg処理。
- `types.go`、`reader.go`、`evidence.go`：モデル版・公開関数の説明。
- 新規`reorg_test.go`、`reader_test.go`、`evidence_test.go`：回帰テストと旧版拒否。
- `README.md`、`EVIDENCE.md`、本書：取得コスト、再利用・移行方針、検証結果。

## 回帰テスト

修正前に、区間間のreorgとreceipt再収録の再現テストが失敗することを確認した。修正後は以下を確認。

- 旧forkの先行区間に対し、新forkで承認イベントが増えた場合、混在した証拠を保存しない。JSON保存・復元後の再評価では、その承認を検出する。
- 途中の取得失敗前でも混在を検出する。作成blockおよび最終観測でのreorgでも証拠を破棄する。
- 末尾の再確認RPCが一時失敗した場合、確認済み区間までを保持し、未完了区間と失敗回数を保存する。次回はその区間から再開する。
- 保存証拠・リクエストcacheのどちらが古くても、移動したreceiptを1回のRPCで取り直し、検証した応答を次回再利用する。
- receipt再取得のRPC失敗・予算上限・古いRPC応答では未判定とし、古いreceiptを保存し直さない。次回の取得成功で復旧する。
- 依存する作成証拠・承認履歴を破棄し、無関係な履歴の失敗回数と入力オブジェクトを保持する。
- v3以前の保存証拠を拒否する。

## 実行した確認

作業ディレクトリ `onchain/go`。すべて成功。

```sh
gofmt -w <変更したGoファイル>
go test ./evm/lpprotection ./evm/clliquidity
go test ./...
go test -race ./evm/lpprotection ./evm/clliquidity
go vet ./evm/lpprotection ./evm/clliquidity
go build ./...
GOWORK=off go test ./evm/lpprotection ./evm/clliquidity
git diff --check
```

## 取得コスト・未確認範囲

- 履歴の各区間は、従来の3 RPCに既存末尾／作成blockの再確認1 RPCを追加し、通常4 RPC。genesisから始まる最初の区間は3 RPCのまま。事前予算判定にも反映。
- receipt再取得も既存の追加RPC・receipt上限に含める。同一transactionへのreceipt RPCは1評価で最大1回で、内部リトライは行わない。
- 今回はfake RPCによる決定的なreorg・失敗注入で修正を検証した。実RPC・Agent E2Eは再実行していない。以前の[v3実測](VERIFICATION_V3_20260923.md)は当時の結果であり、v4の取得時間やRPC数の実測ではない。
- MarketHubへのworker・保存・通知の接続は、このonchain修正には含まれない。
