// Package bootstrapscript は plugins/atelier/hooks/atelier-bootstrap.sh
// (SessionStart hook)の状態検証テストを持つ。スクリプト本体は bash だが、
// 取得・検証・配置の挙動は Go のテストハーネスから実行して固定する
// (ADR 0003: 同梱ピンとの照合成功時のみ配置・失敗は fail-open で無変更)。
package bootstrapscript

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

const pinVersion = "9.9.9"

func scriptPath(t *testing.T) string {
	t.Helper()
	p, err := filepath.Abs(filepath.Join("..", "..", "..", "..", "plugins", "atelier", "hooks", "atelier-bootstrap.sh"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(p); err != nil {
		t.Fatalf("ブートストラップスクリプトが見つかりません: %v", err)
	}
	return p
}

func archiveName() string {
	return fmt.Sprintf("atelier_%s_%s_%s.tar.gz", pinVersion, runtime.GOOS, runtime.GOARCH)
}

// buildArchive は goreleaser 形式(ルート直下に実行ファイル atelier)の
// tar.gz を作り、その中身のバイナリ内容と archive バイト列を返す。
func buildArchive(t *testing.T, binaryContent string) []byte {
	t.Helper()
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)
	data := []byte(binaryContent)
	if err := tw.WriteHeader(&tar.Header{Name: "atelier", Mode: 0o755, Size: int64(len(data))}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write(data); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func sha256hex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// writePin は CLAUDE_PLUGIN_ROOT 相当のディレクトリに pin/version と
// pin/checksums.txt を用意する。
func writePin(t *testing.T, checksum string) string {
	t.Helper()
	root := t.TempDir()
	pinDir := filepath.Join(root, "pin")
	if err := os.MkdirAll(pinDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pinDir, "version"), []byte(pinVersion+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	sums := fmt.Sprintf("%s  %s\n", checksum, archiveName())
	if err := os.WriteFile(filepath.Join(pinDir, "checksums.txt"), []byte(sums), 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}

// newProject は .atelier/ を持つ(= atelier 管理下の)プロジェクトルートを作る。
func newProject(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".atelier"), 0o755); err != nil {
		t.Fatal(err)
	}
	return dir
}

// serveArchive は該当アセット名への GET に archive を返す httptest サーバを立てる。
func serveArchive(t *testing.T, archive []byte) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, archiveName()) {
			_, _ = w.Write(archive)
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(srv.Close)
	return srv
}

// runScript はプロジェクトルート・ピン・取得元 URL を差し替えてスクリプトを実行する。
// extraEnv で追加の環境変数(KEY=VALUE)を注入できる。
func runScript(t *testing.T, projectDir, pluginRoot, baseURL string, extraEnv ...string) (stdout, stderr string, exitCode int) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("bash hook は windows 対象外")
	}
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash がありません")
	}
	if _, err := exec.LookPath("curl"); err != nil {
		t.Skip("curl がありません")
	}
	cmd := exec.Command("bash", scriptPath(t))
	cmd.Env = append(os.Environ(),
		"CLAUDE_PROJECT_DIR="+projectDir,
		"CLAUDE_PLUGIN_ROOT="+pluginRoot,
		"ATELIER_RELEASE_BASE_URL="+baseURL,
	)
	cmd.Env = append(cmd.Env, extraEnv...)
	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf
	err := cmd.Run()
	code := 0
	if exitErr, ok := err.(*exec.ExitError); ok {
		code = exitErr.ExitCode()
	} else if err != nil {
		t.Fatalf("スクリプト実行に失敗: %v", err)
	}
	return outBuf.String(), errBuf.String(), code
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("読めません %s: %v", path, err)
	}
	return string(b)
}

func TestFreshInstall(t *testing.T) {
	archive := buildArchive(t, "binary-v999")
	pluginRoot := writePin(t, sha256hex(archive))
	srv := serveArchive(t, archive)
	project := newProject(t)

	_, stderr, code := runScript(t, project, pluginRoot, srv.URL)

	if code != 0 {
		t.Fatalf("exit=%d stderr=%s", code, stderr)
	}
	bin := filepath.Join(project, ".agents", "bin", "atelier")
	if got := readFile(t, bin); got != "binary-v999" {
		t.Errorf("配置されたバイナリの内容が違います: %q", got)
	}
	info, err := os.Stat(bin)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&0o111 == 0 {
		t.Error("バイナリに実行権がありません")
	}
	marker := filepath.Join(project, ".agents", "bin", "atelier.version")
	if got := strings.TrimSpace(readFile(t, marker)); got != pinVersion {
		t.Errorf("マーカーが違います: %q", got)
	}
}

