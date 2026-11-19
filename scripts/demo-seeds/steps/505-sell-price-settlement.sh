echo "== 结算：新形状合同的两户采用册不登计价范围（票 commercial-four-module-requirements/21） =="
# 销售价格来源在合同上。采用册再登范围，旧路会把那一格当成请求的范围。这两户留空。
# SYN-SETTLEMENT-ACCOUNT-01 的采用行仍带着 SYN-SCOPE-01，证明旧规范化版本合同照旧走采用册。
"$BIN/parcel-settlement-register" settlement-account \
  -input "$SEEDS/settlement/22-settlement-account-shipper-21.json"
"$BIN/parcel-settlement-register" settlement-account \
  -input "$SEEDS/settlement/23-settlement-account-shipper-22.json"
"$BIN/parcel-settlement-register" pre-acceptance-estimate-adoption \
  -input "$SEEDS/settlement/24-pre-acceptance-estimate-adoption-account-21.json"
"$BIN/parcel-settlement-register" sell-billing-moment-adoption \
  -input "$SEEDS/settlement/25-sell-billing-moment-adoption-account-21.json"
"$BIN/parcel-settlement-register" pre-acceptance-estimate-adoption \
  -input "$SEEDS/settlement/26-pre-acceptance-estimate-adoption-account-22.json"
"$BIN/parcel-settlement-register" sell-billing-moment-adoption \
  -input "$SEEDS/settlement/27-sell-billing-moment-adoption-account-22.json"
