package cli_test

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/naito-7110/claude-plugins/tools/atelier/internal/board"
	"github.com/naito-7110/claude-plugins/tools/atelier/internal/cli"
	"github.com/naito-7110/claude-plugins/tools/atelier/internal/ghfake"
)

// executeGate は atelier gate を hook JSON つきで実行する。
// branch が空文字ならブランチ解決の失敗(unborn かつ symbolic-ref も不能)を再現する。
func executeGate(t *testing.T, server *ghfake.Server, branch, root, stdin string) run {
	t.Helper()
	var out, errOut strings.Builder
	deps := cli.Deps{
		NewClient:   func() (board.GraphQL, error) { return server, nil },
		CurrentRepo: func() (string, error) { return testRepo, nil },
		CurrentBranch: func() (string, error) {
			if branch == "" {
				return "", errors.New("ブランチを解決できません")
			}
			return branch, nil
		},
		In:  strings.NewReader(stdin),
		Out: &out,
		Err: &errOut,
	}
	code := cli.Run([]string{"gate", "--root", root}, deps)
	return run{code: code, out: out.String(), err: errOut.String()}
}

// hookJSON は PreToolUse の stdin JSON を組み立てる。
func hookJSON(t *testing.T, tool string, toolInput map[string]interface{}) string {
	t.Helper()
	raw, err := json.Marshal(map[string]interface{}{
		"tool_name": tool, "tool_input": toolInput,
	})
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func bashJSON(t *testing.T, cmd string) string {
	t.Helper()
	return hookJSON(t, "Bash", map[string]interface{}{"command": cmd})
}

// managedRoot は atelier 管理下の root(.atelier/ あり)を作る。
// ゲートは .atelier/ の存在で管理下と判定するため(#103)、
// ゲート規則のテストはすべてこの root を使う。
func managedRoot(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".atelier"), 0o755); err != nil {
		t.Fatal(err)
	}
	return root
}

// mergeReadyPR は全マージ条件(pr verify green・Closes 紐づけ・merge:agent・
// CI green・atelier-review green)を満たす PR #64 を登録する。
func mergeReadyPR(server *ghfake.Server) *ghfake.PullRequest {
	issue := readyIssue(server)
	issue.Labels = append(issue.Labels, "merge:agent")
	issue.LabelEvents = []ghfake.LabelEvent{
		{Label: "merge:agent", CreatedAt: "2026-07-01T00:00:00Z"},
	}
	return server.AddPullRequest(testRepo, &ghfake.PullRequest{
		Number:        64,
		Body:          "## 概要\n\nCloses #38\n",
		HeadRefName:   "agent/issue-38-gate",
		ClosingIssues: []int{38},
		ChecksState:   "SUCCESS",
		ReviewState:   "SUCCESS",
	})
}

func assertBlocked(t *testing.T, result run, reason string) {
	t.Helper()
	if result.code != cli.ExitBlock {
		t.Fatalf("code = %d, want %d(ブロック)(err=%q)", result.code, cli.ExitBlock, result.err)
	}
	if !strings.Contains(result.err, reason) {
		t.Errorf("理由が出力されない: want %q in %q", reason, result.err)
	}
}

func assertAllowed(t *testing.T, result run) {
	t.Helper()
	if result.code != cli.ExitOK {
		t.Fatalf("code = %d, want %d(通過)(err=%q)", result.code, cli.ExitOK, result.err)
	}
}

// --- 0. スコープ: atelier 管理下(.atelier/ の存在)だけをゲートする(#103)---

func TestGateUnmanagedRepoAllowsEverything(t *testing.T) {
	// プラグインはユーザーレベルで有効化され hook は全リポジトリで発火する。
	// .atelier/ の無いリポジトリでは、どのゲートも発動せず全ツールが素通しになる
	// (管理外のリポジトリを fail-closed の人質にしない)。
	unmanaged := t.TempDir() // .atelier/ なし
	for _, stdin := range []string{
		bashJSON(t, "git push origin main"),                    // main 直 push
		bashJSON(t, "git push --force origin main"),            // force push
		bashJSON(t, "gh pr merge 64 --squash"),                 // マージ
		bashJSON(t, "atelier release atelier/v0.3.0"),          // リリース
		bashJSON(t, "git push --tags"),                         // タグ push
		hookJSON(t, "Write", map[string]interface{}{"file_path": "docs/adr/x.md"}),
	} {
		result := executeGate(t, testServer(), "main", unmanaged, stdin)
		assertAllowed(t, result)
	}
}

