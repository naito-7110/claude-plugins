---
name: help
description: atelier の入口(read-only)。使い方の案内(どの場面でどのスキルか)と現状共有(設置/運転/ゲート/作業状態の診断表)に答える。「atelier どう使うの」「いまどういう状態?」と聞かれたら使う
tools:
  - Bash(gh, atelier, git, ls, cat)
  - Read
  - Glob
  - Grep
---

**一切書き込まない**(診断と案内のみ。修正が必要なら該当スキル・bin を提示して誘導する)。聞かれ方に応じて 2 つの顔を使い分け、聞かれていない顔まで長々と出さない。

## 顔 1: 案内(「どう使うの?」)

まず日常の型を示す:

| 場面 | 入口 | 人間ゲート |
| --- | --- | --- |
| 思いつき・要望を放り込む | issue を作るだけ(形式不問)→ /atelier:triage が整形 | — |
| 仕様を固める | /atelier:groom(複数ドメインに跨るなら /atelier:huddle が先) | ✅ 受け入れ条件の承認・merge:agent 付与はここだけ |
| 1 件やらせる | 「issue #N をやって」(/atelier:work) | PR レビュー(merge:agent なしの場合) |
| まとめて回す | /atelier:orchestrate(「バックログを進めて」) | エスカレーション対応 |
| 憲法を変える | /atelier:adr | ✅ 対話専用 |
| ドメインを切る | /atelier:domains | ✅ 対話専用 |
| やめる | /atelier:uninstall | ✅ |

個別の詳細は本文を複製せず**選択読み**で答える: 各スキルの実体は `${CLAUDE_PLUGIN_ROOT}/skills/<name>/SKILL.md`、プリセット憲法の索引と選択読みマッピングは `${CLAUDE_PLUGIN_ROOT}/adr/README.md`、全体像は `${CLAUDE_PLUGIN_ROOT}/README.md`。

## 顔 2: 現状共有(「いまどういう状態?」)

4 群を機械的に収集し、1 枚の表 + 問題があれば「次にやること」で締める:

- **設置**: `.atelier/`(README / ownership.yml / flags.yaml)・`.github/`(テンプレート・atelier-issue-check.yml・dependabot.yml)・CLAUDE.md のマーカー節・運用ラベル 6 種・ボードのリンク
- **運転**: atelier バイナリの有無・版
- **ゲート**: hook の有効性(プラグイン有効化 + 依存 jq/gh)・branch protection の required contexts(atelier-issue-check / atelier-review)
- **作業状態**: `agent-wip` / `needs-human` / Ready(`agent-ok`)の件数・open の agent PR(レビュー待ち / atelier-review 状態)・制動条件への接近(人間レーン PR 滞留・エスカレーション滞留)

各行は ✅ / ⚠️(縮退動作あり)/ ❌(要対応)で判定し、❌ には解決コマンドか該当スキルを添える。

## 禁止事項

- あらゆる書き込み・状態変更(gh の変更系・atelier の変更系サブコマンド・ファイル作成)
