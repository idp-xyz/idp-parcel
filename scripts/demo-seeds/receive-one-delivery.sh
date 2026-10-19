#!/usr/bin/env bash
# 替上海枢纽的合成作业设备收寄一件送站实物（票 ps-external-mark-relations/11；ADR-0212 决定七、十；UC-NO-002），打印答复与交付范围
# 核对态。
#
# 用法（parcel-api 已带操作者族三件起好、库按 seed.sh 灌过，见 README「操作者渠道」一节）：
#   IDP_PARCEL_API_BASE=http://127.0.0.1:19080 [IDP_PARCEL_PRE_ADVICE_ID=PREADV-…] scripts/demo-seeds/receive-one-delivery.sh <外部标识>
#
# 走作业事实渠道（ADR-0149）：配置员令牌（operator-channel/operator-token.sh；设了 IDP_PARCEL_OPERATOR_TOKEN 就直接用它），设备与
# 作业范围取种子 data/access/04 登的上海枢纽那一台（可用 IDP_PARCEL_RECEPTION_DEVICE、IDP_PARCEL_RECEPTION_NODE 改）。外部标识逐字交：
# 用 submit-one-shipment.sh 那一笔的客户包裹参考。
#
# 设了 IDP_PARCEL_PRE_ADVICE_ID 就把它作送站预报标识带上（载荷的 deliveryScopeBasis），没设就不带依据。ADR-0212 之后客户包裹件号的唯一
# 识别要有可验证交付范围：带着列了这个号、送到这个节点的预报送站，答 201 INTAKE_FORMED；不带依据再送一件同号，答 200
# PENDING_IDENTIFICATION，核对态 UNVERIFIED、原因 NO_BASIS——缺的是证据，不记冲突。
#
# 不进 seed.sh：这是运行时命令。每跑一次是一件新实物、一个新来源（按 UTC 时刻起）。看到 401 / 403 / 5xx 就停，不换入口、不写库。证据
# 只记 S。
set -euo pipefail

base="${IDP_PARCEL_API_BASE:-}"
if [[ -z "$base" ]]; then
  printf 'receive-one-delivery: 需要 IDP_PARCEL_API_BASE，例如 http://127.0.0.1:19080\n' >&2
  exit 1
fi
base="${base%/}"
if [[ $# -ne 1 ]]; then
  printf '用法：%s <外部标识>\n' "$0" >&2
  exit 1
fi
if ! command -v python3 >/dev/null 2>&1; then
  printf 'receive-one-delivery: 需要 python3 组装与解析 JSON\n' >&2
  exit 1
fi

token="${IDP_PARCEL_OPERATOR_TOKEN:-}"
if [[ -z "$token" ]]; then
  if ! token="$("$(dirname "$0")/operator-channel/operator-token.sh" syn-operator-config-01@syn-tenant-01.example.invalid)"; then
    printf 'receive-one-delivery: 取不到操作者令牌——演示发行方起了没有（operator-channel/compose.yaml）\n' >&2
    exit 1
  fi
fi

device="${IDP_PARCEL_RECEPTION_DEVICE:-SYN-DEVICE-SHA-HUB-01}"
node="${IDP_PARCEL_RECEPTION_NODE:-SYN-NODE-SHA-HUB}"
stamp="$(date -u +%Y%m%dT%H%M%S%NZ)"

body="$(python3 - "$device" "$stamp" "$1" "${IDP_PARCEL_PRE_ADVICE_ID:-}" "$(date -u +%Y-%m-%dT%H:%M:%SZ)" <<'PY'
import json, sys
device, stamp, mark, pre_advice, occurred_at = sys.argv[1:]
document = {
    "sourceId": f"{device}/{stamp}",
    "deliveredBy": "SYN-DRIVER-01",
    "handlingUnit": f"SYN-HU-{stamp}",
    "mark": mark,
    "claim": "RECEIVED",
    "evidenceRef": f"SYN-EVIDENCE/receipt-{stamp}",
    "occurredAt": occurred_at,
}
if pre_advice:
    document["deliveryScopeBasis"] = {"kind": "DELIVERY_PRE_ADVICE", "id": pre_advice}
print(json.dumps(document, ensure_ascii=False))
PY
)"

answer_file="$(mktemp)"
trap 'rm -f "$answer_file"' EXIT
status="$(curl -sS -o "$answer_file" -w '%{http_code}' -X POST \
  -H 'Content-Type: application/json' -H "Authorization: Bearer ${token}" \
  -H "Parcel-Device-Id: ${device}" -H 'Parcel-Work-Scope-Kind: NODE' -H "Parcel-Work-Scope-Reference: ${node}" \
  --data "$body" "${base}/node-operations/receptions")"

python3 - "$status" "$answer_file" <<'PY'
import json, sys
status, path = sys.argv[1], sys.argv[2]
raw = open(path, encoding="utf-8").read()
if status not in ("200", "201"):
    sys.exit(f"收寄口 HTTP {status}，要 200 或 201。本脚本不绕过。应答：\n{raw}")
body = json.loads(raw)
scope = "/".join(filter(None, [body.get("deliveryScopeCheck"), body.get("deliveryScopeUnverifiedReason"),
                               body.get("deliveryScopeInapplicability")])) or "未记"
print(f"{status} {body.get('outcome')}：实物 {body.get('unit')}，关联 {body.get('association') or '无'}，"
      f"候选 {'、'.join(body.get('candidates') or []) or '无'}，交付范围核对态 {scope}")
PY
