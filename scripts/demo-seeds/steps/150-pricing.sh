echo "== 计价登记（parcel-pricing：价卡 + 参考序列 + 参考复核职责规则 + 序列复核 + 参考目录） =="
"$BIN/parcel-pricing-register" -kind price-card -file "$SEEDS/pricing/price-card-cn-sg.json"
"$BIN/parcel-pricing-register" -kind price-card -file "$SEEDS/pricing/price-card-cn-sg-cost.json"
"$BIN/parcel-pricing-register" -kind reference-series -file "$SEEDS/pricing/reference-series-fuel.json"
"$BIN/parcel-pricing-register" -kind reference-series -file "$SEEDS/pricing/reference-series-fx-cny-sgd.json"
# 人工复核按租户的参考复核职责规则判，没登时一律答未配置（ADR-0262）：演示租户在任何复核之前登一条要求不同主体、
# 不要求授予的规则。
"$BIN/parcel-pricing-register" -kind reference-review-duty-rule -file "$SEEDS/pricing/reference-review-duty-rule.json"
# 序列版本经复核通过才在用（ADR-0099）：先登记再复核，复核责任方与登记责任方不同。
"$BIN/parcel-pricing-register" -kind reference-series-review -file "$SEEDS/pricing/reference-series-fuel-review.json"
"$BIN/parcel-pricing-register" -kind reference-series-review -file "$SEEDS/pricing/reference-series-fx-cny-sgd-review.json"
# 参考目录同一条纪律（ADR-0109）：先登记再复核。售价卡绑这本目录，分区由委托申报的邮编路线经它解析。
"$BIN/parcel-pricing-register" -kind reference-catalogue -file "$SEEDS/pricing/reference-catalogue-zone-cn-sg.json"
"$BIN/parcel-pricing-register" -kind reference-catalogue-review -file "$SEEDS/pricing/reference-catalogue-zone-cn-sg-review.json"
