package cli

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/naito-7110/claude-plugins/tools/factory/internal/tick"
)

// runTick は factory tick <verb> を実行する。crontab のマーカーブロック
// (# factory-tick begin / end)だけを冪等に操作し、他の行には触れない。
func runTick(verb string, args []string, deps Deps) int {
	fs := flag.NewFlagSet("tick "+verb, flag.ContinueOnError)
	fs.SetOutput(deps.Err)
	root := fs.String("root", ".", "リポジトリのルート")
	schedule := fs.String("schedule", tick.DefaultSchedule, "起動スケジュール(cron 式)")
	if err := fs.Parse(args); err != nil {
		return ExitUsage
	}
	if deps.Crontab == nil {
		fmt.Fprintln(deps.Err, "crontab 操作が利用できません")
		return ExitError
	}

	switch verb {
	case "install":
		return runTickInstall(*root, *schedule, deps)
	case "remove":
		removed, err := tick.Remove(deps.Crontab)
		if err != nil {
			fmt.Fprintln(deps.Err, err)
			return ExitError
		}
		if removed {
			fmt.Fprintln(deps.Out, "==> tick を除去しました(マーカーブロック以外の行には触れていません)")
		} else {
			fmt.Fprintln(deps.Out, "==> tick は未設置です(変更なし)")
		}
		return ExitOK
	case "status":
		installed, block, err := tick.Status(deps.Crontab)
		if err != nil {
			fmt.Fprintln(deps.Err, err)
			return ExitError
		}
		if !installed {
			fmt.Fprintln(deps.Out, "==> tick: 未設置(factory tick install で設置できます)")
			return ExitOK
		}
		fmt.Fprintln(deps.Out, "==> tick: 設置済み")
		for _, line := range block {
			fmt.Fprintf(deps.Out, "  %s\n", line)
		}
		return ExitOK
	default:
		fmt.Fprint(deps.Err, usage)
		return ExitUsage
	}
}

func runTickInstall(root, schedule string, deps Deps) int {
	// cron 式の問題は入力の誤り(usage)として先に弾く。
	if err := tick.ValidateSchedule(schedule); err != nil {
		fmt.Fprintln(deps.Err, err)
		return ExitUsage
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		fmt.Fprintln(deps.Err, err)
		return ExitError
	}
	info, err := os.Stat(abs)
	if err != nil || !info.IsDir() {
		fmt.Fprintf(deps.Err, "root がディレクトリとして読めません: %s\n", abs)
		return ExitError
	}

	replaced, err := tick.Install(deps.Crontab, abs, schedule)
	if err != nil {
		fmt.Fprintln(deps.Err, err)
		return ExitError
	}
	if replaced {
		fmt.Fprintln(deps.Out, "==> tick を置換しました(既存のマーカーブロックを更新)")
	} else {
		fmt.Fprintln(deps.Out, "==> tick を設置しました")
	}
	fmt.Fprintf(deps.Out, "  %s\n", tick.Line(abs, schedule))
	fmt.Fprintln(deps.Out, "==> 走ってよいかは運転モードが決めます(factory mode auto で無人運転を許可。既定は manual)")
	return ExitOK
}
