#!/usr/bin/env bash
# atelier-gate.sh — atelier の機械的ゲート(PreToolUse hook の薄い入口)。
#
# 判定の実体は atelier バイナリの gate サブコマンド(ユニットテスト済みの Go)に
# 一本化されており、本スクリプトは「プロジェクトルートへの cd → バイナリ解決 →
# exec」だけを担う(#73)。stdin の hook JSON はそのままバイナリに渡り、
# ブロック時はバイナリが理由を stderr に出して exit 2 を返す(hook 契約)。
#
# 依存: bash / atelier バイナリ(.agents/bin/atelier または PATH。
#       無ければ init の再実行で取得する)。
set -euo pipefail

# hooks は「現在のディレクトリ」で実行される(公式仕様)。相対パス
# (.agents/ / git コマンド)の基準をプロジェクトルートに固定する
# (バイナリ側でも CLAUDE_PROJECT_DIR へ chdir するため二重でも害はない)。
cd "${CLAUDE_PROJECT_DIR:-.}" 2>/dev/null || true

ATELIER_BIN="${ATELIER_BIN:-}"
if [ -z "$ATELIER_BIN" ]; then
  if [ -x ".agents/bin/atelier" ]; then
    ATELIER_BIN=".agents/bin/atelier"
  elif command -v atelier >/dev/null 2>&1; then
    ATELIER_BIN="atelier"
  fi
fi

if [ -z "$ATELIER_BIN" ]; then
  # atelier 管理下の判定 = プロジェクトルートに .atelier/ が存在すること。
  # プラグインはユーザーレベルで有効化され hook は全リポジトリで発火するため、
  # 管理外のリポジトリを fail-closed の人質にしない(#103)。
  if [ ! -d ".atelier" ]; then
    exit 0
  fi
  # 管理下でバイナリ欠落は fail-closed(判定できないままツール実行を通さない)。
  # 復旧はユーザーの ! 直接実行(hook を通らない)1 行で行える。
  echo "atelier-gate: ゲートを実行できないため停止します(atelier 管理下ですが atelier バイナリが見つかりません)。次の 1 行を ! プレフィックスで直接実行して取得してください:" >&2
  echo '!t=$(gh release list -R naito-7110/claude-plugins --json tagName -q '\''[.[].tagName|select(startswith("atelier/v"))][0]'\''); v=${t#atelier/v}; o=$(uname -s|tr A-Z a-z); a=$(uname -m|sed '\''s/x86_64/amd64/;s/aarch64/arm64/'\''); f="atelier_${v}_${o}_${a}.tar.gz"; mkdir -p .agents/bin && cd .agents/bin && gh release download "$t" -R naito-7110/claude-plugins -p "$f" -p checksums.txt --clobber && { sha256sum -c --ignore-missing checksums.txt 2>/dev/null || shasum -a 256 -c <(grep "$f" checksums.txt); } && tar xzf "$f" atelier && rm -f "$f" checksums.txt && cd ../..' >&2
  exit 2
fi

exec "$ATELIER_BIN" gate
