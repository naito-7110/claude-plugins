# 0001: init/teardown を manifest 駆動の冪等 PR として実装する

- ステータス: 承認
- 日付: 2026-07-15

## コンテキスト

atelier は `init`(setup)で設置先レポに commit 済みの footprint を置く — `.github/`(issue/PR テンプレート・`atelier-issue-check.yml`・`dependabot.yml`)、`.atelier/`(地図・所有マップ)、`docs/adr/` 雛形、CLAUDE.md のマーカー節。install 方向は冪等を謳う(マーカー間置換・既存を壊さない・ラベルは API で冪等)が、これに対称な**冪等な撤去手段が無い**。

その帰結が #154 だった。atelier をこのリポジトリからアンインストールする作業が、設置物を 1 つずつ手で消す PR になった — install ⇄ uninstall が非対称で、どちらの方向にも定義的に収束しない。

preset `distributable-boundary` 決定4(「設置先 footprint は追跡可能・可逆にする」)が、この問題を配布物一般の原則として定めた。本 ADR はその atelier 実装を定める。

## 決定

**設置物を単一の manifest で正本管理し、setup と teardown を対称な冪等 PR として実装する。**

1. **manifest が正本**: atelier が設置する物を 1 つの manifest に列挙する — パス・種別・撤去の逆再生方法。`init` と `teardown` は**同じ manifest を読む**(設置物の定義を二重に持たない)。
2. **逆再生単位を種別で区別**する:
   - **whole-file**(atelier がまるごと所有: `.github/workflows/atelier-issue-check.yml`・テンプレート・`dependabot.yml`・`.atelier/` 生成物)→ 撤去はファイル削除。
   - **marker-region**(CLAUDE.md のマーカー節)→ 撤去はマーカー間のみ削除。人間の他の記述には触れない。
   - **api-state**(ラベル・Projects ボード)→ API で操作。撤去は残す/消すを選択制にする(共有資源のため既定は残す)。
3. **setup も teardown も PR を生成**する。working tree の直接改変で終わらせない。footprint の変更は他の全変更と同じ**人間マージのゲート**を通す(atelier 思想: GitHub が durable state・人間が merge を gate)。
4. **冪等**: `init` 再実行は差分ゼロなら PR を作らない。`teardown` は manifest の逆再生で footprint を残さず、既に無ければ no-op。
5. **人間の追記を保存**: manifest に無いファイル・マーカー外の記述・雛形に人間が加えた変更には触れない。

### 検討した代替案

- **マーカーのみ(全設置ファイルにマーカーコメントを埋める)**: 生成ファイル全体をマーカーで汚す。かつ GitHub 機構(workflow・issue/PR テンプレート)はマーカーコメントを解釈しないため、ファイル単位の所有には合わない。→ whole-file は manifest 追跡、テキスト混在ファイル(CLAUDE.md)のみ marker-region、と使い分ける。
- **working tree を直接改変する uninstall スクリプト**: PR ゲートを通らず、atelier の「人間が merge を gate」に反する。撤去がレビュー・可逆にならない。→ PR 化する。

## トレードオフ

- **得るもの**: install ⇄ uninstall が定義上収束する。footprint 変更が reviewed-merge ゲートに乗る。#154 の手作業が、再現可能な teardown PR に置き換わる。preset `distributable-boundary` 決定4 を atelier 自身が満たす(ドッグフード)。
- **諦めるもの**: manifest の維持コスト — 設置物を増減するたび manifest を更新する必要がある。manifest と `init` の実挙動が乖離するリスク。
- **緩和策**: manifest を単一正本にし `init`/`teardown` 双方がそれを読む(二重定義を作らない)。乖離は将来 `atelier docs verify` 相当の機械検証で固定する余地がある(本 ADR の範囲外・別 issue)。
