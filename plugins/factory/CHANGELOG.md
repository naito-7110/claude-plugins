# Changelog

## 0.2.1(2026-07-05)

初回 dogfooding で発覚した設計バグの修正(#103 / PR #104)。

- **ゲートのスコープ導入**: `.factory/` の存在 = factory 管理下。管理外のリポジトリでは hook が一切干渉しない(プラグインはユーザーレベルで有効化されるため、これが無いと無関係なリポジトリを fail-closed の人質にしていた)
- **鶏卵デッドロックの解消**: 管理下でバイナリ欠落時、拒否メッセージ自体が復旧手順(`!` 直接実行のブートストラップ 1 行)を運ぶ。init はバイナリ取得を `.factory/` scaffold より先に実行する(本質的な順序依存)

## 0.2.0(2026-07-05)

初回リリース(0.1.0 = 2026-07-04 の骨格)から、構想の全機能が揃ったリリース。

### スキル(6 → 12 本)

- **orchestrate**: 唯一の駆動ループ(attended / unattended)。制動は PR 滞留・エスカレーション滞留の 2 メトリクスのみ
- **night**: tick 用の無人起動口(mode gate 判定・sentinel 管理・L3 未登録なら無人運転拒否)
- **report**: unattended 期間のダイジェスト(事後レビュー一覧 + revert 導線、沈黙しない)
- **review**: 別コンテキストレビュア(tick 起動・commit status `factory-review`・merge:agent の最後の実行条件)
- **huddle**: ドメインエキスパート協議(独立影響評価 → 矛盾検出 → 収束、groom の前段)
- **uninstall**: やめるときの cleanup(tick 撤去 → .agents/ 削除 → 残るものの提示)
- **help**: 入口(read-only)— 使い方マップ + 現状診断(設置 / 運転 / ゲート / 作業状態)

### bin(tools/factory)

- `mode`(auto / manual の二値・gate)・`tick`(crontab マーカーブロックの冪等管理)
- `gate`(hook 判定の Go 移行 — factory-gate.sh は exec ラッパーに縮小、jq 依存消滅)
- `branch cleanup`(squash 運用対応: PR 状態を正としたマージ済み agent ブランチ・worktree の掃除)
- `release`(安全なタグ打ち: 常に remote ref の SHA・既存タグ拒否・--dry-run)+ **リリースゲート**(AI は release 実行・タグ push 不可 — デプロイの引き金は人間だけ)
- `tick run` は mode を内部確認してから claude を起動(manual 中のセッション消費ゼロ)
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
