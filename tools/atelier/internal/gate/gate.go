// Package gate は atelier の機械的ゲート(PreToolUse hook)の判定を提供する。
//
// 判定は plugins/atelier/hooks/atelier-gate.sh からの移行であり、仕様は現行の
// bash 実装と同一(#73: 移行のみ。ゲートの追加・仕様変更はしない)。シェル側は
// バイナリを exec するだけの薄いラッパーになり、grep / jq への依存が消える。
//
// ゲート(#4 の hook 集約決定。無人 3 種は #122 で撤去 — 人間常駐前提):
//  1. main への直 push / force push: 常にブロック
//  2. push ゲート: agent/issue-<n>-* ブランチの push 前に issue の状態を検証
//  3. マージゲート: PR↔issue 整合 + Closes 紐づけ + merge:agent + CI green +
//     別コンテキストレビュア(atelier-review status = success)
//  4. リリースゲート: atelier release の実行(--dry-run を除く)とタグ push を
//     常にブロック(merge-policy: デプロイ = 人間の tag push。release を便利
//     コマンド化した瞬間に AI が押せる引き金になるため、hook で塞ぐ — #101)
//
// issue / pr verify はプロセス起動ではなく internal/verify の関数呼び出しで行う
// (判定一元化 — #38 と同じ方針)。ブロックの理由は呼び出し側(cli)が stderr に
// 出して exit 2 に変換する(hook 契約: exit 2 = ブロック、その他非ゼロ = 実行失敗)。
package gate

import (
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"

	"github.com/naito-7110/claude-plugins/tools/atelier/internal/verify"
)

// Input は PreToolUse hook が stdin に渡す JSON のうち、ゲート判定に使う部分。
type Input struct {
	ToolName  string `json:"tool_name"`
	ToolInput struct {
		Command string `json:"command"` // Bash
	} `json:"tool_input"`
}

// ParseInput は hook の stdin JSON を読む。
func ParseInput(r io.Reader) (Input, error) {
	var input Input
	if err := json.NewDecoder(r).Decode(&input); err != nil {
		return Input{}, fmt.Errorf("hook 入力(JSON)を解釈できません: %w", err)
	}
	return input, nil
}

// Deps はゲート判定の実行時依存。GraphQL クライアントとリポジトリは
// 必要になるまで解決しない(main 直 push の判定は認証なしでも動く)。
type Deps struct {
	NewClient  func() (verify.GraphQL, error)
	Repo       func() (string, error)
	Branch  func() (string, error) // カレントブランチ(unborn でも名前を返す実装を注入する)
	Managed bool                   // atelier 管理下か(.atelier/ の有無。呼び出し側が解決する)
	Err     io.Writer              // verify 所見の出力先(ブロック理由の判断材料)
}

// --- コマンド検出(bash 版の grep -E と同一のパターン)---
//
// 検出の正規表現こそ壊れやすくテストで固定する価値が高い(#71 で unborn の穴を
// 実測発見済み)。bash の [[:space:]] は Go の \s に対応する。
var (
	gitPushPattern   = regexp.MustCompile(`(^|[;&|\s])git\s[^;&|]*push`)
	forcePushPattern = regexp.MustCompile(`push[^;&|]*\s(-f|--force|--force-with-lease)(\s|$)`)
	mainWordPattern  = regexp.MustCompile(`main|master`)
	directPushMain   = regexp.MustCompile(`push[^;&|]*\s(origin\s+)?(main|master)(\s|$|:)`)

	ghPrMergePattern  = regexp.MustCompile(`gh\s+pr\s+merge`)
	ghAPIMergePattern = regexp.MustCompile(`gh\s+api[^;&|]*/pulls?/[0-9]+/merge`)
	mergeNumberSource = regexp.MustCompile(`pr\s+merge\s+#?[0-9]+|/pulls?/[0-9]+/merge`)
	digitsPattern     = regexp.MustCompile(`[0-9]+`)

	branchIssuePattern = regexp.MustCompile(`^agent/issue-([0-9]+)`)

	// リリースゲート: atelier release の起動(パス付き .agents/bin/atelier も拾う)と、
	// タグを push するコマンド(--tags / refs/tags/ / atelier/v* 引数)。
	atelierReleasePattern = regexp.MustCompile(`(^|[;&|\s/])atelier\s+release(\s[^;&|]*|)($|[;&|])`)
	tagPushPattern        = regexp.MustCompile(`push[^;&|]*(\s--tags(\s|$|[;&|])|refs/tags/|\satelier/v)`)
)

