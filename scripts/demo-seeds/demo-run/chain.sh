#!/usr/bin/env bash
# 一件包裹的主链各段，做成函数供分段脚本 source（票 demo-2026-10-30/03）。载荷照动线文档与 psb/05 取证各节：
# 交接口 version 与 scope 每次各起一个（票 111 之前同版本串会让实际履约发生项认错）；已交接载荷要带 releasingEvidence、
# receivingEvidence、rule，不带 basis；装载分配重量行写整数；干线委托先于交给干线承运商；关务包裹先放行再交干线。
# 调用前要 source lib.sh 并 load_env；变量约定：REQ 委托号、PARCEL 包裹号、PREF 客户包裹参考、STAMP 本次戳。
# 证据只记 S。

CN_EAST=(SYN-DEVICE-OFFSITE-CN-EAST-01 OFFSITE SYN-OFFSITE-CN-EAST)
SG_LM=(SYN-DEVICE-OFFSITE-SG-LM-01 OFFSITE SYN-OFFSITE-SG-LM)

# 提交（首方客户渠道）并等到已接受、初始路由、三段运输准备都有结果。
ch_submit() {
  local out
  out="$("$SEEDS_DIR/submit-one-shipment.sh")" || { printf '[失败] submit-one-shipment.sh：%s\n' "$out" >&2; exit 1; }
  REQ="$(sed -n 's/^已提交 \([^ ]*\) .*/\1/p' <<<"$out")"
  PARCEL="$(sed -n 's/.*包裹号 \([A-Z0-9-]*\)（.*/\1/p' <<<"$out")"
  PREF="$(sed -n 's/.*客户包裹参考 \([^）]*\)）.*/\1/p' <<<"$out")"
  [[ -n "$REQ" && -n "$PARCEL" && -n "$PREF" ]] || { printf '[失败] 解析不出提交输出：%s\n' "$out" >&2; exit 1; }
  printf '%-34s → 201 SUBMITTED request=%s parcel=%s ref=%s\n' "提交委托（首方客户令牌）" "$REQ" "$PARCEL" "$PREF"
  OP="$(operator_token)"
  # 供应商预期成本行不带包裹号：提交前记下已有的成本版本，终局后新出现的那一笔就是这一件的 BUY（见 ch_final）。
  COSTS_BEFORE="$(curl -s -H "Authorization: Bearer $OP" "$API_BASE/settlement-charges?registry=supplier-expected-cost" | python3 -c 'import json,sys; print(",".join(r["version"] for r in json.load(sys.stdin).get("costs",[])))')"
  _accepted() { step "等 dispatch 接受（委托状态）" "$OP" GET "/shipment-request-views?shipmentRequestId=${REQ}" - 200:REQUEST_VIEW request.state >/dev/null 2>&1 && [[ "$(jget "$LAST" request.state)" == ACCEPTED ]]; }
  poll "委托成已接受" 90 _accepted
  printf '%-34s → request.state=ACCEPTED\n' "接受判断（dispatch 下一拍）"
  _routed() { curl -s -H "Authorization: Bearer $OP" "$API_BASE/route-plans?register=initial-route" > "$LAST"; [[ "$(jsel "$LAST" judgments declaredParcelId "$PARCEL" conclusion)" == ROUTE_FORMED ]]; }
  poll "初始路由" 90 _routed
  printf '%-34s → ROUTE_FORMED plan=%s\n' "初始路由" "$(jsel "$LAST" judgments declaredParcelId "$PARCEL" planVersion)"
  _prepared() {
    curl -s -H "Authorization: Bearer $OP" "$API_BASE/transport-fulfillment-records?registry=transport-preparation" > "$LAST"
    python3 - "$LAST" "$PARCEL" <<'PY'
import json, sys
rows = [r for r in json.load(open(sys.argv[1]))["preparations"] if r["object"] == sys.argv[2]]
sys.exit(0 if len(rows) >= 3 and all(r["outcome"] == "OPPORTUNITY_PREPARED" for r in rows) else 1)
PY
  }
  poll "三段运输准备" 90 _prepared
  PLAN="$(jsel "$LAST" preparations object "$PARCEL" plannedLeg | sed 's/#.*//')"
  SCHED1="$(python3 - "$LAST" "$PARCEL" 1 <<'PY'
import json, sys
for r in json.load(open(sys.argv[1]))["preparations"]:
    if r["object"] == sys.argv[2] and r["plannedLeg"].endswith("#" + sys.argv[3]):
        print(r["selectedSchedule"])
PY
)"
  SCHED2="$(python3 - "$LAST" "$PARCEL" 2 <<'PY'
