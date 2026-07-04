---
name: groom
description: 仕様揉み(対話専用・無人実行禁止)。issue の曖昧点・エッジケース・受け入れ条件の漏れを人間と 1 論点ずつ確定し、「確定済みの設計」として本文へ書き戻して Ready 化する。merge:agent の付与はこのスキルだけが行える
tools:
  - Bash(gh issue view, gh issue edit, gh issue comment, gh issue list, gh project item-list, gh project item-edit, gh repo view)
  - AskUserQuestion
  - Read
  - Glob
  - Grep
---

**対話専用。無人セッション(night 等)では絶対に実行しない。** 仕様の確定は人間ゲートであり、このスキルはその意思決定の場を整える。**issue 本文を編集してよいのは本スキルのみ**(triage・work は提案コメントまで)。

## 手順

### 1. 読み込み

- `gh issue view <n> --comments` で本文と議論を読む
- 関連コードを Glob / Grep で確認し、影響範囲の当たりをつける
- **憲法の選択読み**: `${CLAUDE_PLUGIN_ROOT}/adr/README.md` の選択読みマッピングに従う(常時読みセット + issue の変更領域に該当する分)。ローカル `docs/adr/` も確認する

### 2. 論点の列挙

曖昧点・矛盾・エッジケース・エラーパス・受け入れ条件の漏れを列挙し、重要度順に並べる。

### 3. 1 論点ずつ人間と確定

`AskUserQuestion` で 1 論点ずつ確定する。選択肢には必ず trade-off を添える(人間がそのまま判断材料に使える形)。

### 4. 必須アジェンダ

正準は `${CLAUDE_PLUGIN_ROOT}/adr/spec-alignment.md`。issue の内容にかかわらず次を扱い、**該当しない項は「該当なし」を確認して高速に閉じる**:

1. **排他制御の選択**(concurrency-process): 書き込みを伴うか。楽観(既定)か悲観か
2. **feature flag の要否**(feature-flags): 必要なら name / owner / 期限をその場で決める
3. **最小 PR の分割案**(pr-granularity): 依存順つきの分割案を合意する
4. **API リソース設計**(rest-api-design。REST の場合): メソッド × パス・ネスト構造を確定する
5. **マージレーンの提案**(merge-policy): 失格条件(敏感領域・破壊的変更・スキーマ変更)に照らして agent マージ可否を提案し、**人間が承認する**
6. **依存の追加**(dependency-licensing / supply-chain-security): 新規依存を洗い出して明記。選定は pros/cons + ライセンス確認つき

### 5. 憲法照合と ADR 候補の発見

- 確定しようとしている仕様がプリセット・ローカル ADR と矛盾しないかを確認する。矛盾する場合は仕様を直すか、改憲(/factory:adr)を先行させる
- **憲法に答えのない設計判断が含まれる場合は「ADR 候補の発見」として /factory:adr へ誘導**し、改憲の結果を待ってから該当論点を確定する(フライホイールの入口)

### 6. 本文への書き戻し

確定した内容を issue 本文へ反映する(`gh issue edit <n> --body-file`):

- **「確定済みの設計(YYYY-MM-DD groom で確定)」節**として、日付と出所つきで昇格させる
- 受け入れ条件は**機械的に検証可能なチェックリスト**(実行コマンド・「〜が green」の語彙)に更新する
- 分割・依存は `依存: #N` 行、必要な人間ゲートは `[H]` マーカーで明示する

### 7. Ready 化

```bash
gh issue edit <n> --add-label agent-ok --remove-label needs-human
```

- **手順 4-5 で人間が agent マージを承認した場合のみ、ここで `merge:agent` を付与する**(Ready 化と同時。他のどの場でも付与しない — merge-policy)
- Projects ボードがあれば Status → Ready(なければラベルのみでよい)

## 禁止事項

- 無人実行(人間のいないセッションでの起動)
- 人間の確認なしの論点確定・本文書き戻し
- grooming を経ない `merge:agent` 付与

## 備考

- 憲法(プリセット)が育つまでは論点の多くが「憲法に答えがない」側に倒れ、人間の負担が大きい。これは想定どおりで、改憲のたびに次回から自律確定できる範囲が広がる
