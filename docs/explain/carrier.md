# 导读：代理商、Carrier 与 Carrier Service 怎么区分

本文是导读，不是权威来源。角色定义、最小字段和验收情形以[代理商、Carrier 与 Carrier Service 关系开发交接](../design/party-carrier-channel-relationship-development-handoff.md)为准，领域语言以[参与方与商业上下文](../domain/party-commercial/CONTEXT.md)、[运输履约上下文](../domain/transport-fulfillment/CONTEXT.md)和[统一领域语言](../domain/GLOSSARY.md)为准。两处表述不一致时以那几份为准，并回来修本文。

当前设计通过“两条链、六个独立角色”解决这个问题，核心是：**代理商关系、Carrier Service 的销售渠道、实际运输事实不能合并成一个 `carrier_id`。**

**两条业务链**

```text
商业渠道链
我方 ServiceProduct
  → ProductChannelMapping
  → ChannelProduct（Carrier Service）
  → ChannelServiceProvider（Carrier / 代理商 / 转售商 / 聚合平台）

实际履约链
收寄 / 交接 / 取得运输控制的证据
  → ActualFulfillmentSegment
  → ActualCarrier
```

### 三个核心概念

| 概念 | 当前设计含义 |
|---|---|
| `Carrier` | 一个中性的业务参与方，可能在某笔交易中是渠道服务方、底层承运商、实际承运商或责任承担方 |
| 代理商 | Carrier 的代理、授权或转售关系，是有范围、依据和有效期的交易角色，不是企业永久类型 |
| `Carrier Service` | 外部可下单、购买或取得面单的服务定义，领域内规范名称是“渠道产品” |

也就是说，Carrier Service 不是实际承运行为，也不是我方销售给客户的服务产品。当前定义见[参与方与商业上下文](../domain/party-commercial/CONTEXT.md)。

### 一笔交易分别保存六个角色

```text
channelProductId                 使用了哪个 Carrier Service
channelServiceProviderPartyId    谁直接接收我们的下单请求
channelAccountHolderPartyId      使用的渠道账号归谁
contractCounterpartyPartyId      谁与我方发生合同、账单和结算
underlyingCarrierPartyId         渠道明确声明的底层 Carrier
actualCarrierPartyId             实际接收并运输货物的 Carrier
responsiblePartyId               针对明确责任范围由谁承担责任
```

这些角色可以是同一家企业，但必须分别记录，不能相互推导。规则已经明确写在[参与方与商业上下文](../domain/party-commercial/CONTEXT.md)和[统一领域语言](../domain/GLOSSARY.md)，交接文档的[角色关系矩阵](../design/party-carrier-channel-relationship-development-handoff.md#角色关系矩阵)逐个列出了每个角色的权威上下文和禁止推导。

### 代理商场景示例

假设：

- 我方产品：`欧洲经济小包`
- 代理商 A 提供渠道产品：`Carrier B Economy`
- 实际使用我方在代理商 A 开设的账号
- 我方与代理商 A 签约并向 A 结算
- 代理商声明底层网络是 Carrier B
- 包裹最终由 Carrier B 收寄，尾程由 Carrier C 派送

交易记录应为：

```text
服务产品                 = 欧洲经济小包
渠道产品                 = A / Carrier B Economy
渠道服务方               = 代理商 A
渠道账号持有人           = 我方责任法人
合同与结算相对方         = 代理商 A
底层承运商               = Carrier B
实际承运商（干线段）     = Carrier B
实际承运商（尾程段）     = Carrier C
```

因此：

- 采购价和应付可以依据与代理商 A 的协议形成。
- 代理商 A 不因为收钱就成为实际承运商。
- Carrier B 不因为显示在面单上就自动成为所有履约段的实际承运商。
- Carrier C 可以是实际尾程承运商，但不因此成为我方结算相对方。

实际承运商只能由收寄、交接和履约证据确认；代理商接口成功、订舱成功、面单品牌或单号格式都不够。证据不足时保持“待确认”，见[运输履约上下文](../domain/transport-fulfillment/CONTEXT.md)。

### 客户自有账号

如果使用客户自有渠道账号：

- 账号持有人是客户；
- 渠道服务方仍可能是 Carrier 或代理商；
- 我方只是获得范围化账号使用授权；
- 客户与渠道之间的采购、退款和应付不会自动成为我方账务；
- 只有明确合同规定我方承担责任时，才形成我方责任。

### 当前完成度

这套关系在领域文档和业务场景中已经闭合，尤其覆盖了直营 Carrier、Carrier 代理商、转售商和聚合平台四类渠道。角色矩阵与最小开发字段已由[交接文档](../design/party-carrier-channel-relationship-development-handoff.md)冻结，工作包为 `PCR-W01` 至 `PCR-W05`。

当前仍是领域设计与开发契约，尚未形成生产级 `party-commercial` 实体、接口和数据库模型；真实伙伴、协议、账号和渠道实例待参数化。准确的状态与完成定义以交接文档为准，本节不另记一套进度。