// --- 1. main への直 push / force push(常時)---

func TestGatePushToMainBlocked(t *testing.T) {
	result := executeGate(t, testServer(), "agent/issue-38-x", managedRoot(t),
		bashJSON(t, "git push origin main"))
	assertBlocked(t, result, "main への直 push は禁止です")
}

func TestGateForcePushToMainBlocked(t *testing.T) {
	for _, cmd := range []string{
		"git push --force origin main",
		"git push -f origin main",
		"git push --force-with-lease origin main",
		"git push origin main --force", // フラグ後置の変形(迂回パターン)
	} {
		result := executeGate(t, testServer(), "agent/issue-38-x", managedRoot(t), bashJSON(t, cmd))
		assertBlocked(t, result, "force push は禁止です")
	}
}

func TestGateNonPushCommandPasses(t *testing.T) {
	result := executeGate(t, testServer(), "main", managedRoot(t), bashJSON(t, "git status"))
	assertAllowed(t, result)
}

// --- 2. push ゲート: 作業ブランチの issue 状態 ---

func TestGatePushFromMainBranchBlocked(t *testing.T) {
	result := executeGate(t, testServer(), "main", managedRoot(t),
		bashJSON(t, "git push origin feature-x"))
	assertBlocked(t, result, "main ブランチからの push は禁止です")
}

func TestGatePushFromReadyIssueBranchPasses(t *testing.T) {
	server := testServer()
	readyIssue(server)
	result := executeGate(t, server, "agent/issue-38-gate", managedRoot(t),
		bashJSON(t, "git push -u origin agent/issue-38-gate"))
	assertAllowed(t, result)
}

func TestGatePushFromNotReadyIssueBranchBlocked(t *testing.T) {
	server := testServer()
	issue := readyIssue(server)
	issue.Labels = nil // agent-ok なし

	result := executeGate(t, server, "agent/issue-38-gate", managedRoot(t),
		bashJSON(t, "git push -u origin agent/issue-38-gate"))
	assertBlocked(t, result, "issue #38 の状態が push 条件を満たしません")
	if !strings.Contains(result.err, "agent-ok ラベルがありません") {
		t.Errorf("verify の所見が出力されない: %q", result.err)
	}
}

// unborn ブランチ(コミットゼロ)でも symbolic-ref 相当がブランチ名を返すので
// push ゲートは適用される(#71 で実測発見された穴の回帰テスト)。
func TestGatePushUnbornIssueBranchStillGated(t *testing.T) {
	server := testServer()
	issue := readyIssue(server)
	issue.Labels = nil

	result := executeGate(t, server, "agent/issue-38-fresh", managedRoot(t),
		bashJSON(t, "git push -u origin agent/issue-38-fresh"))
	assertBlocked(t, result, "issue #38 の状態が push 条件を満たしません")
}

func TestGateBranchUnresolvedPasses(t *testing.T) {
	// ブランチが解決できない場合は bash 版と同じく素通し(ゲート対象外)。
	result := executeGate(t, testServer(), "", managedRoot(t),
		bashJSON(t, "git push origin HEAD"))
	assertAllowed(t, result)
}

func TestGatePushOtherBranchPasses(t *testing.T) {
	result := executeGate(t, testServer(), "feature/x", managedRoot(t),
		bashJSON(t, "git push origin feature/x"))
	assertAllowed(t, result)
}

// --- 5. リリースゲート(デプロイ = 人間の tag push)---

func TestGateAtelierReleaseBlocked(t *testing.T) {
	for _, cmd := range []string{
		"atelier release atelier/v0.3.0",
		".agents/bin/atelier release v1.0.0",
		"cd /repo && atelier release atelier/v0.3.0 --remote origin",
	} {
		result := executeGate(t, testServer(), "agent/issue-38-x", managedRoot(t), bashJSON(t, cmd))
		assertBlocked(t, result, "リリースタグは人間の操作です")
	}
}

