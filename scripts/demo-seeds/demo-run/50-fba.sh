#!/usr/bin/env bash
# 段五·FBA 货件与多行货物小包（票 declared-goods-lines/08；ADR-0373、ADR-0374）：经首方客户渠道提交一票平台货件——三十箱同款
# 加两箱混装，带平台货件号、逐箱平台箱标号与报全的装箱分配——和一票两行货物的小包，两票都等到已接受，再从委托查阅读回
# 货物明细、装箱分配与平台号。之后在管理台运单详情看。
#
# 服务产品用演示种子里的快递产品 SYN-PROD-CN-SG-EXPRESS：ADR-0374 决定一里快递头程就是这条模型，这一票报全了，走不到
# 提交条件。单列的 FBA 头程产品 SYN-PROD-CN-SG-FBA 与它缺箱标号停下、补齐续走的那条线在 55-fba-supplement.sh（票 12）。
# 这张售价卡按件计重（PER_PACKAGE）、逐箱估价再求和，每箱都付一次首重——试算按定价范围选卡，按整票汇总重量计费的卡
# 要另立范围，没登。
# 平台、平台货件号、箱标号与平台仓全是 SYN- 合成值；平台仓只是这票货的收件资料，没有平台仓登记册可登。
#
# 一票三十来箱按演示售价卡两万多元：预付冻结与信用暴露各占一份。本段每跑一次先补一笔余额（后缀带本次戳，事实号不重），
# 授信额度由种子的信用政策重发 v2 给足（steps 115）。每跑一次多两票委托。证据只记 S。
set -euo pipefail
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"
load_env

STAMP="$(date -u +%Y%m%dT%H%M%SZ)"
section "段五·FBA 货件（30 箱同款 + 2 箱混装）与两行货物小包"
# 后缀进 topup-funds.sh 的状态键（shell 变量名），只用字母数字。
bash "$DEMO_DIR/topup-funds.sh" 3000000 "FBA${STAMP}"

CT="$(customer_token)"
OP="$(operator_token)"
SHIPMENT_ID="SYN-FBA-${STAMP}"

FBA_BODY="$(python3 - "$STAMP" "$SHIPMENT_ID" <<'PY'
import json, sys
stamp, shipment = sys.argv[1:3]
# 两行货：T 恤三十箱每箱 24 件，两箱混装各 6 件；帽子只在混装箱里，每箱 10 件。各行数量就是各箱之和，装箱报全。
parcels = []
for box in range(1, 33):
    mixed = box > 30
    parcels.append({
        "customerParcelReference": f"SYN-CTN-{stamp}-{box:03d}",
        "declaredWeightValue": "8.0" if mixed else "12.5",
        "declaredWeightUnit": "KG",
        "declaredLength": "50" if mixed else "60",
        "declaredWidth": "40",
        "declaredHeight": "30" if mixed else "40",
        "declaredDimensionsUnit": "CM",
        "platformBoxLabel": f"{shipment}U{box:06d}",
        "packing": [{"line": 1, "quantity": "6"}, {"line": 2, "quantity": "10"}] if mixed else [{"line": 1, "quantity": "24"}],
    })
print(json.dumps({
    "accessRequestId": f"SYN-ACCESS-FBA-{stamp}",
    "customerShipmentReference": f"SYN-FBA-REF-{stamp}",
    "requestedServiceProduct": "SYN-PROD-CN-SG-EXPRESS",
    "senderRelation": "SYN-SENDER-01",
    "senderAddress": "合成寄件地址",
    "senderPostalCode": "200000",
    "senderCountryCode": "CN",
    "recipientRelation": "SYN-FBA-WH-SG-01",
    "recipientAddress": "合成平台仓地址",
    "recipientPostalCode": "018000",
    "recipientCountryCode": "SG",
    "destinationServiceScope": "SYN-DEST/SG",
    "platformShipment": {"platform": "SYN-MARKETPLACE-01", "shipmentId": shipment, "referenceId": f"SYN-REF-{stamp}"},
    "goods": {
        "contentCategory": "SALE_OF_GOODS",
        "lines": [
            {"descriptions": [{"lang": "zh", "text": "合成男式棉质T恤"}, {"lang": "en", "text": "SYN cotton T-shirt"}],
             "quantity": "732", "unit": "PCS", "unitValue": "3.20", "currency": "USD",
             "commodityCodes": [{"side": "EXPORT", "code": "6109100000"}], "originCountry": "CN", "sku": "SYN-SKU-TEE-01"},
            {"descriptions": [{"lang": "en", "text": "SYN cotton cap"}],
             "quantity": "20", "unit": "PCS", "lineValue": "30.00", "currency": "USD", "originCountry": "CN", "sku": "SYN-SKU-CAP-01"},
        ],
    },
    "parcels": parcels,
}, ensure_ascii=False))
PY
)"
step "提交 FBA 货件（首方客户令牌）" "$CT" POST /v1/shipment-requests "$FBA_BODY" 201:SUBMITTED shipmentRequestId documentNumber
FBA_REQ="$(jget "$LAST" shipmentRequestId)"
FBA_DOC="$(jget "$LAST" documentNumber)"

