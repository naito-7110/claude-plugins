---
description: Analyze staged changes and create a commit with conventional commit message
---

ステージされた変更を分析し、Conventional Commits形式でコミットを作成します。

## 手順

1. `git status` でステージされた変更を確認
2. `git diff --cached` でステージされた変更の内容を分析
3. 変更内容に基づいてConventional Commit形式のメッセージを生成
4. ユーザーに確認後、コミットを実行

## Conventional Commits形式

```
<type>(<scope>): <subject>

<body>
```

### Type
- `feat`: 新機能
- `fix`: バグ修正
- `docs`: ドキュメントのみの変更
- `style`: コードの意味に影響しない変更（空白、フォーマット等）
- `refactor`: バグ修正でも機能追加でもないコード変更
- `perf`: パフォーマンス改善
- `test`: テストの追加・修正
- `chore`: ビルドプロセスや補助ツールの変更
- `hotfix`: 緊急のバグ修正

### 注意事項
- subjectは50文字以内
- bodyは72文字で折り返す
- コミットメッセージは英語で記述する
