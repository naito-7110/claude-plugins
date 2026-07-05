---
name: domains
description: ドメイン分割(対話専用)。domain-partitioning プリセットの基準に沿ってドメインの定義・分割・統合・パス移管を人間と確定し、ownership.yml と docs/domains の雛形を生成する。init は空マップまでしか作らないため、ドメインを切るのはこのスキルだけ
tools:
  - Bash(gh issue view, gh issue comment, gh pr create, git log, git shortlog, git checkout, git add, git commit, git push, git ls-files, factory)
  - AskUserQuestion
  - Task
  - Read
  - Write
  - Glob
  - Grep
---

**対話専用。** ドメインの境界は所有・文書・huddle の品質を決める意思決定であり、人間の確定が必須。判断基準の正準は `${CLAUDE_PLUGIN_ROOT}/adr/domain-partitioning.md`(+ documentation)。

## 手順

### 1. 現状把握

- `.factory/ownership.yml` と `docs/domains/` の現状(未分割なら `domains: {}`)
- **共変更の分析**: `git log` で「同じ PR / コミットで一緒に変わるファイル群」を観察する(変更理由の単位の実測。広い解析は Explore サブエージェントへ委譲可)
- 依存の向き・公開面(API・イベント)の当たりをつける

### 2. 候補の提示

- 共変更の実測と業務能力から**ドメイン候補**を提示する(名前・責務・所有パス案・他候補との境界)
- domain-partitioning の基準で自己検査してから出す: 技術層で切っていないか / 1 パス 1 ドメインか / 粒度シグナルに照らして妥当か
- 全体を一度に切らない。境界が見えているものだけを候補にし、残りは未所有のままでよい(漸進導入)

### 3. 人間と確定

`AskUserQuestion` で 1 ドメインずつ確定する(名前・責務・所有パス・公開契約の初期セット)。選択肢には trade-off を添える。ドメイン名は**小文字スネークケース**(機械検証の制約)。

### 4. 生成

確定したドメインごとに:

- `.factory/ownership.yml` に `domains.<name>.paths` を追記(既存の宣言は壊さない)
- `docs/domains/<name>/README.md`(責務・ユビキタス言語・不変条件)・`contracts.md`(公開契約の初期記述)・`decisions/` を生成
- 分割の理由と経緯を `decisions/` に**起点の issue / PR 参照つき**で記録する(documentation の出所原則)

### 5. 整合確認

`factory docs verify` を実行し、green(構造・所有マップ・重複所有の警告なし)を確認してから PR を作る。verify が通らない状態で PR を出さない。

### 6. 再編(分割・統合・移管)

既存ドメインの再編も同じ流れで行う。粒度シグナル(domain-partitioning)を根拠として提示し、**所有マップの変更と decisions への記録を同じ PR で**行う。旧ドメインの文書は削除せず、統合先への案内を残して decisions で経緯を辿れるようにする。

### 7. PR

通常の PR フロー(人間レビュー・マージ)。本文にドメイン候補の根拠(共変更の実測・基準への適合)を含める。

## 禁止事項

- 人間の確定なしの ownership.yml 変更
- `factory docs verify` が通らない状態での PR 作成
- コードの移動(本スキルの成果物は宣言と文書まで。コードのドメイン別再配置は通常の issue として起票する)
