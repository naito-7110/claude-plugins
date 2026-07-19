# ローカル ADR(リポジトリ横断)

このリポジトリ固有のアーキテクチャ決定を置く。ポータブルな原則は atelier プラグイン同梱のプリセット ADR が正準(一覧はプラグインの `adr/README.md`)。

## 規約

- 採番: `NNNN-<slug>.md`(0001 から連番)
- frontmatter で `Overrides: <preset-slug>` を宣言すると、該当プリセットよりローカルが優先される
- 新設・改訂・廃止は /atelier:adr(人間承認必須)を経由する

## 置き場の使い分け

- リポジトリ横断の決定 → ここ(`docs/adr/`)
- atelier 製品固有の決定 → `plugins/atelier/docs/adr/`(例: 0001 冪等 setup/teardown PR)