import json, sys
for r in json.load(open(sys.argv[1]))["preparations"]:
    if r["object"] == sys.argv[2] and r["plannedLeg"].endswith("#" + sys.argv[3]):
        print(r["selectedSchedule"])
PY
)"
  printf '%-34s → 三段 OPPORTUNITY_PREPARED #1=%s #2=%s\n' "运输准备" "$SCHED1" "$SCHED2"
}

# 送站预报 → 收寄（上海枢纽）。收寄后等到有效网络收寄已采用（预估客户费用形成）再往下。
ch_intake() {
  local pre
  CHARGES_BEFORE="$(curl -s -H "Authorization: Bearer $OP" "$API_BASE/settlement-charges?registry=customer-charge" | python3 -c 'import json,sys; print(",".join(r["charge"] for r in json.load(sys.stdin).get("charges",[])))')"
  pre="$("$SEEDS_DIR/submit-one-pre-advice.sh" "$PREF" 2>/dev/null)" || { printf '[失败] submit-one-pre-advice.sh\n' >&2; exit 1; }
  printf '%-34s → DELIVERY_PRE_ADVICE_REGISTERED preAdvice=%s\n' "送站预报（首方客户令牌）" "$pre"
  local out
  out="$(IDP_PARCEL_PRE_ADVICE_ID="$pre" "$SEEDS_DIR/receive-one-delivery.sh" "$PREF")" || { printf '[失败] receive-one-delivery.sh：%s\n' "$out" >&2; exit 1; }
  [[ "$out" == 201\ INTAKE_FORMED* ]] || { printf '[失败] 收寄答复不是 201 INTAKE_FORMED：%s\n' "$out" >&2; exit 1; }
  printf '%-34s → %s\n' "收寄（上海枢纽作业设备）" "$out"
  # 客户费用行不带包裹号：收寄前先记下已有的费用号，收寄后新出现的那一笔就是这一件的预估客户费用。
  _estimated() {
    curl -s -H "Authorization: Bearer $OP" "$API_BASE/settlement-charges?registry=customer-charge" > "$LAST"
    CHARGE="$(python3 - "$LAST" "$CHARGES_BEFORE" <<'PY'
import json, sys
before = set(sys.argv[2].split(","))
for r in json.load(open(sys.argv[1])).get("charges", []):
    if r["charge"] not in before:
        print(r["charge"]); break
PY
)"
    [[ -n "$CHARGE" ]]
  }
  poll "预估客户费用形成" 90 _estimated
  printf '%-34s → charge=%s %s\n' "SELL 预估客户费用" "$CHARGE" "$(jsel "$LAST" charges charge "$CHARGE" stage)/$(jsel "$LAST" charges charge "$CHARGE" settlementAmount)"
}

_sched_register() { # $1 = 模板/日期 → 登班次（物化），打印班次
  local sched=$1 tpl date
  tpl="${sched%/*}"; date="${sched##*/}"
  step "登班次 ${sched}" "$OP" POST /transport-fulfillment-transport-schedule-registrations \
    "$(pj template="$tpl" runningDate="$date")" "201:SCHEDULE_REGISTERED,200:EXISTING_SCHEDULE" schedule.scheduleId schedule.departsAt
}

