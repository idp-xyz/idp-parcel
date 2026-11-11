#!/usr/bin/env bash
# 段五之二·FBA 头程缺箱标号停下、客户补齐后接着走（票 declared-goods-lines/12；ADR-0376）：用演示租户单列的 FBA 头程产品
# SYN-PROD-CN-SG-FBA 经首方客户渠道提交一票四箱的平台货件，故意有两箱不报平台箱标号。这个产品采用的接单规则包
# SYN-RULEPKG-FBA-01 引用提交条件 FBA-HEADHAUL@1（决定一），受理任务判到「必要资料」组停下等客户补充，查阅面列出缺的
# 正是那两箱；客户经受控补充口整份重报、带上全部箱标号（决定二），新版本的受理判断走到已接受，缺项随之消失。
# 同一趟再提交一票快递小包，确认快递产品照旧采用 SYN-RULEPKG-01、不受 FBA 条件影响（决定三：规则包随服务产品选）。
#
# FBA 产品沿用快递那张按件计重的售价卡：选卡按（定价范围，方向，目的，时点），没有服务产品一维，定价范围取自账户的结算
# 采用、一户一个，同一账户的不同产品现在选不了各自的卡——这是缺的机制，归票 declared-goods-lines/13。
# 平台、平台货件号与箱标号全是 SYN- 合成值，证据只记 S。每跑一次先补一笔余额、多两票委托；
# 标识前缀都带 55（SYN-ACCESS-FBA55-、SYN-FBA55-、SYN-CTN55- 等），与 50 那一段的不相撞。
#
# 用法：55-fba-supplement.sh [--hold]。带 --hold 停在缺项读出之后、不发补充，那票货件留在等客户补的样子，供在管理台运单
# 详情里讲「等客户补 · 缺平台箱标号」；不带就走完补充到已接受。
set -euo pipefail
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"
load_env

HOLD=""
case "${1:-}" in
  "") ;;
  --hold) HOLD=1 ;;
  *) printf '用法：%s [--hold]\n' "$0" >&2; exit 1 ;;
esac

STAMP="$(date -u +%Y%m%dT%H%M%SZ)"
section "段五之二·FBA 头程缺两箱箱标号 → 受控补充 → 已接受；快递小包照旧接受"
# 后缀进 topup-funds.sh 的状态键（shell 变量名），只用字母数字。
bash "$DEMO_DIR/topup-funds.sh" 1000000 "FBASUPP${STAMP}"

CT="$(customer_token)"
OP="$(operator_token)"
SHIPMENT_ID="SYN-FBA55-${STAMP}"
UNLABELLED="2,4"

# 四箱同款，每箱 24 件；第 2、4 箱不报平台箱标号。$1 写 all 时四箱都带（补充用）。
fba_content() {
  python3 - "$STAMP" "$SHIPMENT_ID" "$UNLABELLED" "$1" <<'PY'
import json, sys
stamp, shipment, unlabelled, mode = sys.argv[1:5]
skip = set() if mode == "all" else {int(box) for box in unlabelled.split(",")}
parcels = []
for box in range(1, 5):
    parcel = {
        "customerParcelReference": f"SYN-CTN55-{stamp}-{box:03d}",
        "declaredWeightValue": "12.5",
        "declaredWeightUnit": "KG",
        "declaredLength": "60",
        "declaredWidth": "40",
        "declaredHeight": "40",
        "declaredDimensionsUnit": "CM",
        "packing": [{"line": 1, "quantity": "24"}],
    }
    if box not in skip:
        parcel["platformBoxLabel"] = f"{shipment}U{box:06d}"
    parcels.append(parcel)
print(json.dumps({
    "accessRequestId": f"SYN-ACCESS-FBA55-{stamp}",
    "customerShipmentReference": f"SYN-FBA55-REF-{stamp}",
    "requestedServiceProduct": "SYN-PROD-CN-SG-FBA",
    "senderRelation": "SYN-SENDER-01",
    "senderAddress": "合成寄件地址",
    "senderPostalCode": "200000",
    "senderCountryCode": "CN",
    "recipientRelation": "SYN-FBA-WH-SG-01",
    "recipientAddress": "合成平台仓地址",
    "recipientPostalCode": "018000",
    "recipientCountryCode": "SG",
    "destinationServiceScope": "SYN-DEST/SG",
    "platformShipment": {"platform": "SYN-MARKETPLACE-01", "shipmentId": shipment, "referenceId": f"SYN-REF55-{stamp}"},
    "goods": {
        "contentCategory": "SALE_OF_GOODS",
        "lines": [
            {"descriptions": [{"lang": "zh", "text": "合成男式棉质T恤"}, {"lang": "en", "text": "SYN cotton T-shirt"}],
             "quantity": "96", "unit": "PCS", "unitValue": "3.20", "currency": "USD",
             "commodityCodes": [{"side": "EXPORT", "code": "6109100000"}], "originCountry": "CN", "sku": "SYN-SKU-TEE-01"},
        ],
    },
    "parcels": parcels,
}, ensure_ascii=False))
PY
}

