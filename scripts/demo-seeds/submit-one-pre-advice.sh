#!/usr/bin/env bash
# 替合成货主系统交一份送站预报（票 ps-external-mark-relations/09；ADR-0212 决定二；UC-NO-006 步骤 1、2），打印签发的预报标识。
#
# 用法（parcel-api 已带首方族三件与治理自授权起好，见 README「首方客户渠道」一节）：
#   IDP_PARCEL_API_BASE=http://127.0.0.1:19080 scripts/demo-seeds/submit-one-pre-advice.sh [客户包裹件号 ...]
#
# 预报走首方客户渠道：先替合成货主系统取一枚客户端凭据令牌（operator-channel/customer-token.sh；设了 IDP_PARCEL_CUSTOMER_TOKEN
# 就直接用它），再带 Bearer 令牌交。载荷里的 accessRequestId 是接入请求标识，铸成这一份的来源请求键。租户与客户账户只取自客户渠道册
# 上那一行，载荷里不带。
#
# 号码逐字交、逐字比：给了参数就用参数（例如 submit-one-shipment.sh 那一笔的客户包裹参考），没给就用一个按 UTC 时刻起的合成号。
# 节点缺省是演示网络的上海枢纽 SYN-NODE-SHA-HUB（作业设备册里收寄范围登的那一个），预计送达时刻缺省是此刻往后两小时；分别可用
# IDP_PARCEL_PRE_ADVICE_NODE、IDP_PARCEL_PRE_ADVICE_EXPECTED_AT（RFC 3339）改。预报算不算数还看演示租户那一行有效时窗，这一步不判。
#
# 不进 seed.sh：这是运行时命令。同一 UTC 秒内重跑撞同一接入请求标识，同内容答 200 EXISTING_RESULT（脚本照样打印在册那一份的标识）。
# 预报两口是生产写、准入范围在铸造侧判：不设 IDP_PARCEL_GOVERNANCE_SELF_AUTHORITY，或种子里那两条 CUSTOMER_COMMAND 的权威区间与对照
# 没登，答 403 OUTSIDE_ADMISSION_SCOPE。本脚本看到 401 / 403 / 503 就停，不换入口、不写库。证据只记 S。
#
# 标识单独一行打到标准输出，供下一步取用：PRE_ADVICE_ID=$(scripts/demo-seeds/submit-one-pre-advice.sh SYN-PCLREF-…)；说明打到标准错误。
set -euo pipefail

base="${IDP_PARCEL_API_BASE:-}"
if [[ -z "$base" ]]; then
  printf 'submit-one-pre-advice: 需要 IDP_PARCEL_API_BASE，例如 http://127.0.0.1:19080\n' >&2
  exit 1
fi
base="${base%/}"

if ! command -v python3 >/dev/null 2>&1; then
  printf 'submit-one-pre-advice: 需要 python3 组装与解析 JSON\n' >&2
  exit 1
fi

token="${IDP_PARCEL_CUSTOMER_TOKEN:-}"
if [[ -z "$token" ]]; then
  if ! token="$("$(dirname "$0")/operator-channel/customer-token.sh")"; then
    printf 'submit-one-pre-advice: 取不到首方客户端令牌——演示发行方起了没有（operator-channel/compose.yaml）\n' >&2
    exit 1
  fi
fi

stamp="$(date -u +%Y%m%dT%H%M%SZ)"
access_request="SYN-ACCESS-REQUEST-PA-${stamp}"
node="${IDP_PARCEL_PRE_ADVICE_NODE:-SYN-NODE-SHA-HUB}"
expected_at="${IDP_PARCEL_PRE_ADVICE_EXPECTED_AT:-$(date -u -d '+2 hours' +%Y-%m-%dT%H:%M:%SZ)}"
if [[ $# -eq 0 ]]; then
  set -- "SYN-PCL-PA-${stamp}"
fi

body="$(python3 - "$access_request" "$node" "$expected_at" "$@" <<'PY'
import json, sys
print(json.dumps({
    "accessRequestId": sys.argv[1],
    "node": sys.argv[2],
    "expectedArrivalAt": sys.argv[3],
    "customerParcelNumbers": sys.argv[4:],
}, ensure_ascii=False))
PY
)"

post_file="$(mktemp)"
trap 'rm -f "$post_file"' EXIT
post_code="$(curl -sS -o "$post_file" -w '%{http_code}' -X POST \
  -H 'Content-Type: application/json' -H "Authorization: Bearer ${token}" \
  --data "$body" "${base}/node-operations/delivery-pre-advices")"

python3 - "$post_code" "$post_file" "$access_request" <<'PY'
import json, sys
code, path, access_request = sys.argv[1], sys.argv[2], sys.argv[3]
raw = open(path, encoding="utf-8").read()
if code == "401":
    sys.exit("预报口 401：首方客户端令牌没过校验（发行方、受众或有效期）。本脚本不绕过。应答：\n" + raw)
if code == "403":
    sys.exit("预报口 403：首方族未配置（ACCESS_CHANNEL_NOT_CONFIGURED）、客户端没有预报提交授予（CUSTOMER_CLIENT_NOT_GRANTED），"
             "或不在准入范围（OUTSIDE_ADMISSION_SCOPE）。本脚本不绕过。应答：\n" + raw)
if code == "503":
    sys.exit("预报口 503：发行方公钥集、客户渠道册或准入范围读不动（IDENTITY_DEPENDENCY_UNAVAILABLE）。应答：\n" + raw)
if code not in ("201", "200"):
    sys.exit(f"预报口 HTTP {code}，要 201。应答：\n" + raw)
body = json.loads(raw)
outcome = body.get("outcome")
if outcome not in ("DELIVERY_PRE_ADVICE_REGISTERED", "EXISTING_RESULT"):
    sys.exit(f"outcome = {outcome!r}，要 DELIVERY_PRE_ADVICE_REGISTERED。应答：\n" + raw)
pre_advice = body.get("preAdvice") or {}
pre_advice_id = pre_advice.get("preAdviceId") or ""
if not pre_advice_id:
    sys.exit("答复里没有 preAdviceId。应答：\n" + raw)
numbers = "、".join(pre_advice.get("customerParcelNumbers") or [])
print(f"{outcome}：预报 {pre_advice_id}，节点 {pre_advice.get('node')}，预计送达 {pre_advice.get('expectedArrivalAt')}，"
      f"号码 {numbers}，接入请求标识 {access_request}", file=sys.stderr)
print(pre_advice_id)
PY
