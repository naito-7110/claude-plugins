package cli_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/naito-7110/claude-plugins/tools/factory/internal/board"
	"github.com/naito-7110/claude-plugins/tools/factory/internal/cli"
	"github.com/naito-7110/claude-plugins/tools/factory/internal/cronfake"
	"github.com/naito-7110/claude-plugins/tools/factory/internal/tick"
)

// cliExecStub は claude 起動の fake(起動内容と回数を状態として記録する)。
type cliExecStub struct {
	code  int
	calls []string
}

func (s *cliExecStub) Run(dir, name string, args ...string) (int, error) {
	s.calls = append(s.calls, name+" "+strings.Join(args, " "))
	return s.code, nil
}

func executeTickRun(t *testing.T, launcher tick.Exec, args ...string) run {
	t.Helper()
	var out, errOut strings.Builder
	deps := cli.Deps{
		NewClient:   func() (board.GraphQL, error) { return nil, errors.New("GraphQL は使わない") },
		CurrentRepo: func() (string, error) { return "", errors.New("no git remote") },
		Crontab:     &cronfake.Crontab{},
		TickExec:    launcher,
		Out:         &out,
		Err:         &errOut,
	}
	code := cli.Run(args, deps)
	return run{code: code, out: out.String(), err: errOut.String()}
}

func TestTickRunPassesThroughExitCode(t *testing.T) {
	root := t.TempDir()
	launcher := &cliExecStub{code: 3}

	result := executeTickRun(t, launcher, "tick", "run", "--root", root)
	if result.code != 3 {
		t.Fatalf("code = %d, want 3(claude の終了コードを引き継ぐ)(err=%q)", result.code, result.err)
	}
	if len(launcher.calls) != 1 || launcher.calls[0] != "claude -p /factory:night" {
		t.Errorf("calls = %q", launcher.calls)
	}
	if _, err := os.Stat(filepath.Join(root, ".agents", "night.lock")); err != nil {
		t.Errorf("ロックファイルが作られていない: %v", err)
	}
}

func TestTickRunCustomPrompt(t *testing.T) {
	root := t.TempDir()
	launcher := &cliExecStub{}

	result := executeTickRun(t, launcher, "tick", "run", "--root", root, "--prompt", "/factory:review")
	if result.code != cli.ExitOK {
		t.Fatalf("code = %d (err=%q)", result.code, result.err)
	}
	if len(launcher.calls) != 1 || launcher.calls[0] != "claude -p /factory:review" {
		t.Errorf("calls = %q", launcher.calls)
	}
	if _, err := os.Stat(filepath.Join(root, ".agents", "review.lock")); err != nil {
		t.Errorf("review 用ロックが作られていない: %v", err)
	}
}

func TestTickInstallLineUsesTickRunWithoutFlock(t *testing.T) {
	root := t.TempDir()
	cron := &cronfake.Crontab{}

	result := executeCron(t, cron, "tick", "install", "--root", root)
	if result.code != cli.ExitOK {
		t.Fatalf("code = %d (err=%q)", result.code, result.err)
	}
	if !strings.Contains(cron.Content, "tick run >> .agents/night.log 2>&1") {
		t.Errorf("生成行が tick run を使っていない: %q", cron.Content)
	}
	if strings.Contains(cron.Content, "flock") {
		t.Errorf("生成行が flock コマンドに依存している(macOS に存在しない): %q", cron.Content)
	}
}
