---
name: verify
description: 変更の検証(実行と報告のみ)。diff が触れた領域に該当する検証コマンド(ビルド・テスト・lint)だけを CLAUDE.md のスタック事実から導出して実行し、結果を構造化して返す。work の実装後検証・review の独立再現がこれを呼ぶ。コマンドは自分で発明せず、スタック事実に無ければ実行せず更新提案を返す。自分では直さない。args - 対象("diff" / パス群)
tools:
  - Bash
  - Read
  - Glob
  - Grep
---

**実行と報告まで。** diff が触れた領域を、スタック事実に書かれたコマンドで検証し、結果を返すだけ。**失敗を自分で直さない**(修正は呼び出し側 work の責務 — verify が直すと自己検証に退化する)。コマンドを推測で発明しない。判定できないものは blocked として返す(fail-closed)。

## 入力

- 対象: `$ARGUMENTS`(現在の diff / パス群)。指定が無ければ現在の worktree の diff を対象にする

## 手順

1. **検証コマンドの導出**: CLAUDE.md の「Atelier: スタック事実」節から、diff が触れた領域に該当するコマンド(ビルド・テスト・lint)**だけ**を取る。触れていない領域の検証は実行も記載もしない
2. **スタック事実の点検**: 節が無い・コマンドが実態と合わない領域は、実行せず `stack_fact_gaps` に積む(呼び出し側が /atelier:init 再実行を提案するための材料)。**コマンドを推測で補完しない**
3. **実行**: 導出したコマンドを実行し、出力を捕捉する。テストは状態検証の結果をそのまま扱う(tdd-doctrine)
4. **分類**: 各コマンドを pass / fail に分ける。fail は**同一の失敗クラス**で束ねて要約する(呼び出し側の自己修正が「同一クラス 2 回まで」を数えられるように)

## 出力(検証契約)

呼び出し側が判断に使える構造で返す(記録先は呼び出し側 — work はジャーナル、review は判定材料)。根拠のない項目は作らない:

```yaml
verification:
  target: string
  commands:               # 実行したコマンドと結果
    - cmd: string
      area: build | test | lint
      result: pass | fail
      failure_class: string   # fail 時のみ。同種の失敗を束ねる名前
      summary: string         # 該当箇所・観測した失敗
  skipped_areas: []       # スタック事実にあるが diff が触れていない領域(意図的な非実行)
  stack_fact_gaps: []     # スタック事実が無い/実態と合わない領域(エスカレーション材料)
  verdict: green | red | blocked   # blocked = コマンドを導出できず検証不能
```

## 禁止事項

- コマンドの発明(スタック事実に無いコマンドを推測実行しない — 無ければ `stack_fact_gaps` へ)
- 失敗の自己修正(直すのは呼び出し側の work。verify は実行と報告まで)
- diff が触れていない領域の検証実行・記載(触れた領域だけ)
