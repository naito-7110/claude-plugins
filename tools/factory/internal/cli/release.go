package cli

import (
	"flag"
	"fmt"
	"strings"

	"github.com/naito-7110/claude-plugins/tools/factory/internal/release"
)

// runRelease は factory release <tag> を実行する。
// タグは positional 引数(フラグの前後どちらでも受ける)。
func runRelease(args []string, deps Deps) int {
	// 先頭が非フラグならタグとして取り出す(release <tag> --dry-run の形)。
	tag := ""
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		tag = args[0]
		args = args[1:]
	}
	fs := flag.NewFlagSet("release", flag.ContinueOnError)
	fs.SetOutput(deps.Err)
	remote := fs.String("remote", release.DefaultRemote, "push 先のリモート")
	ref := fs.String("ref", "", "タグを打つブランチ(既定: リモートの default branch)")
	dryRun := fs.Bool("dry-run", false, "検証と対象の表示のみ(変更しない)")
	if err := fs.Parse(args); err != nil {
		return ExitUsage
	}
	// release --dry-run <tag> の形(フラグが先)にも対応する。
	if tag == "" {
		tag = fs.Arg(0)
	}
	if err := release.ValidateTag(tag); err != nil {
		fmt.Fprintln(deps.Err, err)
		fmt.Fprint(deps.Err, usage)
		return ExitUsage
	}
	if deps.ReleaseGit == nil {
		fmt.Fprintln(deps.Err, "git 操作が利用できません")
		return ExitError
	}

	opts := release.Options{Tag: tag, Remote: *remote, Ref: *ref, DryRun: *dryRun}
	if err := release.Run(deps.ReleaseGit, opts, deps.Out); err != nil {
		fmt.Fprintln(deps.Err, "NG:", err)
		return ExitError
	}
	return ExitOK
}
