// Package cli は factory コマンドの引数解析とサブコマンド実行を担う。
package cli

import (
	"flag"
	"fmt"
	"io"

	"github.com/naito-7110/claude-plugins/tools/factory/internal/board"
	"github.com/naito-7110/claude-plugins/tools/factory/internal/tick"
)

// Deps は実行時依存(GraphQL クライアント生成・カレントリポジトリ解決・
// crontab 操作)。テストでは fake を注入する。
type Deps struct {
	NewClient     func() (board.GraphQL, error)
	CurrentRepo   func() (string, error)
	CurrentBranch func() (string, error) // カレントブランチ(unborn でも名前を返す)
	Crontab       tick.Crontab
	In            io.Reader // hook JSON の入力(gate)
	Out           io.Writer
	Err           io.Writer
}

// 終了コード。
const (
	ExitOK    = 0
	ExitError = 1
	ExitUsage = 2
	// ExitBlock は hook 契約のブロック(PreToolUse は exit 2 でツール呼び出しを拒否する)。
	ExitBlock = 2
)

const usage = `使い方: factory <board|issue|pr|docs|flags|mode|tick|gate|branch> <サブコマンド> [オプション]

サブコマンド:
  board copy    正準ボード(factory board template)を対象 owner へ複製する
                --owner <owner>        複製先の user / organization(必須)
                --title <title>        ボード名(省略時: "<リポジトリ名> board")
                --repo <owner/name>    リンクするリポジトリ(省略時: カレントリポジトリ)
                --source-owner <o>     正準ボードの owner(既定: naito-7110)
                --source-number <n>    正準ボードの番号(既定: 4)
  board verify  ボードの Status 6 値(ハードゲート)とビュー・日付フィールド(警告)を検証する
                --owner <owner>        対象の user / organization(必須)
                --number <n>           ボード番号(必須)
  issue verify  issue の整合(spec-alignment / merge-policy の機械検証可能な部分)を検証する
                --number <n>           issue 番号(必須)
                --repo <owner/name>    対象リポジトリ(省略時: カレントリポジトリ)
                --checks <list>        検査項目のカンマ区切り
                                       (acceptance,labels,deps,freshness。既定: 全部)
  pr verify     PR 本文の Closes/Refs #N から関連 issue を解決し issue verify 相当を実行する
                --number <n>           PR 番号(必須)
                --repo <owner/name>    対象リポジトリ(省略時: カレントリポジトリ)
                --checks <list>        関連 issue への検査項目(issue verify と同じ)
                --dep-manifests <list> 依存マニフェストの glob パターンのカンマ区切り
                                       (例: go.mod,go.sum,**/package.json。未指定なら検査スキップ)
  docs verify   文書構造(documentation プリセット)を検証する
                文書の地図・所有マップ(ownership.yml)の形式・ドメイン文書の必須構造・
                所有マップと実パスの整合(検査対象のパスは internal/docs の Layout に単一定義)
                --root <dir>           リポジトリのルート(既定: カレントディレクトリ)
  flags check   フラグレジストリ(feature-flags プリセット)を検証する
                .factory/flags.yaml の形式(owner / expires_on / description 必須)と期限を検査する。
                レジストリなし = 正常(フラグ未使用)。期限切れは NG、期限接近は警告のみ
                --root <dir>           リポジトリのルート(既定: カレントディレクトリ)
                --warn-days <n>        期限接近を警告する日数(既定: 14)
  mode <verb>   unattended 運転の運転状態(このマシンのローカル状態)を管理する
                verb: status / auto / manual / gate
                状態は auto / manual の二値で .agents/ 配下のローカルファイル(コミットしない)。
                既定 = manual(状態ファイル欠落時も manual — fail-closed)。
                「今すぐ止める」も factory mode manual で即効する
                gate は night の判定入口: auto のときだけ exit 0
                (それ以外は理由を stderr に出して非ゼロ)
                --root <dir>           リポジトリのルート(既定: カレントディレクトリ)
  tick install  crontab に unattended 運転の起動行を設置する(既存ブロックは置換)
                # factory-tick begin / end のマーカーブロックだけを操作し、他の行には触れない
                --root <dir>           リポジトリのルート(既定: カレントディレクトリ)
                --schedule "<cron 式>" 起動スケジュール(既定: "0 3 * * 1-5" = 平日 3:00)
  tick remove   マーカーブロックを crontab から除去する(他の行には触れない)
  tick status   tick の設置有無と内容を表示する
  branch cleanup  マージ済み agent ブランチ(agent/issue-*)を掃除する
                PR 状態を正として判定(squash マージ運用のため --merged は使えない)。
                merged / closed の PR に紐づくブランチだけを削除し、現在ブランチ・
                open PR・PR なし・パターン外には触れない。紐づく worktree は
                未コミット変更が無い場合のみ除去(あればスキップ + 警告)。
                最後に git remote prune origin を実行する
                --dry-run              削除せず対象の一覧だけ表示する
                --root <dir>           リポジトリのルート(既定: カレントディレクトリ)
                --repo <owner/name>    対象リポジトリ(省略時: カレントリポジトリ)
  gate          PreToolUse hook の機械的ゲート(factory-gate.sh から exec される)
                stdin から hook JSON(tool_name / tool_input)を読み、
                main 直 push / push ゲート / マージゲート / 無人 3 種を判定する。
                ブロック時は理由を stderr に出して exit 2(hook 契約)、通過は exit 0
                --root <dir>           リポジトリのルート(既定: カレントディレクトリ)
`

