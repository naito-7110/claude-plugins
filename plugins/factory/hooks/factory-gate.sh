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

if [ -z "$FACTORY_BIN" ]; then
  # factory 管理下の判定 = プロジェクトルートに .factory/ が存在すること。
  # プラグインはユーザーレベルで有効化され hook は全リポジトリで発火するため、
  # 管理外のリポジトリを fail-closed の人質にしない(#103)。
  if [ ! -d ".factory" ]; then
    exit 0
  fi
  # 管理下でバイナリ欠落は fail-closed(判定できないままツール実行を通さない)。
  # 復旧はユーザーの ! 直接実行(hook を通らない)1 行で行える。
  echo "factory-gate: ゲートを実行できないため停止します(factory 管理下ですが factory バイナリが見つかりません)。次の 1 行を ! プレフィックスで直接実行して取得してください:" >&2
  echo '!t=$(gh release list -R naito-7110/claude-plugins --json tagName -q '\''[.[].tagName|select(startswith("factory/v"))][0]'\''); v=${t#factory/v}; o=$(uname -s|tr A-Z a-z); a=$(uname -m|sed '\''s/x86_64/amd64/;s/aarch64/arm64/'\''); f="factory_${v}_${o}_${a}.tar.gz"; mkdir -p .agents/bin && cd .agents/bin && gh release download "$t" -R naito-7110/claude-plugins -p "$f" -p checksums.txt --clobber && { sha256sum -c --ignore-missing checksums.txt 2>/dev/null || shasum -a 256 -c <(grep "$f" checksums.txt); } && tar xzf "$f" factory && rm -f "$f" checksums.txt && cd ../..' >&2
  exit 2
fi

exec "$FACTORY_BIN" gate
