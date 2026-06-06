---
name: plan
description: Turn a request, document, or existing issue into a GitHub issue and add it to the linked Project
tools:
  - Bash(gh issue create, gh issue view, gh repo view, gh project list, gh project item-add, gh project view, gh api)
  - AskUserQuestion
  - Read
---

要望・メモ・既存issueなどを入力として、GitHub Issueを1つ作成し、リポと紐付いたProjectに追加します。

タスク分割（1つの大きな要望を複数issueに分ける）はこのskillの責務外です。別skillで扱います。

## 入力（いずれか）

- チャットで渡された話し言葉の要望
- 既存のissue URL または番号（再構成・補強したい場合）
- メモ・設計ドキュメントのファイルパス（Readで読む）
- 引数なし → `AskUserQuestion` で何をissue化したいか引き出す

## 手順

1. 入力を分類して内容を取得
   - URL/番号 → `gh issue view <num> --json title,body,labels`
   - ファイルパス → `Read` で読み込み
   - チャット/引数なし → `AskUserQuestion` で要望をヒアリング
2. リポを特定: `gh repo view --json nameWithOwner,owner`
3. リポに紐付いたProjectを検出
   - `gh project list --owner <owner> --format json`
   - 複数あれば `AskUserQuestion` で選択。0件ならProject追加をスキップして警告
4. 内容を元にIssueタイトルと本文（下記テンプレート）を生成
5. タイトル/本文をユーザーに提示し、`AskUserQuestion` でOK/修正を確認
6. `gh issue create --title ... --body ...`（HEREDOC使用）で作成
7. 作成したissueをProjectに追加
   - `gh project item-add <project-number> --owner <owner> --url <issue-url>`
   - Statusは Project のデフォルト値（通常 Todo）に任せる
8. 作成したissueのURLを表示

## Issue本文テンプレート

```markdown
## Summary / 概要
<何をやるかの要約>

## Motivation / 背景
<なぜやるか>

## Acceptance Criteria
- [ ] <完了条件>
- [ ] <完了条件>

## Notes / References
- <関連リンク>

## 懸念事項
- <既知の懸念・要検討事項>
```

セクションが空になる場合は、そのセクションごと省略する。

## 注意事項

- タイトルは70文字以内、Conventional Commits形式の prefix（`feat:`, `fix:` など）を付けてもよい
- 本文はHEREDOCでgh issue createに渡す（フォーマット崩れ防止）
- `gh auth status` で未認証の場合は `gh auth login` を案内
- Projectが0件の場合はissueだけ作って「Projectに紐付けたい場合は手動で」と案内
- 既存issueを入力にした場合は、上書きするのか新規作成するのかを最初に確認