# 自营段 SHA→SZX：登班次、两池预占、装载分配、交出、问询、出发、交进深圳口岸。
ch_selfrun() {
  local now seg ho
  seg="SYN-SEG-SHA-SZX-${STAMP}"
  _sched_register "$SCHED1"
  now="$(now_utc)"
  local until; until="$(date -u -d '+1 day' +%Y-%m-%dT%H:%M:%SZ)"
  step "预占·重量池 10 KG" "$OP" POST /transport-fulfillment-capacity-reservations \
    "$(pj pool="${SCHED1}/WEIGHT" reservation="SYN-RES-${STAMP}-W" quantity:=10 unit=KG \
        source:='{"kind":"OPERATOR_DECLARATION","reference":"SYN-DEMAND-DECL-'"${STAMP}"'/v1"}' validUntil="$until" reservedAt="$now")" 201:* reservation.reservation
  step "预占·件数池 5 PCS" "$OP" POST /transport-fulfillment-capacity-reservations \
    "$(pj pool="${SCHED1}/PIECES" reservation="SYN-RES-${STAMP}-P" quantity:=5 unit=PCS \
        source:='{"kind":"OPERATOR_DECLARATION","reference":"SYN-DEMAND-DECL-'"${STAMP}"'/v1"}' validUntil="$until" reservedAt="$now")" 201:* reservation.reservation
  sleep 1
  local src='{"kind":"OPERATOR_DECLARATION","reference":"SYN-DEMAND-DECL-'"${STAMP}"'/v1"}'
  step "装载分配（重量行整数 2）" "$OP" POST /transport-fulfillment-load-assignment-registrations \
    "$(pj assignment="SYN-LOAD-${STAMP}" schedule="$SCHED1" members:='["'"$PARCEL"'"]' version=v1 assignedAt="$(now_utc)" \
        allocations:='[{"pool":"'"${SCHED1}"'/WEIGHT","reservation":"SYN-RES-'"${STAMP}"'-W","quantity":2,"quantityFromMembers":false,"unit":"KG","source":'"$src"'},{"pool":"'"${SCHED1}"'/PIECES","reservation":"SYN-RES-'"${STAMP}"'-P","quantity":0,"quantityFromMembers":true,"unit":"PCS","source":'"$src"'}]')" \
    201:* assignment
  _handover "上海枢纽交出·立自营段" "${CN_EAST[@]}" "SHA-${STAMP}" "SHA-v1-${STAMP}" SYN-NODE-SHA-HUB SYN-OFFSITE-CN-EAST \
    segment="$seg" plannedSegment="${PLAN}#1" segmentServiceAction=LINEHAUL releasingNode=SYN-NODE-SHA-HUB onwardNode=SYN-NODE-SZX-GATE
  sleep 1
  now="$(now_utc)"
  device_headers "${CN_EAST[@]}"
  step "出发前问询（门禁）" "$OP" POST /transport-fulfillment/departure-gate-previews \
    "$(pj schedule="$SCHED1" segment="$seg" departsAt="$now")" 200:DEPARTURE_GATE_PREVIEWED gateReading.answer
  device_headers "${CN_EAST[@]}"
  step "出发（作业事实）" "$OP" POST /transport-fulfillment/movement-facts \
    "$(pj fact="SYN-MOVE-${STAMP}-DEP" schedule="$SCHED1" kind=DEPARTURE location=SYN-NODE-SHA-HUB version=v1 occurredAt="$now" segment="$seg")" \
    201:MOVEMENT_FACT_RECORDED gateReading.answer
  _handover "交进深圳口岸" "${CN_EAST[@]}" "SZX-IN-${STAMP}" "SZX-IN-v1-${STAMP}" SYN-OFFSITE-CN-EAST SYN-NODE-SZX-GATE \
    receivingNode=SYN-NODE-SZX-GATE
}

# _handover 标签 设备三件… scope version releasedBy receivedBy [k=v…]：已交接载荷。
_handover() {
  local label=$1 dev=$2 kind=$3 ref=$4 scope=$5 version=$6 released=$7 received=$8
  shift 8
  device_headers "$dev" "$kind" "$ref"
  step "$label" "$OP" POST /transport-fulfillment/handovers \
    "$(pj object="$PARCEL" scope="$scope" version="$version" releasedBy="$released" receivedBy="$received" verdict=HANDED_OVER \
        releasingEvidence="SYN-EVIDENCE-RELEASE-${scope}" receivingEvidence="SYN-EVIDENCE-RECEIVE-${scope}" rule=SYN-HANDOVER-RULE/v1 \
        judgedAt="$(now_utc)" "$@")" 201:HANDOVER_REGISTERED gateReading.answer participationEnd
}

