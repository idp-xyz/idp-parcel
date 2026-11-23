echo "== 结算：逐委托销售这一户采用「当前有效各件的有效网络收寄已齐」（票 07） =="
# 不改 SYN-SETTLEMENT-ACCOUNT-01 的包裹计费时点，也不改 21、22 的采用行。
# 这一户的合同已经声明了计价范围，受理前估价采用行就不带范围，避免和合同各说一个。
# 委托级采用行的时点只有 SHIPMENT_MEMBERS_INTAKE_ADOPTED，范围与合同声明的那一个相同。
# 没有这一行，收寄齐了也不形成票级费用。费用项目映射沿用 500 那一节已经登的两行。证据只记 S。
"$BIN/parcel-settlement-register" settlement-account \
  -input "$SEEDS/settlement/28-settlement-account-shipper-23.json"
"$BIN/parcel-settlement-register" pre-acceptance-estimate-adoption \
  -input "$SEEDS/settlement/29-pre-acceptance-estimate-adoption-account-23.json"
"$BIN/parcel-settlement-register" shipment-billing-moment-adoption \
  -input "$SEEDS/settlement/30-shipment-billing-moment-adoption-account-23.json"
