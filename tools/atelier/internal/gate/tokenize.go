package gate

import "strings"

// tokenize.go — gate 判定のための軽量トークナイザ(#119)。
//
// 判定を「コマンド文字列全体への正規表現部分一致」から「構文トークン化 → 実
// コマンド識別」へ移すための基盤。フルシェルパーサ(mvdan.cc/sh 等)は gate の
// 用途に過剰で依存追加も避けたいため(ADR 0002 / dependency-licensing)、
// 引用符・エスケープ・トップレベルの区切り演算子だけを扱う最小実装に留める。
//
// **設計上の限界(意図的)**: トップレベルのコマンド列のみを解釈する。引用符内は
// ひとつの文字列トークンとして扱い、その中にネストしたコマンド(bash -c の中身・
// $(...) 置換)は解析しない。gate は「うっかり・自然な操作の防御」であって完璧な
// sandbox ではない(ADR 0002: 防御は配るが完璧は求めない)。

// segmentSeparators はコマンド列を分ける演算子(トップレベルのみ)。
// パイプ | / セミコロン ; / アンパサンド & / 改行。&& と || は & | の連続として
// 自然に空セグメントを生み、それは捨てられる。
func isSeparator(r rune) bool {
	switch r {
	case ';', '&', '|', '\n':
		return true
	}
	return false
}

// Tokenize はコマンド文字列をセグメント(コマンド列要素)ごとのトークン列へ分解する。
// 引用符(' ")とバックスラッシュエスケープを処理し、クオート外の区切り演算子で
// セグメントを割る。空のセグメント・空文字列は結果に含めない。
func Tokenize(cmd string) [][]string {
	var segments [][]string
	var cur []string      // 現在のセグメントのトークン列
	var tok strings.Builder // 構築中のトークン
	inTok := false          // 現在トークンを構築中か(空トークンと未開始を区別する)

	flushTok := func() {
		if inTok {
			cur = append(cur, tok.String())
			tok.Reset()
			inTok = false
		}
	}
	flushSeg := func() {
		flushTok()
		if len(cur) > 0 {
			segments = append(segments, cur)
			cur = nil
		}
	}

	runes := []rune(cmd)
	for i := 0; i < len(runes); i++ {
		r := runes[i]
		switch {
		case r == '\'':
			inTok = true
			for i++; i < len(runes) && runes[i] != '\''; i++ {
				tok.WriteRune(runes[i])
			}
		case r == '"':
			inTok = true
			for i++; i < len(runes) && runes[i] != '"'; i++ {
				// 二重引用符内のバックスラッシュは次の 1 文字をエスケープする。
				if runes[i] == '\\' && i+1 < len(runes) {
					i++
				}
				tok.WriteRune(runes[i])
			}
		case r == '\\':
			// クオート外のエスケープ: 次の 1 文字(改行含む)をリテラル化する。
			if i+1 < len(runes) {
				i++
				inTok = true
				tok.WriteRune(runes[i])
			}
		case r == ' ' || r == '\t':
			flushTok()
		case isSeparator(r):
			flushSeg()
		default:
			inTok = true
			tok.WriteRune(r)
		}
	}
	flushSeg()
	return segments
}

// commandName はセグメント先頭の環境変数代入(FOO=bar)を読み飛ばし、実行名を
// パス接頭辞を除いて返す(.agents/bin/atelier → atelier)。2 つ目の戻り値は
// 実行名を除いた引数トークン列(サブコマンド以降)。
func commandName(seg []string) (string, []string) {
	i := 0
	for i < len(seg) && isAssignment(seg[i]) {
		i++
	}
	if i >= len(seg) {
		return "", nil
	}
	return baseName(seg[i]), seg[i+1:]
}

// isAssignment は環境変数代入トークン(NAME=value)か。NAME は英数字と _ で、
// 先頭は数字でない(POSIX の name 規則の簡略)。
func isAssignment(tok string) bool {
	eq := strings.IndexByte(tok, '=')
	if eq <= 0 {
		return false
	}
	for i, r := range tok[:eq] {
		switch {
		case r == '_':
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z':
		case r >= '0' && r <= '9':
			if i == 0 {
				return false
			}
		default:
			return false
		}
	}
	return true
}

// baseName はパス接頭辞を除いた末尾要素を返す(/usr/bin/gh → gh)。
func baseName(path string) string {
	if idx := strings.LastIndexByte(path, '/'); idx >= 0 {
		return path[idx+1:]
	}
	return path
}
