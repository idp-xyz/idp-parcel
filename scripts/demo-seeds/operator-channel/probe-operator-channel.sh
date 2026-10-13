#!/usr/bin/env bash
# 操作者渠道走通取证（票 operator-channel/17）：对已经在跑、设了操作者族三件参数的 parcel-api，拿配置员与
# 查阅员各自的 Dex 令牌、再加一次不带令牌，打同一个登记写口，逐格断言：
#   配置员    → 200 REFUSED / VERSION_NOT_OVERWRITABLE（业务结果，见下）
#   查阅员    → 403 OPERATOR_NOT_GRANTED（令牌有效、主体在册，但没有登记册配置写的授予）
#   不带令牌  → 401 OPERATOR_CREDENTIAL_REJECTED
# 另打一个运营决定口（票 operator-channel/19）：配置员持终止参与的授予、演示租户登了对照与区间，越过准入范围到编排，
# 答 200 SEGMENT_NOT_FOUND（段号是不存在的那一个，不落行）；查阅员没有这一种决定的授予，答 403 OPERATOR_NOT_GRANTED。
#
# 写口取 POST /visibility-catalogue-notification-policy-registrations（端点表里挂 operatorRegistries.visibility）。
# 批文是种子里那条通知策略去掉 tenantId：在线口的租户取认证结果，批文带 tenantId 即拒。种子已在 SYN-TENANT-01
# 下登过同一条，所以配置员这一枪的业务结果是「版本不可覆盖」——这一格只有令牌解析出的租户下已经有这条策略才
# 答得出。脚本因此不往库里多落一行，可以对演示库反复跑。证据层级只记 S。
#
# 前提：Dex 按 compose.yaml 起好；库按 seed.sh 灌过；parcel-api 按 README「操作者渠道」那一节起。
#
# 用法：
#   IDP_PARCEL_API_BASE=http://127.0.0.1:19080 scripts/demo-seeds/operator-channel/probe-operator-channel.sh
set -euo pipefail

base="${IDP_PARCEL_API_BASE:-}"
if [[ -z "$base" ]]; then
  printf 'probe-operator-channel: 需要 IDP_PARCEL_API_BASE，例如 http://127.0.0.1:19080\n' >&2
  exit 1
fi
base="${base%/}"

if ! command -v python3 >/dev/null 2>&1; then
  printf 'probe-operator-channel: 需要 python3 解析 JSON\n' >&2
  exit 1
fi

here="$(cd "$(dirname "$0")" && pwd)"
config_token="$("$here/operator-token.sh" syn-operator-config-01@syn-tenant-01.example.invalid)"
viewer_token="$("$here/operator-token.sh" syn-operator-viewer-01@syn-tenant-01.example.invalid)"

body="$(python3 - "$here/../data/visibility/03-notification-policy-disclose-v1.json" <<'PY'
import json, sys
batch = json.load(open(sys.argv[1], encoding="utf-8"))
del batch["tenantId"]
print(json.dumps(batch, ensure_ascii=False))
PY
)"

endpoint="${base}/visibility-catalogue-notification-policy-registrations"
answer_file="$(mktemp)"
trap 'rm -f "$answer_file"' EXIT

# probe <标签> <令牌，空串即不带> <期望状态码> <期望答复：outcome/refusalReason 或 error.code>
probe() {
  local label=$1 token=$2 want_status=$3 want=$4 status
  local -a auth=()
  if [[ -n "$token" ]]; then
    auth=(-H "Authorization: Bearer ${token}")
  fi
  status="$(curl -sS -o "$answer_file" -w '%{http_code}' -X POST \
    -H 'Content-Type: application/json' "${auth[@]}" --data "$body" "$endpoint")"
  python3 - "$label" "$status" "$want_status" "$want" "$answer_file" <<'PY'
import json, sys
label, status, want_status, want, path = sys.argv[1:]
raw = open(path, encoding="utf-8").read()
try:
    body = json.loads(raw)
except ValueError:
    sys.exit(f"{label}：HTTP {status}，应答不是 JSON：\n{raw}")
got = "/".join(filter(None, [body.get("outcome"), body.get("refusalReason"), (body.get("error") or {}).get("code")]))
if status != want_status or got != want:
    hints = {
        "ACCESS_CHANNEL_NOT_CONFIGURED": "parcel-api 起进程没带操作者族三件参数",
        "IDENTITY_DEPENDENCY_UNAVAILABLE": "parcel-api 取不到发行方公钥集，或读不动操作者册",
        "OPERATOR_NOT_GRANTED": "令牌过了，但册上没有这个主体的那一格授予：库按 seed.sh 灌过吗？",
        "OPERATOR_CREDENTIAL_REJECTED": "令牌没过：发行方地址或受众与 parcel-api 的三件参数对不上，或 Dex 刚重启、钥还没重取",
        "OUTSIDE_ADMISSION_SCOPE": "授予过了，准入范围没过：parcel-api 没带 IDP_PARCEL_GOVERNANCE_SELF_AUTHORITY，或库里没灌准入范围对照与区间",
    }
    hint = hints.get((body.get("error") or {}).get("code"), "")
    sys.exit(f"{label}：答 {status} {got}，要 {want_status} {want}。{hint}\n应答：{raw}")
print(f"{label}：{status} {got}")
PY
}

probe '配置员' "$config_token" 200 'REFUSED/VERSION_NOT_OVERWRITABLE'
probe '查阅员' "$viewer_token" 403 'OPERATOR_NOT_GRANTED'
probe '不带令牌' '' 401 'OPERATOR_CREDENTIAL_REJECTED'

endpoint="${base}/transport-fulfillment-participation-terminations"
body='{"segment": "SYN-SEGMENT-PROBE-NONE", "object": "SYN-PARCEL-PROBE-NONE", "basis": "SYN-BASIS-PROBE", "endedAt": "2026-01-02T00:00:00Z"}'
probe '配置员 · 终止参与' "$config_token" 200 'SEGMENT_NOT_FOUND'
probe '查阅员 · 终止参与' "$viewer_token" 403 'OPERATOR_NOT_GRANTED'