func TestChecksumMismatchDoesNotInstall(t *testing.T) {
	archive := buildArchive(t, "tampered")
	// ピンには別内容のチェックサムを載せる = アセット差し替えの再現
	pluginRoot := writePin(t, sha256hex([]byte("expected-something-else")))
	srv := serveArchive(t, archive)
	project := newProject(t)

	_, _, code := runScript(t, project, pluginRoot, srv.URL)

	if code != 0 {
		t.Fatalf("ブートストラップ失敗は exit 0 のはず: exit=%d", code)
	}
	if _, err := os.Stat(filepath.Join(project, ".agents", "bin", "atelier")); !os.IsNotExist(err) {
		t.Error("チェックサム不一致なのに配置されています")
	}
	if _, err := os.Stat(filepath.Join(project, ".agents", "bin", "atelier.version")); !os.IsNotExist(err) {
		t.Error("チェックサム不一致なのにマーカーが書かれています")
	}
}

func TestOfflineDoesNothing(t *testing.T) {
	archive := buildArchive(t, "unreachable")
	pluginRoot := writePin(t, sha256hex(archive))
	// 接続不能な取得元(閉じたサーバ)= オフラインの失敗注入
	srv := httptest.NewServer(http.NotFoundHandler())
	url := srv.URL
	srv.Close()
	project := newProject(t)

	_, _, code := runScript(t, project, pluginRoot, url)

	if code != 0 {
		t.Fatalf("オフラインは exit 0 のはず: exit=%d", code)
	}
	if _, err := os.Stat(filepath.Join(project, ".agents", "bin", "atelier")); !os.IsNotExist(err) {
		t.Error("取得できていないのに配置されています")
	}
}

func TestUpToDateKeepsExistingBinary(t *testing.T) {
	archive := buildArchive(t, "newer-content")
	pluginRoot := writePin(t, sha256hex(archive))
	srv := serveArchive(t, archive)
	project := newProject(t)

	binDir := filepath.Join(project, ".agents", "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(binDir, "atelier"), []byte("current-content"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(binDir, "atelier.version"), []byte(pinVersion+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, _, code := runScript(t, project, pluginRoot, srv.URL)

	if code != 0 {
		t.Fatalf("exit=%d", code)
	}
	if got := readFile(t, filepath.Join(binDir, "atelier")); got != "current-content" {
		t.Errorf("ピン一致なのに置換されています: %q", got)
	}
}

func TestVersionSkewReplacesBinary(t *testing.T) {
	archive := buildArchive(t, "pinned-content")
	pluginRoot := writePin(t, sha256hex(archive))
	srv := serveArchive(t, archive)
	project := newProject(t)

	binDir := filepath.Join(project, ".agents", "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(binDir, "atelier"), []byte("old-content"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(binDir, "atelier.version"), []byte("1.0.0\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, _, code := runScript(t, project, pluginRoot, srv.URL)

	if code != 0 {
		t.Fatalf("exit=%d", code)
	}
	if got := readFile(t, filepath.Join(binDir, "atelier")); got != "pinned-content" {
		t.Errorf("スキューが解消されていません: %q", got)
	}
	if got := strings.TrimSpace(readFile(t, filepath.Join(binDir, "atelier.version"))); got != pinVersion {
		t.Errorf("マーカーが更新されていません: %q", got)
	}
}

func TestTimeoutDoesNotInstall(t *testing.T) {
	archive := buildArchive(t, "slow-content")
	pluginRoot := writePin(t, sha256hex(archive))
	// 応答が返らないサーバ = タイムアウトの失敗注入(--max-time はテストシームで短縮)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(3 * time.Second)
		_, _ = w.Write(archive)
	}))
	t.Cleanup(srv.Close)
	project := newProject(t)

	_, _, code := runScript(t, project, pluginRoot, srv.URL, "ATELIER_BOOTSTRAP_MAX_TIME=1")

	if code != 0 {
		t.Fatalf("タイムアウトは exit 0 のはず: exit=%d", code)
	}
	if _, err := os.Stat(filepath.Join(project, ".agents", "bin", "atelier")); !os.IsNotExist(err) {
		t.Error("タイムアウトなのに配置されています")
	}
}

