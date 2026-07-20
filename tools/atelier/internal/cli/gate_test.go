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
// branch はプロジェクトルート(cwd 指定なし)のブランチ。空文字なら
// ブランチ解決の失敗(unborn かつ symbolic-ref も不能)を再現する。
func executeGate(t *testing.T, server *ghfake.Server, branch, root, stdin string) run {
	t.Helper()
	return executeGateAt(t, server, map[string]string{"": branch}, root, stdin)
}

// executeGateAt はディレクトリ → ブランチの対応(キー "" = プロジェクトルート)で
// ブランチ解決を偽装して atelier gate を実行する(#138: hook cwd 基準の判定)。
// 対応が無い・空文字のディレクトリは解決失敗を再現する。
func executeGateAt(t *testing.T, server *ghfake.Server, branches map[string]string, root, stdin string) run {
	t.Helper()
	var out, errOut strings.Builder
	deps := cli.Deps{
		NewClient:   func() (board.GraphQL, error) { return server, nil },
		CurrentRepo: func() (string, error) { return testRepo, nil },
		CurrentBranch: func(dir string) (string, error) {
			if branches[dir] == "" {
				return "", errors.New("ブランチを解決できません")
			}
			return branches[dir], nil
		},
		In:  strings.NewReader(stdin),
		Out: &out,
		Err: &errOut,
	}
	code := cli.Run([]string{"gate", "--root", root}, deps)
	return run{code: code, out: out.String(), err: errOut.String()}
}