# 干线 SZX→SIN：委托先于交出（运输委托 → 订舱 → 承运答复 → 交给干线承运商 → 段采用 → 交进新加坡枢纽）。
# 关务包裹要在调用本函数之前先放行（先放行再交干线），否则交出门禁读数不是 ADMITTED。
ch_trunk() {
  local seg="SYN-SEG-SZX-SIN-${STAMP}" commission="SYN-COMMISSION-${STAMP}" booking="SYN-BOOKING-${STAMP}" ctok
  _sched_register "$SCHED2"
  step "干线委托提交（协议 v3）" "$OP" POST /transport-fulfillment-transport-commission-submissions \
    "$(pj commission="$commission" provider=SYN-PARTY-CARRIER-TRUNK-01 agreement=SYN-SUPPLIER-TRUNK-CN-SG-01 agreementVersion=v3 \
        conditions=SYN-CONDITIONS-DEMO role=SYN-ROLE-DEMO responsibility=SYN-RESPONSIBILITY-DEMO members:='["'"$PARCEL"'"]' submittedAt="$(now_utc)")" \
    201:COMMISSION_SUBMITTED commission.agreement
  step "订舱提交" "$OP" POST /transport-fulfillment-booking-request-submissions \
    "$(pj booking="$booking" commission="$commission" schedule="$SCHED2" quantity:=2 unit=KG requestedAt="$(now_utc)")" 201:BOOKING_SUBMITTED booking.booking
  ctok="$(integration_token syn-integration-carrier-trunk-01 syn-carrier-trunk-secret)"
  step "承运答复（合成干线承运商令牌）" "$ctok" POST /transport-fulfillment/booking-answers \
    "$(pj booking="$booking" answer="SYN-ANSWER-${STAMP}" outcome=ACCEPTED quantity:=2 decidedAt="$(now_utc)")" 201:BOOKING_ANSWERED
  sleep 1
  _handover "深圳口岸交给干线承运商" "${CN_EAST[@]}" "SZX-OUT-${STAMP}" "SZX-OUT-v1-${STAMP}" SYN-PARTY-GATE-SZX SYN-PARTY-CARRIER-TRUNK-01 \
    segment="$seg" plannedSegment="${PLAN}#2" segmentServiceAction=LINEHAUL releasingNode=SYN-NODE-SZX-GATE onwardNode=SYN-NODE-SIN-HUB
  GATE_READING="$(jget "$LAST" gateReading.answer)"
  step "段采用干线委托" "$OP" POST /transport-fulfillment-segment-commission-adoptions \
    "$(pj segment="$seg" commission="$commission" provider=SYN-PARTY-CARRIER-TRUNK-01)" 201:COMMISSION_ADOPTED
  sleep 1
  _handover "干线承运商交进新加坡枢纽" "${SG_LM[@]}" "SIN-IN-${STAMP}" "SIN-IN-v1-${STAMP}" SYN-PARTY-CARRIER-TRUNK-01 SYN-NODE-SIN-HUB \
    receivingNode=SYN-NODE-SIN-HUB
}

