#!/usr/bin/env bash
# 面单渠道一件（票 commercial-four-module-requirements/05）：服务产品 SYN-PROD-CN-SG-LABEL，形态 LABEL_CHANNEL_SERVICE。
# 不等初始路由，也不登班次、不订舱、不集运。网络初始路由对这一形态答不适用，编一份计划也形不成。
#
# 演示里这两笔金额各自只认一条事实：
#   客户费用：这一户采用的计费时点是有效网络收寄已采用。收寄被采用后发起 SELL，与有没有路由无关。
#   供应商预期成本：BUY 触发册只挂实际履约。一段已采用供应商委托，参与被有效交付结束时形成发生项，再形成预期成本。
# 费用确认条件是有效交付已终局。所以这里在收寄之后只立一段末端交接、采用干线协议、做到有效交付，
# 不走自营段、干线和班次。主张账期用 SYN-PERIOD-DEMO- 前缀，不写成费用归属日。
# 末尾读经营结果：这件要有 ESTIMATED 与 CONFIRMED 两行，账期都是 DIRECT，组成项都在；没有就失败退出。
#
# 不写 MAIN_*，不碰 10 / 30。库要已经灌过步骤 117。每跑一次一件新包裹（约占 9600 余额）。证据只记 S。
set -euo pipefail
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"
load_env
export IDP_PARCEL_API_BASE
export IDP_PARCEL_DEMO_SERVICE_PRODUCT=SYN-PROD-CN-SG-LABEL
source "$DEMO_DIR/chain.sh"

