// Package gate は atelier の機械的ゲート(PreToolUse hook)の判定を提供する。
//
// 判定は plugins/atelier/hooks/atelier-gate.sh からの移行であり、仕様は現行の
// bash 実装と同一(#73: 移行のみ。ゲートの追加・仕様変更はしない)。シェル側は
// バイナリを exec するだけの薄いラッパーになり、grep / jq への依存が消える。
//
// ゲート(#4 の hook 集約決定。無人 3 種は #122 で撤去 — 人間常駐前提):
//  1. main への直 push / force push: 常にブロック
//  2. push ゲート: agent/issue-<n>-* ブランチの push 前に issue の状態を検証
//  3. マージゲート: PR↔issue 整合 + Closes 紐づけ + merge:agent + CI green +
//     別コンテキストレビュア(atelier-review status = success かつ投稿者 ≠ PR 作者。
//     #116: 資格情報の同一性で独立を機械検証し、特定不能は fail-closed)
//  4. リリースゲート: リリースコマンドの起動(--dry-run を除く)とタグ push を
//     常にブロック(merge-policy: デプロイ = 人間の tag push)。release
//     サブコマンドは #129 で撤去済みだが、旧版バイナリ(factory 名義を含む)が
//     残存する環境への防御として起動検出は維持する
//
// 判定はコマンド文字列をトークン化(tokenize.go)し、セグメント(コマンド列要素)
// ごとに実コマンド(git / gh / atelier / factory)を識別して行う。文字列全体への
// 正規表現部分一致はしない(#119: コミットメッセージ内の語・連結コマンドの引数
// 混同・引用符すり抜けを構文で解消する)。トップレベルのコマンド列のみを見る保守
// 的な判定で、ネストしたコマンド(bash -c の中身・$() 置換)は追わない(ADR 0002:
// 防御は配るが完璧は求めない)。
//
// issue / pr verify はプロセス起動ではなく internal/verify の関数呼び出しで行う
// (判定一元化 — #38 と同じ方針)。ブロックの理由は呼び出し側(cli)が stderr に
// 出して exit 2 に変換する(hook 契約: exit 2 = ブロック、その他非ゼロ = 実行失敗)。
package gate

import (
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/naito-7110/claude-plugins/tools/atelier/internal/verify"
)

// Input は PreToolUse hook が stdin に渡す JSON のうち、ゲート判定に使う部分。
// CWD はツール(Bash)が実行されるディレクトリ。worktree では checkout 中の
// ブランチがルートと異なるため、カレントブランチの判定は CWD 基準で行う
// (#138。無ければ空文字 = 従来どおりプロジェクトルート判定へフォールバック)。
type Input struct {
	ToolName  string `json:"tool_name"`
	CWD       string `json:"cwd"`
	ToolInput struct {
		Command string `json:"command"` // Bash
	} `json:"tool_input"`
}

// ParseInput は hook の stdin JSON を読む。
func ParseInput(r io.Reader) (Input, error) {
	var input Input
	if err := json.NewDecoder(r).Decode(&input); err != nil {
		return Input{}, fmt.Errorf("hook 入力(JSON)を解釈できません: %w", err)
	}
	return input, nil
}

// Deps はゲート判定の実行時依存。GraphQL クライアントとリポジトリは
// 必要になるまで解決しない(main 直 push の判定は認証なしでも動く)。
type Deps struct {
	NewClient func() (verify.GraphQL, error)
	Repo      func() (string, error)
	// Branch は dir のカレントブランチ(unborn でも名前を返す実装を注入する)。
	// dir が空ならプロジェクトルート。worktree では checkout 中のブランチが
	// ディレクトリごとに異なるため、判定はコマンドの実効ディレクトリ
	// (git の -C、無ければ hook の cwd)基準で行う(#138)。
	Branch  func(dir string) (string, error)
	Managed bool      // atelier 管理下か(.atelier/ の有無。呼び出し側が解決する)
	Err     io.Writer // verify 所見の出力先(ブロック理由の判断材料)
}

// branchIssuePattern は agent/issue-<n>-* ブランチから issue 番号を取り出す。
var branchIssuePattern = regexp.MustCompile(`^agent/issue-([0-9]+)`)

