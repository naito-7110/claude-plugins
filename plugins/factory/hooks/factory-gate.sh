#!/usr/bin/env bash
# factory-gate.sh — factory の機械的ゲート(PreToolUse hook の薄い入口)。
#
# 判定の実体は factory バイナリの gate サブコマンド(ユニットテスト済みの Go)に
# 一本化されており、本スクリプトは「プロジェクトルートへの cd → バイナリ解決 →
# exec」だけを担う(#73)。stdin の hook JSON はそのままバイナリに渡り、
# ブロック時はバイナリが理由を stderr に出して exit 2 を返す(hook 契約)。
#
# 依存: bash / factory バイナリ(.agents/bin/factory または PATH。
#       無ければ init の再実行で取得する)。
set -euo pipefail

# hooks は「現在のディレクトリ」で実行される(公式仕様)。相対パス
# (.agents/ / git コマンド)の基準をプロジェクトルートに固定する
# (バイナリ側でも CLAUDE_PROJECT_DIR へ chdir するため二重でも害はない)。
cd "${CLAUDE_PROJECT_DIR:-.}" 2>/dev/null || true

FACTORY_BIN="${FACTORY_BIN:-}"
if [ -z "$FACTORY_BIN" ]; then
  if [ -x ".agents/bin/factory" ]; then
    FACTORY_BIN=".agents/bin/factory"
  elif command -v factory >/dev/null 2>&1; then
    FACTORY_BIN="factory"
  fi
fi

# fail-closed: 判定できないままツール実行を通さない。
if [ -z "$FACTORY_BIN" ]; then
  echo "factory-gate: ゲートを実行できないため停止します(factory バイナリが見つかりません。/factory:init の再実行で取得してください)" >&2
  exit 2
fi

exec "$FACTORY_BIN" gate
