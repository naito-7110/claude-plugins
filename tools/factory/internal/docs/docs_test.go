package docs

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// --- glob(** 対応)の一致規則 ---

func TestMatchPattern(t *testing.T) {
	tests := []struct {
		pattern string
		file    string
		want    bool
	}{
		// ** は 0 個以上のセグメントに一致する
		{"src/api/**", "src/api/main.go", true},
		{"src/api/**", "src/api/deep/nested/file.go", true},
		{"src/api/**", "src/api", true}, // ** は 0 セグメントにも一致(doublestar と同じ)
		{"src/api/**", "src/web/main.go", false},
		{"**/handler.go", "src/api/handler.go", true},
		{"**/handler.go", "handler.go", true},
		{"src/**/model.go", "src/api/db/model.go", true},
		// セグメント内のワイルドカード(path.Match の規則)
		{"src/*/main.go", "src/api/main.go", true},
		{"src/*/main.go", "src/api/db/main.go", false},
		{"src/api/*.go", "src/api/main.go", true},
		{"src/api/*.go", "src/api/main_test.py", false},
		// ワイルドカード無しはディレクトリ前置としても一致する
		{"src/api", "src/api/main.go", true},
		{"src/api", "src/api", true},
		{"src/api", "src/apiserver/main.go", false},
		{"go.mod", "go.mod", true},
		// 前後の / は無視する
		{"src/api/", "src/api/main.go", true},
	}
	for _, tt := range tests {
		if got := matchPattern(tt.pattern, tt.file); got != tt.want {
			t.Errorf("matchPattern(%q, %q) = %v, want %v", tt.pattern, tt.file, got, tt.want)
		}
	}
}

// --- ファイル列挙(git リポジトリでは gitignore を尊重する)---

func TestListTrackedFilesRespectsGitignore(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git がありません")
	}
	root := t.TempDir()
	write := func(rel, content string) {
		t.Helper()
		p := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(".gitignore", "dist/\n")
	write("src/main.go", "package main\n")
	write("dist/bundle.js", "// build artifact\n")
	if out, err := exec.Command("git", "-C", root, "init", "-q").CombinedOutput(); err != nil {
		t.Fatalf("git init: %v (%s)", err, out)
	}

	files, err := listTrackedFiles(root)
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(files, "\n")
	if !strings.Contains(joined, "src/main.go") {
		t.Errorf("未コミットのファイルが列挙されない: %q", joined)
	}
	if strings.Contains(joined, "dist/bundle.js") {
		t.Errorf("gitignore されたファイルが列挙されている: %q", joined)
	}
}

func TestWalkFilesSkipsDotDirectories(t *testing.T) {
	root := t.TempDir()
	for _, rel := range []string{"src/main.go", ".git/config", ".agents/journal/x.md"} {
		p := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	files, err := walkFiles(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 || files[0] != "src/main.go" {
		t.Errorf("files = %v, want [src/main.go]", files)
	}
}

// --- ドメイン名の検証(パス区切りの混入はディレクトリ逸脱になる)---

func TestVerifyRejectsDomainNameWithSeparator(t *testing.T) {
	root := t.TempDir()
	write := func(rel, content string) {
		t.Helper()
		p := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("docs/factory/README.md", "# 地図\n")
	write("docs/factory/ownership.yml", "domains:\n  \"../escape\":\n    paths: [\"src/**\"]\n")

	report, err := Verify(root)
	if err != nil {
		t.Fatal(err)
	}
	if report.OK() {
		t.Fatalf("不正なドメイン名が通っている: %+v", report.Findings)
	}
	found := false
	for _, f := range report.Findings {
		if strings.Contains(f.Message, "ドメイン名") && strings.Contains(f.Message, "不正") {
			found = true
		}
	}
	if !found {
		t.Errorf("理由が出力されない: %+v", report.Findings)
	}
}
