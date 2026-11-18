echo "== 首方客户端：合同上声明销售价格来源的两位客户各一台（access-identity；票 commercial-four-module-requirements/21） =="
# SYN-ACCOUNT-21 与 SYN-ACCOUNT-22 各绑一台合成货主系统（operator-channel/integration-dex.yaml 里的 syn-customer-client-21、
# -22），演示动线 demo-run/60-sell-price-sources.sh 用它们各自提交委托。一个首方客户端只代表一个客户账户，所以不能借 01 那一台。
# 授予只给这条动线用得到的四类：委托提交、送站预报提交、按包裹与按运单号查追踪。排在 488 之后，理由同那一节：种子按节新增。
# 证据只记 S。
"$BIN/parcel-access-register" customer-client-register -input "$SEEDS/access/39-customer-clients-sell-price.json"
"$BIN/parcel-access-register" customer-client-grant -input "$SEEDS/access/40-customer-client-grants-sell-price.json"
