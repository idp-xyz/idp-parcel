#!/usr/bin/env bash
# 段五之二·FBA 头程缺箱标号停下、客户补齐后接着走（票 declared-goods-lines/12；ADR-0376）：用演示租户单列的 FBA 头程产品
# SYN-PROD-CN-SG-FBA 经首方客户渠道提交一票四箱的平台货件，故意有两箱不报平台箱标号。这个产品采用的接单规则包
# SYN-RULEPKG-FBA-01 引用提交条件 FBA-HEADHAUL@1（决定一），受理任务判到「必要资料」组停下等客户补充，查阅面列出缺的
# 正是那两箱；客户经受控补充口整份重报、带上全部箱标号（决定二），新版本的受理判断走到已接受，缺项随之消失。
# 同一趟再提交一票快递小包，确认快递产品照旧采用 SYN-RULEPKG-01、不受 FBA 条件影响（决定三：规则包随服务产品选）。
#
# 接受之后再看售价（票 declared-goods-lines/17；ADR-0378）：同一计价范围 SYN-SCOPE-01 里两张售价卡各计各的产品——指名 FBA
# 产品的 SYN-PLAN-CN-SG-FBA-01 逐箱按公斤计价、不收首重，不声明产品的 SYN-PLAN-CN-SG-01 接其余产品。断言两条看得见的路：
# 试算（同一只箱子按两个产品各算一次，答复带选卡用的产品与解释）与正式 SELL 评价（FBA 第 1 箱与快递小包各收寄一次，
# 新形成的客户费用经评价号对到价卡的内容摘要）。接受前估价走同一套两层选卡，经结算的估价留痕读口按委托读回（ADR-0379）：
# 判过的每一版各一份，核用的卡、选卡用的产品、逐件明细与两本账上的占用。售价仍逐件评价，按整票汇总计费重的卖法表达不了
# （ADR-0378 越权风险点 8）。
# 平台、平台货件号与箱标号全是 SYN- 合成值，证据只记 S。每跑一次先补一笔余额、多两票委托；
# 标识前缀都带 55（SYN-ACCESS-FBA55-、SYN-FBA55-、SYN-CTN55- 等），与 50 那一段的不相撞。
#
# 用法：55-fba-supplement.sh [--hold]。带 --hold 停在缺项读出之后、不发补充，那票货件留在等客户补的样子，供在管理台运单
# 详情里讲「等客户补 · 缺平台箱标号」；不带就走完补充到已接受，再看售价。
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
SMALL_PARCEL="$(jget "$LAST" parcels.0.parcelId)"
SMALL_VERSION="$(jget "$LAST" submissionVersionId)"
[[ -n "$SMALL_PARCEL" ]] || { printf '[失败] 快递小包的提交回执里没有 parcels.0.parcelId：\n' >&2; cat "$LAST" >&2; exit 1; }

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

section "售价：同一计价范围两张卡，各计各的产品"
# 价卡目录：FBA 卡指名 FBA 产品、快递卡不声明产品，两张同在 SYN-SCOPE-01。按方案标识 q 各取一次（目录分页，不靠第一页
# 里碰巧有），取两张卡的内容摘要，后面拿评价对。
cards_named() {
  curl -s -H "Authorization: Bearer $OP" "$API_BASE/pricing-price-cards?q=$1" \
    | python3 -c 'import json,sys; print(json.dumps(json.load(sys.stdin).get("cards", [])))'
}
CARD_DIGESTS="$(python3 - "$(cards_named SYN-PLAN-CN-SG-FBA-01)" "$(cards_named SYN-PLAN-CN-SG-01)" <<'PY'
import json, sys
def one(raw, plan):
    rows = [c for c in json.loads(raw) if c.get("planId") == plan and c.get("planVersion") == "v1"]
    if len(rows) != 1:
        sys.exit(f"[失败] 价卡目录里 {plan}@v1 有 {len(rows)} 行，要 1 行——种子步骤 170 登过没有")
    return rows[0]
fba, express = one(sys.argv[1], "SYN-PLAN-CN-SG-FBA-01"), one(sys.argv[2], "SYN-PLAN-CN-SG-01")
problems = []
for card in (fba, express):
    if card.get("scope") != "SYN-SCOPE-01" or card.get("direction") != "SELL" or card.get("retired"):
        problems.append(f"{card['planId']} 范围 {card.get('scope')}、方向 {card.get('direction')}、已退役 {card.get('retired')}")
if fba.get("serviceProducts") != ["SYN-PROD-CN-SG-FBA"] or fba.get("canonicalization") != "PPC-7":
    problems.append(f"FBA 卡适用产品 {fba.get('serviceProducts')}、规范化 {fba.get('canonicalization')}，要只指名 FBA 产品、PPC-7")
