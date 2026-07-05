package tick_test

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/naito-7110/claude-plugins/tools/factory/internal/cronfake"
	"github.com/naito-7110/claude-plugins/tools/factory/internal/tick"
)

const otherLines = "MAILTO=ops@example.com\n30 1 * * * /usr/local/bin/backup.sh\n"

func TestInstallOnEmptyCrontab(t *testing.T) {
	cron := &cronfake.Crontab{}

	replaced, err := tick.Install(cron, "/repo", tick.DefaultSchedule, "/usr/local/bin/factory")
	if err != nil {
		t.Fatal(err)
	}
	if replaced {
		t.Error("空の crontab で replaced = true")
	}
	want := tick.MarkerBegin + "\n" + tick.Line("/repo", tick.DefaultSchedule, "/usr/local/bin/factory") + "\n" + tick.MarkerEnd + "\n"
	if cron.Content != want {
		t.Errorf("Content = %q, want %q", cron.Content, want)
	}
}

func TestInstallPreservesOtherLines(t *testing.T) {
	cron := &cronfake.Crontab{Content: otherLines}

	if _, err := tick.Install(cron, "/repo", tick.DefaultSchedule, "/usr/local/bin/factory"); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(cron.Content, otherLines) {
		t.Errorf("既存の行が保存されていない: %q", cron.Content)
	}
	if !strings.Contains(cron.Content, tick.MarkerBegin) || !strings.Contains(cron.Content, tick.MarkerEnd) {
		t.Errorf("マーカーブロックがない: %q", cron.Content)
	}
}

func TestInstallTwiceIsIdempotent(t *testing.T) {
	cron := &cronfake.Crontab{Content: otherLines}

	if _, err := tick.Install(cron, "/repo", tick.DefaultSchedule, "/usr/local/bin/factory"); err != nil {
		t.Fatal(err)
	}
	replaced, err := tick.Install(cron, "/repo", "15 4 * * *", "/usr/local/bin/factory")
	if err != nil {
		t.Fatal(err)
	}
	if !replaced {
		t.Error("2 回目の install で replaced = false")
	}
	if got := strings.Count(cron.Content, tick.MarkerBegin); got != 1 {
		t.Errorf("マーカーブロックが %d 個ある(冪等でない): %q", got, cron.Content)
	}
	if !strings.Contains(cron.Content, "15 4 * * * cd /repo") {
		t.Errorf("スケジュールが置換されていない: %q", cron.Content)
	}
	if strings.Contains(cron.Content, tick.DefaultSchedule+" cd /repo") {
		t.Errorf("旧スケジュールが残っている: %q", cron.Content)
	}
	if !strings.HasPrefix(cron.Content, otherLines) {
		t.Errorf("既存の行が保存されていない: %q", cron.Content)
	}
}

func TestInstallRejectsInvalidSchedule(t *testing.T) {
	cron := &cronfake.Crontab{Content: otherLines}

	if _, err := tick.Install(cron, "/repo", "3:00 に起動", "/usr/local/bin/factory"); err == nil {
		t.Fatal("不正な cron 式が受理されている")
	}
	if cron.Content != otherLines {
		t.Errorf("失敗時に crontab が書き換わっている: %q", cron.Content)
	}
}

