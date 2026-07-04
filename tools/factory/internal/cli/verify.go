package cli

import (
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/naito-7110/claude-plugins/tools/factory/internal/verify"
)

func runIssueVerify(args []string, deps Deps) int {
	fs := flag.NewFlagSet("issue verify", flag.ContinueOnError)
	fs.SetOutput(deps.Err)
	number := fs.Int("number", 0, "issue 番号")
	repoFlag := fs.String("repo", "", "対象リポジトリ(owner/name)")
	checksFlag := fs.String("checks", "", "検査項目(カンマ区切り)")
	if err := fs.Parse(args); err != nil {
		return ExitUsage
	}
	if *number == 0 {
		fmt.Fprintln(deps.Err, "--number は必須です")
		fmt.Fprint(deps.Err, usage)
		return ExitUsage
	}
	checks, err := parseChecks(*checksFlag)
	if err != nil {
		fmt.Fprintln(deps.Err, err)
		return ExitUsage
	}
	repo, client, code := resolveRepoAndClient(*repoFlag, deps)
	if code != ExitOK {
		return code
	}

	report, err := verify.Issue(client, repo, *number, checks)
	if err != nil {
		fmt.Fprintln(deps.Err, err)
		return ExitError
	}
	fmt.Fprintf(deps.Out, "==> issue #%d(%s)の検証\n", report.Number, report.Repo)
	printFindings(deps.Out, report.Findings)
	return printResult(deps.Out, report.NGCount())
}

func runPRVerify(args []string, deps Deps) int {
	fs := flag.NewFlagSet("pr verify", flag.ContinueOnError)
	fs.SetOutput(deps.Err)
	number := fs.Int("number", 0, "PR 番号")
	repoFlag := fs.String("repo", "", "対象リポジトリ(owner/name)")
	checksFlag := fs.String("checks", "", "関連 issue への検査項目(カンマ区切り)")
	manifestsFlag := fs.String("dep-manifests", "", "依存マニフェストの glob パターン(カンマ区切り)")
	if err := fs.Parse(args); err != nil {
		return ExitUsage
	}
	if *number == 0 {
		fmt.Fprintln(deps.Err, "--number は必須です")
		fmt.Fprint(deps.Err, usage)
		return ExitUsage
	}
	checks, err := parseChecks(*checksFlag)
	if err != nil {
		fmt.Fprintln(deps.Err, err)
		return ExitUsage
	}
	repo, client, code := resolveRepoAndClient(*repoFlag, deps)
	if code != ExitOK {
		return code
	}

	report, err := verify.PR(client, repo, *number, checks, splitList(*manifestsFlag))
	if err != nil {
		fmt.Fprintln(deps.Err, err)
		return ExitError
	}
	fmt.Fprintf(deps.Out, "==> PR #%d(%s)の検証\n", report.Number, report.Repo)
	printFindings(deps.Out, report.Findings)
	for _, issue := range report.Issues {
		fmt.Fprintf(deps.Out, "==> 関連 issue #%d の検証\n", issue.Number)
		printFindings(deps.Out, issue.Findings)
	}
	return printResult(deps.Out, report.NGCount())
}

// resolveRepoAndClient は --repo(省略時はカレントリポジトリ)と GraphQL クライアントを
// 解決する。失敗時は ExitOK 以外を返す。
func resolveRepoAndClient(repoFlag string, deps Deps) (string, verify.GraphQL, int) {
	repo := repoFlag
	if repo == "" {
		current, err := deps.CurrentRepo()
		if err != nil {
			fmt.Fprintln(deps.Err, "カレントリポジトリを解決できません(--repo <owner/name> で指定してください)")
			return "", nil, ExitUsage
		}
		repo = current
	}
	client, err := deps.NewClient()
	if err != nil {
		fmt.Fprintln(deps.Err, err)
		return "", nil, ExitError
	}
	return repo, client, ExitOK
}

func parseChecks(value string) ([]string, error) {
	if value == "" {
		return verify.AllChecks, nil
	}
	valid := map[string]bool{}
	for _, check := range verify.AllChecks {
		valid[check] = true
	}
	var checks []string
	for _, check := range splitList(value) {
		if !valid[check] {
			return nil, fmt.Errorf("不明な検査項目です: %s(有効: %s)",
				check, strings.Join(verify.AllChecks, ","))
		}
		checks = append(checks, check)
	}
	if len(checks) == 0 {
		return verify.AllChecks, nil
	}
	return checks, nil
}

func splitList(value string) []string {
	var items []string
	for _, item := range strings.Split(value, ",") {
		item = strings.TrimSpace(item)
		if item != "" {
			items = append(items, item)
		}
	}
	return items
}

func printFindings(out io.Writer, findings []verify.Finding) {
	for _, finding := range findings {
		fmt.Fprintf(out, "%s: %s\n", finding.Level, finding.Message)
	}
}

func printResult(out io.Writer, ngCount int) int {
	if ngCount == 0 {
		fmt.Fprintln(out, "==> 結果: OK")
		return ExitOK
	}
	fmt.Fprintf(out, "==> 結果: NG(%d 件)\n", ngCount)
	return ExitError
}
