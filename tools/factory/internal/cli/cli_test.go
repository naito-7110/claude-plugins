package cli_test

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/naito-7110/claude-plugins/tools/factory/internal/board"
	"github.com/naito-7110/claude-plugins/tools/factory/internal/cli"
	"github.com/naito-7110/claude-plugins/tools/factory/internal/ghfake"
)

func testServer() *ghfake.Server {
	server := ghfake.NewServer()
	server.AddOwner("naito-7110", "User")
	server.AddProject(&ghfake.Project{
		Owner:         "naito-7110",
		Number:        4,
		Title:         "factory board template",
		StatusOptions: slices.Clone(board.StatusOptions),
		ViewLayouts:   []string{"TABLE_LAYOUT", "BOARD_LAYOUT", "ROADMAP_LAYOUT"},
		Fields:        map[string]string{"Target date": "DATE"},
	})
	return server
}

type run struct {
	code int
	out  string
	err  string
}

func execute(t *testing.T, server *ghfake.Server, currentRepo string, args ...string) run {
	t.Helper()
	var out, errOut strings.Builder
	deps := cli.Deps{
		NewClient: func() (board.GraphQL, error) { return server, nil },
		CurrentRepo: func() (string, error) {
			if currentRepo == "" {
				return "", errors.New("no git remote")
			}
			return currentRepo, nil
		},
		Out: &out,
		Err: &errOut,
	}
	code := cli.Run(args, deps)
	return run{code: code, out: out.String(), err: errOut.String()}
}

func TestRunNoArgsShowsUsage(t *testing.T) {
	result := execute(t, testServer(), "")
	if result.code != cli.ExitUsage {
		t.Errorf("code = %d, want %d", result.code, cli.ExitUsage)
	}
	if !strings.Contains(result.err, "使い方") {
		t.Errorf("usage が表示されない: %q", result.err)
	}
}

func TestRunUnknownSubcommand(t *testing.T) {
	result := execute(t, testServer(), "", "board", "destroy")
	if result.code != cli.ExitUsage {
		t.Errorf("code = %d, want %d", result.code, cli.ExitUsage)
	}
}

func TestVerifyRequiresFlags(t *testing.T) {
	result := execute(t, testServer(), "", "board", "verify")
	if result.code != cli.ExitUsage {
		t.Errorf("code = %d, want %d", result.code, cli.ExitUsage)
	}
	if !strings.Contains(result.err, "--owner と --number は必須です") {
		t.Errorf("err = %q", result.err)
	}
}

func TestVerifyGreenExitsZero(t *testing.T) {
	result := execute(t, testServer(), "", "board", "verify", "--owner", "naito-7110", "--number", "4")
	if result.code != cli.ExitOK {
		t.Fatalf("code = %d, want 0 (err=%q)", result.code, result.err)
	}
	if !strings.Contains(result.out, "Status は factory の 6 値です") {
		t.Errorf("out = %q", result.out)
	}
}

func TestVerifyStatusMismatchExitsOne(t *testing.T) {
	server := testServer()
	server.FindProject("naito-7110", 4).StatusOptions = []string{"Todo", "Done"}

	result := execute(t, server, "", "board", "verify", "--owner", "naito-7110", "--number", "4")
	if result.code != cli.ExitError {
		t.Errorf("code = %d, want 1", result.code)
	}
	if !strings.Contains(result.err, "NG: Status の選択肢が期待と異なります") {
		t.Errorf("err = %q", result.err)
	}
}

func TestVerifyWarningsDoNotAffectExitCode(t *testing.T) {
	server := testServer()
	server.FindProject("naito-7110", 4).ViewLayouts = []string{"TABLE_LAYOUT"}

	result := execute(t, server, "", "board", "verify", "--owner", "naito-7110", "--number", "4")
	if result.code != cli.ExitOK {
		t.Errorf("code = %d, want 0(ビュー不足は警告のみ)", result.code)
	}
	if !strings.Contains(result.out, "警告") {
		t.Errorf("警告が出力されない: %q", result.out)
	}
}

func TestCopyRequiresOwner(t *testing.T) {
	result := execute(t, testServer(), "", "board", "copy")
	if result.code != cli.ExitUsage {
		t.Errorf("code = %d, want %d", result.code, cli.ExitUsage)
	}
}

func TestCopyLinksCurrentRepoByDefault(t *testing.T) {
	server := testServer()
	server.AddOwner("target-user", "User")
	server.Repos["target-user/myrepo"] = "REPO_myrepo"

	result := execute(t, server, "target-user/myrepo",
		"board", "copy", "--owner", "target-user")
	if result.code != cli.ExitOK {
		t.Fatalf("code = %d (err=%q)", result.code, result.err)
	}
	copied := server.FindProject("target-user", 1)
	if copied == nil {
		t.Fatal("ボードが複製されていない")
	}
	if copied.Title != "myrepo board" {
		t.Errorf("Title = %q, want %q(カレントリポジトリ名から導出)", copied.Title, "myrepo board")
	}
	if !slices.Contains(copied.LinkedRepos, "REPO_myrepo") {
		t.Errorf("カレントリポジトリにリンクされていない: %v", copied.LinkedRepos)
	}
	if !strings.Contains(result.out, "残る手作業") {
		t.Errorf("チェックリストが出力されない: %q", result.out)
	}
}

func TestCopyWithoutResolvableRepoSkipsLink(t *testing.T) {
	server := testServer()
	server.AddOwner("target-user", "User")

	result := execute(t, server, "", // CurrentRepo は失敗する
		"board", "copy", "--owner", "target-user", "--title", "standalone board")
	if result.code != cli.ExitOK {
		t.Fatalf("code = %d (err=%q)", result.code, result.err)
	}
	if !strings.Contains(result.out, "リンクはスキップします") {
		t.Errorf("スキップの案内がない: %q", result.out)
	}
	copied := server.FindProject("target-user", 1)
	if len(copied.LinkedRepos) != 0 {
		t.Errorf("LinkedRepos = %v, want empty", copied.LinkedRepos)
	}
}

func TestCopyExplicitRepoAndTitle(t *testing.T) {
	server := testServer()
	server.AddOwner("target-user", "User")
	server.Repos["target-user/other"] = "REPO_other"

	result := execute(t, server, "target-user/ignored",
		"board", "copy", "--owner", "target-user", "--repo", "target-user/other", "--title", "custom")
	if result.code != cli.ExitOK {
		t.Fatalf("code = %d (err=%q)", result.code, result.err)
	}
	copied := server.FindProject("target-user", 1)
	if copied.Title != "custom" {
		t.Errorf("Title = %q", copied.Title)
	}
	if !slices.Contains(copied.LinkedRepos, "REPO_other") {
		t.Errorf("LinkedRepos = %v", copied.LinkedRepos)
	}
}
