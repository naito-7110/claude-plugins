package cli_test

import (
	"strings"
	"testing"

	"github.com/naito-7110/claude-plugins/tools/factory/internal/cli"
	"github.com/naito-7110/claude-plugins/tools/factory/internal/ghfake"
)

const testRepo = "naito-7110/claude-plugins"

// readyBody は spec-alignment を満たす Ready な issue 本文。
const readyBody = `## 目的

検証コマンドを提供する。

## 受け入れ条件

- [ ] チェックリスト無し issue で非ゼロ exit
- [ ] 正常系で exit 0

依存: #10(基盤 = マージ済み PR #23)
`

// readyIssue は全検査を通過する issue(#38)と、クローズ済みの依存(#10)を登録する。
func readyIssue(server *ghfake.Server) *ghfake.Issue {
	server.AddIssue(testRepo, &ghfake.Issue{
		Number: 10, State: "CLOSED", Body: "## 受け入れ条件\n\n- [x] done\n",
	})
	return server.AddIssue(testRepo, &ghfake.Issue{
		Number: 38,
		State:  "OPEN",
		Body:   readyBody,
		Labels: []string{"agent-ok"},
	})
}

// --- issue verify: 受け入れ条件(AC 1)---

func TestIssueVerifyMissingChecklistFails(t *testing.T) {
	server := testServer()
	issue := readyIssue(server)
	issue.Body = "## 目的\n\n受け入れ条件の見出しがない issue。\n"

	result := execute(t, server, testRepo, "issue", "verify", "--number", "38")
	if result.code != cli.ExitError {
		t.Fatalf("code = %d, want %d (out=%q)", result.code, cli.ExitError, result.out)
	}
	if !strings.Contains(result.out, "「受け入れ条件」見出しが issue 本文にありません") {
		t.Errorf("理由が出力されない: %q", result.out)
	}
}

func TestIssueVerifyEmptyChecklistFails(t *testing.T) {
	server := testServer()
	issue := readyIssue(server)
	issue.Body = "## 受け入れ条件\n\n(あとで書く)\n"

	result := execute(t, server, testRepo, "issue", "verify", "--number", "38")
	if result.code != cli.ExitError {
		t.Fatalf("code = %d, want %d", result.code, cli.ExitError)
	}
	if !strings.Contains(result.out, "チェックリスト(- [ ] 形式)がありません") {
		t.Errorf("理由が出力されない: %q", result.out)
	}
}

// --- issue verify: ラベル状態(AC 2)---

func TestIssueVerifyNeedsHumanFails(t *testing.T) {
	server := testServer()
	issue := readyIssue(server)
	issue.Labels = []string{"agent-ok", "needs-human"}

	result := execute(t, server, testRepo, "issue", "verify", "--number", "38")
	if result.code != cli.ExitError {
		t.Fatalf("code = %d, want %d", result.code, cli.ExitError)
	}
	if !strings.Contains(result.out, "needs-human ラベルが付与されています") {
		t.Errorf("理由が出力されない: %q", result.out)
	}
}

func TestIssueVerifyMissingAgentOKFails(t *testing.T) {
	server := testServer()
	issue := readyIssue(server)
	issue.Labels = nil

	result := execute(t, server, testRepo, "issue", "verify", "--number", "38")
	if result.code != cli.ExitError {
		t.Fatalf("code = %d, want %d", result.code, cli.ExitError)
	}
	if !strings.Contains(result.out, "agent-ok ラベルがありません") {
		t.Errorf("理由が出力されない: %q", result.out)
	}
}

// --- issue verify: 依存の解消(AC 3)---

func TestIssueVerifyOpenDependencyFails(t *testing.T) {
	server := testServer()
	readyIssue(server)
	server.FindIssue(testRepo, 10).State = "OPEN"

	result := execute(t, server, testRepo, "issue", "verify", "--number", "38")
	if result.code != cli.ExitError {
		t.Fatalf("code = %d, want %d", result.code, cli.ExitError)
	}
	if !strings.Contains(result.out, "依存 #10 が未解消です") {
		t.Errorf("理由が出力されない: %q", result.out)
	}
}

func TestIssueVerifyDependencyParsingIgnoresParenthetical(t *testing.T) {
	// 「依存: #10(... PR #23)」の括弧内 #23 は依存として扱わない。
	// #23 は fake に存在しないので、誤って解析されるとテストが NG を検出する。
	server := testServer()
	readyIssue(server)

	result := execute(t, server, testRepo, "issue", "verify", "--number", "38")
	if result.code != cli.ExitOK {
		t.Fatalf("code = %d, want 0 (out=%q err=%q)", result.code, result.out, result.err)
	}
	if strings.Contains(result.out, "#23") {
		t.Errorf("括弧内の #23 が依存として解析されている: %q", result.out)
	}
}

// --- issue verify: merge:agent の鮮度(AC 4)---

