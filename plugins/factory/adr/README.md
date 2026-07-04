# プリセット ADR コーパス

factory の共有憲法(スタック非依存のポータブル原則)。ここに置かれた ADR は factory を設置した**すべてのリポジトリ**に適用される。

## 参照モデル(コピーしない)

- 対象リポジトリへはコピーしない。スキル(work / groom / adr / triage)は `${CLAUDE_PLUGIN_ROOT}/adr/` を直接読む
- プラグインの更新 = 全プロジェクトへの改訂の配布。エスカレーションから生まれた汎用な原則は、このディレクトリへの PR として中央に蓄積される
- プロジェクト固有の決定(技術選定を含む)はここではなく、各リポジトリの `docs/adr/` に置く(三層構造はプラグイン README を参照)

## Overrides 規約

ローカル ADR(`docs/adr/`)が frontmatter で `Overrides: <slug>` を宣言した場合、そのプロジェクトでは該当プリセットよりローカル ADR が優先される。宣言・改訂は /factory:adr(人間承認必須)を通す。

## 形式

各プリセットは次を持つ:

1. **適用除外条項**(冒頭): 該当しないプロジェクト(API を持たない等)が読み飛ばせる条件
2. **コンテキスト**: なぜこの原則が要るか
3. **決定**: 原則と判断基準(スタック固有の固有名詞は書かない)
4. **トレードオフ**: 得るもの・諦めるもの

## 収録一覧

追加は **1 プリセット 1 PR**(issue #11)。

| slug | 状態 | 領域 |
| --- | --- | --- |
| [tdd-doctrine](./tdd-doctrine.md) | 収録済み | テスト方針(状態検証主義) |
| supply-chain-security | 予定 | 開発環境のセキュリティ |
| product-security | 予定 | 開発物のセキュリティ |
| logging-observability | 予定 | ログ・可観測性 |
| feature-flags | 予定 | フィーチャーフラグ運用 |
| pr-granularity | 予定 | PR 粒度 |
| error-handling | 予定 | エラーハンドリング |
| authorization | 予定 | 認可 |
| concurrency-process | 予定 | 排他制御の選択プロセス |
| api-resource-design | 予定 | API リソース設計 |
| i18n-copy | 予定 | 文言・i18n |
| performance | 予定 | パフォーマンス |
| spec-alignment | 予定 | 仕様すり合わせ |
| merge-policy | 予定 | マージポリシー |
