# architecture-strategy: アーキテクチャ品質戦略の原則

## 適用除外

単一 module 内の局所的な責務分割で、外部契約・データ所有・deploy・運用へ影響しない変更には該当なし(code-design / domain-modeling / dependency-boundaries を使う)。本 ADR は**複数 module・データ所有・deploy・team・利用者 journey を跨ぐ system-wide な構造判断**を扱う。変更単位の品質 portfolio は spec-alignment、コア度に応じた設計投資は domain-partitioning が持つ。

## コンテキスト

アーキテクチャは流行の構成やレイヤ数を選ぶことではなく、プロダクトが継続的に価値を生むために、重要な品質特性を促進する構造判断を system 全体で整合させることである。エージェントは「マイクロサービス化」「Clean Architecture 化」を目的化しがちで、各所の局所最適が利用者 journey や変更リードタイムを悪化させる。全体最適とは全品質の最大化ではなく、優先する品質と許容する劣化を明示して局所改善が全体を悪化させないことである。

## 決定

### 名前付きアーキテクチャでなく価値と品質から始める

- 「マイクロサービス化」「Clean Architecture 化」「event-driven 化」を目的にしない。名前付きパターンは品質シナリオを満たす候補手段として比較する
- 判断は「誰へどの価値を、どの品質特性で成立させ、現構造がどのシナリオを妨げるか」から始める

### アーキテクチャ判断の単位を明示する

- 設計判断は、責務境界・依存方向・データ所有(source of truth / writer / reader / 整合性境界)・公開契約・実行方式(同期 / 非同期 / transaction / retry / idempotency)・配備 / 障害境界・変更境界のどれを変えるかを明示する。パッケージ名や diagram だけでは判断にならない

### system 全体の品質 portfolio を持つ

- 一判断期間で primary を原則 3 つ以内に絞り、secondary・constraint・意図的に最適化しない品質を分ける。SLO / threshold は品質名へ混ぜず、対応するシナリオの期待応答または constraint へ置く
- 各品質を刺激・対象・環境・期待応答・測定を持つ代表シナリオにする(品質語彙の粒度は spec-alignment の品質 portfolio と同じ扱い)

### 局所改善と全体効果を分ける

- 各案を local / system / journey / organization / future change で別々に評価する。1 class がきれいでも、API 呼び出し・分散 transaction・複数 team 調整が増えるなら全体最適とは限らない

### trade-off を決定として残し、不可逆性で証拠の強さを変える

- 改善する品質だけでなく、悪化し得る品質・受容理由・監視指標・撤回条件を記録する
- 証拠の強さを不可逆性に応じて上げる: 可逆で局所 → prototype / 1 module で検証、外部契約変更 → consumer inventory と互換 window、データ所有変更 → migration / dual-read-write / recovery、service 分割・統合 → deploy / observability / 障害伝播、法令 / security / 課金 → 専門 review と明示承認

### target だけでなく transition を設計する

- current → 保護テスト / 観測 → 安定 interface → 新しい所有者 → 段階移行 → cutover → 旧経路削除。各 phase に独立した価値またはリスク低減と、二値判定可能な完了条件・abort 条件・recovery を持たせる(technical-debt の大規模リファクタリングと同型。暫定 adapter / dual-write の owner・削除条件も同じ規律)

### source of truth と authority を system 全体で一意にする

- 状態を変更できる component、不変条件を所有する model、authoritative な schema / event / service を一意にする。cache / read model / replica を真実の所有者と混同しない。複数 writer が必要なら競合解決と整合性契約を明示する

### 人間が価値と不可逆判断を持つ

- AI は現状 inventory・品質シナリオ候補・代替案・trade-off・依存影響・検証項目を出す。人間は顧客価値・優先品質・許容する劣化・移行リスク・不可逆判断を所有する。AI の説明の流暢さを判断根拠にしない
- 採用判断は根拠・反証・仮定・unknown・再評価 trigger とともに決定として記録する(documentation / ローカル ADR)。diagram レビューで終わらせず、代表変更の impact・contract / dependency test・failure injection・migration dry-run で構造を検証する

## トレードオフ

- **得るもの**: 局所最適が全体を悪化させる事故を防ぎ、アーキテクチャ判断が価値と品質シナリオに紐づく。移行が rollback 可能な phase になり、不可逆判断に証拠と承認が入る
- **諦めるもの**: 品質 portfolio・シナリオ・transition 設計の前工程コスト
- **緩和策**: 単一 module 内の局所判断には適用しない。primary は 3 つ以内に絞る。名前付きパターンは候補手段として扱い、最初から全体移行しない