// リリースタグと判定する refspec の接頭辞(atelier/v* / factory/v* / refs/tags/)。
var tagRefPrefixes = []string{"atelier/v", "factory/v", "refs/tags/"}

// Check は hook 入力を判定し、ブロックすべきなら理由(非空)を返す。
// error は判定不能な実行失敗(hook 契約では exit 2 に変換しない)。
//
// atelier 管理外(.atelier/ が無い)のリポジトリでは全ツールを許可する。
// プラグインはユーザーレベルで有効化され hook は全リポジトリで発火するため、
// スコープを切らないと無関係なリポジトリまでゲートされる(#103 の実地バグ)。
func Check(input Input, deps Deps) (string, error) {
	if !deps.Managed {
		return "", nil
	}
	switch input.ToolName {
	case "Bash":
		return checkBash(input, deps), nil
	default:
		return "", nil
	}
}

// --- Bash: push ゲート・マージゲート・リリースゲート ---
//
// コマンドをトークン化し、セグメントごとに実コマンドを識別して判定する。
// 最初にブロック理由が立ったセグメントで返す(fail-closed)。

func checkBash(input Input, deps Deps) string {
	for _, seg := range Tokenize(input.ToolInput.Command) {
		name, args := commandName(seg)
		var reason string
		switch name {
		case "atelier", "factory":
			reason = checkReleaseCmd(args)
		case "git":
			reason = checkGit(args, input.CWD, deps)
		case "gh":
			reason = checkGh(args, input.CWD, deps)
		}
		if reason != "" {
			return reason
		}
	}
	return ""
}

// checkReleaseCmd はリリースコマンド(atelier / factory release)の起動を
// ブロックする。--dry-run を含む起動は許可(検証は無害で、状態確認は正当)。
// release サブコマンドは #129 で撤去済みだが、旧版バイナリが残存する環境への
// 防御として起動検出は維持する(merge-policy: デプロイ = 人間の tag push)。
func checkReleaseCmd(args []string) string {
	if len(args) == 0 || args[0] != "release" {
		return ""
	}
	for _, a := range args {
		if a == "--dry-run" {
			return ""
		}
	}
	return "リリースタグは人間の操作です(merge-policy: デプロイ = 人間の tag push)。--dry-run での確認は可能です"
}

