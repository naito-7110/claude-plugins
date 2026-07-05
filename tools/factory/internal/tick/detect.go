package tick

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// 作業検知プリチェック(#111)。
//
// 課金方針によりループはローカル claude(サブスク)で回るため、反応速度を
// 上げる手段は tick の短周期化しかない。しかし検知は gh API だけでできる —
// claude が要るのは「対処」だけ。そこで claude 起動前に Go で作業の有無を
// 検知し、空振りのセッション(1 分 tick なら日に 1000 回)をゼロにする。
//
// 検知 4 条件(いずれかがあれば起動):
//  1. Ready の open issue(agent-ok あり・needs-human なし・agent-wip なし)
//  2. open の agent PR に未解決レビュースレッドがあり、最新コメントが自分
//     (viewer)でない(#107 の再配車判定と同一の意味論 — 自分の返信が最新の
//     スレッドは人間の再応答待ちが正常)
//  3. factory-review = FAILURE の open agent PR
//  4. 前回起動以降にマージされた agent PR(未回収の可能性。台帳ではなく
//     GitHub の mergedAt と StateFile の比較で判定)
//
// API 予算: 1 tick あたり 3 クエリ(issue 一覧 / open PR + スレッド + status +
// viewer / merged PR 一覧)。1 分周期でも 180 req/h で GraphQL レート制限
// (5000 point/h)に対して十分小さい。

// GraphQL は GitHub GraphQL API へのプロセス境界(他パッケージと同形)。
type GraphQL interface {
	Do(query string, variables map[string]interface{}, response interface{}) error
}

// StateFile は前回 claude を起動した時刻の記録(ローカル・非コミット)。
// **起動したときだけ**更新する — 検知だけの空振りで進めると、検知と起動の
// 間の取りこぼしが発生するため(取りこぼし防止が優先)。
const StateFile = ".agents/tick-state"

// defaultLookback は StateFile が無い初回の遡り幅(条件 4 の基準時刻)。
const defaultLookback = 24 * time.Hour

// agentBranchPattern は agent PR の判定(head ブランチ名)。
var agentBranchPattern = regexp.MustCompile(`^agent/issue-`)

// --- GraphQL クエリ(合計 3 回 / tick)---

const readyIssuesQuery = `query($owner: String!, $name: String!) {
  repository(owner: $owner, name: $name) {
    issues(states: OPEN, labels: ["agent-ok"], first: 50) {
      nodes { number labels(first: 20) { nodes { name } } }
    }
  }
}`

const openPRWorkQuery = `query($owner: String!, $name: String!) {
  viewer { login }
  repository(owner: $owner, name: $name) {
    pullRequests(states: OPEN, first: 50) {
      nodes {
        number
        headRefName
        reviewThreads(first: 50) {
          nodes { isResolved comments(last: 1) { nodes { author { login } } } }
        }
        commits(last: 1) {
          nodes { commit { status { context(name: "factory-review") { state } } } }
        }
      }
    }
  }
}`

const mergedPRsQuery = `query($owner: String!, $name: String!) {
  repository(owner: $owner, name: $name) {
    pullRequests(states: MERGED, first: 20, orderBy: {field: UPDATED_AT, direction: DESC}) {
      nodes { number headRefName mergedAt }
    }
  }
}`

// detectWork は 4 条件を順に検査し、最初に見つかった作業の理由を返す
// (無ければ空文字列)。since は条件 4 の基準時刻。
func detectWork(client GraphQL, owner, name string, since time.Time) (string, error) {
	if reason, err := detectReadyIssue(client, owner, name); reason != "" || err != nil {
		return reason, err
	}
	if reason, err := detectPRWork(client, owner, name); reason != "" || err != nil {
		return reason, err
	}
	return detectMergedPR(client, owner, name, since)
}

func detectReadyIssue(client GraphQL, owner, name string) (string, error) {
	var resp struct {
		Repository *struct {
			Issues struct {
				Nodes []struct {
					Number int `json:"number"`
					Labels struct {
						Nodes []struct {
							Name string `json:"name"`
						} `json:"nodes"`
					} `json:"labels"`
				} `json:"nodes"`
			} `json:"issues"`
		} `json:"repository"`
	}
	vars := map[string]interface{}{"owner": owner, "name": name}
	if err := client.Do(readyIssuesQuery, vars, &resp); err != nil {
		return "", fmt.Errorf("issue 一覧を取得できません: %w", err)
	}
	if resp.Repository == nil {
		return "", nil
	}
	for _, issue := range resp.Repository.Issues.Nodes {
		hasOK, blocked := false, false
		for _, label := range issue.Labels.Nodes {
			switch label.Name {
			case "agent-ok":
				hasOK = true
			case "needs-human", "agent-wip":
				blocked = true
			}
		}
		if hasOK && !blocked {
			return fmt.Sprintf("Ready の issue #%d があります", issue.Number), nil
		}
	}
	return "", nil
}

