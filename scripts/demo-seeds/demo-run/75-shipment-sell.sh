#!/usr/bin/env bash
# 段：逐委托销售（票 07）。新开的 SYN-ACCOUNT-23 交一份两件委托。第一件收寄之后不形成客户费用；
# 两件都完成有效网络收寄之后，形成一份委托级销售请求和一笔客户费用，不另起逐包裹的销售请求。
#
# 要 up.sh 起好的环境。发行方要认得 syn-customer-client-23：up.sh 探不到时会另起私有的那一只。
# 这一户的预收款首次跑时补一笔 1000.00（后缀 SHIP23，同一只库上重跑不重复补）。证据只记 S。
set -euo pipefail
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"
load_env
export IDP_PARCEL_API_BASE
source "$DEMO_DIR/chain.sh"

"$DEMO_DIR/topup-funds.sh" 100000 SHIP23 SYN-SETTLEMENT-ACCOUNT-23 SYN-PARTY-SHIPPER-23

ss_read() {
  docker exec "$IDP_PARCEL_DEMO_PG_CONTAINER" psql -U parcel -d "$IDP_PARCEL_DEMO_DB" -v ON_ERROR_STOP=1 -Atc "$1"
}

section "提交两件委托"
export DEMO_CUSTOMER_CLIENT="syn-customer-client-23" DEMO_CUSTOMER_SECRET="syn-customer-client-23-secret"
IDP_PARCEL_CUSTOMER_TOKEN="$(customer_token)"
export IDP_PARCEL_CUSTOMER_TOKEN
OP="$(operator_token)"
stamp="$(date -u +%Y%m%dT%H%M%SZ)"
ref="SYN-CUSTREF-SHIP-${stamp}"
parcel_a="SYN-PCLREF-SHIP-A-${stamp}"
parcel_b="SYN-PCLREF-SHIP-B-${stamp}"
access_request="SYN-ACCESS-REQUEST-SHIP-${stamp}"
body="$(python3 - "$ref" "$parcel_a" "$parcel_b" "$access_request" <<'PY'
import json, sys
ref, parcel_a, parcel_b, access_request = sys.argv[1:]
def piece(number, weight):
    return {
        "customerParcelReference": number,
        "declaredWeightValue": weight,
        "declaredWeightUnit": "KG",
        "declaredLength": "20",
        "declaredWidth": "15",
        "declaredHeight": "10",
        "declaredDimensionsUnit": "CM",
    }
print(json.dumps({
    "accessRequestId": access_request,
    "customerShipmentReference": ref,
    "requestedServiceProduct": "SYN-PROD-CN-SG-EXPRESS",
    "senderRelation": "SYN-SENDER-01",
    "senderAddress": "合成寄件地址",
    "senderPostalCode": "200000",
    "senderCountryCode": "CN",
    "recipientRelation": "SYN-RECIPIENT-01",
    "recipientAddress": "合成收件地址",
    "recipientPostalCode": "018000",
    "recipientCountryCode": "SG",
    "destinationServiceScope": "SYN-DEST/SG",
    "parcels": [piece(parcel_a, "1.50"), piece(parcel_b, "2.00")],
}, ensure_ascii=False))
PY
)"
step "提交两件委托" "$IDP_PARCEL_CUSTOMER_TOKEN" POST /v1/shipment-requests "$body" "201:SUBMITTED" \
  shipmentRequestId parcels.0.parcelId parcels.0.customerParcelReference parcels.1.parcelId parcels.1.customerParcelReference
REQ="$(jget "$LAST" shipmentRequestId)"
PARCEL_A="$(jget "$LAST" parcels.0.parcelId)"
PARCEL_B="$(jget "$LAST" parcels.1.parcelId)"
REF_A="$(jget "$LAST" parcels.0.customerParcelReference)"
REF_B="$(jget "$LAST" parcels.1.customerParcelReference)"
[[ -n "$REQ" && -n "$PARCEL_A" && -n "$PARCEL_B" && "$REF_A" == "$parcel_a" && "$REF_B" == "$parcel_b" ]] \
  || { printf '[失败] 两件没有都带回来\n' >&2; cat "$LAST" >&2; exit 1; }
_accepted() { step "等 dispatch 接受" "$OP" GET "/shipment-request-views?shipmentRequestId=${REQ}" - 200:REQUEST_VIEW request.state >/dev/null 2>&1 && [[ "$(jget "$LAST" request.state)" == ACCEPTED ]]; }
poll "委托成已接受" 90 _accepted
printf '%-34s → request=%s parcelA=%s parcelB=%s\n' "已接受" "$REQ" "$PARCEL_A" "$PARCEL_B"

