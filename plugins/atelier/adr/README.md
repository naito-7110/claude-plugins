# プリセット ADR コーパス

atelier の共有憲法(スタック非依存のポータブル原則)。ここに置かれた ADR は atelier を設置した**すべてのリポジトリ**に適用される。

## 参照モデル(コピーしない)

- 対象リポジトリへはコピーしない。スキル(work / groom / adr / triage)は `${CLAUDE_PLUGIN_ROOT}/adr/` を直接読む
- プラグインの更新 = 全プロジェクトへの改訂の配布。エスカレーションから生まれた汎用な原則は、このディレクトリへの PR として中央に蓄積される
- プロジェクト固有の決定(技術選定を含む)はここではなく、各リポジトリの `docs/adr/` に置く(三層構造はプラグイン README を参照)

## Overrides 規約

ローカル ADR(`docs/adr/`)が frontmatter で `Overrides: <slug>` を宣言した場合、そのプロジェクトでは該当プリセットよりローカル ADR が優先される。宣言・改訂は /atelier:adr(人間承認必須)を通す。

## 形式

各プリセットは次を持つ:

1. **適用除外条項**(冒頭): 該当しないプロジェクト(API を持たない等)が読み飛ばせる条件
2. **コンテキスト**: なぜこの原則が要るか
3. **決定**: 原則と判断基準(スタック固有の固有名詞は書かない)
4. **トレードオフ**: 得るもの・諦めるもの

加えて: **本文に外部リンクを貼らない**(リンク切れで規範が空洞化するため)。参照した資料は内容を本文へ蒸留する。ローカル資料・私有物への参照も書かない。

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
| [schema-migration](./schema-migration.md) | スキーマ変更とデータ移行 |
| [dependency-licensing](./dependency-licensing.md) | 依存の選定基準とライセンス |
| [data-lifecycle](./data-lifecycle.md) | データライフサイクル |
| [ai-usage](./ai-usage.md) | AI 利用の境界と責務 |
| [db-design](./db-design.md) | DB 設計 |
| [documentation](./documentation.md) | ドキュメント管理 |
| [domain-partitioning](./domain-partitioning.md) | ドメイン分割 |
| [distributable-boundary](./distributable-boundary.md) | 配布物の責任境界 |
| [dependency-boundaries](./dependency-boundaries.md) | 依存境界(モジュールの抽象化基準) |
| [technical-debt](./technical-debt.md) | 技術的負債(あるべき姿との差・優先順位) |
| [domain-modeling](./domain-modeling.md) | ドメインモデルの形(目的起点・暗黙概念の明示) |
| [code-design](./code-design.md) | コード設計(目的駆動の命名・責務診断) |
| [design-by-contract](./design-by-contract.md) | 契約による設計(要件→事前/事後/不変条件) |
| [architecture-strategy](./architecture-strategy.md) | アーキテクチャ品質戦略(価値起点・全体最適・移行) |

## 選択読みマッピング(スキル向け)

コーパス全読みはコンテキストを圧迫するため、スキル(特に work)は次の規約で**選択読み**してよい。

**常時読み**(タスクの種類によらず): spec-alignment・pr-granularity・merge-policy・git-workflow・documentation(同 PR 更新原則)、および product-security の「エージェントの裁量制限」節(敏感領域判定)。

**変更領域に応じて追加で読む**:

| 変更領域 | 追加で読むプリセット |
| --- | --- |
| テストを書く・変える | tdd-doctrine |
| 依存・CI・workflow | supply-chain-security・dependency-licensing |
| HTTP API | rest-api-design・error-handling・authorization |
| DB・スキーマ・排他 | db-design・schema-migration・concurrency-process・tdd-doctrine |
| 外部 I/O・非同期・多段処理 | resilience |
| UI・ユーザー向け文言 | i18n-copy・product-security |
| インフラ・デプロイ | infrastructure |
| 大量データ・高頻度処理 | performance |
| ログ・計測・アラート | logging-observability |
| フィーチャーフラグ | feature-flags |
| AI 機能・エージェント設定 | ai-usage |
| 個人データの追加・変更 | data-lifecycle・product-security |
| ドメインの分割・所有マップ変更 | domain-partitioning・documentation |
| 配布物(プラグイン・CLI・パッケージ)の機能追加・設計 | distributable-boundary |
| モジュール新設・層の追加・外部 I/O の接続面設計 | dependency-boundaries |
| リファクタリング・負債解消の計画と実行(大規模移行・段階置換を含む) | technical-debt |
| 業務ロジック(判断・状態遷移・不変条件)を持つモデルの新設・変更・分割、暗黙の業務概念の明示 | domain-modeling |
| 命名・シンボル設計・責務の切り分け・抽象化やインターフェースの導入 | code-design |
| 公開操作・状態変更の契約設計・要件の検証可能化 | design-by-contract |
| 複数 module・データ所有・deploy・team・利用者 journey を跨ぐ構造判断 | architecture-strategy |

複数領域に跨る場合は和集合。迷ったら読む側に倒す(fail-closed)。