func TestMarkerlessManualBinaryConvergesToPin(t *testing.T) {
	archive := buildArchive(t, "pinned-content")
	pluginRoot := writePin(t, sha256hex(archive))
	srv := serveArchive(t, archive)
	project := newProject(t)

	// マーカー無しの手動配置バイナリ = 不一致として扱い、ピンへ収束させる(ADR 0003)
	binDir := filepath.Join(project, ".agents", "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(binDir, "atelier"), []byte("manually-placed"), 0o755); err != nil {
		t.Fatal(err)
	}

	_, _, code := runScript(t, project, pluginRoot, srv.URL)

	if code != 0 {
		t.Fatalf("exit=%d", code)
	}
	if got := readFile(t, filepath.Join(binDir, "atelier")); got != "pinned-content" {
		t.Errorf("手動配置バイナリがピンへ収束していません: %q", got)
	}
	if got := strings.TrimSpace(readFile(t, filepath.Join(binDir, "atelier.version"))); got != pinVersion {
		t.Errorf("マーカーが書かれていません: %q", got)
	}
}

func TestMarkerMatchButBinaryMissingRefetches(t *testing.T) {
	archive := buildArchive(t, "refetched-content")
	pluginRoot := writePin(t, sha256hex(archive))
	srv := serveArchive(t, archive)
	project := newProject(t)

	// マーカーはピン一致だがバイナリ本体が無い = 再取得されるべき
	binDir := filepath.Join(project, ".agents", "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(binDir, "atelier.version"), []byte(pinVersion+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, _, code := runScript(t, project, pluginRoot, srv.URL)

	if code != 0 {
		t.Fatalf("exit=%d", code)
	}
	if got := readFile(t, filepath.Join(binDir, "atelier")); got != "refetched-content" {
		t.Errorf("バイナリが再取得されていません: %q", got)
	}
}

func TestUnknownAssetInChecksumsDoesNotInstall(t *testing.T) {
	archive := buildArchive(t, "unlisted")
	// ピンの checksums.txt に該当アセット行が無い = 配置しない
	root := t.TempDir()
	pinDir := filepath.Join(root, "pin")
	if err := os.MkdirAll(pinDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pinDir, "version"), []byte(pinVersion+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	sums := fmt.Sprintf("%s  atelier_%s_other_arch.tar.gz\n", sha256hex(archive), pinVersion)
	if err := os.WriteFile(filepath.Join(pinDir, "checksums.txt"), []byte(sums), 0o644); err != nil {
		t.Fatal(err)
	}
	srv := serveArchive(t, archive)
	project := newProject(t)

	_, _, code := runScript(t, project, root, srv.URL)

	if code != 0 {
		t.Fatalf("exit=%d", code)
	}
	if _, err := os.Stat(filepath.Join(project, ".agents", "bin", "atelier")); !os.IsNotExist(err) {
		t.Error("ピンに載っていないアセットが配置されています")
	}
}

func TestCorruptArchiveDoesNotInstall(t *testing.T) {
	// チェックサムは一致するが tar.gz として壊れている = 展開失敗で無変更終了
	corrupt := []byte("this is not a tarball")
	pluginRoot := writePin(t, sha256hex(corrupt))
	srv := serveArchive(t, corrupt)
	project := newProject(t)

	_, _, code := runScript(t, project, pluginRoot, srv.URL)

	if code != 0 {
		t.Fatalf("展開失敗は exit 0 のはず: exit=%d", code)
	}
	if _, err := os.Stat(filepath.Join(project, ".agents", "bin", "atelier")); !os.IsNotExist(err) {
		t.Error("壊れた archive なのに配置されています")
	}
	if _, err := os.Stat(filepath.Join(project, ".agents", "bin", "atelier.version")); !os.IsNotExist(err) {
		t.Error("壊れた archive なのにマーカーが書かれています")
	}
}

func TestSymlinkedAgentsDirIsRefused(t *testing.T) {
	archive := buildArchive(t, "anything")
	pluginRoot := writePin(t, sha256hex(archive))
	srv := serveArchive(t, archive)
	project := newProject(t)

	// commit された .agents symlink 経由でリポジトリ外へ書かされる経路の遮断
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(project, ".agents")); err != nil {
		t.Fatal(err)
	}

	_, _, code := runScript(t, project, pluginRoot, srv.URL)

	if code != 0 {
		t.Fatalf("exit=%d", code)
	}
	entries, err := os.ReadDir(outside)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Errorf("symlink 先に書き込まれています: %v", entries)
	}
}

func TestUnmanagedRepoIsNoop(t *testing.T) {
	archive := buildArchive(t, "anything")
	pluginRoot := writePin(t, sha256hex(archive))
	srv := serveArchive(t, archive)
	project := t.TempDir() // .atelier/ なし = 管理外

	_, _, code := runScript(t, project, pluginRoot, srv.URL)

	if code != 0 {
		t.Fatalf("管理外は exit 0 のはず: exit=%d", code)
	}
	if _, err := os.Stat(filepath.Join(project, ".agents")); !os.IsNotExist(err) {
		t.Error("管理外リポジトリに書き込んでいます")
	}
}
