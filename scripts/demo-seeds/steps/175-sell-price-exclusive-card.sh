echo "== 计价登记：专属售价卡只写谈下来的线路（票 commercial-four-module-requirements/21） =="
# 范围 SYN-SCOPE-EXCL-01。价表只有 Z1；分区目录仍是 SYN-CAT-ZONE-CN-SG，Z1 与 Z2 都解得出。
# 没谈到的 Z2 不在运费表里，目录未决与运费表缺格是两件事：前者停在这一范围上，后者才问下一个范围。
# 不绑燃油序列：探针先看运费表、后补序列。绑了序列时，没谈到的分区会在补序列之前停住，问不到后一个范围。
# 偏远附加是卡上的固定金额，不靠序列。
"$BIN/parcel-pricing-register" -kind price-card -file "$SEEDS/pricing/price-card-cn-sg-exclusive.json"
