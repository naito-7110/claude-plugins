package cli_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/naito-7110/claude-plugins/tools/factory/internal/cli"
	"github.com/naito-7110/claude-plugins/tools/factory/internal/docs"
)

// layout は検査対象パスの単一定義(internal/docs)。テストのパスも
// すべてここを参照し、配置の変更に定義 1 箇所で追随できるようにする。
var layout = docs.DefaultLayout

// writeFile は root 配下に rel(/ 区切り)のファイルを作る(親ディレクトリごと)。
func writeFile(t *testing.T, root, rel, content string) {
	t.Helper()
	p := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// removeFile は root 配下の rel(/ 区切り)を削除する。
func removeFile(t *testing.T, root, rel string) {
	t.Helper()
	if err := os.Remove(filepath.Join(root, filepath.FromSlash(rel))); err != nil {
		t.Fatal(err)
	}
}

// scaffoldDocs は documentation プリセットに適合する最小のリポジトリを作る。
// ドメイン api が src/api/** を所有し、実ファイルと必須文書が揃っている。
func scaffoldDocs(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	writeFile(t, root, layout.MapReadme, "# 文書の地図\n")
	writeFile(t, root, layout.OwnershipFile, `# パス(glob)→ ドメインの所有マップ
domains:
  api:
    paths:
      - "src/api/**"
`)
	writeFile(t, root, layout.DomainDoc("api", "README.md"), "# api\n")
	writeFile(t, root, layout.DomainDoc("api", "contracts.md"), "# 公開契約\n")
	writeFile(t, root, "src/api/main.go", "package main\n")
	return root
}

func executeDocs(t *testing.T, root string) run {
	t.Helper()
	return execute(t, testServer(), testRepo, "docs", "verify", "--root", root)
}

// --- 正常系 ---

func TestDocsVerifyOK(t *testing.T) {
	root := scaffoldDocs(t)

	result := executeDocs(t, root)
	if result.code != cli.ExitOK {
		t.Fatalf("code = %d, want %d (out=%q err=%q)", result.code, cli.ExitOK, result.out, result.err)
	}
	if !strings.Contains(result.out, "==> 結果: OK") {
		t.Errorf("結果が出力されない: %q", result.out)
	}
}

func TestDocsVerifyEmptyDomainsOK(t *testing.T) {
	// domains: {}(ドメイン未分割)は漸進導入のため正常。
	root := t.TempDir()
	writeFile(t, root, layout.MapReadme, "# 文書の地図\n")
	writeFile(t, root, layout.OwnershipFile, "domains: {}\n")

	result := executeDocs(t, root)
	if result.code != cli.ExitOK {
		t.Fatalf("code = %d, want %d (out=%q)", result.code, cli.ExitOK, result.out)
	}
	if !strings.Contains(result.out, "ドメイン未分割") {
		t.Errorf("理由が出力されない: %q", result.out)
	}
}

// --- 地図の存在 ---

func TestDocsVerifyMissingMapFails(t *testing.T) {
	root := scaffoldDocs(t)
	removeFile(t, root, layout.MapReadme)

	result := executeDocs(t, root)
	if result.code != cli.ExitError {
		t.Fatalf("code = %d, want %d", result.code, cli.ExitError)
	}
	if !strings.Contains(result.out, layout.MapReadme+"(文書の地図)がありません") {
		t.Errorf("理由が出力されない: %q", result.out)
	}
	if !strings.Contains(result.out, "/factory:init を実行") {
		t.Errorf("init の案内が出力されない: %q", result.out)
	}
}

// --- 所有マップの形式 ---

func TestDocsVerifyMissingOwnershipFails(t *testing.T) {
	root := scaffoldDocs(t)
	removeFile(t, root, layout.OwnershipFile)

	result := executeDocs(t, root)
	if result.code != cli.ExitError {
		t.Fatalf("code = %d, want %d", result.code, cli.ExitError)
	}
	if !strings.Contains(result.out, layout.OwnershipFile+"(所有マップ)がありません") {
		t.Errorf("理由が出力されない: %q", result.out)
	}
}

func TestDocsVerifyBrokenYAMLFails(t *testing.T) {
	root := scaffoldDocs(t)
	writeFile(t, root, layout.OwnershipFile, "domains: [broken\n")

	result := executeDocs(t, root)
	if result.code != cli.ExitError {
		t.Fatalf("code = %d, want %d (out=%q)", result.code, cli.ExitError, result.out)
	}
	if !strings.Contains(result.out, "パースできません") {
		t.Errorf("理由が出力されない: %q", result.out)
	}
}

func TestDocsVerifyUnknownFieldFails(t *testing.T) {
	// domains.<name>.paths 以外のキーは形式違反(タイポの検出)。
	root := scaffoldDocs(t)
	writeFile(t, root, layout.OwnershipFile, `domains:
  api:
    path:
      - "src/api/**"
`)

	result := executeDocs(t, root)
	if result.code != cli.ExitError {
		t.Fatalf("code = %d, want %d (out=%q)", result.code, cli.ExitError, result.out)
	}
	if !strings.Contains(result.out, "domains.<name>.paths: [glob]") {
		t.Errorf("期待する形式が出力されない: %q", result.out)
	}
}

func TestDocsVerifyMissingDomainsKeyFails(t *testing.T) {
	root := scaffoldDocs(t)
	writeFile(t, root, layout.OwnershipFile, "# 空の所有マップ\n")

	result := executeDocs(t, root)
	if result.code != cli.ExitError {
		t.Fatalf("code = %d, want %d (out=%q)", result.code, cli.ExitError, result.out)
	}
	if !strings.Contains(result.out, "domains キーがありません") {
		t.Errorf("理由が出力されない: %q", result.out)
	}
}

func TestDocsVerifyEmptyPathsFails(t *testing.T) {
	// paths の無いドメイン宣言は所有の実体が無い(形式違反)。
	root := scaffoldDocs(t)
	writeFile(t, root, layout.OwnershipFile, "domains:\n  api: {}\n")

	result := executeDocs(t, root)
	if result.code != cli.ExitError {
		t.Fatalf("code = %d, want %d (out=%q)", result.code, cli.ExitError, result.out)
	}
	if !strings.Contains(result.out, "ドメイン api に paths がありません") {
		t.Errorf("理由が出力されない: %q", result.out)
	}
}

// --- 必須構造(ドメイン文書)---

func TestDocsVerifyMissingContractsFails(t *testing.T) {
	root := scaffoldDocs(t)
	removeFile(t, root, layout.DomainDoc("api", "contracts.md"))

	result := executeDocs(t, root)
	if result.code != cli.ExitError {
		t.Fatalf("code = %d, want %d", result.code, cli.ExitError)
	}
	if !strings.Contains(result.out, layout.DomainDoc("api", "contracts.md")+" がありません") {
		t.Errorf("理由が出力されない: %q", result.out)
	}
}

func TestDocsVerifyMissingDomainReadmeFails(t *testing.T) {
	root := scaffoldDocs(t)
	removeFile(t, root, layout.DomainDoc("api", "README.md"))

	result := executeDocs(t, root)
	if result.code != cli.ExitError {
		t.Fatalf("code = %d, want %d", result.code, cli.ExitError)
	}
	if !strings.Contains(result.out, layout.DomainDoc("api", "README.md")+" がありません") {
		t.Errorf("理由が出力されない: %q", result.out)
	}
}

// --- 所有マップと実パスの整合 ---

func TestDocsVerifyDeadPatternFails(t *testing.T) {
	// 実在ファイルに 1 件もマッチしない glob は死んだ宣言。
	root := scaffoldDocs(t)
	writeFile(t, root, layout.OwnershipFile, `domains:
  api:
    paths:
      - "src/api/**"
      - "src/ghost/**"
`)

	result := executeDocs(t, root)
	if result.code != cli.ExitError {
		t.Fatalf("code = %d, want %d (out=%q)", result.code, cli.ExitError, result.out)
	}
	if !strings.Contains(result.out, `"src/ghost/**" に一致するファイルがありません`) {
		t.Errorf("理由が出力されない: %q", result.out)
	}
}

func TestDocsVerifyUnownedFileWarnsButPasses(t *testing.T) {
	// どのドメインにも属さないソースファイルは警告のみ(漸進導入のため exit 0)。
	root := scaffoldDocs(t)
	writeFile(t, root, "cmd/main.go", "package main\n")

	result := executeDocs(t, root)
	if result.code != cli.ExitOK {
		t.Fatalf("code = %d, want %d (out=%q)", result.code, cli.ExitOK, result.out)
	}
	if !strings.Contains(result.out, "警告") || !strings.Contains(result.out, "cmd/main.go") {
		t.Errorf("警告が出力されない: %q", result.out)
	}
	if !strings.Contains(result.out, "==> 結果: OK") {
		t.Errorf("警告のみで NG になっている: %q", result.out)
	}
}

func TestDocsVerifyDocsAndDotfilesNotWarned(t *testing.T) {
	// 文書(文書ツリー配下・*.md)と dot 始まりのパスは所有カバレッジの警告対象外。
	root := scaffoldDocs(t)
	writeFile(t, root, "README.md", "# repo\n")
	writeFile(t, root, ".github/workflows/ci.yml", "name: ci\n")

	result := executeDocs(t, root)
	if result.code != cli.ExitOK {
		t.Fatalf("code = %d, want %d (out=%q)", result.code, cli.ExitOK, result.out)
	}
	if strings.Contains(result.out, "警告") {
		t.Errorf("警告対象外のファイルで警告が出ている: %q", result.out)
	}
}

// --- 実行失敗 ---

func TestDocsVerifyMissingRootFails(t *testing.T) {
	result := executeDocs(t, filepath.Join(t.TempDir(), "no-such-dir"))
	if result.code != cli.ExitError {
		t.Fatalf("code = %d, want %d", result.code, cli.ExitError)
	}
	if !strings.Contains(result.err, "root を読めません") {
		t.Errorf("理由が出力されない: %q", result.err)
	}
}
