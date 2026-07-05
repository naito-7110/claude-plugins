# .atelier — 文書の地図(machine-read)

このリポジトリは atelier プラグインの開発リポジトリであり、**atelier 自身で管理される**(セルフホスト — docs/adr/0001 参照)。

## 層と場所

| 層 | 場所 | 役割 |
| --- | --- | --- |
| プリセット憲法(製品) | `plugins/atelier/adr/` | 全リポジトリ向けの原則。このリポジトリでは編集対象の製品でもある |
| ローカル ADR | `docs/adr/` | このリポジトリ固有の決定(セルフホストの安全レール等) |
| スタック事実 | `CLAUDE.md` のマーカー節 | ビルド / テスト / lint / リリースのコマンド |
| 所有マップ | `.atelier/ownership.yml` | ドメイン → パス(現状 domains: {} — 分割は /atelier:domains で漸進) |

## 文書一覧(2026-07-05 文書監査で登載)

| 文書 | 内容 | 所有 |
| --- | --- | --- |
| `README.md` | プラグイン一覧(実態と一致を確認済み) | リポジトリ |
| `CLAUDE.md` | セッション規約 + 憲法案内 + スタック事実 | リポジトリ |
| `plugins/atelier/README.md` | 工場の全体像・スキル表・やめるとき | atelier プラグイン |
| `plugins/atelier/CHANGELOG.md` | プラグイン + bin の変更履歴(リリース前に更新必須) | atelier プラグイン |
| `plugins/atelier/adr/` + `recipes/` | プリセット憲法 24 本 + レシピ(外部リンク禁止) | atelier プラグイン |
| `plugins/atelier/hooks/README.md` | ゲート一覧・登録・限界 | atelier プラグイン |
| `tools/atelier/README.md` | bin の開発・リリース手順(version 前出しの規律) | tools/atelier |
| `docs/adr/` | ローカル ADR | リポジトリ |

文書を増やす・大きく変えるときは、同じ PR でこの地図と ownership を更新する(documentation プリセット: 同 PR 更新)。
