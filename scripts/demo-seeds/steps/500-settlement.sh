echo "== 结算账户与受理前估价采用登记（settlement-accounting：演示租户货主的应收结算账户与它采用的估价形态；票 routing-first-cut/16、product-strategy-boundary/26） =="
# 受理前财务控制的作用域从那一次商业解析回显的结算政策派生，账户按（法人、相对方、应收、币种、结算政策）
# 在结算账户登记册上找（cmd/parcel-dispatch 的 acceptanceFinancialControl）。这一行的五维照发布批里
# `SYN-SETTLEMENT-PREPAID-01` 的正文配，差一维就查不到，控制停在 `CONTROL_SCOPE_NOT_CONFIGURED`。账户标识与
# 责任依据是演示租户的合成取值：登记口与执行器都不代拟账户，没登的租户照旧停在那一格。
"$BIN/parcel-settlement-register" settlement-account \
  -input "$SEEDS/settlement/01-settlement-account-shipper-01-receivable-cny.json"
# 账户级预收（ADR-0182 决定五）：第一张对账单之前没有可映射的对账单，收款映射到应收账户本身。
# 金额是这一件演示委托的合成事实，按售价卡粗算（1.50kg、Z1：首重 ¥55 + 两档续重 ¥36，再加约一成燃油）
# 后留出余量到 200.00 CNY；不够盖住试算合计时只调这一笔，不写进生产默认。
"$BIN/parcel-settlement-register" external-funds-fact \
  -input "$SEEDS/settlement/03-external-funds-receipt-account-01.json"
"$BIN/parcel-settlement-register" funds-mapping \
  -input "$SEEDS/settlement/04-funds-mapping-account-01.json"
# 受理前估价采用（ADR-0179 决定五）：这一户采用「逐件 SELL 试算合计」，计价范围与售价卡 SYN-PLAN-CN-SG-01 的
# 范围同值——估价按（租户、计价范围、SELL、客户费用、判断时点）解析在用售价卡（决定三），范围对不上就解不出卡。
# 采用册对账户册设外键，所以排在账户之后。没登这一行的户，控制停在金额未配置；形态与范围是演示租户的合成取值。
"$BIN/parcel-settlement-register" pre-acceptance-estimate-adoption \
  -input "$SEEDS/settlement/02-pre-acceptance-estimate-adoption-account-01.json"
# SELL 计费时点采用（ADR-0190 决定三）：这一户在「有效网络收寄已采用」那一拍发起 SELL 评价，计价范围与受理前估价
# 采用的同值，估价与随后的预估客户费用因此选同一张卡（ADR-0190 越权风险点 9）。没登这一行的户，收寄采用之后不计费。
"$BIN/parcel-settlement-register" sell-billing-moment-adoption \
  -input "$SEEDS/settlement/05-sell-billing-moment-adoption-account-01.json"
# 费用项目映射（ADR-0190 决定五）：售价卡出的基础运费与偏远附加都映到同一个合成费用项目——一份评价只映一个费用
# 项目才形成，把附加并进运费是 SYN 简化（ADR-0190 越权风险点 2）。
"$BIN/parcel-settlement-register" fee-item-mapping \
  -input "$SEEDS/settlement/06-fee-item-mapping-base-freight.json"
"$BIN/parcel-settlement-register" fee-item-mapping \
  -input "$SEEDS/settlement/07-fee-item-mapping-remote-area-surcharge.json"
# BUY 触发（ADR-0164、ADR-0208）：干线段的实际履约发生项一形成就发起 BUY 评价，费用项目记合成的干线成本项。只登这一个
# 原因——演示先走的是实际履约；失败尝试、订舱、取消不登，形成了也答未配置。主要范围不在这一行，取干线协议的商业范围。
"$BIN/parcel-settlement-register" buy-evaluation-trigger \
  -input "$SEEDS/settlement/08-buy-evaluation-trigger-actual-fulfillment.json"
