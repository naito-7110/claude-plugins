---
name: triage
description: Inbox 仕分け。運用ラベルを持たない open issue を「エージェントが自律着手できる状態か」で判定し、agent-ok / needs-human / priority を付与して Projects Status を同期する。issue の整理・仕分けを頼まれたとき、または orchestrate の Inbox 処理で使う
tools:
  - Bash(gh issue list, gh issue view, gh issue edit, gh issue comment, gh project item-list, gh project item-edit, gh repo view)
  - AskUserQuestion
  - Read
  - Glob
  - Grep
---

未整理の open issue を「エージェントが自律着手できる状態か」で仕分ける。判定は fail-closed(迷ったら needs-human)。

> **ガードレールは opt-in・縮退可。** 本スキルの価値は「自律着手可否の判定」で、ラベル付与はその**出力形式**にすぎない。運用ラベルが未設置の repo では判定結果を issue コメント + サマリ表で表現する(縮退)。Projects ボードが使えない場合はラベルのみにフォールバックする(既存の手順どおり)。

## 手順

### 1. 対象の収集

```bash
gh issue list --state open --json number,title,labels,body --limit 100
```

`agent-ok` / `agent-wip` / `needs-human` のいずれかを既に持つ issue を除外し、残りを対象とする。

### 2. 判定(spec-alignment プリセットの Ready 条件に照らす)

`${CLAUDE_PLUGIN_ROOT}/adr/spec-alignment.md` の Ready 条件が正準。判定基準:

- **受け入れ条件が機械的に検証可能か**: チェックリストが存在し、各項目が実行できるコマンド・観測できる挙動・「〜が green」の語彙で書かれているか
- **スコープが閉じているか**: 「今回やること / やらないこと」が明確で、外部の決定待ち・他 issue への暗黙依存がないか(依存は `依存: #N` 行で明示されているか)
- **実装に必要な情報が特定できるか**: 対象パス・関連するプリセット / ローカル ADR・仕様が本文か議論から辿れるか

### 3. 判定結果ごとのアクション

**着手可能** → `agent-ok` を付与し、緊急度が本文から読める場合のみ `priority:high` / `priority:low` も付与する。

```bash
gh issue edit <n> --add-label agent-ok
```

- **`merge:agent` は付与しない**(付与は grooming の場に限定 — merge-policy プリセット)。agent マージ可能に見える場合は、merge-policy の失格条件に照らした**提案コメント**までに留める

**情報不足** → 何がどう足りないかを、**人間がそのまま補筆できる粒度**で issue コメントに書き、`needs-human` を付与する。「情報が足りない」とだけ書くことを禁止する。

```bash
gh issue comment <n> --body "受け入れ条件の 3 点目が検証不能です。期待する出力形式(コマンドと green の判定条件)を明記してください"
gh issue edit <n> --add-label needs-human
```

**見送り候補**(重複・方針との矛盾・価値の消失)→ **勝手に閉じない**。理由と選択肢(pros/cons)を添えて `AskUserQuestion` で人間へ提案する(判断材料は issue コメントにも残す)。

**迷う** → fail-closed。`agent-ok` にせず、迷った理由をコメントして `needs-human`。

### 4. Projects Status の同期

ボードがあれば Status を同期する(`agent-ok` → Ready、`needs-human` → Spec)。`gh project` が権限不足等で使えない場合はラベルのみの運用にフォールバックし、**その旨をサマリに明記**する。

### 5. トリアージサマリ

最後に必ず表で報告する:

| # | タイトル | 判定 | 付与ラベル | 理由(1 行) |
| --- | --- | --- | --- | --- |

## 禁止事項

- **人間が書いた issue 本文を編集しない**(本文への反映は groom の役割)
- `merge:agent` を付与しない(grooming 限定)
- issue を閉じない(見送りは人間の同意後のみ、それも人間ゲート側の操作)
- 判定に迷う issue を `agent-ok` にしない

## 備考

- 種別ラベル(feat / bugfix 等)は各リポジトリの流儀に任せ、本スキルは運用ラベルのみを扱う
- issue テンプレート(#13)が未設置のリポジトリでも判定基準は同じ(形式ではなく「検証可能性・スコープ・情報」で判定する)