func TestInstallAcceptsAtNotation(t *testing.T) {
	cron := &cronfake.Crontab{}
	if _, err := tick.Install(cron, "/repo", "@daily", "/usr/local/bin/factory"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(cron.Content, "@daily cd /repo") {
		t.Errorf("@ 記法が使えない: %q", cron.Content)
	}
}

func TestRemoveDeletesOnlyMarkerBlock(t *testing.T) {
	cron := &cronfake.Crontab{Content: otherLines}
	if _, err := tick.Install(cron, "/repo", tick.DefaultSchedule, "/usr/local/bin/factory"); err != nil {
		t.Fatal(err)
	}

	removed, err := tick.Remove(cron)
	if err != nil {
		t.Fatal(err)
	}
	if !removed {
		t.Error("removed = false")
	}
	if cron.Content != otherLines {
		t.Errorf("Content = %q, want %q(他の行だけが残る)", cron.Content, otherLines)
	}
}

func TestRemoveWhenAbsentDoesNotWrite(t *testing.T) {
	// 未設置なら書き込み自体が発生しない: Write を失敗させても Remove は成功する。
	cron := &cronfake.Crontab{Content: otherLines, WriteErr: errors.New("書き込み禁止")}

	removed, err := tick.Remove(cron)
	if err != nil {
		t.Fatal(err)
	}
	if removed {
		t.Error("未設置で removed = true")
	}
	if cron.Content != otherLines {
		t.Errorf("未設置なのに内容が変わっている: %q", cron.Content)
	}
}

func TestBrokenMarkersFailWithoutWriting(t *testing.T) {
	broken := otherLines + tick.MarkerBegin + "\n0 3 * * * something\n" // end がない
	cron := &cronfake.Crontab{Content: broken}

	if _, err := tick.Install(cron, "/repo", tick.DefaultSchedule, "/usr/local/bin/factory"); err == nil {
		t.Fatal("壊れたマーカーで install が成功している")
	}
	if _, err := tick.Remove(cron); err == nil {
		t.Fatal("壊れたマーカーで remove が成功している")
	}
	if cron.Content != broken {
		t.Errorf("壊れた crontab に書き込んでいる: %q", cron.Content)
	}
}

func TestStatus(t *testing.T) {
	cron := &cronfake.Crontab{Content: otherLines}

	installed, _, err := tick.Status(cron)
	if err != nil {
		t.Fatal(err)
	}
	if installed {
		t.Error("未設置で installed = true")
	}

	if _, err := tick.Install(cron, "/repo", tick.DefaultSchedule, "/usr/local/bin/factory"); err != nil {
		t.Fatal(err)
	}
	installed, block, err := tick.Status(cron)
	if err != nil {
		t.Fatal(err)
	}
	if !installed {
		t.Error("設置済みで installed = false")
	}
	if len(block) != 1 || !strings.Contains(block[0], "tick run") {
		t.Errorf("block = %q(tick run の起動行が入る)", block)
	}
}

func TestLineShape(t *testing.T) {
	// flock コマンドは macOS に存在しないため、生成行は OS コマンドに依存しない
	// (多重起動防止は factory tick run が内蔵する)。
	line := tick.Line("/path/to/repo", tick.DefaultSchedule, "/usr/local/bin/factory")
	for _, want := range []string{
		"0 3 * * 1-5 cd /path/to/repo && ",
		"/usr/local/bin/factory tick run",
		">> .agents/night.log 2>&1",
	} {
		if !strings.Contains(line, want) {
			t.Errorf("Line に %q がない: %q", want, line)
		}
	}
	if strings.Contains(line, "flock") {
		t.Errorf("生成行が flock コマンドに依存している: %q", line)
	}
}

func TestInstallReplacesLegacyFlockBlock(t *testing.T) {
	// 旧形式(flock コマンド)のブロックが入っている環境でも、
	// install はマーカーごと新形式に置換する(冪等仕様)。
	legacy := otherLines +
		tick.MarkerBegin + "\n" +
		`0 3 * * 1-5 cd /repo && flock -n .agents/night.lock -c 'claude -p "/factory:night" >> .agents/night.log 2>&1'` + "\n" +
		tick.MarkerEnd + "\n"
	cron := &cronfake.Crontab{Content: legacy}

	replaced, err := tick.Install(cron, "/repo", tick.DefaultSchedule, "/usr/local/bin/factory")
	if err != nil {
		t.Fatal(err)
	}
	if !replaced {
		t.Error("旧ブロックの置換で replaced = false")
	}
	if strings.Contains(cron.Content, "flock") {
		t.Errorf("flock の行が残っている: %q", cron.Content)
	}
	if !strings.Contains(cron.Content, "/usr/local/bin/factory tick run") {
		t.Errorf("新形式の行がない: %q", cron.Content)
	}
	if !strings.HasPrefix(cron.Content, otherLines) {
		t.Errorf("既存の行が保存されていない: %q", cron.Content)
	}
}

// --- tick run(多重起動ロック)---

// stubExec は起動を記録して固定の終了コードを返す fake(状態検証用)。
type stubExec struct {
	code  int
	calls []string
}

func (s *stubExec) Run(dir, name string, args ...string) (int, error) {
	s.calls = append(s.calls, name+" "+strings.Join(args, " "))
	return s.code, nil
}

// blockingExec は起動後、release されるまでロックを保持し続ける fake。
type blockingExec struct {
	started chan struct{}
	release chan struct{}
	code    int
}

func (b *blockingExec) Run(dir, name string, args ...string) (int, error) {
	close(b.started)
	<-b.release
	return b.code, nil
}

func TestRunPassesThroughExitCode(t *testing.T) {
	root := t.TempDir()
	launcher := &stubExec{code: 7}

	code, err := tick.Run(launcher, root, "", io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if code != 7 {
		t.Errorf("code = %d, want 7(claude の終了コードを引き継ぐ)", code)
	}
	if len(launcher.calls) != 1 || launcher.calls[0] != "claude -p /factory:night" {
		t.Errorf("calls = %q(既定は /factory:night)", launcher.calls)
	}
}

func TestRunSecondInstanceSkipsWithExitZero(t *testing.T) {
	// 二重起動: 先行がロックを保持している間、後発は exit 0 で即終了する(正常系)。
	root := t.TempDir()
	first := &blockingExec{started: make(chan struct{}), release: make(chan struct{}), code: 7}
	done := make(chan int, 1)
	go func() {
		code, err := tick.Run(first, root, "", io.Discard)
		if err != nil {
			t.Error(err)
		}
		done <- code
	}()
	<-first.started // 先行がロックを取得して claude 実行中になるまで待つ

	second := &stubExec{}
	var out strings.Builder
	code, err := tick.Run(second, root, "", &out)
	if err != nil {
		t.Fatal(err)
	}
	if code != 0 {
		t.Errorf("後発の code = %d, want 0(多重起動はエラーにしない)", code)
	}
	if !strings.Contains(out.String(), "スキップします") {
		t.Errorf("スキップの 1 行が出力されない: %q", out.String())
	}
	if len(second.calls) != 0 {
		t.Errorf("後発が claude を起動している(二重起動): %q", second.calls)
	}

	close(first.release)
	if code := <-done; code != 7 {
		t.Errorf("先行の code = %d, want 7", code)
	}
}

func TestRunPromptSelectsLockAndSkill(t *testing.T) {
	// prompt ごとに独立したロック(night と review は同時に走れる)。
	root := t.TempDir()
	night := &blockingExec{started: make(chan struct{}), release: make(chan struct{})}
	done := make(chan int, 1)
	go func() {
		code, _ := tick.Run(night, root, "", io.Discard)
		done <- code
	}()
	<-night.started

	review := &stubExec{}
	code, err := tick.Run(review, root, "/factory:review", io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if code != 0 || len(review.calls) != 1 || review.calls[0] != "claude -p /factory:review" {
		t.Errorf("review 側が night のロックに阻まれている: code=%d calls=%q", code, review.calls)
	}
	if _, err := os.Stat(filepath.Join(root, ".agents", "review.lock")); err != nil {
		t.Errorf("review 用ロックファイルが作られていない: %v", err)
	}

	close(night.release)
	<-done
}
