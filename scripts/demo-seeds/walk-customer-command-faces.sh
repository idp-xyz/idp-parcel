#!/usr/bin/env bash
# 替合成货主系统把首方客户命令五口各走一遍（票 first-party-api/12；ADR-0310）：补充、撤回、修订、索赔、取消，外加两条反例。
# 每一步断言状态码与 outcome，打一行摘要到标准错误，出错即退。
#
# 用法（parcel-api 已带首方族三件与治理自授权起好，见 README「首方客户渠道」一节；委托 B 要接受，另需 parcel-dispatch 在跑）：
#   IDP_PARCEL_API_BASE=http://127.0.0.1:19080 scripts/demo-seeds/walk-customer-command-faces.sh
#
# 走两份委托，都经 POST /v1/shipment-requests 现提（载荷照 submit-one-shipment.sh）：
#   委托 A 停在已提交：补充 PSU-1（201 SUPPLEMENT_RECORDED，原样重放 200 SUPPLEMENT_ALREADY_HANDLED），再撤回 PWD-1
#     （201 FORMED，原样重放 200 DECISION_ALREADY_FORMED 带同一份 withdrawalId——撤回重放是「接着办」，已撤回的委托读回原决定）。
#   委托 B 等到已接受：修订 PAM-1、索赔 VCL-1、取消 PCN-1，各一次成立、一次原样重放。
#   反例：不存在的原提交标识撤回、取消各答 404；载荷自报 customerAccount 答 400。他账户那一格（与不存在同字节）只有一台客户端
#     演示不了，由 cmd/parcel-api 的生产装配真库用例钉，本脚本不造第二个账户。
#
# 委托 B 的接受靠 dispatch 跑一拍。本脚本用客户追踪视图判「已接受」（接受之前答 VIEW_NOT_FOUND，之后答 NO_VISIBLE_RESULT 或
# CURRENT_VIEW），最多等 IDP_PARCEL_WALK_ACCEPT_TIMEOUT 秒（缺省 90）；超时就停并说清楚该起什么。dispatch 要七个环境变量，
# 取值见 docs/design/synthetic-demo-journey-script.md「起 dispatch」一节。
#
# 取值全部取自种子：修订取发布批 sourceDataAmendment 里已登的格（SENDER_PLACE × ACCEPTED_NOT_YET_RECEIVED × SUPPLEMENT，
# 授权靠授权册 SYN-AUTH-SOURCE-AMEND-01 与账户解析键行）；取消靠发布批里带取消目录的 SYN-AUTH-CANCEL-01；索赔的合同
# SYN-CONTRACT-01、类型 LOSS 登在 SYN-ACCOUNT-01 名下（data/visibility/04），申请人 SYN-APPLICANT-01 在名单里（05）。原因代码
# 与批次、项是演示的合成值，批次与项按 UTC 时刻合成。
#
# 不进 seed.sh：这是运行时命令。所有接入请求标识按 UTC 秒合成，同一 UTC 秒内重跑撞同一提交键，提交口答 200 EXISTING_RESULT，
# 脚本因要求 201 失败退出、不落第二笔；隔一秒再跑即可（每跑一次落两笔委托、一次撤回、一次修订、一份索赔、一次取消）。
# 本脚本看到 401 / 403 / 503 就停（各步的断言都要求精确状态码），不换入口、不写库。证据只记 S。
set -euo pipefail

base="${IDP_PARCEL_API_BASE:-}"
if [[ -z "$base" ]]; then
  printf 'walk-customer-command-faces: 需要 IDP_PARCEL_API_BASE，例如 http://127.0.0.1:19080\n' >&2
  exit 1
fi
base="${base%/}"

if ! command -v python3 >/dev/null 2>&1; then
  printf 'walk-customer-command-faces: 需要 python3 组装与解析 JSON\n' >&2
  exit 1
fi

token="${IDP_PARCEL_CUSTOMER_TOKEN:-}"
if [[ -z "$token" ]]; then
  if ! token="$("$(dirname "$0")/operator-channel/customer-token.sh")"; then
    printf 'walk-customer-command-faces: 取不到首方客户端令牌——演示发行方起了没有（operator-channel/compose.yaml）\n' >&2
    exit 1
  fi