# 尾程：新加坡枢纽交给自营尾程（FINAL_DELIVERY）→ 派送任务 → 派送前问询 → 派送尝试 → 有效交付。
ch_lastmile() {
  local seg="SYN-SEG-SIN-LM-${STAMP}" task from to at
  sleep 1
  _handover "新加坡枢纽交给自营尾程" "${SG_LM[@]}" "SIN-LM-${STAMP}" "SIN-LM-v1-${STAMP}" SYN-NODE-SIN-HUB SYN-OFFSITE-SG-LM \
    segment="$seg" plannedSegment="${PLAN}#4" segmentServiceAction=FINAL_DELIVERY releasingNode=SYN-NODE-SIN-HUB
  # 派送任务随交接信封由 dispatch 下一拍形成；这里的派送重问口（运营决定）已形成就回同一项任务，没形成就当场形成。
  sleep 5
  step "派送任务（派送重问口）" "$OP" POST /transport-fulfillment-delivery-dispatch-triggers \
    "$(pj segment="$seg" object="$PARCEL" occurredAt="$(now_utc)")" "201:DISPATCH_TASK_FORMED,200:DISPATCH_TASK_ALREADY_FORMED" task windowFrom windowTo
  task="$(jget "$LAST" task)"; from="$(jget "$LAST" windowFrom)"; to="$(jget "$LAST" windowTo)"
  at="$(now_utc)"
  device_headers "${SG_LM[@]}"
  step "派送前问询（交付门禁）" "$OP" POST /transport-fulfillment/delivery-gate-previews \
    "$(pj object="$PARCEL" deliversAt="$at")" 200:DELIVERY_GATE_PREVIEWED gateReading.answer
  device_headers "${SG_LM[@]}"
  step "派送尝试（妥投）" "$OP" POST /transport-fulfillment/delivery-attempts \
    "$(pj task="$task" attempt="SYN-ATTEMPT-${STAMP}" executedBy=SYN-COURIER-DEMO-01 place=SYN-PLACE-DEMO-CONSIGNEE plannedFrom="$from" plannedTo="$to" \
        arrivedAt="$at" evidence="SYN-EVIDENCE-ARRIVAL-${STAMP}" objects:='[{"object":"'"$PARCEL"'","outcome":"DELIVERED","occurredAt":"'"$at"'"}]')" \
    201:ATTEMPT_RECORDED attempt
  device_headers "${SG_LM[@]}"
  step "有效交付" "$OP" POST /transport-fulfillment/deliveries \
    "$(pj attempt="SYN-ATTEMPT-${STAMP}" object="$PARCEL" method=HANDED_TO_RECIPIENT recipient=SYN-RECIPIENT-DEMO-01 proof="SYN-POD-${STAMP}")" \
    201:DELIVERY_REGISTERED gateReading.answer
}

# 终局与客户视图：等 PS 终局，再用首方客户令牌读回客户轨迹视图。
ch_final() {
  local ctok
  ctok="$(customer_token)"
  _finalized() {
    curl -s -H "Authorization: Bearer $OP" "$API_BASE/tracking-projections" > "$LAST"
    grep -q "EFFECTIVE_DELIVERY" "$LAST" && grep -q "$PARCEL" "$LAST"
  }
  poll "PS 终局（追踪投影含 EFFECTIVE_DELIVERY）" 90 _finalized
  printf '%-34s → 追踪投影含 EFFECTIVE_DELIVERY\n' "PS 终局"
  _cost() {
    curl -s -H "Authorization: Bearer $OP" "$API_BASE/settlement-charges?registry=supplier-expected-cost" > "$LAST"
    COST="$(python3 - "$LAST" "$COSTS_BEFORE" <<'PY'
import json, sys
before = set(sys.argv[2].split(","))
for r in json.load(open(sys.argv[1])).get("costs", []):
    if r["version"] not in before:
        print(r["version"]); break
PY
)"
    [[ -n "$COST" ]]
  }
  poll "BUY 供应商预期成本形成" 60 _cost
  COST_FEE="$(jsel "$LAST" costs version "$COST" feeItem)"; COST_AMOUNT="$(jsel "$LAST" costs version "$COST" settlementAmount)"
  printf '%-34s → cost=%s feeItem=%s %s CNY\n' "BUY 供应商预期成本" "$COST" "$COST_FEE" "$COST_AMOUNT"
  # 客户视图在终局之后由 dispatch 再发布一版（终局显示从待确认变已显示），等到那一版。
  _viewfinal() { curl -s -H "Authorization: Bearer $ctok" "$API_BASE/v1/customer-tracking-view?parcel=${PARCEL}" > "$LAST"; [[ "$(jget "$LAST" view.final.state)" == SHOWN ]]; }
  poll "客户视图终局显示" 60 _viewfinal
  step "客户轨迹视图（首方客户令牌）" "$ctok" GET "/v1/customer-tracking-view?parcel=${PARCEL}" - 200:CURRENT_VIEW view.version view.milestones.state view.eta.state view.final.state
}
