package cli_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/naito-7110/claude-plugins/tools/factory/internal/cli"
	"github.com/naito-7110/claude-plugins/tools/factory/internal/ghfake"
)

// branch cleanup のテスト。git 操作は temp ディレクトリの実 git
// (fake では worktree・ref の挙動の再現に意味が無い)、PR 状態は ghfake。

func mustGit(t *testing.T, root string, args ...string) string {
	t.Helper()
	out, err := exec.Command("git", append([]string{"-C", root}, args...)...).CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v (%s)", strings.Join(args, " "), err, out)
	}
	return string(out)
}

// initGitRepo は main ブランチ 1 コミットの実 git リポジトリを作る。
func initGitRepo(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git がありません")
	}
	root := t.TempDir()
	mustGit(t, root, "init", "-q", "-b", "main")
	mustGit(t, root, "config", "user.email", "test@example.com")
	mustGit(t, root, "config", "user.name", "test")
	writeFile(t, root, "README.md", "# test\n")
	mustGit(t, root, "add", ".")
	mustGit(t, root, "commit", "-q", "-m", "initial")
	return root
}

func localBranches(t *testing.T, root string) string {
	t.Helper()
	return mustGit(t, root, "for-each-ref", "--format=%(refname:short)", "refs/heads/")
}

func executeCleanup(t *testing.T, server *ghfake.Server, root string, extra ...string) run {
	t.Helper()
	args := append([]string{"branch", "cleanup", "--root", root, "--repo", testRepo}, extra...)
	return execute(t, server, testRepo, args...)
}

// --- 受け入れ条件 1: merged が消え、open・現在・パターン外・PR なしが残る ---

func TestBranchCleanupDeletesMergedKeepsOthers(t *testing.T) {
	root := initGitRepo(t)
	mustGit(t, root, "branch", "agent/issue-1-merged")
	mustGit(t, root, "branch", "agent/issue-2-open")
	mustGit(t, root, "branch", "agent/issue-3-nopr")
	mustGit(t, root, "branch", "feature/keep")

	server := testServer()
	server.AddPullRequest(testRepo, &ghfake.PullRequest{
		Number: 11, HeadRefName: "agent/issue-1-merged", State: "MERGED"})
	server.AddPullRequest(testRepo, &ghfake.PullRequest{
		Number: 12, HeadRefName: "agent/issue-2-open", State: "OPEN"})

	result := executeCleanup(t, server, root)
	if result.code != cli.ExitOK {
		t.Fatalf("code = %d (out=%q err=%q)", result.code, result.out, result.err)
	}

	branches := localBranches(t, root)
	if strings.Contains(branches, "agent/issue-1-merged") {
		t.Errorf("merged ブランチが残っている: %q", branches)
	}
	for _, keep := range []string{"agent/issue-2-open", "agent/issue-3-nopr", "feature/keep", "main"} {
		if !strings.Contains(branches, keep) {
			t.Errorf("%s が消えている: %q", keep, branches)
		}
	}
	for _, want := range []string{
		"削除: agent/issue-1-merged(PR #11 merged)",
		"スキップ: agent/issue-2-open(PR #12 が open)",
		"スキップ: agent/issue-3-nopr(PR が見つかりません",
	} {
		if !strings.Contains(result.out, want) {
			t.Errorf("出力に %q がない: %q", want, result.out)
		}
	}
	// パターン外は対象にすら上がらない。
	if strings.Contains(result.out, "feature/keep") {
		t.Errorf("パターン外のブランチに触れている: %q", result.out)
	}
}

func TestBranchCleanupSkipsCurrentBranch(t *testing.T) {
	root := initGitRepo(t)
	mustGit(t, root, "checkout", "-q", "-b", "agent/issue-4-current")
	server := testServer()
	server.AddPullRequest(testRepo, &ghfake.PullRequest{
		Number: 14, HeadRefName: "agent/issue-4-current", State: "MERGED"})

	result := executeCleanup(t, server, root)
	if result.code != cli.ExitOK {
		t.Fatalf("code = %d (err=%q)", result.code, result.err)
	}
	if !strings.Contains(result.out, "スキップ: agent/issue-4-current(現在チェックアウト中)") {
		t.Errorf("現在ブランチのスキップが出力されない: %q", result.out)
	}
	if !strings.Contains(localBranches(t, root), "agent/issue-4-current") {
		t.Error("現在ブランチが消えている")
	}
}