// checkGit は git セグメントの push を判定する。push 以外の git サブコマンドは
// 対象外(コミットメッセージ等の引数に push / main の語が入っても誤爆しない)。
//
// 判定は push の refspec(引数)から宛先・push 元ブランチを構文的に取る:
//   - タグを push する(--tags / refs/tags/ / atelier|factory/v*)→ リリースゲート
//   - 宛先が main / master → 直 push(force なら force push)をブロック
//   - 宛先または push 元が agent/issue-<n> → issue の状態を検証
//
// refspec が無い(`git push` のみ)ときは、push 元をカレントブランチに
// フォールバックする。明示的な refspec があればそれを優先する(#119: worktree
// からの push の誤判定を解消)。
//
// カレントブランチは「コマンドが実際に動くディレクトリ」で解決する(#138):
// git のグローバル -C があればそのパス(相対なら cwd 起点)、無ければ hook
// stdin JSON の cwd(= Bash の実行ディレクトリ)、それも無ければプロジェクト
// ルート。解決に失敗したらプロジェクトルートで再解決する(cwd を非リポジトリへ
// 移してから -C や cd でルートの main を push する迂回を、従来どおり宛先 main
// 判定に落とすため — fail-open にしない)。GIT_DIR / --git-dir / 連結 cd の追跡
// はしない(ADR 0002: 防御は配るが完璧は求めない)。
func checkGit(args []string, cwd string, deps Deps) string {
	sub, rest := gitSubcommand(args)
	if sub != "push" {
		return ""
	}
	force, hasTagsFlag, allRefs, positionals := parsePushArgs(rest)

	// --all / --mirror は refspec 無しでもローカルの全 ref(main を含みうる)を
	// push する。refspec 無し = カレントのみという仮定が崩れるため、直 push
	// (force なら force push)相当でブロックする(#119 セルフレビュー指摘)。
	if allRefs {
		if force {
			return "main への force push は禁止です(git-workflow)"
		}
		return "全ブランチの push(--all / --mirror)は禁止です(main を含みうる — git-workflow)"
	}

	// refspec を src / dst に分解する(remote は positionals[0]、以降が refspec)。
	var refs []string
	if len(positionals) > 1 {
		refs = positionals[1:]
	}
	// カレントブランチは HEAD 解決と refspec 無しのフォールバックに使う。
	current := resolveBranch(deps, gitEffectiveDir(args, cwd))

	var srcs, dsts []string
	tagPush := hasTagsFlag
	for _, ref := range refs {
		src, dst, refForce := splitRefspec(ref)
		if refForce {
			force = true // 先頭 + は force refspec
		}
		if hasTagRefPrefix(src) || hasTagRefPrefix(dst) {
			tagPush = true
		}
		if src != "" {
			srcs = append(srcs, normalizeRef(src, current))
		}
		if dst != "" {
			dsts = append(dsts, normalizeRef(dst, current))
		}
	}
	// refspec が無ければ push 元 = カレントブランチ(src=dst)にフォールバックする。
	if len(refs) == 0 && current != "" {
		srcs = append(srcs, current)
		dsts = append(dsts, current)
	}

	if tagPush {
		return "タグの push は人間の操作です(merge-policy: デプロイ = 人間の tag push。手順は tools/atelier/README.md のリリース節を参照)"
	}
	for _, dst := range dsts {
		if dst == "main" || dst == "master" {
			if force {
				return "main への force push は禁止です(git-workflow)"
			}
			return "main への直 push は禁止です(PR を経由してください — git-workflow)"
		}
	}
	for _, ref := range append(append([]string{}, srcs...), dsts...) {
		if m := branchIssuePattern.FindStringSubmatch(ref); m != nil {
			number, _ := strconv.Atoi(m[1])
			if !issueVerifyOK(deps, number) {
				return fmt.Sprintf("issue #%d の状態が push 条件を満たしません(ラベルなしの実装は push できません)", number)
			}
		}
	}
	return ""
}

// resolveBranch は dir のカレントブランチを返す。解決できなければプロジェクト
// ルート("")で再解決し、それも失敗なら空文字(従来の「未解決 = 判定材料なし」。
// refspec 無しの push はゲート対象外になるが、その状況では git 自体も失敗する)。
// ルートへのフォールバックは #138 以前の「常にルートで判定」の防御水準を下限と
// して維持するため(解決失敗を素通しにすると、非リポジトリな cwd を経由した
// bare push がルートの main 判定を逃れる)。
func resolveBranch(deps Deps, dir string) string {
	if br, err := deps.Branch(dir); err == nil {
		return br
	}
	if dir == "" {
		return ""
	}
	br, err := deps.Branch("")
	if err != nil {
		return ""
	}
	return br
}

// gitEffectiveDir は git セグメントのグローバル -C から、コマンドが実際に動く
// ディレクトリを求める(相対パスは cwd 起点で合成。複数の -C は git と同じく
// 順に連結する)。-C が無ければ cwd(hook stdin JSON 由来。無ければ空 =
// プロジェクトルート)。--git-dir / --work-tree は追わない(ADR 0002)。
func gitEffectiveDir(args []string, cwd string) string {
	dir := cwd
	i := 0
	for i < len(args) {
		a := args[i]
		if !strings.HasPrefix(a, "-") { // 最初の非オプション = サブコマンドで打ち切り
			break
		}
		if a == "-C" && i+1 < len(args) {
			v := args[i+1]
			if filepath.IsAbs(v) || dir == "" {
				dir = v
			} else {
				dir = filepath.Join(dir, v)
			}
			i += 2
			continue
		}
		if strings.Contains(a, "=") { // --opt=value は 1 トークン
			i++
			continue
		}
		if gitValueOpts[a] { // 値を取るオプションは値ごと読み飛ばす
			i += 2
			continue
		}
		i++
	}
	return dir
}

// gitValueOpts は git のグローバルオプションのうち、次のトークンを値として取るもの
// (`--opt=value` の = 形式は 1 トークンなので別扱い)。
var gitValueOpts = map[string]bool{
	"-C": true, "-c": true, "--git-dir": true, "--work-tree": true,
	"--namespace": true, "--exec-path": true, "--super-prefix": true,
}

