# atelier hooks — 機械的ゲート

`atelier-gate.sh` は PreToolUse hook として動く**薄い入口**で、判定の実体は atelier バイナリ(`issue verify` / `pr verify`)に一本化されている。拒否は exit 2 + stderr の理由で行われ、エージェントはその理由をそのままエスカレーション材料に使える。

`atelier-bootstrap.sh` は SessionStart hook として動く**ゲートバイナリのセルフブートストラップ**(ADR 0003)。バイナリの欠落/ピン不一致を検知したとき、公開 Releases から curl で取得し、プラグイン同梱のピン(`pin/version` + `pin/checksums.txt`、レビュー済み commit が信頼の根)との照合が成功した場合のみ `.agents/bin/` に配置する。失敗(オフライン・チェックサム不一致)は何もせず終了し、ゲートの fail-closed + 手動 1 行案内が現行どおり働く — ブートストラップは利便であって防御の前提ではない。不一致の検知は配置時に書くマーカー(`.agents/bin/atelier.version`)との比較で行い、全マシンをプラグインのピンへ収束させる(windows は対象外 — 従来の手動手順)。挙動は `tools/atelier/internal/bootstrapscript` の Go テストで固定されている。

## 登録(自動)

`hooks/hooks.json` により、**プラグインを有効化すると自動で登録される**(Claude Code のプラグイン hooks 機構)。手動の settings 編集は不要。

- hooks はセッション開始時に読み込まれる(プラグイン更新後は新しいセッションで反映)
- 登録状態は `/hooks` で確認できる
- 実行時の依存: `bash` / `jq` / `gh` / atelier バイナリ(`.agents/bin/atelier` または PATH — SessionStart のブートストラップが自動取得し、/atelier:init でも取得できる。無い場合、マージゲートは fail-closed で停止する)。ブートストラップ自体は `curl` / `tar` / `sha256sum`(または `shasum`)のみに依存し、`gh` と GitHub 認証を要求しない

## ゲート一覧

| ゲート | 判定 |
| --- | --- |
| main 直 push / force push | 常にブロック(git-workflow) |
| push ゲート | `agent/issue-<n>-*` ブランチの push 前に `atelier issue verify`(ラベルなしの実装は push 不可) |
| マージゲート | `atelier pr verify` + Closes 紐づけ + 紐づく issue の `merge:agent` + CI green + **atelier-review status = success かつ投稿者 ≠ PR 作者**(merge-policy の全実行条件。投稿者・作者が特定できない場合も fail-closed でブロック — 独立の (d) 資格情報の機械検証) |
| リリースゲート | タグ push(`--tags` / `refs/tags/` / `atelier/v*`・旧名 `factory/v*`)と、残存する旧版バイナリのリリースコマンド起動をブロック — デプロイは人間の tag push(merge-policy) |

## 検証手順

```bash
# 1. main 直 push が止まること
echo '{"tool_name":"Bash","tool_input":{"command":"git push origin main"}}' \
  | bash atelier-gate.sh; echo "exit=$?"   # => exit=2

# 2. merge:agent なしの PR のマージが止まること(実 PR 番号で)
echo '{"tool_name":"Bash","tool_input":{"command":"gh pr merge 123"}}' \
  | bash atelier-gate.sh; echo "exit=$?"
```

## 限界(認識の上で運用する)

- hook は **Claude Code 経由の操作しか縛れない**(L2)。curl + token の直叩きはサーバー側(#17 の GHA required check = L3)の守備範囲
- 文字列マッチの迂回は原理的に可能。ここでの目的は敵対防御ではなく**事故防止**(fail-closed の機械化)
