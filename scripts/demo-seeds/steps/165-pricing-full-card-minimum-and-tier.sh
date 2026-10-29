echo "== 计价登记·完整售价卡三（SYN-SCOPE-04：偏远一条规则按档位查价表 + 单票合计最低收费） =="
# 第三张完整价卡放在自己的计价范围 SYN-SCOPE-04，不进 SYN-SCOPE-01、SYN-SCOPE-02：评价按范围选卡，v1 售价卡与完整价卡一的评价不变。
# 它绑的分区目录、偏远档位目录与燃油序列就是 160 一节已登记并复核的那一批（价卡只绑标识不绑版本，ADR-0099、ADR-0109），
# 所以本节只登卡，排在 160 之后。
"$BIN/parcel-pricing-register" -kind price-card -file "$SEEDS/pricing/price-card-cn-sg-full-03.json"