fi
accept_timeout="${IDP_PARCEL_WALK_ACCEPT_TIMEOUT:-90}"

stamp="$(date -u +%Y%m%dT%H%M%SZ)"
resp="$(mktemp)"
trap 'rm -f "$resp"' EXIT

# pj k=v … 组装 JSON 对象：k=v 是字符串值，k:=<json> 是原样 JSON 值。
pj() {
  python3 - "$@" <<'PY'
import json, sys
out = {}
for arg in sys.argv[1:]:
    key = arg.split("=", 1)[0]
    if key.endswith(":"):
        out[key[:-1]] = json.loads(arg.split("=", 1)[1])
    else:
        key, value = arg.split("=", 1)
        out[key] = value
print(json.dumps(out, ensure_ascii=False))
PY
}

# jget <字段> 读上一步答复里的顶层字段（缺则空串）。
jget() {
  python3 - "$resp" "$1" <<'PY'
import json, sys
value = json.load(open(sys.argv[1], encoding="utf-8")).get(sys.argv[2], "")
print(value if isinstance(value, str) else json.dumps(value, ensure_ascii=False))
PY
}

# step <标签> <路径> <载荷> <要的状态码> <要的 outcome，空串表示不看>：POST 并断言，答复留在 $resp。
step() {
  local label="$1" path="$2" body="$3" want_code="$4" want_outcome="$5" code
  code="$(curl -sS -o "$resp" -w '%{http_code}' -X POST \
    -H 'Content-Type: application/json' -H "Authorization: Bearer ${token}" \
    --data "$body" "${base}${path}")"
  python3 - "$label" "$code" "$resp" "$want_code" "$want_outcome" <<'PY'
import json, sys
label, code, path, want_code, want_outcome = sys.argv[1:6]
raw = open(path, encoding="utf-8").read()
if code in ("401", "403", "503") and code != want_code:
    sys.exit(f"{label}：HTTP {code}（首方令牌没过、客户端无该口授予或不在准入范围、依赖读不动）。本脚本不绕过。应答：\n" + raw)
if code != want_code:
    sys.exit(f"{label}：HTTP {code}，要 {want_code}。应答：\n" + raw)
outcome = ""
if want_outcome:
    outcome = json.loads(raw).get("outcome")
    if outcome != want_outcome:
        sys.exit(f"{label}：outcome = {outcome!r}，要 {want_outcome}。应答：\n" + raw)
print(f"{label}：HTTP {code} {outcome or json.loads(raw).get('code', '')}", file=sys.stderr)
PY
}

