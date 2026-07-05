// Package board は atelier の GitHub Projects ボード操作(複製・検証)を提供する。
//
// GitHub GraphQL API はプロセス境界であり、GraphQL interface で抽象化する。
// テストではこの境界に fake を注入する(デトロイト派 TDD)。
package board

import (
	"fmt"
	"slices"
	"strings"
)

// GraphQL は GitHub GraphQL API へのプロセス境界。
// go-gh の api.GraphQLClient がこの interface を満たす。
type GraphQL interface {
	Do(query string, variables map[string]interface{}, response interface{}) error
}

// 正準ボードの既定値と検証仕様。
var (
	// StatusOptions は atelier の Status 6 値(この順)。
	StatusOptions = []string{"Inbox", "Spec", "Ready", "In Progress", "In Review", "Done"}
	// RequiredLayouts は正準ボードが備えるビューのレイアウト。
	RequiredLayouts = []string{"TABLE_LAYOUT", "BOARD_LAYOUT", "ROADMAP_LAYOUT"}
)

const (
	// TargetDateField は roadmap ビューの日付軸に使うフィールド名。
	TargetDateField = "Target date"
	// DefaultSourceOwner / DefaultSourceNumber は正準ボードの所在。
	DefaultSourceOwner  = "naito-7110"
	DefaultSourceNumber = 4
)

// Report は verify の結果。Status の一致がハードゲートで、
// ビュー・フィールドの不足は警告(Warnings)にとどめる。
type Report struct {
	Owner        string
	Number       int
	StatusOK     bool
	ActualStatus []string
	Warnings     []string
}

// --- GraphQL クエリ(%s には user / organization のルートが入る) ---

const projectDetailQuery = `query($login: String!, $number: Int!) {
  %s(login: $login) {
    projectV2(number: $number) {
      field(name: "Status") {
        ... on ProjectV2SingleSelectField { options { name } }
      }
      views(first: 20) { nodes { layout } }
      targetDate: field(name: "Target date") {
        ... on ProjectV2FieldCommon { name dataType }
      }
    }
  }
}`

const projectIDQuery = `query($login: String!, $number: Int!) {
  %s(login: $login) {
    projectV2(number: $number) { id }
  }
}`

const ownerIDQuery = `query($login: String!) {
  repositoryOwner(login: $login) { id }
}`

const repoIDQuery = `query($owner: String!, $name: String!) {
  repository(owner: $owner, name: $name) { id }
}`

const copyMutation = `mutation($projectId: ID!, $ownerId: ID!, $title: String!) {
  copyProjectV2(input: {projectId: $projectId, ownerId: $ownerId, title: $title}) {
    projectV2 { id number url }
  }
}`

const descriptionMutation = `mutation($projectId: ID!, $description: String!) {
  updateProjectV2(input: {projectId: $projectId, shortDescription: $description}) {
    projectV2 { id }
  }
}`

const linkMutation = `mutation($projectId: ID!, $repositoryId: ID!) {
  linkProjectV2ToRepository(input: {projectId: $projectId, repositoryId: $repositoryId}) {
    repository { id }
  }
}`

type projectDetail struct {
	Field struct {
		Options []struct {
			Name string `json:"name"`
		} `json:"options"`
	} `json:"field"`
	Views struct {
		Nodes []struct {
			Layout string `json:"layout"`
		} `json:"nodes"`
	} `json:"views"`
	TargetDate *struct {
		Name     string `json:"name"`
		DataType string `json:"dataType"`
	} `json:"targetDate"`
}

type detailResponse struct {
	User         *struct{ ProjectV2 *projectDetail } `json:"user"`
	Organization *struct{ ProjectV2 *projectDetail } `json:"organization"`
}

type idResponse struct {
	User         *struct{ ProjectV2 *struct{ ID string } } `json:"user"`
	Organization *struct{ ProjectV2 *struct{ ID string } } `json:"organization"`
}

// fetchProjectDetail は owner が user / organization のどちらでも解決できるよう
// 両ルートを順に試す(GitHub GraphQL は owner 種別ごとにルートが分かれるため)。
func fetchProjectDetail(client GraphQL, owner string, number int) (*projectDetail, error) {
	var lastErr error
	for _, root := range []string{"user", "organization"} {
		var resp detailResponse
		err := client.Do(fmt.Sprintf(projectDetailQuery, root),
			map[string]interface{}{"login": owner, "number": number}, &resp)
		if err != nil {
			lastErr = err
			continue
		}
		node := resp.User
		if root == "organization" {
			node = resp.Organization
		}
		if node != nil && node.ProjectV2 != nil {
			return node.ProjectV2, nil
		}
		lastErr = fmt.Errorf("%s/projects/%d が見つかりません", owner, number)
	}
	return nil, lastErr
}

func fetchProjectID(client GraphQL, owner string, number int) (string, error) {
	var lastErr error
	for _, root := range []string{"user", "organization"} {
		var resp idResponse
		err := client.Do(fmt.Sprintf(projectIDQuery, root),
			map[string]interface{}{"login": owner, "number": number}, &resp)
		if err != nil {
			lastErr = err
			continue
		}
		node := resp.User
		if root == "organization" {
			node = resp.Organization
		}
		if node != nil && node.ProjectV2 != nil {
			return node.ProjectV2.ID, nil
		}
		lastErr = fmt.Errorf("%s/projects/%d が見つかりません", owner, number)
	}
	return "", lastErr
}

