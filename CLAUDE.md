# cc-plugins

**Important**
- Thinking with English, Respond with Japanese

<!-- atelier:constitution:start (managed by /atelier:init — edit via re-run) -->
## Atelier: 憲法

- 開発判断の根拠は二層: atelier プラグイン同梱のプリセット ADR(ポータブル原則。一覧はプラグインの `adr/README.md`)+ このリポジトリの `docs/adr/`(プロジェクト固有の決定。技術選定はこちら)。atelier 製品固有の決定は `plugins/atelier/docs/adr/`
- スキルはプリセットを `/Users/n7110/.claude/plugins/cache/7110-claude-plugins/atelier/1.1.1/adr/` から読む
- ローカル ADR が frontmatter で `Overrides: <slug>` を宣言した場合、該当プリセットよりローカルが優先
- 改訂は /atelier:adr(人間承認必須)。検証コマンド等の事実は下のスタック事実の節へ
<!-- atelier:constitution:end -->

<!-- atelier:stack-facts:start (managed by /atelier:init — edit via re-run) -->
## Atelier: スタック事実

リポジトリから導出した事実。憲法(ADR)ではないため、スタック変更時は /atelier:init の再実行で更新する。

| 用途 | コマンド | 根拠 |
| --- | --- | --- |
| ビルド | `CGO_ENABLED=0 go build ./...`(`tools/atelier` で実行) | `.github/workflows/atelier-cli.yml` build job |
| テスト | `go vet ./... && go test ./...`(`tools/atelier` で実行) | 同 test job |
| lint | `golangci-lint run`(v2.10.1、`tools/atelier` で実行) | 同 lint job |
| プラグイン(`plugins/*`) | ビルド・テストなし(markdown スキル) | CI 対象外 |
<!-- atelier:stack-facts:end -->
