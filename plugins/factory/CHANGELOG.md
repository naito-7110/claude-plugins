# Changelog

## 0.2.0(2026-07-05)

初回リリース(0.1.0 = 2026-07-04 の骨格)から、構想の全機能が揃ったリリース。

### スキル(6 → 12 本)

- **orchestrate**: 唯一の駆動ループ(attended / unattended)。制動は PR 滞留・エスカレーション滞留の 2 メトリクスのみ
- **night**: tick 用の無人起動口(mode gate 判定・sentinel 管理・L3 未登録なら無人運転拒否)
- **report**: unattended 期間のダイジェスト(事後レビュー一覧 + revert 導線、沈黙しない)
- **review**: 別コンテキストレビュア(tick 起動・commit status `factory-review`・merge:agent の最後の実行条件)
- **huddle**: ドメインエキスパート協議(独立影響評価 → 矛盾検出 → 収束、groom の前段)
- **uninstall**: やめるときの cleanup(tick 撤去 → .agents/ 削除 → 残るものの提示)

### bin(tools/factory)

- `mode`(auto / manual の二値・gate)・`tick`(crontab マーカーブロックの冪等管理)
- `gate`(hook 判定の Go 移行 — factory-gate.sh は exec ラッパーに縮小、jq 依存消滅)
- `branch cleanup`(squash 運用対応: PR 状態を正としたマージ済み agent ブランチ・worktree の掃除)
- `flags check` / `docs verify` の堅牢化(壊れ方調査に基づく 5 修正)

### 設計決定

- **レーン統一**: 「夜間」概念を廃止し attended / unattended の 2 モードへ。1 晩 1 issue 制限は撤廃
- **課金方針の明文化**: 開発ループはサブスク内完結、API 課金はオプトイン
- hooks は公式のプラグイン機構(hooks/hooks.json)で自動登録
- テンプレート同梱: issue / PR / factory-issue-check.yml(L3)/ dependabot.yml(SHA 風化対策)

## 0.1.0(2026-07-04)

- スキル 6 本(init / adr / triage / groom / work / domains)・プリセット ADR 24 本・レシピ 6 本
- bin: board copy/verify・issue/pr verify・docs verify
- goreleaser による 6 プラットフォーム配布(factory/vX.Y.Z タグ)
