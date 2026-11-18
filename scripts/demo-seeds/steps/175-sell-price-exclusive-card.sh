echo "== 计价登记：专属售价卡只写谈下来的线路（票 commercial-four-module-requirements/21） =="
# 范围 SYN-SCOPE-EXCL-01。价表只有 Z1，价比公共卡低；分区目录与燃油序列都和公共卡绑同一份，Z1 与 Z2 都解得出。
# 没谈到的 Z2 不在运费表里。目录未决与运费表缺格是两件事：前者停在这一范围上，后者才问合同声明的下一个范围。
# 排在 150 之后：这张卡绑的分区目录与燃油序列在那一节登记并复核。卡由 seedgen 生成，不手改 JSON。
"$BIN/parcel-pricing-register" -kind price-card -file "$SEEDS/pricing/price-card-cn-sg-exclusive.json"
