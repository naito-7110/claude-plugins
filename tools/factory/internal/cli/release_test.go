package cli_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/naito-7110/claude-plugins/tools/factory/internal/board"
	"github.com/naito-7110/claude-plugins/tools/factory/internal/cli"
)

// cliGitStub は release.Git の最小 fake(CLI 配線テスト用)。
type cliGitStub struct {
	created map[string]string
	pushed  []string
}

func newCLIGitStub() *cliGitStub {
	return &cliGitStub{created: map[string]string{}}
}

func (s *cliGitStub) Fetch(string) error                   { return nil }
func (s *cliGitStub) DefaultBranch(string) (string, error) { return "main", nil }
func (s *cliGitStub) ResolveRemoteRef(remote, ref string) (string, error) {
	if remote == "origin" && ref == "main" {
		return "abc1234", nil
	}
	return "", errors.New("unknown revision")
}
func (s *cliGitStub) LocalTagExists(string) (bool, error)          { return false, nil }
func (s *cliGitStub) RemoteTagExists(string, string) (bool, error) { return false, nil }
func (s *cliGitStub) CreateTag(tag, sha string) error              { s.created[tag] = sha; return nil }
func (s *cliGitStub) PushTag(remote, tag string) error {
	s.pushed = append(s.pushed, remote+"/"+tag)
	return nil
}

func executeRelease(t *testing.T, git *cliGitStub, args ...string) run {
	t.Helper()
	var out, errOut strings.Builder
	deps := cli.Deps{
		NewClient:   func() (board.GraphQL, error) { return nil, errors.New("GraphQL は使わない") },
		CurrentRepo: func() (string, error) { return "", errors.New("no git remote") },
		ReleaseGit:  git,
		Out:         &out,
		Err:         &errOut,
	}
	code := cli.Run(args, deps)
	return run{code: code, out: out.String(), err: errOut.String()}
}

func TestReleaseTagThenFlags(t *testing.T) {
	git := newCLIGitStub()

	result := executeRelease(t, git, "release", "factory/v9.9.9", "--dry-run")
	if result.code != cli.ExitOK {
		t.Fatalf("code = %d (err=%q)", result.code, result.err)
	}
	if len(git.created) != 0 || len(git.pushed) != 0 {
		t.Errorf("dry-run で変更がある: %v %v", git.created, git.pushed)
	}
	if !strings.Contains(result.out, "dry-run: 変更しません") {
		t.Errorf("出力: %q", result.out)
	}
}

func TestReleaseFlagsThenTag(t *testing.T) {
	// フラグが先・タグが後の順でも受ける。
	git := newCLIGitStub()

	result := executeRelease(t, git, "release", "--dry-run", "factory/v9.9.9")
	if result.code != cli.ExitOK {
		t.Fatalf("code = %d (err=%q)", result.code, result.err)
	}
}

func TestReleaseExecutesTagAndPush(t *testing.T) {
	git := newCLIGitStub()

	result := executeRelease(t, git, "release", "v1.0.0")
	if result.code != cli.ExitOK {
		t.Fatalf("code = %d (err=%q)", result.code, result.err)
	}
	if git.created["v1.0.0"] != "abc1234" {
		t.Errorf("created = %v", git.created)
	}
	if len(git.pushed) != 1 || git.pushed[0] != "origin/v1.0.0" {
		t.Errorf("pushed = %v", git.pushed)
	}
}

func TestReleaseWithoutTagIsUsageError(t *testing.T) {
	result := executeRelease(t, newCLIGitStub(), "release")
	if result.code != cli.ExitUsage {
		t.Fatalf("code = %d, want %d", result.code, cli.ExitUsage)
	}
	if !strings.Contains(result.err, "タグ名を指定してください") {
		t.Errorf("err = %q", result.err)
	}
}

func TestReleaseUnknownRefIsError(t *testing.T) {
	result := executeRelease(t, newCLIGitStub(), "release", "v1.0.0", "--ref", "ghost")
	if result.code != cli.ExitError {
		t.Fatalf("code = %d, want %d", result.code, cli.ExitError)
	}
	if !strings.Contains(result.err, "origin/ghost を解決できません") {
		t.Errorf("err = %q", result.err)
	}
}
