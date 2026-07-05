package cli_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/naito-7110/claude-plugins/tools/factory/internal/board"
	"github.com/naito-7110/claude-plugins/tools/factory/internal/cli"
	"github.com/naito-7110/claude-plugins/tools/factory/internal/cronfake"
	"github.com/naito-7110/claude-plugins/tools/factory/internal/tick"
)

// executeCron は crontab fake を注入して factory を実行する
// (mode / tick の配線テスト用。実 crontab には触れない)。
func executeCron(t *testing.T, cron tick.Crontab, args ...string) run {
	t.Helper()
	var out, errOut strings.Builder
	deps := cli.Deps{
		NewClient:   func() (board.GraphQL, error) { return nil, errors.New("GraphQL は使わない") },
		CurrentRepo: func() (string, error) { return "", errors.New("no git remote") },
		Crontab:     cron,
		Out:         &out,
		Err:         &errOut,
	}
	code := cli.Run(args, deps)
	return run{code: code, out: out.String(), err: errOut.String()}
}

// --- mode gate(受け入れ条件)---

func TestModeGateDefaultIsManualAndBlocks(t *testing.T) {
	root := t.TempDir()

	result := executeCron(t, &cronfake.Crontab{}, "mode", "gate", "--root", root)
	if result.code != cli.ExitError {
		t.Fatalf("code = %d, want %d(既定 = manual の fail-closed)", result.code, cli.ExitError)
	}
	if !strings.Contains(result.err, "運転モードが manual です") {
		t.Errorf("理由が stderr に出ない: %q", result.err)
	}
}

func TestModeFullTransition(t *testing.T) {
	// manual(既定)→ auto → pause → resume → manual の全遷移と gate の対応。
	root := t.TempDir()
	cron := &cronfake.Crontab{}
	gate := func() run { return executeCron(t, cron, "mode", "gate", "--root", root) }

	if result := gate(); result.code != cli.ExitError {
		t.Fatalf("初期状態(manual)で gate = %d, want 1", result.code)
	}

	if result := executeCron(t, cron, "mode", "auto", "--root", root); result.code != cli.ExitOK {
		t.Fatalf("mode auto: code = %d (err=%q)", result.code, result.err)
	}
	if result := gate(); result.code != cli.ExitOK {
		t.Fatalf("auto で gate = %d, want 0 (err=%q)", result.code, result.err)
	}

	if result := executeCron(t, cron, "mode", "pause", "--root", root); result.code != cli.ExitOK {
		t.Fatalf("mode pause: code = %d", result.code)
	}
	result := gate()
	if result.code != cli.ExitError {
		t.Fatalf("paused で gate = %d, want 1", result.code)
	}
	if !strings.Contains(result.err, "一時停止中です") {
		t.Errorf("理由が stderr に出ない: %q", result.err)
	}

	if result := executeCron(t, cron, "mode", "resume", "--root", root); result.code != cli.ExitOK {
		t.Fatalf("mode resume: code = %d", result.code)
	}
	if result := gate(); result.code != cli.ExitOK {
		t.Fatalf("resume 後の gate = %d, want 0", result.code)
	}

	if result := executeCron(t, cron, "mode", "manual", "--root", root); result.code != cli.ExitOK {
		t.Fatalf("mode manual: code = %d", result.code)
	}
	if result := gate(); result.code != cli.ExitError {
		t.Fatalf("manual に戻した後の gate = %d, want 1", result.code)
	}
}

// --- mode status ---

func TestModeStatusShowsStateAndTick(t *testing.T) {
	root := t.TempDir()
	cron := &cronfake.Crontab{}
	if result := executeCron(t, cron, "mode", "auto", "--root", root); result.code != cli.ExitOK {
		t.Fatal(result.err)
	}
	if result := executeCron(t, cron, "tick", "install", "--root", root); result.code != cli.ExitOK {
		t.Fatal(result.err)
	}

	result := executeCron(t, cron, "mode", "status", "--root", root)
	if result.code != cli.ExitOK {
		t.Fatalf("code = %d (err=%q)", result.code, result.err)
	}
	for _, want := range []string{
		"運転モード: auto",
		"一時停止: なし",
		"tick: 設置済み",
		"gate: 通過",
	} {
		if !strings.Contains(result.out, want) {
			t.Errorf("出力に %q がない: %q", want, result.out)
		}
	}
}

