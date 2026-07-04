---
name: init
description: Set up the factory in a repository (idempotent): create operation labels, optionally a Projects board, generate the constitution (ADR 0000) through dialogue, derive stack facts into CLAUDE.md, and scaffold .agents/
tools:
  - Bash(gh auth status, gh repo view, gh label create, gh label list, gh project list, gh project create, gh project link, gh api, git ls-files, git check-ignore)
  - AskUserQuestion
  - Read
  - Write
  - Edit
  - Glob
  - Grep
---

工場の設置スキル。対象リポジトリに factory の前提物(ラベル・憲法・スタック事実・`.agents/`)を揃えます。

**冪等**です。再実行しても既存の ADR・CLAUDE.md の記述を壊しません。**対話専用**です(憲法の生成に人間の判断が要るため、無人実行は禁止)。

## 手順

### 1. 前提確認(fail-closed)

```bash
gh auth status
gh repo view --json nameWithOwner,defaultBranchRef
```

- 認証がなければ**中断**し、`gh auth login` を案内する(`! gh auth login` の実行を提案)
- GitHub リポジトリでなければ中断する

### 2. ラベル作成(冪等)

```bash
gh label create agent-ok --color 0E8A16 --description "エージェントが自律着手してよい" --force
gh label create agent-wip --color FBCA04 --description "エージェント作業中(ミューテックス)" --force
gh label create needs-human --color D93F0B --description "人間の判断待ち。エージェントは触らない" --force
gh label create "priority:high" --color B60205 --description "優先度: 高" --force
gh label create "priority:low" --color C5DEF5 --description "優先度: 低" --force
gh label create "merge:agent" --color 5319E7 --description "agent がマージまで実行してよい(人間が付与。無ければ人間マージが既定)" --force
```

