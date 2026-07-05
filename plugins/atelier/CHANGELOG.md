# Changelog

## 1.0.0(2026-07-05)

**初のメジャーリリース**。プラグインを factory から atelier へ改名し、無人自律運転を撤去して「人間(親方)が常駐する工房」として再定義した。破壊的変更を含む。

- **無人自律機構の撤去**(#122・**破壊的変更**): CLI から `tick` / `mode` サブコマンドが消滅。night / report スキルを削除。gate の無人 3 種(改憲ブロック・配車ゲート・merge:agent 付与ブロック)を撤去し、hook の matcher は `Bash` のみに縮小。残るゲートは main 直 push / push / マージ / リリースの 4 つ。orchestrate は人間常駐前提に単一化
- **factory → atelier 改名**(#123): ディレクトリ・Go module・バイナリ名(`atelier`)・hook(`atelier-gate.sh`)・commit status(`atelier-review`)・required check(`atelier-issue-check`)・スキル名前空間(`/atelier:*`)・タグ規約(`atelier/vX.Y.Z`)・管理マーカー(`.atelier/`)。**旧名で設置済みのリポジトリは README の「旧 factory からの移行」に従うこと**(`.factory` のリネーム + init 再実行。放置するとゲートが働かない)
- **リリースの引き金(release サブコマンド)を配布物から撤去**(#129・破壊的変更): AI とオーナーは gh 認証を共有しサーバー側で区別できないため、デプロイの便利コマンドをバイナリに含めない。タグ打ちは人間の生 git 手順(tools/atelier README)。gate は旧版バイナリ対策として起動検出を維持
- 経緯の記録は naito-7110/claude-plugins#4(crontab 運用の実測破綻 → プラグインの責務境界の再定義 → 無人工場構想は別プロダクトへ)

## 0.2.2(2026-07-05)

- **レビューコメントの自動巻き取り**(#107): orchestrate の回収が未解決レビュースレッドを検出し、work のレビュー対応モード(修正 → push → スレッド単位の返信)で再配車。最新コメントが自分のスレッドは対象外(ループ防止)
- **リアルタイム化 — tick の作業検知プリチェック**(#111): `tick run` が claude 起動前に gh API(4 クエリ以内)で仕事の有無を検知。仕事ゼロなら起動せず 1 行ログで終了 — 1 分周期の tick でもセッション消費は実作業時のみ。`.agents/tick-state` は起動時のみ更新(取りこぼし防止)
- init に後入れ認識 = 文書監査(#96)・セルフホスト運用の開始(#108、安全レールは対象リポジトリの docs/adr/0001)


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
