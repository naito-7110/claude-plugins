// Package release はリリースタグ(= 本番デプロイの引き金)を安全に打つ操作を提供する。
//
// merge-policy: デプロイは常に人間の tag push。生の git 2 行での運用は
// 「ローカル作業ブランチの HEAD にタグを打つ」実事故を起こした(2026-07-05。
// 実害なし・付け直しで回収)。人間の操作こそ bin で安全化する。
//
// 事故防止の設計:
//   - **ローカル HEAD を一切見ない**: 対象 SHA は常に fetch 直後の
//     <remote>/<ref> から解決する。Git interface に HEAD を読む操作が
//     存在しないため、実装ミスでもローカル HEAD に打ちようがない(構造的防止)
//   - **既存タグへの上書き不可**: ローカル・リモートのどちらかに同名タグが
//     あれば理由を出して失敗する。force は提供しない
//
// git 操作はプロセス境界であり、Git interface で抽象化する。
// テストでは fake を注入する(既存流儀)。
package release

import (
	"fmt"
	"io"
	"os/exec"
	"strings"
)

// Git は release が使う git 操作(プロセス境界)。
// 意図的にローカル HEAD・ローカルブランチを読む操作を持たない。
type Git interface {
	// Fetch は remote から ref とタグを取得する。
	Fetch(remote string) error
	// DefaultBranch は remote の default branch 名を返す。
	DefaultBranch(remote string) (string, error)
	// ResolveRemoteRef は <remote>/<ref> の SHA を返す(fetch 済み前提)。
	ResolveRemoteRef(remote, ref string) (string, error)
	// LocalTagExists はローカルに tag が存在するか。
	LocalTagExists(tag string) (bool, error)
	// RemoteTagExists は remote に tag が存在するか。
	RemoteTagExists(remote, tag string) (bool, error)
	// CreateTag は sha に tag を作成する(上書きしない)。
	CreateTag(tag, sha string) error
	// PushTag は tag を remote へ push する(force しない)。
	PushTag(remote, tag string) error
}

// DefaultRemote は既定のリモート名。
const DefaultRemote = "origin"

// Options は Run の入力。
type Options struct {
	Tag    string
	Remote string // 空なら DefaultRemote
	Ref    string // 空ならリモートの default branch
	DryRun bool
}

// ValidateTag はタグ名の軽い検証(空でない・空白なし・"-" 始まりでない)。
// 意味的な検証(semver 等)はしない — タグ名は人間が指定し、責任も人間が持つ。
func ValidateTag(tag string) error {
	if tag == "" {
		return fmt.Errorf("タグ名を指定してください")
	}
	if strings.ContainsAny(tag, " \t\n") {
		return fmt.Errorf("タグ名に空白を含められません: %q", tag)
	}
	if strings.HasPrefix(tag, "-") {
		return fmt.Errorf("タグ名を - で始められません: %q", tag)
	}
	return nil
}