FBA_BODY="$(fba_content partial)"
step "提交 FBA 头程货件（缺第 2、4 箱箱标号）" "$CT" POST /v1/shipment-requests "$FBA_BODY" 201:SUBMITTED \
  shipmentRequestId documentNumber submissionVersionId
FBA_SUBMIT="$(cat "$LAST")"
FBA_REQ="$(jget "$LAST" shipmentRequestId)"
FBA_DOC="$(jget "$LAST" documentNumber)"
FBA_VERSION="$(jget "$LAST" submissionVersionId)"

SMALL_BODY="$(python3 - "$STAMP" <<'PY'
import json, sys
stamp = sys.argv[1]
print(json.dumps({
    "accessRequestId": f"SYN-ACCESS-SMALL55-{stamp}",
    "customerShipmentReference": f"SYN-SMALL55-REF-{stamp}",
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
    "goods": {
        "contentCategory": "SALE_OF_GOODS",
        "lines": [{"descriptions": [{"lang": "zh", "text": "合成手表"}], "quantity": "1", "unit": "PCS", "unitValue": "25.00",
                   "currency": "USD", "originCountry": "CN"}],
    },
    "parcels": [{"customerParcelReference": f"SYN-PCLREF55-{stamp}", "declaredWeightValue": "0.50", "declaredWeightUnit": "KG",
                 "declaredLength": "20", "declaredWidth": "15", "declaredHeight": "10", "declaredDimensionsUnit": "CM"}],
}, ensure_ascii=False))
PY
)"
step "提交快递小包（首方客户令牌）" "$CT" POST /v1/shipment-requests "$SMALL_BODY" 201:SUBMITTED shipmentRequestId documentNumber
SMALL_REQ="$(jget "$LAST" shipmentRequestId)"
SMALL_DOC="$(jget "$LAST" documentNumber)"