func TestIssueVerifyStaleMergeAgentFails(t *testing.T) {
	server := testServer()
	issue := readyIssue(server)
	issue.Labels = []string{"agent-ok", "merge:agent"}
	issue.LabelEvents = []ghfake.LabelEvent{
		{Label: "merge:agent", CreatedAt: "2026-07-01T10:00:00Z"},
	}
	issue.LastEditedAt = "2026-07-02T09:00:00Z" // 付与後に本文編集

	result := execute(t, server, testRepo, "issue", "verify", "--number", "38")
	if result.code != cli.ExitError {
		t.Fatalf("code = %d, want %d", result.code, cli.ExitError)
	}
	if !strings.Contains(result.out, "後に issue 本文が編集されています") {
		t.Errorf("理由が出力されない: %q", result.out)
	}
}

func TestIssueVerifyFreshMergeAgentPasses(t *testing.T) {
	server := testServer()
	issue := readyIssue(server)
	issue.Labels = []string{"agent-ok", "merge:agent"}
	issue.LabelEvents = []ghfake.LabelEvent{
		// 編集後に再付与されたケース: 最新の付与が編集より後なら新鮮。
		{Label: "merge:agent", CreatedAt: "2026-07-01T10:00:00Z"},
		{Label: "merge:agent", CreatedAt: "2026-07-03T10:00:00Z"},
	}
	issue.LastEditedAt = "2026-07-02T09:00:00Z"

	result := execute(t, server, testRepo, "issue", "verify", "--number", "38")
	if result.code != cli.ExitOK {
		t.Fatalf("code = %d, want 0 (out=%q)", result.code, result.out)
	}
	if !strings.Contains(result.out, "merge:agent は新鮮です") {
		t.Errorf("鮮度の確認が出力されない: %q", result.out)
	}
}

// --- issue verify: 正常系(AC 5)---

func TestIssueVerifyGreenExitsZero(t *testing.T) {
	server := testServer()
	readyIssue(server)

	result := execute(t, server, testRepo, "issue", "verify", "--number", "38")
	if result.code != cli.ExitOK {
		t.Fatalf("code = %d, want 0 (out=%q err=%q)", result.code, result.out, result.err)
	}
	for _, want := range []string{
		"受け入れ条件チェックリストあり(2 項目)",
		"agent-ok ラベルあり",
		"needs-human ラベルなし",
		"依存 #10 はクローズ済み",
		"==> 結果: OK",
	} {
		if !strings.Contains(result.out, want) {
			t.Errorf("出力に %q がない: %q", want, result.out)
		}
	}
}

// --- issue verify: フラグ ---

func TestIssueVerifyChecksSelection(t *testing.T) {
	// チェックリストの無い issue でも --checks labels なら通る。
	server := testServer()
	issue := readyIssue(server)
	issue.Body = "本文のみ"

	result := execute(t, server, testRepo,
		"issue", "verify", "--number", "38", "--checks", "labels")
	if result.code != cli.ExitOK {
		t.Fatalf("code = %d, want 0 (out=%q)", result.code, result.out)
	}
	if strings.Contains(result.out, "受け入れ条件") {
		t.Errorf("選択していない検査が実行されている: %q", result.out)
	}
}

func TestIssueVerifyUnknownCheckIsUsageError(t *testing.T) {
	server := testServer()
	readyIssue(server)

	result := execute(t, server, testRepo,
		"issue", "verify", "--number", "38", "--checks", "vibes")
	if result.code != cli.ExitUsage {
		t.Fatalf("code = %d, want %d", result.code, cli.ExitUsage)
	}
	if !strings.Contains(result.err, "不明な検査項目です: vibes") {
		t.Errorf("err = %q", result.err)
	}
}

func TestIssueVerifyRequiresNumber(t *testing.T) {
	result := execute(t, testServer(), testRepo, "issue", "verify")
	if result.code != cli.ExitUsage {
		t.Fatalf("code = %d, want %d", result.code, cli.ExitUsage)
	}
	if !strings.Contains(result.err, "--number は必須です") {
		t.Errorf("err = %q", result.err)
	}
}

func TestIssueVerifyMissingIssueIsError(t *testing.T) {
	result := execute(t, testServer(), testRepo, "issue", "verify", "--number", "999")
	if result.code != cli.ExitError {
		t.Fatalf("code = %d, want %d", result.code, cli.ExitError)
	}
	if !strings.Contains(result.err, "issue #999 が見つかりません") {
		t.Errorf("err = %q", result.err)
	}
}

// --- pr verify ---

func TestPRVerifyClosesRunsIssueChecks(t *testing.T) {
	server := testServer()
	readyIssue(server)
	server.AddPullRequest(testRepo, &ghfake.PullRequest{
		Number: 50,
		Body:   "# 概要\n\n## 関連 Issue\n\nCloses #38\n",
		Files:  []string{"tools/factory/main.go"},
	})

	result := execute(t, server, testRepo, "pr", "verify", "--number", "50")
	if result.code != cli.ExitOK {
		t.Fatalf("code = %d, want 0 (out=%q err=%q)", result.code, result.out, result.err)
	}
	for _, want := range []string{
		"関連 issue: #38",
		"==> 関連 issue #38 の検証",
		"受け入れ条件チェックリストあり",
		"依存マニフェスト検査はスキップ(--dep-manifests 未指定)",
		"==> 結果: OK",
	} {
		if !strings.Contains(result.out, want) {
			t.Errorf("出力に %q がない: %q", want, result.out)
		}
	}
}

