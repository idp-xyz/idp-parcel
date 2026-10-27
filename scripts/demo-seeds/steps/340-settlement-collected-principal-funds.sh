echo "== 代收本金的银行到账：结算先采用、整条认作代收本金（settlement-accounting；ADR-0274、ADR-0330） =="
# 运营企业真实到账只经结算的采用进入本产品：下一节（350-collection）登记那条 COD 指令的银行到账时，证据引用必须指向
# 一条结算已采用、且已整条认作代收本金的外部资金事实，所以这两步排在它前面。事实 140000.00 分（1400.00 CNY）与那条
# 到账同额；认作代收本金的事实不进任何客户应收作用域的入账余额，也不核销，去向在代收与清分的分户账里。
# 重放答已采用 / 已存在，退出码 0。
"$BIN/parcel-settlement-register" external-funds-fact \
  -input "$SEEDS/settlement/09-external-funds-collected-principal-bank-arrival-cod-01.json"
"$BIN/parcel-settlement-register" funds-mapping \
  -input "$SEEDS/settlement/10-funds-mapping-collected-principal-cod-01.json"
