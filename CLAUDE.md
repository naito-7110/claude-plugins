# cc-plugins

**Important**
- Thinking with English, Respond with Japanese

<!-- atelier:constitution begin -->
## 憲法(atelier)

- **プリセット憲法**: atelier プラグイン同梱の `adr/`(索引・選択読みマッピングは同ディレクトリの README.md)。このリポジトリではプリセット原本 = 製品でもある(編集は通常の PR、参照は常時読み + 変更領域の選択読み)
- **ローカル ADR**: `docs/adr/` — このリポジトリ固有の決定。`Overrides: <slug>` でプリセットに勝つ
- 実装・レビューの前に、変更領域に対応するプリセットを選択読みすること
<!-- atelier:constitution end -->

<!-- atelier:stack-facts begin -->
## スタック事実(atelier が導出)

| 対象 | コマンド | 根拠 |
| --- | --- | --- |
| tools/atelier(Go) | ビルド: `go build ./...` / テスト: `go test ./...` / lint: `golangci-lint run`(いずれも tools/atelier で) | .github/workflows/atelier-cli.yml |
| plugins/(markdown) | スタック固有語の混入検査: `grep -riE "vue\|dotnet\|pnpm\|valibot\|msw" <対象>` | 開発規律(PR テスト節の実績) |
| リリース | `atelier release atelier/vX.Y.Z`(人間のみ。**先に plugin.json + CHANGELOG を上げる**) | tools/atelier/README.md |
<!-- atelier:stack-facts end -->
