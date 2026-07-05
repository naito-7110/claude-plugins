# cc-plugins

**Important**
- Thinking with English, Respond with Japanese

<!-- factory:constitution begin -->
## 憲法(factory)

- **プリセット憲法**: factory プラグイン同梱の `adr/`(索引・選択読みマッピングは同ディレクトリの README.md)。このリポジトリではプリセット原本 = 製品でもある(編集は通常の PR、参照は常時読み + 変更領域の選択読み)
- **ローカル ADR**: `docs/adr/` — このリポジトリ固有の決定。`Overrides: <slug>` でプリセットに勝つ
- 実装・レビューの前に、変更領域に対応するプリセットを選択読みすること
<!-- factory:constitution end -->

<!-- factory:stack-facts begin -->
## スタック事実(factory が導出)

| 対象 | コマンド | 根拠 |
| --- | --- | --- |
| tools/factory(Go) | ビルド: `go build ./...` / テスト: `go test ./...` / lint: `golangci-lint run`(いずれも tools/factory で) | .github/workflows/factory-cli.yml |
| plugins/(markdown) | スタック固有語の混入検査: `grep -riE "vue\|dotnet\|pnpm\|valibot\|msw" <対象>` | 開発規律(PR テスト節の実績) |
| リリース | `factory release factory/vX.Y.Z`(人間のみ。**先に plugin.json + CHANGELOG を上げる**) | tools/factory/README.md |
<!-- factory:stack-facts end -->