SMALL_BODY="$(python3 - "$STAMP" <<'PY'
import json, sys
stamp = sys.argv[1]
print(json.dumps({
    "accessRequestId": f"SYN-ACCESS-SMALL-{stamp}",
    "customerShipmentReference": f"SYN-SMALL-REF-{stamp}",
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
        "lines": [
            {"descriptions": [{"lang": "zh", "text": "合成手表"}], "quantity": "1", "unit": "PCS", "unitValue": "25.00",
             "currency": "USD", "originCountry": "CN", "salesOrder": {"platform": "SYN-SHOP-01", "orderNumber": f"SYN-ORDER-{stamp}", "orderLine": "1"}},
            {"descriptions": [{"lang": "en", "text": "SYN watch strap"}], "quantity": "2", "unit": "PCS", "unitValue": "3.00",
             "currency": "USD", "originCountry": "CN", "sku": "SYN-SKU-STRAP-01",
             "salesOrder": {"platform": "SYN-SHOP-01", "orderNumber": f"SYN-ORDER-{stamp}", "orderLine": "2"}},
        ],
    },
    "parcels": [{"customerParcelReference": f"SYN-PCLREF-{stamp}", "declaredWeightValue": "0.50", "declaredWeightUnit": "KG",
                 "declaredLength": "20", "declaredWidth": "15", "declaredHeight": "10", "declaredDimensionsUnit": "CM"}],
}, ensure_ascii=False))
PY
)"
step "提交两行货物小包（首方客户令牌）" "$CT" POST /v1/shipment-requests "$SMALL_BODY" 201:SUBMITTED shipmentRequestId documentNumber
SMALL_REQ="$(jget "$LAST" shipmentRequestId)"
SMALL_DOC="$(jget "$LAST" documentNumber)"

for req in "$FBA_REQ" "$SMALL_REQ"; do
  # 受理判断在 dispatch 下一拍；被拒、或任务不在跑了都如实报出那一份的状态，不当成超时。等受控补充时任务仍是 RUNNING、
  # 读面上只多出 supplementGaps，这一票报全了不会走到那里；那条线的轮询在 55-fba-supplement.sh。
  _decided() {
    curl -s -H "Authorization: Bearer $OP" "$API_BASE/shipment-request-views?shipmentRequestId=${req}" > "$LAST"
    [[ "$(jget "$LAST" request.state)" != SUBMITTED || "$(jget "$LAST" request.acceptanceTask.state)" != RUNNING ]]
  }
  poll "委托 ${req} 形成受理判断" 120 _decided
  state="$(jget "$LAST" request.state)"
  [[ "$state" == ACCEPTED ]] || {
    printf '[失败] 委托 %s 答 %s（受理任务 %s），要 ACCEPTED。详情：\n' "$req" "$state" "$(jget "$LAST" request.acceptanceTask.state)" >&2
    cat "$LAST" >&2
    exit 1
  }
  printf '%-34s → request.state=ACCEPTED\n' "受理判断 ${req}"
done

curl -s -H "Authorization: Bearer $OP" "$API_BASE/shipment-request-views?shipmentRequestId=${FBA_REQ}" > "$LAST"
python3 - "$LAST" "$SHIPMENT_ID" <<'PY'
import json, sys
request = json.load(open(sys.argv[1], encoding="utf-8"))["request"]
goods = request.get("goods") or {}
parcels = request.get("declaredParcels") or []
platform = request.get("platformShipment") or {}
problems = []
if len(goods.get("lines") or []) != 2:
    problems.append(f"货物明细 {len(goods.get('lines') or [])} 行，要 2 行")
if len(parcels) != 32 or not all(p.get("packing") for p in parcels) or not all(p.get("platformBoxLabel") for p in parcels):
    problems.append("32 箱没有逐箱带回装箱分配与平台箱标号")
if platform.get("shipmentId") != sys.argv[2]:
    problems.append(f"平台货件 = {platform!r}")
if problems:
    sys.exit("[失败] FBA 货件详情：" + "；".join(problems))
print(f"{'详情读回（委托查阅）':<30} → 货物明细 2 行、32 箱逐箱带装箱分配与平台箱标号、平台货件号 {platform['shipmentId']}")
PY

printf '\nFBA 货件 运单号 %s（委托 %s，平台货件号 %s）；小包 运单号 %s（委托 %s）。\n' "$FBA_DOC" "$FBA_REQ" "$SHIPMENT_ID" "$SMALL_DOC" "$SMALL_REQ"
printf '去管理台：#/shipment-request-inquiry/%s 看平台货件、货物明细与逐箱装箱；#/shipment-request-inquiry/%s 看两行货物。\n' "$FBA_DOC" "$SMALL_DOC"
printf '同样的货件也可以在「新建平台货件」页（#/fba-shipment）手填提交。\n'
