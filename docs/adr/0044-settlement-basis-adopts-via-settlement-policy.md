# ADR-0044: 结算政策依据经政策采用，结果带回预付/账期方式与适用范围

Status: Accepted  
Date: 2026-08-12

## Context

UC-PC-002 交接：「结算政策结果必须携带解析得到的预付/账期方式和明确适用范围，调用方不得预选方式，也不能退化成客户布尔字段。」

领域层已有 `SettlementPolicy` / `ResolveSettlementPolicy`（AT-PC-031/032 绿），但解析与闭包对 `SettlementPolicyObject` 走通用「按范围选版本」路径：结果只剩一份 `CommercialVersion`，方式与六维适用范围不可观察——与 [ADR-0034](./0034-pricing-closure-adopts-via-price-policy.md) 落地前价格规则的形状相同。

另有一处不能照抄 0034 的坑：结算解析按 CONTEXT 要六维唯一（法人、相对方、合同、费用范围、币种、期间），而键上只有粗粒度 scope。若闭包级只按 scope 数候选，同一货主在**不同费用范围**上的预付与账期政策会被误判冲突，直接撞 AT-PC-031。

## Decision

**一、不新设 `SettlementPurpose`。** 结算政策本就是接受闭包的成员依据（合同引用它）；方式是**输出**不是输入，无需像价格方向那样成为目的维度。触发按 `RequiredBasis == SettlementPolicyObject`。

**二、键上新增 `SettlementSelector`（相对方、合同版本、费用范围、币种）。** 纪律与 `PriceDirection` 相同：请求结算依据必填，其余请求必缺，部分给出即`输入未受理`。法人取键上 `LegalEntityCandidate`，时点取锚点。

**三、成功路径经 `ResolveSettlementPolicy` 采用。** 登记册增设结算政策通道（`RegisterSettlementPolicy`，按政策版本的租户+范围收窄候选）；零候选→`无适用依据`，同一精确范围多候选→`适用冲突`（即使结论一致，AT-PC-032 行为不变）；不同费用范围互不冲突（AT-PC-031 保留）。光有版本没有政策 = 该范围没有可用结算依据。

**四、结果携带政策。** `Resolution.AdoptedSettlementPolicy` 与闭包 `AdoptedBasis.SettlementPolicy` 交回方式（PREPAID/TERMS）与完整 `SettlementApplicability`；结算政策参与 `ViewRevision` 派生，只改政策不动版本时解析身份仍变。

## Consequences

- SA 消费侧适配器可观察方式+范围，不再拿裸版本自行猜测。
- 闭包夹具凡请求结算依据须给选择器并登记政策；解析键指纹随选择器扩展。

## Alternatives considered

- **闭包级按 scope 数候选（照抄 0034 粗粒度）。** 否决：不同费用范围的预付+账期会被误判冲突，撞 AT-PC-031。
- **新设 SettlementPurpose。** 否决：方式是解析输出；结算政策与合同、规则包同属接受闭包成员，拆目的会逼消费方跑第二次闭包。
- **方式塞进 CommercialVersion。** 否决：与 0034 同理——通用版本解析产不出结构化依据，正是要修的洞。

## Links

- [ADR-0034](./0034-pricing-closure-adopts-via-price-policy.md)：镜像先例
- [UC-PC-002](../application/party-commercial/UC-PC-002-RESOLVE-COMMERCIAL-BASIS.md)：`AT-PC-031`/`AT-PC-032`
