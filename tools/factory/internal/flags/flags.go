// Package flags は feature-flags プリセットの canonical レジストリ検証を提供する。
//
// feature-flags プリセット(plugins/factory/adr/feature-flags.md)は
// 「フラグには必ず期限と所有者がある」を構造的に強制し、期限切れフラグの残存で
// CI を失敗させることを決定している。本パッケージがその判定の実体で、
// issue / pr / docs verify と同じく、hook / GHA の薄い入口から呼ばれる
// (スタックごとの自作検証を持たない。判定はここに一本化する)。
//
// canonical レジストリは RegistryPath(.factory/flags.yaml)に単一定義する
// (配置の基準は documentation プリセットの C 案: factory が生成し機械が読む
// 運用ファイルは .factory/)。形式:
//
//	flags:
//	  <name>:                  # 小文字スネークケース(識別子)
//	    owner: <string>        # 必須(削除に責任を持つ人)
//	    expires_on: 2026-12-31 # 必須(期限日)
//	    description: <string>  # 必須(目的)
//
// フラグの値は含めない(値の所有は各プロダクトの配信側 — 二重管理の禁止)。
//
// 検査項目:
//   - レジストリなし = 正常(フラグ未使用。情報表示のみ)
//   - 形式不正・必須属性の欠落・不正なフラグ名 = NG
//   - 期限切れ(expires_on < 今日)= NG(フラグ名・owner・期限を理由に出す)
//   - 期限接近(今日から warnDays 日以内)= 警告のみ(朝 report の材料)
//
// 「今日」は呼び出し側から注入する(tdd-doctrine: 時刻はプロセス境界)。
package flags

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/naito-7110/claude-plugins/tools/factory/internal/verify"
)

// 検査項目の名前。
const (
	CheckRegistry = "flags-registry" // レジストリの形式(スキーマ・必須属性・フラグ名)
	CheckExpiry   = "flags-expiry"   // 期限切れ(NG)と期限接近(警告)
)

// RegistryPath は canonical フラグレジストリの配置(単一定義)。
// 検査ロジックとテストはすべてここを参照する。
const RegistryPath = ".factory/flags.yaml"

// DefaultWarnDays は期限接近を警告する既定の日数。
const DefaultWarnDays = 14

// DateFormat は expires_on の日付形式。
const DateFormat = "2006-01-02"

// flagNamePattern はフラグ名の制約(小文字スネークケース)。
// フラグ名はコード・設定・レジストリの全層で共有される識別子であり、
// 大文字を許すと参照側との突き合わせで大小文字を区別しない FS / DB との
// 環境差事故が起きる(docs verify のドメイン名と同じ理由)。
var flagNamePattern = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

// Report はフラグレジストリの検証結果。
type Report struct {
	Root     string
	Findings []verify.Finding
}

func (r *Report) add(check string, level verify.Level, format string, args ...interface{}) {
	r.Findings = append(r.Findings, verify.Finding{
		Check: check, Level: level, Message: fmt.Sprintf(format, args...),
	})
}

// NGCount は NG の所見数を返す(警告は数えない)。
func (r Report) NGCount() int {
	count := 0
	for _, f := range r.Findings {
		if f.Level == verify.LevelNG {
			count++
		}
	}
	return count
}

// OK は NG の所見がないとき true。
func (r Report) OK() bool {
	return r.NGCount() == 0
}

// entry はレジストリの 1 フラグ分の宣言(name はマップのキー)。
// expires_on は文字列で受けて自前でパースする(不正な日付に
// yaml エラーではなくフラグ名つきの理由を出すため)。
type entry struct {
	Owner       string `yaml:"owner"`
	ExpiresOn   string `yaml:"expires_on"`
	Description string `yaml:"description"`
}

// registryDoc は flags.yaml のスキーマ(flags.<name>.{owner,expires_on,description})。
type registryDoc struct {
	Flags map[string]entry `yaml:"flags"`
}

// Check は root(リポジトリのルート)のフラグレジストリを検証する。
// today は「今日」の基準時刻(時刻部分は無視して日付で比較する)。
// error は検証の成否ではなく、root 自体が読めない等の実行失敗を表す。
func Check(root string, warnDays int, today time.Time) (Report, error) {
	info, err := os.Stat(root)
	if err != nil {
		return Report{}, fmt.Errorf("root を読めません: %w", err)
	}
	if !info.IsDir() {
		return Report{}, fmt.Errorf("root がディレクトリではありません: %s", root)
	}
	report := Report{Root: root}

	entries, ok := loadRegistry(&report, root)
	if !ok {
		return report, nil
	}
	names := make([]string, 0, len(entries))
	for name := range entries {
		names = append(names, name)
	}
	sort.Strings(names)
	if len(names) == 0 {
		report.add(CheckRegistry, verify.LevelOK,
			"%s は正常(フラグ 0 件。flags: {} は正常)", RegistryPath)
		return report, nil
	}
	report.add(CheckRegistry, verify.LevelOK,
		"%s は正常(フラグ %d 件: %s)", RegistryPath, len(names), strings.Join(names, ", "))

	date := dateOnly(today)
	for _, name := range names {
		checkEntry(&report, name, entries[name], date, warnDays)
	}
	return report, nil
}