// Check は hook 入力を判定し、ブロックすべきなら理由(非空)を返す。
// error は判定不能な実行失敗(hook 契約では exit 2 に変換しない)。
//
// atelier 管理外(.atelier/ が無い)のリポジトリでは全ツールを許可する。
// プラグインはユーザーレベルで有効化され hook は全リポジトリで発火するため、
// スコープを切らないと無関係なリポジトリまでゲートされる(#103 の実地バグ)。
func Check(input Input, deps Deps) (string, error) {
	if !deps.Managed {
		return "", nil
	}
	switch input.ToolName {
	case "Bash":
		return checkBash(input, deps), nil
	default:
		return "", nil
	}
}

// --- Bash: push ゲート・マージゲート・リリースゲート ---

func checkBash(input Input, deps Deps) string {
	cmd := input.ToolInput.Command
	if cmd == "" {
		return ""
	}

	// 4. リリースゲート(常時): デプロイの引き金は人間の操作。
	if reason := checkRelease(cmd); reason != "" {
		return reason
	}

	// 1. main への直 push / force push + 2. push ゲート
	if gitPushPattern.MatchString(cmd) {
		if forcePushPattern.MatchString(cmd) && mainWordPattern.MatchString(cmd) {
			return "main への force push は禁止です(git-workflow)"
		}
		if directPushMain.MatchString(cmd) {
			return "main への直 push は禁止です(PR を経由してください — git-workflow)"
		}
		if reason := checkPushBranch(deps); reason != "" {
			return reason
		}
	}

	// 3. マージゲート
	if ghPrMergePattern.MatchString(cmd) || ghAPIMergePattern.MatchString(cmd) {
		if reason := checkMerge(cmd, deps); reason != "" {
			return reason
		}
	}
	return ""
}

// checkRelease はリリースゲート(merge-policy: デプロイ = 人間の tag push)。
//   - atelier release の実行をブロックする。ただし --dry-run を含む起動は許可
//     (検証は無害で、AI がリリース状態を確認する用途は正当)
//   - タグを push するコマンド(--tags / refs/tags/ / atelier/v* 引数)をブロックする
func checkRelease(cmd string) string {
	for _, match := range atelierReleasePattern.FindAllStringSubmatch(cmd, -1) {
		if !strings.Contains(match[2], "--dry-run") {
			return "リリースタグは人間の操作です(merge-policy: デプロイ = 人間の tag push)。--dry-run での確認は可能です"
		}
	}
	if gitPushPattern.MatchString(cmd) && tagPushPattern.MatchString(cmd) {
		return "タグの push は人間の操作です(merge-policy: デプロイ = 人間の tag push。リリースは人間が atelier release を実行してください)"
	}
	return ""
}

// checkPushBranch は push 時のブランチゲート。ブランチ名の解決は
// symbolic-ref 相当(コミットゼロの unborn ブランチでも名前を返す)を注入する。
// 解決できない場合は bash 版と同じく素通し(ブランチ不明はゲート対象外)。
func checkPushBranch(deps Deps) string {
	branch, err := deps.Branch()
	if err != nil {
		return ""
	}
	if branch == "main" || branch == "master" {
		return "main ブランチからの push は禁止です(作業は agent/issue-<n>-<slug> ブランチで)"
	}
	match := branchIssuePattern.FindStringSubmatch(branch)
	if match == nil {
		return ""
	}
	number, _ := strconv.Atoi(match[1])
	if !issueVerifyOK(deps, number) {
		return fmt.Sprintf("issue #%d の状態が push 条件を満たしません(ラベルなしの実装は push できません)", number)
	}
	return ""
}

