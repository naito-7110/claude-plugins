---
name: work
description: 中核スキル。issue 番号を受け取り、影響調査 → worktree → TDD 実装 → 検証 → 文書同期 → セルフレビュー → PR 作成(merge:agent なら条件付きでマージまで)を一気通貫で行う。「issue #N をやって」と頼まれたとき、または orchestrate からの配車で使う。エスカレーション条件の正準リストは本スキル末尾にあり、他スキルはそこを参照する。args - issue 番号
tools:
  - Bash(git, gh, atelier)
  - Task
  - AskUserQuestion
  - Read
  - Write
  - Edit
  - Glob
  - Grep
---

対象: `$ARGUMENTS` の issue 番号。迷ったら推測で進まず、停止してエスカレーションする(fail-closed)。記録はすべて issue コメント(外部サービスを使わない)。

## 手順

### 0. 前提のセルフチェック(fail-closed)

`atelier issue verify --number <n>` があれば実行し、無ければ手動で確認する:

- `agent-ok` あり・`needs-human` なし・`agent-wip` なし(他の作業と衝突しない)
- `依存: #N` 行の依存 issue がすべてクローズ済み
- `merge:agent` が付いている場合は鮮度(付与後に本文が編集されていないか)

満たさなければ**着手せず**、理由を issue コメントに残して終了する(hook が機械強制するが、スキル側でも守る)。

### 1. issue を読む

`gh issue view <n> --comments` で本文と議論を読む。**「確定済みの設計」節を仕様として扱う**。受け入れ条件が曖昧・検証不能・矛盾している場合は実装に入らずエスカレーションする(末尾参照)。

### 2. 憲法の選択読み

`${CLAUDE_PLUGIN_ROOT}/adr/README.md` の選択読みマッピングに従う(常時読みセット + diff 予定領域の分)。ローカル `docs/adr/` と、`.atelier/ownership.yml` があれば管轄ドメインの `docs/domains/<domain>/`(README・contracts)も読む。

### 3. 影響調査 → ジャーナル

- コード面は **/atelier:analyze を `impact` 目的で呼ぶ**(全参照・全 writer・既存テストまでの影響マップ)。仕様面(公開契約・関連決定)は自分で確認する
- 影響マップと調査結果は **`.agents/journal/issue-<n>.md` に逐次追記**する(影響範囲・決定・次アクション)。ジャーナルはコンテキスト圧縮・中断からの唯一の再開点
- **最小 PR の切り方を確認する**(pr-granularity)。groom の分割案があればそれに従い、調査の結果分割が必要と分かったら分割案を issue コメントで提案してエスカレーションする

### 4. worktree 作成と着手宣言

```bash
git fetch origin
git worktree add .worktrees/issue-<n> -b agent/issue-<n>-<slug> origin/main
gh issue edit <n> --add-label agent-wip
```

Projects ボードがあれば Status → In Progress。以降の作業はすべて worktree 内で行う(git-workflow: main へ直接触らない・追従は rebase)。

### 5. TDD 実装

tdd-doctrine に従う: **受け入れ条件を固定する失敗するテストから書く**。状態検証(interaction 検証禁止)、層別の既定、異常系は stub で失敗注入。未完成の機能は feature flag の背後に置き、main を常にリリース可能に保つ。

### 6. 検証(コマンドはスタック事実から導出)

- **検証コマンドは本スキルに書かれていない**。CLAUDE.md の「Atelier: スタック事実」節から、diff が触れた領域に該当するもの(ビルド・テスト・lint)だけを実行する。触れていない領域の検証は実行も記載もしない
- スタック事実の節が無い・コマンドが実態と合わない場合は、**「スタック事実の更新提案」としてエスカレーション**する(/atelier:init の再実行を提案)
- 失敗したら自己修正する。ただし**同一の失敗クラスにつき 2 回まで**。2 回直しても red ならエスカレーションする

### 7. 文書の同期

documentation プリセットに従い、この変更で古くなる文書(ドメイン知識・公開契約・地図・ローカル ADR の参照)を**同じ PR で更新**する。仕様すり合わせで特定済みのもの(アジェンダ 9)に加え、実装中に気づいた乖離も含める。同 PR で直せない乖離は即時 issue 化する。

### 8. セルフレビュー(観点別・並列)

- **レビュー観点 team を並列 dispatch する**(Task tool)。team は plugin 同梱の `atelier:review:*` agent 群で、**各 agent が自分の観点の preset だけを read-only で読み、新鮮な目で見る**(定義は `agents/review/`)。各 agent には **diff と issue の受け入れ条件だけ**を渡し、実装の経緯・実装セッションのコンテキストは共有しない:
  - **security / tests / granularity / docsync** は常時
  - **contract**(所有マップがあり公開契約に触れる場合)・**design**(diff がドメイン・契約・公開面に触れる場合)は条件付き — 選択読みと同条件で過剰起動を避ける
