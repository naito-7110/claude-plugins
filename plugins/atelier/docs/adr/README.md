# atelier 製品 ADR

atelier **自身をどう作るか**の設計決定を記録する。消費側レポの憲法ではなく、atelier というプロダクトの憲法。

## `plugins/atelier/adr/`(preset corpus)との違い

| | 置き場 | 対象 | 配布 |
|---|---|---|---|
| **preset ADR** | `plugins/atelier/adr/` | 消費側プロジェクトへ配る汎用原則(code-design・supply-chain-security 等) | ✅ 設置先へ配られる出荷物 |
| **製品 ADR(ここ)** | `plugins/atelier/docs/adr/` | atelier 自身の実装・構造の決定 | ❌ 配られない。atelier リポ内で完結 |

境界の目安:
- 「配布物が host repo をどう扱うべきか」のような**他プロジェクトにも効く原則** → preset へ(汎用化のフライホイール)。
- 「atelier の init/teardown をどう実装するか」のような **atelier 固有の実装決定** → ここ。

汎用原則を preset に持ちつつ、それを atelier 自身がどう満たすかを製品 ADR に書く、の二層で対にする(例: preset `distributable-boundary` 決定4 ⇄ 製品 ADR [0001](./0001-idempotent-setup-teardown-prs.md))。

## 索引

| # | 決定 | ステータス |
|---|---|---|
| [0001](./0001-idempotent-setup-teardown-prs.md) | init/teardown を manifest 駆動の冪等 PR として実装する | 承認 |
