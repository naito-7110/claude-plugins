package cli

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/naito-7110/claude-plugins/tools/factory/internal/gate"
	"github.com/naito-7110/claude-plugins/tools/factory/internal/verify"
)

// runGate は factory gate(PreToolUse hook の判定)を実行する。
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
	// (.agents/unattended・git コマンド)の基準をプロジェクトルートに固定する
	// (bash ラッパーの cd と同じ。失敗しても bash 版同様に続行する)。
	if dir := os.Getenv("CLAUDE_PROJECT_DIR"); dir != "" {
		_ = os.Chdir(dir)
	}

	input, err := gate.ParseInput(deps.In)
	if err != nil {
		fmt.Fprintf(deps.Err, "factory-gate: %v\n", err)
		return ExitError
	}

	reason, err := gate.Check(input, gate.Deps{
		NewClient:  func() (verify.GraphQL, error) { return deps.NewClient() },
		Repo:       deps.CurrentRepo,
		Branch:     deps.CurrentBranch,
		Managed:    isManaged(*root),
		Unattended: isUnattended(*root),
		Err:        deps.Err,
	})
	if err != nil {
		fmt.Fprintf(deps.Err, "factory-gate: %v\n", err)
		return ExitError
	}
	if reason != "" {
		fmt.Fprintf(deps.Err, "factory-gate: %s\n", reason)
		return ExitBlock
	}
	return ExitOK
}

// isUnattended は無人モードか(.agents/unattended の存在。bash 版の [ -f ] と同一)。
func isUnattended(root string) bool {
	info, err := os.Stat(filepath.Join(root, ".agents", "unattended"))
	return err == nil && !info.IsDir()
}

// isManaged は factory 管理下か(.factory/ ディレクトリの存在。
// bash ラッパーの [ -d .factory ] と同一の判定 — #103)。
func isManaged(root string) bool {
	info, err := os.Stat(filepath.Join(root, ".factory"))
	return err == nil && info.IsDir()
}