- 小さな diff では観点を束ねてよい(1〜2 agent に集約)。project 側が `.claude/agents/` で同名(`security` 等)を定義していればそれが優先される(リポジトリ固有のレビュー観点への上書き)
- 指摘(critical / major)には対応し、内容をジャーナルへ記録する
- **注意**: これらは実装セッションが dispatch するセルフレビューであり、merge:agent の実行条件「実装コンテキストを共有しない別コンテキストのレビュア」は**満たさない**(それは review スキル / 外部の仕組みが担う)。agent 自体は実装経緯を知らないため思い込み検出には有効だが、起動と材料選択を実装側が握る以上、独立レビューの代替にはしない

### 9. コミット・push・PR 作成

- conventional commit(type + scope、英語)。squash マージ前提の積み方(git-workflow)
- push(hook の push ゲートを通る)。PR 本文は**日本語**で次の構成をすべて埋める: `# 概要` / `## 関連 Issue`(Closes #n)/ `## 変更内容` / `## 影響範囲分析の要約`(判断が要った点と根拠 — ジャーナルから)/ `## テスト`(実行したコマンドと結果)/ `## Feature Flag` / `## セルフレビューチェックリスト` / `## レビュー観点`(迷った点・見てほしい箇所)

```bash
git push -u origin agent/issue-<n>-<slug>
gh pr create --title "..." --body-file <filled.md>
gh issue edit <n> --remove-label agent-wip
```

Projects ボードがあれば Status → In Review。

### 10. マージレーン(merge-policy)

- **`merge:agent` なし(既定)**: ここで終了。人間のレビュー・マージを待つ
- **`merge:agent` あり**: 実行条件をすべて確認してからマージする(squash) — (1) CI が green(プロダクト CI + PR↔issue 整合チェックの両方)(2) **実装コンテキストを共有しない別コンテキストのレビュアが green**(3) 鮮度が維持されている。レビュアから要対応の指摘が出たら、`merge:agent` を外して指摘要約を issue コメントに記録し、人間レーンへ再ラベリングして終了する

### 11. 報告

ジャーナルの要約(調査結果・決定・PR URL・検証結果)を issue コメントへ同期し、ユーザーへも報告する。

## エスカレーション条件(正準リスト)

以下のいずれかに該当したら作業を止め、4 点セットを実行する:

- 要件が曖昧・矛盾している(受け入れ条件が検証不能な場合を含む)
- 同一の失敗クラスへの修正 2 回後もテスト・検証が red
- **セキュリティ敏感領域に触れる**: 認証・認可・secrets・CORS・脆弱性のある依存(product-security の正準リスト)
- 破壊的な外部 API 変更(rest-api-design の定義で判定)・DB スキーマ変更が必要になった
- **issue に明記されていない依存(ライブラリ・ツール)の追加**が必要になった(merge-policy / dependency-licensing)
- diff の関心事が単一に絞れない(pr-granularity。分割案を添えて)
- **憲法(プリセット・ローカル ADR)に答えのない設計判断**(「ADR 候補の発見」として /atelier:adr へ誘導)
- スタック事実が実態と乖離している(更新提案として)
- 文書と実装の乖離を発見し、同 PR で直せない

## エスカレーション 4 点セット

1. issue コメントで理由・現状・試したことを説明する
2. 同じコメントに、取り得る選択肢と各々の利点・欠点を添える(人間がそのまま判断材料に使える形。推奨案があれば明示)
3. ラベルを付け替える: `gh issue edit <n> --remove-label agent-wip --add-label needs-human`。失格条件(敏感領域・破壊的変更・明記なき依存)に該当した場合は `merge:agent` も外す
4. ユーザーへ報告する(記録は issue コメントに残す)

## レビュー対応モード(orchestrate からの再配車)

既存の自分の PR に人間のレビューコメントが付いたときの縮退フロー(新規実装ではない):

1. 対象 PR の未解決スレッドを読み、指摘ごとに対応方針を決める(修正する / しない場合は根拠を用意)
2. PR ブランチ上で修正し、必要なら `git fetch && git rebase origin/main`(git-workflow: merge コミット禁止・自分の PR ブランチは force-with-lease 可)
3. push 後、**各スレッドに個別に返信する**: 何をどう変えたか(commit 参照)/ 変えない場合はその理由と根拠。まとめての 1 コメントで済ませない(スレッド単位が人間の追跡単位)
4. 影響範囲が指摘により広がった場合は通常フロー(影響調査 → エスカレーション判断)に戻る

## 禁止事項

- main への直接コミット・push(hook でも機械強制)
- 受け入れ条件を満たすためのテスト弱体化・「通すためだけのテスト」(tdd-doctrine)
- issue 本文の編集(提案はコメントまで。本文は groom の権限)
- 推測での続行(不明点は `AskUserQuestion` かエスカレーション)