# 受理判断在 dispatch 下一拍。判过一轮的读面信号有三种：委托离开已提交；任务不在跑了；或查阅面列出了缺项——等受控补充时
# 任务仍是 RUNNING（只是续办方换成客户），读面上能看见的只有 supplementGaps。$1 是委托号，$2 给了就还要求查阅面已读到
# 这一版：补充落库时新版本的任务同一笔起跑、缺项清空，这一格只防读到补充前的旧行。
# 查阅答不出这份委托就地停下并打出原始答复，不让空答复漏进后面的断言，也不让 poll 的超时把它说成 dispatch 没跑。
# 404 也停：查阅直读提交同一笔写下的行，提交答过 201 之后读不到不是「还没到」，是统一不可见——委托号不对，或这位
# 操作者的作用域（令牌）看不到它。
_decided() {
  local code
  code="$(curl -sS -o "$LAST" -w '%{http_code}' -H "Authorization: Bearer $OP" \
    "$API_BASE/shipment-request-views?shipmentRequestId=$1" 2>/dev/null)" || code="${code:-000}"
  if [[ "$code" != 200 || -z "$(jget "$LAST" request.state)" ]]; then
    printf '\n[失败] 读委托 %s 的查阅答 HTTP %s（000 即连不上 API；404 是看不到这份委托：委托号或操作者令牌不对），没有可判的委托。原始答复：\n' "$1" "$code" >&2
    cat "$LAST" >&2 2>/dev/null || true
    printf '\n' >&2
    exit 1
  fi
  [[ -z "${2:-}" || "$(jget "$LAST" request.submissionVersionId)" == "$2" ]] || return 1
  [[ "$(jget "$LAST" request.state)" != SUBMITTED || "$(jget "$LAST" request.acceptanceTask.state)" != RUNNING ||
    -n "$(jget "$LAST" request.acceptanceTask.supplementGaps)" ]]
}
must_be_accepted() {
  local req=$1 state
  state="$(jget "$LAST" request.state)"
  [[ "$state" == ACCEPTED ]] || {
    printf '[失败] 委托 %s 答 %s（受理任务 %s），要 ACCEPTED。详情：\n' "$req" "$state" "$(jget "$LAST" request.acceptanceTask.state)" >&2
    cat "$LAST" >&2
    exit 1
  }
}

poll "快递小包 ${SMALL_REQ} 形成受理判断" 120 _decided "$SMALL_REQ"
must_be_accepted "$SMALL_REQ"
printf '%-34s → request.state=ACCEPTED（快递产品照旧采用 SYN-RULEPKG-01）\n' "受理判断 ${SMALL_REQ}"

poll "FBA 货件 ${FBA_REQ} 停下" 120 _decided "$FBA_REQ"
# 停下的样子要逐格对上：仍是已提交、任务仍在（RUNNING，等客户）、缺项正好是第 2、4 箱的箱标号且判在「必要资料」组
# （ADR-0376 决定一的要点就是这一组）——多一条（平台货件、装箱）、少一箱或归错组都不算。
python3 - "$LAST" "$FBA_SUBMIT" "$STAMP" "$UNLABELLED" <<'PY'
import json, sys
view = json.load(open(sys.argv[1], encoding="utf-8"))["request"]
submitted = json.loads(sys.argv[2])
stamp, unlabelled = sys.argv[3], sys.argv[4]
ids = {p["customerParcelReference"]: p["parcelId"] for p in submitted["parcels"]}
want = {ids[f"SYN-CTN55-{stamp}-{int(box):03d}"] for box in unlabelled.split(",")}
task = view.get("acceptanceTask") or {}
gaps = task.get("supplementGaps") or []
got = {g.get("parcelId") for g in gaps}
problems = []
if view.get("state") != "SUBMITTED" or task.get("state") != "RUNNING":
    problems.append(f"委托 {view.get('state')}、任务 {task.get('state')}，要 SUBMITTED、RUNNING（等客户补充）")
if len(gaps) != 2 or got != want or any(
        g.get("group") != "REQUIRED_DOCUMENT" or g.get("reason") != "EVERY_PARCEL_HAS_PLATFORM_BOX_LABEL" for g in gaps):
    problems.append(f"缺项 = {json.dumps(gaps, ensure_ascii=False)}，要第 2、4 箱 {sorted(want)} 各一条"
                    " REQUIRED_DOCUMENT / EVERY_PARCEL_HAS_PLATFORM_BOX_LABEL")
if problems:
    print(json.dumps(view, ensure_ascii=False), file=sys.stderr)
    sys.exit("[失败] FBA 货件停下的样子：" + "；".join(problems))
print(f"{'受理任务停下等客户补充':<24} → request.state=SUBMITTED acceptanceTask.state={task['state']} "
      f"supplementGaps={len(gaps)} 条")
for gap in sorted(gaps, key=lambda g: g["parcelId"]):
    print(f"{'  缺项':<30} → group={gap['group']} parcelId={gap['parcelId']} reason={gap['reason']}")
PY