func TestModeStatusWithoutCrontabStillWorks(t *testing.T) {
	// crontab が使えない環境でも mode status 自体は成功する(情報表示に留める)。
	root := t.TempDir()

	result := executeCron(t, cronfake.Broken(), "mode", "status", "--root", root)
	if result.code != cli.ExitOK {
		t.Fatalf("code = %d (err=%q)", result.code, result.err)
	}
	for _, want := range []string{
		"運転モード: manual",
		"tick: 確認できません",
		"gate: 遮断",
	} {
		if !strings.Contains(result.out, want) {
			t.Errorf("出力に %q がない: %q", want, result.out)
		}
	}
}

// --- tick 配線 ---

func TestTickInstallStatusRemoveFlow(t *testing.T) {
	root := t.TempDir()
	cron := &cronfake.Crontab{Content: "MAILTO=ops@example.com\n"}

	result := executeCron(t, cron, "tick", "install", "--root", root)
	if result.code != cli.ExitOK {
		t.Fatalf("install: code = %d (err=%q)", result.code, result.err)
	}
	if !strings.Contains(result.out, "==> tick を設置しました") {
		t.Errorf("設置の報告がない: %q", result.out)
	}
	if !strings.Contains(cron.Content, tick.MarkerBegin) || !strings.Contains(cron.Content, "MAILTO=ops@example.com") {
		t.Errorf("crontab の最終状態が不正: %q", cron.Content)
	}

	result = executeCron(t, cron, "tick", "status")
	if result.code != cli.ExitOK || !strings.Contains(result.out, "==> tick: 設置済み") {
		t.Fatalf("status: code = %d out = %q", result.code, result.out)
	}

	// 再 install は置換(冪等)。
	result = executeCron(t, cron, "tick", "install", "--root", root, "--schedule", "0 4 * * *")
	if result.code != cli.ExitOK || !strings.Contains(result.out, "==> tick を置換しました") {
		t.Fatalf("再 install: code = %d out = %q", result.code, result.out)
	}
	if strings.Count(cron.Content, tick.MarkerBegin) != 1 {
		t.Errorf("ブロックが増殖している: %q", cron.Content)
	}

	result = executeCron(t, cron, "tick", "remove")
	if result.code != cli.ExitOK || !strings.Contains(result.out, "==> tick を除去しました") {
		t.Fatalf("remove: code = %d out = %q", result.code, result.out)
	}
	if cron.Content != "MAILTO=ops@example.com\n" {
		t.Errorf("他の行が壊れている: %q", cron.Content)
	}

	result = executeCron(t, cron, "tick", "remove")
	if result.code != cli.ExitOK || !strings.Contains(result.out, "未設置です(変更なし)") {
		t.Fatalf("二重 remove: code = %d out = %q", result.code, result.out)
	}
}

func TestTickInstallInvalidScheduleIsUsageError(t *testing.T) {
	cron := &cronfake.Crontab{}

	result := executeCron(t, cron, "tick", "install", "--root", t.TempDir(), "--schedule", "毎晩")
	if result.code != cli.ExitUsage {
		t.Fatalf("code = %d, want %d", result.code, cli.ExitUsage)
	}
	if cron.Content != "" {
		t.Errorf("失敗時に crontab が書き換わっている: %q", cron.Content)
	}
}

func TestTickInstallMissingRootIsError(t *testing.T) {
	result := executeCron(t, &cronfake.Crontab{}, "tick", "install", "--root", t.TempDir()+"/no-such-dir")
	if result.code != cli.ExitError {
		t.Fatalf("code = %d, want %d", result.code, cli.ExitError)
	}
	if !strings.Contains(result.err, "root がディレクトリとして読めません") {
		t.Errorf("err = %q", result.err)
	}
}

func TestModeUnknownVerbShowsUsage(t *testing.T) {
	result := executeCron(t, &cronfake.Crontab{}, "mode", "turbo")
	if result.code != cli.ExitUsage {
		t.Fatalf("code = %d, want %d", result.code, cli.ExitUsage)
	}
}
