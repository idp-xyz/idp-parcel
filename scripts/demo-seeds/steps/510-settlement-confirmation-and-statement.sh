echo "== 费用确认、到点截单与供应商账单审核的演示租户取值（settlement-accounting、access-identity、pilot-governance；ADR-0336、0339、0340） =="
# 演示租户经参考配置采用这几格，证据只记 S。确认：运费项目按「有效交付已终局」确认，确认之后按确认日归属；确认与截单两个时点都采用。
# 截单：货主应收账户按上海时区、18:00 截单，日截、自 2026-10-27 起——日截是为了演示当天就看得到一张单，不是给任何租户的建议。
"$BIN/parcel-settlement-register" charge-confirmation-condition \
  -input "$SEEDS/settlement/11-charge-confirmation-condition-freight.json"
"$BIN/parcel-settlement-register" charge-attribution \
  -input "$SEEDS/settlement/12-charge-attribution-freight-on-confirmation.json"
"$BIN/parcel-settlement-register" statement-arrangement \
  -input "$SEEDS/settlement/13-statement-arrangement-account-01.json"
"$BIN/parcel-settlement-register" settlement-moment \
  -input "$SEEDS/settlement/14-settlement-moment-confirm.json"
"$BIN/parcel-settlement-register" settlement-moment \
  -input "$SEEDS/settlement/15-settlement-moment-cut-off.json"
"$BIN/parcel-settlement-register" statement-cycle \
  -input "$SEEDS/settlement/16-statement-cycle-account-01-daily.json"
# 供应商账单：干线承运商在法人 SYN-LE-01 下的 CNY 应付账户，审核授权方与越权升级上限（1 万元）。账单本身不由种子登，演示时经
# /settlement-supplier-bill-receptions 照来件录入。
"$BIN/parcel-settlement-register" settlement-account \
  -input "$SEEDS/settlement/17-settlement-account-carrier-trunk-payable-cny.json"
"$BIN/parcel-settlement-register" supplier-payable-account \
  -input "$SEEDS/settlement/18-supplier-payable-account-carrier-trunk.json"
"$BIN/parcel-settlement-register" supplier-audit-authority \
  -input "$SEEDS/settlement/19-supplier-audit-authority-carrier-trunk.json"
"$BIN/parcel-settlement-register" audit-escalation-ceiling \
  -input "$SEEDS/settlement/20-audit-escalation-ceiling-carrier-trunk.json"
# 四种结算运营决定由配置员持有：费用确认、对账单发布、供应商账单接收、审核应付。照 477 那一节的约定，每一种登一行准入对照、一段权威
# 区间——少一行，授予齐备也答 OUTSIDE_ADMISSION_SCOPE。谁持有是租户取值，这里登的是演示租户的取值。
"$BIN/parcel-governance-register" authority-interval -input "$SEEDS/governance/69-authority-interval-operation-decision-charge-confirmation.json"
"$BIN/parcel-governance-register" authority-interval -input "$SEEDS/governance/70-authority-interval-operation-decision-statement-publication.json"
"$BIN/parcel-governance-register" authority-interval -input "$SEEDS/governance/71-authority-interval-operation-decision-supplier-bill-reception.json"
"$BIN/parcel-governance-register" authority-interval -input "$SEEDS/governance/72-authority-interval-operation-decision-supplier-bill-audit.json"
"$BIN/parcel-access-register" operator-grant -input "$SEEDS/access/24-grants-settlement-decisions.json"
"$BIN/parcel-access-register" admission-scope-mapping-register -input "$SEEDS/access/25-admission-scope-mappings-settlement-decisions.json"