func detectPRWork(client GraphQL, owner, name string) (string, error) {
	var resp struct {
		Viewer struct {
			Login string `json:"login"`
		} `json:"viewer"`
		Repository *struct {
			PullRequests struct {
				Nodes []struct {
					Number        int    `json:"number"`
					HeadRefName   string `json:"headRefName"`
					ReviewThreads struct {
						Nodes []struct {
							IsResolved bool `json:"isResolved"`
							Comments   struct {
								Nodes []struct {
									Author *struct {
										Login string `json:"login"`
									} `json:"author"`
								} `json:"nodes"`
							} `json:"comments"`
						} `json:"nodes"`
					} `json:"reviewThreads"`
					Commits struct {
						Nodes []struct {
							Commit struct {
								Status *struct {
									Context *struct {
										State string `json:"state"`
									} `json:"context"`
								} `json:"status"`
							} `json:"commit"`
						} `json:"nodes"`
					} `json:"commits"`
				} `json:"nodes"`
			} `json:"pullRequests"`
		} `json:"repository"`
	}
	vars := map[string]interface{}{"owner": owner, "name": name}
	if err := client.Do(openPRWorkQuery, vars, &resp); err != nil {
		return "", fmt.Errorf("open PR の状態を取得できません: %w", err)
	}
	if resp.Repository == nil {
		return "", nil
	}
	viewer := resp.Viewer.Login
	for _, pr := range resp.Repository.PullRequests.Nodes {
		if !agentBranchPattern.MatchString(pr.HeadRefName) {
			continue
		}
		// 条件 2: 未解決スレッドで最新コメントが自分でない(#107 と同一判定)。
		for _, thread := range pr.ReviewThreads.Nodes {
			if thread.IsResolved {
				continue
			}
			last := ""
			if n := len(thread.Comments.Nodes); n > 0 && thread.Comments.Nodes[n-1].Author != nil {
				last = thread.Comments.Nodes[n-1].Author.Login
			}
			if last != viewer {
				return fmt.Sprintf("PR #%d に未対応のレビュースレッドがあります(最新コメント: %s)", pr.Number, last), nil
			}
		}
		// 条件 3: factory-review = FAILURE。
		for _, commit := range pr.Commits.Nodes {
			status := commit.Commit.Status
			if status != nil && status.Context != nil && status.Context.State == "FAILURE" {
				return fmt.Sprintf("PR #%d が factory-review = failure です", pr.Number), nil
			}
		}
	}
	return "", nil
}

func detectMergedPR(client GraphQL, owner, name string, since time.Time) (string, error) {
	var resp struct {
		Repository *struct {
			PullRequests struct {
				Nodes []struct {
					Number      int    `json:"number"`
					HeadRefName string `json:"headRefName"`
					MergedAt    string `json:"mergedAt"`
				} `json:"nodes"`
			} `json:"pullRequests"`
		} `json:"repository"`
	}
	vars := map[string]interface{}{"owner": owner, "name": name}
	if err := client.Do(mergedPRsQuery, vars, &resp); err != nil {
		return "", fmt.Errorf("merged PR を取得できません: %w", err)
	}
	if resp.Repository == nil {
		return "", nil
	}
	for _, pr := range resp.Repository.PullRequests.Nodes {
		if !agentBranchPattern.MatchString(pr.HeadRefName) {
			continue
		}
		mergedAt, err := time.Parse(time.RFC3339, pr.MergedAt)
		if err != nil {
			continue
		}
		if mergedAt.After(since) {
			return fmt.Sprintf("PR #%d が前回 tick 以降にマージされています(未回収の可能性)", pr.Number), nil
		}
	}
	return "", nil
}

// --- tick-state(前回起動時刻)---

// readTickState は前回起動時刻を読む。無い(初回)場合は
// now - defaultLookback を基準にする(回収漏れを 1 日分だけ遡って拾う)。
func readTickState(root string, now time.Time) time.Time {
	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(StateFile)))
	if err == nil {
		if t, err := time.Parse(time.RFC3339, strings.TrimSpace(string(data))); err == nil {
			return t
		}
	}
	return now.Add(-defaultLookback)
}

// writeTickState は起動時刻を記録する(claude を起動するときだけ呼ぶ)。
func writeTickState(root string, now time.Time) error {
	p := filepath.Join(root, filepath.FromSlash(StateFile))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	return os.WriteFile(p, []byte(now.UTC().Format(time.RFC3339)+"\n"), 0o644)
}
