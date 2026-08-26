#!/usr/bin/env bash
# 从 WSL 起 admin-web 的 vite 开发服。走 WSL 是因为本仓的 node_modules 是 WSL 侧
# pnpm 装的 POSIX 链接农场，Windows 进程解析不到那些链接（票 07 环境注记）。
#
# **每次核 DOM 前重起，不要复用已在跑的那个。** vite 靠 inotify 失效模块图，而
# /mnt/d 是 Windows 侧写入，事件传不进 WSL——改完 tsx 不重起，dev server 会继续
# 交旧的转译产物，页面看起来像「新列没接上」，实则读的是上一版代码。这一条踩过
# 一次：端点已答出新字段、真库测试全绿，只有 DOM 核不中。
set -uo pipefail

PORT=${PORT:-5199}
API=${PARCEL_API_TARGET:-http://127.0.0.1:19081}
export PATH="$HOME/.local/node22/bin:$PATH"
export PARCEL_API_TARGET="$API"

cd /mnt/d/tops/idp-parcel/apps/admin-web || exit 1
exec node ./node_modules/vite/bin/vite.js --port "$PORT" --strictPort --host 127.0.0.1
