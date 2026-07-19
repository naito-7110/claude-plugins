# atelier 文書の地図

このリポジトリの開発判断・文書がどこにあるかの案内(machine-readable な運用ファイルは `.atelier/`、人間の一次文書は `docs/` と各製品ディレクトリ)。

## 憲法(判断の根拠)

| 層 | 場所 | 内容 |
| --- | --- | --- |
| プリセット ADR | atelier プラグイン同梱 `adr/`(一覧は同 `adr/README.md`) | ポータブルな開発原則(30 本)。リポジトリへはコピーしない |
| ローカル ADR(リポジトリ横断) | `docs/adr/` | このリポジトリ固有の決定。`Overrides: <slug>` でプリセットに優先 |
| 製品 ADR(atelier) | `plugins/atelier/docs/adr/` | atelier 製品固有の決定(0001: 冪等 setup/teardown PR) |
| スタック事実 | `CLAUDE.md` のマーカー節 | ビルド・テスト・lint コマンド(事実。ADR ではない) |

## 既存文書の登載(2026-07-19 init 軽量監査)

| 文書 | 役割 |
| --- | --- |
| `README.md` | リポジトリ全体の案内(marketplace・プラグイン一覧) |
| `plugins/atelier/README.md` | atelier プラグインの全体像・思想 |
| `plugins/atelier/CHANGELOG.md` | プラグインの変更履歴 |
| `tools/atelier/README.md` | atelier CLI(Go)の案内 |

## 運用ファイル

- `.atelier/ownership.yml`: パス → ドメインの所有マップ(未分割: `domains: {}`)
- `.agents/`(gitignore 領域): バイナリ・台帳・ジャーナル(コミットしない)
