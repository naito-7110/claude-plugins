package flags_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/naito-7110/claude-plugins/tools/factory/internal/flags"
	"github.com/naito-7110/claude-plugins/tools/factory/internal/verify"
)

// today は検査の基準日(2026-07-05)。時刻部分は日付比較で無視されることを
// 検証するため、日中の時刻を含めておく。
var today = time.Date(2026, 7, 5, 15, 30, 0, 0, time.UTC)

func writeRegistry(t *testing.T, content string) string {
	t.Helper()
	root := t.TempDir()
	p := filepath.Join(root, filepath.FromSlash(flags.RegistryPath))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}

func check(t *testing.T, root string) flags.Report {
	t.Helper()
	report, err := flags.Check(root, flags.DefaultWarnDays, today)
	if err != nil {
		t.Fatal(err)
	}
	return report
}

func joined(report flags.Report) string {
	var lines []string
	for _, f := range report.Findings {
		lines = append(lines, string(f.Level)+": "+f.Message)
	}
	return strings.Join(lines, "\n")
}

func levelOf(report flags.Report, substr string) verify.Level {
	for _, f := range report.Findings {
		if strings.Contains(f.Message, substr) {
			return f.Level
		}
	}
	return ""
}

// --- レジストリの存在 ---

func TestCheckMissingRegistryIsOK(t *testing.T) {
	report := check(t, t.TempDir())
	if !report.OK() {
		t.Fatalf("レジストリなしで NG になっている: %s", joined(report))
	}
	if levelOf(report, "がありません(フラグ未使用は正常") != verify.LevelInfo {
		t.Errorf("情報表示がない: %s", joined(report))
	}
}

func TestCheckEmptyFlagsIsOK(t *testing.T) {
	report := check(t, writeRegistry(t, "flags: {}\n"))
	if !report.OK() {
		t.Fatalf("flags: {} で NG になっている: %s", joined(report))
	}
	if levelOf(report, "フラグ 0 件") != verify.LevelOK {
		t.Errorf("正常の所見がない: %s", joined(report))
	}
}

// --- 形式 ---

func TestCheckMissingFlagsKeyFails(t *testing.T) {
	report := check(t, writeRegistry(t, "# 空のレジストリ\n"))
	if report.OK() {
		t.Fatalf("flags キー無しで OK になっている: %s", joined(report))
	}
	if levelOf(report, "flags キーがありません") != verify.LevelNG {
		t.Errorf("理由がない: %s", joined(report))
	}
}

func TestCheckBrokenYAMLFails(t *testing.T) {
	report := check(t, writeRegistry(t, "flags: [broken\n"))
	if report.OK() {
		t.Fatalf("壊れた yaml で OK になっている: %s", joined(report))
	}
	if levelOf(report, "パースできません") != verify.LevelNG {
		t.Errorf("理由がない: %s", joined(report))
	}
}

func TestCheckUnknownFieldFails(t *testing.T) {
	// スキーマ外のキーは strict デコードで弾く。値の混入
	// (値の所有は配信側 — 二重管理の禁止)もこれで検出される。
	report := check(t, writeRegistry(t, `flags:
  new_checkout:
    owner: alice
    expires_on: 2026-12-31
    description: 新しい決済フロー
    value: true
`))
	if report.OK() {
		t.Fatalf("スキーマ外キーで OK になっている: %s", joined(report))
	}
	if levelOf(report, "パースできません") != verify.LevelNG {
		t.Errorf("理由がない: %s", joined(report))
	}
}

func TestCheckMissingRequiredAttributesFails(t *testing.T) {
	report := check(t, writeRegistry(t, `flags:
  new_checkout:
    expires_on: 2026-12-31
  dark_mode:
    owner: bob
    description: ダークモード
`))
	if report.OK() {
		t.Fatalf("必須属性の欠落で OK になっている: %s", joined(report))
	}
	for _, want := range []string{
		"フラグ new_checkout に owner がありません",
		"フラグ new_checkout に description がありません",
		"フラグ dark_mode に expires_on がありません",
	} {
		if levelOf(report, want) != verify.LevelNG {
			t.Errorf("理由 %q がない: %s", want, joined(report))
		}
	}
}

func TestCheckInvalidDateFails(t *testing.T) {
	report := check(t, writeRegistry(t, `flags:
  new_checkout:
    owner: alice
    expires_on: 2026/12/31
    description: 新しい決済フロー
`))
	if report.OK() {
		t.Fatalf("不正な日付で OK になっている: %s", joined(report))
	}
	if levelOf(report, `expires_on "2026/12/31" を日付`) != verify.LevelNG {
		t.Errorf("理由がない: %s", joined(report))
	}
}

func TestCheckInvalidFlagNameFails(t *testing.T) {
	report := check(t, writeRegistry(t, `flags:
  newCheckout:
    owner: alice
    expires_on: 2026-12-31
    description: 新しい決済フロー
`))
	if report.OK() {
		t.Fatalf("不正なフラグ名で OK になっている: %s", joined(report))
	}
	if levelOf(report, `フラグ名 "newCheckout" が不正です`) != verify.LevelNG {
		t.Errorf("理由がない: %s", joined(report))
	}
}

