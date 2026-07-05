package tick_test

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/naito-7110/claude-plugins/tools/factory/internal/ghfake"
	"github.com/naito-7110/claude-plugins/tools/factory/internal/tick"
)

// 作業検知プリチェック(#111)のテスト。GitHub は ghfake、時刻は注入。

const detectRepo = "acme/widgets"

var detectNow = time.Date(2026, 7, 5, 12, 0, 0, 0, time.UTC)

func detectServer() *ghfake.Server {
	server := ghfake.NewServer()
	server.Viewer = "factory-bot" // 「自分」= 認証ユーザー(#107 と同一判定)
	return server
}

func detectOpts(root string, server *ghfake.Server) tick.RunOptions {
	return tick.RunOptions{
		Root:   root,
		Repo:   detectRepo,
		Client: server,
		Now:    func() time.Time { return detectNow },
	}
}

func writeState(t *testing.T, root string, at time.Time) {
	t.Helper()
	p := filepath.Join(root, filepath.FromSlash(tick.StateFile))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(at.UTC().Format(time.RFC3339)+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func readState(t *testing.T, root string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(tick.StateFile)))
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(string(data))
}

// --- AC 1: 仕事ゼロのとき claude が起動されない ---

func TestRunNoWorkSkipsClaudeAndKeepsState(t *testing.T) {
	root := autoRoot(t)
	server := detectServer()
	// 検知条件に「かすって外れる」実態を全部並べる(誤検知の回帰テストを兼ねる)。
	server.AddIssue(detectRepo, &ghfake.Issue{ // needs-human でブロック中
		Number: 1, State: "OPEN", Labels: []string{"agent-ok", "needs-human"}})
	server.AddIssue(detectRepo, &ghfake.Issue{ // 作業中(agent-wip)
		Number: 2, State: "OPEN", Labels: []string{"agent-ok", "agent-wip"}})
	server.AddIssue(detectRepo, &ghfake.Issue{ // クローズ済み
		Number: 3, State: "CLOSED", Labels: []string{"agent-ok"}})
	server.AddIssue(detectRepo, &ghfake.Issue{ // agent-ok なし
		Number: 4, State: "OPEN", Labels: []string{"priority:high"}})
	server.AddPullRequest(detectRepo, &ghfake.PullRequest{ // 最新コメントが自分 → 再応答待ち
		Number: 10, HeadRefName: "agent/issue-1-x", State: "OPEN",
		ReviewThreads: []ghfake.ReviewThread{{Resolved: false, LastAuthor: "factory-bot"}}})
	server.AddPullRequest(detectRepo, &ghfake.PullRequest{ // 解決済みスレッドのみ
		Number: 11, HeadRefName: "agent/issue-2-y", State: "OPEN",
		ReviewThreads: []ghfake.ReviewThread{{Resolved: true, LastAuthor: "human"}}})
	server.AddPullRequest(detectRepo, &ghfake.PullRequest{ // agent PR でない
		Number: 12, HeadRefName: "feature/z", State: "OPEN",
		ReviewThreads: []ghfake.ReviewThread{{Resolved: false, LastAuthor: "human"}}})
	server.AddPullRequest(detectRepo, &ghfake.PullRequest{ // 前回起動より前の merged
		Number: 13, HeadRefName: "agent/issue-3-z", State: "MERGED",
		MergedAt: "2026-07-05T09:00:00Z"})
	baseline := time.Date(2026, 7, 5, 10, 0, 0, 0, time.UTC)
	writeState(t, root, baseline)

	launcher := &stubExec{}
	var out strings.Builder
	code, err := tick.Run(launcher, detectOpts(root, server), &out)
	if err != nil {
		t.Fatal(err)
	}
	if code != 0 {
		t.Errorf("code = %d, want 0(空振りは正常系)", code)
	}
	if len(launcher.calls) != 0 {
		t.Errorf("仕事ゼロなのに claude が起動されている: %q", launcher.calls)
	}
	if lines := strings.Count(strings.TrimSpace(out.String()), "\n") + 1; lines != 1 {
		t.Errorf("空振りの出力が 1 行でない(%d 行): %q", lines, out.String())
	}
	// AC 3: tick-state は起動時のみ更新(空振りで進まない)。
	if got := readState(t, root); got != baseline.Format(time.RFC3339) {
		t.Errorf("空振りで tick-state が更新されている: %q", got)
	}
}

// --- AC 2: 4 条件それぞれで起動される + AC 3: 起動時に state 更新 ---

func TestRunReadyIssueLaunches(t *testing.T) {
	root := autoRoot(t)
	server := detectServer()
	server.AddIssue(detectRepo, &ghfake.Issue{
		Number: 42, State: "OPEN", Labels: []string{"agent-ok"}})

	launcher := &stubExec{}
	var out strings.Builder
	code, err := tick.Run(launcher, detectOpts(root, server), &out)
	if err != nil {
		t.Fatal(err)
	}
	if code != 0 || len(launcher.calls) != 1 {
		t.Fatalf("起動されない: code=%d calls=%q", code, launcher.calls)
	}
	if !strings.Contains(out.String(), "Ready の issue #42") {
		t.Errorf("検知理由が出力されない: %q", out.String())
	}
	// 起動時に tick-state が記録される。
	if got := readState(t, root); got != detectNow.Format(time.RFC3339) {
		t.Errorf("tick-state = %q, want %q", got, detectNow.Format(time.RFC3339))
	}
}