// loadRegistry は flags.yaml を読み、スキーマ検証の所見を report に積む。
// 期限の検査を続けられるときだけ ok = true を返す。
// ファイルが無いのは正常(フラグ未使用)で、情報表示のみ行う。
func loadRegistry(report *Report, root string) (map[string]entry, bool) {
	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(RegistryPath)))
	if errors.Is(err, fs.ErrNotExist) {
		report.add(CheckRegistry, verify.LevelInfo,
			"%s がありません(フラグ未使用は正常。検査対象なし)", RegistryPath)
		return nil, false
	}
	if err != nil {
		report.add(CheckRegistry, verify.LevelNG, "%s を読めません: %v", RegistryPath, err)
		return nil, false
	}

	var doc registryDoc
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true) // スキーマ外のキー(値の混入・タイポ)は形式違反として弾く
	if err := decoder.Decode(&doc); err != nil && !errors.Is(err, io.EOF) {
		report.add(CheckRegistry, verify.LevelNG,
			"%s をパースできません(期待する形式: flags.<name>.{owner, expires_on, description}。フラグの値は配信側が所有するためレジストリに含めません): %v",
			RegistryPath, err)
		return nil, false
	}
	if doc.Flags == nil {
		report.add(CheckRegistry, verify.LevelNG,
			"%s に flags キーがありません(フラグ 0 件でも flags: {} を宣言してください)",
			RegistryPath)
		return nil, false
	}

	valid := true
	names := make([]string, 0, len(doc.Flags))
	for name := range doc.Flags {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		if !flagNamePattern.MatchString(name) {
			report.add(CheckRegistry, verify.LevelNG,
				"フラグ名 %q が不正です(小文字スネークケース ^[a-z][a-z0-9_]*$ のみ。フラグ名は全層で共有される識別子のため、環境差の出る大文字を許しません)",
				name)
			valid = false
		}
		valid = checkRequired(report, name, doc.Flags[name]) && valid
	}
	if !valid {
		return nil, false
	}
	return doc.Flags, true
}

// checkRequired は 1 フラグの必須属性(owner / expires_on / description)を検査する。
// 期限の解釈は checkEntry の責務(ここでは存在のみ)。
func checkRequired(report *Report, name string, e entry) bool {
	valid := true
	if strings.TrimSpace(e.Owner) == "" {
		report.add(CheckRegistry, verify.LevelNG,
			"フラグ %s に owner がありません(削除に責任を持つ人を宣言してください)", name)
		valid = false
	}
	if strings.TrimSpace(e.ExpiresOn) == "" {
		report.add(CheckRegistry, verify.LevelNG,
			"フラグ %s に expires_on がありません(期限日 YYYY-MM-DD を宣言してください)", name)
		valid = false
	}
	if strings.TrimSpace(e.Description) == "" {
		report.add(CheckRegistry, verify.LevelNG,
			"フラグ %s に description がありません(フラグの目的を宣言してください)", name)
		valid = false
	}
	return valid
}

// checkEntry は 1 フラグの期限を検査する。today は日付に正規化済みであること。
func checkEntry(report *Report, name string, e entry, today time.Time, warnDays int) {
	expires, err := time.ParseInLocation(DateFormat, e.ExpiresOn, time.UTC)
	if err != nil {
		report.add(CheckRegistry, verify.LevelNG,
			"フラグ %s の expires_on %q を日付(%s)として解釈できません", name, e.ExpiresOn, "YYYY-MM-DD")
		return
	}
	days := int(expires.Sub(today).Hours() / 24)
	switch {
	case days < 0:
		report.add(CheckExpiry, verify.LevelNG,
			"フラグ %s は期限切れです(owner: %s / 期限: %s、%d 日超過)。フラグを削除する PR をもって正式リリースとしてください(feature-flags)",
			name, e.Owner, e.ExpiresOn, -days)
	case days <= warnDays:
		report.add(CheckExpiry, verify.LevelWarn,
			"フラグ %s の期限が近づいています(owner: %s / 期限: %s、残り %d 日)。削除 PR の計画を確認してください",
			name, e.Owner, e.ExpiresOn, days)
	default:
		report.add(CheckExpiry, verify.LevelOK,
			"フラグ %s は期限内(期限: %s、残り %d 日)", name, e.ExpiresOn, days)
	}
}

// dateOnly は時刻を日付(UTC の 0 時)に正規化する。
// 期限は「日」の粒度で比較する(expires_on 当日はまだ期限内)。
func dateOnly(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}
