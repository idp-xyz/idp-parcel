#!/usr/bin/env bash
# 面单渠道一件（票 commercial-four-module-requirements/05）：服务产品 SYN-PROD-CN-SG-LABEL，形态 LABEL_CHANNEL_SERVICE。
# 走到费用确认和审核应付，但不进集运、也不上未取消且已接受的订舱——那种包裹按现行规则答成本不全。
# 干线只提交委托并在交出后采用，不订舱、不收承运答复。主张账期用 SYN-PERIOD-DEMO- 前缀，不写成费用归属日。
# 末尾读 GET /settlement-operating-results?registry=operating-result：这件要有 ESTIMATED 与 CONFIRMED 两行，
# 账期都是 DIRECT，组成项都在；没有就失败退出。
#
# 不写 MAIN_*，不碰 30-settlement.sh 认的那一件主链。库要已经灌过步骤 117，否则这一件产品不在册，接受答无适用依据。
# 用法：70-label-channel.sh    每跑一次是一件新包裹（约占 9600 余额）。证据只记 S。
set -euo pipefail
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"
load_env
export IDP_PARCEL_API_BASE
export IDP_PARCEL_DEMO_SERVICE_PRODUCT=SYN-PROD-CN-SG-LABEL
source "$DEMO_DIR/chain.sh"

# 干线照主链走到段采用和交进新加坡枢纽，中间那两步订舱与承运答复不走。
# 共享成本认的是未取消且已接受的订舱，不认「提交了委托」。不订舱，这件就不挂在那种班次上。
ch_trunk_direct() {
  local seg="SYN-SEG-SZX-SIN-${STAMP}" commission="SYN-COMMISSION-${STAMP}"
  _sched_register "$SCHED2"
  step "干线委托提交（协议 v3，不订舱）" "$OP" POST /transport-fulfillment-transport-commission-submissions \
    "$(pj commission="$commission" provider=SYN-PARTY-CARRIER-TRUNK-01 agreement=SYN-SUPPLIER-TRUNK-CN-SG-01 agreementVersion=v3 \
        conditions=SYN-CONDITIONS-DEMO role=SYN-ROLE-DEMO responsibility=SYN-RESPONSIBILITY-DEMO members:='["'"$PARCEL"'"]' submittedAt="$(now_utc)")" \
    201:COMMISSION_SUBMITTED commission.agreement
  sleep 1
  _handover "深圳口岸交给干线承运商" "${CN_EAST[@]}" "SZX-OUT-${STAMP}" "SZX-OUT-v1-${STAMP}" SYN-PARTY-GATE-SZX SYN-PARTY-CARRIER-TRUNK-01 \
    segment="$seg" plannedSegment="${PLAN}#2" segmentServiceAction=LINEHAUL releasingNode=SYN-NODE-SZX-GATE onwardNode=SYN-NODE-SIN-HUB
  step "段采用干线委托" "$OP" POST /transport-fulfillment-segment-commission-adoptions \
    "$(pj segment="$seg" commission="$commission" provider=SYN-PARTY-CARRIER-TRUNK-01)" 201:COMMISSION_ADOPTED
  sleep 1
  _handover "干线承运商交进新加坡枢纽" "${SG_LM[@]}" "SIN-IN-${STAMP}" "SIN-IN-v1-${STAMP}" SYN-PARTY-CARRIER-TRUNK-01 SYN-NODE-SIN-HUB \
    receivingNode=SYN-NODE-SIN-HUB
}

STAMP="$(date -u +%H%M%S)"
section "面单渠道·一件包裹（上海当地日 $(today_sha)，产品 ${IDP_PARCEL_DEMO_SERVICE_PRODUCT}）"
ch_submit
ch_intake
ch_selfrun
ch_trunk_direct
ch_lastmile
ch_final

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
