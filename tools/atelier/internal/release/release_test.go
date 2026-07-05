package release_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/naito-7110/claude-plugins/tools/atelier/internal/release"
)

// fakeGit は Git のインメモリ fake。最終状態(CreatedTags / PushedTags)を
// アサートする状態検証に使う。ローカル HEAD という概念を持たない
// (release.Git interface に HEAD を読む操作が無い — 事故の構造的防止)。
type fakeGit struct {
	Default     string            // default branch 名
	RemoteRefs  map[string]string // "origin/main" -> SHA(リモートの実態)
	LocalTags   map[string]bool
	RemoteTags  map[string]bool
	CreatedTags map[string]string // tag -> SHA
	PushedTags  []string
	Fetched     []string
	FetchErr    error
}

func newFakeGit() *fakeGit {
	return &fakeGit{
		Default:     "main",
		RemoteRefs:  map[string]string{"origin/main": "abc1234deadbeef"},
		LocalTags:   map[string]bool{},
		RemoteTags:  map[string]bool{},
		CreatedTags: map[string]string{},
	}
}

func (f *fakeGit) Fetch(remote string) error {
	f.Fetched = append(f.Fetched, remote)
	return f.FetchErr
}

func (f *fakeGit) DefaultBranch(remote string) (string, error) {
	if f.Default == "" {
		return "", errors.New("default branch なし")
	}
	return f.Default, nil
}

func (f *fakeGit) ResolveRemoteRef(remote, ref string) (string, error) {
	sha, ok := f.RemoteRefs[remote+"/"+ref]
	if !ok {
		return "", errors.New("unknown revision")
	}
	return sha, nil
}

func (f *fakeGit) LocalTagExists(tag string) (bool, error)     { return f.LocalTags[tag], nil }
func (f *fakeGit) RemoteTagExists(_, tag string) (bool, error) { return f.RemoteTags[tag], nil }
func (f *fakeGit) CreateTag(tag, sha string) (err error)       { f.CreatedTags[tag] = sha; return nil }
func (f *fakeGit) PushTag(remote, tag string) error {
	f.PushedTags = append(f.PushedTags, remote+"/"+tag)
	return nil
}

func run(t *testing.T, git *fakeGit, opts release.Options) (string, error) {
	t.Helper()
	var out strings.Builder
	err := release.Run(git, opts, &out)
	return out.String(), err
}

// --- 正常系 ---

func TestRunTagsRemoteDefaultBranchSHA(t *testing.T) {
	git := newFakeGit()

	out, err := run(t, git, release.Options{Tag: "atelier/v0.3.0"})
	if err != nil {
		t.Fatal(err)
	}
	// AC 1: 対象 SHA は常にリモートの実態(RemoteRefs)から解決される。
	// fake はローカル HEAD を持たないため、ローカルの状態は結果に影響しようがない。
	if git.CreatedTags["atelier/v0.3.0"] != "abc1234deadbeef" {
		t.Errorf("CreatedTags = %v(origin/main の SHA に打たれるべき)", git.CreatedTags)
	}
	if len(git.PushedTags) != 1 || git.PushedTags[0] != "origin/atelier/v0.3.0" {
		t.Errorf("PushedTags = %v", git.PushedTags)
	}
	if len(git.Fetched) != 1 || git.Fetched[0] != "origin" {
		t.Errorf("fetch されていない: %v", git.Fetched)
	}
	for _, want := range []string{
		"対象: origin/main = abc1234deadbeef(リモートの default branch)",
		"タグ atelier/v0.3.0 を abc1234deadbeef に作成し、origin へ push しました",
		"リリース workflow(atelier-release)が起動します",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("出力に %q がない: %q", want, out)
		}
	}
}

func TestRunExplicitRefAndRemote(t *testing.T) {
	git := newFakeGit()
	git.RemoteRefs["upstream/release-1.x"] = "fedcba987654"

	_, err := run(t, git, release.Options{Tag: "v1.2.3", Remote: "upstream", Ref: "release-1.x"})
	if err != nil {
		t.Fatal(err)
	}
	if git.CreatedTags["v1.2.3"] != "fedcba987654" {
		t.Errorf("CreatedTags = %v", git.CreatedTags)
	}
	if len(git.PushedTags) != 1 || git.PushedTags[0] != "upstream/v1.2.3" {
		t.Errorf("PushedTags = %v", git.PushedTags)
	}
}