func TestRunUnresolvedThreadLaunches(t *testing.T) {
	root := autoRoot(t)
	server := detectServer()
	server.AddPullRequest(detectRepo, &ghfake.PullRequest{
		Number: 59, HeadRefName: "agent/issue-38-verify", State: "OPEN",
		ReviewThreads: []ghfake.ReviewThread{
			{Resolved: false, LastAuthor: "factory-bot"}, // 自分が最新 → 対象外
			{Resolved: false, LastAuthor: "naito-7110"},  // 人間が最新 → 検知
		}})

	launcher := &stubExec{}
	var out strings.Builder
	code, err := tick.Run(launcher, detectOpts(root, server), &out)
	if err != nil {
		t.Fatal(err)
	}
	if code != 0 || len(launcher.calls) != 1 {
		t.Fatalf("起動されない: code=%d calls=%q", code, launcher.calls)
	}
	if !strings.Contains(out.String(), "PR #59 に未対応のレビュースレッド") {
		t.Errorf("検知理由が出力されない: %q", out.String())
	}
}

func TestRunReviewFailureLaunches(t *testing.T) {
	root := autoRoot(t)
	server := detectServer()
	server.AddPullRequest(detectRepo, &ghfake.PullRequest{
		Number: 60, HeadRefName: "agent/issue-40-x", State: "OPEN",
		ReviewState: "FAILURE"})

	launcher := &stubExec{}
	var out strings.Builder
	code, err := tick.Run(launcher, detectOpts(root, server), &out)
	if err != nil {
		t.Fatal(err)
	}
	if code != 0 || len(launcher.calls) != 1 {
		t.Fatalf("起動されない: code=%d calls=%q", code, launcher.calls)
	}
	if !strings.Contains(out.String(), "PR #60 が factory-review = failure") {
		t.Errorf("検知理由が出力されない: %q", out.String())
	}
}

func TestRunMergedSinceLastRunLaunches(t *testing.T) {
	root := autoRoot(t)
	server := detectServer()
	server.AddPullRequest(detectRepo, &ghfake.PullRequest{
		Number: 61, HeadRefName: "agent/issue-41-y", State: "MERGED",
		MergedAt: "2026-07-05T11:00:00Z"}) // 前回起動(10:00)より後
	writeState(t, root, time.Date(2026, 7, 5, 10, 0, 0, 0, time.UTC))

	launcher := &stubExec{}
	var out strings.Builder
	code, err := tick.Run(launcher, detectOpts(root, server), &out)
	if err != nil {
		t.Fatal(err)
	}
	if code != 0 || len(launcher.calls) != 1 {
		t.Fatalf("起動されない: code=%d calls=%q", code, launcher.calls)
	}
	if !strings.Contains(out.String(), "PR #61 が前回 tick 以降にマージ") {
		t.Errorf("検知理由が出力されない: %q", out.String())
	}
}

// --- AC 4: API 呼び出しは 1 tick あたり 4 回以内 ---

// countingClient は Do の呼び出し回数を状態として記録する。
type countingClient struct {
	inner tick.GraphQL
	calls int
}

func (c *countingClient) Do(query string, vars map[string]interface{}, response interface{}) error {
	c.calls++
	return c.inner.Do(query, vars, response)
}

func TestRunDetectionAPIBudget(t *testing.T) {
	// 空振り(全条件を最後まで検査する最悪ケース)でも 4 クエリ以内。
	root := autoRoot(t)
	counting := &countingClient{inner: detectServer()}
	opts := detectOpts(root, nil)
	opts.Client = counting

	if _, err := tick.Run(&stubExec{}, opts, io.Discard); err != nil {
		t.Fatal(err)
	}
	if counting.calls > 4 {
		t.Errorf("API 呼び出しが %d 回(4 回以内に収める — 1 分周期のレート対策)", counting.calls)
	}
}

// --- 検知不能時のフォールバック(取りこぼし防止優先)---

func TestRunWithoutClientFallsBackToLaunch(t *testing.T) {
	root := autoRoot(t)
	launcher := &stubExec{}
	var out strings.Builder

	code, err := tick.Run(launcher, tick.RunOptions{Root: root}, &out)
	if err != nil {
		t.Fatal(err)
	}
	if code != 0 || len(launcher.calls) != 1 {
		t.Fatalf("フォールバック起動されない: code=%d calls=%q", code, launcher.calls)
	}
	if !strings.Contains(out.String(), "作業検知を実行できないため起動します") {
		t.Errorf("フォールバックの説明がない: %q", out.String())
	}
}