// Run は引数を解釈してサブコマンドを実行し、終了コードを返す。
func Run(args []string, deps Deps) int {
	// gate は単語 1 つのサブコマンド(hook から exec される入口)。
	if len(args) >= 1 && args[0] == "gate" {
		return runGate(args[1:], deps)
	}
	if len(args) < 2 {
		fmt.Fprint(deps.Err, usage)
		return ExitUsage
	}
	switch args[0] + " " + args[1] {
	case "board copy":
		return runCopy(args[2:], deps)
	case "board verify":
		return runVerify(args[2:], deps)
	case "issue verify":
		return runIssueVerify(args[2:], deps)
	case "pr verify":
		return runPRVerify(args[2:], deps)
	case "docs verify":
		return runDocsVerify(args[2:], deps)
	case "flags check":
		return runFlagsCheck(args[2:], deps)
	case "mode status", "mode auto", "mode manual", "mode gate":
		return runMode(args[1], args[2:], deps)
	case "tick install", "tick remove", "tick status":
		return runTick(args[1], args[2:], deps)
	case "branch cleanup":
		return runBranchCleanup(args[2:], deps)
	default:
		fmt.Fprint(deps.Err, usage)
		return ExitUsage
	}
}

func runVerify(args []string, deps Deps) int {
	fs := flag.NewFlagSet("board verify", flag.ContinueOnError)
	fs.SetOutput(deps.Err)
	owner := fs.String("owner", "", "対象の user / organization")
	number := fs.Int("number", 0, "ボード番号")
	if err := fs.Parse(args); err != nil {
		return ExitUsage
	}
	if *owner == "" || *number == 0 {
		fmt.Fprintln(deps.Err, "--owner と --number は必須です")
		fmt.Fprint(deps.Err, usage)
		return ExitUsage
	}

	client, err := deps.NewClient()
	if err != nil {
		fmt.Fprintln(deps.Err, err)
		return ExitError
	}
	report, err := board.Verify(client, *owner, *number)
	if err != nil {
		fmt.Fprintln(deps.Err, err)
		return ExitError
	}
	return printReport(deps, report)
}

func runCopy(args []string, deps Deps) int {
	fs := flag.NewFlagSet("board copy", flag.ContinueOnError)
	fs.SetOutput(deps.Err)
	opts := board.CopyOptions{}
	fs.StringVar(&opts.TargetOwner, "owner", "", "複製先の user / organization")
	fs.StringVar(&opts.Title, "title", "", "ボード名")
	fs.StringVar(&opts.Repo, "repo", "", "リンクするリポジトリ(owner/name)")
	fs.StringVar(&opts.SourceOwner, "source-owner", board.DefaultSourceOwner, "正準ボードの owner")
	fs.IntVar(&opts.SourceNumber, "source-number", board.DefaultSourceNumber, "正準ボードの番号")
	if err := fs.Parse(args); err != nil {
		return ExitUsage
	}
	if opts.TargetOwner == "" {
		fmt.Fprintln(deps.Err, "--owner は必須です")
		fmt.Fprint(deps.Err, usage)
		return ExitUsage
	}

	// --repo 省略時はカレントリポジトリへリンクする(解決できなければスキップ)。
	if opts.Repo == "" {
		repo, err := deps.CurrentRepo()
		if err == nil {
			opts.Repo = repo
		} else {
			fmt.Fprintln(deps.Out, "==> カレントリポジトリを解決できないため、リンクはスキップします(--repo で指定できます)")
		}
	}
	if opts.Title == "" {
		opts.Title = defaultTitle(opts.Repo)
	}

	client, err := deps.NewClient()
	if err != nil {
		fmt.Fprintln(deps.Err, err)
		return ExitError
	}

	fmt.Fprintf(deps.Out, "==> 正準ボード %s/projects/%d を %s へ複製します\n",
		opts.SourceOwner, opts.SourceNumber, opts.TargetOwner)
	result, err := board.Copy(client, opts)
	if err != nil {
		fmt.Fprintln(deps.Err, err)
		return ExitError
	}
	fmt.Fprintf(deps.Out, "==> 複製完了: %s (#%d)\n", result.URL, result.Number)
	fmt.Fprintln(deps.Out, "==> 説明欄を設定しました")
	if result.LinkedRepo != "" {
		fmt.Fprintf(deps.Out, "==> %s にリンクしました\n", result.LinkedRepo)
	}
	code := printReport(deps, result.Report)
	printChecklist(deps.Out, result.URL)
	return code
}

func defaultTitle(repo string) string {
	if repo == "" {
		return "factory board"
	}
	name := repo
	for i := len(repo) - 1; i >= 0; i-- {
		if repo[i] == '/' {
			name = repo[i+1:]
			break
		}
	}
	return name + " board"
}

func printReport(deps Deps, report board.Report) int {
	code := ExitOK
	if report.StatusOK {
		fmt.Fprintf(deps.Out, "OK: %s/projects/%d の Status は factory の 6 値です\n",
			report.Owner, report.Number)
	} else {
		fmt.Fprintln(deps.Err, "NG: Status の選択肢が期待と異なります")
		fmt.Fprintf(deps.Err, "  期待: %v\n", board.StatusOptions)
		fmt.Fprintf(deps.Err, "  実際: %v\n", report.ActualStatus)
		code = ExitError
	}
	for _, warning := range report.Warnings {
		fmt.Fprintf(deps.Out, "警告: %s\n", warning)
	}
	return code
}

func printChecklist(out io.Writer, url string) {
	fmt.Fprintf(out, `
==> 残る手作業(API では自動化できません):
  [ ] auto-add ワークフローの有効化(issue を自動で Inbox に入れる):
      %s → 右上「…」→ Workflows → Auto-add to project
      → 対象リポジトリを選択、フィルタ「is:issue is:open」、Status = Inbox で有効化
`, url)
}
