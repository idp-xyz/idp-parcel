#!/usr/bin/env bash
# 段六·销售价格来源（票 commercial-four-module-requirements/21）：两位从开户起就在合同上声明销售价格来源的客户各发一件，
# 专属在前的那一位再发一件走专属卡没谈到的线路。每件都走 提交 → 受理（受理前估价留痕）→ 初始路由 → 运输准备 → 送站预报 →
# 收寄 → 预估客户费用形成，然后对四样：
#   - 受理前估价用的计价范围与价卡；
#   - 估价金额与预估客户费用金额相等；
#   - 费用形成时登下的主要计费范围是评价实际用上的那一个范围，不是合同列表的第一项；
#   - 正式评价的解释逐格写出问过的来源（只声明一个来源的合同没有这种行）。
#
# 期望金额按种子价卡手算，箱子是 submit-one-shipment.sh 提交的那一件（1.50 kg、20×15×10 cm，计费重 1.5 kg，首重加两档续重）。
# 读面上的金额是最小币单位：
#   SYN-ACCOUNT-21（只声明公共范围）  邮编 018 → 公共卡 Z1                    55 + 2×18 + 5 = 96.00 → 9600
#   SYN-ACCOUNT-22（专属在前）        邮编 018 → 专属卡 Z1                    50 + 2×16 + 5 = 87.00 → 8700
#   SYN-ACCOUNT-22                    邮编 238 → 专属卡没有 Z2，落到公共卡 Z2  48 + 2×15 + 5 = 83.00 → 8300
#
# 后两样直接读库，只读：主要计费范围是确认事实里的一项，费用确认之前客户费用读面不显示它；评价的解释没有读面。库的坐标取
# up.sh 记在 env.sh 里的那一份。
#
# 走 Z2 的那一件不等初始路由：演示网络的成本卡 SYN-PLAN-CN-SG-COST-01 只有 Z1，Z2 的候选路线算不出成本，初始路由停在
# 候选成本待定。受理、收寄与销售计费都不等路由，所以这一件照样走到预估客户费用；它往后的运输各段走不了。
#
# 用法：60-sell-price-sources.sh     每跑一次三件新包裹，停在预估客户费用形成，不往下走运输。
# 两户的预收款首次跑时各补一笔 1000.00（后缀固定，同一只库上重跑不重复补）：21 那一户每跑一次用 96.00，22 那一户用 170.00，
# 约够跑五次，用尽了换一个后缀再补（topup-funds.sh）。发行方要认得 21、22 两台客户端：up.sh 探到共享发行方不认时会另起
# 私有的那一只。证据只记 S。
set -euo pipefail
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"
load_env
export IDP_PARCEL_API_BASE
source "$DEMO_DIR/chain.sh"

"$DEMO_DIR/topup-funds.sh" 100000 SP21 SYN-SETTLEMENT-ACCOUNT-21 SYN-PARTY-SHIPPER-21
"$DEMO_DIR/topup-funds.sh" 100000 SP22 SYN-SETTLEMENT-ACCOUNT-22 SYN-PARTY-SHIPPER-22

# sp_read SQL：对演示库跑一句只读查询，打印一格。标识都是本脚本刚从接口拿到的委托产物，不来自外部输入。
sp_read() {
  docker exec "$IDP_PARCEL_DEMO_PG_CONTAINER" psql -U parcel -d "$IDP_PARCEL_DEMO_DB" -v ON_ERROR_STOP=1 -Atc "$1"
}

