package cli

import (
	"flag"
	"fmt"

	"github.com/naito-7110/claude-plugins/tools/atelier/internal/docs"
)

func runDocsVerify(args []string, deps Deps) int {
	fs := flag.NewFlagSet("docs verify", flag.ContinueOnError)
	fs.SetOutput(deps.Err)
	root := fs.String("root", ".", "検証するリポジトリのルート")
	if err := fs.Parse(args); err != nil {
		return ExitUsage
	}

	report, err := docs.Verify(*root)
	if err != nil {
		fmt.Fprintln(deps.Err, err)
		return ExitError
	}
	fmt.Fprintf(deps.Out, "==> 文書構造の検証(root: %s)\n", *root)
	printFindings(deps.Out, report.Findings)
	return printResult(deps.Out, report.NGCount())
}