if [[ -n "$HOLD" ]]; then
  printf '\nFBA 头程货件 运单号 %s（委托 %s，平台货件号 %s）停在等客户补；快递小包 运单号 %s（委托 %s）已接受。\n' \
    "$FBA_DOC" "$FBA_REQ" "$SHIPMENT_ID" "$SMALL_DOC" "$SMALL_REQ"
  printf '去管理台：#/shipment-request-inquiry/%s 的受理进度里逐箱列出「等客户补 · 缺平台箱标号」。不带 --hold 再跑一次看补齐之后。\n' "$FBA_DOC"
  exit 0
fi

# 受控补充：整份重报提交内容（不从旧版本继承），成员用回执里的 parcelId，平台声明连同四箱箱标号整份带上（决定二）。
SUPPLEMENT_BODY="$(python3 - "$(fba_content all)" "$FBA_SUBMIT" "$STAMP" <<'PY'
import json, sys
content, submitted, stamp = json.loads(sys.argv[1]), json.loads(sys.argv[2]), sys.argv[3]
ids = {p["customerParcelReference"]: p["parcelId"] for p in submitted["parcels"]}
original = content["accessRequestId"]
content["accessRequestId"] = f"SYN-ACCESS-FBA55-SUPP-{stamp}"
content["parcels"] = [{"parcelId": ids[p["customerParcelReference"]], **p} for p in content["parcels"]]
print(json.dumps({"originalAccessRequestId": original, "shipmentRequestId": submitted["shipmentRequestId"],
                  "basisSubmissionVersionId": submitted["submissionVersionId"], **content}, ensure_ascii=False))
PY
)"
step "受控补充（补齐第 2、4 箱箱标号）" "$CT" POST /v1/shipment-requests/supplements "$SUPPLEMENT_BODY" \
  201:SUPPLEMENT_RECORDED submissionVersionId
NEW_VERSION="$(jget "$LAST" submissionVersionId)"

poll "FBA 货件 ${FBA_REQ} 按新版本形成受理判断" 120 _decided "$FBA_REQ" "$NEW_VERSION"
must_be_accepted "$FBA_REQ"
python3 - "$LAST" "$FBA_VERSION" "$SUPPLEMENT_BODY" <<'PY'
import json, sys
view = json.load(open(sys.argv[1], encoding="utf-8"))["request"]
sent = {p["parcelId"]: p["platformBoxLabel"] for p in json.loads(sys.argv[3])["parcels"]}
gaps = (view.get("acceptanceTask") or {}).get("supplementGaps")
got = {p.get("parcelId"): p.get("platformBoxLabel") for p in view.get("declaredParcels") or []}
problems = []
if gaps:
    problems.append(f"缺项还在：{json.dumps(gaps, ensure_ascii=False)}")
if got != sent:
    problems.append(f"逐箱箱标号 = {json.dumps(got, ensure_ascii=False)}，要补充里发出的 {json.dumps(sent, ensure_ascii=False)}")
if view.get("submissionVersionId") == sys.argv[2]:
    problems.append("判的还是补充前那一版")
if view.get("priorVersionCount") != 1:
    problems.append(f"priorVersionCount = {view.get('priorVersionCount')!r}，补过一版要 1")
if problems:
    sys.exit("[失败] FBA 货件补充后：" + "；".join(problems))
print(f"{'受理判断（补充后的版本）':<28} → request.state=ACCEPTED priorVersionCount={view.get('priorVersionCount')} "
      f"supplementGaps=无，4 箱箱标号与补充所发逐一相同")
PY

printf '\nFBA 头程货件 运单号 %s（委托 %s，平台货件号 %s，补过一版）；快递小包 运单号 %s（委托 %s）。\n' \
  "$FBA_DOC" "$FBA_REQ" "$SHIPMENT_ID" "$SMALL_DOC" "$SMALL_REQ"
printf '去管理台：#/shipment-request-inquiry/%s 看补充后的平台货件与逐箱箱标号；#/shipment-request-inquiry/%s 看快递小包。\n' \
  "$FBA_DOC" "$SMALL_DOC"