# 收寄之后立一段末端交接并采用干线委托，再用有效交付结束这段参与。不挂计划段，不订舱。
label_direct_facts() {
  local seg="SYN-SEG-LABEL-${STAMP}" commission="SYN-COMMISSION-LABEL-${STAMP}" task from to at
  step "干线委托提交（协议 v3，不订舱）" "$OP" POST /transport-fulfillment-transport-commission-submissions \
    "$(pj commission="$commission" provider=SYN-PARTY-CARRIER-TRUNK-01 agreement=SYN-SUPPLIER-TRUNK-CN-SG-01 agreementVersion=v3 \
        conditions=SYN-CONDITIONS-DEMO role=SYN-ROLE-DEMO responsibility=SYN-RESPONSIBILITY-DEMO members:='["'"$PARCEL"'"]' submittedAt="$(now_utc)")" \
    201:COMMISSION_SUBMITTED commission.agreement
  sleep 1
  _handover "收寄节点交给末端（不挂计划段）" "${CN_EAST[@]}" "LM-${STAMP}" "LM-v1-${STAMP}" SYN-NODE-SHA-HUB SYN-OFFSITE-SG-LM \
    segment="$seg" segmentServiceAction=FINAL_DELIVERY releasingNode=SYN-NODE-SHA-HUB
  step "段采用干线委托" "$OP" POST /transport-fulfillment-segment-commission-adoptions \
    "$(pj segment="$seg" commission="$commission" provider=SYN-PARTY-CARRIER-TRUNK-01)" 201:COMMISSION_ADOPTED
  sleep 1
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

STAMP="$(date -u +%H%M%S)"
section "面单渠道·一件包裹（不等初始路由，产品 ${IDP_PARCEL_DEMO_SERVICE_PRODUCT}）"
CH_SUBMIT_STOP_AFTER_ACCEPTANCE=1
ch_submit
ch_intake
label_direct_facts

_cost() {
  curl -sS -H "Authorization: Bearer $OP" "$API_BASE/settlement-charges?registry=supplier-expected-cost" > "$LAST"
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
COST_FEE="$(jsel "$LAST" costs version "$COST" feeItem)"
COST_AMOUNT="$(jsel "$LAST" costs version "$COST" settlementAmount)"
COST_SUPPLIER="$(jsel "$LAST" costs version "$COST" supplier)"
if [[ -z "$COST_SUPPLIER" ]]; then
  printf '[失败] 预期成本 %s 没记供应商\n' "$COST" >&2
  exit 1
fi
printf '%-34s → cost=%s feeItem=%s %s CNY supplier=%s\n' "BUY 供应商预期成本" "$COST" "$COST_FEE" "$COST_AMOUNT" "$COST_SUPPLIER"

stamp="$(date -u +%Y%m%dT%H%M%SZ)"
section "面单渠道·费用确认与审核应付 ${CHARGE}"
_confirmed() {
  curl -sS -H "Authorization: Bearer $OP" "$API_BASE/settlement-charges?registry=customer-charge" > "$LAST"
  python3 - "$LAST" "$CHARGE" <<'PY'
import json, sys
for r in json.load(open(sys.argv[1])).get("charges", []):
    if r["charge"] == sys.argv[2] and r["stage"] == "CONFIRMED":
        sys.exit(0)
sys.exit(1)
PY
}
poll "费用确认为 CONFIRMED" 60 _confirmed
python3 - "$LAST" "$CHARGE" <<'PY'
import json, sys
for r in json.load(open(sys.argv[1]))["charges"]:
    if r["charge"] == sys.argv[2]:
        print(f"{'费用确认读数':<30s} → {r['charge']} stage=CONFIRMED {r['settlementAmount']} {r['settlementCurrency']} basis={r['confirmationBasis']}")
        break
PY
amount="$(jsel "$LAST" charges charge "$CHARGE" settlementAmount)"
attrdate="$(jsel "$LAST" charges charge "$CHARGE" attributionDate)"
if [[ -z "$attrdate" ]]; then
  printf '[失败] 费用 %s 读不到归属日 attributionDate\n' "$CHARGE" >&2
  exit 1
fi
period="SYN-PERIOD-DEMO-${stamp}"
if [[ "$period" == "$attrdate" || "$period" != SYN-PERIOD-DEMO-* ]]; then
  printf '[失败] 主张账期 %s 要带 SYN-PERIOD-DEMO- 前缀，且不能写成归属日 %s\n' "$period" "$attrdate" >&2
  exit 1
fi
printf '%-34s → %s（归属日 %s）\n' "主张账期" "$period" "$attrdate"

step "费用确认补拍（运营确认口）" "$OP" POST /settlement-charge-confirmations "$(pj charge="$CHARGE")" "200:ALREADY_CONFIRMED" charge
step "发布对账单" "$OP" POST /settlement-statement-publications \
  "$(pj account=SYN-SETTLEMENT-ACCOUNT-01 period="$period" version=v1 currency=CNY \
      charges:='["'"$CHARGE"'"]' lastAttributionDate="$attrdate" declaredTotal="$amount" cutOffAt="$(now_utc)")" \
  201:STATEMENT_PUBLISHED statement total

bill="SYN-BILL-DEMO-${stamp}"
step "供应商账单接收（逐行 MATCHED）" "$OP" POST /settlement-supplier-bill-receptions \
  "$(pj claim="$bill" version=v1 supplier="$COST_SUPPLIER" legalEntity=SYN-LE-01 period="$period" currency=CNY \
      lines:='[{"line":"L1","feeItem":"'"$COST_FEE"'","claimed":"'"$COST_AMOUNT"'"}]' receivedAt="$(now_utc)" \
      directives:='[{"line":"L1","classification":"MATCHED","expectedVersion":"'"$COST"'"}]')" \
  201:BILL_RECEIVED auditAuthority
step "审核成应付" "$OP" POST /settlement-supplier-bill-audits \
  "$(pj claim="$bill" version=v1 line=L1 payable="SYN-PAYABLE-DEMO-${stamp}")" 201:PAYABLE_FORMED documentNumber payable amount currency

_operating() {
  local query
  query="$(python3 -c 'import sys, urllib.parse; print(urllib.parse.quote(sys.argv[1], safe=""))' "$PARCEL")"
  curl -sS -H "Authorization: Bearer $OP" \
    "$API_BASE/settlement-operating-results?registry=operating-result&q=${query}" > "$LAST" || return 1
  python3 - "$LAST" "$PARCEL" <<'PY' || return 1
import json, sys
body = json.load(open(sys.argv[1], encoding="utf-8"))
if body.get("outcome") != "OPERATING_RESULTS_LISTED":
    sys.exit(1)
parcel = sys.argv[2]
want = {
    "ESTIMATED": {"CUSTOMER_ESTIMATE", "SUPPLIER_EXPECTED_COST"},
    "CONFIRMED": {"CUSTOMER_OPERATING_RECEIVABLE", "AUDITED_PAYABLE"},
}
rows = [r for r in body.get("results") or [] if r.get("scope") == parcel]
by = {}
for row in rows:
    by.setdefault(row.get("basis"), []).append(row)
if set(by) != set(want) or any(len(found) != 1 for found in by.values()):
    sys.exit(1)
for basis, roles in want.items():
    row = by[basis][0]
    got = [c.get("role") for c in row.get("components") or []]
    if row.get("period") != "DIRECT" or set(got) != roles or len(got) != len(roles):
        sys.exit(1)
    if any(not c.get("source") or c.get("amount") in (None, "") for c in row["components"]):
        sys.exit(1)
if any(d.get("scope") == parcel for d in body.get("dispositions") or []):
    sys.exit(1)
PY
}
operating_ok=0
end=$((SECONDS + 30))
while ((SECONDS < end)); do
  if _operating; then operating_ok=1; break; fi
  sleep 2
done
if [[ "$operating_ok" != 1 ]]; then
  printf '\n[失败] 经营结果册里没有这件包裹的 ESTIMATED 与 CONFIRMED（账期都是 DIRECT，组成项都在）。原始响应：\n' >&2
  cat "$LAST" >&2
  printf '\n' >&2
  exit 1
fi
python3 - "$LAST" "$PARCEL" <<'PY'
import json, sys
body = json.load(open(sys.argv[1], encoding="utf-8"))
parcel = sys.argv[2]
for row in body["results"]:
    if row.get("scope") != parcel:
        continue
    parts = " ".join(f"{c['role']}={c['source']}/{c['amount']}" for c in row["components"])
    print(f"{'经营结果 ' + row['basis']:<30s} → period={row['period']} margin={row['margin']} {row['currency']} {parts}")
PY

state_set LABEL_REQ "$REQ"
state_set LABEL_PARCEL "$PARCEL"
state_set LABEL_CHARGE "$CHARGE"
state_set LABEL_COST "$COST"
state_set LABEL_PERIOD "$period"
printf '\n面单渠道完成：包裹 %s，费用 %s 已确认，供应商账单 %s 已审核成应付，经营结果 ESTIMATED 与 CONFIRMED 账期都是 DIRECT。证据只记 S。\n' \
  "$PARCEL" "$CHARGE" "$bill"
