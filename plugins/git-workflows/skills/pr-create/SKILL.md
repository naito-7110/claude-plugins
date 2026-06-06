---
description: Analyze branch changes and create a GitHub pull request
tools:
  - Bash(git status, git diff, git log, git push, git rev-parse, git branch, gh pr create, gh pr view, gh repo view)
  - AskUserQuestion
  - Read
---

現在のブランチの変更を分析し、GitHubのPull Requestを作成します。

## 手順

1. `git status` で未コミット変更がないか確認
2. ベースブランチを特定（デフォルトは `gh repo view --json defaultBranchRef` で取得）
3. `git log <base>..HEAD` および `git diff <base>...HEAD` で差分を分析
4. リモートに未pushのコミットがあれば `git push -u origin <branch>` を実行
5. `AskUserQuestionTool` で以下を順に確認
   - PR本文の言語（英語 / 日本語）
   - draft / ready のどちらで作成するか
6. 変更内容に基づいてPRタイトルと本文を生成
7. ユーザーに最終確認後、`gh pr create` で作成
8. 作成したPRのURLを表示

## PRタイトル

- 70文字以内
- Conventional Commits形式に倣う（`feat:`, `fix:` など）
- 1つ目のコミットメッセージをベースにしつつ、複数コミットを総括する内容にする

## PR本文テンプレート

```markdown
## Summary
- <変更の概要を1-3 bullet points>

## Why / Motivation
<なぜこの変更が必要かの背景>

## Test plan
- [ ] <テスト項目>
- [ ] <テスト項目>

## Screenshots
<UI変更がある場合のみ。なければセクションごと省略>
```

## 注意事項

- HEREDOC を使ってPR本文を渡す（フォーマット崩れ防止）
- `--draft` フラグは draft 選択時のみ付与
- 既にPRが存在する場合は新規作成せず、既存PRのURLを表示
- Screenshots セクションはUI/フロントエンド変更がある場合のみ含める
- `gh` コマンドが未認証の場合はユーザーに `gh auth login` を案内