`merge:agent` はマージ軸のラベルで、着手軸(`agent-ok` / `agent-wip` / `needs-human`)と直交する。hook ゲート(#14)がマージ可否の判定に使う。付与は issue の時点で行う: **AI が merge-policy(プリセット)を基に「このタスクは agent マージ可か」を提案し(groom / triage の必須アジェンダ)、人間が承認して付与する**。

### 3. Projects ボード(既定で作成)

`project` スコープがあれば(`gh auth status` で確認)、ボードを**既定で作成する**。スコープがなければ `gh auth refresh -s project` を案内する。ユーザーが不要と明言した場合のみラベル運用にフォールバックする(**ラベルのみでも全スキルは動作する**)。

```bash
# 正準ボード(factory が公開するテンプレート)からのコピーを優先。
# フィールド・ビューごと複製されるため Status の手作業が不要になる(#15)
gh project copy <template-number> --source-owner <canonical-owner> --target-owner <owner> --title "<repo> board"

# コピー元にアクセスできない場合のフォールバック
gh project create --owner <owner> --title "<repo> board"

gh project link <number> --owner <owner>
```

**途中導入・再実行時の転記**: ボードが存在するのに未登録の open issue があれば `gh project item-add` で登録し、ラベルから Status を同期する(`agent-ok` → Ready、`agent-wip` → In Progress、`needs-human` → Spec、いずれも無し → Inbox)。ラベル運用で始めたリポジトリも、init の再実行だけでボード運用へ移行できる。

正準ボードのコピーで Status 選択肢(`Inbox / Spec / Ready / In Progress / In Review / Done`)は複製される想定(正準ボードの整備・検証と、コピーで埋まらない部分を自動化する bin は #15)。フォールバック作成になった場合の Status 選択肢の変更と、issue の auto-add ワークフローの有効化は、残る手作業として完了報告のチェックリストに載せる。

### 4. 憲法の設置(ADR 0000)

`docs/adr/` を確認する:

- **`0000-*.md` が既にある** → 読み込んで内容を要約提示し、変更提案があれば /factory:adr(改憲手続き)へ誘導する。**このスキルでは既存 ADR を書き換えない**
- **ない** → 対話で生成する。`AskUserQuestion` で以下を 1 論点ずつ確定する(最小で始め、フライホイールで育てる。全部を初回に決めない):
  1. テスト方針(例: 失敗するテストを先に書くか、カバレッジより挙動固定を優先するか)
  2. PR 粒度の原則(例: 関心事が単一なら大きくてよいか、成果物ごとに分割するか)
  3. セキュリティベースライン(例: secrets をコードに書かない、認証・認可の変更は必ずエスカレーション)
  4. エラーハンドリング思想(例: fail-fast か縮退運転か)

**ガード(三層の切り分け)**: 対話で挙がった内容は三層に振り分ける。

1. **スタック非依存の原則** → ADR 0000 に書く
2. **技術選定**(「frontend は Vue で組む」「ORM は Prisma」の類)→ 立派なアーキテクチャ決定なので捨てないが、0000 には混ぜず**専用のローカル ADR**(理由・代替案つき)として起こすことを案内する(起こすのは /factory:adr の仕事)
3. **事実**(検証コマンド・ビルド手順)→ 手順 5 のスタック事実(CLAUDE.md)へ

目的はポータブルな原則とプロジェクト固有の決定を混ぜないこと。どちらも憲法の一部だが、置き場が違う。

テンプレート(`docs/adr/0000-constitution.md`):

````markdown
# ADR 0000: 開発憲法

- Status: accepted
- Date: <today>

このリポジトリで働くすべてのエージェント・人間の判断の根拠。
改訂は /factory:adr(人間承認必須)を通す。

> **書いてよいこと**: スタック非依存の原則(テスト方針・セキュリティ・アーキテクチャ原則・PR 粒度)。
> **書いてはいけないこと**: 技術選定(フレームワーク・ライブラリ・言語の指名)。それは専用のローカル ADR として起こす(検証コマンド等の事実は CLAUDE.md のスタック事実へ)。

## 原則

### 1. <対話で確定した原則>

<内容と、なぜそうするか>

## 運用ラベル

agent-ok / agent-wip / needs-human / priority:high / priority:low(意味は factory README 参照)。
種別ラベル(feat / bugfix 等)とは直交する。
````

### 5. スタック事実の導出 → CLAUDE.md

リポジトリの**事実**からビルド・テスト・検証コマンドを導出する。推測で書かない:

1. CI 設定(`.github/workflows/*.yml` 等)の steps が最優先の真実
2. マニフェストの scripts / targets(package.json・Makefile・pyproject.toml・*.csproj など、Glob で存在するものを読む)
3. 見つからない・確信が持てない場合は `AskUserQuestion` でユーザーに確認する(fail-closed)

導出結果をユーザーに提示して確認を取ってから、CLAUDE.md にマーカー付きで記録する。再実行時は**マーカー間のみ**を置換する(それ以外の CLAUDE.md 記述には触らない):

````markdown
<!-- factory:stack-facts:start (managed by /factory:init — edit via re-run) -->
## Factory: スタック事実

リポジトリから導出した事実。憲法(ADR)ではないため、スタック変更時は /factory:init の再実行で更新する。

| 用途 | コマンド | 根拠 |
| --- | --- | --- |
| ビルド | `<command>` | `<CI ファイル名 or マニフェスト>` |
| テスト | `<command>` | `<根拠>` |
| lint / format | `<command>` | `<根拠>` |
<!-- factory:stack-facts:end -->
````

CLAUDE.md が存在しなければ新規作成する。

### 6. `.agents/` scaffold

- `.agents/journal/` を作成(work のジャーナル置き場。`.gitkeep` を置く)
- `.gitignore` に `.worktrees/` がなければ追記する(work が worktree を切る場所)

### 7. 完了報告

実行した / スキップした項目と、残る手動作業を表で報告する:

| 項目 | 状態 |
| --- | --- |
| ラベル 6 種 | ✅ / スキップ(理由) |
| Projects ボード | ✅ / スキップ |
| ADR 0000 | ✅ 新規 / 既存を尊重 |
| CLAUDE.md スタック事実 | ✅ |
| `.agents/` | ✅ |

残る手動作業(提示のみ。実行しない):

- Projects ボードの Status 選択肢変更・auto-add(Web UI)
- ブランチ保護(マージゲートの機械的強制)。**夜間無人モード(Phase 3)を使う前に必須**
- 夜間 cron の登録(Phase 3 で /factory:night と併せて案内)

最後に、最初の issue の起票と /factory:groom での仕様揉みを提案する。

## 注意事項

- **スタック固有のコマンド(pnpm / dotnet 等)をこの SKILL.md にもとづいて決め打ちしない**。必ずリポジトリの事実から導出する
- 既存の ADR・CLAUDE.md・.gitignore の記述を消さない。追記とマーカー間置換のみ
- 通知・記録はすべて GitHub(issue コメント)に置く。外部サービスの設定を要求しない