// checkMerge はマージゲート。PR 番号をコマンドから抽出し(無ければカレント
// ブランチの PR)、pr verify → Closes 紐づけ → merge:agent → CI green →
// atelier-review green の順に確認する(fail-closed: 確認できないものはブロック)。
func checkMerge(cmd string, deps Deps) string {
	client, repo, err := resolveClient(deps)
	if err != nil {
		return fmt.Sprintf("マージゲートを実行できないため停止します(%v)", err)
	}

	number := parseMergeNumber(cmd)
	if number == 0 {
		number = currentPRNumber(client, repo, deps)
	}
	if number == 0 {
		return "マージ対象の PR 番号を特定できません"
	}

	report, err := verify.PR(client, repo, number, verify.AllChecks, nil)
	if err != nil || !report.OK() {
		printPRFindings(deps.Err, report)
		if err != nil {
			fmt.Fprintln(deps.Err, err)
		}
		return fmt.Sprintf("PR #%d は PR↔issue 整合を満たしません(上記の理由)", number)
	}
	printPRFindings(deps.Err, report)

	status, err := fetchMergeStatus(client, repo, number)
	if err != nil {
		return fmt.Sprintf("マージゲートを実行できないため停止します(%v)", err)
	}
	if status.LinkedIssue == 0 {
		return fmt.Sprintf("PR #%d に Closes での issue 紐づけがありません(agent マージは不可)", number)
	}
	if !hasMergeAgentLabel(client, repo, status.LinkedIssue) {
		return fmt.Sprintf("issue #%d に merge:agent がありません。人間のレビュー・マージを待ってください(merge-policy)", status.LinkedIssue)
	}
	if status.ChecksState != "SUCCESS" {
		return fmt.Sprintf("PR #%d の CI が green ではありません(merge-policy の実行条件)", number)
	}
	if status.ReviewState != "SUCCESS" {
		return fmt.Sprintf("PR #%d は別コンテキストレビュア(atelier-review)の green がありません(merge-policy の実行条件)", number)
	}
	return ""
}

// parseMergeNumber は「gh pr merge 64」「gh api .../pulls/64/merge」から
// PR 番号を取り出す(bash 版と同じく最初の数字)。
func parseMergeNumber(cmd string) int {
	match := mergeNumberSource.FindString(cmd)
	if match == "" {
		return 0
	}
	number, _ := strconv.Atoi(digitsPattern.FindString(match))
	return number
}

func resolveClient(deps Deps) (verify.GraphQL, string, error) {
	repo, err := deps.Repo()
	if err != nil {
		return nil, "", fmt.Errorf("カレントリポジトリを解決できません: %w", err)
	}
	client, err := deps.NewClient()
	if err != nil {
		return nil, "", err
	}
	return client, repo, nil
}

func issueVerifyOK(deps Deps, number int) bool {
	client, repo, err := resolveClient(deps)
	if err != nil {
		fmt.Fprintln(deps.Err, err)
		return false
	}
	report, err := verify.Issue(client, repo, number, verify.AllChecks)
	if err != nil {
		fmt.Fprintln(deps.Err, err)
		return false
	}
	printFindings(deps.Err, report.Findings)
	return report.OK()
}

func printFindings(w io.Writer, findings []verify.Finding) {
	for _, finding := range findings {
		fmt.Fprintf(w, "%s: %s\n", finding.Level, finding.Message)
	}
}

func printPRFindings(w io.Writer, report verify.PRReport) {
	printFindings(w, report.Findings)
	for _, issue := range report.Issues {
		fmt.Fprintf(w, "==> 関連 issue #%d の検証\n", issue.Number)
		printFindings(w, issue.Findings)
	}
}

// --- マージゲートの GraphQL クエリ ---

// mergeStatusQuery は Closes 紐づけ・CI rollup・atelier-review status を 1 回で取る。
// statusCheckRollup が SUCCESS 以外(FAILURE / PENDING / 無し)は bash 版の
// `gh pr checks` 非ゼロと同じくブロック対象。
const mergeStatusQuery = `query($owner: String!, $name: String!, $number: Int!) {
  repository(owner: $owner, name: $name) {
    pullRequest(number: $number) {
      closingIssuesReferences(first: 1) { nodes { number } }
      commits(last: 1) {
        nodes {
          commit {
            statusCheckRollup { state }
            status { context(name: "atelier-review") { state } }
          }
        }
      }
    }
  }
}`

const prByBranchQuery = `query($owner: String!, $name: String!, $branch: String!) {
  repository(owner: $owner, name: $name) {
    pullRequests(first: 1, states: OPEN, headRefName: $branch) { nodes { number } }
  }
}`

const issueLabelsQuery = `query($owner: String!, $name: String!, $number: Int!) {
  repository(owner: $owner, name: $name) {
    issue(number: $number) {
      labels(first: 100) { nodes { name } }
    }
  }
}`