// Run はリリースタグを安全に作成して push する。
// 対象 SHA は常に <remote>/<ref>(既定: リモートの default branch)から解決し、
// ローカル HEAD は一切見ない。DryRun のときは検証と対象の表示だけ行い、変更しない。
func Run(git Git, opts Options, out io.Writer) error {
	if err := ValidateTag(opts.Tag); err != nil {
		return err
	}
	remote := opts.Remote
	if remote == "" {
		remote = DefaultRemote
	}

	// 1. fetch(タグ含む)— リモートの実態だけを判断材料にする。
	if err := git.Fetch(remote); err != nil {
		return fmt.Errorf("%s を fetch できません: %w", remote, err)
	}

	// 2. 対象 SHA を <remote>/<ref> から解決する(ローカル HEAD は見ない)。
	ref := opts.Ref
	refNote := ""
	if ref == "" {
		defaultBranch, err := git.DefaultBranch(remote)
		if err != nil {
			return fmt.Errorf("%s の default branch を解決できません(--ref で指定できます): %w", remote, err)
		}
		ref = defaultBranch
		refNote = "(リモートの default branch)"
	}
	sha, err := git.ResolveRemoteRef(remote, ref)
	if err != nil {
		return fmt.Errorf("%s/%s を解決できません: %w", remote, ref, err)
	}

	// 3. 既存タグの確認(ローカル・リモートの両方)。上書き・force は提供しない。
	if exists, err := git.LocalTagExists(opts.Tag); err != nil {
		return fmt.Errorf("ローカルタグを確認できません: %w", err)
	} else if exists {
		return fmt.Errorf("タグ %s はローカルに既に存在します(上書きは提供しません。別のタグ名を使うか、誤タグなら手で削除してから再実行してください)", opts.Tag)
	}
	if exists, err := git.RemoteTagExists(remote, opts.Tag); err != nil {
		return fmt.Errorf("リモートタグを確認できません: %w", err)
	} else if exists {
		return fmt.Errorf("タグ %s は %s に既に存在します(上書きは提供しません。リリース済みのタグは動かさず、次のバージョンを打ってください)", opts.Tag, remote)
	}

	fmt.Fprintf(out, "==> 対象: %s/%s = %s%s\n", remote, ref, sha, refNote)
	fmt.Fprintf(out, "==> タグ: %s(ローカル・%s ともに未使用)\n", opts.Tag, remote)

	if opts.DryRun {
		fmt.Fprintln(out, "==> dry-run: 変更しません。実行すると上記 SHA にタグを作成して push します")
		return nil
	}

	// 4. タグ作成 → push(どちらも force なし)。
	if err := git.CreateTag(opts.Tag, sha); err != nil {
		return fmt.Errorf("タグを作成できません: %w", err)
	}
	if err := git.PushTag(remote, opts.Tag); err != nil {
		return fmt.Errorf("タグを push できません(ローカルには作成済み。原因を解消して git push %s refs/tags/%s で再送できます): %w",
			remote, opts.Tag, err)
	}
	fmt.Fprintf(out, "==> タグ %s を %s に作成し、%s へ push しました\n", opts.Tag, sha, remote)
	if strings.HasPrefix(opts.Tag, "factory/v") {
		fmt.Fprintln(out, "==> factory/v* のタグのため、リリース workflow(factory-release)が起動します")
	}
	return nil
}

// SystemGit は実 git を使う Git 実装(カレントディレクトリのリポジトリを操作する)。
type SystemGit struct{}

func (SystemGit) git(args ...string) (string, error) {
	out, err := exec.Command("git", args...).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git %s: %v(%s)", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return strings.TrimSpace(string(out)), nil
}

// Fetch は Git を満たす。
func (s SystemGit) Fetch(remote string) error {
	_, err := s.git("fetch", "--tags", remote)
	return err
}

// DefaultBranch は Git を満たす。ローカルの origin/HEAD が未設定でも
// ls-remote --symref で必ずリモート側から解決できる。
func (s SystemGit) DefaultBranch(remote string) (string, error) {
	if out, err := s.git("symbolic-ref", "--short", "refs/remotes/"+remote+"/HEAD"); err == nil {
		return strings.TrimPrefix(out, remote+"/"), nil
	}
	out, err := s.git("ls-remote", "--symref", remote, "HEAD")
	if err != nil {
		return "", err
	}
	for _, line := range strings.Split(out, "\n") {
		if rest, found := strings.CutPrefix(line, "ref: refs/heads/"); found {
			return strings.Fields(rest)[0], nil
		}
	}
	return "", fmt.Errorf("%s の default branch を特定できません", remote)
}

// ResolveRemoteRef は Git を満たす。
func (s SystemGit) ResolveRemoteRef(remote, ref string) (string, error) {
	return s.git("rev-parse", "--verify", "refs/remotes/"+remote+"/"+ref)
}

// LocalTagExists は Git を満たす。
func (s SystemGit) LocalTagExists(tag string) (bool, error) {
	out, err := s.git("tag", "--list", tag)
	if err != nil {
		return false, err
	}
	return out != "", nil
}

// RemoteTagExists は Git を満たす。
func (s SystemGit) RemoteTagExists(remote, tag string) (bool, error) {
	out, err := s.git("ls-remote", "--tags", remote, "refs/tags/"+tag)
	if err != nil {
		return false, err
	}
	return out != "", nil
}

// CreateTag は Git を満たす(force なし — 既存タグがあれば git 自体が失敗する)。
func (s SystemGit) CreateTag(tag, sha string) error {
	_, err := s.git("tag", tag, sha)
	return err
}

// PushTag は Git を満たす(force なし)。
func (s SystemGit) PushTag(remote, tag string) error {
	_, err := s.git("push", remote, "refs/tags/"+tag)
	return err
}
