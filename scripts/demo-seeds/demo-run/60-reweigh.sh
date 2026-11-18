#!/usr/bin/env bash
# 复磅差额走查：申报 1.50 KG、20×15×10 CM（提交脚本里的申报）→ 收寄后称重量方 2.10 KG、22×16×11 CM，申报预估被
# 实测预估替代 → 终局确认 → 复测 2.10 KG、30×25×20 CM 形成借项 → 截单把借项算进对账单 → 作废这次复测形成贷项
# → 贷项单独出一张只有调整行的对账单。
#
# 称重量方是一条：重量与长宽高都必填，只报其中一样不收。复测是再登一条完整的，取发生时间最近的那一条仍有效的。
#
# 售价卡取实重与体积重（长×宽×高÷5000）的较大者，按 0.5 千克向上取整。第一条 22×16×11 的体积重不到 1 千克，
# 按实重 2.10 进到 2.5 千克。复测重量没变、尺寸量大了：30×25×20 的体积重是 3.0 千克，计价重量从 2.5 变到 3.0，
# 差额就出在尺寸上。证据只记 S。每跑一次一件新包裹。
#
# 确认之后故意走一次「定不下」：同一时刻录了两条复测，哪一条都不取，那一代评价停下、没有费用；作废多录的那一条
# 之后的一代要越过停下的那一代，认出已确认的那笔费用，只记差额、不再建费用。请求册没有只读口，「各代都办完了」
# 等不到一个可查的信号，所以两条复测之后、作废之后各留 SETTLE 秒。
set -euo pipefail
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"
load_env
export IDP_PARCEL_API_BASE
source "$DEMO_DIR/chain.sh"

STAMP="$(date -u +%H%M%S)"
OP="$(operator_token)"
SETTLE="${IDP_PARCEL_DEMO_REWEIGH_SETTLE:-12}"

fail() { printf '[失败] %s\n' "$*" >&2; exit 1; }
# charge_row 费用标识：把这一笔在客户费用册上的那一行读进 $LAST。
charge_row() {
  curl -s -H "Authorization: Bearer $OP" "$API_BASE/settlement-charges?registry=customer-charge&q=$1" > "$LAST"
}
charge_ids() {
  curl -s -H "Authorization: Bearer $OP" "$API_BASE/settlement-charges?registry=customer-charge" |
    python3 -c 'import json,sys; print(",".join(sorted(r["charge"] for r in json.load(sys.stdin).get("charges",[]))))'
}
# adjustments_of 费用标识 方向：打印这一笔上该方向的调整金额，一行一笔。
adjustments_of() {
  charge_row "$1"
  python3 - "$LAST" "$1" "$2" <<'PY'
import json, sys
body = json.load(open(sys.argv[1]))
for row in body.get("charges", []):
    if row.get("charge") == sys.argv[2]:
        for adjustment in row.get("adjustments") or []:
            if adjustment.get("direction") == sys.argv[3]:
                print(adjustment.get("settlementAmount"))
PY
}
# weight_cells 费用标识：打印这一笔的「计价重量｜来源｜实重｜尺寸」。计价重量是评价进位之后的那一个，与实重是两格。
weight_cells() {
  charge_row "$1"
  python3 - "$LAST" "$1" <<'PY'
import json, sys
body = json.load(open(sys.argv[1]))
for row in body.get("charges", []):
    if row.get("charge") == sys.argv[2]:
        print("｜".join([
            "%s %s" % (row.get("pricedWeight", ""), row.get("pricedWeightUnit", "")),
            row.get("weightSource", ""),
            "%s %s" % (row.get("actualWeight", ""), row.get("actualWeightUnit", "")),
            "%s %s" % ("×".join(row.get("dimensionSides") or []), row.get("dimensionUnit", "")),
        ]))
PY
}

section "复磅·一件包裹（申报 1.50 KG）"
ch_submit
ch_intake
[[ -n "${UNIT:-}" ]] || fail "收寄没有交回作业实物"
FIRST="$CHARGE"
FIRST_AMOUNT="$(jsel "$LAST" charges charge "$FIRST" settlementAmount)"
printf '%-34s → unit=%s 申报预估 %s 分\n' "收寄实物" "$UNIT" "$FIRST_AMOUNT"

# measure 来源标识 毛重 长 宽 高 发生时间：登一条称重量方，重量（KG）与长宽高（CM）一起报。
measure() {
  local source="$1" weight="$2" length="$3" width="$4" height="$5" at="$6"
  step "称重量方 ${weight} KG、${length}×${width}×${height} CM" "$OP" POST /node-operations/manual-measurements \
    "$(pj sourceId="$source" objectKind=HANDLING_UNIT objectId="$UNIT" node=SYN-NODE-SHA-HUB \
        location=SYN-BENCH-01 basis="$PARCEL" performedBy=SYN-SCALE-01 evidenceRef="WEIGH/$source" \
        occurredAt="$at" weightValue="$weight" weightUnit=KG \
        length="$length" width="$width" height="$height" dimensionUnit=CM originalDocument="SYN-DOC-$source")" \
    "201:MEASUREMENT_RECORDED,200:EXISTING_RESULT"
}

