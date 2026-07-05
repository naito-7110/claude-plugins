---
name: init
description: Set up the factory in a repository (idempotent): create operation labels, a Projects board, install the constitution guidance (preset ADR reference + local docs/adr scaffold), derive stack facts into CLAUDE.md, and scaffold the document map (docs/factory), domain docs, and .agents/
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

`merge:agent` はマージ軸のラベルで、着手軸(`agent-ok` / `agent-wip` / `needs-human`)と直交する。hook ゲート(#14)がマージ可否の判定に使う。

**付与は grooming の場に限定する**: AI が merge-policy(プリセット)を基に「このタスクは agent マージ可か」を提案し、人間が承認して **Ready 化と同時に**付与する。triage は提案コメントまで(ラベルは付けない)。無人セッションは付与・変更とも禁止(hook #14 が強制)。これにより「人間がレビューしていない merge:agent」は構造的に存在しない。work / night は着手時に**鮮度チェック**(付与後に issue 本文・受け入れ条件が変わっていないか)を行い、変わっていれば無人マージせず人間レーンへ降格する。

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

### 4. 憲法の案内の設置(プリセット参照 + 固有分のみ対話)

憲法は三層(プリセット / ローカル ADR / スタック事実)。**プリセットはプラグイン同梱**(`${CLAUDE_PLUGIN_ROOT}/adr/`)の参照モデルで、リポジトリへ**コピーしない**(プラグイン更新に追従させるため)。一括適用で選択 UX は設けない。

1. **CLAUDE.md にマーカー付きで案内を設置する**(再実行時はマーカー間のみ置換):

````markdown
<!-- factory:constitution:start (managed by /factory:init — edit via re-run) -->
## Factory: 憲法

- 開発判断の根拠は二層: factory プラグイン同梱のプリセット ADR(ポータブル原則。一覧はプラグインの `adr/README.md`)+ このリポジトリの `docs/adr/`(プロジェクト固有の決定。技術選定はこちら)
- スキルはプリセットを `${CLAUDE_PLUGIN_ROOT}/adr/` から読む
- ローカル ADR が frontmatter で `Overrides: <slug>` を宣言した場合、該当プリセットよりローカルが優先
- 改訂は /factory:adr(人間承認必須)。検証コマンド等の事実は下のスタック事実の節へ
<!-- factory:constitution:end -->
````

2. **`docs/adr/` を確認する**:
   - 既存の ADR があれば読み込んで要約提示する(**書き換えない**。変更提案は /factory:adr へ誘導)
   - 無ければ `docs/adr/README.md`(NNNN 採番・`Overrides: <slug>` 規約・/factory:adr 経由の改訂、を説明する小さな案内)を作成する

3. **対話はプロジェクト固有の原則のみ**: プリセットの守備範囲(テスト方針・セキュリティ 2 領域・ログ・フラグ・PR 粒度・エラーハンドリング・認可・排他制御・API 設計・i18n・パフォーマンス・仕様すり合わせ・マージポリシー)を一覧提示し、**「プリセットで足りない、このプロジェクト固有の原則・制約」だけ**を `AskUserQuestion` で確認する。確定した固有原則はローカル ADR として `docs/adr/` に起こす(出なければ起こさない。最小で始め、フライホイールで育てる)

**ガード(三層の切り分け)**: 対話で挙がった内容は三層に振り分ける。

1. **スタック非依存の原則** → プリセットに既にあるなら重複記録しない(該当プリセットを示す)。プリセットに無い汎用な原則は「プリセット候補」としてプラグインへの PR を提案する
2. **技術選定**(「frontend は Vue で組む」「ORM は Prisma」の類)→ 立派なアーキテクチャ決定なので捨てないが、**専用のローカル ADR**(理由・代替案つき)として起こす
3. **事実**(検証コマンド・ビルド手順)→ 手順 5 のスタック事実(CLAUDE.md)へ

目的はポータブルな原則とプロジェクト固有の決定を混ぜないこと。どちらも憲法の一部だが、置き場が違う。

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

### 6. 文書構造と `.agents/` の scaffold

**factory バイナリ(`.agents/bin/`)— 必ず `.factory/` scaffold より先に取得する**:

> **この順序は本質的な依存(#103)**: hook ゲートは「`.factory/` の存在 = factory 管理下」で発動し、管理下でバイナリが欠落していると fail-closed で全コマンドを止める。`.factory/` を先に作るとバイナリ取得コマンド自体がブロックされる鶏卵デッドロックになる。

- Releases(タグ `factory/vX.Y.Z`)から OS / arch に合う factory バイナリを `gh release download` で取得し、checksums.txt を検証して `.agents/bin/factory` に置く(`.agents/` は gitignore 済み — バイナリをコミットしない)
- リリースが未整備・取得不能な場合はスキップし、その旨を完了報告に載せる(hook ゲート・スキルの前提チェックはバイナリ無しでは縮退動作になる)
- 万一デッドロックに入った場合(`.factory/` あり・バイナリなし)、ゲートの拒否メッセージに含まれるブートストラップ 1 行を**人間が `!` プレフィックスで直接実行**すれば復旧できる(`!` 実行は hook を通らない)

**factory 運用ファイル(`.factory/`)** — 配置の基準は「誰の持ち物か」(documentation プリセット): factory が生成し機械が読む運用ファイルは `.factory/`(dotdir、既存リポジトリの慣習と競合しない)、人間の一次文書(ADR・ドメイン知識)は標準の `docs/` に置く。無ければ作成する(既存は壊さない)。**`.factory/` はコミット対象**(gitignore しない)。**`.factory/` を作った時点でこのリポジトリは hook ゲートの対象になる**(バイナリ取得を先に済ませていること):

- `.factory/README.md`: 文書の地図。各層(プリセット + `docs/adr/` / `docs/domains/` / CLAUDE.md マーカー節 / `.factory/`)の場所と役割の案内
- `.factory/ownership.yml`: 機械可読な所有マップ。`factory docs verify` が検証する:

```yaml
# パス(glob)→ ドメインの所有マップ
domains:
  <domain>:
    paths:
      - "src/<domain>/**"
```

- ドメイン未分割のリポジトリは `domains: {}` で開始してよい(漸進導入)

**ドメインの定義・分割は init では行わない**。`domains: {}` の空マップを置くまでが init の仕事で、ドメインを切る意思決定と `docs/domains/` 雛形の生成は /factory:domains(専用スキル)が対話で行う(分割基準は domain-partitioning プリセット)。

**issue / PR テンプレート(`.github/`)** — GitHub のテンプレート機構はリポジトリ内のファイルしか読まないため、プラグイン同梱のテンプレート(`${CLAUDE_PLUGIN_ROOT}/templates/`)を**コピーで設置**する:

- `.github/ISSUE_TEMPLATE/template.md` と `.github/PULL_REQUEST_TEMPLATE.md`
- `.github/workflows/factory-issue-check.yml`(PR↔issue 整合のサーバー側ゲート。`templates/workflows/` から)
- `.github/dependabot.yml`(github-actions ecosystem + cooldown。**設置した workflow の SHA 固定を風化させないための必須セット**。言語 ecosystem はスタック確定後にレシピを参照して追記)
- 無ければコピーする。**既にある場合は差分を提示して上書き可否を人間に確認する**(黙って壊さない)。プラグイン更新への追随は init の再実行(同じ確認フロー)で行う

**作業状態(`.agents/`)**:

- `.agents/journal/` を作成(work のジャーナル置き場。`.gitkeep` を置く)
- `.gitignore` に `.worktrees/` と `.agents/` がなければ追記する

### 6.4 後入れ認識 = 文書監査(既存コードがあるリポジトリのみ・対話)

既にコードのあるリポジトリでは、決定はコードに暗黙で存在し、文書は「ある場合」も「ない場合」も「あるが古い場合」もある。**init がやるのは棚卸し・突き合わせ・起票まで。生成・是正の本体は issue 化して factory の通常レーン(triage → groom → work)で処理する**(監査は網羅、対処は漸進):

1. **棚卸し**: 既存文書を網羅的に列挙する — README・docs/ 配下・別形式の ADR・設計メモ・コード内の規約コメント。大きいリポジトリでは観点別サブエージェントに並列で読ませてよい
2. **突き合わせ**: 各文書をコードと照合して 3 分類する
   - **正しい** → `.factory/README.md`(地図)に登載し、`ownership.yml` の所有に含める(以後は documentation プリセットの「同 PR 更新」が腐敗を防ぐ)
   - **古い / コードと矛盾** → 是正 issue を起票してレーンに乗せる(init 中に直さない)
   - **決定の記録に相当する内容** → 下記 3 の記録 ADR へ蒸留する候補に加える
3. **暗黙決定の抽出と記録 ADR 化**: コード解析で暗黙の決定候補を抽出する。優先するのは、プリセットが「プロジェクト判断」に委ねている点: 技術選定(フレームワーク・ORM・主要ライブラリ)・レイヤ構成・エラー処理の型・認可の置き場所・命名 / 配置規約。候補を一覧提示し、**人間が選んだものだけ**を「記録 ADR」として `docs/adr/` に起こす(冒頭に**「遡及記録: 既に採用済みの決定の文書化であり、新規決定ではない」**を明示。改廃は通常の /factory:adr で)
4. **欠落の起票**: 文書が無い領域は、factory の文書モデル(スタック事実 / ローカル ADR / ドメイン文書)に対する欠落として issue 化する。ドメイン文書の生成は /factory:domains をオンボーディングの続きとして推奨する(当たり = ディレクトリ構成・共変更の 1 段解析だけここで示す)
5. **`.agents/` には現状認識をシードしない** — 台帳は orchestrate が GitHub と worktree の実態から毎回再構築する設計であり、キャッシュに真実を持たせない

新規リポジトリ(コードがまだ無い)ではこのステップをスキップする。既存の open issue も変換しない(triage が触るときに漸進整形)。

### 6.5 hook ゲートの検査

hook はプラグインの `hooks/hooks.json` により**プラグイン有効化で自動登録される**(手動の settings 編集は不要)。init が行うのは検査と案内のみ:

- 実行依存(`jq`・factory バイナリ)が揃っているかを確認する
- hook の有効性を確認する(登録状態は `/hooks` で確認できる旨を案内。プラグイン更新直後は新しいセッションで反映されることも添える)
- 結果を完了報告に載せる。**夜間無人(Phase 3)は hook ゲートの有効化が前提条件**であることを添える

### 7. 完了報告

実行した / スキップした項目と、残る手動作業を表で報告する:

| 項目 | 状態 |
| --- | --- |
| ラベル 6 種 | ✅ / スキップ(理由) |
| Projects ボード | ✅ / スキップ |
| 憲法(CLAUDE.md マーカー節 + docs/adr/ 案内) | ✅ / 既存 ADR を尊重 |
| CLAUDE.md スタック事実 | ✅ |
| factory 運用ファイル(.factory: 地図 + 空の所有マップ) | ✅ / 既存を尊重 |
| issue / PR テンプレート + issue-check workflow(.github) | ✅ / 既存は確認の上 |
| factory バイナリ(.agents/bin) | ✅ / スキップ(リリース未整備) |
| 後入れ認識(文書監査) | 登載 n / 是正 issue n / 記録 ADR n / スキップ(新規) |
| hook ゲート | 有効(自動登録) / 要確認(依存欠落・新セッション待ち) |
| `.agents/` | ✅ |

残る手動作業(提示のみ。実行しない):

- Projects ボードの Status 選択肢変更・auto-add(Web UI)
- **factory-issue-check を required status check に登録**(全クライアントを縛る L3。unattended 運転の前提条件): `gh api -X PUT "repos/{owner}/{repo}/branches/main/protection" --input -` で `required_status_checks.contexts` に `factory-issue-check` を含める(既存の保護設定とマージすること)。**agent マージを使う場合は `factory-review` も追加**(レビュア green なしのマージをサーバー側でも遮断)
- **cron の登録**(unattended 運転を使う場合): `factory tick install` で設置する(生成行は `factory tick run` — 多重起動防止を内蔵し flock コマンドに依存しない。crontab 例は night スキル参照)
- **GitHub の「Automatically delete head branches」を有効化**(Settings → General): マージ済みリモートブランチの掃除は GitHub に委ね、ローカルは `factory branch cleanup` が担う
- ブランチ保護(マージゲートの機械的強制)。**夜間無人モード(Phase 3)を使う前に必須**
- 夜間 cron の登録(Phase 3 で /factory:night と併せて案内)

最後に、最初の issue の起票と /factory:groom での仕様揉みを提案する。

## 注意事項

- **スタック固有のコマンド(pnpm / dotnet 等)をこの SKILL.md にもとづいて決め打ちしない**。必ずリポジトリの事実から導出する
- 既存の ADR・CLAUDE.md・.gitignore の記述を消さない。追記とマーカー間置換のみ
- 通知・記録はすべて GitHub(issue コメント)に置く。外部サービスの設定を要求しない
