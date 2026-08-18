# ADR-0057: 价格政策册保全发布期邻接答复，不重查也不绕过绑定校验

Status: Accepted  
Date: 2026-08-18

## Context

`NewCommercialPricePolicy` 除政策自身的方向、方案引用、范围与区间外，还收 `planDirection` 与 `conversion` 两个入参，交给 `checkPlanBinding` 判完即弃——两者都不在 `CommercialPricePolicy` 结构体上。

后果是：**不把这两项和政策一起保全，装载口重建不出政策**，因为构造函数要它们。族 A 其它三册的正文都在结构体上，装载按列填回去即可；价格政策是唯一的例外。

这两项不是本上下文推断出来的字段。`planDirection` 是发布当时 `parcel-pricing` 对该方案自身方向的答复。本上下文只持方案引用，没有资格自己去查价卡——与 [ADR-0034](./0034-pricing-closure-adopts-via-price-policy.md) 把 `PricingPlanStandingLookup` 设为入参而非查询的纪律是同一条。[UC-PC-001](../application/party-commercial/UC-PC-001-MAINTAIN-AND-PUBLISH-COMMERCIAL-AUTHORITY.md) 步骤 1 要求保全来源、来源版本、内容摘要、请求方、批准依据；发布期邻接答复属于来源保全，不是给结构体补字段。

装载面是生产解析每次都要走的路。`AT-PC-033`「不把 BUY 价卡隐式当 SELL 价卡」若只在发布面守、装载面绕过 `checkPlanBinding`，守卫就降级成一次写入时的检查，读出来的政策不再经过那道门。

## Decision

**一、价格政策册存 `plan_direction` 与 `binding_conversion`。** 它们是发布当时邻接上下文给出的答复与当时声明的转换，随政策正文一起落行。装载时原样交回 `NewCommercialPricePolicy`，`checkPlanBinding` 每次装载重跑一遍。

**二、不新增一条绕过绑定校验的重建门。** 不为价格政策开 `RehydratePricePolicy`。装载走与发布同一道 `NewCommercialPricePolicy`；绑定不成立则整次装载失败，不把坏行跳过装成一份看起来正常、绑定却未校验的政策。

**三、本上下文不在装载时向 `parcel-pricing` 重查方案方向。** 重查得到的是此刻的答复，不是发布当时保全下来的那一份；价卡方向若在期间被改写，装载会按新答复重判，等于用邻接上下文此刻的状态去改写本上下文已经发布的政策。发布期答复一经保全，装载只回放。

**四、这两列参与同键内容判定。** 结构体上看不见它们，但同一价格规则版本改挂另一种方案方向或另一种转换是另一次发布内容。同四元键下这两列任一不同，持久化面答`内容冲突`，不覆盖。

## Consequences

- `SavePricePolicy` 在签名上显式收 `planDirection` 与 `conversion`：调用方必须交出发布当时的答复，忘了就编不过。写入前用这两项与政策正文重跑 `NewCommercialPricePolicy`；库内 CHECK 再镜像同一份绑定矩阵，已知无法重建的行写不进去。
- 封闭集 CHECK 与 `PriceDirection` / `PlanBindingConversion` 同步扩展；集外取值整次装载上抛。
- [ADR-0034](./0034-pricing-closure-adopts-via-price-policy.md) 的选用与缺席语义不变；本记录只定登记册存什么。

## Alternatives considered

- **不存这两列，装载时向 parcel-pricing 重查方案方向。** 否决：见 Decision 三。那是用此刻的邻接状态改写已发布政策，且本上下文没有资格发起那次查询。
- **开 `RehydratePricePolicy`，跳过 `checkPlanBinding`。** 否决：AT-PC-033 的守卫降级成只在发布面有效，而装载面才是每次解析都走的路。
- **把两列补进 `CommercialPricePolicy` 结构体。** 否决：结构体表达的是选用时要观察的正文（方向、方案、范围、区间）；邻接答复是发布期证据，不是选用结果的一部分。补进结构体会让解析路径开始依赖一份它从不读取的字段。

## Links

- [ADR-0034](./0034-pricing-closure-adopts-via-price-policy.md)：价格政策采用与 `PricingPlanStandingLookup` 为入参的纪律
- [UC-PC-001](../application/party-commercial/UC-PC-001-MAINTAIN-AND-PUBLISH-COMMERCIAL-AUTHORITY.md)：步骤 1 来源保全；`AT-PC-033`
