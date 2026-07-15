---
description: Analyze open GitHub pull requests and list them in a single terminal-friendly table
tools:
  - Bash(gh pr list, gh pr view, gh repo view)
  - Read
---

GitHub の open PR を **単一のテーブル**で一覧表示します（issues の PR 版）。

## 手順

1. PR を取得する（引数があれば `--label` / `--state` / `--search` に素通しする）:

   ```sh
   gh pr list --state open --limit 100 --json number,title,url,isDraft,headRefName,baseRefName,reviewDecision,mergeable,statusCheckRollup,updatedAt
   ```

2. **テーブルは 1 つだけ**にする。カテゴリやレーンで複数テーブルに分割しない。

3. 列構成（この順）:

   | 列 | 内容 |
   | --- | --- |
   | # | **生 URL をそのまま書く**（例: `https://github.com/owner/repo/pull/123`）。`[#N](url)` の markdown リンク形式はターミナルでクリックできないため**禁止**（ターミナルは生 URL を自動リンク化する） |
   | 件名 | PR タイトルの**短い要約（全角 15〜20 字目安）**。原文をそのまま貼らず意訳して詰める。プレフィックス（`@rsi/ui:` 等の対象）は残す |
   | base←head | `main ← agent/issue-120` の形式。stack（base が main 以外）は一目で分かるようにする |
   | CI/レビュー | CI 集計（green/fail/実行中）+ mergeable + reviewDecision を短く |
   | 状況 | **1〜2 文の説明**: いまどういう状態か・何を待っているか・次のアクションは何か。CI 状態の言い換え（「green」だけ等）で済ませない。会話・PR 本文/レビュースレッド（あれば運用台帳）から根拠を取る |

4. 状況の導出規則（状態ベース・**絵文字で色分けする**。ターミナルの markdown 表は文字色を持てないため絵文字が色の代替）:
   - CI 全 green + MERGEABLE + レビュー指摘なし → 🟢 人間マージ待ち
   - CI 実行中（pending あり・fail なし）→ ⏳ CI 待ち
   - CI に fail あり → ❌ 要修正（fail したチェック名を状況に書く）
   - `CONFLICTING` → ⚠️ 競合（rebase 要。**merge 禁止・rebase のみ**が repo ルール）
   - reviewDecision が CHANGES_REQUESTED、または最新レビューコメント末尾が人間の指摘で未返信 → 🔴 レビュー対応中/要対応
   - isDraft → 📝 draft
   - stack PR（base ≠ main）→ 🔗 を併記し、base PR 番号を状況に書く
   - 絵文字は状況文の先頭に付ける。**状況の本文は絵文字と別に必ず書く**（推測で書かず、根拠が無い項目は「詳細未読・状態のみ」と正直に書く）
   - 凡例をテーブルの直前に 1 行で示す

5. `statusCheckRollup` の判定は「SUCCESS/NEUTRAL/SKIPPED 以外の conclusion」を fail、conclusion 未確定（null/PENDING 等）を実行中として数える（pending→成功のノイズを fail 扱いしない）。

6. 並び順は PR 番号の降順（新しい順）。stack がある場合はマージ推奨順（依存順）を気づきに添える。

7. テーブルの後に、気づき（マージ推奨順・stale・レビュー未返信スレッド等）があれば 2〜3 行の箇条書きで添える。無ければ省略。
