#!/usr/bin/env bash
# 段四补·给「收寄后处置决定」页备一件待决请求（票 demo-2026-10-30 彩排发现）：40 甲的退运是运营直接发起的，跑完后页上
# 「等回应的请求」是空的。待决请求只由客户在取消边界之后提的取消形成——本段让一件新包裹走到收寄，再经首方客户渠道提取消，
# 取消口答 DISPOSITION_PENDING 并把它保全为收寄后处置请求。之后演示者在管理台该页上对它做决定、登记待人工处理项。
# 占一次冻结余额。证据只记 S。
set -euo pipefail
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"
load_env
export IDP_PARCEL_API_BASE
source "$DEMO_DIR/chain.sh"

STAMP="$(date -u +%H%M%S)"
section "异常补·客户在收寄后提取消 → 待决的收寄后处置请求"
ch_submit
ch_intake
access="${PREF/SYN-PCLREF-/SYN-ACCESS-REQUEST-}"
CT="$(customer_token)"
step "客户取消（首方客户渠道，已过取消边界）" "$CT" POST /v1/shipment-requests/parcel-cancellations \
  "$(pj "accessRequestId=parcel-cancellation-pending-${STAMP}" "originalAccessRequestId=$access" "shipmentRequestId=$REQ" \
      "parcelId=$PARCEL" reason=SYN-REASON-DEMO "requestedAt=$(now_utc)")" 200:DISPOSITION_PENDING
step "待决请求清单" "$OP" GET /post-intake-disposition-requests - 200:*
python3 - "$LAST" "$PARCEL" <<'PY'
import json, sys
raw = json.dumps(json.load(open(sys.argv[1])), ensure_ascii=False)
if sys.argv[2] not in raw:
    sys.exit(f"[失败] 待决请求清单里没有包裹 {sys.argv[2]}")
PY
printf '\n去管理台「收寄后处置决定」页：等回应的请求里有包裹 %s，可以当场做决定（例如退运）并登记待人工处理项。\n' "$PARCEL"
