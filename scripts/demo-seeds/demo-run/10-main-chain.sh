#!/usr/bin/env bash
# 段一·主链（票 demo-2026-10-30/03）：提交 → 受理 → 初始路由 → 运输准备 → 送站预报 → 收寄 → 自营段（登班次、两池预占、
# 装载分配、交出、问询、出发、交进口岸）→ 干线（委托先于交出、订舱、承运答复、交给干线承运商、段采用、交进新加坡枢纽）
# → 尾程交接 → 派送任务 → 派送尝试 → 有效交付 → PS 终局 → 客户轨迹视图。
#
# 用法：10-main-chain.sh            每跑一次是一件新包裹（约占 9600 余额；up.sh 补的余额约够十二件）。
# 状态写入 .run/state.env：MAIN_REQ、MAIN_PARCEL、MAIN_CHARGE 等，供 30-settlement.sh 读（它读最近一次主链）。
# 班次日期不是写死的：取运输准备对该包裹各段选中的班次，第一段是当天的班次（上海当地日），第二、三段随窗口落在次日。
# 现场预热时先跑一遍确认环境，演示时再跑一遍即可。证据只记 S。
set -euo pipefail
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"
load_env
export IDP_PARCEL_API_BASE
source "$DEMO_DIR/chain.sh"

STAMP="$(date -u +%H%M%S)"
section "主链·一件包裹（上海当地日 $(today_sha)）"
ch_submit
ch_intake
ch_selfrun
ch_trunk
ch_lastmile
ch_final

state_set MAIN_REQ "$REQ"; state_set MAIN_PARCEL "$PARCEL"; state_set MAIN_PREF "$PREF"; state_set MAIN_CHARGE "$CHARGE"
state_set MAIN_COST "$COST"; state_set MAIN_COST_FEE "$COST_FEE"; state_set MAIN_COST_AMOUNT "$COST_AMOUNT"
state_set MAIN_PLAN "$PLAN"; state_set MAIN_STAMP "$STAMP"
printf '\n主链完成：委托 %s，包裹 %s，客户费用 %s\n' "$REQ" "$PARCEL" "$CHARGE"
