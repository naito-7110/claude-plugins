---
name: analyze
description: コード実態の調査(read-only)。issue 番号・パス・現在の diff を受け取り、接触面・参照・呼び出し元・依存・共同変更・既存テスト・公開面・該当ドメインを構造化した「影響マップ」として返す。groom / work / domains が「コードの実態を調べる」段でこれを呼ぶ。自分では何も変更せず、仕様も実装も決めない。args - 対象(issue 番号 / パス / "diff")
tools:
  - Bash(git, gh, atelier)
  - Task
  - Read
  - Glob
  - Grep
---

**read-only。** 対象がコード上で何に触れ、何へ波及するかの地図を作るだけ。**仕様は確定しない(groom)・実装しない(work)・ドメイン境界は裁定しない(domains/huddle)。** 迷ったら断定せず unknown として残す(fail-closed)。

## 入力と深さ

- 対象: `$ARGUMENTS`(issue 番号 / パス群 / 現在の diff のいずれか)
- **呼び出し側の目的で深さを変える**(呼び出し側がプロンプトで指定する):
  - `spec` 目的(groom): 接触面・規模感・現状挙動との突き合わせまで。網羅より速さ
  - `impact` 目的(work): 全参照・全 writer・既存テストまで。正確さ優先(未検出の writer は不完全状態の温床 — domain-modeling の完全性監査と対)

## 手順

1. **接触面の特定**: 対象が触れるパス・モジュールと、公開面(API・イベント・スキーマ・設定)への接触を洗い出す
2. **参照の解決**: 呼び出し元・依存先・逆依存を辿る。広い探索は Explore サブエージェント(Task)へ並列委譲してよいが、**戻りは下記の影響マップ契約に正規化する**
3. **共同変更の観測**: `git log` で「同じ PR / コミットで一緒に変わるファイル群」を観察する(変更理由の単位・ホットスポットの実測)
4. **テストの確認**: 対象領域を覆う既存テストと、その空白を特定する
5. **ドメインの判定**: `.atelier/ownership.yml` があれば接触パスから該当ドメインを引く(**2 ドメイン以上なら huddle 発動のシグナル**)
6. **現状挙動の確認**(spec 目的): issue の前提が現状コードと一致しているか。ズレていれば論点候補として印を付ける

## 出力(影響マップ契約)

呼び出し側が判断に使える構造で返す(記録先は呼び出し側が決める — work はジャーナル、groom は論点材料)。根拠のない項目は作らず unknown へ:

```yaml
impact_map:
  target: string            # 分析対象
  depth: spec | impact
  touchpoints:              # 接触するパス・モジュール
    - path: string
      kind: code | api | event | schema | config
  references:
    callers: []             # 呼び出し元
    dependencies: []        # 依存先
    writers: []             # impact 目的: 対象状態を変える全経路(API/batch/migration/admin/consumer/直接SQL)
  co_change: []             # git log で一緒に変わるファイル群(ホットスポット)
  existing_tests:
    covering: []
    gaps: []
  domains:                  # ownership.yml 由来。2 件以上は huddle シグナル
    touched: []
    multi_domain: true | false
  current_behavior_notes: []# spec 目的: issue の前提とのズレ
  size: S | M | L           # 変更面積の当たり(根拠 1 行)
  risks: []
  unknowns: []
```

## 禁止事項

- コード・issue 本文・文書の変更(本スキルは read-only)
- 仕様の確定・受け入れ条件の決定(groom の権限)・実装(work の権限)
- 根拠のない参照・writer・ドメインの捏造(未確認は unknown)
- 網羅を口実にした過剰探索(呼び出し側の深さ指定を超えない)
