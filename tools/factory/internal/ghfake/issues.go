package ghfake

import "fmt"

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
	Number int
	Body   string
	Files  []string // changed files のパス
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
