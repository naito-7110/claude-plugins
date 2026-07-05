# factory hooks — 機械的ゲート

`factory-gate.sh` は PreToolUse hook として動く**薄い入口**で、判定の実体は factory バイナリ(`issue verify` / `pr verify`)に一本化されている。拒否は exit 2 + stderr の理由で行われ、エージェントはその理由をそのままエスカレーション材料に使える。

## 登録(自動)

`hooks/hooks.json` により、**プラグインを有効化すると自動で登録される**(Claude Code のプラグイン hooks 機構)。手動の settings 編集は不要。

- hooks はセッション開始時に読み込まれる(プラグイン更新後は新しいセッションで反映)
- 登録状態は `/hooks` で確認できる
- 実行時の依存: `bash` / `jq` / `gh` / factory バイナリ(`.agents/bin/factory` または PATH — /factory:init が取得する。無い場合、マージ・配車ゲートは fail-closed で停止する)

## ゲート一覧

| ゲート | 常時 / 無人時 | 判定 |
| --- | --- | --- |
| main 直 push / force push | 常時 | 常にブロック(git-workflow) |
| push ゲート | 常時 | `agent/issue-<n>-*` ブランチの push 前に `factory issue verify`(ラベルなしの実装は push 不可) |
| マージゲート | 常時 | `factory pr verify` + Closes 紐づけ + 紐づく issue の `merge:agent` + CI green + **factory-review status = success**(merge-policy の全実行条件) |
| 改憲ブロック | 無人時のみ | `docs/adr/` への Write / Edit を拒否(改憲は対話専用) |
| merge:agent 付与ブロック | 無人時のみ | ラベル付与・変更コマンドを拒否(付与は grooming 限定) |
| 配車ゲート | 無人時のみ | Task 起動前に対象 issue を `factory issue verify` |

無人モードの判定は sentinel ファイル **`.agents/unattended`** の存在(night スキルが作成・削除する)。

## 検証手順

```bash
# 1. main 直 push が止まること
echo '{"tool_name":"Bash","tool_input":{"command":"git push origin main"}}' \
  | bash factory-gate.sh; echo "exit=$?"   # => exit=2

# 2. merge:agent なしの PR のマージが止まること(実 PR 番号で)
echo '{"tool_name":"Bash","tool_input":{"command":"gh pr merge 123"}}' \
  | bash factory-gate.sh; echo "exit=$?"
```

## 限界(認識の上で運用する)

- hook は **Claude Code 経由の操作しか縛れない**(L2)。curl + token の直叩きはサーバー側(#17 の GHA required check = L3)の守備範囲
- 文字列マッチの迂回は原理的に可能。ここでの目的は敵対防御ではなく**事故防止**(fail-closed の機械化)