# sp_parcel 标签 客户序号 收件邮编 期望计价范围 期望价卡 期望金额（最小币单位） 期望问过的来源（逗号分隔的「范围:结论」，没有就给空串）
sp_parcel() {
  local label=$1 seq=$2 postal=$3 scope=$4 card=$5 amount=$6 trail=$7
  section "$label"
  export DEMO_CUSTOMER_CLIENT="syn-customer-client-${seq}" DEMO_CUSTOMER_SECRET="syn-customer-client-${seq}-secret"
  IDP_PARCEL_CUSTOMER_TOKEN="$(customer_token)"
  export IDP_PARCEL_CUSTOMER_TOKEN
  export IDP_PARCEL_DEMO_RECIPIENT_POSTAL_CODE="$postal"
  STAMP="$(date -u +%H%M%S)"
  ch_submit
  step "受理前估价留痕" "$OP" GET "/settlement-pre-acceptance-estimates?shipmentRequest=${REQ}" - "200:*" \
    estimates.0.totalAmount estimates.0.pricingScope estimates.0.priceCard.plan estimates.0.parcels.0.pricingScope
  local estimate_file="$RUN_DIR/sell-price-estimate-${seq}-${postal}.json"
  cp "$LAST" "$estimate_file"
  ch_intake
  local charge_amount evaluation charge_scope explanation
  charge_amount="$(jsel "$LAST" charges charge "$CHARGE" settlementAmount)"
  evaluation="$(jsel "$LAST" charges charge "$CHARGE" evaluation)"
  # 确认事实随费用形成之后的那一步登下，等它落册。
  _fact_registered() {
    charge_scope="$(sp_read "SELECT primary_charging_scope FROM settlement_accounting.charge_confirmation_fact WHERE tenant_id = 'SYN-TENANT-01' AND charge_id = '${CHARGE}'")"
    [[ -n "$charge_scope" ]]
  }
  poll "费用的确认事实登下" 60 _fact_registered
  explanation="$(sp_read "SELECT snapshot::text FROM parcel_pricing.evaluation WHERE tenant_id = 'SYN-TENANT-01' AND evaluation_id = '${evaluation}'")"
  python3 - "$estimate_file" "$PARCEL" "$scope" "$card" "$amount" "$charge_amount" "$charge_scope" "$trail" "$label" "$explanation" <<'PY'
import json, sys

path, parcel_id, scope, card, amount, charge_amount, charge_scope, trail, label, snapshot = sys.argv[1:11]
estimates = json.load(open(path, encoding="utf-8")).get("estimates") or []
if not estimates:
    sys.exit(f"[失败] {label}：这份委托没有受理前估价留痕")
estimate = estimates[-1]
rows = [row for row in estimate.get("parcels") or [] if row.get("parcel") == parcel_id] or (estimate.get("parcels") or [])
if len(rows) != 1:
    sys.exit(f"[失败] {label}：估价留痕里对不上这一件 {parcel_id}：{estimate.get('parcels')}")
row = rows[0]
# 只带一个范围的估价，逐件那一格不写范围与卡，它们在留痕的头上。
used_scope = row.get("pricingScope") or estimate.get("pricingScope")
used_card = row.get("priceCard") or (estimate.get("priceCard") or {}).get("plan") or ""
problems = []
if used_scope != scope:
    problems.append(f"估价用的计价范围是 {used_scope}，应是 {scope}")
if card not in used_card:
    problems.append(f"估价用的价卡是 {used_card}，应是 {card}")
if str(row.get("totalAmount")) != amount:
    problems.append(f"估价金额 {row.get('totalAmount')}，手算 {amount}")
if charge_amount != str(row.get("totalAmount")):
    problems.append(f"预估客户费用 {charge_amount} 与受理前估价 {row.get('totalAmount')} 不同额")
if charge_scope != scope:
    problems.append(f"费用的主要计费范围是 {charge_scope}，应是评价实际用上的 {scope}")
if not snapshot:
    problems.append("正式评价读不回")
for step in [s for s in trail.split(",") if s]:
    asked, _, kind = step.partition(":")
    line = f"sell price source {asked}: {kind}"
    if line not in snapshot:
        problems.append(f"正式评价的解释里没有「{line}」")
if not trail and "sell price source " in snapshot:
    problems.append("只声明一个来源的合同，评价的解释里不该有逐来源的行")
if problems:
    sys.exit(f"[失败] {label}：" + "；".join(problems))
yuan = f"{int(amount) // 100}.{int(amount) % 100:02d}"
print(f"{'估价与预估客户费用':<34} → 同额 {yuan} CNY；范围 {used_scope}；价卡 {used_card}；问过 {trail or '（只有一个来源）'}")
PY
}

sp_parcel "只声明公共范围的客户（SYN-ACCOUNT-21，邮编 018）" 21 018000 SYN-SCOPE-01 SYN-PLAN-CN-SG-01 9600 ""
sp_parcel "专属在前的客户·谈下来的线路（SYN-ACCOUNT-22，邮编 018）" 22 018000 SYN-SCOPE-EXCL-01 SYN-PLAN-CN-SG-EXCL-01 8700 \
  "SYN-SCOPE-EXCL-01:SELECTED"
CH_SUBMIT_STOP_AFTER_ACCEPTANCE=1 \
  sp_parcel "专属在前的客户·没谈到的线路（SYN-ACCOUNT-22，邮编 238）" 22 238000 SYN-SCOPE-01 SYN-PLAN-CN-SG-01 8300 \
  "SYN-SCOPE-EXCL-01:BASE_RATE_NOT_IN_TABLE,SYN-SCOPE-01:SELECTED"

printf '\n销售价格来源走通：三件包裹的受理前估价与预估客户费用同额，没谈到的线路落到合同声明的后一个范围。\n'
