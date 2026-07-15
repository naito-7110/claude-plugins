---
description: Analyze open GitHub issues and list them in a single terminal-friendly table
tools:
  - Bash(gh issue list, gh issue view, gh repo view)
  - Read
---

GitHub の open issue を **単一のテーブル**で一覧表示します。

## 手順

1. issue を取得する（引数があれば `--label` / `--state` / `--search` に素通しする）:

   ```sh
   gh issue list --state open --limit 100 --json number,title,labels,url,updatedAt
   ```

2. **テーブルは 1 つだけ**にする。カテゴリやレーンで複数テーブルに分割しない。

3. 列構成（この順）:

   | 列 | 内容 |
   | --- | --- |
   | # | **生 URL をそのまま書く**（例: `https://github.com/owner/repo/issues/82`）。`[#N](url)` の markdown リンク形式はターミナルでクリックできないため**禁止**（ターミナルは生 URL を自動リンク化する） |
   | 件名 | issue タイトルの**短い要約（全角 15〜20 字目安）**。原文をそのまま貼らず意訳して詰める（正式タイトルはリンク先で読める）。プレフィックス（`@rsi/ui:` 等の対象）は残す |
   | ラベル | ラベルをカンマ区切り（`priority:` は絵文字に吸収して省略可） |
   | 状況 | **1〜2 文の説明**: いまどういう状態か・何を待っているか・次のアクションは何か。ラベルの言い換え（「着手可」だけ等）で済ませない。会話・issue 本文（あれば運用台帳）から根拠を取る |

4. 状況の導出規則（ラベルベース・**絵文字で色分けする**。ターミナルの markdown 表は文字色を持てないため絵文字が色の代替）:
   - `agent-wip` → 🟡 作業中
   - `agent-ok` → 🟢 着手可
   - `needs-human` → 🔴 人間判断待ち
   - 上記いずれも無い → ⚪ 未トリアージ
   - `priority:high` → 🔥 を併記 / `priority:low` → ❄️ を併記
   - `bug` → 🐛 を併記
   - 絵文字は状況文の先頭に付ける。台帳や会話から「クローズ候補」「parked」等が分かる場合は ✅ / 🅿️ を使う。**状況の本文は絵文字と別に必ず書く**（推測で書かず、根拠が無い項目は「本文未読・ラベルのみ」と正直に書く）
   - 凡例をテーブルの直前に 1 行で示す

5. 並び順は issue 番号の降順（新しい順）。

6. テーブルの後に、気づき（クローズ候補・stale 等）があれば 2〜3 行の箇条書きで添える。無ければ省略。
