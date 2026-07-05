package cli

import (
	"flag"
	"fmt"

	"github.com/naito-7110/claude-plugins/tools/factory/internal/mode"
	"github.com/naito-7110/claude-plugins/tools/factory/internal/tick"
)

// runMode は factory mode <verb> を実行する。
// 状態は .agents/ 配下のローカルファイル(コミットしない)で、
// 操作は常にこの入口を経由する(スキルは状態ファイルを直接触らない)。
func runMode(verb string, args []string, deps Deps) int {
	fs := flag.NewFlagSet("mode "+verb, flag.ContinueOnError)
	fs.SetOutput(deps.Err)
	root := fs.String("root", ".", "リポジトリのルート")
	if err := fs.Parse(args); err != nil {
		return ExitUsage
	}

	switch verb {
	case "auto", "manual":
		if err := mode.SetMode(*root, verb); err != nil {
			fmt.Fprintln(deps.Err, err)
			return ExitError
		}
		if verb == mode.Auto {
			fmt.Fprintf(deps.Out, "==> 運転モードを auto にしました(%s)。tick が来れば無人運転します\n", mode.ModeFile)
		} else {
			fmt.Fprintf(deps.Out, "==> 運転モードを manual にしました(%s)。無人運転は行われません\n", mode.ModeFile)
		}
		return ExitOK
	case "status":
		return runModeStatus(*root, deps)
	case "gate":
		return runModeGate(*root, deps)
	default:
		fmt.Fprint(deps.Err, usage)
		return ExitUsage
	}
}

func runModeStatus(root string, deps Deps) int {
	state, err := mode.Load(root)
	if err != nil {
		fmt.Fprintln(deps.Err, err)
		return ExitError
	}
	fmt.Fprintf(deps.Out, "==> 運転状態(root: %s)\n", root)
	if state.Note != "" {
		fmt.Fprintf(deps.Out, "運転モード: %s(%s)\n", state.Mode, state.Note)
	} else {
		fmt.Fprintf(deps.Out, "運転モード: %s\n", state.Mode)
	}
	printTickState(deps)
	if ok, _ := mode.Gate(state); ok {
		fmt.Fprintln(deps.Out, "gate: 通過(unattended 運転は許可されています)")
	} else {
		fmt.Fprintln(deps.Out, "gate: 遮断(unattended 運転は行われません)")
	}
	return ExitOK
}

// printTickState は mode status に tick の設置状態を添える。
// crontab が使えなくても mode status 自体は失敗させない(情報表示に留める)。
func printTickState(deps Deps) {
	if deps.Crontab == nil {
		fmt.Fprintln(deps.Out, "tick: 確認できません(crontab 操作が利用できません)")
		return
	}
	installed, block, err := tick.Status(deps.Crontab)
	switch {
	case err != nil:
		fmt.Fprintf(deps.Out, "tick: 確認できません(%v)\n", err)
	case installed:
		fmt.Fprintf(deps.Out, "tick: 設置済み(%d 行。factory tick status で内容を表示)\n", len(block))
	default:
		fmt.Fprintln(deps.Out, "tick: 未設置(factory tick install で設置できます)")
	}
}

// runModeGate は night が使う判定入口。auto のときだけ exit 0。
// 遮断の理由は stderr に出す(cron ログで拾える)。
func runModeGate(root string, deps Deps) int {
	state, err := mode.Load(root)
	if err != nil {
		fmt.Fprintln(deps.Err, err)
		return ExitError
	}
	ok, reason := mode.Gate(state)
	if !ok {
		fmt.Fprintf(deps.Err, "NG: %s\n", reason)
		return ExitError
	}
	fmt.Fprintln(deps.Out, "OK: 運転モード auto(gate 通過)")
	return ExitOK
}
