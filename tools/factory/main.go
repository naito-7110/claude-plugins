// factory — factory プラグインの CLI。
//
// GitHub Projects の正準ボード「factory board template」の複製・検証と、
// issue / PR の整合検証(spec-alignment / merge-policy の機械検証可能な部分)、
// 文書構造の検証(documentation プリセット: 地図・所有マップ・ドメイン文書)、
// unattended 運転の制御(mode = 運転状態 / tick = crontab の起動機構)を行う。
// 認証は gh CLI のセッションを継承する(go-gh)。docs verify / mode / tick は
// ローカルで完結する。
package main

import (
	"fmt"
	"os"

	"github.com/cli/go-gh/v2/pkg/api"
	"github.com/cli/go-gh/v2/pkg/repository"

	"github.com/naito-7110/claude-plugins/tools/factory/internal/board"
	"github.com/naito-7110/claude-plugins/tools/factory/internal/cli"
	"github.com/naito-7110/claude-plugins/tools/factory/internal/tick"
)

func main() {
	deps := cli.Deps{
		NewClient: func() (board.GraphQL, error) {
			client, err := api.DefaultGraphQLClient()
			if err != nil {
				return nil, fmt.Errorf("gh の認証情報を取得できません(gh auth login を実行してください): %w", err)
			}
			return client, nil
		},
		CurrentRepo: func() (string, error) {
			repo, err := repository.Current()
			if err != nil {
				return "", err
			}
			return repo.Owner + "/" + repo.Name, nil
		},
		Crontab: tick.System{},
		Out:     os.Stdout,
		Err:     os.Stderr,
	}
	os.Exit(cli.Run(os.Args[1:], deps))
}