func TestGateAtelierReleaseDryRunPasses(t *testing.T) {
	// --dry-run は検証のみで無害(AI がリリース状態を確認する用途は正当)。
	for _, cmd := range []string{
		"atelier release atelier/v0.3.0 --dry-run",
		"atelier release --dry-run atelier/v0.3.0",
	} {
		result := executeGate(t, testServer(), "agent/issue-38-x", managedRoot(t), bashJSON(t, cmd))
		assertAllowed(t, result)
	}
}

func TestGateTagPushBlocked(t *testing.T) {
	for _, cmd := range []string{
		"git push --tags",
		"git push origin --tags",
		"git push origin refs/tags/atelier/v0.3.0",
		"git push origin atelier/v0.3.0",
	} {
		result := executeGate(t, testServer(), "agent/issue-38-x", managedRoot(t), bashJSON(t, cmd))
		assertBlocked(t, result, "タグの push は人間の操作です")
	}
}

func TestGateReleaseGateNoFalsePositives(t *testing.T) {
	// 通常ブランチの push・release を含むだけの無関係なコマンドは誤爆しない。
	server := testServer()
	readyIssue(server)
	for _, cmd := range []string{
		"git push -u origin agent/issue-38-gate", // 従来どおりの push ゲート通過
		"echo atelier released",                  // 単語の部分一致ではブロックしない
		"gh release view atelier/v0.1.0",         // gh release(閲覧)は対象外
	} {
		result := executeGate(t, server, "agent/issue-38-gate", managedRoot(t), bashJSON(t, cmd))
		assertAllowed(t, result)
	}
}

// --- 3. マージゲート ---

func TestGateMergeAllGreenPasses(t *testing.T) {
	server := testServer()
	mergeReadyPR(server)
	result := executeGate(t, server, "agent/issue-38-gate", managedRoot(t),
		bashJSON(t, "gh pr merge 64 --squash"))
	assertAllowed(t, result)
}

// gh api によるマージ(gh pr merge の迂回形)も同じゲートに掛かる。
func TestGateMergeViaAPIGated(t *testing.T) {
	server := testServer()
	pr := mergeReadyPR(server)
	pr.ReviewState = "FAILURE"

	result := executeGate(t, server, "agent/issue-38-gate", managedRoot(t),
		bashJSON(t, `gh api -X PUT "repos/naito-7110/claude-plugins/pulls/64/merge"`))
	assertBlocked(t, result, "別コンテキストレビュア(atelier-review)の green がありません")
}

func TestGateMergePRVerifyNGBlocked(t *testing.T) {
	server := testServer()
	pr := mergeReadyPR(server)
	pr.Body = "## 概要\n\n関連 issue の記載なし\n"

	result := executeGate(t, server, "agent/issue-38-gate", managedRoot(t),
		bashJSON(t, "gh pr merge 64 --squash"))
	assertBlocked(t, result, "PR #64 は PR↔issue 整合を満たしません")
	if !strings.Contains(result.err, "関連 issue が見つかりません") {
		t.Errorf("verify の所見が出力されない: %q", result.err)
	}
}

func TestGateMergeWithoutClosesLinkBlocked(t *testing.T) {
	server := testServer()
	pr := mergeReadyPR(server)
	pr.Body = "## 概要\n\nRefs #38\n" // pr verify は通るが Closes 紐づけがない
	pr.ClosingIssues = nil

	result := executeGate(t, server, "agent/issue-38-gate", managedRoot(t),
		bashJSON(t, "gh pr merge 64 --squash"))
	assertBlocked(t, result, "Closes での issue 紐づけがありません")
}

func TestGateMergeWithoutMergeAgentBlocked(t *testing.T) {
	server := testServer()
	mergeReadyPR(server)
	issue := server.FindIssue(testRepo, 38)
	issue.Labels = []string{"agent-ok"} // merge:agent なし
	issue.LabelEvents = nil

	result := executeGate(t, server, "agent/issue-38-gate", managedRoot(t),
		bashJSON(t, "gh pr merge 64 --squash"))
	assertBlocked(t, result, "issue #38 に merge:agent がありません")
}

