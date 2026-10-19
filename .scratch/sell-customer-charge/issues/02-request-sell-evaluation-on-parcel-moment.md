# 02 结算：包裹计费时点到来时发起 SELL 评价请求

Category: enhancement
Status: ready-for-agent——2026-10-19 Claude Code 会话立。
Blocked by: [01](./01-sell-request-identity-and-registers.md)。02、03、04 都改 `cmd/parcel-dispatch` 的路由表，按 02 → 03 → 04 串行做。
地盘：`internal/settlementaccounting` 的 application、adapters/inbox、新目录 adapters/parcelshipment（若无）、adapters/partycommercial；`cmd/parcel-dispatch` 的路由表与装配。
出处：[ADR-0190](../../../docs/adr/0190-a-parcel-billing-moment-requests-a-sell-evaluation-that-forms-the-estimated-customer-charge.md) 决定二、三、六。

## 做什么

1. 编排 `RequestSellEvaluationOnParcelMoment`：
   - 收（租户、包裹、计费时点、来源事实引用、计价基准时点）。
   - 包裹到结算账户走受理前财务控制同一条链（小包托运接受时固定的商业解析 → 参与方与商业闭包的结算政策 → 结算账户册）。
   - 查采用册：没有这一户、或采用的不是这次到来的时点，答未配置，入账不重投。
   - 按自然键登记请求并同一事务交信封；重放答已存在。
2. 两只 inbox 消费者：
   - 收寄采用：`parcel-shipment.network-intake.recorded`。回读采用记录，不采用的略过；基准时点取责任起点。
   - 终局：`parcel-shipment.final-outcome.formed`。回读终局，只认已终局的「有效交付」；基准时点取生效时点。
3. 派发：这两种信封改成扇出，各路记各自的 inbox，各列自己的未决哨兵。未配置、不采用、非有效交付一律入账。

## 完成判据

- 应用层用例：没登记、时点不符、重放、两种时点各一条。
- 真库装配用例：经生产路由表，一封收寄采用信封让一户采用了 `NETWORK_INTAKE_ADOPTED` 的包裹落一份请求、交一封信封；没登记时入账、零行。