if express.get("serviceProducts"):
    problems.append(f"快递卡声明了适用产品 {express['serviceProducts']}，要不声明（接其余产品）")
if problems:
    sys.exit("[失败] 价卡目录：" + "；".join(problems))
print(fba["contentDigest"], express["contentDigest"])
PY
)"
FBA_CARD_DIGEST="${CARD_DIGESTS% *}"
EXPRESS_CARD_DIGEST="${CARD_DIGESTS#* }"
printf '%-34s → SYN-PLAN-CN-SG-FBA-01 指名 SYN-PROD-CN-SG-FBA（PPC-7）；SYN-PLAN-CN-SG-01 不声明产品，同在 SYN-SCOPE-01\n' "价卡目录"

# 接受前估价：结算按委托交出每次受理前估价的留痕（ADR-0379）。演示账户的受理前财务控制策略 SYN-FIN-CONTROL-01 先预付冻结、
# 再信用检查，两条都要过，所以每一版估价在两本账上各占一笔，冻结在前：资金冻结与信用暴露。被接受的那一版占着（冻结 HELD、
# 暴露 RECORDED）——后面收寄形成的预估客户费用不转出它们，转出在费用确认时；FBA 货件补充前那一版被新版本换下，两笔都已释放。
estimates_of() { # $1 委托 $2 标签 $3 应选的卡 $4 应带的产品 $5 件数 $6 「版本=冻结状态/暴露状态」，逗号分隔，按提交先后
  step "读接受前估价 ${2}" "$OP" GET "/settlement-pre-acceptance-estimates?shipmentRequest=$1" - 200:PRE_ACCEPTANCE_ESTIMATES_LISTED >/dev/null
  python3 - "$LAST" "$2" "$3" "$4" "$5" "$6" <<'PY'
import json, sys
estimates = json.load(open(sys.argv[1], encoding="utf-8")).get("estimates", [])
label, plan, product, count = sys.argv[2], sys.argv[3], sys.argv[4], int(sys.argv[5])
expected = [item.split("=", 1) for item in sys.argv[6].split(",")]
by_version = {}
for estimate in estimates:
    by_version.setdefault(estimate.get("submissionVersion"), []).append(estimate)
problems = []
if sorted(by_version) != sorted(version for version, _ in expected) or any(len(rows) != 1 for rows in by_version.values()):
    problems.append(f"估价按版本 {json.dumps({v: len(r) for v, r in by_version.items()})}，要判过的每一版恰一份："
                    + "、".join(version for version, _ in expected))
for index, (version, statuses) in enumerate(expected, start=1):
    rows = by_version.get(version) or [{}]
    estimate = rows[0]
    parcels = estimate.get("parcels") or []
    occupations = estimate.get("occupations") or []
    total = int(estimate.get("totalAmount") or 0)
    want = [("FUNDS_FREEZE", statuses.split("/")[0]), ("CREDIT_EXPOSURE", statuses.split("/")[1])]
    got = [(o.get("kind"), o.get("status")) for o in occupations]
    if (estimate.get("priceCard") or {}).get("plan") != plan or estimate.get("serviceProduct") != product:
        problems.append(f"{version} 价卡 {estimate.get('priceCard')}、产品 {estimate.get('serviceProduct')}，要 {plan}、{product}")
    if estimate.get("detailRecorded") is not True or len(parcels) != count \
            or not all(p.get("pricingWeight") and p.get("chargeLines") for p in parcels):
        problems.append(f"{version} 明细：detailRecorded={estimate.get('detailRecorded')}、{len(parcels)} 件，"
                        f"要补记过明细、{count} 件、每件都有计价重量与费用行")
    if sum(int(p.get("totalAmount") or 0) for p in parcels) != total:
        problems.append(f"{version} 各件合计与整份合计 {total} 对不上")
    if got != want or any(o.get("amount") != estimate.get("totalAmount") for o in occupations):
        problems.append(f"{version} 占用 {got}，要 {want}，金额都是整份合计 {total}")
    if not problems:
        occupied = " ".join(f"{kind}/{status}" for kind, status in got)
        print(f"{'接受前估价 ' + label + ' 第 ' + str(index) + ' 版':<34} → {version} plan={plan} product={product} "
              f"total={total // 100}.{total % 100:02d} {estimate['currency']}（{count} 件，各件计价重量与费用行齐） {occupied}")
if problems:
    print(json.dumps(estimates, ensure_ascii=False), file=sys.stderr)
    sys.exit(f"[失败] 接受前估价 {label}：" + "；".join(problems))
PY
}
estimates_of "$FBA_REQ" "FBA 货件" SYN-PLAN-CN-SG-FBA-01 SYN-PROD-CN-SG-FBA 4 \
  "${FBA_VERSION}=RELEASED/RELEASED,${NEW_VERSION}=HELD/RECORDED"
