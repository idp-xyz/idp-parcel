#!/usr/bin/env bash
# 工作台就绪度总览是 page-registry 的 liveIds 派生出来的，不是第二份状态。
# 接线一页要在这里能看见变化——看不见就说明页面自己写了状态，那是双写。
set -uo pipefail
F=${1:-/tmp/parcel-dom/workbench.html}
echo '--- 总览四格（标签后紧跟的数字） ---'
grep -oP '(?<=>)(已接线|演示|骨架|占位)</span><span[^>]*>[0-9]+' "$F" | sed 's/<[^>]*>/ /g'
echo '--- 分区「已接线 n/m」 ---'
grep -oP '已接线 [0-9]+/[0-9]+' "$F"
