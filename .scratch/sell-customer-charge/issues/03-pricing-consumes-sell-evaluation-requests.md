# 03 计价：接 SELL 评价请求，按包裹建快照形成 SELL·客户费用评价

Category: enhancement
Status: ready-for-agent——2026-10-19 Claude Code 会话立。
Blocked by: [01](./01-sell-request-identity-and-registers.md)
地盘：`internal/parcelpricing` 的 ports、application（请求受理门）、adapters/settlementaccounting、adapters/inbox、按包裹的解析器（从 adapters/transportfulfillment 现有解析器里拆）；`cmd/parcel-dispatch` 路由与装配。
出处：[ADR-0190](../../../docs/adr/0190-a-parcel-billing-moment-requests-a-sell-evaluation-that-forms-the-estimated-customer-charge.md) 决定四；ADR-0171；ADR-0179 决定三。

## 做什么

1. 请求受理门从「必须点名发生项」放宽到「发生项与包裹二者有一」。
2. 把现有解析器里「按已受理包裹取申报与邮编路线」那一半拆成按包裹的解析器，原解析器组合它。快照加结算币种，取结算账户币种。
3. SELL 请求翻译：方向 SELL、目的客户费用、计价范围与基准时点取自请求；选卡同受理前试算口径。复用现有的「由请求形成评价」编排（选卡、按回指幂等、入册、交付）。
4. inbox 消费者（名 `parcel-pricing/form-sell-evaluation-from-request`）与派发路由。

## 完成判据

- 计价侧用例：一份 SELL 请求形成一份评价并回指请求；未配置、适用冲突、输入不可得各一条。
- 真库装配：经生产路由表，一封 SELL 请求信封形成评价、发出评价已记录信封。
