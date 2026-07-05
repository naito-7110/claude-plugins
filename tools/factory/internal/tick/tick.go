// Package tick は unattended 運転の起動機構(cron の tick)を crontab の
// マーカーブロックとして冪等に管理する。
//
// tick は入れっぱなしでよい(走ってよいかは factory mode が決める — night
// スキル参照)。本パッケージの責務はマーカーブロック
// (# factory-tick begin / end)の設置・置換・除去のみで、**ブロック外の行には
// 一切触れない**。マーカーが壊れている(begin / end が対応しない)場合は
// 何も書き込まずにエラーを返す(fail-closed: 他の行を壊すくらいなら止まる)。
//
// crontab コマンドはプロセス境界であり、Crontab interface で抽象化する。
// テストではインメモリ fake(internal/cronfake)を注入し、実 crontab に触れない。
package tick

import (
	"fmt"
	"os/exec"
	"strings"
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
)

// Line は tick の起動行を組み立てる(night スキルの方式 A と同形)。
// root はリポジトリの絶対パス。flock で多重起動を防ぎ、ログは
// .agents/night.log(gitignore 領域)へ追記する。
func Line(root, schedule string) string {
	return fmt.Sprintf(`%s cd %s && flock -n .agents/night.lock -c 'claude -p "/factory:night" >> .agents/night.log 2>&1'`,
		schedule, root)
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
func Install(c Crontab, root, schedule string) (replaced bool, err error) {
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
	lines := append(rest, MarkerBegin, Line(root, strings.TrimSpace(schedule)), MarkerEnd)
	if err := c.Write(join(lines)); err != nil {
		return false, fmt.Errorf("crontab を書き込めません: %w", err)
	}
	return len(blocks) > 0, nil
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
