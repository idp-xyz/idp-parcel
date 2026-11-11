# 07 票级销售价卡登得进、用不上——销售计费请求按包裹发起，没有任何生产路径形成票级评价

Category: enhancement
Status: draft
父票：[spec](../spec.md)
Blocked by: 票 [06](./06-shipment-level-weight-aggregation-lands-in-the-domain-and-the-import-template.md)。Q2 已由会话按行业做法代答、待用户认可（见 [spec](../spec.md)「待用户确认」）。开工时随票立一篇 ADR：[ADR-0190](../../../docs/adr/0190-a-parcel-billing-moment-requests-a-sell-evaluation-that-forms-the-estimated-customer-charge.md) 的销售评价请求是按包裹的，本票要扩出委托一路，照 `docs/adr/README.md` 顶部要挂指针
地盘：`internal/settlementaccounting`、`internal/parcelpricing`（输入解析与适配器）、`internal/parcelshipment`（读口）、`cmd/parcel-dispatch` 装配

对应要求：Rating「分票计费」的生产一半。

## 现象

取证钉 `11bfa1ffb`。

- 导入模板能登记聚合方式为逐委托的价卡（`AggregationPerShipment`）。
- 销售评价请求按包裹：`SellEvaluationRequestSpec` 带的是 `Parcel`，ADR-0190 决定一的自然键是租户、包裹与计费时点。
- 票级评价输入的构造函数 `NewAggregatePricingInputSnapshot` 在非测试代码里没有调用方。
- 票级卡只收委托主体（`AggregationMode` 的 `admits`），包裹主体的请求用不了它。
- 受理前估价是逐件试算再合计（[ADR-0179](../../../docs/adr/0179-pre-acceptance-control-amount-is-estimated-by-settlement-from-per-parcel-sell-trial-evaluations.md)）。

后果：租户登了一张票级销售价卡，这个范围上的委托要么等不到评价，要么只能改登逐包裹的卡，按件各算，金额与按整票算的不同。

## 要做的

下面是建议形态，由随票的 ADR 定。

1. **委托级销售评价请求**：请求身份、自然键与计费时点。
2. **票级输入组装**：成员清单、合计量或逐件实重与尺寸（声明还是实测，照 [ADR-0171](../../../docs/adr/0171-pricing-input-resolver-uses-declaration-until-a-measurement-port-exists.md) 的取源）、邮编路线。
3. **票级客户费用**：主要计费范围是委托。
4. **受理前估价**：账户采用的计价范围上是票级卡时，按委托整票试算，不逐件合计。
5. **选哪一路由价卡定**：范围上在用的销售卡是逐包裹的走今天的路，是逐委托的走新路；同一范围两种并存照旧是适用冲突。

## 要裁的

- **计费时点。** 一份委托下各件包裹收寄的时刻不同。是首件收寄、全部收寄，还是另有一格。ADR-0190 当初不采用「委托已接受」的理由（服务还没发生、收寄前可取消）在这里同样要过一遍。
- **成员变了怎么办。** 费用形成之后其中一件被取消、拆分或补充，票级费用怎么失效与重算。
- **一件收寄、其余迟迟不到。** 费用停在哪一格，会不会永远不形成。
- **实测与申报混着来。** 一部分件有节点实测、一部分只有申报时，合计怎么取。

## 三类

请求、输入与费用的形态是机制。内置哪几种委托级计费时点是产品策略。一个结算账户采用哪一种是租户取值，没登记答未配置。

## 完成判据

- 演示租户登一张逐委托的销售卡，一份多件委托走到计费时点，形成一份票级评价与一笔票级客户费用；有真库用例。
- 同一份委托不会既按件又按票各收一遍。
- 没采用委托级计费时点的账户答未配置。
- `UC-SA-002` 与 `UC-PP-001` 同步，AT 号取空号。

## 不在本票

- 把票级金额摊到每件包裹看毛利。那是成本分摊（ADR-0111 决定三），仍在冻结；对客户收一笔票级费用本身不需要它。
- 采购一侧多成员的运输收费发生项。它归票 [14](./14-buy-side-multi-piece-charge-occurrences-cannot-be-priced.md)。
- 主单级计费。它等运输履约的承运总单登记册。
