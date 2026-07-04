# supply-chain-gha: GitHub Actions での実現手法

[supply-chain-security](../adr/supply-chain-security.md) プリセットの CI 防護原則を GitHub Actions で実現する具体手順(非規範)。

## SHA 固定

- `uses:` は commit SHA + タグコメントで書く:

```yaml
- uses: actions/checkout@9c091bb21b7c1c1d1991bb908d89e4e9dddfe3e0 # v7
```

- SHA の解決はタグの指すコミットを取得する:

```bash
gh api repos/<owner>/<repo>/commits/<tag> --jq .sha
```

- checkout は v7 以降を使う(pull_request_target の安全な既定が入っている)

## dependabot の設定

`.github/dependabot.yml` に github-actions ecosystem + cooldown を設定する(SHA とタグコメントの両方を自動追随):

```yaml
version: 2
updates:
  - package-ecosystem: "github-actions"
    directory: "/"
    schedule:
      interval: "weekly"
    cooldown:
      default-days: 7
```

## permissions の最小化

- workflow のトップレベルで `permissions: {}` を明示し、必要なジョブにだけ最小の権限を付与する:

```yaml
permissions: {}

jobs:
  build:
    permissions:
      contents: read
```

## スクリプトインジェクション対策

- 外部由来コンテキストは `run:` で直接展開せず、`env:` 経由でシェル変数として参照する:

```yaml
- env:
    TITLE: ${{ github.event.issue.title }}
  run: echo "$TITLE"
```

## その他のチェックリスト

- `pull_request_target` 等の特権トリガーで fork 由来コードを checkout しない
- secrets・リリースを扱う workflow で `actions/cache` を使わない
- 再利用ワークフローに `secrets: inherit` を使わず、必要な secret のみ明示的に渡す
- パブリックリポジトリで self-hosted runner を使わない
- gitleaks 等の secrets スキャンを pre-commit(lefthook 等)と CI の両方に入れる
