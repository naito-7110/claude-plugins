package cli

import (
	"flag"
	"fmt"
	"time"

	"github.com/naito-7110/claude-plugins/tools/atelier/internal/flags"
)

func runFlagsCheck(args []string, deps Deps) int {
	fs := flag.NewFlagSet("flags check", flag.ContinueOnError)
	fs.SetOutput(deps.Err)
	root := fs.String("root", ".", "検証するリポジトリのルート")
	warnDays := fs.Int("warn-days", flags.DefaultWarnDays, "期限接近を警告する日数")
	if err := fs.Parse(args); err != nil {
		return ExitUsage
	}
	if *warnDays < 0 {
		fmt.Fprintln(deps.Err, "--warn-days は 0 以上を指定してください")
		return ExitUsage
	}

	report, err := flags.Check(*root, *warnDays, time.Now())
	if err != nil {
		fmt.Fprintln(deps.Err, err)
		return ExitError
	}
	fmt.Fprintf(deps.Out, "==> フラグレジストリの検証(root: %s)\n", *root)
	printFindings(deps.Out, report.Findings)
	return printResult(deps.Out, report.NGCount())
}
