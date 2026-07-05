// Package docs は文書構造(documentation プリセット)の機械検証を提供する。
//
// documentation プリセット(plugins/factory/adr/documentation.md)は、文書の
// 階層(憲法 / ドメイン知識 / 事実 / 地図)と所有マップ(パス → ドメイン)の
// 機械検証を CI に乗せることを決定している。本パッケージがその判定の実体で、
// issue / pr verify と同じく、hook / GHA の薄い入口から呼ばれる
// (二重実装を持たない。判定はここに一本化する)。
//
// 検査項目:
//   - 地図の存在: docs/factory/README.md(無ければ /factory:init を案内)
//   - 所有マップの形式: docs/factory/ownership.yml が domains.<name>.paths: [glob]
//     に適合すること。domains: {}(ドメイン未分割)は正常(漸進導入)
//   - 必須構造: 宣言された各ドメインの docs/domains/<domain>/README.md と contracts.md
//   - マップと実パスの整合: glob が実在ファイルに 1 件もマッチしない宣言は NG
//     (死んだ宣言)。どのドメインにも属さない追跡対象ファイルは警告のみ
//     (exit code に影響させない。所有の宣言は 1 ドメインから漸進導入できるため)
//
// ファイル列挙は git ls-files(untracked 含む・gitignore 除外)を優先する。
// git リポジトリでない場合はファイルシステム走査(dot ディレクトリ除外)に
// フォールバックする。glob の ** は 0 個以上のパスセグメントに一致する
// (依存最小のため doublestar 等は足さず自前で展開する)。
package docs

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/naito-7110/claude-plugins/tools/factory/internal/verify"
)

// 検査項目の名前。
const (
	CheckMap       = "docs-map"        // 地図(docs/factory/README.md)の存在
	CheckOwnership = "ownership"       // 所有マップ(ownership.yml)の形式
	CheckStructure = "domain-docs"     // ドメイン文書(README.md / contracts.md)の存在
	CheckPaths     = "ownership-paths" // 所有マップと実パスの整合
)

// 検証対象の相対パス(documentation プリセット / init scaffold の正準配置)。
const (
	mapPath       = "docs/factory/README.md"
	ownershipPath = "docs/factory/ownership.yml"
	domainsDir    = "docs/domains"
)

// initGuidance は scaffold 欠落時の案内(init が生成する)。
const initGuidance = "/factory:init を実行して文書の地図を生成してください"

// coverageExamples は所有されないファイルの警告に載せる例の上限。
const coverageExamples = 5

// Report は文書構造の検証結果。
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

// domainDecl は所有マップの 1 ドメイン分の宣言。
type domainDecl struct {
	Paths []string `yaml:"paths"`
}

// ownershipDoc は ownership.yml のスキーマ(domains.<name>.paths: [glob])。
type ownershipDoc struct {
	Domains map[string]domainDecl `yaml:"domains"`
}

// Verify は root(リポジトリのルート)の文書構造を検証する。
// error は検証の成否ではなく、root 自体が読めない等の実行失敗を表す。
func Verify(root string) (Report, error) {
	info, err := os.Stat(root)
	if err != nil {
		return Report{}, fmt.Errorf("root を読めません: %w", err)
	}
	if !info.IsDir() {
		return Report{}, fmt.Errorf("root がディレクトリではありません: %s", root)
	}
	report := Report{Root: root}

	// 地図の存在。所有マップと独立に検査する(欠落は両方報告する)。
	if fileExists(filepath.Join(root, filepath.FromSlash(mapPath))) {
		report.add(CheckMap, verify.LevelOK, "%s(文書の地図)あり", mapPath)
	} else {
		report.add(CheckMap, verify.LevelNG,
			"%s(文書の地図)がありません(%s)", mapPath, initGuidance)
	}

	// 所有マップの形式。パースできなければ以降の検査は成立しない。
	domains, ok := loadOwnership(&report, root)
	if !ok {
		return report, nil
	}
	if len(domains) == 0 {
		report.add(CheckOwnership, verify.LevelOK,
			"%s は正常(ドメイン未分割: domains: {}。漸進導入のため正常)", ownershipPath)
		return report, nil
	}
	names := make([]string, 0, len(domains))
	for name := range domains {
		names = append(names, name)
	}
	sort.Strings(names)
	report.add(CheckOwnership, verify.LevelOK,
		"%s は正常(ドメイン %d 件: %s)", ownershipPath, len(names), strings.Join(names, ", "))

	checkStructure(&report, root, names)

	files, err := listTrackedFiles(root)
	if err != nil {
		return Report{}, fmt.Errorf("ファイル一覧を取得できません: %w", err)
	}
	checkPaths(&report, domains, names, files)
	return report, nil
}

