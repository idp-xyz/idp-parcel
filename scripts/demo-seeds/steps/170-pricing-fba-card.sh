echo "== 计价登记·FBA 头程售价卡（SYN-SCOPE-01：指名 SYN-PROD-CN-SG-FBA，逐箱按公斤计价） =="
# 与 150 一节那张不声明产品的售价卡同在 SYN-SCOPE-01（票 declared-goods-lines/17，ADR-0378）：选卡指名优先——申报 FBA 头程
# 产品的委托由这张卡计价，其余产品照旧由那一张计价，同一范围两张卡不成适用冲突。它绑 150 一节已登记并复核的分区目录
# （只绑标识），所以排在 150 之后；服务产品本身在 116 一节发布，计价只引标识不解析，不依赖那一节的先后。
# 这张卡带适用服务产品，写规范化版本 PPC-7，登记要部署开关（ADR-0352）。同 165 一节，只对这一次调用设开关。
IDP_PARCEL_PRICING_REGISTER_NEWER_CANONICALIZATION=true \
  "$BIN/parcel-pricing-register" -kind price-card -file "$SEEDS/pricing/price-card-cn-sg-fba.json"
