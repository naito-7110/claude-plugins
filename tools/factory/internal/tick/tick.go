// Package tick は unattended 運転の起動機構(cron の tick)を提供する。
//
// tick は入れっぱなしでよい(走ってよいかは factory mode が決める — night
// スキル参照)。責務は 2 つ:
//
//  1. crontab のマーカーブロック(# factory-tick begin / end)の設置・置換・
//     除去。**ブロック外の行には一切触れない**。マーカーが壊れている
//     (begin / end が対応しない)場合は何も書き込まずにエラーを返す
//     (fail-closed: 他の行を壊すくらいなら止まる)
//  2. tick run — 多重起動ロックの下で claude を 1 回起動する実行入口。
//     ロックは Go 実装(unix: flock(2) / windows: LockFileEx)で行う。
//     **flock コマンドは util-linux 由来で macOS に存在しない**ため、
//     生成する cron 行は OS コマンドに依存させない(#97)。
//     ロック取得失敗 = 他 tick が実行中の正常系で、exit 0 で即終了する
//
// crontab コマンドと claude の起動はプロセス境界であり、それぞれ Crontab /
// Exec interface で抽象化する。テストでは fake を注入し、実環境に触れない。
package tick

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/naito-7110/claude-plugins/tools/factory/internal/mode"
)

// Crontab は crontab コマンド(プロセス境界)。
type Crontab interface {
	// Read は現在の crontab を返す(未設定なら空文字列)。
	Read() (string, error)
	// Write は crontab 全体を置き換える。
	Write(content string) error
}

// マーカーとスケジュールの既定(単一定義)。
const (
	// MarkerBegin / MarkerEnd は factory が管理するブロックの境界。
	MarkerBegin = "# factory-tick begin"
	MarkerEnd   = "# factory-tick end"
	// DefaultSchedule は既定の起動スケジュール(平日 3:00)。
	DefaultSchedule = "0 3 * * 1-5"
	// DefaultPrompt は tick run が起動する既定のスキル。
	DefaultPrompt = "/factory:night"
)

// promptName は prompt からロック・ログのファイル名を導出する
// ("/factory:night" → "night"、"/factory:review" → "review")。
func promptName(prompt string) string {
	name := strings.ToLower(strings.TrimSpace(prompt[strings.LastIndex(prompt, ":")+1:]))
	var b strings.Builder
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			b.WriteRune(r)
		}
	}
	if b.Len() == 0 {
		return "tick"
	}
	return b.String()
}

// LockFile は prompt に対応するロックファイルの相対パス(.agents/ 配下)。
func LockFile(prompt string) string { return ".agents/" + promptName(prompt) + ".lock" }

// LogFile は prompt に対応するログファイルの相対パス(.agents/ 配下)。
func LogFile(prompt string) string { return ".agents/" + promptName(prompt) + ".log" }

// Line は tick の起動行を組み立てる。root はリポジトリの絶対パス、
// factoryPath は factory バイナリの絶対パス(cron の PATH は最小のため)。
// 多重起動防止は factory tick run が内蔵する(flock コマンドを使わない)。
func Line(root, schedule, factoryPath string) string {
	return fmt.Sprintf(`%s cd %s && %s tick run >> %s 2>&1`,
		schedule, root, factoryPath, LogFile(DefaultPrompt))
}

// ValidateSchedule は cron 式の形式(5 フィールドまたは @ 記法)を軽く検査する。
// 各フィールドの意味の検証は cron 側の責務(ここでは行の破壊だけを防ぐ)。
func ValidateSchedule(schedule string) error {
	schedule = strings.TrimSpace(schedule)
	if strings.HasPrefix(schedule, "@") && len(strings.Fields(schedule)) == 1 {
		return nil
	}
	if len(strings.Fields(schedule)) != 5 {
		return fmt.Errorf("cron 式は 5 フィールド(分 時 日 月 曜日)または @ 記法で指定してください: %q", schedule)
	}
	if strings.Contains(schedule, "\n") {
		return fmt.Errorf("cron 式に改行を含められません")
	}
	return nil
}

// Install は crontab のマーカーブロックを設置する(既存があれば置換)。
// ブロック外の行は変更しない。replaced は既存ブロックを置換したとき true。
// 旧形式(flock コマンド)のブロックもマーカーごと置換される(冪等)。
func Install(c Crontab, root, schedule, factoryPath string) (replaced bool, err error) {
	if err := ValidateSchedule(schedule); err != nil {
		return false, err
	}
	content, err := c.Read()
	if err != nil {
		return false, fmt.Errorf("crontab を読めません: %w", err)
	}
	rest, blocks, err := splitBlocks(content)
	if err != nil {
		return false, err
	}
	lines := append(rest, MarkerBegin, Line(root, strings.TrimSpace(schedule), factoryPath), MarkerEnd)
	if err := c.Write(join(lines)); err != nil {
		return false, fmt.Errorf("crontab を書き込めません: %w", err)
	}
	return len(blocks) > 0, nil
}

// --- tick run(多重起動ロック付きの実行入口)---

// Exec は claude の起動(プロセス境界)。テストでは fake を注入する。
type Exec interface {
	// Run は dir をカレントディレクトリに name を起動し、終了コードを返す。
	// error は起動自体の失敗(コマンドが無い等)のみ。
	Run(dir, name string, args ...string) (int, error)
}

// SystemExec は実プロセスを起動する Exec 実装。
type SystemExec struct {
	Stdout io.Writer
	Stderr io.Writer
}

// Run は Exec を満たす。
func (s SystemExec) Run(dir, name string, args ...string) (int, error) {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	cmd.Stdout = s.Stdout
	cmd.Stderr = s.Stderr
	err := cmd.Run()
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode(), nil
	}
	if err != nil {
		return 1, err
	}
	return 0, nil
}