// loadOwnership は ownership.yml を読み、形式検証の所見を report に積む。
// 以降の検査を続けられるときだけ ok = true を返す。
func loadOwnership(report *Report, root string) (map[string]domainDecl, bool) {
	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(ownershipPath)))
	if errors.Is(err, fs.ErrNotExist) {
		report.add(CheckOwnership, verify.LevelNG,
			"%s(所有マップ)がありません(%s)", ownershipPath, initGuidance)
		return nil, false
	}
	if err != nil {
		report.add(CheckOwnership, verify.LevelNG, "%s を読めません: %v", ownershipPath, err)
		return nil, false
	}

	var doc ownershipDoc
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true) // スキーマ外のキーは形式違反として弾く
	if err := decoder.Decode(&doc); err != nil && !errors.Is(err, io.EOF) {
		report.add(CheckOwnership, verify.LevelNG,
			"%s をパースできません(期待する形式: domains.<name>.paths: [glob]): %v",
			ownershipPath, err)
		return nil, false
	}
	if doc.Domains == nil {
		report.add(CheckOwnership, verify.LevelNG,
			"%s に domains キーがありません(ドメイン未分割でも domains: {} を宣言してください)",
			ownershipPath)
		return nil, false
	}

	valid := true
	for name, decl := range doc.Domains {
		if name == "" || name != path.Clean(name) || strings.ContainsAny(name, "/\\") {
			report.add(CheckOwnership, verify.LevelNG,
				"ドメイン名 %q が不正です(docs/domains/ のディレクトリ名になるため、パス区切りを含められません)", name)
			valid = false
			continue
		}
		if len(decl.Paths) == 0 {
			report.add(CheckOwnership, verify.LevelNG,
				"ドメイン %s に paths がありません(所有するパスの glob を 1 件以上宣言してください)", name)
			valid = false
		}
		for _, pattern := range decl.Paths {
			if strings.TrimSpace(pattern) == "" {
				report.add(CheckOwnership, verify.LevelNG,
					"ドメイン %s の paths に空のパターンがあります", name)
				valid = false
			}
		}
	}
	if !valid {
		return nil, false
	}
	return doc.Domains, true
}

// checkStructure は宣言された各ドメインの必須文書
// (docs/domains/<domain>/README.md と contracts.md)の存在を検査する。
func checkStructure(report *Report, root string, names []string) {
	for _, name := range names {
		for _, doc := range []string{"README.md", "contracts.md"} {
			rel := path.Join(domainsDir, name, doc)
			if fileExists(filepath.Join(root, filepath.FromSlash(rel))) {
				report.add(CheckStructure, verify.LevelOK, "%s あり", rel)
			} else {
				report.add(CheckStructure, verify.LevelNG,
					"%s がありません(所有マップに宣言されたドメインには必須の文書です)", rel)
			}
		}
	}
}

