package gate

import (
	"reflect"
	"testing"
)

// Tokenize はコマンド文字列を「セグメント(コマンド列要素)ごとのトークン列」に
// 分解する。#119 の核心: 判定を文字列全体の部分一致でなくトークンで行うための基盤。
func TestTokenizeSplitsOnOperators(t *testing.T) {
	cases := []struct {
		name string
		cmd  string
		want [][]string
	}{
		{
			name: "単純なコマンド",
			cmd:  "git push origin main",
			want: [][]string{{"git", "push", "origin", "main"}},
		},
		{
			name: "&& で分割",
			cmd:  "git rebase --onto origin/main x && git push --force-with-lease origin agent/issue-7",
			want: [][]string{
				{"git", "rebase", "--onto", "origin/main", "x"},
				{"git", "push", "--force-with-lease", "origin", "agent/issue-7"},
			},
		},
		{
			name: "セミコロン直後に空白なし",
			cmd:  "atelier release --dry-run;atelier release atelier/v1.0.0",
			want: [][]string{
				{"atelier", "release", "--dry-run"},
				{"atelier", "release", "atelier/v1.0.0"},
			},
		},
		{
			name: "パイプと || の混在",
			cmd:  "cat x | grep y || echo z",
			want: [][]string{
				{"cat", "x"},
				{"grep", "y"},
				{"echo", "z"},
			},
		},
		{
			name: "単一引用符はひとつのトークン(区切り演算子を含んでも割らない)",
			cmd:  "git commit -m 'covers push/merge/release; and && more'",
			want: [][]string{
				{"git", "commit", "-m", "covers push/merge/release; and && more"},
			},
		},
		{
			name: "二重引用符もひとつのトークン",
			cmd:  `git commit -m "main への直 push は禁止"`,
			want: [][]string{
				{"git", "commit", "-m", "main への直 push は禁止"},
			},
		},
		{
			name: "引用符付きタグ名は剥がして1トークン",
			cmd:  "git push origin 'atelier/v1.0.0'",
			want: [][]string{
				{"git", "push", "origin", "atelier/v1.0.0"},
			},
		},
		{
			name: "バックスラッシュエスケープ",
			cmd:  `echo a\ b`,
			want: [][]string{{"echo", "a b"}},
		},
		{
			name: "空セグメントは落とす",
			cmd:  "git push ;; ",
			want: [][]string{{"git", "push"}},
		},
		{
			name: "改行も区切り",
			cmd:  "git add -A\ngit push origin main",
			want: [][]string{
				{"git", "add", "-A"},
				{"git", "push", "origin", "main"},
			},
		},
		{
			name: "空文字列",
			cmd:  "",
			want: nil,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Tokenize(tc.cmd)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("Tokenize(%q) =\n  %#v\nwant\n  %#v", tc.cmd, got, tc.want)
			}
		})
	}
}

// commandName はセグメント先頭の環境変数代入とパス接頭辞を除いた実行名を返す。
func TestCommandName(t *testing.T) {
	cases := []struct {
		seg  []string
		want string
	}{
		{[]string{"git", "push"}, "git"},
		{[]string{".agents/bin/atelier", "release"}, "atelier"},
		{[]string{"/usr/local/bin/gh", "pr", "merge"}, "gh"},
		{[]string{"FOO=bar", "git", "push"}, "git"},
		{[]string{"FOO=bar", "BAZ=1", "atelier", "release"}, "atelier"},
		{[]string{"FOO=bar"}, ""}, // 代入だけで実行名がない
		{nil, ""},
	}
	for _, tc := range cases {
		name, _ := commandName(tc.seg)
		if name != tc.want {
			t.Errorf("commandName(%v) = %q, want %q", tc.seg, name, tc.want)
		}
	}
}