// gitSubcommand は `git [グローバルオプション...] <サブコマンド> [引数...]` から
// サブコマンドと以降の引数を返す。グローバルオプション(-C <path> / -c <k=v> /
// --no-pager 等)を読み飛ばしてから最初の非オプショントークンをサブコマンドとする
// (#119: `git -C <worktree> push origin main` の取りこぼしを防ぐ)。
func gitSubcommand(args []string) (string, []string) {
	i := 0
	for i < len(args) {
		a := args[i]
		if !strings.HasPrefix(a, "-") {
			return a, args[i+1:]
		}
		if strings.Contains(a, "=") { // --opt=value は 1 トークン
			i++
			continue
		}
		if gitValueOpts[a] { // -C <path> 等は次トークンが値
			i += 2
			continue
		}
		i++ // 値なしフラグ(--no-pager 等)
	}
	return "", nil
}

// parsePushArgs は push の引数を force / --tags / --all|--mirror / 位置引数へ分ける。
// 単一ダッシュの結合短縮フラグ(-fu 等)も f を含めば force とみなす。
func parsePushArgs(args []string) (force, hasTags, allRefs bool, positionals []string) {
	for _, a := range args {
		switch {
		case a == "--force" || a == "--force-with-lease" || a == "--force-if-includes" ||
			strings.HasPrefix(a, "--force-with-lease="):
			force = true
		case a == "--tags":
			hasTags = true
		case a == "--all" || a == "--mirror":
			allRefs = true
		case strings.HasPrefix(a, "--"):
			// その他の long フラグは判定に使わない(--set-upstream 等)。
		case strings.HasPrefix(a, "-") && len(a) > 1:
			// 単一ダッシュの短縮フラグ(結合可: -fu = -f -u)。f を含めば force。
			if strings.ContainsRune(a[1:], 'f') {
				force = true
			}
		default:
			positionals = append(positionals, a)
		}
	}
	return force, hasTags, allRefs, positionals
}

// splitRefspec は refspec を src / dst / force(先頭 +)に分ける。
// `+src:dst` はコロンで分割し先頭 + を force とする。コロンが無ければ src = dst。
func splitRefspec(ref string) (src, dst string, force bool) {
	if strings.HasPrefix(ref, "+") {
		force = true
		ref = ref[1:]
	}
	if i := strings.IndexByte(ref, ':'); i >= 0 {
		return ref[:i], ref[i+1:], force
	}
	return ref, ref, force
}

// normalizeRef はブランチ参照を比較用に正規化する。refs/heads/ 接頭辞を畳み、
// HEAD はカレントブランチに解決する(#119: refs/heads/main や HEAD:main、
// カレント main での HEAD が main 宛て判定を漏れるのを防ぐ)。
func normalizeRef(ref, current string) string {
	ref = strings.TrimPrefix(ref, "refs/heads/")
	if (ref == "HEAD" || ref == "@") && current != "" { // @ は HEAD の同義語
		return current
	}
	return ref
}

func hasTagRefPrefix(ref string) bool {
	for _, p := range tagRefPrefixes {
		if strings.HasPrefix(ref, p) {
			return true
		}
	}
	return false
}

// checkGh は gh セグメントのマージ(gh pr merge / gh api .../pulls/<n>/merge)を
// 検出し、マージゲートに掛ける。番号がコマンドに無い(gh pr merge --squash 等)
// 場合も、マージコマンドと判定したらゲートに入れる(番号はカレントブランチの
// PR にフォールバックし、解決できなければ fail-closed でブロック)。
func checkGh(args []string, cwd string, deps Deps) string {
	args = ghSubArgs(args)
	if !isGhMerge(args) {
		return ""
	}
	return checkMerge(parseGhMergeNumber(args), cwd, deps)
}

// ghSubArgs は gh のグローバルフラグ(-R/--repo <value> 等)を読み飛ばして、
// サブコマンド以降の引数を返す(#119: `gh -R o/r pr merge 5` の取りこぼし防止)。
func ghSubArgs(args []string) []string {
	i := 0
	for i < len(args) {
		a := args[i]
		if !strings.HasPrefix(a, "-") {
			return args[i:]
		}
		if strings.Contains(a, "=") { // --repo=o/r は 1 トークン
			i++
			continue
		}
		if a == "-R" || a == "--repo" { // 値を取るグローバルフラグ
			i += 2
			continue
		}
		i++
	}
	return nil
}