void_measurement() {
  local label="$1" source="$2" target="$3"
  step "$label" "$OP" POST /node-operations/manual-measurement-invalidations \
    "$(pj sourceId="$source" targetSourceId="$target" reason=SYN-VOID-REWEIGH \
        performedBy=SYN-SCALE-01 evidenceRef="WEIGH/$source" occurredAt="$(now_utc)" \
        originalDocument="SYN-DOC-$source")" \
    "201:INVALIDATION_RECORDED,200:EXISTING_RESULT"
}

# 只报重量、不报尺寸的那一条不收：重量与尺寸都是必填字段，口上答 400 并点名缺的那一格。
step "只称重不量方（不收）" "$OP" POST /node-operations/manual-measurements \
  "$(pj sourceId="SYN-WEIGH-${STAMP}-ONLY" objectKind=HANDLING_UNIT objectId="$UNIT" node=SYN-NODE-SHA-HUB \
      location=SYN-BENCH-01 basis="$PARCEL" performedBy=SYN-SCALE-01 evidenceRef="WEIGH/SYN-WEIGH-${STAMP}-ONLY" \
      occurredAt="$(now_utc)" weightValue=2.10 weightUnit=KG originalDocument="SYN-DOC-SYN-WEIGH-${STAMP}-ONLY")" \
  "400:*"

# 第一条称重量方。有了实测，重量与尺寸都用它，申报一格都不补。申报预估的替代者就是当前这一笔。
measure "SYN-WEIGH-${STAMP}-M1" 2.10 22 16 11 "$(now_utc)"
_measured_estimate() {
  charge_row "$FIRST"
  NEXT="$(jsel "$LAST" charges charge "$FIRST" supersededBy)"
  [[ -n "$NEXT" ]] || return 1
  charge_row "$NEXT"
  [[ "$(jsel "$LAST" charges charge "$NEXT" weightSource)" == "actual-measurement" ]]
}
poll "称重量方之后形成实测预估" 120 _measured_estimate
NEXT_AMOUNT="$(jsel "$LAST" charges charge "$NEXT" settlementAmount)"
[[ "$NEXT_AMOUNT" != "$FIRST_AMOUNT" ]] || fail "实测预估 $NEXT 的金额与申报预估相同（$NEXT_AMOUNT）"
printf '%-34s → 申报预估 %s 被 %s 替代，%s 分，重量与尺寸都取实测\n' "预估重估" "$FIRST" "$NEXT" "$NEXT_AMOUNT"
# 费用册上两笔各说各的计价重量与来源：申报那一笔是申报的 1.5 千克与申报的尺寸，实测那一笔的实重 2.1 进到 2.5 千克、
# 尺寸是量出来的 22×16×11，申报的 20×15×10 一格都没留下。
FIRST_CELLS="$(weight_cells "$FIRST")"
NEXT_CELLS="$(weight_cells "$NEXT")"
[[ "$FIRST_CELLS" == "1.5 KG｜declared-measurement｜1.5 KG｜20×15×10 CM" ]] || fail "申报预估 $FIRST 的计价重量与来源 = $FIRST_CELLS"
[[ "$NEXT_CELLS" == "2.5 KG｜actual-measurement｜2.1 KG｜22×16×11 CM" ]] || fail "实测预估 $NEXT 的计价重量与来源 = $NEXT_CELLS"
printf '%-34s → 申报 %s；实测 %s\n' "计价重量｜来源｜实重｜尺寸" "$FIRST_CELLS" "$NEXT_CELLS"
CHARGE="$NEXT"

ch_selfrun
ch_trunk
ch_lastmile
ch_final
_confirmed() {
  charge_row "$CHARGE"
  [[ "$(jsel "$LAST" charges charge "$CHARGE" stage)" == CONFIRMED ]]
}
poll "终局确认实测预估" 90 _confirmed
CONFIRMED_AMOUNT="$(jsel "$LAST" charges charge "$CHARGE" settlementAmount)"
printf '%-34s → %s CONFIRMED %s 分\n' "费用确认" "$CHARGE" "$CONFIRMED_AMOUNT"

