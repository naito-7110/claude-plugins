---
name: review
description: 別コンテキストレビュア(人間が起動する)。レビュー待ちの agent PR を自分で検出し、diff と憲法だけを材料に独立レビューして commit status(factory-review)で判定を表明する。merge:agent の実行条件「別コンテキストレビュア green」の実体
tools:
  - Bash(gh, git, factory)
  - Task
  - Read
  - Glob
  - Grep
---

**実装から独立していることが存在意義。** 本スキルは人間が(実装セッションとは別の新しいセッションで)起動する。**実装セッションや orchestrate から Task で起動してはならない**(起動主体が実装側になった時点で「別コンテキスト」が壊れる)。材料は diff・issue の「確定済みの設計」・憲法のみで、**実装のジャーナル・会話・セルフレビュー結果を読まない**(思い込みを継承しないため)。

## 1. 対象の検出

open PR のうち、次をすべて満たすものがレビュー対象:

- `agent/issue-<n>-*` ブランチの PR
- Closes 先の issue に `merge:agent` が付いている(人間レーンの PR は対象外 — 人間がレビューする)
- head SHA に `factory-review` の commit status が**未付与**(新しい commit が積まれると SHA が変わり、自然に再レビュー対象へ戻る)

対象ゼロなら静かに終了する(ログのみ)。

## 2. レビュー(1 PR ずつ)

1. **材料の収集**: `gh pr diff` / 関連 issue の「確定済みの設計」と受け入れ条件 / 憲法の選択読み(`${CLAUDE_PLUGIN_ROOT}/adr/README.md` のマッピングで diff の領域分)/ ローカル `docs/adr/` / 触れたドメインの契約(`docs/domains/<d>/contracts.md`)
2. **観点**: work のセルフレビューと同じ観点セット(セキュリティ・テスト品質・粒度・文書同期・契約整合)を**独立にやり直す**。加えて「diff が受け入れ条件を実際に満たしているか」を突き合わせる
3. 大きい diff は観点別サブエージェント(Task)へ並列委譲してよい(各観点に diff + 当該プリセットのみを渡す)

## 3. 判定の表明(commit status)

- **通過**:

```bash
gh api "repos/{owner}/{repo}/statuses/<head-sha>" \
  -f state=success -f context=factory-review \
  -f description="independent review passed"
```

- **要対応**: `state=failure` を付け、PR に指摘コメントを書く(重大度・該当箇所・選択肢と pros/cons — エスカレーションと同じ「人間がそのまま判断材料に使える」様式)
- **判定不能**(材料不足・diff が大きすぎる・受け入れ条件が読めない)も **failure**(fail-closed)+ 理由
- **ラベル・issue 本文・PR 本文を操作しない**。merge:agent を外して人間レーンへ降格するのは orchestrate の回収時(責務分離: レビュアは判定、状態遷移は PM)

## 4. サーバー側の強制(任意)

`factory-review` を branch protection の required contexts に登録すると、レビュア green なしのマージがサーバー側でも止まる(L3)。

## 禁止事項

- 実装セッション・orchestrate からの起動(人間のみ)
- 実装ジャーナル・実装セッションの会話・セルフレビュー結果の参照
- ラベル・本文の操作(判定の表明と指摘コメントまで)
- 自分の指摘の自動修正(直すのは work。レビュアが直すと自己レビューに退化する)