// isGhMerge は gh のマージコマンドか(番号の有無に依らない)。
//   - gh pr merge [<n>] [flags]
//   - gh api ... <path が /pulls/<n>/merge を含む>
func isGhMerge(args []string) bool {
	if len(args) >= 2 && args[0] == "pr" && args[1] == "merge" {
		return true
	}
	if len(args) >= 1 && args[0] == "api" {
		for _, a := range args[1:] {
			if mergePathNumber(a) != 0 {
				return true
			}
		}
	}
	return false
}

// parseGhMergeNumber は gh のマージコマンドから PR 番号を返す(取れなければ 0)。
func parseGhMergeNumber(args []string) int {
	if len(args) >= 3 && args[0] == "pr" && args[1] == "merge" {
		return atoiLoose(args[2])
	}
	if len(args) >= 1 && args[0] == "api" {
		for _, a := range args[1:] {
			if n := mergePathNumber(a); n != 0 {
				return n
			}
		}
	}
	return 0
}

// mergePathNumber は "repos/o/r/pulls/64/merge"(/pull/ も可)から 64 を返す。
func mergePathNumber(path string) int {
	for _, seg := range []string{"/pulls/", "/pull/"} {
		i := strings.Index(path, seg)
		if i < 0 {
			continue
		}
		rest := path[i+len(seg):]
		j := strings.IndexByte(rest, '/')
		if j < 0 {
			continue
		}
		if !strings.HasPrefix(rest[j:], "/merge") {
			continue
		}
		return atoiLoose(rest[:j])
	}
	return 0
}

// atoiLoose は先頭の # を落として数値化する(#64 → 64)。非数値は 0。
func atoiLoose(s string) int {
	s = strings.TrimPrefix(s, "#")
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0
	}
	return n
}

// checkMerge はマージゲート。PR 番号を受け取り(0 ならカレントブランチ —
// hook cwd 基準(#138)— の PR)、pr verify → Closes 紐づけ → merge:agent →
// CI green → atelier-review green → レビュア投稿者 ≠ PR 作者の順に確認する
// (fail-closed: 確認できないものはブロック)。
//
// 投稿者検証(#116)は merge-policy の「独立とは支配の非保有」のうち (d) 資格
// 情報の機械検証: status が green でも、投稿した資格情報が PR 作者と同一なら
// 独立レビューとは認めない。投稿者・作者のどちらかでも特定できなければ同一性を
// 検証できないため fail-closed でブロックする(検証できない体制では agent
// レーンを開かない)。
func checkMerge(number int, cwd string, deps Deps) string {
	client, repo, err := resolveClient(deps)
	if err != nil {
		return fmt.Sprintf("マージゲートを実行できないため停止します(%v)", err)
	}

	if number == 0 {
		number = currentPRNumber(client, repo, cwd, deps)
	}
	if number == 0 {
		return "マージ対象の PR 番号を特定できません"
	}

	report, err := verify.PR(client, repo, number, verify.AllChecks, nil)
	if err != nil || !report.OK() {
		printPRFindings(deps.Err, report)
		if err != nil {
			fmt.Fprintln(deps.Err, err)
		}
		return fmt.Sprintf("PR #%d は PR↔issue 整合を満たしません(上記の理由)", number)
	}
	printPRFindings(deps.Err, report)

	status, err := fetchMergeStatus(client, repo, number)
	if err != nil {
		return fmt.Sprintf("マージゲートを実行できないため停止します(%v)", err)
	}
	if status.LinkedIssue == 0 {
		return fmt.Sprintf("PR #%d に Closes での issue 紐づけがありません(agent マージは不可)", number)
	}
	if !hasMergeAgentLabel(client, repo, status.LinkedIssue) {
		return fmt.Sprintf("issue #%d に merge:agent がありません。人間のレビュー・マージを待ってください(merge-policy)", status.LinkedIssue)
	}
	if status.ChecksState != "SUCCESS" {
		return fmt.Sprintf("PR #%d の CI が green ではありません(merge-policy の実行条件)", number)
	}
	if status.ReviewState != "SUCCESS" {
		return fmt.Sprintf("PR #%d は別コンテキストレビュア(atelier-review)の green がありません(merge-policy の実行条件)", number)
	}
	if status.ReviewerLogin == "" || status.AuthorLogin == "" {
		return fmt.Sprintf("PR #%d は atelier-review の投稿者を検証できません(投稿者または PR 作者が特定できない — 独立を検証できない体制では agent マージ不可。merge-policy)", number)
	}
	if status.ReviewerLogin == status.AuthorLogin {
		return fmt.Sprintf("PR #%d の atelier-review は PR 作者と同一アカウント(%s)の投稿です(独立レビューではありません — merge-policy の実行条件)", number, status.ReviewerLogin)
	}
	return ""
}