# submit <后缀>：现提一份委托，设 request_id、version_id、parcel_id 与该份的接入请求标识 access。
submit() {
  local suffix="$1"
  access="SYN-ACCESS-REQUEST-WALK-${stamp}-${suffix}"
  parcel_ref="SYN-PCLREF-WALK-${stamp}-${suffix}"
  local body
  body="$(pj "accessRequestId=$access" "customerShipmentReference=SYN-CUSTREF-WALK-${stamp}-${suffix}" \
    requestedServiceProduct=SYN-PROD-CN-SG-EXPRESS senderRelation=SYN-SENDER-01 senderAddress=合成寄件地址 \
    senderPostalCode=200000 senderCountryCode=CN recipientRelation=SYN-RECIPIENT-01 recipientAddress=合成收件地址 \
    recipientPostalCode=018000 recipientCountryCode=SG destinationServiceScope=SYN-DEST/SG \
    "parcels:=[{\"customerParcelReference\":\"$parcel_ref\",\"declaredWeightValue\":\"1.50\",\"declaredWeightUnit\":\"KG\",\"declaredLength\":\"20\",\"declaredWidth\":\"15\",\"declaredHeight\":\"10\",\"declaredDimensionsUnit\":\"CM\"}]")"
  step "提交委托 ${suffix}" /v1/shipment-requests "$body" 201 SUBMITTED
  request_id="$(jget shipmentRequestId)"
  version_id="$(jget submissionVersionId)"
  parcel_id="$(python3 -c 'import json,sys; print(json.load(open(sys.argv[1]))["parcels"][0]["parcelId"])' "$resp")"
  if [[ -z "$request_id" || -z "$version_id" || -z "$parcel_id" ]]; then
    printf 'walk-customer-command-faces: 提交答复缺 shipmentRequestId、submissionVersionId 或 parcelId：\n%s\n' "$(cat "$resp")" >&2
    exit 1
  fi
}

printf '== 委托 A：停在已提交 ==\n' >&2
submit A
a_access="$access" a_request="$request_id" a_version="$version_id" a_parcel="$parcel_id" a_ref="$parcel_ref"

# 补充 PSU-1：重报整份内容（这里只改收件地址），以回执的 submissionVersionId 为基准；成员集合须与当前版本一致。
supp="$(pj "accessRequestId=supplement-walk-${stamp}" "originalAccessRequestId=$a_access" "shipmentRequestId=$a_request" \
  "basisSubmissionVersionId=$a_version" "customerShipmentReference=SYN-CUSTREF-WALK-${stamp}-A" \
  requestedServiceProduct=SYN-PROD-CN-SG-EXPRESS senderRelation=SYN-SENDER-01 senderAddress=合成寄件地址 \
  senderPostalCode=200000 senderCountryCode=CN recipientRelation=SYN-RECIPIENT-01 recipientAddress=合成收件地址（补充） \
  recipientPostalCode=018000 recipientCountryCode=SG destinationServiceScope=SYN-DEST/SG \
  "parcels:=[{\"parcelId\":\"$a_parcel\",\"customerParcelReference\":\"$a_ref\",\"declaredWeightValue\":\"1.50\",\"declaredWeightUnit\":\"KG\",\"declaredLength\":\"20\",\"declaredWidth\":\"15\",\"declaredHeight\":\"10\",\"declaredDimensionsUnit\":\"CM\"}]")"
step "补充 PSU-1" /v1/shipment-requests/supplements "$supp" 201 SUPPLEMENT_RECORDED
step "补充 PSU-1 原样重放" /v1/shipment-requests/supplements "$supp" 200 SUPPLEMENT_ALREADY_HANDLED

# 撤回 PWD-1：重放是接着办，委托已撤回，读回原决定与同一份 withdrawalId。
wd="$(pj "accessRequestId=withdrawal-walk-${stamp}" "originalAccessRequestId=$a_access" reason=SYN-REASON-WALK)"
step "撤回 PWD-1" /v1/shipment-requests/withdrawals "$wd" 201 FORMED
first_withdrawal="$(jget withdrawalId)"
step "撤回 PWD-1 原样重放" /v1/shipment-requests/withdrawals "$wd" 200 DECISION_ALREADY_FORMED
if [[ -z "$first_withdrawal" || "$(jget withdrawalId)" != "$first_withdrawal" ]]; then
  printf 'walk-customer-command-faces: 撤回重放没有读回同一份 withdrawalId（首次 %q）：\n%s\n' "$first_withdrawal" "$(cat "$resp")" >&2
  exit 1
fi

printf '== 委托 B：等到已接受 ==\n' >&2
submit B
b_access="$access" b_request="$request_id" b_version="$version_id" b_parcel="$parcel_id"

deadline=$((SECONDS + accept_timeout))
while :; do
  view_code="$(curl -sS -o "$resp" -w '%{http_code}' -H "Authorization: Bearer ${token}" \
    --get "${base}/v1/customer-tracking-view" --data-urlencode "parcel=${b_parcel}")"
  if [[ "$view_code" != "200" ]]; then
    printf 'walk-customer-command-faces: 客户追踪视图 HTTP %s，要 200：\n%s\n' "$view_code" "$(cat "$resp")" >&2
    exit 1
  fi
  view_outcome="$(jget outcome)"
  if [[ "$view_outcome" != "VIEW_NOT_FOUND" ]]; then
    printf '委托 B 已接受（追踪视图 outcome %s）\n' "$view_outcome" >&2
    break
  fi
  if (( SECONDS >= deadline )); then
    printf 'walk-customer-command-faces: 等了 %s 秒委托 B 仍未接受（追踪视图仍是 VIEW_NOT_FOUND）。接受靠 cmd/parcel-dispatch 跑一拍：\n' "$accept_timeout" >&2
    printf '  与 parcel-api 同库起它，七个环境变量见 docs/design/synthetic-demo-journey-script.md「起 dispatch」一节；\n' >&2
    printf '  已起的话查 bento.outbox 那一行的 status 与 failure_code。委托 B 已提交：%s\n' "$b_request" >&2
    exit 1
  fi
  sleep 2
done

# 修订 PAM-1：SUPPLEMENT 不带基准（以接受基线为基准），内容字段落在 SENDER_PLACE 范围内。
am="$(pj "accessRequestId=amendment-walk-${stamp}" "originalAccessRequestId=$b_access" "shipmentRequestId=$b_request" \
  dataGroup=SENDER_PLACE intent=SUPPLEMENT reason=SYN-REASON-ADDRESS postalCode=200001 countryCode=CN)"
step "修订 PAM-1" /v1/shipment-requests/source-data-amendments "$am" 201 RECORDED
step "修订 PAM-1 原样重放" /v1/shipment-requests/source-data-amendments "$am" 200 ALREADY_HANDLED

# 索赔 VCL-1：唯一性按（账户、批次、项），批次与项按 UTC 时刻合成。
claim="$(pj "accessRequestId=claim-walk-${stamp}" "batch=SYN-CLAIM-BATCH-${stamp}" "item=SYN-CLAIM-ITEM-${stamp}" \
  applicant=SYN-APPLICANT-01 contract=SYN-CONTRACT-01 "target=$b_parcel" kind=LOSS)"
step "索赔 VCL-1" /v1/claims "$claim" 201 CLAIM_RECEIVED
step "索赔 VCL-1 原样重放" /v1/claims "$claim" 200 CLAIM_EXISTING_RESULT

# 取消 PCN-1：requestedAt 是客户自报的审计业务时间，不比服务端时钟领先即可。
cancel="$(pj "accessRequestId=parcel-cancellation-walk-${stamp}" "originalAccessRequestId=$b_access" "shipmentRequestId=$b_request" \
  "parcelId=$b_parcel" reason=SYN-REASON-WALK "requestedAt=$(date -u +%Y-%m-%dT%H:%M:%SZ)")"
step "取消 PCN-1" /v1/shipment-requests/parcel-cancellations "$cancel" 201 PARCEL_CANCELLED
step "取消 PCN-1 原样重放" /v1/shipment-requests/parcel-cancellations "$cancel" 200 EXISTING_RESULT

printf '== 反例 ==\n' >&2
ghost="SYN-ACCESS-REQUEST-WALK-${stamp}-NOSUCH"
step "撤回 不存在的原提交" /v1/shipment-requests/withdrawals \
  "$(pj "accessRequestId=withdrawal-walk-ghost-${stamp}" "originalAccessRequestId=$ghost" reason=SYN-REASON-WALK)" 404 ""
step "取消 不存在的原提交" /v1/shipment-requests/parcel-cancellations \
  "$(pj "accessRequestId=parcel-cancellation-walk-ghost-${stamp}" "originalAccessRequestId=$ghost" "shipmentRequestId=$b_request" \
    "parcelId=$b_parcel" reason=SYN-REASON-WALK "requestedAt=$(date -u +%Y-%m-%dT%H:%M:%SZ)")" 404 ""
step "撤回 载荷自报 customerAccount" /v1/shipment-requests/withdrawals \
  "$(pj "accessRequestId=withdrawal-walk-selfreport-${stamp}" "originalAccessRequestId=$a_access" reason=SYN-REASON-WALK customerAccount=SYN-ACCOUNT-02)" 400 ""

printf '五口走完：补充、撤回（委托 %s）与修订、索赔、取消（委托 %s，包裹 %s）各一次成立、一次重放；两条 404、一条 400 反例都符合契约\n' \
  "$a_request" "$b_request" "$b_parcel" >&2
