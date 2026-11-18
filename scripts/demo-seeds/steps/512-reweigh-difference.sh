echo "== 复磅差额：当前有效实测形态、实测差异自动调整、调整登记授予（演示租户） =="
# 这三件都是演示租户的取值，不是产品默认。没采用形态时，同一实物多于一条仍有效实测（复测过）就不定一条。
# 没采用自动调整时，已确认费用上的差额停住，不形成计价纠错。证据只记 S。种子不形成任何测量。
"$BIN/parcel-node-operations-register" -kind measurement-selection \
  -file "$SEEDS/node-operations/02-measurement-selection-syn-tenant-01.json"
"$BIN/parcel-settlement-register" reweigh-adjustment-adoption \
  -input "$SEEDS/settlement/21-reweigh-adjustment-adoption-account-01.json"
"$BIN/parcel-governance-register" authority-interval -input "$SEEDS/governance/79-authority-interval-operation-decision-charge-adjustment-registration.json"
"$BIN/parcel-access-register" operator-grant -input "$SEEDS/access/41-grants-charge-adjustment-registration.json"
"$BIN/parcel-access-register" admission-scope-mapping-register -input "$SEEDS/access/42-admission-scope-mapping-charge-adjustment-registration.json"
