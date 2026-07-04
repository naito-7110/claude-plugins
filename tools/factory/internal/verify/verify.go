// Package verify は issue / PR の整合検証を提供する。
//
// spec-alignment / merge-policy(plugins/factory/adr/)の機械検証可能な部分
// (受け入れ条件の形式・ラベル状態・依存の解消・merge:agent の鮮度)を検査する。
// 検証の入口は hook と GHA required check の 2 つあるが、判定の実体は本パッケージに
// 一本化し、二重実装を持たない。出力は人間可読の理由つきで、呼び出し側が
// 非ゼロ exit(hook の exit 2 変換・check summary)に変換する。
//
// GitHub GraphQL API はプロセス境界であり、GraphQL interface で抽象化する。
// テストではこの境界に fake を注入する(状態検証主義: tdd-doctrine)。
//
// 鮮度検査に issue の updatedAt を使わないのは意図的である: updatedAt は
// コメント追加でも更新され、本文編集と区別できない。本文編集のみを追跡する
// lastEditedAt と、timeline の LabeledEvent(付与時刻)を突き合わせる。
package verify

import (
	"fmt"
	"path"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// GraphQL は GitHub GraphQL API へのプロセス境界。
// go-gh の api.GraphQLClient がこの interface を満たす。
type GraphQL interface {
	Do(query string, variables map[string]interface{}, response interface{}) error
}

// 検査項目の名前(--checks フラグで選択する)。
const (
	CheckAcceptance = "acceptance" // 受け入れ条件チェックリストの存在・非空
	CheckLabels     = "labels"     // agent-ok あり / needs-human なし / agent-wip は情報
	CheckDeps       = "deps"       // 「依存: #N」の解析と依存 issue のクローズ状態
	CheckFreshness  = "freshness"  // merge:agent 付与後に本文が編集されていないか

	// pr verify 固有の検査(--checks の対象外で常に実行される)。
	CheckIssueRefs    = "issue-refs"    // Closes/Refs #N の解析
	CheckDepManifests = "dep-manifests" // 依存マニフェスト変更時の issue 明記
)

// AllChecks は issue verify の検査項目の既定(全部)。
var AllChecks = []string{CheckAcceptance, CheckLabels, CheckDeps, CheckFreshness}

// 検査対象のラベル名。
const (
	labelAgentOK    = "agent-ok"
	labelNeedsHuman = "needs-human"
	labelAgentWIP   = "agent-wip"
	labelMergeAgent = "merge:agent"
)

// Level は所見の重さ。NG が 1 つでもあれば検証は失敗(非ゼロ exit)になる。
type Level string

// 所見のレベル。OK / NG がゲートで、Info は判断材料の表示のみ。
const (
	LevelOK   Level = "OK"
	LevelNG   Level = "NG"
	LevelInfo Level = "情報"
)

// Finding は 1 件の所見(検査項目・レベル・人間可読の理由)。
type Finding struct {
	Check   string
	Level   Level
	Message string
}

// Report は issue 1 件の検証結果。
type Report struct {
	Repo     string
	Number   int
	Findings []Finding
}

func (r *Report) add(check string, level Level, format string, args ...interface{}) {
	r.Findings = append(r.Findings, Finding{
		Check: check, Level: level, Message: fmt.Sprintf(format, args...),
	})
}

// NGCount は NG の所見数を返す。
func (r Report) NGCount() int {
	return countNG(r.Findings)
}

// OK は NG の所見がないとき true。
func (r Report) OK() bool {
	return r.NGCount() == 0
}

// PRReport は PR 1 件の検証結果。Findings は PR 自体の所見
// (関連 issue の解析・依存マニフェスト検査)、Issues は関連 issue ごとの結果。
type PRReport struct {
	Repo     string
	Number   int
	Findings []Finding
	Issues   []Report
}

func (r *PRReport) add(check string, level Level, format string, args ...interface{}) {
	r.Findings = append(r.Findings, Finding{
		Check: check, Level: level, Message: fmt.Sprintf(format, args...),
	})
}

// NGCount は PR 自体と関連 issue の NG の合計を返す。
func (r PRReport) NGCount() int {
	total := countNG(r.Findings)
	for _, issue := range r.Issues {
		total += issue.NGCount()
	}
	return total
}

// OK は NG の所見がないとき true。
func (r PRReport) OK() bool {
	return r.NGCount() == 0
}

func countNG(findings []Finding) int {
	count := 0
	for _, f := range findings {
		if f.Level == LevelNG {
			count++
		}
	}
	return count
}

// --- GraphQL クエリ ---

// timelineItems は LABELED_EVENT のみに絞る(鮮度検査で付与時刻を特定するため)。
// first: 250 は GraphQL の上限。これを超える付与イベントは実運用で想定しない。
const issueQuery = `query($owner: String!, $name: String!, $number: Int!) {
  repository(owner: $owner, name: $name) {
    issue(number: $number) {
      state
      body
      lastEditedAt
      labels(first: 100) { nodes { name } }
      timelineItems(itemTypes: [LABELED_EVENT], first: 250) {
        nodes { ... on LabeledEvent { createdAt label { name } } }
      }
    }
  }
}`

const prQuery = `query($owner: String!, $name: String!, $number: Int!) {
  repository(owner: $owner, name: $name) {
    pullRequest(number: $number) {
      body
      files(first: 100) { nodes { path } }
    }
  }
}`

type labelEvent struct {
	Label     string
	CreatedAt string
}

type issueData struct {
	State        string
	Body         string
	LastEditedAt string // 空 = 本文編集なし(null)
	Labels       []string
	LabelEvents  []labelEvent
}

type issueResponse struct {
	Repository *struct {
		Issue *struct {
			State        string  `json:"state"`
			Body         string  `json:"body"`
			LastEditedAt *string `json:"lastEditedAt"`
			Labels       struct {
				Nodes []struct {
					Name string `json:"name"`
				} `json:"nodes"`
			} `json:"labels"`
			TimelineItems struct {
				Nodes []struct {
					CreatedAt string `json:"createdAt"`
					Label     *struct {
						Name string `json:"name"`
					} `json:"label"`
				} `json:"nodes"`
			} `json:"timelineItems"`
		} `json:"issue"`
	} `json:"repository"`
}

type prResponse struct {
	Repository *struct {
		PullRequest *struct {
			Body  string `json:"body"`
			Files struct {
				Nodes []struct {
					Path string `json:"path"`
				} `json:"nodes"`
			} `json:"files"`
		} `json:"pullRequest"`
	} `json:"repository"`
}

func splitRepo(repo string) (owner, name string, err error) {
	owner, name, ok := strings.Cut(repo, "/")
	if !ok || owner == "" || name == "" {
		return "", "", fmt.Errorf("リポジトリは owner/name 形式で指定してください: %s", repo)
	}
	return owner, name, nil
}

func fetchIssue(client GraphQL, owner, name string, number int) (*issueData, error) {
	var resp issueResponse
	vars := map[string]interface{}{"owner": owner, "name": name, "number": number}
	if err := client.Do(issueQuery, vars, &resp); err != nil {
		return nil, fmt.Errorf("issue #%d の取得に失敗しました: %w", number, err)
	}
	if resp.Repository == nil || resp.Repository.Issue == nil {
		return nil, fmt.Errorf("%s/%s の issue #%d が見つかりません", owner, name, number)
	}
	raw := resp.Repository.Issue
	data := &issueData{State: raw.State, Body: raw.Body}
	if raw.LastEditedAt != nil {
		data.LastEditedAt = *raw.LastEditedAt
	}
	for _, node := range raw.Labels.Nodes {
		data.Labels = append(data.Labels, node.Name)
	}
	for _, node := range raw.TimelineItems.Nodes {
		if node.Label == nil {
			continue
		}
		data.LabelEvents = append(data.LabelEvents, labelEvent{
			Label: node.Label.Name, CreatedAt: node.CreatedAt,
		})
	}
	return data, nil
}

// Issue は issue 1 件に指定された検査を実行する。
func Issue(client GraphQL, repo string, number int, checks []string) (Report, error) {
	owner, name, err := splitRepo(repo)
	if err != nil {
		return Report{}, err
	}
	data, err := fetchIssue(client, owner, name, number)
	if err != nil {
		return Report{}, err
	}
	return issueChecks(client, owner, name, repo, number, data, checks), nil
}

func issueChecks(client GraphQL, owner, name, repo string, number int, data *issueData, checks []string) Report {
	report := Report{Repo: repo, Number: number}
	enabled := map[string]bool{}
	for _, check := range checks {
		enabled[check] = true
	}
	if enabled[CheckAcceptance] {
		checkAcceptance(&report, data.Body)
	}
	if enabled[CheckLabels] {
		checkLabels(&report, data.Labels)
	}
	if enabled[CheckDeps] {
		checkDeps(&report, client, owner, name, data.Body)
	}
	if enabled[CheckFreshness] {
		checkFreshness(&report, data)
	}
	return report
}

// --- acceptance: 受け入れ条件チェックリストの存在・非空 ---

var (
	acceptanceHeading = regexp.MustCompile(`^#{1,6}\s*受け入れ条件`)
	anyHeading        = regexp.MustCompile(`^#{1,6}\s`)
	checklistItem     = regexp.MustCompile(`^\s*[-*]\s*\[[ xX]\]\s*\S`)
)

func checkAcceptance(report *Report, body string) {
	headingFound := false
	inSection := false
	items := 0
	for _, line := range strings.Split(body, "\n") {
		if anyHeading.MatchString(line) {
			inSection = acceptanceHeading.MatchString(line)
			if inSection {
				headingFound = true
			}
			continue
		}
		if inSection && checklistItem.MatchString(line) {
			items++
		}
	}
	switch {
	case !headingFound:
		report.add(CheckAcceptance, LevelNG,
			"「受け入れ条件」見出しが issue 本文にありません(spec-alignment: 受け入れ条件が唯一の完了定義)")
	case items == 0:
		report.add(CheckAcceptance, LevelNG,
			"「受け入れ条件」配下にチェックリスト(- [ ] 形式)がありません")
	default:
		report.add(CheckAcceptance, LevelOK, "受け入れ条件チェックリストあり(%d 項目)", items)
	}
}

// --- labels: agent-ok あり / needs-human なし / agent-wip は情報表示 ---

func checkLabels(report *Report, labels []string) {
	if hasLabel(labels, labelAgentOK) {
		report.add(CheckLabels, LevelOK, "%s ラベルあり(Ready 化済み)", labelAgentOK)
	} else {
		report.add(CheckLabels, LevelNG,
			"%s ラベルがありません(仕様すり合わせによる Ready 化が未完了)", labelAgentOK)
	}
	if hasLabel(labels, labelNeedsHuman) {
		report.add(CheckLabels, LevelNG,
			"%s ラベルが付与されています(人間の判断待ち。エージェントは進めない)", labelNeedsHuman)
	} else {
		report.add(CheckLabels, LevelOK, "%s ラベルなし", labelNeedsHuman)
	}
	if hasLabel(labels, labelAgentWIP) {
		report.add(CheckLabels, LevelInfo, "%s ラベルあり(他セッションが作業中の可能性)", labelAgentWIP)
	} else {
		report.add(CheckLabels, LevelInfo, "%s ラベルなし", labelAgentWIP)
	}
}

func hasLabel(labels []string, name string) bool {
	for _, label := range labels {
		if label == name {
			return true
		}
	}
	return false
}

// --- deps: 「依存: #N」行の解析と依存 issue のクローズ状態 ---

var (
	depsLine = regexp.MustCompile(`(?m)^依存[::]\s*(.*)$`)
	depRef   = regexp.MustCompile(`^#(\d+)`)
)

// parseDeps は「依存: #N」行から issue 番号を取り出す。
// 「依存: #15(説明 = PR #23)」のような括弧内の補足は依存に含めない
// (行頭から連続する #N の並びだけを読む)。
func parseDeps(body string) []int {
	seen := map[int]bool{}
	var numbers []int
	for _, match := range depsLine.FindAllStringSubmatch(body, -1) {
		rest := match[1]
		for {
			rest = strings.TrimLeft(rest, " \t,、・")
			sub := depRef.FindStringSubmatch(rest)
			if sub == nil {
				break
			}
			number, err := strconv.Atoi(sub[1])
			if err == nil && !seen[number] {
				seen[number] = true
				numbers = append(numbers, number)
			}
			rest = rest[len(sub[0]):]
		}
	}
	return numbers
}

func checkDeps(report *Report, client GraphQL, owner, name, body string) {
	deps := parseDeps(body)
	if len(deps) == 0 {
		report.add(CheckDeps, LevelOK, "依存の明示なし(「依存: #N」行が本文にありません)")
		return
	}
	for _, number := range deps {
		data, err := fetchIssue(client, owner, name, number)
		if err != nil {
			// fail-closed: 依存の状態が確認できなければ NG にする。
			report.add(CheckDeps, LevelNG, "依存 #%d の状態を確認できません: %v", number, err)
			continue
		}
		if data.State == "CLOSED" {
			report.add(CheckDeps, LevelOK, "依存 #%d はクローズ済み", number)
		} else {
			report.add(CheckDeps, LevelNG,
				"依存 #%d が未解消です(state: %s)。先に依存 issue を完了させてください", number, data.State)
		}
	}
}

// --- freshness: merge:agent 付与後に本文が編集されていないか ---

func checkFreshness(report *Report, data *issueData) {
	if !hasLabel(data.Labels, labelMergeAgent) {
		report.add(CheckFreshness, LevelInfo,
			"%s ラベルなし(既定の人間マージレーン)。鮮度検査は対象外", labelMergeAgent)
		return
	}
	var labeledAt time.Time
	for _, event := range data.LabelEvents {
		if event.Label != labelMergeAgent {
			continue
		}
		t, err := time.Parse(time.RFC3339, event.CreatedAt)
		if err == nil && t.After(labeledAt) {
			labeledAt = t
		}
	}
	if labeledAt.IsZero() {
		report.add(CheckFreshness, LevelNG,
			"%s の付与時刻を timeline から特定できません(fail-closed で NG とします)", labelMergeAgent)
		return
	}
	if data.LastEditedAt == "" {
		report.add(CheckFreshness, LevelOK, "%s 付与後の本文編集なし(本文は未編集)", labelMergeAgent)
		return
	}
	editedAt, err := time.Parse(time.RFC3339, data.LastEditedAt)
	if err != nil {
		report.add(CheckFreshness, LevelNG, "本文の最終編集時刻を解釈できません: %s", data.LastEditedAt)
		return
	}
	if editedAt.After(labeledAt) {
		report.add(CheckFreshness, LevelNG,
			"%s 付与(%s)後に issue 本文が編集されています(最終編集: %s)。付与時の承認は無効です(merge-policy: 人間レーンへ降格)",
			labelMergeAgent, labeledAt.Format(time.RFC3339), editedAt.Format(time.RFC3339))
		return
	}
	report.add(CheckFreshness, LevelOK,
		"%s は新鮮です(付与: %s / 本文の最終編集: %s)",
		labelMergeAgent, labeledAt.Format(time.RFC3339), editedAt.Format(time.RFC3339))
}

// --- pr verify ---

// issueRefPattern は GitHub のクローズキーワード(Closes/Fixes/Resolves 系)と
// 参照キーワード(Refs)を拾う。
var issueRefPattern = regexp.MustCompile(`(?i)\b(?:close[sd]?|fix(?:e[sd])?|resolve[sd]?|refs?)\s+#(\d+)`)

func parseIssueRefs(body string) []int {
	seen := map[int]bool{}
	var numbers []int
	for _, match := range issueRefPattern.FindAllStringSubmatch(body, -1) {
		number, err := strconv.Atoi(match[1])
		if err == nil && !seen[number] {
			seen[number] = true
			numbers = append(numbers, number)
		}
	}
	return numbers
}

// depDeclaration は issue 本文に依存の追加が明記されているかの判定
// (spec-alignment のアジェンダ 6「依存の追加」に対応する表記)。
var depDeclaration = regexp.MustCompile(`依存の追加|依存追加|新規依存`)

// PR は PR 本文の Closes/Refs #N から関連 issue を解決し、各 issue に
// 指定された検査を実行する。depManifests が非空なら、changed files に
// マッチするファイルがある場合に関連 issue への依存明記を要求する。
func PR(client GraphQL, repo string, number int, checks []string, depManifests []string) (PRReport, error) {
	owner, name, err := splitRepo(repo)
	if err != nil {
		return PRReport{}, err
	}
	var resp prResponse
	vars := map[string]interface{}{"owner": owner, "name": name, "number": number}
	if err := client.Do(prQuery, vars, &resp); err != nil {
		return PRReport{}, fmt.Errorf("PR #%d の取得に失敗しました: %w", number, err)
	}
	if resp.Repository == nil || resp.Repository.PullRequest == nil {
		return PRReport{}, fmt.Errorf("%s/%s の PR #%d が見つかりません", owner, name, number)
	}
	pr := resp.Repository.PullRequest

	report := PRReport{Repo: repo, Number: number}
	refs := parseIssueRefs(pr.Body)
	if len(refs) == 0 {
		report.add(CheckIssueRefs, LevelNG,
			"関連 issue が見つかりません(PR 本文に Closes #N / Refs #N を記載してください)")
	} else {
		report.add(CheckIssueRefs, LevelOK, "関連 issue: %s", joinRefs(refs))
	}

	bodies := map[int]string{}
	for _, ref := range refs {
		data, err := fetchIssue(client, owner, name, ref)
		if err != nil {
			report.add(CheckIssueRefs, LevelNG, "関連 issue #%d を取得できません: %v", ref, err)
			continue
		}
		bodies[ref] = data.Body
		report.Issues = append(report.Issues, issueChecks(client, owner, name, repo, ref, data, checks))
	}

	if len(depManifests) == 0 {
		report.add(CheckDepManifests, LevelInfo,
			"依存マニフェスト検査はスキップ(--dep-manifests 未指定)")
	} else {
		var files []string
		for _, node := range pr.Files.Nodes {
			files = append(files, node.Path)
		}
		checkDepManifests(&report, depManifests, files, bodies)
	}
	return report, nil
}

func checkDepManifests(report *PRReport, patterns, files []string, issueBodies map[int]string) {
	var matched []string
	for _, file := range files {
		for _, pattern := range patterns {
			if matchManifest(pattern, file) {
				matched = append(matched, file)
				break
			}
		}
	}
	if len(matched) == 0 {
		report.add(CheckDepManifests, LevelOK, "依存マニフェストの変更なし(パターン: %s)",
			strings.Join(patterns, ", "))
		return
	}
	for _, body := range issueBodies {
		if depDeclaration.MatchString(body) {
			report.add(CheckDepManifests, LevelOK,
				"依存マニフェストの変更(%s)は関連 issue に明記済み", strings.Join(matched, ", "))
			return
		}
	}
	report.add(CheckDepManifests, LevelNG,
		"依存マニフェスト(%s)が変更されていますが、関連 issue に依存の追加が明記されていません(merge-policy: 明記なき依存追加は agent マージ失格。issue 本文に「依存の追加」を記載してください)",
		strings.Join(matched, ", "))
}

// matchManifest は glob パターンとファイルパスを突き合わせる。
// パス全体・ベース名の両方で判定し、「**/xxx」はベース名一致として扱う。
func matchManifest(pattern, file string) bool {
	if ok, _ := path.Match(pattern, file); ok {
		return true
	}
	base := path.Base(file)
	if ok, _ := path.Match(pattern, base); ok {
		return true
	}
	if rest, found := strings.CutPrefix(pattern, "**/"); found {
		if ok, _ := path.Match(rest, base); ok {
			return true
		}
	}
	return false
}

func joinRefs(numbers []int) string {
	parts := make([]string, 0, len(numbers))
	for _, number := range numbers {
		parts = append(parts, fmt.Sprintf("#%d", number))
	}
	return strings.Join(parts, ", ")
}
