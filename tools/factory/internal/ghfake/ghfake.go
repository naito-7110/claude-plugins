// Package ghfake は GitHub GraphQL API(プロセス境界)のインメモリ fake。
// board.GraphQL interface を満たし、テストから注入する。
// 実 API の観測済みの挙動を再現する: copy は Status・ビュー・フィールドを
// 複製するが、説明欄・リンクは複製しない。
package ghfake

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Project は fake 上のボードの状態。
type Project struct {
	ID            string
	Owner         string
	Number        int
	Title         string
	Description   string
	StatusOptions []string
	ViewLayouts   []string
	Fields        map[string]string // name -> dataType
	LinkedRepos   []string          // repository ID
}

// Server はインメモリの GitHub。
type Server struct {
	OwnerTypes   map[string]string // login -> "User" | "Organization"
	OwnerIDs     map[string]string // login -> node ID
	Repos        map[string]string // "owner/name" -> node ID
	Projects     []*Project
	Issues       map[string][]*Issue       // "owner/name" -> issues
	PullRequests map[string][]*PullRequest // "owner/name" -> PRs
}

// NewServer は空の fake を返す。
func NewServer() *Server {
	return &Server{
		OwnerTypes:   map[string]string{},
		OwnerIDs:     map[string]string{},
		Repos:        map[string]string{},
		Issues:       map[string][]*Issue{},
		PullRequests: map[string][]*PullRequest{},
	}
}

// AddOwner は user / organization を登録する。
func (s *Server) AddOwner(login, kind string) {
	s.OwnerTypes[login] = kind
	s.OwnerIDs[login] = "OWNER_" + login
}

// AddProject はボードを登録する。
func (s *Server) AddProject(p *Project) *Project {
	if p.ID == "" {
		p.ID = fmt.Sprintf("PVT_%s_%d", p.Owner, p.Number)
	}
	s.Projects = append(s.Projects, p)
	return p
}

// FindProject は owner と番号でボードを探す。
func (s *Server) FindProject(owner string, number int) *Project {
	for _, p := range s.Projects {
		if p.Owner == owner && p.Number == number {
			return p
		}
	}
	return nil
}

func (s *Server) findProjectByID(id string) *Project {
	for _, p := range s.Projects {
		if p.ID == id {
			return p
		}
	}
	return nil
}

// Do は board.GraphQL を満たす。クエリの内容からオペレーションを判別する。
func (s *Server) Do(query string, vars map[string]interface{}, response interface{}) error {
	switch {
	case strings.Contains(query, "copyProjectV2"):
		return s.doCopy(vars, response)
	case strings.Contains(query, "updateProjectV2("):
		return s.doUpdateDescription(vars, response)
	case strings.Contains(query, "linkProjectV2ToRepository"):
		return s.doLink(vars, response)
	case strings.Contains(query, "repositoryOwner"):
		return s.doOwnerID(vars, response)
	case strings.Contains(query, "closingIssuesReferences"):
		return s.doMergeStatusQuery(vars, response)
	case strings.Contains(query, "pullRequests("):
		return s.doPRByBranchQuery(vars, response)
	case strings.Contains(query, "pullRequest(number:"):
		return s.doPullRequestQuery(vars, response)
	case strings.Contains(query, "issue(number:"):
		return s.doIssueQuery(vars, response)
	case strings.Contains(query, "repository(owner:"):
		return s.doRepoID(vars, response)
	case strings.Contains(query, "projectV2(number:"):
		return s.doProjectQuery(query, vars, response)
	default:
		return fmt.Errorf("ghfake: 未対応のクエリ: %s", query)
	}
}

func reply(response interface{}, data map[string]interface{}) error {
	raw, err := json.Marshal(data)
	if err != nil {
		return err
	}
	return json.Unmarshal(raw, response)
}

func (s *Server) doProjectQuery(query string, vars map[string]interface{}, response interface{}) error {
	login, _ := vars["login"].(string)
	number := asInt(vars["number"])

	root := "user"
	wantType := "User"
	if strings.Contains(query, "organization(login:") {
		root = "organization"
		wantType = "Organization"
	}
	if s.OwnerTypes[login] != wantType {
		return fmt.Errorf("GraphQL: Could not resolve to a %s with the login of '%s'", wantType, login)
	}
	project := s.FindProject(login, number)
	if project == nil {
		return reply(response, map[string]interface{}{root: map[string]interface{}{"projectV2": nil}})
	}

	if !strings.Contains(query, "ProjectV2SingleSelectField") {
		return reply(response, map[string]interface{}{
			root: map[string]interface{}{"projectV2": map[string]interface{}{"id": project.ID}},
		})
	}

	options := []map[string]string{}
	for _, name := range project.StatusOptions {
		options = append(options, map[string]string{"name": name})
	}
	views := []map[string]string{}
	for _, layout := range project.ViewLayouts {
		views = append(views, map[string]string{"layout": layout})
	}
	var targetDate interface{}
	if dataType, ok := project.Fields["Target date"]; ok {
		targetDate = map[string]string{"name": "Target date", "dataType": dataType}
	}
	return reply(response, map[string]interface{}{
		root: map[string]interface{}{
			"projectV2": map[string]interface{}{
				"field":      map[string]interface{}{"options": options},
				"views":      map[string]interface{}{"nodes": views},
				"targetDate": targetDate,
			},
		},
	})
}