estimates_of "$SMALL_REQ" "快递小包" SYN-PLAN-CN-SG-01 SYN-PROD-CN-SG-EXPRESS 1 "${SMALL_VERSION}=HELD/RECORDED"

# 试算：同一只箱子（12.5 kg、60×40×40 cm，上海 200000 → 新加坡 018000）按两个产品各算一次。试算答复交出选卡用的产品与
# 解释，是今天唯一读得到「按哪个产品选的卡」的面；正式评价的快照同样记着产品，但计价没有评价详情读口。
trial() { # $1 产品 $2 应选的卡
  step "试算 ${1}" "$OP" POST /pricing-estimates \
    "$(pj scope=SYN-SCOPE-01 direction=SELL basisAt="$(now_utc)" weight:='{"value":"12.5","unit":"KG"}' \
        dimensions:='{"length":"60","width":"40","height":"40","unit":"CM"}' \
        postalRoute:='{"origin":"200000","destination":"018000"}' settlementCurrency=CNY serviceProduct="$1")" \
    200:FORMED >/dev/null
  python3 - "$LAST" "$1" "$2" <<'PY'
import json, sys
answer, product, plan = json.load(open(sys.argv[1], encoding="utf-8")), sys.argv[2], sys.argv[3]
candidates = answer.get("candidates") or []
evaluation = (candidates[0].get("evaluation") or {}) if len(candidates) == 1 else {}
first = (evaluation.get("explanation") or [""])[0]
if (len(candidates) != 1 or candidates[0]["plan"]["id"] != plan or evaluation.get("status") != "COMPLETED"
        or evaluation.get("serviceProduct") != product
        or first != f"price card selected for declared service product {product}"):
    print(json.dumps(answer, ensure_ascii=False), file=sys.stderr)
    sys.exit(f"[失败] 试算 {product}：要恰一张 {plan}、评价 COMPLETED、快照与解释首行都写 {product}")
total = evaluation["total"]
print(f"{'试算 ' + product:<34} → plan={plan} serviceProduct={product} total={total['amount']} {total['currency']}"
      f"（解释首行：{first}）")
PY
}
trial SYN-PROD-CN-SG-FBA SYN-PLAN-CN-SG-FBA-01
trial SYN-PROD-CN-SG-EXPRESS SYN-PLAN-CN-SG-01

# 正式 SELL 评价在网络收寄已采用时形成（演示账户采用的计费时点），所以 FBA 第 1 箱与快递小包各送站预报、收寄一次，逐一收，
# 不并发。
FBA_BOX1_REF="SYN-CTN55-${STAMP}-001"
FBA_BOX1="$(python3 -c 'import json,sys; print({p["customerParcelReference"]: p["parcelId"] for p in json.loads(sys.argv[1])["parcels"]}[sys.argv[2]])' \
  "$FBA_SUBMIT" "$FBA_BOX1_REF")"
SMALL_REF="SYN-PCLREF55-${STAMP}"
_prepared() { # 收寄前等三段运输准备都有结果，同主链 chain.sh 的 ch_submit。
  curl -s -H "Authorization: Bearer $OP" "$API_BASE/transport-fulfillment-records?registry=transport-preparation" > "$LAST"
  python3 - "$LAST" "$1" <<'PY'
import json, sys
rows = [r for r in json.load(open(sys.argv[1])).get("preparations", []) if r["object"] == sys.argv[2]]
sys.exit(0 if len(rows) >= 3 and all(r["outcome"] == "OPPORTUNITY_PREPARED" for r in rows) else 1)
PY
}
poll "FBA 第 1 箱 ${FBA_BOX1} 三段运输准备" 90 _prepared "$FBA_BOX1"
poll "快递小包 ${SMALL_PARCEL} 三段运输准备" 90 _prepared "$SMALL_PARCEL"
PRE="$(bash "$SEEDS_DIR/submit-one-pre-advice.sh" "$FBA_BOX1_REF" "$SMALL_REF" 2>/dev/null)" \
  || { printf '[失败] submit-one-pre-advice.sh\n' >&2; exit 1; }
printf '%-34s → DELIVERY_PRE_ADVICE_REGISTERED preAdvice=%s（FBA 第 1 箱与快递小包）\n' "送站预报（首方客户令牌）" "$PRE"

