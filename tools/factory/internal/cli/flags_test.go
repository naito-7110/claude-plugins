package cli_test

import (
	"strings"
	"testing"

	"github.com/naito-7110/claude-plugins/tools/factory/internal/cli"
	"github.com/naito-7110/claude-plugins/tools/factory/internal/flags"
)

// CLI 配線のテスト。時刻の境界(接近日数の窓)は today を注入する
// internal/flags 側で固定済みのため、ここでは実時刻に依存しない
// 遠い過去・遠い未来の期限だけを使う。

func executeFlags(t *testing.T, root string, extra ...string) run {
	t.Helper()
	args := append([]string{"flags", "check", "--root", root}, extra...)
	return execute(t, testServer(), testRepo, args...)
}

func TestFlagsCheckMissingRegistryExitsZero(t *testing.T) {
	result := executeFlags(t, t.TempDir())
	if result.code != cli.ExitOK {
		t.Fatalf("code = %d, want %d (out=%q err=%q)", result.code, cli.ExitOK, result.out, result.err)
	}
	if !strings.Contains(result.out, "フラグ未使用は正常") {
		t.Errorf("情報表示がない: %q", result.out)
	}
	if !strings.Contains(result.out, "==> 結果: OK") {
		t.Errorf("結果が出力されない: %q", result.out)
	}
}

func TestFlagsCheckExpiredExitsOne(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, flags.RegistryPath, `flags:
  old_banner:
    owner: alice
    expires_on: 1999-01-01
    description: 旧バナーの切り替え
`)

	result := executeFlags(t, root)
	if result.code != cli.ExitError {
		t.Fatalf("code = %d, want %d (out=%q)", result.code, cli.ExitError, result.out)
	}
	if !strings.Contains(result.out, "フラグ old_banner は期限切れです(owner: alice / 期限: 1999-01-01") {
		t.Errorf("理由が出力されない: %q", result.out)
	}
}

func TestFlagsCheckMissingAttributeExitsOne(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, flags.RegistryPath, `flags:
  new_checkout:
    owner: alice
    expires_on: 2999-01-01
`)

	result := executeFlags(t, root)
	if result.code != cli.ExitError {
		t.Fatalf("code = %d, want %d (out=%q)", result.code, cli.ExitError, result.out)
	}
	if !strings.Contains(result.out, "フラグ new_checkout に description がありません") {
		t.Errorf("理由が出力されない: %q", result.out)
	}
}

func TestFlagsCheckHealthyExitsZero(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, flags.RegistryPath, `flags:
  new_checkout:
    owner: alice
    expires_on: 2999-01-01
    description: 新しい決済フロー
`)

	result := executeFlags(t, root)
	if result.code != cli.ExitOK {
		t.Fatalf("code = %d, want %d (out=%q err=%q)", result.code, cli.ExitOK, result.out, result.err)
	}
	if !strings.Contains(result.out, "フラグ new_checkout は期限内") {
		t.Errorf("OK の所見がない: %q", result.out)
	}
}

func TestFlagsCheckWarnDaysFlagWidensWarning(t *testing.T) {
	// 遠い未来の期限も --warn-days を広げれば接近警告になる(警告のみで exit 0)。
	root := t.TempDir()
	writeFile(t, root, flags.RegistryPath, `flags:
  new_checkout:
    owner: alice
    expires_on: 2999-01-01
    description: 新しい決済フロー
`)

	result := executeFlags(t, root, "--warn-days", "1000000")
	if result.code != cli.ExitOK {
		t.Fatalf("code = %d, want %d (out=%q)", result.code, cli.ExitOK, result.out)
	}
	if !strings.Contains(result.out, "フラグ new_checkout の期限が近づいています") {
		t.Errorf("接近警告がない: %q", result.out)
	}
}

func TestFlagsCheckNegativeWarnDaysIsUsageError(t *testing.T) {
	result := executeFlags(t, t.TempDir(), "--warn-days", "-1")
	if result.code != cli.ExitUsage {
		t.Fatalf("code = %d, want %d", result.code, cli.ExitUsage)
	}
	if !strings.Contains(result.err, "--warn-days は 0 以上") {
		t.Errorf("err = %q", result.err)
	}
}