// hookJSON は PreToolUse の stdin JSON を組み立てる(extra はトップレベルに足す)。
func hookJSON(t *testing.T, tool string, toolInput map[string]interface{}, extra ...map[string]interface{}) string {
	t.Helper()
	payload := map[string]interface{}{"tool_name": tool, "tool_input": toolInput}
	for _, m := range extra {
		for k, v := range m {
			payload[k] = v
		}
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func bashJSON(t *testing.T, cmd string) string {
	t.Helper()
	return hookJSON(t, "Bash", map[string]interface{}{"command": cmd})
}

// bashJSONAt は実行ディレクトリ(hook JSON のトップレベル cwd)つきの Bash 入力。
func bashJSONAt(t *testing.T, cwd, cmd string) string {
	t.Helper()
	return hookJSON(t, "Bash", map[string]interface{}{"command": cmd},
		map[string]interface{}{"cwd": cwd})
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
// CI green・atelier-review green・レビュア投稿者 ≠ PR 作者)を満たす PR #64 を
// 登録する。
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
		Author:        "impl-agent",
		ReviewCreator: "reviewer-bot",
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
		bashJSON(t, "git push origin main"),           // main 直 push
		bashJSON(t, "git push --force origin main"),   // force push
		bashJSON(t, "gh pr merge 64 --squash"),        // マージ
		bashJSON(t, "atelier release atelier/v0.3.0"), // リリース
		bashJSON(t, "git push --tags"),                // タグ push
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

// #119 の仕様変更: push ゲートは「push 元のカレントブランチ」ではなく
// 「push が実際に触る refspec(宛先 / push 元)」で判定する。カレントが main でも、
// 明示的に別ブランチを push するなら main を変えないので通す(worktree で main を
// checkout したルートから agent ブランチを push する正規の操作を塞がないため)。
func TestGatePushFromMainWithExplicitBranchPasses(t *testing.T) {
	result := executeGate(t, testServer(), "main", managedRoot(t),
		bashJSON(t, "git push origin feature-x"))
	assertAllowed(t, result)
}

// ただし refspec 無しの `git push`(カレント main を暗黙に push)は宛先が main と
// 解釈され、直 push ゲートで止まる(防御の核心は維持)。
func TestGateBarePushFromMainBlocked(t *testing.T) {
	result := executeGate(t, testServer(), "main", managedRoot(t),
		bashJSON(t, "git push"))
	assertBlocked(t, result, "main への直 push は禁止です")
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

// --- #119: 文字列部分一致の誤爆が構文判定で解消されることの回帰テスト ---

// コミットメッセージ・引数に push / main / release / タグの語が入っても、
// 実コマンドが該当操作でなければ誤爆しない(トークン化で語とコマンドを区別)。
func TestGateNoFalsePositiveOnQuotedWords(t *testing.T) {
	server := testServer()
	readyIssue(server)
	for _, cmd := range []string{
		`git commit -m "covers push/merge/release; and && more"`, // メッセージ内の語
		`git commit -m "main への直 push は禁止"`,                      // メッセージ内の main + push
		`gh issue create --title "atelier/v1.0.0 のタグを push する話"`, // 引用符内のタグ + push
		`echo "git push origin main"`,                            // echo の引数
	} {
		result := executeGate(t, server, "agent/issue-38-gate", managedRoot(t), bashJSON(t, cmd))
		assertAllowed(t, result)
	}
}

// 連結コマンドの引数が混ざっても、セグメント分割で別コマンドとして判定する。
// 原本の誤爆: rebase の `origin/main` と push の `--force-with-lease` が同居して
// 「main への force push」と誤判定された(dogfoodry#7)。
func TestGateNoFalsePositiveOnChainedCommands(t *testing.T) {
	server := testServer()
	readyIssue(server)
	for _, cmd := range []string{
		"git rebase --onto origin/main x && git push --force-with-lease origin agent/issue-38-gate",
		"git fetch origin main && git push -u origin agent/issue-38-gate", // fetch の main と push が同居
	} {
		result := executeGate(t, server, "agent/issue-38-gate", managedRoot(t), bashJSON(t, cmd))
		assertAllowed(t, result)
	}
}

// 連結内に本物の禁止操作があれば、そのセグメントで捕捉する(すり抜けさせない)。
func TestGateChainedCommandStillCatchesRealPush(t *testing.T) {
	result := executeGate(t, testServer(), "agent/issue-38-gate", managedRoot(t),
		bashJSON(t, "git add -A && git push origin main"))
	assertBlocked(t, result, "main への直 push は禁止です")
}

// 引用符付きタグ名でもリリースゲートに掛かる(旧実装のすり抜け穴)。
func TestGateQuotedTagPushBlocked(t *testing.T) {
	result := executeGate(t, testServer(), "agent/issue-38-gate", managedRoot(t),
		bashJSON(t, "git push origin 'atelier/v1.0.0'"))
	assertBlocked(t, result, "タグの push は人間の操作です")
}

// セミコロン直後に空白が無い連結でもセグメント分割される(旧実装のすり抜け穴)。
func TestGateNoSpaceSeparatorStillCatchesRelease(t *testing.T) {
	result := executeGate(t, testServer(), "agent/issue-38-gate", managedRoot(t),
		bashJSON(t, "echo start;atelier release atelier/v1.0.0"))
	assertBlocked(t, result, "リリースタグは人間の操作です")
}

// worktree からの明示 refspec push は、ルートのカレントブランチに依らず
// refspec(宛先)で判定する(#119 証拠 2: ルート main で agent push が誤爆した)。
func TestGateWorktreePushJudgedByRefspec(t *testing.T) {
	server := testServer()
	readyIssue(server)
	// ルートは main を checkout しているが、push するのは agent ブランチ。
	result := executeGate(t, server, "main", managedRoot(t),
		bashJSON(t, "git push -u origin agent/issue-38-gate"))
	assertAllowed(t, result) // main 扱いされず、agent の issue 検証(green)を通る
}

// --- #138: ブランチ判定は hook stdin JSON の cwd 基準(worktree からの push)---
//
// hook の cwd(= Bash が実行されるディレクトリ)が worktree のとき、カレント
// ブランチはルート checkout ではなく worktree のブランチで判定する。ルートが
// main のままでも、worktree 内からの agent ブランチ push を誤ブロックしない。

// worktree(agent ブランチ)内からの refspec 無し `git push` は、worktree の
// ブランチに解決され、push ゲート(issue 検証 green)を通る。
func TestGateBarePushFromWorktreeCwdUsesWorktreeBranch(t *testing.T) {
	server := testServer()
	readyIssue(server)
	branches := map[string]string{"": "main", "/repo/.worktrees/issue-38": "agent/issue-38-gate"}
	result := executeGateAt(t, server, branches, managedRoot(t),
		bashJSONAt(t, "/repo/.worktrees/issue-38", "git push"))
	assertAllowed(t, result)
}

// HEAD / 明示 refspec の push も cwd のブランチで解決・判定される。
func TestGateWorktreeCwdPushVariantsPass(t *testing.T) {
	branches := map[string]string{"": "main", "/repo/.worktrees/issue-38": "agent/issue-38-gate"}
	for _, cmd := range []string{
		"git push origin HEAD", // HEAD は cwd のブランチ(agent)に解決される
		"git push -u origin agent/issue-38-gate",
	} {
		server := testServer()
		readyIssue(server)
		result := executeGateAt(t, server, branches, managedRoot(t),
			bashJSONAt(t, "/repo/.worktrees/issue-38", cmd))
		assertAllowed(t, result)
	}
}

// cwd 基準になっても push ゲートは弱まらない: worktree の issue が push 条件を
// 満たさなければ従来どおりブロックする。
func TestGateWorktreeCwdPushNotReadyIssueBlocked(t *testing.T) {
	server := testServer()
	issue := readyIssue(server)
	issue.Labels = nil // agent-ok なし
	branches := map[string]string{"": "main", "/repo/.worktrees/issue-38": "agent/issue-38-gate"}
	result := executeGateAt(t, server, branches, managedRoot(t),
		bashJSONAt(t, "/repo/.worktrees/issue-38", "git push"))
	assertBlocked(t, result, "issue #38 の状態が push 条件を満たしません")
}

// main への直 push は cwd がルートでも worktree でも引き続きブロックする。
func TestGatePushToMainFromWorktreeCwdStillBlocked(t *testing.T) {
	branches := map[string]string{
		"":                          "main",
		"/repo/.worktrees/issue-38": "agent/issue-38-gate",
		"/repo/.worktrees/hotfix":   "main", // worktree 側が main の変則も塞ぐ
	}
	cases := []struct{ cwd, cmd, want string }{
		{"/repo/.worktrees/issue-38", "git push origin main", "main への直 push は禁止です"},              // 明示 main 宛て
		{"/repo/.worktrees/issue-38", "git push --force origin main", "main への force push は禁止です"}, // force
		{"/repo/.worktrees/hotfix", "git push", "main への直 push は禁止です"},                            // cwd のブランチが main
		{"/repo/.worktrees/hotfix", "git push origin HEAD", "main への直 push は禁止です"},                // HEAD = main
	}
	for _, tc := range cases {
		result := executeGateAt(t, testServer(), branches, managedRoot(t),
			bashJSONAt(t, tc.cwd, tc.cmd))
		assertBlocked(t, result, tc.want)
	}
}

// マージゲートの PR 番号フォールバック(番号なし gh pr merge)も cwd のブランチで
// 解決される(deps.Branch を共有するため。ルート main では PR を特定できないが、
// worktree のブランチなら PR #64 に解決され、そのゲート判定が適用される)。
func TestGateMergeNumberFromWorktreeCwdBranch(t *testing.T) {
	server := testServer()
	pr := mergeReadyPR(server)
	pr.ReviewState = "FAILURE" // 解決された PR #64 がゲートされることで解決を実証する
	branches := map[string]string{"": "main", "/repo/.worktrees/issue-38": "agent/issue-38-gate"}
	result := executeGateAt(t, server, branches, managedRoot(t),
		bashJSONAt(t, "/repo/.worktrees/issue-38", "gh pr merge --squash"))
	assertBlocked(t, result, "PR #64 は別コンテキストレビュア")
}

// cwd が JSON に無ければ従来どおりプロジェクトルートのブランチで判定する
// (フォールバック。ルート main の bare push は直 push ゲートで止まる)。
func TestGateNoCwdFallsBackToRootBranch(t *testing.T) {
	branches := map[string]string{"": "main"}
	result := executeGateAt(t, testServer(), branches, managedRoot(t),
		bashJSON(t, "git push")) // cwd なし
	assertBlocked(t, result, "main への直 push は禁止です")
}

// cwd のブランチが解決できない(git リポジトリ外等)場合はプロジェクトルートで
// 再解決する(#138 以前の「常にルートで判定」の防御水準を下限として維持 —
// 非リポジトリな cwd を経由した bare push / HEAD push がルートの main 判定を
// 逃れる fail-open にしない)。
func TestGateUnresolvedCwdFallsBackToRootBranch(t *testing.T) {
	branches := map[string]string{"": "main"} // cwd の対応なし = 解決失敗
	for _, cmd := range []string{"git push origin HEAD", "git push"} {
		result := executeGateAt(t, testServer(), branches, managedRoot(t),
			bashJSONAt(t, "/outside/repo", cmd))
		assertBlocked(t, result, "main への直 push は禁止です")
	}
}

// cwd でもルートでも解決できなければ、従来どおり「未解決 = 判定材料なし」で
// 素通し(その環境では git push 自体が失敗する)。
func TestGateBranchUnresolvedEverywherePasses(t *testing.T) {
	branches := map[string]string{} // どこでも解決失敗
	result := executeGateAt(t, testServer(), branches, managedRoot(t),
		bashJSONAt(t, "/outside/repo", "git push origin HEAD"))
	assertAllowed(t, result)
}

// git のグローバル -C はコマンドの実効ディレクトリを変える。ブランチ判定は
// hook cwd ではなく -C のパスで行う(cwd を別の場所へ移してから
// `git -C <main の checkout> push` でルートの main を押す迂回を塞ぐ)。
func TestGateDashCDirOverridesCwdForBranch(t *testing.T) {
	branches := map[string]string{
		"":                          "feature/x", // ルートは main 以外(フォールバックでは検出できない配置)
		"/repo/wt-main":             "main",
		"/repo/.worktrees/issue-38": "agent/issue-38-gate",
	}
	// cwd は解決不能な場所・agent worktree のどちらでも、-C の先が main なら止まる。
	for _, cwd := range []string{"/outside/repo", "/repo/.worktrees/issue-38"} {
		result := executeGateAt(t, testServer(), branches, managedRoot(t),
			bashJSONAt(t, cwd, "git -C /repo/wt-main push"))
		assertBlocked(t, result, "main への直 push は禁止です")
	}
}

// 相対パスの -C は cwd 起点で合成される(worktree 相対の自然な操作)。
func TestGateRelativeDashCJoinedWithCwd(t *testing.T) {
	server := testServer()
	readyIssue(server)
	branches := map[string]string{"": "main", "/repo/.worktrees/issue-38": "agent/issue-38-gate"}
	result := executeGateAt(t, server, branches, managedRoot(t),
		bashJSONAt(t, "/repo/.worktrees", "git -C issue-38 push"))
	assertAllowed(t, result) // /repo/.worktrees/issue-38 の agent ブランチとして判定される
}

// git のグローバルオプション(-C <path> / --no-pager / -c k=v)を前置しても
// push を見失わない(セルフレビュー指摘の退行修正。worktree で -C は自然)。
func TestGateGitGlobalOptionsStillGatePush(t *testing.T) {
	for _, cmd := range []string{
		"git -C /some/worktree push origin main",
		"git --no-pager push origin main",
		"git -c protocol.version=2 push origin main",
	} {
		result := executeGate(t, testServer(), "agent/issue-38-x", managedRoot(t), bashJSON(t, cmd))
		assertBlocked(t, result, "main への直 push は禁止です")
	}
}

// 宛先 main の別表記(force refspec + / refs/heads/ / HEAD)も捕捉する。
func TestGateMainRefspecVariantsBlocked(t *testing.T) {
	cases := []struct{ cmd, want string }{
		{"git push origin +main", "main への force push は禁止です"},           // 先頭 + = force refspec
		{"git push origin refs/heads/main", "main への直 push は禁止です"},      // refs/heads/ 接頭辞
		{"git push origin HEAD:refs/heads/main", "main への直 push は禁止です"}, // 明示 dst
		{"git push -fu origin main", "main への force push は禁止です"},        // 結合短縮フラグ -fu
	}
	for _, tc := range cases {
		result := executeGate(t, testServer(), "feature/x", managedRoot(t), bashJSON(t, tc.cmd))
		assertBlocked(t, result, tc.want)
	}
}

// カレントが main のとき、refspec 無し `git push`・`git push origin HEAD`・
// その同義語 `@` はいずれも宛先が main に解決され、直 push ゲートで止まる。
func TestGateHeadRefspecFromMainBlocked(t *testing.T) {
	for _, cmd := range []string{"git push origin HEAD", "git push origin @"} {
		result := executeGate(t, testServer(), "main", managedRoot(t), bashJSON(t, cmd))
		assertBlocked(t, result, "main への直 push は禁止です")
	}
}

// --all / --mirror は refspec 無しでもローカルの全 ref(main を含む)を push する。
// カレントが main 以外でもブロックする(refspec 無し = カレントのみの仮定が崩れる穴)。
func TestGateAllRefsPushBlocked(t *testing.T) {
	for _, cmd := range []string{"git push --all origin", "git push --mirror origin"} {
		result := executeGate(t, testServer(), "feature/x", managedRoot(t), bashJSON(t, cmd))
		assertBlocked(t, result, "全ブランチの push")
	}
}

// gh のグローバルフラグ(-R o/r)を前置してもマージゲートに掛かる。
func TestGateGhGlobalRepoFlagStillGatesMerge(t *testing.T) {
	server := testServer()
	mergeReadyPR(server)
	// merge 条件は満たすので通る = ゲートに掛かった上で条件 green を通過。
	result := executeGate(t, server, "agent/issue-38-gate", managedRoot(t),
		bashJSON(t, "gh -R o/r pr merge 64 --squash"))
	assertAllowed(t, result)
}

func TestGatePushOtherBranchPasses(t *testing.T) {
	result := executeGate(t, testServer(), "feature/x", managedRoot(t),
		bashJSON(t, "git push origin feature/x"))
	assertAllowed(t, result)
}

// --- 5. リリースゲート(デプロイ = 人間の tag push)---

func TestGateAtelierReleaseBlocked(t *testing.T) {
	// release サブコマンドは #129 で撤去済みだが、旧版バイナリ(factory 名義
	// 含む)が残存する環境への防御として起動検出は維持する。
	for _, cmd := range []string{
		"atelier release atelier/v0.3.0",
		".agents/bin/atelier release v1.0.0",
		"cd /repo && atelier release atelier/v0.3.0 --remote origin",
		".agents/bin/factory release factory/v0.2.2", // 旧名バイナリの残存対策
		"factory release factory/v0.2.2",
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
		"git push origin factory/v0.2.2", // 旧名タグも塞ぐ
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

// --- #116: atelier-review 投稿者の検証(merge-policy: 独立の (d) 資格情報)---
//
// status が green でも、その投稿者(creator.login)が PR 作者と同一なら独立
// レビューではない(実装セッションが起動したサブエージェントの自己承認を、
// 資格情報の同一性で機械検出する)。投稿者・PR 作者が特定できない場合も
// fail-closed でブロックする。

// 投稿者が PR 作者と別アカウントなら通る(不一致 = 独立の検証可能な状態)。
func TestGateMergeReviewerDistinctFromAuthorPasses(t *testing.T) {
	server := testServer()
	pr := mergeReadyPR(server)
	pr.Author = "impl-agent"
	pr.ReviewCreator = "reviewer-bot"

	result := executeGate(t, server, "agent/issue-38-gate", managedRoot(t),
		bashJSON(t, "gh pr merge 64 --squash"))
	assertAllowed(t, result)
}

// 投稿者が PR 作者と同一アカウントなら、status が green でもブロックする。
func TestGateMergeReviewerSameAsAuthorBlocked(t *testing.T) {
	server := testServer()
	pr := mergeReadyPR(server)
	pr.Author = "impl-agent"
	pr.ReviewCreator = "impl-agent"

	result := executeGate(t, server, "agent/issue-38-gate", managedRoot(t),
		bashJSON(t, "gh pr merge 64 --squash"))
	assertBlocked(t, result, "PR 作者と同一アカウント")
}

// 投稿者(または PR 作者)が特定できない場合も fail-closed でブロックする。
func TestGateMergeReviewerUnknownBlocked(t *testing.T) {
	cases := []struct{ author, creator string }{
		{"impl-agent", ""}, // 投稿者不明
		{"", "reviewer-bot"}, // PR 作者不明(作者側が欠けても同一性を検証できない)
		{"", ""},             // 両方不明
	}
	for _, tc := range cases {
		server := testServer()
		pr := mergeReadyPR(server)
		pr.Author = tc.author
		pr.ReviewCreator = tc.creator

		result := executeGate(t, server, "agent/issue-38-gate", managedRoot(t),
			bashJSON(t, "gh pr merge 64 --squash"))
		assertBlocked(t, result, "投稿者を検証できません")
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