# 确认之后复测：重量还是 2.10，尺寸量出来是 30×25×20，体积重 3.0 千克。同一时刻又多录了一条 30×25×25：两条发生
# 时间相同，哪一条都不取，那一代停下。作废多录的那一条，之后的一代按 30×25×20 算，越过停下的那一代认出已确认的
# 那一笔：借项只有一笔，客户费用册上不多出费用。
CHARGES_AT_CONFIRMATION="$(charge_ids)"
sleep 2
REWEIGH_AT="$(now_utc)"
measure "SYN-WEIGH-${STAMP}-M2" 2.10 30 25 20 "$REWEIGH_AT"
measure "SYN-WEIGH-${STAMP}-M3" 2.10 30 25 25 "$REWEIGH_AT"
# 先等这两代办完再作废：作废得太快，它们会赶在作废之后才评价，就走不到「定不下」那一格。
sleep "$SETTLE"
[[ -z "$(adjustments_of "$CHARGE" DEBIT)" ]] || fail "两条复测发生时间相同、定不下，$CHARGE 上却已经有借项"
printf '%-34s → 两条复测发生时间相同，哪一条都不取，没有形成调整\n' "定不下"
void_measurement "作废多录的 30×25×25" "SYN-WEIGH-${STAMP}-M3VOID" "SYN-WEIGH-${STAMP}-M3"
_debit() { [[ -n "$(adjustments_of "$CHARGE" DEBIT)" ]]; }
poll "复测的尺寸形成借项" 120 _debit
sleep "$SETTLE"
DEBIT="$(adjustments_of "$CHARGE" DEBIT)"
[[ "$DEBIT" =~ ^[0-9]+$ ]] || fail "$CHARGE 上的借项不是恰好一笔：$DEBIT"
[[ -z "$(adjustments_of "$CHARGE" CREDIT)" ]] || fail "还没作废复测，$CHARGE 上却有贷项"
[[ "$(charge_ids)" == "$CHARGES_AT_CONFIRMATION" ]] || fail "已确认之后复测，客户费用册上多出了费用：$(charge_ids)"
charge_row "$CHARGE"
BASE="$(jsel "$LAST" charges charge "$CHARGE" settlementAmount)"
ATTR="$(jsel "$LAST" charges charge "$CHARGE" attributionDate)"
NET="$((BASE + DEBIT))"
printf '%-34s → 费用 %s + 借项 %s = %s 分，没有多出费用\n' "计价纠错" "$BASE" "$DEBIT" "$NET"

stamp="$(date -u +%Y%m%dT%H%M%SZ)"
step "截单带入借项" "$OP" POST /settlement-statement-publications \
  "$(pj account=SYN-SETTLEMENT-ACCOUNT-01 period="SYN-PERIOD-REWEIGH-${stamp}" version=v1 currency=CNY \
      charges:='["'"$CHARGE"'"]' lastAttributionDate="$ATTR" declaredTotal="$NET" cutOffAt="$(now_utc)")" \
  201:STATEMENT_PUBLISHED statement total
BILLED_ON="$(jget "$LAST" statement)"
printf '%-34s → 对账单 %s 合计 %s\n' "差额进账期" "$BILLED_ON" "$(jget "$LAST" total)"

# 作废复测那一条，仍有效的只剩第一条，计价重量回到 2.5 千克。
void_measurement "作废 30×25×20 那次复测" "SYN-WEIGH-${STAMP}-M2VOID" "SYN-WEIGH-${STAMP}-M2"
_credit() { [[ -n "$(adjustments_of "$CHARGE" CREDIT)" ]]; }
poll "作废复测形成贷项" 90 _credit
CREDIT="$(adjustments_of "$CHARGE" CREDIT)"
[[ "$CREDIT" == "$DEBIT" ]] || fail "作废复测的贷项是 $CREDIT，want 与借项 $DEBIT 相抵"

# 费用已经出账，贷项不改那张单。这一户没有新费用也照出：一张只有调整行的对账单，净额为负。
step "贷项单独出单" "$OP" POST /settlement-statement-publications \
  "$(pj account=SYN-SETTLEMENT-ACCOUNT-01 period="SYN-PERIOD-REWEIGH-CREDIT-${stamp}" version=v1 currency=CNY \
      charges:='[]' lastAttributionDate="$ATTR" declaredTotal=0 carryUnbilledAdjustments:=true cutOffAt="$(now_utc)")" \
  201:STATEMENT_PUBLISHED statement total
CREDIT_NOTE="$(jget "$LAST" statement)"
CREDIT_TOTAL="$(jget "$LAST" total)"
[[ "$CREDIT_TOTAL" == "-$CREDIT" ]] || fail "贷记单 $CREDIT_NOTE 合计 $CREDIT_TOTAL，want -$CREDIT"
printf '%-34s → 对账单 %s 合计 %s（原账单 %s 不变）\n' "贷项出单" "$CREDIT_NOTE" "$CREDIT_TOTAL" "$BILLED_ON"

printf '\n复磅走查完成：申报预估 %s 被实测预估 %s 替代；确认后复测尺寸的借项 %s 分进了对账单 %s；作废复测的贷项 %s 分单独出了 %s。\n' \
  "$FIRST" "$CHARGE" "$DEBIT" "$BILLED_ON" "$CREDIT" "$CREDIT_NOTE"

# 本段不写 MAIN_* 状态。30-settlement.sh 读的是最近一次主链的那笔费用；这一件的费用在本段里已经出过账，写了会让
# 30 拿它再发一次对账单，被答 ALREADY_BILLED 而停下，10 那一件反倒没人结。