type mergeStatus struct {
	LinkedIssue int
	ChecksState string // statusCheckRollup.state("" = check なし)
	ReviewState string // atelier-review context の state("" = status なし)
}

func splitRepo(repo string) (string, string, error) {
	owner, name, ok := strings.Cut(repo, "/")
	if !ok || owner == "" || name == "" {
		return "", "", fmt.Errorf("リポジトリは owner/name 形式で指定してください: %s", repo)
	}
	return owner, name, nil
}

func fetchMergeStatus(client verify.GraphQL, repo string, number int) (mergeStatus, error) {
	owner, name, err := splitRepo(repo)
	if err != nil {
		return mergeStatus{}, err
	}
	var resp struct {
		Repository *struct {
			PullRequest *struct {
				ClosingIssuesReferences struct {
					Nodes []struct {
						Number int `json:"number"`
					} `json:"nodes"`
				} `json:"closingIssuesReferences"`
				Commits struct {
					Nodes []struct {
						Commit struct {
							StatusCheckRollup *struct {
								State string `json:"state"`
							} `json:"statusCheckRollup"`
							Status *struct {
								Context *struct {
									State string `json:"state"`
								} `json:"context"`
							} `json:"status"`
						} `json:"commit"`
					} `json:"nodes"`
				} `json:"commits"`
			} `json:"pullRequest"`
		} `json:"repository"`
	}
	vars := map[string]interface{}{"owner": owner, "name": name, "number": number}
	if err := client.Do(mergeStatusQuery, vars, &resp); err != nil {
		return mergeStatus{}, fmt.Errorf("PR #%d の状態を取得できません: %w", number, err)
	}
	if resp.Repository == nil || resp.Repository.PullRequest == nil {
		return mergeStatus{}, fmt.Errorf("PR #%d が見つかりません", number)
	}
	pr := resp.Repository.PullRequest
	status := mergeStatus{}
	if len(pr.ClosingIssuesReferences.Nodes) > 0 {
		status.LinkedIssue = pr.ClosingIssuesReferences.Nodes[0].Number
	}
	if len(pr.Commits.Nodes) > 0 {
		commit := pr.Commits.Nodes[0].Commit
		if commit.StatusCheckRollup != nil {
			status.ChecksState = commit.StatusCheckRollup.State
		}
		if commit.Status != nil && commit.Status.Context != nil {
			status.ReviewState = commit.Status.Context.State
		}
	}
	return status, nil
}

// currentPRNumber はカレントブランチの OPEN な PR 番号を返す(bash 版の
// `gh pr view` フォールバックに対応)。解決できなければ 0。
func currentPRNumber(client verify.GraphQL, repo string, deps Deps) int {
	branch, err := deps.Branch()
	if err != nil || branch == "" {
		return 0
	}
	owner, name, err := splitRepo(repo)
	if err != nil {
		return 0
	}
	var resp struct {
		Repository *struct {
			PullRequests struct {
				Nodes []struct {
					Number int `json:"number"`
				} `json:"nodes"`
			} `json:"pullRequests"`
		} `json:"repository"`
	}
	vars := map[string]interface{}{"owner": owner, "name": name, "branch": branch}
	if err := client.Do(prByBranchQuery, vars, &resp); err != nil {
		return 0
	}
	if resp.Repository == nil || len(resp.Repository.PullRequests.Nodes) == 0 {
		return 0
	}
	return resp.Repository.PullRequests.Nodes[0].Number
}

func hasMergeAgentLabel(client verify.GraphQL, repo string, number int) bool {
	owner, name, err := splitRepo(repo)
	if err != nil {
		return false
	}
	var resp struct {
		Repository *struct {
			Issue *struct {
				Labels struct {
					Nodes []struct {
						Name string `json:"name"`
					} `json:"nodes"`
				} `json:"labels"`
			} `json:"issue"`
		} `json:"repository"`
	}
	vars := map[string]interface{}{"owner": owner, "name": name, "number": number}
	if err := client.Do(issueLabelsQuery, vars, &resp); err != nil {
		return false
	}
	if resp.Repository == nil || resp.Repository.Issue == nil {
		return false
	}
	for _, node := range resp.Repository.Issue.Labels.Nodes {
		if node.Name == "merge:agent" {
			return true
		}
	}
	return false
}
