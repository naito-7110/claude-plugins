---
name: uninstall
description: atelier をやめるときの cleanup(対話専用)。ローカル状態(.agents/)の削除 → 残るもの(committed 設置物・GitHub 側)の一覧提示。プラグイン機構に uninstall フックは無いため、プラグイン本体を uninstall する前にこれを実行する
tools:
  - Bash(atelier, crontab, rm, ls, gh)
  - Read
  - AskUserQuestion
---

**プラグイン本体を uninstall する前に実行する**(Claude Code のプラグイン機構に uninstall フックは無く、自動 cleanup は構造的に不可能)。hooks とスキルはプラグイン無効化で自動的に解除されるので、ここで片付けるのは**プラグインの外に作られたもの**だけ。

## 手順

### 1. 旧版の残骸確認(v0.2.x 以前から使っている場合のみ)

旧版(factory 名義・#122 で撤去)の無人運転機構が crontab に行を残していることがある。`crontab -l 2>/dev/null | grep -n factory` でヒットしたら、`# factory-tick begin / end` のマーカーブロックを手で除去する(放置すると uninstall 後も cron が `claude -p` を起動し続ける)。

### 2. ローカル状態の削除

削除前に、退避したいもの(ジャーナル `.agents/journal/`・台帳)が無いか人間に確認してから:

```bash
rm -rf .agents/
```

これで atelier バイナリ・台帳・ジャーナル(旧版の運転状態ファイルが残っていればそれも)がすべて消える(`.agents/` は gitignore 領域なのでリポジトリには影響しない)。

### 3. 残るものの一覧提示(削除しない)

以下は**プロジェクトの所有物または人間判断の領域**なので削除せず、一覧にして提示する:

| 残るもの | 外したい場合 |
| --- | --- |
| `.atelier/`(地図・ownership・flags) | プロジェクトが不要と判断したら通常の PR で削除 |
| `docs/adr/` のローカル ADR | 同上(意思決定の記録なので残す価値が高い) |
| `.github/` テンプレート・atelier-issue-check.yml・dependabot.yml | 同上。workflow を消す場合は required check 解除が先 |
| CLAUDE.md のマーカー節 | 同上 |
| 運用ラベル 6 種・Projects ボード | `gh label delete` / ボードは Web UI |
| branch protection の required contexts(atelier-issue-check / atelier-review) | `gh api -X PUT .../branches/main/protection` で contexts から除去(**workflow 削除より先に行う** — 逆順だと必須チェックが永遠に pending になり全 PR が詰まる) |

### 4. 完了報告と案内

- 掃除した項目(.agents ほか)と残した項目の表を提示する
- 最後に案内する: 「プラグイン本体の削除はこの後 `/plugin` の Manage から。hooks・スキルはそれで自動的に解除されます」

## 禁止事項

- committed ファイル(.atelier/ / docs/ / .github/ / CLAUDE.md)の削除(提示のみ。削除はプロジェクトの通常 PR で)
- GitHub 側(ラベル・ボード・保護設定)の変更(コマンド例の提示のみ)
- 確認なしの `.agents/` 削除(退避確認が先)
