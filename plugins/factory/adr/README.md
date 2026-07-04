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

| slug | 領域 |
| --- | --- |
| [tdd-doctrine](./tdd-doctrine.md) | テスト方針(状態検証主義) |
| [supply-chain-security](./supply-chain-security.md) | 開発環境のセキュリティ |
| [product-security](./product-security.md) | 開発物のセキュリティ |
| [logging-observability](./logging-observability.md) | ログ・可観測性 |
| [feature-flags](./feature-flags.md) | フィーチャーフラグ運用 |
| [pr-granularity](./pr-granularity.md) | PR 粒度 |
| [error-handling](./error-handling.md) | エラーハンドリング |
| [authorization](./authorization.md) | 認可 |
| [concurrency-process](./concurrency-process.md) | 排他制御とトランザクション境界 |
| [rest-api-design](./rest-api-design.md) | REST API リソース設計 |
| [i18n-copy](./i18n-copy.md) | 文言・i18n |
| [performance](./performance.md) | パフォーマンス・SLO |
| [spec-alignment](./spec-alignment.md) | 仕様すり合わせ |
| [merge-policy](./merge-policy.md) | マージポリシー |
| [infrastructure](./infrastructure.md) | インフラ |
| [resilience](./resilience.md) | 外部 I/O の回復性 |
| [git-workflow](./git-workflow.md) | git ワークフロー |
