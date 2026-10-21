#!/usr/bin/env bash
# 关务三口走一遍（票 product-strategy-boundary/18，ADR-0227）：对已经在跑、设了操作者族三件参数的 parcel-api，拿配置员的 Dex
# 令牌，为一件已接受的包裹依次建出口案件、确认演示单元的提交授权、发起申报提交，逐步断言答复，末行打出提交版本号与首次尝试的
# 结果。首次尝试恒为 REGISTERED_PENDING_SEND：本产品还没有把申报交给海关或报关服务商的连接器（票 10 第 1 项），提交执行口不发送
# 任何东西，也不报发送时间。
#
# 用法：
#   IDP_PARCEL_API_BASE=http://127.0.0.1:19080 scripts/demo-seeds/declare-one-parcel.sh <包裹号>
# 包裹号取 submit-one-shipment.sh 末行打出的那一个，且那一笔委托要已经成了`已接受`（dispatch 跑过一拍）：建案口只认当前已接受
# 委托里的包裹，没接受的答 PARCEL_NOT_ACCEPTED。
#
# 监管范围、单元、资料与角色快照引用、发送目标都是合成串（证据只记 S）：范围与 250-customs 登的那一条建案要求规则同键，单元的
# 就绪由 460-customs-declaration-faces 登。单元只有一个：同一个库上换一件包裹再跑，提交执行答 SOURCE_CONFLICT（已固定的提交
# 版本不可覆盖）；同一件包裹重跑，三口各答已有结果、不多落一行。看到 401 / 403 就停，不换入口、不写库。
set -euo pipefail

if [[ $# -ne 1 ]]; then
  printf '用法：%s <包裹号>\n' "$0" >&2
  exit 1
fi
parcel="$1"

base="${IDP_PARCEL_API_BASE:-}"
if [[ -z "$base" ]]; then
  printf 'declare-one-parcel: 需要 IDP_PARCEL_API_BASE，例如 http://127.0.0.1:19080\n' >&2
  exit 1
fi
base="${base%/}"

if ! command -v python3 >/dev/null 2>&1; then
  printf 'declare-one-parcel: 需要 python3 解析 JSON\n' >&2
  exit 1
fi

here="$(cd "$(dirname "$0")" && pwd)"
if ! token="$("$here/operator-channel/operator-token.sh" syn-operator-config-01@syn-tenant-01.example.invalid)"; then
  printf 'declare-one-parcel: 取不到配置员令牌——演示发行方起了没有（operator-channel/compose.yaml）\n' >&2
  exit 1
fi

unit="SYN-UNIT-CN-EXPORT-DEMO-01"
procedure="SYN-PROC-CN-EXPORT"
answer_file="$(mktemp)"
trap 'rm -f "$answer_file"' EXIT

# step <口> <载荷> <可接受的「状态码:outcome」，逗号分隔> <要从答复里取出的字段>：断言答复后打印那个字段的值。
step() {
  local endpoint=$1 body=$2 accepted=$3 field=$4 status
  status="$(curl -sS -o "$answer_file" -w '%{http_code}' -X POST \
    -H 'Content-Type: application/json' -H "Authorization: Bearer ${token}" \
    --data "$body" "${base}${endpoint}")"
  python3 - "$endpoint" "$status" "$accepted" "$field" "$answer_file" <<'PY'
import json, sys
endpoint, status, accepted, field, path = sys.argv[1:]
raw = open(path, encoding="utf-8").read()
if status in ("401", "403"):
    sys.exit(f"{endpoint} {status}：令牌、授予或准入范围没过。本脚本不绕过。应答：\n" + raw)
body = json.loads(raw)
got = f"{status}:{body.get('outcome')}"
if got not in accepted.split(","):
    sys.exit(f"{endpoint} 答 {got}，要 {accepted}。应答：\n" + raw)
print(body.get(field, ""))
PY
}

case_body="$(python3 - "$parcel" <<'PY'
import json, sys
print(json.dumps({
    "jurisdiction": "SYN-JUR-CN",
    "direction": "EXPORT",
    "procedure": "SYN-PROC-CN-EXPORT",
    "obligation": "SYN-OBL-CN-EXPORT",
    "parcels": [sys.argv[1]],
}))
PY
)"
case_id="$(step /customs-cases "$case_body" "201:ESTABLISHED,200:EXISTING" caseId)"

step /customs-submission-authorizations "{\"unitId\":\"${unit}\"}" \
  "201:AUTHORIZATION_CONFIRMED,200:AUTHORIZATION_EXISTING" authority >/dev/null

submission_body="$(python3 - "$unit" "$case_id" "$procedure" "$parcel" <<'PY'
import json, sys
unit, case_id, procedure, parcel = sys.argv[1:]
print(json.dumps({
    "unitId": unit,
    "caseId": case_id,
    "procedure": procedure,
    "members": [parcel],
    "dossier": "SYN-DOSSIER-DEMO-01",
    "roles": "SYN-ROLES-DEMO-01",
    "target": "SYN-CHANNEL-CN-EXPORT",
}))
PY
)"
version_id="$(step /customs-declaration-submissions "$submission_body" "201:DECLARATION_SUBMITTED,200:EXISTING_VERSION" versionId)"
attempt="$(python3 -c 'import json,sys; print(json.load(open(sys.argv[1])).get("attemptResult", ""))' "$answer_file")"

printf '案件 %s；提交版本 %s；首次尝试 %s\n' "$case_id" "$version_id" "$attempt"
