echo "== 计价登记·完整售价卡（SYN-SCOPE-02：燃油 / 偏远 / 超尺寸 / 超重四项附加费 + 燃油序列 + 两本目录） =="
# 完整价卡放在自己的计价范围 SYN-SCOPE-02，不进 SYN-SCOPE-01：演示动线的评价按范围选卡，v1 售价卡（150 一节）照旧是动线上
# 唯一的一张，客户费用金额不因这一节而变。参考复核职责规则在 150 一节登记，复核都依赖它，所以本节排在它之后。
# 序列与目录同一条纪律（ADR-0099、ADR-0109）：先登记再复核，复核责任方与登记责任方不同，不复核就不在用。
# 燃油序列另登一条 SYN-SERIES-FUEL-FULL-01：150 一节那条的取值不是百分点，原因写在 seedgen 的 fullFuelSeriesSnapshot。
"$BIN/parcel-pricing-register" -kind reference-series -file "$SEEDS/pricing/reference-series-fuel-full.json"
"$BIN/parcel-pricing-register" -kind reference-series-review -file "$SEEDS/pricing/reference-series-fuel-full-review.json"
"$BIN/parcel-pricing-register" -kind reference-catalogue -file "$SEEDS/pricing/reference-catalogue-zone-cn-sg-full.json"
"$BIN/parcel-pricing-register" -kind reference-catalogue-review -file "$SEEDS/pricing/reference-catalogue-zone-cn-sg-full-review.json"
"$BIN/parcel-pricing-register" -kind reference-catalogue -file "$SEEDS/pricing/reference-catalogue-remote-tier-cn-sg.json"
"$BIN/parcel-pricing-register" -kind reference-catalogue-review -file "$SEEDS/pricing/reference-catalogue-remote-tier-cn-sg-review.json"
"$BIN/parcel-pricing-register" -kind price-card -file "$SEEDS/pricing/price-card-cn-sg-full.json"
