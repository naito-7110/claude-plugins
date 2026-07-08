#!/usr/bin/env bash
# atelier-bootstrap.sh — ゲートバイナリのセルフブートストラップ(SessionStart hook)。
#
# バイナリの欠落/ピン不一致を検知したとき、公開 Releases から curl で取得し、
# プラグイン同梱のピン(pin/version + pin/checksums.txt)と照合が成功した場合のみ
# .agents/bin/ に配置する(ADR 0003)。信頼の根はレビュー済み commit のピンであり、
# リリースアセット側の checksums.txt は信用しない。
#
# 失敗(オフライン・タイムアウト・チェックサム不一致)はすべて exit 0 で何もしない:
# ブートストラップは利便であって防御の前提ではなく、バイナリ欠落時の停止は
# atelier-gate.sh の fail-closed + 手動 1 行案内が現行どおり担う。
#
# ピン不一致の検知は、配置時に書くマーカー(.agents/bin/atelier.version)との比較で
# 行う(バイナリ自身は version を報告しないため)。マーカーの無い手動配置バイナリは
# 不一致として扱い、ピンへ収束させる(ADR 0003「全マシンがプラグインのピンに収束」)。
#
# 依存: bash / curl / tar / sha256sum または shasum(gh CLI・GitHub 認証は不要)。
# windows は対象外(従来の手動手順)。
set -uo pipefail

cd "${CLAUDE_PROJECT_DIR:-.}" 2>/dev/null || exit 0

# atelier 管理下(.atelier/ あり)のみ対象。管理外リポジトリには書き込まない(#103 と同じ判定)
[ -d .atelier ] || exit 0

PLUGIN_ROOT="${CLAUDE_PLUGIN_ROOT:-}"
PIN_FILE="$PLUGIN_ROOT/pin/version"
SUMS_FILE="$PLUGIN_ROOT/pin/checksums.txt"
{ [ -n "$PLUGIN_ROOT" ] && [ -f "$PIN_FILE" ] && [ -f "$SUMS_FILE" ]; } || exit 0

PIN=$(tr -d '[:space:]' < "$PIN_FILE")
[ -n "$PIN" ] || exit 0

BIN=".agents/bin/atelier"
MARKER=".agents/bin/atelier.version"

# 既にピンと一致していれば何もしない(ゲート発火パスにネットワーク I/O を混ぜない、
# の SessionStart 版: 一致時のセッション開始は即座に返す)
if [ -x "$BIN" ] && [ -f "$MARKER" ] && [ "$(tr -d '[:space:]' < "$MARKER")" = "$PIN" ]; then
  exit 0
fi

os=$(uname -s | tr '[:upper:]' '[:lower:]')
case "$os" in
  linux | darwin) ;;
  *) exit 0 ;;
esac
arch=$(uname -m)
case "$arch" in
  x86_64) arch=amd64 ;;
  aarch64 | arm64) arch=arm64 ;;
  *) exit 0 ;;
esac

# .agents / .agents/bin が symlink の場合は書き込まない(commit された symlink 経由で
# リポジトリ外へ書かされる経路の遮断。gitignore は commit を防がない)
if [ -L .agents ] || [ -L .agents/bin ]; then
  exit 0
fi

ASSET="atelier_${PIN}_${os}_${arch}.tar.gz"
# ATELIER_RELEASE_BASE_URL / ATELIER_BOOTSTRAP_MAX_TIME はテストシーム。
# 取得元を差し替えない本番経路では https 以外のスキームを拒否する
CURL_OPTS=(-fsSL --max-time "${ATELIER_BOOTSTRAP_MAX_TIME:-20}")
if [ -z "${ATELIER_RELEASE_BASE_URL:-}" ]; then
  CURL_OPTS+=(--proto '=https')
fi
BASE_URL="${ATELIER_RELEASE_BASE_URL:-https://github.com/naito-7110/claude-plugins/releases/download}"
# タグは atelier/vX.Y.Z(スラッシュ入り)のため URL では %2F にエンコードする
URL="${BASE_URL}/atelier%2Fv${PIN}/${ASSET}"

TMP=$(mktemp -d) || exit 0
trap 'rm -rf "$TMP"' EXIT

if ! curl "${CURL_OPTS[@]}" -o "$TMP/$ASSET" "$URL" 2>/dev/null; then
  echo "atelier-bootstrap: 取得できませんでした(オフライン等)。ゲートは従来どおり fail-closed で案内します" >&2
  exit 0
fi

EXPECTED=$(awk -v f="$ASSET" '$2 == f { print $1 }' "$SUMS_FILE")
[ -n "$EXPECTED" ] || exit 0
if command -v sha256sum > /dev/null 2>&1; then
  ACTUAL=$(sha256sum "$TMP/$ASSET" | awk '{ print $1 }')
else
  ACTUAL=$(shasum -a 256 "$TMP/$ASSET" | awk '{ print $1 }')
fi
if [ "$ACTUAL" != "$EXPECTED" ]; then
  echo "atelier-bootstrap: チェックサムが同梱ピンと一致しません(配置しません): $ASSET" >&2
  exit 0
fi

tar -xzf "$TMP/$ASSET" -C "$TMP" atelier 2>/dev/null || exit 0
mkdir -p .agents/bin || exit 0
install -m 0755 "$TMP/atelier" "$BIN" || exit 0
printf '%s\n' "$PIN" > "$MARKER"
echo "atelier-bootstrap: atelier v${PIN} を .agents/bin/ に配置しました(同梱ピンで検証済み)"
exit 0
