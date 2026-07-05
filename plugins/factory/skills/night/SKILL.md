---
name: night
description: cron 用の無人起動口。前提チェック → .agents/unattended sentinel 作成 → orchestrate を unattended で実行 → sentinel 削除。ガードレール・制動条件の実体は orchestrate 側にある。無人専用(人間がいるなら orchestrate を直接使う)
tools:
  - Bash(gh, git, factory, flock, rm, touch, ls)
  - Skill
  - Read
---

**無人専用の起動口。** レーン統一(#4)により「夜のレーン」は存在しない — 本スキルは cron から orchestrate(unattended)を安全に起動して後始末するだけの薄い皮である。対話セッションで呼ばれた場合は、orchestrate を直接使うよう案内して終了する。

## 手順

### 1. 前提チェック(fail-closed: 不成立なら何も実行せず終了)

いずれかが不成立なら、理由を運用 issue(report と同じ「factory operations」issue)にコメントして**即終了**する:

- factory バイナリが使える(`.agents/bin/factory` または PATH)
- `gh auth status` が有効
- required check(`factory-issue-check`)が main のブランチ保護に登録されている(`gh api repos/{owner}/{repo}/branches/main/protection` で確認)— **L3 ゲートなしの無人運転をしない**
- 古い sentinel(`.agents/unattended`)の残留を検出したら、前回の異常終了として警告を記録した上で掃除する

### 2. sentinel の作成

```bash
touch .agents/unattended
```

hook の無人ゲート(改憲ブロック・merge:agent 付与ブロック・配車検証)がこれで発動する。

### 3. orchestrate(unattended)の実行

orchestrate スキルを実行する。制動条件(人間レーン PR 滞留・エスカレーション滞留)・配車・回収はすべて orchestrate の責務。本スキルは介入しない。

### 4. 後始末(必ず実行する)

- `.agents/unattended` を削除する(orchestrate が失敗しても必ず)
- orchestrate のサイクル報告を運用 issue へコメントする(report の入力データになる)

## cron 設置(人間が行う)

多重起動防止は cron 側の `flock` で行う:

```cron
# 平日 3:00 に無人実行(パスは環境に合わせる)
0 3 * * 1-5 cd /path/to/repo && flock -n .agents/night.lock -c 'claude -p "/factory:night" >> .agents/night.log 2>&1'
```

- ログは `.agents/night.log`(gitignore 領域)
- 停止したいときは crontab の行をコメントアウトするだけでよい(工場の耐久状態はすべて GitHub にあるため、いつ止めても壊れない)

## 禁止事項

- 対話セッションでの実行(orchestrate を直接使う)
- sentinel を残したままの終了
- orchestrate を経ない直接の配車・マージ