func TestGateMergeCINotGreenBlocked(t *testing.T) {
	for _, state := range []string{"PENDING", "FAILURE", ""} { // "" = check なし(fail-closed)
		server := testServer()
		pr := mergeReadyPR(server)
		pr.ChecksState = state

		result := executeGate(t, server, "agent/issue-38-gate", managedRoot(t),
			bashJSON(t, "gh pr merge 64 --squash"))
		assertBlocked(t, result, "CI が green ではありません")
	}
}

func TestGateMergeReviewNotGreenBlocked(t *testing.T) {
	for _, state := range []string{"FAILURE", "PENDING", ""} { // "" = status なし(fail-closed)
		server := testServer()
		pr := mergeReadyPR(server)
		pr.ReviewState = state

		result := executeGate(t, server, "agent/issue-38-gate", managedRoot(t),
			bashJSON(t, "gh pr merge 64 --squash"))
		assertBlocked(t, result, "別コンテキストレビュア(atelier-review)の green がありません")
	}
}

func TestGateMergeNumberFromCurrentBranch(t *testing.T) {
	// 番号なしの gh pr merge はカレントブランチの PR に解決する(gh pr view 相当)。
	server := testServer()
	pr := mergeReadyPR(server)
	pr.ReviewState = "FAILURE" // 解決された PR #64 がゲートされることで解決を実証する

	result := executeGate(t, server, "agent/issue-38-gate", managedRoot(t),
		bashJSON(t, "gh pr merge --squash"))
	assertBlocked(t, result, "PR #64 は別コンテキストレビュア")
}

func TestGateMergeNumberUnresolvedBlocked(t *testing.T) {
	result := executeGate(t, testServer(), "feature/x", managedRoot(t),
		bashJSON(t, "gh pr merge --squash"))
	assertBlocked(t, result, "マージ対象の PR 番号を特定できません")
}

// --- 4. Write / Edit / Task はゲート対象外(無人 3 種は #122 で撤去)---

func TestGateAdrWritePasses(t *testing.T) {
	// 改憲の保護は対話の permission フローと /atelier:adr の手続きに任せる
	// (hook はブロックしない — 人間常駐前提)。
	result := executeGate(t, testServer(), "main", managedRoot(t),
		hookJSON(t, "Edit", map[string]interface{}{"file_path": "docs/adr/0001-x.md"}))
	assertAllowed(t, result)
}

func TestGateTaskPasses(t *testing.T) {
	result := executeGate(t, testServer(), "main", managedRoot(t),
		hookJSON(t, "Task", map[string]interface{}{"prompt": "適当に進めて"}))
	assertAllowed(t, result)
}

func TestGateMergeAgentLabelPasses(t *testing.T) {
	result := executeGate(t, testServer(), "main", managedRoot(t),
		bashJSON(t, "gh issue edit 38 --add-label merge:agent"))
	assertAllowed(t, result)
}

// --- 入力の境界 ---

func TestGateOtherToolPasses(t *testing.T) {
	result := executeGate(t, testServer(), "main", managedRoot(t),
		hookJSON(t, "Read", map[string]interface{}{"file_path": "docs/adr/0001.md"}))
	assertAllowed(t, result)
}

func TestGateEmptyCommandPasses(t *testing.T) {
	result := executeGate(t, testServer(), "main", managedRoot(t),
		hookJSON(t, "Bash", map[string]interface{}{}))
	assertAllowed(t, result)
}

func TestGateInvalidJSONIsErrorNotBlock(t *testing.T) {
	// 壊れた入力は「実行失敗(exit 1)」であり、hook 契約のブロック(exit 2)ではない。
	result := executeGate(t, testServer(), "main", managedRoot(t), "not json")
	if result.code != cli.ExitError {
		t.Fatalf("code = %d, want %d", result.code, cli.ExitError)
	}
	if !strings.Contains(result.err, "hook 入力(JSON)を解釈できません") {
		t.Errorf("理由が出力されない: %q", result.err)
	}
}