// --- 期限 ---

func TestCheckExpiredFlagFails(t *testing.T) {
	report := check(t, writeRegistry(t, `flags:
  old_banner:
    owner: alice
    expires_on: 2026-07-01
    description: 旧バナーの切り替え
`))
	if report.OK() {
		t.Fatalf("期限切れで OK になっている: %s", joined(report))
	}
	want := "フラグ old_banner は期限切れです(owner: alice / 期限: 2026-07-01、4 日超過)"
	if levelOf(report, want) != verify.LevelNG {
		t.Errorf("理由 %q がない: %s", want, joined(report))
	}
}

func TestCheckExpiresTodayIsNotExpired(t *testing.T) {
	// 期限当日はまだ期限内(expires_on < 今日 が NG 条件)。接近警告にはなる。
	// today の時刻部分(15:30)が日付比較に影響しないこともここで固定する。
	report := check(t, writeRegistry(t, `flags:
  last_day:
    owner: alice
    expires_on: 2026-07-05
    description: 当日期限
`))
	if !report.OK() {
		t.Fatalf("期限当日が NG になっている: %s", joined(report))
	}
	if levelOf(report, "フラグ last_day の期限が近づいています(owner: alice / 期限: 2026-07-05、残り 0 日)") != verify.LevelWarn {
		t.Errorf("接近警告がない: %s", joined(report))
	}
}

func TestCheckApproachingFlagWarnsOnly(t *testing.T) {
	// 既定 14 日以内(2026-07-19 = 残り 14 日)は警告のみで exit に影響しない。
	report := check(t, writeRegistry(t, `flags:
  soon:
    owner: bob
    expires_on: 2026-07-19
    description: 期限接近
`))
	if !report.OK() {
		t.Fatalf("接近のみで NG になっている: %s", joined(report))
	}
	if levelOf(report, "フラグ soon の期限が近づいています(owner: bob / 期限: 2026-07-19、残り 14 日)") != verify.LevelWarn {
		t.Errorf("接近警告がない: %s", joined(report))
	}
}

func TestCheckHealthyFlagIsOK(t *testing.T) {
	// 残り 15 日(既定 warnDays の 1 日外)は警告なしの OK。
	report := check(t, writeRegistry(t, `flags:
  healthy:
    owner: carol
    expires_on: 2026-07-20
    description: 期限内
`))
	if !report.OK() {
		t.Fatalf("正常系が NG になっている: %s", joined(report))
	}
	if levelOf(report, "フラグ healthy は期限内(期限: 2026-07-20、残り 15 日)") != verify.LevelOK {
		t.Errorf("OK の所見がない: %s", joined(report))
	}
	if strings.Contains(joined(report), "警告") {
		t.Errorf("warnDays の外で警告が出ている: %s", joined(report))
	}
}

func TestCheckWarnDaysIsConfigurable(t *testing.T) {
	root := writeRegistry(t, `flags:
  soon:
    owner: bob
    expires_on: 2026-07-20
    description: 残り 15 日
`)
	report, err := flags.Check(root, 30, today)
	if err != nil {
		t.Fatal(err)
	}
	if !report.OK() {
		t.Fatalf("接近のみで NG になっている: %s", joined(report))
	}
	if levelOf(report, "残り 15 日)。削除 PR の計画を確認してください") != verify.LevelWarn {
		t.Errorf("warn-days 30 で接近警告が出ない: %s", joined(report))
	}
}

func TestCheckMixedRegistryReportsAll(t *testing.T) {
	// 期限切れ・接近・正常が混在しても、全フラグの所見が名前順で出る。
	report := check(t, writeRegistry(t, `flags:
  expired_one:
    owner: alice
    expires_on: 2026-01-01
    description: 期限切れ
  healthy_one:
    owner: carol
    expires_on: 2026-12-31
    description: 正常
  soon_one:
    owner: bob
    expires_on: 2026-07-10
    description: 接近
`))
	if report.OK() {
		t.Fatalf("期限切れ混在で OK になっている: %s", joined(report))
	}
	if report.NGCount() != 1 {
		t.Errorf("NGCount = %d, want 1(接近・正常は NG に数えない): %s", report.NGCount(), joined(report))
	}
	out := joined(report)
	for _, want := range []string{
		"expired_one は期限切れです",
		"soon_one の期限が近づいています",
		"healthy_one は期限内",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("所見 %q がない: %s", want, out)
		}
	}
}

// --- 実行失敗 ---

func TestCheckMissingRootIsError(t *testing.T) {
	_, err := flags.Check(filepath.Join(t.TempDir(), "no-such-dir"), flags.DefaultWarnDays, today)
	if err == nil || !strings.Contains(err.Error(), "root を読めません") {
		t.Errorf("err = %v", err)
	}
}