func TestBranchCleanupDeletesClosedPRBranch(t *testing.T) {
	root := initGitRepo(t)
	mustGit(t, root, "branch", "agent/issue-5-closed")
	server := testServer()
	server.AddPullRequest(testRepo, &ghfake.PullRequest{
		Number: 15, HeadRefName: "agent/issue-5-closed", State: "CLOSED"})

	result := executeCleanup(t, server, root)
	if result.code != cli.ExitOK {
		t.Fatalf("code = %d (err=%q)", result.code, result.err)
	}
	if !strings.Contains(result.out, "削除: agent/issue-5-closed(PR #15 closed)") {
		t.Errorf("closed の削除が出力されない: %q", result.out)
	}
	if strings.Contains(localBranches(t, root), "agent/issue-5-closed") {
		t.Error("closed PR のブランチが残っている")
	}
}

// --- worktree の扱い ---

func TestBranchCleanupRemovesCleanWorktree(t *testing.T) {
	root := initGitRepo(t)
	mustGit(t, root, "branch", "agent/issue-6-wt")
	mustGit(t, root, "worktree", "add", "-q", ".worktrees/issue-6", "agent/issue-6-wt")
	server := testServer()
	server.AddPullRequest(testRepo, &ghfake.PullRequest{
		Number: 16, HeadRefName: "agent/issue-6-wt", State: "MERGED"})

	result := executeCleanup(t, server, root)
	if result.code != cli.ExitOK {
		t.Fatalf("code = %d (out=%q err=%q)", result.code, result.out, result.err)
	}
	if !strings.Contains(result.out, "+ worktree ") {
		t.Errorf("worktree の除去が出力されない: %q", result.out)
	}
	if strings.Contains(localBranches(t, root), "agent/issue-6-wt") {
		t.Error("ブランチが残っている")
	}
	if _, err := os.Stat(filepath.Join(root, ".worktrees", "issue-6")); !os.IsNotExist(err) {
		t.Error("worktree ディレクトリが残っている")
	}
}

// --- 受け入れ条件 2: 未コミット変更のある worktree はスキップ + 警告 ---

func TestBranchCleanupSkipsDirtyWorktree(t *testing.T) {
	root := initGitRepo(t)
	mustGit(t, root, "branch", "agent/issue-7-dirty")
	mustGit(t, root, "worktree", "add", "-q", ".worktrees/issue-7", "agent/issue-7-dirty")
	writeFile(t, root, ".worktrees/issue-7/wip.txt", "未コミットの作業\n")
	server := testServer()
	server.AddPullRequest(testRepo, &ghfake.PullRequest{
		Number: 17, HeadRefName: "agent/issue-7-dirty", State: "MERGED"})

	result := executeCleanup(t, server, root)
	if result.code != cli.ExitOK {
		t.Fatalf("code = %d(警告は exit に影響しない)(err=%q)", result.code, result.err)
	}
	if !strings.Contains(result.out, "警告") || !strings.Contains(result.out, "未コミット変更があります") {
		t.Errorf("警告が出力されない: %q", result.out)
	}
	if !strings.Contains(localBranches(t, root), "agent/issue-7-dirty") {
		t.Error("dirty worktree のブランチが削除されている(fail-closed 違反)")
	}
	if _, err := os.Stat(filepath.Join(root, ".worktrees", "issue-7", "wip.txt")); err != nil {
		t.Error("未コミットの作業が消えている")
	}
}

// --- 受け入れ条件 3: --dry-run は変更ゼロで一覧を出す ---

