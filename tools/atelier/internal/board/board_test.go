package board_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/naito-7110/claude-plugins/tools/atelier/internal/board"
	"github.com/naito-7110/claude-plugins/tools/atelier/internal/ghfake"
)

// canonical は正準ボード相当の fake ボードを登録した Server を返す。
func canonical() (*ghfake.Server, *ghfake.Project) {
	server := ghfake.NewServer()
	server.AddOwner("naito-7110", "User")
	project := server.AddProject(&ghfake.Project{
		Owner:         "naito-7110",
		Number:        4,
		Title:         "atelier board template",
		StatusOptions: slices.Clone(board.StatusOptions),
		ViewLayouts:   []string{"TABLE_LAYOUT", "BOARD_LAYOUT", "ROADMAP_LAYOUT"},
		Fields:        map[string]string{"Target date": "DATE"},
	})
	return server, project
}

func TestVerifyGreen(t *testing.T) {
	server, _ := canonical()

	report, err := board.Verify(server, "naito-7110", 4)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if !report.StatusOK {
		t.Errorf("StatusOK = false, want true (actual=%v)", report.ActualStatus)
	}
	if len(report.Warnings) != 0 {
		t.Errorf("Warnings = %v, want empty", report.Warnings)
	}
}

func TestVerifyStatusMismatchIsHardFailure(t *testing.T) {
	server, project := canonical()
	project.StatusOptions = []string{"Todo", "In Progress", "Done"} // GitHub 既定の 3 値

	report, err := board.Verify(server, "naito-7110", 4)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if report.StatusOK {
		t.Error("StatusOK = true, want false")
	}
	if !slices.Equal(report.ActualStatus, []string{"Todo", "In Progress", "Done"}) {
		t.Errorf("ActualStatus = %v", report.ActualStatus)
	}
}

func TestVerifyStatusOrderMatters(t *testing.T) {
	server, project := canonical()
	project.StatusOptions = []string{"Spec", "Inbox", "Ready", "In Progress", "In Review", "Done"}

	report, err := board.Verify(server, "naito-7110", 4)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if report.StatusOK {
		t.Error("順序違いの Status を許容してはいけない")
	}
}

func TestVerifyMissingViewsAndFieldAreWarnings(t *testing.T) {
	server, project := canonical()
	project.ViewLayouts = []string{"TABLE_LAYOUT"} // Board / Roadmap 不足
	project.Fields = map[string]string{}           // Target date 不足

	report, err := board.Verify(server, "naito-7110", 4)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if !report.StatusOK {
		t.Error("ビュー・フィールド不足で StatusOK を落としてはいけない(警告のみ)")
	}
	if len(report.Warnings) != 3 {
		t.Fatalf("Warnings = %v, want 3 (BOARD, ROADMAP, Target date)", report.Warnings)
	}
	joined := strings.Join(report.Warnings, "\n")
	for _, want := range []string{"BOARD", "ROADMAP", "Target date"} {
		if !strings.Contains(joined, want) {
			t.Errorf("Warnings に %q が含まれない: %v", want, report.Warnings)
		}
	}
}

func TestVerifyResolvesOrganizationOwner(t *testing.T) {
	server, _ := canonical()
	server.AddOwner("some-org", "Organization")
	server.AddProject(&ghfake.Project{
		Owner:         "some-org",
		Number:        7,
		StatusOptions: slices.Clone(board.StatusOptions),
		ViewLayouts:   []string{"TABLE_LAYOUT", "BOARD_LAYOUT", "ROADMAP_LAYOUT"},
		Fields:        map[string]string{"Target date": "DATE"},
	})

	report, err := board.Verify(server, "some-org", 7)
	if err != nil {
		t.Fatalf("Verify(org): %v", err)
	}
	if !report.StatusOK {
		t.Errorf("StatusOK = false, want true (actual=%v)", report.ActualStatus)
	}
}

func TestVerifyMissingProject(t *testing.T) {
	server, _ := canonical()

	_, err := board.Verify(server, "naito-7110", 999)
	if err == nil {
		t.Fatal("存在しないボードでエラーにならない")
	}
}

