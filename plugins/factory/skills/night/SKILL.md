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

### 0. 運転モードの確認(人間のスイッチ。fail-closed = 既定は走らない)

- **`factory mode gate` を実行し、非ゼロなら即終了する**。運転状態は **auto / manual の二値**(bin が管理するローカル状態、`.agents/` 配下)で、既定は manual(明示的に `factory mode auto` されたマシンだけが無人運転する)
- **運転状態は git 管理外・マシンごとに独立**(`.agents/` は init が .gitignore へ追記する): コミット履歴を濁さず、他の人の環境にも影響しない。同じリポジトリを複数人が clone しても、無人運転するのは `factory mode auto` した本人のマシンだけ
- 状態ファイルを直接触らない(操作は常に `factory mode ...` 経由 — orchestrate に「止めて」と頼むフローも同じ bin に落ちる)
- この終了は**正常系**(ログにのみ記録し、issue コメントは出さない — 毎 tick のスパム防止。設定異常の記録は次の前提チェックの役目)

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

## 起動機構(tick)の設置(人間が行う)

**本質は cron ではない。** orchestrate は re-entrant(台帳 + GitHub の状態から毎回復元して 1 サイクルで終わる)なので、「常駐」と「定期起動」は外側の周期の違いでしかない。**1 つの常駐プロセスに寄せない理由**: プロセスが死んだ瞬間に工場が止まる(監視が要る)・コンテキストが伸び続ける。定期 tick なら死んでも次の tick で必ず蘇り、毎回フレッシュなコンテキストで動く。**リアルタイム性が欲しければ間隔を縮めればよい**。

いずれの方式でも多重起動防止は `flock` で行う:

```bash
# 方式 A: cron(最も堅い。夜だけ回す例)
0 3 * * 1-5 cd /path/to/repo && flock -n .agents/night.lock -c 'claude -p "/factory:night" >> .agents/night.log 2>&1'

# 方式 B: 準リアルタイムの常駐風ラッパー(15 分間隔の tick。稼働時間帯は好みで)
while true; do
  flock -n .agents/night.lock -c 'claude -p "/factory:night" >> .agents/night.log 2>&1' || true
  sleep 900
done
```

- launchd / systemd timer も方式 A の同類として使える
- ログは `.agents/night.log`(gitignore 領域)
- **tick は入れっぱなしでよい**。走ってよいかは人間のスイッチ(bin)が決める:
  - `factory mode auto` / `manual`: 無人運転の許可 / 禁止(**二値**。既定 manual。状態はローカル・非コミット・即効 — 「今すぐ止める」も manual でよい)
  - `factory tick install` / `remove` / `status`: crontab のマーカーブロックを冪等に管理(手で crontab を書いてもよい)
  - **人間は orchestrate(PM)との会話から頼めばよい**(「止めて」→ `factory mode manual`、「無人運転を設置して」→ `factory tick install`)
- 停止はいつでも安全(工場の耐久状態はすべて GitHub にあるため、どのタイミングで止めても壊れない)
- issue イベント駆動(webhook で即起動)は将来の拡張候補(現状は tick 間隔の短縮で近似できる)

## 禁止事項

- 対話セッションでの実行(orchestrate を直接使う)
- sentinel を残したままの終了
- orchestrate を経ない直接の配車・マージ
