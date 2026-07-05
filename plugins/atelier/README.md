# atelier

人間が常駐する開発工場を組み立てるプラグイン。issue を起点に、仕様揉み・TDD 実装・独立レビュー・機械ゲートまでの「仕事の流れ方」を提供する(無人自律運転は #122 で撤去)。設計の全文と経緯は [naito-7110/claude-plugins#4](https://github.com/naito-7110/claude-plugins/issues/4) にある。

## 思想

1. **プロセスと規約の分離** — プラグインが持つのは「仕事の流れ方」だけ。「何が正しいコードか」は各プロジェクトの ADR が持つ。プラグインにスタック固有のコマンドは一切書かない
2. **ADR = 憲法、エージェントは司法** — エージェントは ADR を解釈・適用する。改憲は人間の承認ゲートを必ず通る。ADR に答えがない設計判断は「ADR 候補の発見」としてエスカレーションする
3. **人間 = 意思決定、エージェント = 実行** — 人間のゲートは 3 つだけ: 仕様の確定(groom)・改憲(adr)・マージ
4. **GitHub が唯一の耐久状態** — issue・ラベル・Projects・PR がすべての真実。通知も issue コメントで行う
5. **迷ったら止まる(fail-closed)** — 同一失敗 2 回でエスカレーション、推測で進まない

スタック差分は三層構造で吸収する:

- **ポータブル原則(共有憲法)**: スタック非依存の原則。プロジェクトを跨いで再利用する。技術選定は入れない
- **ローカル ADR**: プロジェクト固有の決定。技術選定(「frontend は Vue」の類)は理由・代替案つきでここに記録する(改訂は /atelier:adr、人間承認必須)
- **スタック事実**: 決定ではなく**事実**(検証コマンド・ビルド手順)。リポジトリ自体(CI 設定・マニフェスト・Makefile)から導出して CLAUDE.md に記録。`/atelier:init` が生成・更新する

ポータブル原則の実体は [`adr/`](./adr/README.md) に同梱する**プリセット ADR コーパス**(参照モデル)。対象リポジトリへはコピーせず、スキルが `${CLAUDE_PLUGIN_ROOT}/adr/` を直接読むため、**プラグインの更新 = 全プロジェクトへの改訂の配布**になる。ローカル ADR が frontmatter で `Overrides: <slug>` を宣言すると、そのプロジェクトでは該当プリセットよりローカルが優先される。

```mermaid
flowchart LR
    subgraph human[人間ゲート]
        G1[仕様確定 groom]
        G2[改憲 adr]
        G3[マージ]
    end
    subgraph board[GitHub Projects / ラベル]
        Inbox --> Spec --> Ready --> InProgress[In Progress] --> InReview[In Review] --> Done
    end
    Inbox -- triage --> Ready
    Inbox -- 情報不足 --> Spec
    Spec --- G1
    Ready -- "orchestrate が配車" --> InProgress
    InProgress -- "work が PR 作成" --> InReview
    InReview --- G3
    work -. "ADR に答えがない" .-> G2
    work -. "needs-human" .-> Spec
```

## スキル

| スキル | 状態 | 役割 |
| --- | --- | --- |
| `/atelier:init` | ✅ | 工場の設置: ラベル・ボード・憲法(ADR 0000)・スタック事実・`.agents/` scaffold |
| `/atelier:adr` | ✅ | 改憲手続き: ローカル ADR の新設・改訂・廃止・Overrides、プリセット候補の還流(人間承認必須) |
| `/atelier:triage` | ✅ | Inbox 仕分け: agent-ok / needs-human / priority 付与(fail-closed) |
| `/atelier:groom` | ✅ | 仕様揉み(対話専用): 確定済みの設計を issue へ書き戻し Ready 化。merge:agent 付与の唯一の場 |
| `/atelier:work` | ✅ | 中核: 影響調査 → worktree → TDD 実装 → 検証 → 文書同期 → セルフレビュー → PR(merge:agent なら条件付きマージまで) |
| `/atelier:domains` | ✅ | ドメイン分割(対話専用): 共変更の実測を根拠に境界を確定し、所有マップと domains 文書を生成 |
| `/atelier:orchestrate` | ✅ | PM: 台帳復元 → ボード読み → 並列配車(最大 2・backpressure)→ 回収 |
| `/atelier:review` | ✅ | 別コンテキストレビュア(人間が起動): agent PR を独立レビューし commit status で判定。merge:agent の最後の条件の実体 |

## 運用ラベル

`/atelier:init` が作成する。エージェント運用ラベルと種別ラベル(feat 等)は直交する。

| ラベル | 意味 |
| --- | --- |
| `agent-ok` | エージェントが自律着手してよい |
| `agent-wip` | エージェント作業中(ミューテックス) |
| `needs-human` | 人間の判断待ち。エージェントは触らない |
| `priority:high` / `priority:low` | 優先度 |
| `merge:agent` | マージ軸(着手軸と直交)。grooming で AI が merge-policy を基に提案し、人間が承認して Ready 化と同時に付与。付いていれば agent がマージまで実行、無ければ人間マージが既定 |

## 旧 factory からの移行

本プラグインは v0.2.x まで「factory」という名前だった(改名は #123)。旧名で設置済みのリポジトリは、そのままでは**管理外扱いになりゲートが働かない**(管理判定が `.atelier/` の存在になったため)。次の 2 つで移行する:

1. `git mv .factory .atelier` を通常の PR で行う
2. `/atelier:init` を再実行する(バイナリ `.agents/bin/atelier` の取得・設置物の更新。冪等なので既存の決定は壊さない)

旧 `.agents/bin/factory` と crontab の旧残骸は `/atelier:uninstall` の手順 1〜2 で掃除できる。

## やめるとき

プラグイン機構に uninstall フックは無いため、**本体を uninstall する前に** `/atelier:uninstall` を実行する(ローカル状態の削除 → 残るものの提示)。手動なら最低限これだけ:

```bash
rm -rf .agents/       # ローカル状態(退避物がないか確認してから)
```

committed の設置物(.atelier/ / docs/adr / .github/)はプロジェクトの所有物なので削除しない。required check を外す場合は **workflow の削除より先に** branch protection の contexts から除去する(逆順だと全 PR が pending で詰まる)。
