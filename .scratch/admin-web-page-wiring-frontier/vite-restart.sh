#!/usr/bin/env bash
# 重起 admin-web 的 vite 开发服并等它真的在听。
#
# 「重起」是取 DOM 前的必做步骤而不是故障处置:/mnt/d 的写入不产生 WSL inotify 事件,
# 不重起就会核到上一版转译产物(理由写在 vite-dev.sh 里)。
#
# 单独成脚是因为 PowerShell → wsl → tmux 三层引号嵌套极易被吃掉,而命令被吃掉时
# tmux 只是开一个空 shell、不报错,看起来像 vite 起了又立刻死。
set -uo pipefail

SESSION=vite-admin-web
LOG=/tmp/vite-5199.log
PORT=${PORT:-5199}

cd /mnt/d/tops/idp-parcel || exit 1

tmux kill-session -t "$SESSION" 2>/dev/null
for _ in $(seq 1 20); do
  ss -ltn 2>/dev/null | grep -q ":$PORT " || break
  sleep 0.5
done

rm -f "$LOG"
tmux new-session -d -s "$SESSION" \
  "PORT=$PORT bash .scratch/admin-web-page-wiring-frontier/vite-dev.sh >$LOG 2>&1"

for _ in $(seq 1 40); do
  if ss -ltn 2>/dev/null | grep -q ":$PORT "; then
    echo "vite 已在 :$PORT 监听"
    exit 0
  fi
  sleep 0.5
done

echo "vite 未起来,日志:"
cat "$LOG" 2>/dev/null
exit 1