section "只收寄第一件"
pre="$("$SEEDS_DIR/submit-one-pre-advice.sh" "$REF_A" 2>/dev/null)" || { printf '[失败] 第一件预报\n' >&2; exit 1; }
out="$(IDP_PARCEL_PRE_ADVICE_ID="$pre" "$SEEDS_DIR/receive-one-delivery.sh" "$REF_A")" || { printf '[失败] 第一件收寄：%s\n' "$out" >&2; exit 1; }
[[ "$out" == 201\ INTAKE_FORMED* ]] || { printf '[失败] 第一件收寄不是 201 INTAKE_FORMED：%s\n' "$out" >&2; exit 1; }
printf '%-34s → %s\n' "第一件收寄" "$out"
# 给 dispatch 几拍。少一件时委托级请求不该出现。
sleep 12
early="$(ss_read "SELECT count(*) FROM settlement_accounting.shipment_sell_evaluation_request WHERE tenant_id = 'SYN-TENANT-01' AND shipment_id = '${REQ}'")"
if [[ "$early" != 0 ]]; then
  printf '[失败] 少一件却已有 %s 份委托级销售请求\n' "$early" >&2
  exit 1
fi
printf '%-34s → 委托级请求 0\n' "少一件不计费"

section "收寄第二件"
pre_b="$("$SEEDS_DIR/submit-one-pre-advice.sh" "$REF_B" 2>/dev/null)" || { printf '[失败] 第二件预报\n' >&2; exit 1; }
out_b="$(IDP_PARCEL_PRE_ADVICE_ID="$pre_b" "$SEEDS_DIR/receive-one-delivery.sh" "$REF_B")" || { printf '[失败] 第二件收寄：%s\n' "$out_b" >&2; exit 1; }
[[ "$out_b" == 201\ INTAKE_FORMED* ]] || { printf '[失败] 第二件收寄不是 201 INTAKE_FORMED：%s\n' "$out_b" >&2; exit 1; }
printf '%-34s → %s\n' "第二件收寄" "$out_b"
_formed() {
  ss_read "SELECT state FROM settlement_accounting.shipment_sell_evaluation_request WHERE tenant_id = 'SYN-TENANT-01' AND shipment_id = '${REQ}'" | grep -qx FORMED
}
poll "票级客户费用形成" 90 _formed

section "读回"
ss_read "SELECT account_id, billing_moment, coalesce(pricing_scope_id, '') FROM settlement_accounting.shipment_billing_moment_adoption WHERE tenant_id = 'SYN-TENANT-01' AND account_id = 'SYN-SETTLEMENT-ACCOUNT-23'"
ss_read "SELECT request_id, billing_moment, pricing_scope_id, state, customer_charge_id, array_to_string(member_ids, ',') FROM settlement_accounting.shipment_sell_evaluation_request WHERE tenant_id = 'SYN-TENANT-01' AND shipment_id = '${REQ}'"
charge_id="$(ss_read "SELECT customer_charge_id FROM settlement_accounting.shipment_sell_evaluation_request WHERE tenant_id = 'SYN-TENANT-01' AND shipment_id = '${REQ}'")"
ss_read "SELECT charge_id, stage, settlement_currency, settlement_minor, evaluation_ref FROM settlement_accounting.customer_charge WHERE tenant_id = 'SYN-TENANT-01' AND charge_id = '${charge_id}'"
parcel_requests="$(ss_read "SELECT count(*) FROM settlement_accounting.sell_evaluation_request WHERE tenant_id = 'SYN-TENANT-01' AND parcel_id IN ('${PARCEL_A}', '${PARCEL_B}')")"
shipment_requests="$(ss_read "SELECT count(*) FROM settlement_accounting.shipment_sell_evaluation_request WHERE tenant_id = 'SYN-TENANT-01' AND shipment_id = '${REQ}'")"
charges="$(ss_read "SELECT count(*) FROM settlement_accounting.customer_charge WHERE tenant_id = 'SYN-TENANT-01' AND charge_id = '${charge_id}'")"
if [[ "$parcel_requests" != 0 || "$shipment_requests" != 1 || "$charges" != 1 ]]; then
  printf '[失败] 逐包裹请求 %s、委托级请求 %s、客户费用 %s，应是 0、1、1\n' "$parcel_requests" "$shipment_requests" "$charges" >&2
  exit 1
fi
printf '%-34s → 一笔 %s\n' "票级客户费用" "$charge_id"
