#!/usr/bin/env bash
# 段三·结算（票 demo-2026-10-30/03）：对最近一次主链那一件包裹——费用确认读数（终局后自动 CONFIRMED，运营补拍口答
# ALREADY_CONFIRMED）→ 对账单发布 → 供应商账单接收（逐行裁决 MATCHED）→ 审核成应付。
#
# 前置：先跑过 10-main-chain.sh（要它写下的 MAIN_CHARGE、MAIN_COST）。本段不另起包裹、不占余额。
# 对账单发布的 lastAttributionDate 取费用行上服务端定的归属日（按结算账户截单时刻折，上海 18:00 之后确认的归下一日），
# 不拿 confirmedAt 的 UTC 日期自己算——那样在上海傍晚以后答 NO_CHARGE_IN_PERIOD；cutOffAt 取此刻（自动截单要等上海 18:00，演示不等，走运营发布口）。
# 同一笔费用发布过对账单后不能再列进第二张（billedOn 已回指），所以本段对同一件主链只能走一次；再演示换一件新主链再走。
# 证据只记 S。
set -euo pipefail
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"
load_env
need_state MAIN_CHARGE MAIN_COST MAIN_COST_FEE MAIN_COST_AMOUNT
if [[ "${SETTLEMENT_DONE_FOR:-}" == "$MAIN_CHARGE" ]]; then
  printf '这件主链的结算已经走过（%s）。再演示请先跑 10-main-chain.sh 出一件新的\n' "$MAIN_CHARGE" >&2
  exit 1
fi
OP="$(operator_token)"
stamp="$(date -u +%Y%m%dT%H%M%SZ)"
section "结算·费用 ${MAIN_CHARGE}"

step "费用读数（客户费用册）" "$OP" GET "/settlement-charges?registry=customer-charge" - 200:CUSTOMER_CHARGES_LISTED
python3 - "$LAST" "$MAIN_CHARGE" <<'PY'
import json, sys
for r in json.load(open(sys.argv[1]))["charges"]:
    if r["charge"] == sys.argv[2]:
        if r["stage"] != "CONFIRMED":
            sys.exit(f"[失败] 费用 {r['charge']} 阶段 {r['stage']}，要 CONFIRMED（终局后自动确认；主链跑完了吗）")
        print(f"{'费用确认读数':<30s} → {r['charge']} stage=CONFIRMED {r['settlementAmount']} {r['settlementCurrency']} basis={r['confirmationBasis']}")
        break
else:
    sys.exit("[失败] 客户费用册里没有 " + sys.argv[2])
PY
amount="$(jsel "$LAST" charges charge "$MAIN_CHARGE" settlementAmount)"
attrdate="$(jsel "$LAST" charges charge "$MAIN_CHARGE" attributionDate)"
if [[ -z "$attrdate" ]]; then
  printf '[失败] 费用 %s 读不到归属日 attributionDate\n' "$MAIN_CHARGE" >&2
  exit 1
fi

step "费用确认补拍（运营确认口）" "$OP" POST /settlement-charge-confirmations "$(pj charge="$MAIN_CHARGE")" "200:ALREADY_CONFIRMED" charge

step "发布对账单" "$OP" POST /settlement-statement-publications \
  "$(pj number="SYN-STMT-DEMO-${stamp}" account=SYN-SETTLEMENT-ACCOUNT-01 period="SYN-PERIOD-DEMO-${stamp}" version=v1 currency=CNY \
      charges:='["'"$MAIN_CHARGE"'"]' lastAttributionDate="$attrdate" declaredTotal="$amount" cutOffAt="$(now_utc)")" \
  201:STATEMENT_PUBLISHED statement total
step "读对账单册" "$OP" GET "/settlement-statements?registry=customer-statement" - 200:*
python3 - "$LAST" "SYN-STMT-DEMO-${stamp}" <<'PY'
import json, sys
body = json.load(open(sys.argv[1]))
rows = [r for r in next((v for v in body.values() if isinstance(v, list)), []) if sys.argv[2] in json.dumps(r)]
print(f"{'  对账单册读回':<30s} → {json.dumps(rows[0], ensure_ascii=False) if rows else '未读到'}")
if not rows:
    sys.exit("[失败] 对账单册里读不到刚发布的单")
PY

bill="SYN-BILL-DEMO-${stamp}"
step "供应商账单接收（逐行 MATCHED）" "$OP" POST /settlement-supplier-bill-receptions \
  "$(pj claim="$bill" version=v1 supplier=SYN-PARTY-CARRIER-TRUNK-01 legalEntity=SYN-LE-01 period="SYN-PERIOD-DEMO-${stamp}" currency=CNY \
      lines:='[{"line":"L1","feeItem":"'"$MAIN_COST_FEE"'","claimed":"'"$MAIN_COST_AMOUNT"'"}]' receivedAt="$(now_utc)" \
      directives:='[{"line":"L1","classification":"MATCHED","expectedVersion":"'"$MAIN_COST"'"}]')" \
  201:BILL_RECEIVED auditAuthority
step "审核成应付" "$OP" POST /settlement-supplier-bill-audits \
  "$(pj claim="$bill" version=v1 line=L1 payable="SYN-PAYABLE-DEMO-${stamp}")" 201:PAYABLE_FORMED payable amount currency
step "读供应商账单接收册" "$OP" GET "/settlement-statements?registry=supplier-bill-reception" - 200:*
printf '%-34s → %s\n' "  接收册读回" "$(head -c 300 "$LAST")"

state_set SETTLEMENT_DONE_FOR "$MAIN_CHARGE"
printf '\n结算完成：费用 %s 已发对账单，供应商账单 %s 已审核成应付\n' "$MAIN_CHARGE" "$bill"