// Run は運転モードと多重起動ロックを確認した上で claude -p <prompt> を
// 1 回起動し、その終了コードを引き継ぐ。次の 2 つはどちらも**正常系**で、
// 1 行出力して 0 を返す(cron からの起動をエラーにしない):
//
//   - mode gate 不通過(manual 等): claude を起動する前に mode を内部関数で
//     確認する。night スキルの手順 0 でも mode gate を見るが、そこまで進むと
//     claude セッションが 1 本立ってしまう(15 分 tick なら日に 96 回)。
//     サブスク枠の浪費を止めるため、起動前にここで落とす(night 側の確認は
//     defense-in-depth として残る)
//   - ロック取得失敗: 他 tick が実行中(多重起動防止)
func Run(launcher Exec, root, prompt string, out io.Writer) (int, error) {
	if strings.TrimSpace(prompt) == "" {
		prompt = DefaultPrompt
	}

	state, err := mode.Load(root)
	if err != nil {
		return 1, err
	}
	if ok, reason := mode.Gate(state); !ok {
		fmt.Fprintf(out, "==> mode gate によりスキップします(%s)\n", reason)
		return 0, nil
	}

	lockPath := filepath.Join(root, filepath.FromSlash(LockFile(prompt)))
	if err := os.MkdirAll(filepath.Dir(lockPath), 0o755); err != nil {
		return 1, fmt.Errorf("%s を作成できません: %w", filepath.Dir(lockPath), err)
	}
	release, busy, err := tryLock(lockPath)
	if err != nil {
		return 1, fmt.Errorf("ロック %s を取得できません: %w", LockFile(prompt), err)
	}
	if busy {
		fmt.Fprintf(out, "==> 別の tick が実行中のためスキップします(%s。多重起動防止の正常系)\n", LockFile(prompt))
		return 0, nil
	}
	defer release()

	code, err := launcher.Run(root, "claude", "-p", prompt)
	if err != nil {
		return 1, fmt.Errorf("claude を起動できません: %w", err)
	}
	return code, nil
}

// Remove はマーカーブロックを除去する。ブロック外の行は変更しない。
// removed はブロックが存在したとき true(無ければ何も書き込まない)。
func Remove(c Crontab) (removed bool, err error) {
	content, err := c.Read()
	if err != nil {
		return false, fmt.Errorf("crontab を読めません: %w", err)
	}
	rest, blocks, err := splitBlocks(content)
	if err != nil {
		return false, err
	}
	if len(blocks) == 0 {
		return false, nil
	}
	if err := c.Write(join(rest)); err != nil {
		return false, fmt.Errorf("crontab を書き込めません: %w", err)
	}
	return true, nil
}

// Status は設置有無とブロックの内容(マーカー間の行)を返す。
func Status(c Crontab) (installed bool, block []string, err error) {
	content, err := c.Read()
	if err != nil {
		return false, nil, fmt.Errorf("crontab を読めません: %w", err)
	}
	_, blocks, err := splitBlocks(content)
	if err != nil {
		return false, nil, err
	}
	return len(blocks) > 0, blocks, nil
}

// splitBlocks は crontab をマーカーブロック外の行(rest)とブロック内の行
// (blocks。マーカー自体は含まない)に分ける。begin / end が対応しない場合は
// エラー(呼び出し側は書き込まない)。
func splitBlocks(content string) (rest, blocks []string, err error) {
	inBlock := false
	for _, line := range strings.Split(content, "\n") {
		switch strings.TrimSpace(line) {
		case MarkerBegin:
			if inBlock {
				return nil, nil, fmt.Errorf("crontab のマーカーが壊れています(%s が二重)。手で修復してから実行してください", MarkerBegin)
			}
			inBlock = true
		case MarkerEnd:
			if !inBlock {
				return nil, nil, fmt.Errorf("crontab のマーカーが壊れています(%s だけがあります)。手で修復してから実行してください", MarkerEnd)
			}
			inBlock = false
		default:
			if inBlock {
				blocks = append(blocks, line)
			} else {
				rest = append(rest, line)
			}
		}
	}
	if inBlock {
		return nil, nil, fmt.Errorf("crontab のマーカーが壊れています(%s に対応する %s がありません)。手で修復してから実行してください", MarkerBegin, MarkerEnd)
	}
	// 末尾の空行を丸める(設置・除去を繰り返しても空行が増殖しないように)。
	for len(rest) > 0 && strings.TrimSpace(rest[len(rest)-1]) == "" {
		rest = rest[:len(rest)-1]
	}
	return rest, blocks, nil
}

// join は行を crontab の内容(末尾改行つき。空なら空文字列)に戻す。
func join(lines []string) string {
	if len(lines) == 0 {
		return ""
	}
	return strings.Join(lines, "\n") + "\n"
}

// System は実 crontab コマンドを使う Crontab 実装。
type System struct{}

// Read は crontab -l の内容を返す。crontab が未設定の場合
// (exit 1 + "no crontab for ..." )は空文字列を返す。
func (System) Read() (string, error) {
	out, err := exec.Command("crontab", "-l").CombinedOutput()
	if err != nil {
		if strings.Contains(string(out), "no crontab") {
			return "", nil
		}
		return "", fmt.Errorf("crontab -l: %v (%s)", err, strings.TrimSpace(string(out)))
	}
	return string(out), nil
}

// Write は crontab - に content を流し込んで置き換える。
func (System) Write(content string) error {
	cmd := exec.Command("crontab", "-")
	cmd.Stdin = strings.NewReader(content)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("crontab -: %v (%s)", err, strings.TrimSpace(string(out)))
	}
	return nil
}
