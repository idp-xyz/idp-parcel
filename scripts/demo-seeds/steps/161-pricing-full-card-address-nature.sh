echo "== 计价登记·第二张完整售价卡（SYN-SCOPE-03：超重费按计价重量触发、另带按地址性质收的住宅派送费） =="
# 第二张完整价卡放在自己的计价范围 SYN-SCOPE-03，与 SYN-SCOPE-01（演示动线）、SYN-SCOPE-02（第一张完整价卡）都不重叠：
# 评价按范围选卡，既有动线与第一张卡的评价结果不因这一节而变。它绑的两本目录与燃油序列都是 160 一节登记并复核的那几份
# （价卡绑的是目录与序列的标识，不绑版本），所以本节只登卡本身，排在 160 之后。
"$BIN/parcel-pricing-register" -kind price-card -file "$SEEDS/pricing/price-card-cn-sg-full-02.json"
