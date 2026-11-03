echo "== 首方客户追踪按运单号查询的授予（access-identity；票 document-numbering/12，ADR-0356 决定一） =="
# 演示客户端（data/access/07 那一台）登一笔 CUSTOMER_SHIPMENT_TRACKING_QUERY：一口一词，按包裹查那一项（data/access/08）不放行这一口。
# 查询口不判准入范围（ADR-0139 决定八），所以没有对照与权威区间。另起一节而不并进 08，是因为种子按节新增、不改既有各节。证据只记 S。
"$BIN/parcel-access-register" customer-client-grant -input "$SEEDS/access/38-customer-client-grant-shipment-tracking.json"