func TestCopyReplicatesCanonicalBoard(t *testing.T) {
	server, _ := canonical()
	server.AddOwner("target-user", "User")
	server.Repos["target-user/myrepo"] = "REPO_myrepo"

	result, err := board.Copy(server, board.CopyOptions{
		SourceOwner:  "naito-7110",
		SourceNumber: 4,
		TargetOwner:  "target-user",
		Title:        "myrepo board",
		Repo:         "target-user/myrepo",
	})
	if err != nil {
		t.Fatalf("Copy: %v", err)
	}

	copied := server.FindProject("target-user", result.Number)
	if copied == nil {
		t.Fatal("複製先にボードが存在しない")
	}
	// Status は正準ボードから完全複製される(実 API で観測済みの挙動)
	if !slices.Equal(copied.StatusOptions, board.StatusOptions) {
		t.Errorf("StatusOptions = %v", copied.StatusOptions)
	}
	// 説明欄は copy では複製されないため、Copy が設定する
	if !strings.Contains(copied.Description, "naito-7110/projects/4") {
		t.Errorf("Description = %q, 正準ボードへの言及がない", copied.Description)
	}
	// リポジトリへリンクされている
	if !slices.Contains(copied.LinkedRepos, "REPO_myrepo") {
		t.Errorf("LinkedRepos = %v", copied.LinkedRepos)
	}
	if result.LinkedRepo != "target-user/myrepo" {
		t.Errorf("LinkedRepo = %q", result.LinkedRepo)
	}
	// 複製直後の verify が green
	if !result.Report.StatusOK {
		t.Errorf("Report.StatusOK = false (actual=%v)", result.Report.ActualStatus)
	}
	if len(result.Report.Warnings) != 0 {
		t.Errorf("Report.Warnings = %v", result.Report.Warnings)
	}
	if result.URL == "" || result.Number == 0 {
		t.Errorf("URL/Number が空: %+v", result)
	}
}

func TestCopyWithoutRepoSkipsLink(t *testing.T) {
	server, _ := canonical()
	server.AddOwner("target-user", "User")

	result, err := board.Copy(server, board.CopyOptions{
		SourceOwner:  "naito-7110",
		SourceNumber: 4,
		TargetOwner:  "target-user",
		Title:        "no-link board",
	})
	if err != nil {
		t.Fatalf("Copy: %v", err)
	}
	copied := server.FindProject("target-user", result.Number)
	if len(copied.LinkedRepos) != 0 {
		t.Errorf("LinkedRepos = %v, want empty", copied.LinkedRepos)
	}
	if result.LinkedRepo != "" {
		t.Errorf("LinkedRepo = %q, want empty", result.LinkedRepo)
	}
}

func TestCopyToOrganizationOwner(t *testing.T) {
	server, _ := canonical()
	server.AddOwner("some-org", "Organization")

	result, err := board.Copy(server, board.CopyOptions{
		SourceOwner:  "naito-7110",
		SourceNumber: 4,
		TargetOwner:  "some-org",
		Title:        "org board",
	})
	if err != nil {
		t.Fatalf("Copy(org): %v", err)
	}
	if !result.Report.StatusOK {
		t.Errorf("Report.StatusOK = false")
	}
	if !strings.Contains(result.URL, "/orgs/some-org/") {
		t.Errorf("URL = %q", result.URL)
	}
}

func TestCopyUnknownSourceFails(t *testing.T) {
	server := ghfake.NewServer()
	server.AddOwner("naito-7110", "User")
	server.AddOwner("target-user", "User")

	_, err := board.Copy(server, board.CopyOptions{
		SourceOwner:  "naito-7110",
		SourceNumber: 4,
		TargetOwner:  "target-user",
		Title:        "x",
	})
	if err == nil {
		t.Fatal("存在しない正準ボードでエラーにならない")
	}
}

func TestCopyUnknownTargetOwnerFails(t *testing.T) {
	server, _ := canonical()

	_, err := board.Copy(server, board.CopyOptions{
		SourceOwner:  "naito-7110",
		SourceNumber: 4,
		TargetOwner:  "no-such-owner",
		Title:        "x",
	})
	if err == nil {
		t.Fatal("存在しない owner でエラーにならない")
	}
}