func resolveClient(deps Deps) (verify.GraphQL, string, error) {
	repo, err := deps.Repo()
	if err != nil {
		return nil, "", fmt.Errorf("カレントリポジトリを解決できません: %w", err)
	}
	client, err := deps.NewClient()
	if err != nil {
		return nil, "", err
	}
	return client, repo, nil
}

func issueVerifyOK(deps Deps, number int) bool {
	client, repo, err := resolveClient(deps)
	if err != nil {
		fmt.Fprintln(deps.Err, err)
		return false
	}
	report, err := verify.Issue(client, repo, number, verify.AllChecks)
	if err != nil {
		fmt.Fprintln(deps.Err, err)
		return false
	}
	printFindings(deps.Err, report.Findings)
	return report.OK()
}

func printFindings(w io.Writer, findings []verify.Finding) {
	for _, finding := range findings {
		fmt.Fprintf(w, "%s: %s\n", finding.Level, finding.Message)
	}
}

func printPRFindings(w io.Writer, report verify.PRReport) {
	printFindings(w, report.Findings)
	for _, issue := range report.Issues {
		fmt.Fprintf(w, "==> 関連 issue #%d の検証\n", issue.Number)
		printFindings(w, issue.Findings)
	}
}

// --- マージゲートの GraphQL クエリ ---

// mergeStatusQuery は Closes 紐づけ・CI rollup・atelier-review status(state と
// 投稿者)・PR 作者を 1 回で取る。statusCheckRollup が SUCCESS 以外(FAILURE /
// PENDING / 無し)は bash 版の `gh pr checks` 非ゼロと同じくブロック対象。
// creator / author はアカウント削除等で null になりうる(その場合は fail-closed)。
const mergeStatusQuery = `query($owner: String!, $name: String!, $number: Int!) {
  repository(owner: $owner, name: $name) {
    pullRequest(number: $number) {
      author { login }
      closingIssuesReferences(first: 1) { nodes { number } }
      commits(last: 1) {
        nodes {
          commit {
            statusCheckRollup { state }
            status { context(name: "atelier-review") { state creator { login } } }
          }
        }
      }
    }
  }
}`

const prByBranchQuery = `query($owner: String!, $name: String!, $branch: String!) {
  repository(owner: $owner, name: $name) {
    pullRequests(first: 1, states: OPEN, headRefName: $branch) { nodes { number } }
  }
}`

const issueLabelsQuery = `query($owner: String!, $name: String!, $number: Int!) {
  repository(owner: $owner, name: $name) {
    issue(number: $number) {
      labels(first: 100) { nodes { name } }
    }
  }
}`

type mergeStatus struct {
	LinkedIssue   int
	ChecksState   string // statusCheckRollup.state("" = check なし)
	ReviewState   string // atelier-review context の state("" = status なし)
	AuthorLogin   string // PR 作者の login("" = 特定不能)
	ReviewerLogin string // atelier-review status の投稿者 login("" = 特定不能)
}

func splitRepo(repo string) (string, string, error) {
	owner, name, ok := strings.Cut(repo, "/")
	if !ok || owner == "" || name == "" {
		return "", "", fmt.Errorf("リポジトリは owner/name 形式で指定してください: %s", repo)
	}
	return owner, name, nil
}