func TestPRVerifyRefsAlsoVerified(t *testing.T) {
	// Refs で参照した issue が needs-human なら PR 検証も失敗する。
	server := testServer()
	readyIssue(server)
	server.AddIssue(testRepo, &ghfake.Issue{
		Number: 40, State: "OPEN",
		Body:   "## 受け入れ条件\n\n- [ ] x\n",
		Labels: []string{"agent-ok", "needs-human"},
	})
	server.AddPullRequest(testRepo, &ghfake.PullRequest{
		Number: 50,
		Body:   "Closes #38\nRefs #40\n",
	})

	result := execute(t, server, testRepo, "pr", "verify", "--number", "50")
	if result.code != cli.ExitError {
		t.Fatalf("code = %d, want %d", result.code, cli.ExitError)
	}
	if !strings.Contains(result.out, "関連 issue: #38, #40") {
		t.Errorf("Refs が解析されない: %q", result.out)
	}
	if !strings.Contains(result.out, "needs-human ラベルが付与されています") {
		t.Errorf("Refs 先の検査結果がない: %q", result.out)
	}
}

func TestPRVerifyWithoutIssueRefsFails(t *testing.T) {
	server := testServer()
	server.AddPullRequest(testRepo, &ghfake.PullRequest{
		Number: 50,
		Body:   "関連 issue の記載が無い PR。",
	})

	result := execute(t, server, testRepo, "pr", "verify", "--number", "50")
	if result.code != cli.ExitError {
		t.Fatalf("code = %d, want %d", result.code, cli.ExitError)
	}
	if !strings.Contains(result.out, "関連 issue が見つかりません") {
		t.Errorf("理由が出力されない: %q", result.out)
	}
}

func TestPRVerifyManifestChangeWithoutDeclarationFails(t *testing.T) {
	server := testServer()
	readyIssue(server) // readyBody に依存追加の明記はない
	server.AddPullRequest(testRepo, &ghfake.PullRequest{
		Number: 50,
		Body:   "Closes #38",
		Files:  []string{"tools/factory/go.mod", "tools/factory/go.sum", "tools/factory/main.go"},
	})

	result := execute(t, server, testRepo, "pr", "verify", "--number", "50",
		"--dep-manifests", "go.mod,go.sum")
	if result.code != cli.ExitError {
		t.Fatalf("code = %d, want %d (out=%q)", result.code, cli.ExitError, result.out)
	}
	if !strings.Contains(result.out, "関連 issue に依存の追加が明記されていません") {
		t.Errorf("理由が出力されない: %q", result.out)
	}
	if !strings.Contains(result.out, "tools/factory/go.mod") {
		t.Errorf("該当ファイルが出力されない: %q", result.out)
	}
}

func TestPRVerifyManifestChangeWithDeclarationPasses(t *testing.T) {
	server := testServer()
	issue := readyIssue(server)
	issue.Body += "\n## 依存の追加\n\n- example.com/lib(pros: 軽量 / cons: 若い)\n"
	server.AddPullRequest(testRepo, &ghfake.PullRequest{
		Number: 50,
		Body:   "Closes #38",
		Files:  []string{"tools/factory/go.mod"},
	})

	result := execute(t, server, testRepo, "pr", "verify", "--number", "50",
		"--dep-manifests", "go.mod,go.sum")
	if result.code != cli.ExitOK {
		t.Fatalf("code = %d, want 0 (out=%q)", result.code, result.out)
	}
	if !strings.Contains(result.out, "関連 issue に明記済み") {
		t.Errorf("明記の確認が出力されない: %q", result.out)
	}
}

func TestPRVerifyNoManifestChangePasses(t *testing.T) {
	server := testServer()
	readyIssue(server)
	server.AddPullRequest(testRepo, &ghfake.PullRequest{
		Number: 50,
		Body:   "Closes #38",
		Files:  []string{"tools/factory/main.go"},
	})

	result := execute(t, server, testRepo, "pr", "verify", "--number", "50",
		"--dep-manifests", "go.mod,go.sum,**/package.json")
	if result.code != cli.ExitOK {
		t.Fatalf("code = %d, want 0 (out=%q)", result.code, result.out)
	}
	if !strings.Contains(result.out, "依存マニフェストの変更なし") {
		t.Errorf("出力: %q", result.out)
	}
}

func TestPRVerifyMissingPRIsError(t *testing.T) {
	result := execute(t, testServer(), testRepo, "pr", "verify", "--number", "999")
	if result.code != cli.ExitError {
		t.Fatalf("code = %d, want %d", result.code, cli.ExitError)
	}
	if !strings.Contains(result.err, "PR #999 が見つかりません") {
		t.Errorf("err = %q", result.err)
	}
}
