#!/usr/bin/env bash
# factory-gate.sh — factory の機械的ゲート(PreToolUse hook の薄い入口)。
#
# 判定ロジックの実体は factory バイナリ(issue verify / pr verify)に一本化し、
# 本スクリプトは「ツール呼び出しの検出 → バイナリ実行 → exit 2 変換」だけを担う。
# 拒否理由は stderr に出し、エージェントがそのままエスカレーション材料に使える。
#
# ゲート(#4 の hook 集約決定):
#   1. main への直 push / force push: 常にブロック
#   2. push ゲート: agent/issue-<n>-* ブランチの push 前に issue の状態を検証
#   3. マージゲート: merge:agent + PR↔issue 整合 + CI green を確認
#   4. 無人モード(.agents/unattended が存在):
#      - docs/adr/ への Write / Edit をブロック(改憲は対話専用)
#      - merge:agent ラベルの付与・変更をブロック(付与は grooming 限定)
#      - 配車(Task)前に対象 issue を検証
#
# 依存: bash / jq / gh / factory バイナリ(.agents/bin/factory または PATH。
#       無ければ init の再実行で取得する)。
set -euo pipefail

INPUT=$(cat)
TOOL=$(jq -r '.tool_name // empty' <<<"$INPUT")

deny() {
  echo "factory-gate: $1" >&2
  exit 2
}

UNATTENDED=0
[ -f ".agents/unattended" ] && UNATTENDED=1

FACTORY_BIN="${FACTORY_BIN:-}"
if [ -z "$FACTORY_BIN" ]; then
  if [ -x ".agents/bin/factory" ]; then
    FACTORY_BIN=".agents/bin/factory"
  elif command -v factory >/dev/null 2>&1; then
    FACTORY_BIN="factory"
  fi
fi

require_bin() {
  [ -n "$FACTORY_BIN" ] || deny "$1(factory バイナリが見つかりません。/factory:init の再実行で取得してください)"
}

case "$TOOL" in
  Write | Edit)
    # 無人モードの改憲ブロック。対話中は permission フローに任せる。
    [ "$UNATTENDED" = 1 ] || exit 0
    FILE=$(jq -r '.tool_input.file_path // empty' <<<"$INPUT")
    case "$FILE" in
      docs/adr/* | */docs/adr/*)
        deny "無人モード中は docs/adr/ への書き込みを禁止しています(改憲は対話専用 — /factory:adr)"
        ;;
    esac
    exit 0
    ;;
  Task)
    # 配車ゲート(無人モードのみ): 対象 issue が配車条件を満たすか。
    [ "$UNATTENDED" = 1 ] || exit 0
    PROMPT=$(jq -r '.tool_input.prompt // empty' <<<"$INPUT")
    N=$(grep -oE 'issue[[:space:]]*#?[0-9]+' <<<"$PROMPT" | grep -oE '[0-9]+' | head -1 || true)
    [ -n "$N" ] || deny "無人配車のプロンプトに issue 番号が必要です(配車規約)"
    require_bin "無人配車を検証できません"
    "$FACTORY_BIN" issue verify --number "$N" >&2 ||
      deny "issue #$N は配車条件を満たしません(上記の理由を確認してください)"
    exit 0
    ;;
  Bash) ;;
  *)
    exit 0
    ;;
esac

CMD=$(jq -r '.tool_input.command // empty' <<<"$INPUT")
[ -n "$CMD" ] || exit 0

# --- 1. main への直 push / force push -------------------------------------
if grep -qE '(^|[;&|[:space:]])git[[:space:]][^;&|]*push' <<<"$CMD"; then
  if grep -qE 'push[^;&|]*[[:space:]](-f|--force|--force-with-lease)([[:space:]]|$)' <<<"$CMD" &&
    grep -qE '(main|master)' <<<"$CMD"; then
    deny "main への force push は禁止です(git-workflow)"
  fi
  if grep -qE 'push[^;&|]*[[:space:]](origin[[:space:]]+)?(main|master)([[:space:]]|$|:)' <<<"$CMD"; then
    deny "main への直 push は禁止です(PR を経由してください — git-workflow)"
  fi

  # --- 2. push ゲート: 作業ブランチの issue 状態 ---------------------------
  # symbolic-ref はコミットゼロ(unborn)のブランチでも名前を返す
  BRANCH=$(git symbolic-ref --short HEAD 2>/dev/null ||
    git rev-parse --abbrev-ref HEAD 2>/dev/null || echo "")
  case "$BRANCH" in
    main | master)
      deny "main ブランチからの push は禁止です(作業は agent/issue-<n>-<slug> ブランチで)"
      ;;
    agent/issue-*)
      N=$(sed -E 's|agent/issue-([0-9]+).*|\1|' <<<"$BRANCH")
      if [ -n "$FACTORY_BIN" ] && [ "$N" != "$BRANCH" ]; then
        "$FACTORY_BIN" issue verify --number "$N" >&2 ||
          deny "issue #$N の状態が push 条件を満たしません(ラベルなしの実装は push できません)"
      fi
      ;;
  esac
fi

# --- 3. マージゲート --------------------------------------------------------
if grep -qE 'gh[[:space:]]+pr[[:space:]]+merge' <<<"$CMD" ||
  grep -qE 'gh[[:space:]]+api[^;&|]*/pulls?/[0-9]+/merge' <<<"$CMD"; then
  N=$(grep -oE '(pr[[:space:]]+merge[[:space:]]+#?[0-9]+|/pulls?/[0-9]+/merge)' <<<"$CMD" |
    grep -oE '[0-9]+' | head -1 || true)
  if [ -z "$N" ]; then
    N=$(gh pr view --json number -q '.number' 2>/dev/null || true)
  fi
  [ -n "$N" ] || deny "マージ対象の PR 番号を特定できません"
  require_bin "マージゲートを実行できないため停止します"

  "$FACTORY_BIN" pr verify --number "$N" >&2 ||
    deny "PR #$N は PR↔issue 整合を満たしません(上記の理由)"

  LINKED=$(gh pr view "$N" --json closingIssuesReferences \
    -q '.closingIssuesReferences[0].number' 2>/dev/null || true)
  [ -n "$LINKED" ] || deny "PR #$N に Closes での issue 紐づけがありません(agent マージは不可)"
  gh issue view "$LINKED" --json labels -q '.labels[].name' 2>/dev/null |
    grep -qx 'merge:agent' ||
    deny "issue #$LINKED に merge:agent がありません。人間のレビュー・マージを待ってください(merge-policy)"

  gh pr checks "$N" >/dev/null 2>&1 ||
    deny "PR #$N の CI が green ではありません(merge-policy の実行条件)"

  SHA=$(gh pr view "$N" --json headRefOid -q '.headRefOid' 2>/dev/null || true)
  REVIEW=$(gh api "repos/{owner}/{repo}/commits/$SHA/status"     -q '[.statuses[] | select(.context == "factory-review")][0].state' 2>/dev/null || true)
  [ "$REVIEW" = "success" ] ||
    deny "PR #$N は別コンテキストレビュア(factory-review)の green がありません(merge-policy の実行条件)"
fi

# --- 4. 無人モードの merge:agent 付与ブロック -------------------------------
if [ "$UNATTENDED" = 1 ] &&
  grep -qE 'merge:agent' <<<"$CMD" &&
  grep -qE '(gh[[:space:]]+issue[[:space:]]+edit|gh[[:space:]]+pr[[:space:]]+edit|--add-label)' <<<"$CMD"; then
  deny "無人モード中の merge:agent 付与・変更は禁止です(付与は grooming の場に限定 — merge-policy)"
fi

exit 0