func fetchMergeStatus(client verify.GraphQL, repo string, number int) (mergeStatus, error) {
	owner, name, err := splitRepo(repo)
	if err != nil {
		return mergeStatus{}, err
	}
	var resp struct {
		Repository *struct {
			PullRequest *struct {
				Author *struct {
					Login string `json:"login"`
				} `json:"author"`
				ClosingIssuesReferences struct {
					Nodes []struct {
						Number int `json:"number"`
					} `json:"nodes"`
				} `json:"closingIssuesReferences"`
				Commits struct {
					Nodes []struct {
						Commit struct {
							StatusCheckRollup *struct {
								State string `json:"state"`
							} `json:"statusCheckRollup"`
							Status *struct {
								Context *struct {
									State   string `json:"state"`
									Creator *struct {
										Login string `json:"login"`
									} `json:"creator"`
								} `json:"context"`
							} `json:"status"`
						} `json:"commit"`
					} `json:"nodes"`
				} `json:"commits"`
			} `json:"pullRequest"`
		} `json:"repository"`
	}
	vars := map[string]interface{}{"owner": owner, "name": name, "number": number}
	if err := client.Do(mergeStatusQuery, vars, &resp); err != nil {
		return mergeStatus{}, fmt.Errorf("PR #%d の状態を取得できません: %w", number, err)
	}
	if resp.Repository == nil || resp.Repository.PullRequest == nil {
		return mergeStatus{}, fmt.Errorf("PR #%d が見つかりません", number)
	}
	pr := resp.Repository.PullRequest
	status := mergeStatus{}
	if pr.Author != nil {
		status.AuthorLogin = pr.Author.Login
	}
	if len(pr.ClosingIssuesReferences.Nodes) > 0 {
		status.LinkedIssue = pr.ClosingIssuesReferences.Nodes[0].Number
	}
	if len(pr.Commits.Nodes) > 0 {
		commit := pr.Commits.Nodes[0].Commit
		if commit.StatusCheckRollup != nil {
			status.ChecksState = commit.StatusCheckRollup.State
		}
		if commit.Status != nil && commit.Status.Context != nil {
			status.ReviewState = commit.Status.Context.State
			if commit.Status.Context.Creator != nil {
				status.ReviewerLogin = commit.Status.Context.Creator.Login
			}
		}
	}
	return status, nil
}

// currentPRNumber はカレントブランチ(hook cwd 基準。#138)の OPEN な PR 番号を
// 返す(bash 版の `gh pr view` フォールバックに対応)。解決できなければ 0。
func currentPRNumber(client verify.GraphQL, repo, cwd string, deps Deps) int {
	branch := resolveBranch(deps, cwd)
	if branch == "" {
		return 0
	}
	owner, name, err := splitRepo(repo)
	if err != nil {
		return 0
	}
	var resp struct {
		Repository *struct {
			PullRequests struct {
				Nodes []struct {
					Number int `json:"number"`
				} `json:"nodes"`
			} `json:"pullRequests"`
		} `json:"repository"`
	}
	vars := map[string]interface{}{"owner": owner, "name": name, "branch": branch}
	if err := client.Do(prByBranchQuery, vars, &resp); err != nil {
		return 0
	}
	if resp.Repository == nil || len(resp.Repository.PullRequests.Nodes) == 0 {
		return 0
	}
	return resp.Repository.PullRequests.Nodes[0].Number
}

func hasMergeAgentLabel(client verify.GraphQL, repo string, number int) bool {
	owner, name, err := splitRepo(repo)
	if err != nil {
		return false
	}
	var resp struct {
		Repository *struct {
			Issue *struct {
				Labels struct {
					Nodes []struct {
						Name string `json:"name"`
					} `json:"nodes"`
				} `json:"labels"`
			} `json:"issue"`
		} `json:"repository"`
	}
	vars := map[string]interface{}{"owner": owner, "name": name, "number": number}
	if err := client.Do(issueLabelsQuery, vars, &resp); err != nil {
		return false
	}
	if resp.Repository == nil || resp.Repository.Issue == nil {
		return false
	}
	for _, node := range resp.Repository.Issue.Labels.Nodes {
		if node.Name == "merge:agent" {
			return true
		}
	}
	return false
}
