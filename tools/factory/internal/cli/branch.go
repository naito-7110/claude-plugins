package cli

import (
	"flag"
	"fmt"

	"github.com/naito-7110/claude-plugins/tools/factory/internal/branch"
)

func runBranchCleanup(args []string, deps Deps) int {
	fs := flag.NewFlagSet("branch cleanup", flag.ContinueOnError)
	fs.SetOutput(deps.Err)
	dryRun := fs.Bool("dry-run", false, "削除せず対象の一覧だけ表示する")
	root := fs.String("root", ".", "リポジトリのルート")
	repoFlag := fs.String("repo", "", "対象リポジトリ(owner/name)")
	if err := fs.Parse(args); err != nil {
		return ExitUsage
	}
	repo, client, code := resolveRepoAndClient(*repoFlag, deps)
	if code != ExitOK {
		return code
	}

	report, err := branch.Cleanup(client, repo, *root, *dryRun)
	if err != nil {
		fmt.Fprintln(deps.Err, err)
		return ExitError
	}
	note := ""
	if *dryRun {
		note = "、dry-run: 変更しません"
	}
	fmt.Fprintf(deps.Out, "==> agent ブランチの掃除(root: %s%s)\n", *root, note)
	printFindings(deps.Out, report.Findings)
	return printResult(deps.Out, report.NGCount())
}
