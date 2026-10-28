#!/usr/bin/env bash
# 段二·关务旁路（票 demo-2026-10-30/03）：一件关务专项包裹走完：提交 → 受理 → 收寄 → 立案、授权确认、申报提交 →
# 起合成报关服务商，等 dispatch 发送，送异步技术回执 → 报关服务商回放行结果 FULL → 放行门禁核对（MET）→ 自营段 →
# 干线（委托先于交出）→ 交给干线承运商，交出门禁读数是 ADMITTED（放行在先）→ 尾程 → 有效交付。
#
# **一库一件**：declare-one-parcel.sh 头注写死一个申报单元与义务引用（SYN-UNIT-CN-EXPORT-DEMO-01 / SYN-OBL-CN-EXPORT），同库第二件
# 关务包裹立案会答 200 SCOPE_CONFLICT。要换单元就得往种子里再登一组单元就绪与门禁目录行，那是改种子，不在本票范围；所以本段在一个库里
# 只能走一次，再走先 up.sh <库名> --reset 复灌。已走过本段就直接停并说明（不去撞 SCOPE_CONFLICT）。
# 放行结果的 occurredAt 取「此刻往前 5 秒」：服务端拒绝比自己时钟晚的时刻（EVALUATION_INSTANT_UNTRUSTED）。
# 合成报关服务商（declaration-broker）本段起、本段收；它只听 127.0.0.1:19180。证据只记 S。
set -euo pipefail
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"
load_env
export IDP_PARCEL_API_BASE
source "$DEMO_DIR/chain.sh"

if [[ -n "${CUSTOMS_DONE:-}" ]]; then
  printf '本库的关务段已经走过一次（演示申报单元一库一件）。要再走：%s/up.sh %s --reset\n' "$DEMO_DIR" "$IDP_PARCEL_DEMO_DB" >&2
  exit 1
fi

unit=SYN-UNIT-CN-EXPORT-DEMO-01
STAMP="$(date -u +%H%M%S)"
section "关务旁路·一件关务专项包裹"

# 先起合成报关服务商，再让 dispatch 有地方发（申报一提交，dispatch 下一拍就发）。
if [[ -f "$RUN_DIR/declaration-broker.pid" ]] && kill -0 "$(cat "$RUN_DIR/declaration-broker.pid")" 2>/dev/null; then
  echo "合成报关服务商已在跑"
else
  nohup "$BIN_DIR/declaration-broker" >"$RUN_DIR/declaration-broker.log" 2>&1 &
  echo $! > "$RUN_DIR/declaration-broker.pid"
  sleep 1
  kill -0 "$(cat "$RUN_DIR/declaration-broker.pid")" || { echo '[失败] 合成报关服务商起不来：' >&2; cat "$RUN_DIR/declaration-broker.log" >&2; exit 1; }
  printf '%-34s → 127.0.0.1:19180（PID %s）\n' "起合成报关服务商" "$(cat "$RUN_DIR/declaration-broker.pid")"
fi
stop_broker() { kill "$(cat "$RUN_DIR/declaration-broker.pid")" 2>/dev/null || true; rm -f "$RUN_DIR/declaration-broker.pid"; }
trap 'stop_broker; rm -f "$LAST"' EXIT

ch_submit
ch_intake

out="$(bash "$SEEDS_DIR/declare-one-parcel.sh" "$PARCEL")" || { printf '[失败] declare-one-parcel.sh：%s\n' "$out" >&2; exit 1; }
printf '%-34s → %s\n' "立案、授权确认、申报提交" "$out"
CASE="$(sed -n 's/^案件 \([^；]*\)；.*/\1/p' <<<"$out")"
VERSION="$(sed -n 's/.*提交版本 \([^；]*\)；.*/\1/p' <<<"$out")"
[[ -n "$CASE" && -n "$VERSION" ]] || { printf '[失败] 解析不出案件与提交版本：%s\n' "$out" >&2; exit 1; }
state_set CUSTOMS_DONE started

out="$(bash "$SEEDS_DIR/send-one-declaration.sh" "$VERSION")" || { printf '[失败] send-one-declaration.sh：%s\n' "$out" >&2; exit 1; }
printf '%-34s → %s\n' "dispatch 经通道发送 + 技术回执" "$out"
if grep -q "$VERSION" "$RUN_DIR/declaration-broker.log"; then
  printf '%-34s → 服务商日志记下提交版本 %s\n' "合成服务商收到申报" "$VERSION"
else
  printf '[失败] 合成服务商日志里没有提交版本 %s\n' "$VERSION" >&2; exit 1
fi

btok="$(integration_token syn-integration-customs-broker-01 syn-customs-broker-secret)"
step "放行结果 FULL（报关服务商令牌）" "$btok" POST /customs/external-results \
  "$(pj sourceId="SYN-RELEASE-${STAMP}" layer=RELEASE_RESULT role=CUSTOMS_AUTHORITY rawSemantics="SYN 放行回执" claimedVersion="$VERSION" \
      attempt:=1 scope="$unit" occurredAt="$(date -u -d '-5 seconds' +%Y-%m-%dT%H:%M:%SZ)" \
      release:='{"kind":"FULL","authority":"SYN-AUTHORITY/customs-cn"}')" 201:RESULT_RECORDED
step "放行门禁核对（申报单元范围）" "$OP" POST /customs-release-gate-verifications \
  "$(pj scope="$unit" action=CROSS_CUSTOMS_MOVEMENT boundary=SYN-PROC-CN-EXPORT)" 201:RECORDED conclusion preconditions

ch_selfrun
ch_trunk
if [[ "$GATE_READING" != ADMITTED ]]; then
  printf '\n[失败] 交给干线承运商的交出门禁读数是 %s，要 ADMITTED（放行在先）。上一步原始响应已在上方。\n' "$GATE_READING" >&2
  exit 1
fi
printf '%-34s → 交出门禁读数 ADMITTED（放行 FULL 在先）\n' "交出门禁核对"
ch_lastmile
ch_final

state_set CUSTOMS_REQ "$REQ"; state_set CUSTOMS_PARCEL "$PARCEL"; state_set CUSTOMS_CASE "$CASE"; state_set CUSTOMS_VERSION "$VERSION"
state_set CUSTOMS_DONE finished
printf '\n关务段完成：包裹 %s，案件 %s，提交版本 %s\n' "$PARCEL" "$CASE" "$VERSION"