func TestBranchCleanupDryRunChangesNothing(t *testing.T) {
	root := initGitRepo(t)
	mustGit(t, root, "branch", "agent/issue-8-merged")
	mustGit(t, root, "worktree", "add", "-q", ".worktrees/issue-8", "agent/issue-8-merged")
	// 追跡ブランチの prune 対象も用意する(bare リモートに push 後、リモート側だけ削除)。
	bare := t.TempDir()
	mustGit(t, bare, "init", "-q", "--bare")
	mustGit(t, root, "remote", "add", "origin", bare)
	mustGit(t, root, "push", "-q", "origin", "main", "agent/issue-8-merged")
	mustGit(t, bare, "branch", "-D", "agent/issue-8-merged")

	server := testServer()
	server.AddPullRequest(testRepo, &ghfake.PullRequest{
		Number: 18, HeadRefName: "agent/issue-8-merged", State: "MERGED"})

	result := executeCleanup(t, server, root, "--dry-run")
	if result.code != cli.ExitOK {
		t.Fatalf("code = %d (out=%q err=%q)", result.code, result.out, result.err)
	}
	for _, want := range []string{
		"dry-run: 変更しません",
		"削除対象: agent/issue-8-merged(PR #18 merged) + worktree ",
		"origin/agent/issue-8-merged(dry-run)",
	} {
		if !strings.Contains(result.out, want) {
			t.Errorf("出力に %q がない: %q", want, result.out)
		}
	}
	// 変更ゼロの検証: ブランチ・worktree・追跡ブランチのすべてが残る。
	if !strings.Contains(localBranches(t, root), "agent/issue-8-merged") {
		t.Error("dry-run でブランチが消えている")
	}
	if _, err := os.Stat(filepath.Join(root, ".worktrees", "issue-8")); err != nil {
		t.Error("dry-run で worktree が消えている")
	}
	remote := mustGit(t, root, "for-each-ref", "--format=%(refname:short)", "refs/remotes/")
	if !strings.Contains(remote, "origin/agent/issue-8-merged") {
		t.Errorf("dry-run で追跡ブランチが消えている: %q", remote)
	}
}

// --- remote prune ---

func TestBranchCleanupPrunesRemoteTrackingRefs(t *testing.T) {
	root := initGitRepo(t)
	bare := t.TempDir()
	mustGit(t, bare, "init", "-q", "--bare")
	mustGit(t, root, "remote", "add", "origin", bare)
	mustGit(t, root, "branch", "agent/issue-9-merged")
	mustGit(t, root, "push", "-q", "origin", "main", "agent/issue-9-merged")
	mustGit(t, bare, "branch", "-D", "agent/issue-9-merged") // リモート側は削除済み(GitHub の自動削除相当)

	server := testServer()
	server.AddPullRequest(testRepo, &ghfake.PullRequest{
		Number: 19, HeadRefName: "agent/issue-9-merged", State: "MERGED"})

	result := executeCleanup(t, server, root)
	if result.code != cli.ExitOK {
		t.Fatalf("code = %d (err=%q)", result.code, result.err)
	}
	if !strings.Contains(result.out, "remote prune: origin/agent/issue-9-merged") {
		t.Errorf("prune の報告がない: %q", result.out)
	}
	remote := mustGit(t, root, "for-each-ref", "--format=%(refname:short)", "refs/remotes/")
	if strings.Contains(remote, "origin/agent/issue-9-merged") {
		t.Errorf("追跡ブランチが残っている: %q", remote)
	}
}

func TestBranchCleanupWithoutOriginSkipsPrune(t *testing.T) {
	root := initGitRepo(t)

	result := executeCleanup(t, testServer(), root)
	if result.code != cli.ExitOK {
		t.Fatalf("code = %d (err=%q)", result.code, result.err)
	}
	for _, want := range []string{"対象ブランチなし(agent/issue-*)", "origin リモートなし"} {
		if !strings.Contains(result.out, want) {
			t.Errorf("出力に %q がない: %q", want, result.out)
		}
	}
}
