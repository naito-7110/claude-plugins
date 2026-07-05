package tick_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/naito-7110/claude-plugins/tools/factory/internal/cronfake"
	"github.com/naito-7110/claude-plugins/tools/factory/internal/tick"
)

const otherLines = "MAILTO=ops@example.com\n30 1 * * * /usr/local/bin/backup.sh\n"

func TestInstallOnEmptyCrontab(t *testing.T) {
	cron := &cronfake.Crontab{}

	replaced, err := tick.Install(cron, "/repo", tick.DefaultSchedule)
	if err != nil {
		t.Fatal(err)
	}
	if replaced {
		t.Error("空の crontab で replaced = true")
	}
	want := tick.MarkerBegin + "\n" + tick.Line("/repo", tick.DefaultSchedule) + "\n" + tick.MarkerEnd + "\n"
	if cron.Content != want {
		t.Errorf("Content = %q, want %q", cron.Content, want)
	}
}

func TestInstallPreservesOtherLines(t *testing.T) {
	cron := &cronfake.Crontab{Content: otherLines}

	if _, err := tick.Install(cron, "/repo", tick.DefaultSchedule); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(cron.Content, otherLines) {
		t.Errorf("既存の行が保存されていない: %q", cron.Content)
	}
	if !strings.Contains(cron.Content, tick.MarkerBegin) || !strings.Contains(cron.Content, tick.MarkerEnd) {
		t.Errorf("マーカーブロックがない: %q", cron.Content)
	}
}

func TestInstallTwiceIsIdempotent(t *testing.T) {
	cron := &cronfake.Crontab{Content: otherLines}

	if _, err := tick.Install(cron, "/repo", tick.DefaultSchedule); err != nil {
		t.Fatal(err)
	}
	replaced, err := tick.Install(cron, "/repo", "15 4 * * *")
	if err != nil {
		t.Fatal(err)
	}
	if !replaced {
		t.Error("2 回目の install で replaced = false")
	}
	if got := strings.Count(cron.Content, tick.MarkerBegin); got != 1 {
		t.Errorf("マーカーブロックが %d 個ある(冪等でない): %q", got, cron.Content)
	}
	if !strings.Contains(cron.Content, "15 4 * * * cd /repo") {
		t.Errorf("スケジュールが置換されていない: %q", cron.Content)
	}
	if strings.Contains(cron.Content, tick.DefaultSchedule+" cd /repo") {
		t.Errorf("旧スケジュールが残っている: %q", cron.Content)
	}
	if !strings.HasPrefix(cron.Content, otherLines) {
		t.Errorf("既存の行が保存されていない: %q", cron.Content)
	}
}

func TestInstallRejectsInvalidSchedule(t *testing.T) {
	cron := &cronfake.Crontab{Content: otherLines}

	if _, err := tick.Install(cron, "/repo", "3:00 に起動"); err == nil {
		t.Fatal("不正な cron 式が受理されている")
	}
	if cron.Content != otherLines {
		t.Errorf("失敗時に crontab が書き換わっている: %q", cron.Content)
	}
}

func TestInstallAcceptsAtNotation(t *testing.T) {
	cron := &cronfake.Crontab{}
	if _, err := tick.Install(cron, "/repo", "@daily"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(cron.Content, "@daily cd /repo") {
		t.Errorf("@ 記法が使えない: %q", cron.Content)
	}
}

func TestRemoveDeletesOnlyMarkerBlock(t *testing.T) {
	cron := &cronfake.Crontab{Content: otherLines}
	if _, err := tick.Install(cron, "/repo", tick.DefaultSchedule); err != nil {
		t.Fatal(err)
	}

	removed, err := tick.Remove(cron)
	if err != nil {
		t.Fatal(err)
	}
	if !removed {
		t.Error("removed = false")
	}
	if cron.Content != otherLines {
		t.Errorf("Content = %q, want %q(他の行だけが残る)", cron.Content, otherLines)
	}
}

func TestRemoveWhenAbsentDoesNotWrite(t *testing.T) {
	// 未設置なら書き込み自体が発生しない: Write を失敗させても Remove は成功する。
	cron := &cronfake.Crontab{Content: otherLines, WriteErr: errors.New("書き込み禁止")}

	removed, err := tick.Remove(cron)
	if err != nil {
		t.Fatal(err)
	}
	if removed {
		t.Error("未設置で removed = true")
	}
	if cron.Content != otherLines {
		t.Errorf("未設置なのに内容が変わっている: %q", cron.Content)
	}
}

func TestBrokenMarkersFailWithoutWriting(t *testing.T) {
	broken := otherLines + tick.MarkerBegin + "\n0 3 * * * something\n" // end がない
	cron := &cronfake.Crontab{Content: broken}

	if _, err := tick.Install(cron, "/repo", tick.DefaultSchedule); err == nil {
		t.Fatal("壊れたマーカーで install が成功している")
	}
	if _, err := tick.Remove(cron); err == nil {
		t.Fatal("壊れたマーカーで remove が成功している")
	}
	if cron.Content != broken {
		t.Errorf("壊れた crontab に書き込んでいる: %q", cron.Content)
	}
}

func TestStatus(t *testing.T) {
	cron := &cronfake.Crontab{Content: otherLines}

	installed, _, err := tick.Status(cron)
	if err != nil {
		t.Fatal(err)
	}
	if installed {
		t.Error("未設置で installed = true")
	}

	if _, err := tick.Install(cron, "/repo", tick.DefaultSchedule); err != nil {
		t.Fatal(err)
	}
	installed, block, err := tick.Status(cron)
	if err != nil {
		t.Fatal(err)
	}
	if !installed {
		t.Error("設置済みで installed = false")
	}
	if len(block) != 1 || !strings.Contains(block[0], "flock -n .agents/night.lock") {
		t.Errorf("block = %q(flock 付き起動行が入る)", block)
	}
}

func TestLineShape(t *testing.T) {
	line := tick.Line("/path/to/repo", tick.DefaultSchedule)
	for _, want := range []string{
		"0 3 * * 1-5 cd /path/to/repo && ",
		"flock -n .agents/night.lock",
		`claude -p "/factory:night"`,
		">> .agents/night.log 2>&1",
	} {
		if !strings.Contains(line, want) {
			t.Errorf("Line に %q がない: %q", want, line)
		}
	}
}
