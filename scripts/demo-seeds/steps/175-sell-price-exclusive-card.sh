echo "== 计价登记：专属售价卡只写谈下来的线路（票 commercial-four-module-requirements/21） =="
# 范围 SYN-SCOPE-EXCL-01。价表只有 Z1；分区目录仍是 SYN-CAT-ZONE-CN-SG，Z1 与 Z2 都解得出。
# 没谈到的 Z2 不在运费表里，目录未决与运费表缺格是两件事：前者停在这一范围上，后者才问下一个范围。
# 不指名服务产品，与 SYN-SCOPE-01 上那张公共卡同一选法。排在 150 之后：目录与燃油序列已经复核在用。
"$BIN/parcel-pricing-register" -kind price-card -file "$SEEDS/pricing/price-card-cn-sg-exclusive.json"
