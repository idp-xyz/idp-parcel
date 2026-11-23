echo "== 计价登记：逐委托售价卡（票 07） =="
# 范围 SYN-SCOPE-SHIP-23，聚合方式逐委托。公共范围上的逐包裹卡不动。
# 排在 150 之后：这张卡绑的分区目录与燃油序列在那一节登记并复核。卡由 seedgen 生成，不手改 JSON。
"$BIN/parcel-pricing-register" -kind price-card -file "$SEEDS/pricing/price-card-cn-sg-shipment.json"