// Verify はボードの Status 6 値(ハードゲート)と、ビューのレイアウト・
// Target date フィールド(警告)を検査する。
func Verify(client GraphQL, owner string, number int) (Report, error) {
	report := Report{Owner: owner, Number: number}
	detail, err := fetchProjectDetail(client, owner, number)
	if err != nil {
		return report, err
	}

	for _, opt := range detail.Field.Options {
		report.ActualStatus = append(report.ActualStatus, opt.Name)
	}
	report.StatusOK = slices.Equal(report.ActualStatus, StatusOptions)

	layouts := map[string]bool{}
	for _, view := range detail.Views.Nodes {
		layouts[view.Layout] = true
	}
	for _, want := range RequiredLayouts {
		if !layouts[want] {
			name := strings.TrimSuffix(want, "_LAYOUT")
			report.Warnings = append(report.Warnings,
				fmt.Sprintf("%s レイアウトのビューがありません(正準ボードからの複製で揃う想定)", name))
		}
	}
	if detail.TargetDate == nil || detail.TargetDate.DataType != "DATE" {
		report.Warnings = append(report.Warnings,
			fmt.Sprintf("日付フィールド「%s」(DATE)がありません(roadmap ビューの日付軸に使用)", TargetDateField))
	}
	return report, nil
}

// CopyOptions は Copy の入力。Repo は "owner/name" 形式(空ならリンクしない)。
type CopyOptions struct {
	SourceOwner  string
	SourceNumber int
	TargetOwner  string
	Title        string
	Repo         string
}

// CopyResult は Copy の結果。Report には複製直後の verify 結果が入る。
type CopyResult struct {
	ProjectID  string
	Number     int
	URL        string
	LinkedRepo string
	Report     Report
}

// Copy は正準ボードを対象 owner へ複製し、説明欄の設定(copy では複製されない)、
// リポジトリへのリンク、直後の verify までを行う。
func Copy(client GraphQL, opts CopyOptions) (CopyResult, error) {
	result := CopyResult{}

	sourceID, err := fetchProjectID(client, opts.SourceOwner, opts.SourceNumber)
	if err != nil {
		return result, fmt.Errorf("正準ボード %s/projects/%d を解決できません: %w",
			opts.SourceOwner, opts.SourceNumber, err)
	}

	var ownerResp struct {
		RepositoryOwner *struct{ ID string } `json:"repositoryOwner"`
	}
	if err := client.Do(ownerIDQuery,
		map[string]interface{}{"login": opts.TargetOwner}, &ownerResp); err != nil {
		return result, fmt.Errorf("owner %s を解決できません: %w", opts.TargetOwner, err)
	}
	if ownerResp.RepositoryOwner == nil {
		return result, fmt.Errorf("owner %s が見つかりません", opts.TargetOwner)
	}

	var copyResp struct {
		CopyProjectV2 struct {
			ProjectV2 struct {
				ID     string `json:"id"`
				Number int    `json:"number"`
				URL    string `json:"url"`
			} `json:"projectV2"`
		} `json:"copyProjectV2"`
	}
	err = client.Do(copyMutation, map[string]interface{}{
		"projectId": sourceID,
		"ownerId":   ownerResp.RepositoryOwner.ID,
		"title":     opts.Title,
	}, &copyResp)
	if err != nil {
		return result, fmt.Errorf("copyProjectV2 に失敗しました: %w", err)
	}
	result.ProjectID = copyResp.CopyProjectV2.ProjectV2.ID
	result.Number = copyResp.CopyProjectV2.ProjectV2.Number
	result.URL = copyResp.CopyProjectV2.ProjectV2.URL

	description := fmt.Sprintf("atelier ボード(正準テンプレート %s/projects/%d から複製)",
		opts.SourceOwner, opts.SourceNumber)
	var descResp struct{}
	if err := client.Do(descriptionMutation, map[string]interface{}{
		"projectId":   result.ProjectID,
		"description": description,
	}, &descResp); err != nil {
		return result, fmt.Errorf("説明欄の設定に失敗しました: %w", err)
	}

	if opts.Repo != "" {
		if err := linkRepo(client, result.ProjectID, opts.Repo); err != nil {
			return result, err
		}
		result.LinkedRepo = opts.Repo
	}

	report, err := Verify(client, opts.TargetOwner, result.Number)
	if err != nil {
		return result, fmt.Errorf("複製後の verify に失敗しました: %w", err)
	}
	result.Report = report
	return result, nil
}

func linkRepo(client GraphQL, projectID, repo string) error {
	owner, name, ok := strings.Cut(repo, "/")
	if !ok {
		return fmt.Errorf("リポジトリは owner/name 形式で指定してください: %s", repo)
	}
	var repoResp struct {
		Repository *struct{ ID string } `json:"repository"`
	}
	if err := client.Do(repoIDQuery,
		map[string]interface{}{"owner": owner, "name": name}, &repoResp); err != nil {
		return fmt.Errorf("リポジトリ %s を解決できません: %w", repo, err)
	}
	if repoResp.Repository == nil {
		return fmt.Errorf("リポジトリ %s が見つかりません", repo)
	}
	var linkResp struct{}
	if err := client.Do(linkMutation, map[string]interface{}{
		"projectId":    projectID,
		"repositoryId": repoResp.Repository.ID,
	}, &linkResp); err != nil {
		return fmt.Errorf("リポジトリ %s へのリンクに失敗しました: %w", repo, err)
	}
	return nil
}
