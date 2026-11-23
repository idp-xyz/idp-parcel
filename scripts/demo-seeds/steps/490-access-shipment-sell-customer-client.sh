echo "== 首方客户端：逐委托销售的客户一台（access-identity；票 07） =="
# SYN-ACCOUNT-23 绑 syn-customer-client-23。一个首方客户端只代表一个客户账户，不能借 01、21、22。
# 授予只给提交委托、送站预报、按包裹与按运单号查追踪。排在 489 之后：种子按节新增。证据只记 S。
"$BIN/parcel-access-register" customer-client-register -input "$SEEDS/access/45-customer-clients-shipment-sell.json"
"$BIN/parcel-access-register" customer-client-grant -input "$SEEDS/access/46-customer-client-grants-shipment-sell.json"
