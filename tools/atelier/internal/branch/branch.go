// Package branch はマージ済み agent ブランチの掃除を提供する。
//
// squash マージ運用(git-workflow)では `git branch --merged` がマージ済みを
// 検出できない(squash はコミット履歴上つながらない)ため、**PR の状態を正**
// として判定する: 対象ブランチに紐づく PR が merged / closed のものだけを
// `git branch -D` で削除する。
//
// fail-closed の原則:
//   - 対象は BranchGlob(agent/issue-*)のローカルブランチのみ。それ以外に触れない
//   - 現在チェックアウト中のブランチ・open PR のブランチ・PR が見つからない
//     ブランチはスキップ(未マージの作業を壊さない)
//   - ブランチに紐づく worktree に未コミット変更があればブランチごとスキップして警告
//   - PR 状態の取得に失敗したブランチは削除しない(NG として報告)
//
// リモートブランチの削除は GitHub の「Automatically delete head branches」に
// 委ね、ここでは `git remote prune origin` で追跡ブランチだけを掃除する。
//
// git はプロセス境界だが、挙動(worktree・ref・status)の再現に意味のある fake を
// 作れないため、テストは temp ディレクトリの実 git リポジトリで行う(tdd-doctrine:
// 意味を保てない代替で緑を作らない)。PR 状態の照会は GraphQL interface で
// 抽象化し、テストでは ghfake を注入する。
package branch

import (
	"fmt"
	"os/exec"
	"sort"
	"strings"

	"github.com/naito-7110/claude-plugins/tools/atelier/internal/verify"
)

// GraphQL は GitHub GraphQL API へのプロセス境界。
type GraphQL interface {
	Do(query string, variables map[string]interface{}, response interface{}) error
}

// BranchGlob は掃除対象のブランチパターン(単一定義)。これ以外に触れない。
const BranchGlob = "agent/issue-*"

// CheckCleanup は所見の検査項目名。
const CheckCleanup = "branch-cleanup"

// Report はブランチ掃除の結果。
type Report struct {
	Root     string
	Findings []verify.Finding
}

func (r *Report) add(level verify.Level, format string, args ...interface{}) {
	r.Findings = append(r.Findings, verify.Finding{
		Check: CheckCleanup, Level: level, Message: fmt.Sprintf(format, args...),
	})
}

// NGCount は NG の所見数を返す(警告は数えない)。
func (r Report) NGCount() int {
	count := 0
	for _, f := range r.Findings {
		if f.Level == verify.LevelNG {
			count++
		}
	}
	return count
}

// OK は NG の所見がないとき true。
func (r Report) OK() bool {
	return r.NGCount() == 0
}

// prStateQuery は headRefName に紐づく PR(全状態)を返す。
// 同一ブランチ名の再利用を考慮して first: 20 まで見る。
const prStateQuery = `query($owner: String!, $name: String!, $branch: String!) {
  repository(owner: $owner, name: $name) {
    pullRequests(headRefName: $branch, first: 20) {
      nodes { number state }
    }
  }
}`

type prRef struct {
	Number int    `json:"number"`
	State  string `json:"state"` // OPEN | MERGED | CLOSED
}

type prStateResponse struct {
	Repository *struct {
		PullRequests struct {
			Nodes []prRef `json:"nodes"`
		} `json:"pullRequests"`
	} `json:"repository"`
}

// Cleanup は root のローカルリポジトリからマージ済み agent ブランチを掃除する。
// dryRun のときは一切変更せず、削除対象の一覧だけを所見にする。
// error は前提の失敗(root が git リポジトリでない等)のみ。個々のブランチの
// 失敗は NG 所見にして続行する。
func Cleanup(client GraphQL, repo, root string, dryRun bool) (Report, error) {
	owner, name, ok := strings.Cut(repo, "/")
	if !ok || owner == "" || name == "" {
		return Report{}, fmt.Errorf("リポジトリは owner/name 形式で指定してください: %s", repo)
	}
	report := Report{Root: root}

	branches, err := listBranches(root)
	if err != nil {
		return Report{}, err
	}
	current, err := currentBranch(root)
	if err != nil {
		return Report{}, err
	}
	worktrees, err := worktreesByBranch(root)
	if err != nil {
		return Report{}, err
	}

	if len(branches) == 0 {
		report.add(verify.LevelInfo, "対象ブランチなし(%s)", BranchGlob)
	}
	for _, br := range branches {
		cleanupBranch(client, &report, owner, name, root, br, current, worktrees, dryRun)
	}

	pruneRemote(&report, root, dryRun)
	return report, nil
}

