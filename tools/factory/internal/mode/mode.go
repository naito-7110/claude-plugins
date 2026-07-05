// Package mode は unattended 運転の運転状態(このマシンの運用状態)を管理する。
//
// 運転状態はリポジトリの状態ではなくマシンの状態なので、コミット対象にしない
// (PR #80 レビュー: コミット履歴を濁さない)。状態は .agents/ 配下(gitignore
// 領域)のローカルファイルに置き、操作は常に factory mode 経由で行う
// (人間は orchestrate との会話から「止めて」と頼めば、PM がこの bin を呼ぶ)。
//
// fail-closed: 既定は manual。状態ファイルが無い・読めない・内容が不正でも
// manual として扱う(明示的に factory mode auto されたマシンだけが無人運転する)。
//
// 状態ファイルの配置はこのパッケージに単一定義する(docs / flags と同じパターン)。
package mode

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// 状態ファイルの配置(単一定義)。
const (
	// Dir は運転状態を置くローカルディレクトリ(gitignore 領域・コミットしない)。
	Dir = ".agents"
	// ModeFile は運転モード(auto / manual)の状態ファイル。
	ModeFile = ".agents/mode"
)

// 運転モードの値。
const (
	Auto   = "auto"
	Manual = "manual"
)

// State は現在の運転状態。auto / manual の二値(PR #82 レビュー:
// 状態機械は小さいほど事故が少ない。「今すぐ止める」も manual で即効)。
type State struct {
	Mode string // Auto | Manual(fail-closed の正規化済み)
	Note string // Mode が既定値に落ちた理由(状態ファイル欠落・不正内容)。空なら明示設定
}

// Load は root(リポジトリのルート)の運転状態を読む。
// fail-closed: 状態ファイルが無い・不正でもエラーにせず manual として返す
// (Note に理由が入る)。error は root 自体が読めない等の実行失敗のみ。
func Load(root string) (State, error) {
	info, err := os.Stat(root)
	if err != nil {
		return State{}, fmt.Errorf("root を読めません: %w", err)
	}
	if !info.IsDir() {
		return State{}, fmt.Errorf("root がディレクトリではありません: %s", root)
	}

	state := State{Mode: Manual}
	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(ModeFile)))
	switch {
	case errors.Is(err, fs.ErrNotExist):
		state.Note = fmt.Sprintf("%s がありません(既定 = manual)", ModeFile)
	case err != nil:
		state.Note = fmt.Sprintf("%s を読めません(fail-closed で manual 扱い): %v", ModeFile, err)
	default:
		content := strings.TrimSpace(string(data))
		if content == Auto || content == Manual {
			state.Mode = content
		} else {
			state.Note = fmt.Sprintf("%s の内容 %q が不正です(fail-closed で manual 扱い。factory mode auto/manual で設定し直してください)",
				ModeFile, content)
		}
	}
	return state, nil
}

// SetMode は運転モード(Auto / Manual)を書き込む。
func SetMode(root, value string) error {
	if value != Auto && value != Manual {
		return fmt.Errorf("不正な運転モードです: %s(auto / manual のみ)", value)
	}
	p := filepath.Join(root, filepath.FromSlash(ModeFile))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return fmt.Errorf("%s を作成できません: %w", Dir, err)
	}
	if err := os.WriteFile(p, []byte(value+"\n"), 0o644); err != nil {
		return fmt.Errorf("%s を書き込めません: %w", ModeFile, err)
	}
	return nil
}

// Gate は無人起動口(night)の判定。auto のときだけ ok = true。
// ok = false のとき reason に人間可読の理由(と復帰手順)が入る。
// 「今すぐ止める」も factory mode manual で即効する(状態はローカルなので反映は即時)。
func Gate(state State) (ok bool, reason string) {
	if state.Mode != Auto {
		reason := "運転モードが manual です。unattended 運転を許可するには factory mode auto を実行してください"
		if state.Note != "" {
			reason += "(" + state.Note + ")"
		}
		return false, reason
	}
	return true, ""
}