func (s *Server) doOwnerID(vars map[string]interface{}, response interface{}) error {
	login, _ := vars["login"].(string)
	id, ok := s.OwnerIDs[login]
	if !ok {
		return reply(response, map[string]interface{}{"repositoryOwner": nil})
	}
	return reply(response, map[string]interface{}{
		"repositoryOwner": map[string]interface{}{"id": id},
	})
}

func (s *Server) doRepoID(vars map[string]interface{}, response interface{}) error {
	key := fmt.Sprintf("%s/%s", vars["owner"], vars["name"])
	id, ok := s.Repos[key]
	if !ok {
		return reply(response, map[string]interface{}{"repository": nil})
	}
	return reply(response, map[string]interface{}{
		"repository": map[string]interface{}{"id": id},
	})
}

func (s *Server) doCopy(vars map[string]interface{}, response interface{}) error {
	source := s.findProjectByID(fmt.Sprintf("%v", vars["projectId"]))
	if source == nil {
		return fmt.Errorf("GraphQL: source project not found")
	}
	ownerLogin := ""
	for login, id := range s.OwnerIDs {
		if id == fmt.Sprintf("%v", vars["ownerId"]) {
			ownerLogin = login
		}
	}
	if ownerLogin == "" {
		return fmt.Errorf("GraphQL: owner not found")
	}
	number := 1
	for _, p := range s.Projects {
		if p.Owner == ownerLogin && p.Number >= number {
			number = p.Number + 1
		}
	}
	// 実 API の挙動: Status・ビュー・フィールドは複製、説明欄・リンクは複製しない。
	copied := s.AddProject(&Project{
		Owner:         ownerLogin,
		Number:        number,
		Title:         fmt.Sprintf("%v", vars["title"]),
		StatusOptions: append([]string{}, source.StatusOptions...),
		ViewLayouts:   append([]string{}, source.ViewLayouts...),
		Fields:        copyMap(source.Fields),
	})
	segment := "users"
	if s.OwnerTypes[ownerLogin] == "Organization" {
		segment = "orgs"
	}
	url := fmt.Sprintf("https://github.com/%s/%s/projects/%d", segment, ownerLogin, number)
	return reply(response, map[string]interface{}{
		"copyProjectV2": map[string]interface{}{
			"projectV2": map[string]interface{}{
				"id": copied.ID, "number": copied.Number, "url": url,
			},
		},
	})
}

func (s *Server) doUpdateDescription(vars map[string]interface{}, response interface{}) error {
	project := s.findProjectByID(fmt.Sprintf("%v", vars["projectId"]))
	if project == nil {
		return fmt.Errorf("GraphQL: project not found")
	}
	project.Description = fmt.Sprintf("%v", vars["description"])
	return reply(response, map[string]interface{}{
		"updateProjectV2": map[string]interface{}{"projectV2": map[string]interface{}{"id": project.ID}},
	})
}

func (s *Server) doLink(vars map[string]interface{}, response interface{}) error {
	project := s.findProjectByID(fmt.Sprintf("%v", vars["projectId"]))
	if project == nil {
		return fmt.Errorf("GraphQL: project not found")
	}
	repoID := fmt.Sprintf("%v", vars["repositoryId"])
	project.LinkedRepos = append(project.LinkedRepos, repoID)
	return reply(response, map[string]interface{}{
		"linkProjectV2ToRepository": map[string]interface{}{"repository": map[string]interface{}{"id": repoID}},
	})
}

func copyMap(m map[string]string) map[string]string {
	out := map[string]string{}
	for k, v := range m {
		out[k] = v
	}
	return out
}

func asInt(v interface{}) int {
	switch n := v.(type) {
	case int:
		return n
	case float64:
		return int(n)
	case json.Number:
		i, _ := n.Int64()
		return int(i)
	default:
		return 0
	}
}