func TestRunNonAtelierTagOmitsWorkflowHint(t *testing.T) {
	git := newFakeGit()

	out, err := run(t, git, release.Options{Tag: "v1.0.0"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "atelier-release") {
		t.Errorf("atelier/v* でないのに workflow ヒントが出ている: %q", out)
	}
}

// --- 既存タグ(AC 2)---

func TestRunExistingLocalTagFails(t *testing.T) {
	git := newFakeGit()
	git.LocalTags["atelier/v0.3.0"] = true

	_, err := run(t, git, release.Options{Tag: "atelier/v0.3.0"})
	if err == nil || !strings.Contains(err.Error(), "ローカルに既に存在します") {
		t.Fatalf("err = %v", err)
	}
	if len(git.CreatedTags) != 0 || len(git.PushedTags) != 0 {
		t.Errorf("失敗時に変更がある: created=%v pushed=%v", git.CreatedTags, git.PushedTags)
	}
}

func TestRunExistingRemoteTagFails(t *testing.T) {
	git := newFakeGit()
	git.RemoteTags["atelier/v0.3.0"] = true

	_, err := run(t, git, release.Options{Tag: "atelier/v0.3.0"})
	if err == nil || !strings.Contains(err.Error(), "origin に既に存在します") {
		t.Fatalf("err = %v", err)
	}
	if len(git.CreatedTags) != 0 || len(git.PushedTags) != 0 {
		t.Errorf("失敗時に変更がある: created=%v pushed=%v", git.CreatedTags, git.PushedTags)
	}
}

// --- dry-run(AC 3)---

func TestRunDryRunChangesNothing(t *testing.T) {
	git := newFakeGit()

	out, err := run(t, git, release.Options{Tag: "atelier/v0.3.0", DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(git.CreatedTags) != 0 || len(git.PushedTags) != 0 {
		t.Errorf("dry-run で変更がある: created=%v pushed=%v", git.CreatedTags, git.PushedTags)
	}
	for _, want := range []string{
		"対象: origin/main = abc1234deadbeef",
		"dry-run: 変更しません",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("出力に %q がない: %q", want, out)
		}
	}
}

func TestRunDryRunStillDetectsExistingTag(t *testing.T) {
	// dry-run でも既存タグの検査(手順 3)までは実行される。
	git := newFakeGit()
	git.RemoteTags["atelier/v0.3.0"] = true

	_, err := run(t, git, release.Options{Tag: "atelier/v0.3.0", DryRun: true})
	if err == nil || !strings.Contains(err.Error(), "既に存在します") {
		t.Fatalf("err = %v", err)
	}
}

// --- タグ形式・解決失敗 ---

func TestValidateTag(t *testing.T) {
	tests := []struct {
		tag string
		ok  bool
	}{
		{"atelier/v0.3.0", true},
		{"v1.0.0", true},
		{"", false},
		{"v1 0", false},
		{"v1\t0", false},
		{"-v1", false},
	}
	for _, tt := range tests {
		err := release.ValidateTag(tt.tag)
		if (err == nil) != tt.ok {
			t.Errorf("ValidateTag(%q) = %v, want ok=%v", tt.tag, err, tt.ok)
		}
	}
}

func TestRunInvalidTagFailsBeforeFetch(t *testing.T) {
	git := newFakeGit()

	_, err := run(t, git, release.Options{Tag: "bad tag"})
	if err == nil {
		t.Fatal("空白入りタグが通っている")
	}
	if len(git.Fetched) != 0 {
		t.Errorf("不正なタグで fetch している: %v", git.Fetched)
	}
}

func TestRunUnknownRefFails(t *testing.T) {
	git := newFakeGit()

	_, err := run(t, git, release.Options{Tag: "v1.0.0", Ref: "no-such-branch"})
	if err == nil || !strings.Contains(err.Error(), "origin/no-such-branch を解決できません") {
		t.Fatalf("err = %v", err)
	}
	if len(git.CreatedTags) != 0 {
		t.Errorf("解決失敗なのにタグが作られている: %v", git.CreatedTags)
	}
}

func TestRunFetchFailureFails(t *testing.T) {
	git := newFakeGit()
	git.FetchErr = errors.New("network down")

	_, err := run(t, git, release.Options{Tag: "v1.0.0"})
	if err == nil || !strings.Contains(err.Error(), "fetch できません") {
		t.Fatalf("err = %v", err)
	}
	if len(git.CreatedTags) != 0 || len(git.PushedTags) != 0 {
		t.Errorf("fetch 失敗で変更がある: %v %v", git.CreatedTags, git.PushedTags)
	}
}