intake_priced() { # $1 标签 $2 客户件号 $3 应选的卡 $4 该卡的内容摘要
  local label=$1 mark out
  # 估价阶段的客户费用行还没有包裹号：来源事实与确认依据两格到确认才落，按包裹号 q 检索不到它。所以按形成时刻认——收寄
  # 之前记下此刻，费用册按 formedAt 倒序取第一页，此刻之后形成的恰一笔就是这一件的；两件逐一收寄，前一件的费用形成了才收
  # 下一件，不会认错。
  mark="$(date -u +%Y-%m-%dT%H:%M:%S.%NZ)"
  out="$(IDP_PARCEL_PRE_ADVICE_ID="$PRE" bash "$SEEDS_DIR/receive-one-delivery.sh" "$2")" \
    || { printf '[失败] receive-one-delivery.sh：%s\n' "$out" >&2; exit 1; }
  [[ "$out" == 201\ INTAKE_FORMED* ]] || { printf '[失败] %s 收寄答复不是 201 INTAKE_FORMED：%s\n' "$1" "$out" >&2; exit 1; }
  printf '%-34s → %s\n' "收寄 ${1}（上海枢纽作业设备）" "${out%%，候选*}"
  _charged() {
    curl -s -H "Authorization: Bearer $OP" "$API_BASE/settlement-charges?registry=customer-charge&sort=-formedAt" > "$LAST"
    CHARGE_ROW="$(python3 - "$LAST" "$mark" "$label" <<'PY'
import json, re, sys
from datetime import datetime

def instant(text):
    # 纳秒截到微秒再比：fromisoformat 只认到微秒，同一台机器的两只钟差不到这一位。
    return datetime.fromisoformat(re.sub(r"(\.\d{6})\d+", r"\1", text).replace("Z", "+00:00"))

mark, label = instant(sys.argv[2]), sys.argv[3]
fresh = [r for r in json.load(open(sys.argv[1])).get("charges", []) if instant(r["formedAt"]) >= mark]
if len(fresh) > 1:
    sys.exit(f"[失败] {label} 收寄之后形成了 {len(fresh)} 笔客户费用，认不出哪笔是它的：" + json.dumps(fresh, ensure_ascii=False))
print(json.dumps(fresh[0]) if fresh else "")
PY
)" || exit 1
    [[ -n "$CHARGE_ROW" ]]
  }
  poll "${1} 的预估客户费用形成" 90 _charged
  local evaluation
  evaluation="$(python3 -c 'import json,sys; print(json.loads(sys.argv[1])["evaluation"])' "$CHARGE_ROW")"
  curl -s -H "Authorization: Bearer $OP" "$API_BASE/pricing-evaluations?q=${evaluation}" > "$LAST"
  python3 - "$LAST" "$CHARGE_ROW" "$1" "$3" "$4" <<'PY'
import json, sys
evaluations = json.load(open(sys.argv[1], encoding="utf-8")).get("evaluations", [])
charge, label, plan, digest = json.loads(sys.argv[2]), sys.argv[3], sys.argv[4], sys.argv[5]
rows = [e for e in evaluations if e.get("evaluationId") == charge.get("evaluation")]
if charge.get("stage") != "ESTIMATED" or len(rows) != 1 or rows[0].get("status") != "COMPLETED" \
        or rows[0].get("planContentDigest") != digest:
    print(json.dumps({"charge": charge, "evaluations": evaluations}, ensure_ascii=False), file=sys.stderr)
    sys.exit(f"[失败] {label}：客户费用 {charge.get('charge')}（{charge.get('stage')}）的评价 {charge.get('evaluation')}"
             f" 没对到 {plan} 的内容摘要 {digest}")
amount = int(charge["originalAmount"])
print(f"{'正式 SELL 评价 ' + label:<34} → charge={charge['charge']} evaluation={charge['evaluation']} "
      f"plan={plan}（内容摘要对上，{rows[0]['canonicalization']}） {amount // 100}.{amount % 100:02d} {charge['originalCurrency']}")
PY
}
intake_priced "FBA 第 1 箱" "$FBA_BOX1_REF" SYN-PLAN-CN-SG-FBA-01 "$FBA_CARD_DIGEST"
intake_priced "快递小包" "$SMALL_REF" SYN-PLAN-CN-SG-01 "$EXPRESS_CARD_DIGEST"

printf '\nFBA 头程货件 运单号 %s（委托 %s，平台货件号 %s，补过一版，第 1 箱已收寄）；快递小包 运单号 %s（委托 %s，已收寄）。\n' \
  "$FBA_DOC" "$FBA_REQ" "$SHIPMENT_ID" "$SMALL_DOC" "$SMALL_REQ"
printf '去管理台：#/shipment-request-inquiry/%s 看补充后的平台货件与逐箱箱标号；#/shipment-request-inquiry/%s 看快递小包；\n' \
  "$FBA_DOC" "$SMALL_DOC"
printf '#/price-card-catalog 看两张卡的适用服务产品；#/pricing-estimate 选产品试算；#/charges-billing 看两笔预估客户费用。\n'
