package ghfake

import (
	"fmt"
	"strings"
)

// LabelEvent は timeline 上のラベル付与イベント(LabeledEvent)。
type LabelEvent struct {
	Label     string
	CreatedAt string // RFC3339
}

// Issue は fake 上の issue の状態。
type Issue struct {
	Number       int
	State        string // "OPEN" | "CLOSED"
	Body         string
	LastEditedAt string // RFC3339。空なら本文編集なし(null)
	Labels       []string
	LabelEvents  []LabelEvent
}

// PullRequest は fake 上の PR の状態。
type PullRequest struct {
	Number      int
	Body        string
	Files       []string // changed files のパス
	HeadRefName string   // head ブランチ名(カレントブランチの PR 解決に使う)
	State       string   // "OPEN" | "MERGED" | "CLOSED"(空 = OPEN)
	// マージゲート(atelier gate)用の状態。
	ClosingIssues []int  // Closes で紐づく issue 番号
	ChecksState   string // statusCheckRollup.state("" = check なし)
	ReviewState   string // atelier-review status context の state("" = status なし)
	Author        string // PR 作者の login("" = 作者不明。実 API では null になりうる)
	ReviewCreator string // atelier-review status の投稿者 login("" = 投稿者不明)
}

// AddIssue は issue を "owner/name" のリポジトリへ登録する。
func (s *Server) AddIssue(repo string, issue *Issue) *Issue {
	s.Issues[repo] = append(s.Issues[repo], issue)
	return issue
}

// FindIssue はリポジトリと番号で issue を探す。
func (s *Server) FindIssue(repo string, number int) *Issue {
	for _, issue := range s.Issues[repo] {
		if issue.Number == number {
			return issue
		}
	}
	return nil
}

// AddPullRequest は PR を "owner/name" のリポジトリへ登録する。
func (s *Server) AddPullRequest(repo string, pr *PullRequest) *PullRequest {
	s.PullRequests[repo] = append(s.PullRequests[repo], pr)
	return pr
}

func (s *Server) findPullRequest(repo string, number int) *PullRequest {
	for _, pr := range s.PullRequests[repo] {
		if pr.Number == number {
			return pr
		}
	}
	return nil
}

func (s *Server) doIssueQuery(vars map[string]interface{}, response interface{}) error {
	repo := fmt.Sprintf("%v/%v", vars["owner"], vars["name"])
	issue := s.FindIssue(repo, asInt(vars["number"]))
	if issue == nil {
		return reply(response, map[string]interface{}{
			"repository": map[string]interface{}{"issue": nil},
		})
	}
	labels := []map[string]string{}
	for _, name := range issue.Labels {
		labels = append(labels, map[string]string{"name": name})
	}
	events := []map[string]interface{}{}
	for _, event := range issue.LabelEvents {
		events = append(events, map[string]interface{}{
			"createdAt": event.CreatedAt,
			"label":     map[string]string{"name": event.Label},
		})
	}
	var lastEdited interface{}
	if issue.LastEditedAt != "" {
		lastEdited = issue.LastEditedAt
	}
	return reply(response, map[string]interface{}{
		"repository": map[string]interface{}{
			"issue": map[string]interface{}{
				"state":         issue.State,
				"body":          issue.Body,
				"lastEditedAt":  lastEdited,
				"labels":        map[string]interface{}{"nodes": labels},
				"timelineItems": map[string]interface{}{"nodes": events},
			},
		},
	})
}

// doMergeStatusQuery はマージゲートの状態クエリ(Closes 紐づけ・CI rollup・
// atelier-review status)に応答する。
func (s *Server) doMergeStatusQuery(vars map[string]interface{}, response interface{}) error {
	repo := fmt.Sprintf("%v/%v", vars["owner"], vars["name"])
	pr := s.findPullRequest(repo, asInt(vars["number"]))
	if pr == nil {
		return reply(response, map[string]interface{}{
			"repository": map[string]interface{}{"pullRequest": nil},
		})
	}
	closing := []map[string]int{}
	for _, number := range pr.ClosingIssues {
		closing = append(closing, map[string]int{"number": number})
	}
	var rollup interface{}
	if pr.ChecksState != "" {
		rollup = map[string]string{"state": pr.ChecksState}
	}
	var status interface{}
	if pr.ReviewState != "" {
		var creator interface{}
		if pr.ReviewCreator != "" {
			creator = map[string]string{"login": pr.ReviewCreator}
		}
		status = map[string]interface{}{"context": map[string]interface{}{
			"state":   pr.ReviewState,
			"creator": creator,
		}}
	} else {
		status = map[string]interface{}{"context": nil}
	}
	var author interface{}
	if pr.Author != "" {
		author = map[string]string{"login": pr.Author}
	}
	return reply(response, map[string]interface{}{
		"repository": map[string]interface{}{
			"pullRequest": map[string]interface{}{
				"author":                  author,
				"closingIssuesReferences": map[string]interface{}{"nodes": closing},
				"commits": map[string]interface{}{
					"nodes": []map[string]interface{}{
						{"commit": map[string]interface{}{
							"statusCheckRollup": rollup,
							"status":            status,
						}},
					},
				},
			},
		},
	})
}

// doPRByBranchQuery は headRefName による PR の解決に応答する。
// クエリに states: OPEN フィルタがあれば OPEN のみ返す(gate の解決)。
// フィルタが無ければ全状態を返す(branch cleanup のマージ済み判定)。
func (s *Server) doPRByBranchQuery(query string, vars map[string]interface{}, response interface{}) error {
	repo := fmt.Sprintf("%v/%v", vars["owner"], vars["name"])
	branch := fmt.Sprintf("%v", vars["branch"])
	nodes := []map[string]interface{}{}
	for _, pr := range s.PullRequests[repo] {
		if pr.HeadRefName != branch {
			continue
		}
		state := pr.State
		if state == "" {
			state = "OPEN"
		}
		if strings.Contains(query, "states: OPEN") && state != "OPEN" {
			continue
		}
		nodes = append(nodes, map[string]interface{}{"number": pr.Number, "state": state})
	}
	return reply(response, map[string]interface{}{
		"repository": map[string]interface{}{
			"pullRequests": map[string]interface{}{"nodes": nodes},
		},
	})
}

func (s *Server) doPullRequestQuery(vars map[string]interface{}, response interface{}) error {
	repo := fmt.Sprintf("%v/%v", vars["owner"], vars["name"])
	pr := s.findPullRequest(repo, asInt(vars["number"]))
	if pr == nil {
		return reply(response, map[string]interface{}{
			"repository": map[string]interface{}{"pullRequest": nil},
		})
	}
	files := []map[string]string{}
	for _, path := range pr.Files {
		files = append(files, map[string]string{"path": path})
	}
	return reply(response, map[string]interface{}{
		"repository": map[string]interface{}{
			"pullRequest": map[string]interface{}{
				"body":  pr.Body,
				"files": map[string]interface{}{"nodes": files},
			},
		},
	})
}
