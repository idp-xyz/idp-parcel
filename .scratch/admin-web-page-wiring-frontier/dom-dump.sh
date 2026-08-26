#!/usr/bin/env bash
# 从 WSL 调 Windows 侧 Edge 取无头 DOM。走 WSL 而不是 PowerShell，是因为
# PowerShell 5.1 会按控制台代码页重编码子进程 stdout，中文列头与状态词会在
# 落盘时变成问号——取证脚本自己把证据毁掉就没法核了。bash 重定向不动字节。
set -uo pipefail

EDGE="/mnt/c/Program Files (x86)/Microsoft/Edge/Application/msedge.exe"
BASE=${1:-http://127.0.0.1:5199}
OUT=${2:-/tmp/parcel-dom}
PROFILE=$(mktemp -d)
trap 'rm -rf "$PROFILE"' EXIT

mkdir -p "$OUT"
for id in "${@:3}"; do
  "$EDGE" --headless=new --disable-gpu --no-sandbox \
    --user-data-dir="$(wslpath -w "$PROFILE")" \
    --virtual-time-budget=9000 \
    --dump-dom "$BASE/#/$id" 2>/dev/null | tr -d '\r' > "$OUT/$id.html"
  echo "$id -> $(wc -c <"$OUT/$id.html") bytes"
done