func cleanupBranch(client GraphQL, report *Report, owner, name, root, br, current string,
	worktrees map[string]string, dryRun bool) {
	if br == current {
		report.add(verify.LevelInfo, "スキップ: %s(現在チェックアウト中)", br)
		return
	}

	var resp prStateResponse
	err := client.Do(prStateQuery, map[string]interface{}{
		"owner": owner, "name": name, "branch": br,
	}, &resp)
	if err != nil || resp.Repository == nil {
		// fail-closed: PR 状態が確認できないブランチは削除しない。
		report.add(verify.LevelNG, "%s の PR 状態を確認できません(削除しません): %v", br, err)
		return
	}
	prs := resp.Repository.PullRequests.Nodes
	if len(prs) == 0 {
		report.add(verify.LevelInfo, "スキップ: %s(PR が見つかりません — 未マージの可能性があるため触りません)", br)
		return
	}
	var done *prRef
	for i, pr := range prs {
		if pr.State == "OPEN" {
			report.add(verify.LevelInfo, "スキップ: %s(PR #%d が open)", br, pr.Number)
			return
		}
		// merged を優先して表示(closed しか無ければ closed)。
		if done == nil || pr.State == "MERGED" {
			done = &prs[i]
		}
	}
	label := fmt.Sprintf("PR #%d %s", done.Number, strings.ToLower(done.State))

	// 紐づく worktree の処理。未コミット変更があればブランチごとスキップ(fail-closed)。
	suffix := ""
	if wt, ok := worktrees[br]; ok {
		dirty, err := isDirty(wt)
		if err != nil {
			report.add(verify.LevelNG, "%s の worktree %s の状態を確認できません(削除しません): %v", br, wt, err)
			return
		}
		if dirty {
			report.add(verify.LevelWarn,
				"スキップ: %s(worktree %s に未コミット変更があります。手で確認してから削除してください)", br, wt)
			return
		}
		suffix = fmt.Sprintf(" + worktree %s", wt)
		if !dryRun {
			if _, err := git(root, "worktree", "remove", wt); err != nil {
				report.add(verify.LevelNG, "%s の worktree %s を除去できません: %v", br, wt, err)
				return
			}
		}
	}

	if dryRun {
		report.add(verify.LevelOK, "削除対象: %s(%s)%s", br, label, suffix)
		return
	}
	if _, err := git(root, "branch", "-D", br); err != nil {
		report.add(verify.LevelNG, "%s を削除できません: %v", br, err)
		return
	}
	report.add(verify.LevelOK, "削除: %s(%s)%s", br, label, suffix)
}

// pruneRemote は origin の追跡ブランチを掃除する。origin が無ければスキップ。
func pruneRemote(report *Report, root string, dryRun bool) {
	remotes, err := git(root, "remote")
	if err != nil {
		report.add(verify.LevelNG, "リモートを列挙できません: %v", err)
		return
	}
	hasOrigin := false
	for _, remote := range strings.Fields(remotes) {
		if remote == "origin" {
			hasOrigin = true
		}
	}
	if !hasOrigin {
		report.add(verify.LevelInfo, "origin リモートなし(remote prune はスキップ)")
		return
	}

	args := []string{"remote", "prune", "origin"}
	if dryRun {
		args = []string{"remote", "prune", "-n", "origin"}
	}
	out, err := git(root, args...)
	if err != nil {
		report.add(verify.LevelNG, "git remote prune origin に失敗しました: %v", err)
		return
	}
	if pruned := prunedRefs(out); len(pruned) > 0 {
		report.add(verify.LevelOK, "remote prune: %s", strings.Join(pruned, ", "))
	} else {
		report.add(verify.LevelInfo, "remote prune: 対象なし")
	}
}

// prunedRefs は git remote prune の出力から掃除された(される)ref 名を抜き出す。
func prunedRefs(out string) []string {
	var refs []string
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if rest, found := strings.CutPrefix(line, "* [would prune]"); found {
			refs = append(refs, strings.TrimSpace(rest)+"(dry-run)")
		} else if rest, found := strings.CutPrefix(line, "* [pruned]"); found {
			refs = append(refs, strings.TrimSpace(rest))
		}
	}
	return refs
}

// --- git ヘルパー ---

func git(root string, args ...string) (string, error) {
	out, err := exec.Command("git", append([]string{"-C", root}, args...)...).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git %s: %v(%s)", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return string(out), nil
}

// listBranches は BranchGlob に一致するローカルブランチを名前順で返す。
func listBranches(root string) ([]string, error) {
	out, err := git(root, "for-each-ref", "--format=%(refname:short)", "refs/heads/"+BranchGlob)
	if err != nil {
		return nil, err
	}
	var branches []string
	for _, line := range strings.Split(out, "\n") {
		if line = strings.TrimSpace(line); line != "" {
			branches = append(branches, line)
		}
	}
	sort.Strings(branches)
	return branches, nil
}

func currentBranch(root string) (string, error) {
	out, err := git(root, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

// worktreesByBranch はブランチ名 → worktree パスの対応を返す
// (git worktree list --porcelain の branch 行から構築する)。
func worktreesByBranch(root string) (map[string]string, error) {
	out, err := git(root, "worktree", "list", "--porcelain")
	if err != nil {
		return nil, err
	}
	result := map[string]string{}
	var path string
	for _, line := range strings.Split(out, "\n") {
		if rest, found := strings.CutPrefix(line, "worktree "); found {
			path = rest
		} else if rest, found := strings.CutPrefix(line, "branch refs/heads/"); found && path != "" {
			result[rest] = path
		}
	}
	return result, nil
}

func isDirty(worktree string) (bool, error) {
	out, err := git(worktree, "status", "--porcelain")
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(out) != "", nil
}
