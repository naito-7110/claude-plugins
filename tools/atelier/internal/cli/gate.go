package cli

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/naito-7110/claude-plugins/tools/atelier/internal/gate"
	"github.com/naito-7110/claude-plugins/tools/atelier/internal/verify"
)

// runGate は atelier gate(PreToolUse hook の判定)を実行する。
// stdin から hook JSON を読み、ブロック時は理由を stderr に出して exit 2
// (hook 契約)、通過は exit 0、判定不能な実行失敗は exit 1 を返す。
func runGate(args []string, deps Deps) int {
	fs := flag.NewFlagSet("gate", flag.ContinueOnError)
	fs.SetOutput(deps.Err)
	root := fs.String("root", ".", "リポジトリのルート")
	if err := fs.Parse(args); err != nil {
		return ExitUsage
	}

	// hooks は「現在のディレクトリ」で実行される(公式仕様)。相対パス
	// (.atelier/ の判定と、ブランチ解決のフォールバック基準)をプロジェクト
	// ルートに固定する(bash ラッパーの cd と同じ。失敗しても bash 版同様に
	// 続行する)。ブランチ判定そのものは hook JSON の cwd 基準(#138)。
	if dir := os.Getenv("CLAUDE_PROJECT_DIR"); dir != "" {
		_ = os.Chdir(dir)
	}

	input, err := gate.ParseInput(deps.In)
	if err != nil {
		fmt.Fprintf(deps.Err, "atelier-gate: %v\n", err)
		return ExitError
	}

	reason, err := gate.Check(input, gate.Deps{
		NewClient: func() (verify.GraphQL, error) { return deps.NewClient() },
		Repo:      deps.CurrentRepo,
		// ブランチ解決はディレクトリ指定つき(gate 側がコマンドの実効
		// ディレクトリ — -C / hook cwd — を選んで渡す。#138)。
		Branch:  deps.CurrentBranch,
		Managed: isManaged(*root),
		Err:     deps.Err,
	})
	if err != nil {
		fmt.Fprintf(deps.Err, "atelier-gate: %v\n", err)
		return ExitError
	}
	if reason != "" {
		fmt.Fprintf(deps.Err, "atelier-gate: %s\n", reason)
		return ExitBlock
	}
	return ExitOK
}

// isManaged は atelier 管理下か(.atelier/ ディレクトリの存在。
// bash ラッパーの [ -d .atelier ] と同一の判定 — #103)。
func isManaged(root string) bool {
	info, err := os.Stat(filepath.Join(root, ".atelier"))
	return err == nil && info.IsDir()
}