// checkPaths は所有マップと実パスの整合を検査する。
//   - 実在ファイルに 1 件もマッチしないパターン → NG(死んだ宣言)
//   - どのドメインにも属さない追跡対象のソースファイル → 警告のみ(漸進導入)
func checkPaths(report *Report, domains map[string]domainDecl, names []string, files []string) {
	owned := make([]bool, len(files))
	for _, name := range names {
		for _, pattern := range domains[name].Paths {
			matched := 0
			for i, file := range files {
				if matchPattern(pattern, file) {
					owned[i] = true
					matched++
				}
			}
			if matched == 0 {
				report.add(CheckPaths, verify.LevelNG,
					"ドメイン %s のパターン %q に一致するファイルがありません(死んだ宣言。パスの実態に合わせて所有マップを更新してください)",
					name, pattern)
			} else {
				report.add(CheckPaths, verify.LevelOK,
					"ドメイン %s のパターン %q は %d ファイルに一致", name, pattern, matched)
			}
		}
	}

	var unowned []string
	for i, file := range files {
		if !owned[i] && isSourceFile(file) {
			unowned = append(unowned, file)
		}
	}
	if len(unowned) == 0 {
		report.add(CheckPaths, verify.LevelOK, "すべての追跡対象ソースファイルがいずれかのドメインに属しています")
		return
	}
	examples := unowned
	suffix := ""
	if len(examples) > coverageExamples {
		examples = examples[:coverageExamples]
		suffix = " ..."
	}
	report.add(CheckPaths, verify.LevelWarn,
		"どのドメインにも属さないファイルが %d 件あります(例: %s%s)。所有マップへの追加を検討してください(漸進導入のため警告のみ)",
		len(unowned), strings.Join(examples, ", "), suffix)
}

// isSourceFile は所有カバレッジの警告対象かを判定する。
// 文書(docs/ 配下・Markdown)と dot 始まりのパス(.github 等の設定)は
// 所有マップの対象外とする。
func isSourceFile(file string) bool {
	if strings.HasSuffix(file, ".md") {
		return false
	}
	for _, segment := range strings.Split(file, "/") {
		if strings.HasPrefix(segment, ".") {
			return false
		}
	}
	return !strings.HasPrefix(file, "docs/")
}

// matchPattern は所有マップの glob パターンとファイルの相対パス(/ 区切り)を
// 突き合わせる。** は 0 個以上のパスセグメントに、* / ? / [] はセグメント内で
// path.Match の規則に従って一致する。ワイルドカードを含まないパターンは
// ディレクトリ前置(pattern/ 配下すべて)としても一致する。
func matchPattern(pattern, file string) bool {
	pattern = strings.Trim(pattern, "/")
	if !strings.ContainsAny(pattern, "*?[") &&
		(pattern == file || strings.HasPrefix(file, pattern+"/")) {
		return true
	}
	return matchSegments(strings.Split(pattern, "/"), strings.Split(file, "/"))
}

func matchSegments(pattern, segments []string) bool {
	if len(pattern) == 0 {
		return len(segments) == 0
	}
	if pattern[0] == "**" {
		for skip := 0; skip <= len(segments); skip++ {
			if matchSegments(pattern[1:], segments[skip:]) {
				return true
			}
		}
		return false
	}
	if len(segments) == 0 {
		return false
	}
	if ok, err := path.Match(pattern[0], segments[0]); err != nil || !ok {
		return false
	}
	return matchSegments(pattern[1:], segments[1:])
}

// listTrackedFiles は root 配下の追跡対象ファイル(root からの相対 / 区切り)を
// 列挙する。git リポジトリなら ls-files(untracked 含む・gitignore 除外)を使い、
// そうでなければファイルシステム走査(dot 始まりの名前を除外)にフォールバックする。
func listTrackedFiles(root string) ([]string, error) {
	out, err := exec.Command("git", "-C", root, "ls-files", "-z",
		"--cached", "--others", "--exclude-standard").Output()
	if err == nil {
		var files []string
		for _, file := range strings.Split(string(out), "\x00") {
			if file != "" {
				files = append(files, file)
			}
		}
		return files, nil
	}
	return walkFiles(root)
}

func walkFiles(root string) ([]string, error) {
	var files []string
	err := filepath.WalkDir(root, func(p string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if p == root {
			return nil
		}
		if strings.HasPrefix(entry.Name(), ".") {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if !entry.IsDir() {
			rel, err := filepath.Rel(root, p)
			if err != nil {
				return err
			}
			files = append(files, filepath.ToSlash(rel))
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return files, nil
}

func fileExists(p string) bool {
	info, err := os.Stat(p)
	return err == nil && !info.IsDir()
}
