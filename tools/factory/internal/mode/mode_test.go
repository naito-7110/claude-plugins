package mode_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/naito-7110/claude-plugins/tools/factory/internal/mode"
)

func load(t *testing.T, root string) mode.State {
	t.Helper()
	state, err := mode.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	return state
}

// --- fail-closed: 既定は manual ---

func TestLoadMissingStateIsManual(t *testing.T) {
	state := load(t, t.TempDir())
	if state.Mode != mode.Manual {
		t.Errorf("Mode = %q, want manual(fail-closed)", state.Mode)
	}
	if !strings.Contains(state.Note, "既定 = manual") {
		t.Errorf("Note = %q", state.Note)
	}
}

func TestLoadInvalidContentIsManual(t *testing.T) {
	root := t.TempDir()
	if err := mode.SetMode(root, mode.Auto); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(root, filepath.FromSlash(mode.ModeFile))
	if err := os.WriteFile(p, []byte("turbo\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	state := load(t, root)
	if state.Mode != mode.Manual {
		t.Errorf("Mode = %q, want manual(不正内容は fail-closed)", state.Mode)
	}
	if !strings.Contains(state.Note, `"turbo" が不正です`) {
		t.Errorf("Note = %q", state.Note)
	}
}

// --- 遷移 ---

func TestSetModeRoundTrip(t *testing.T) {
	root := t.TempDir()
	if err := mode.SetMode(root, mode.Auto); err != nil {
		t.Fatal(err)
	}
	if state := load(t, root); state.Mode != mode.Auto || state.Note != "" {
		t.Errorf("state = %+v, want auto(明示設定)", state)
	}
	if err := mode.SetMode(root, mode.Manual); err != nil {
		t.Fatal(err)
	}
	if state := load(t, root); state.Mode != mode.Manual || state.Note != "" {
		t.Errorf("state = %+v, want manual(明示設定)", state)
	}
}

func TestSetModeRejectsUnknownValue(t *testing.T) {
	if err := mode.SetMode(t.TempDir(), "turbo"); err == nil {
		t.Error("不正な値が受理されている")
	}
}

// --- gate 判定(auto / manual の二値)---

func TestGateAllowsAuto(t *testing.T) {
	ok, reason := mode.Gate(mode.State{Mode: mode.Auto})
	if !ok || reason != "" {
		t.Errorf("ok = %v, reason = %q", ok, reason)
	}
}

func TestGateBlocksManual(t *testing.T) {
	ok, reason := mode.Gate(mode.State{Mode: mode.Manual, Note: "状態ファイルなし"})
	if ok {
		t.Fatal("manual で gate 通過")
	}
	if !strings.Contains(reason, "運転モードが manual です") || !strings.Contains(reason, "状態ファイルなし") {
		t.Errorf("reason = %q", reason)
	}
}

// --- 状態ファイルの配置(.agents/ 配下)---

func TestStateFilesLiveUnderAgentsDir(t *testing.T) {
	root := t.TempDir()
	if err := mode.SetMode(root, mode.Auto); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(mode.ModeFile, mode.Dir+"/") {
		t.Errorf("%s が %s/ 配下にない", mode.ModeFile, mode.Dir)
	}
	if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(mode.ModeFile))); err != nil {
		t.Errorf("%s が作成されていない: %v", mode.ModeFile, err)
	}
}
