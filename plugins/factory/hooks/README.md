# factory hooks — 機械的ゲート

`factory-gate.sh` は Claude Code の PreToolUse hook として動く**薄い入口**で、判定の実体は factory バイナリ(`issue verify` / `pr verify`)に一本化されている。拒否は exit 2 + stderr の理由で行われ、エージェントはその理由をそのままエスカレーション材料に使える。

## ゲート一覧

| ゲート | 常時 / 無人時 | 判定 |
| --- | --- | --- |
| main 直 push / force push | 常時 | 常にブロック(git-workflow) |
| push ゲート | 常時 | `agent/issue-<n>-*` ブランチの push 前に `factory issue verify`(ラベルなしの実装は push 不可) |
| マージゲート | 常時 | `factory pr verify` + Closes 紐づけ + 紐づく issue の `merge:agent` + CI green(merge-policy) |
| 改憲ブロック | 無人時のみ | `docs/adr/` への Write / Edit を拒否(改憲は対話専用) |
| merge:agent 付与ブロック | 無人時のみ | ラベル付与・変更コマンドを拒否(付与は grooming 限定) |
| 配車ゲート | 無人時のみ | Task 起動前に対象 issue を `factory issue verify` |

無人モードの判定は sentinel ファイル **`.agents/unattended`** の存在(night スキルが作成・削除する)。

## 設置(人間が行う)

**ユーザーレベルの settings**(`~/.claude/settings.json`)に追記する。プロジェクトの settings はエージェント自身が触れる場所のため、防護としてはユーザーレベルに置くこと(hooks はセッション開始時にスナップショットされる):

```json
{
  "hooks": {
    "PreToolUse": [
      {
        "matcher": "Bash|Write|Edit|Task",
        "hooks": [
          {
            "type": "command",
            "command": "bash <plugin-cache-path>/factory/hooks/factory-gate.sh"
          }
        ]
      }
    ]
  }
}
```

`<plugin-cache-path>` はプラグインの実体パス(/factory:init が検査時に実パスを提示する)。

依存: `bash` / `jq` / `gh` / factory バイナリ(`.agents/bin/factory` または PATH — /factory:init が取得を提案する)。

## 検証手順(設置後)

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
