#!/usr/bin/env bash
# 从 WSL 调 Windows 侧 Edge 取无头 DOM。走 WSL 而不是 PowerShell，是因为
# PowerShell 5.1 会按控制台代码页重编码子进程 stdout，中文列头与状态词会在
# 落盘时变成问号——取证脚本自己把证据毁掉就没法核了。bash 重定向不动字节。
#
# 取 DOM 前先按 vite-dev.sh 重起开发服。/mnt/d 的写入不产生 WSL inotify 事件，
# 不重起会核到上一版代码的转译产物——那次假失败长得跟「新列没接上」一模一样。
#
# 参数与 dom-check.sh 同序（先产物目录、再页 id），页面基址走 PARCEL_WEB_BASE。
# 两个脚本总是连着跑，前一版这里把基址排在第一位，串起来时极易错位一格：那样
# dump 会静默地一页都不取，check 转去核上一轮的旧产物，两边都不报错。
set -uo pipefail

EDGE="/mnt/c/Program Files (x86)/Microsoft/Edge/Application/msedge.exe"
OUT=${1:-/tmp/parcel-dom}
BASE=${PARCEL_WEB_BASE:-http://127.0.0.1:5199}
PROFILE=$(mktemp -d)
trap 'rm -rf "$PROFILE"' EXIT

mkdir -p "$OUT"
for id in "${@:2}"; do
  "$EDGE" --headless=new --disable-gpu --no-sandbox \
    --user-data-dir="$(wslpath -w "$PROFILE")" \
    --virtual-time-budget=9000 \
    --dump-dom "$BASE/#/$id" 2>/dev/null | tr -d '\r' > "$OUT/$id.html"
  echo "$id -> $(wc -c <"$OUT/$id.html") bytes"
done
