# 19 财务控制未形成时消费方整笔回滚：任务与可达性判断都不落，动线上看不出停在哪一格

Category: enhancement
Status: needs-triage——2026-10-11 通道 1 立（用户授权自决），出自 [16](16-demo-tenant-pre-acceptance-financial-control-cells.md) 判据二的取证与其「评审 ← 通道 5」Spec 非阻断
地盘：待分诊。候选：`internal/parcelshipment/application` 的 `AdvanceFinancialControlJudgmentHandler`（续办不带 `assessment.Reason`）与 `cmd/parcel-dispatch` 里消费方未决的处置；另含两处带日期的文档补记（见「现象」末条）。
出处：16「分诊裁定」第 4 条（「要不要立票，留到下一次走动线时定」）；16 完成记录判据二的三行对照表；16 Comments「评审 ← 通道 5」与通道 1 处置。

## 现象（取自 16 的完成记录与评审，钉 `f84515c2`；证据层级 `S`，立票时未另跑）

- 原样种子（停在时点格）：`acceptance_processing_attempt` 落一行 `FINANCIAL_CONTROL_AS_OF_NOT_CONFIGURED` / `OPERATOR_REGISTRATION`，可达性判断也落一行。
- 越过时点之后（16 的「只改时点」与「两格都登」两次）：dispatch 三次投递都是 `dispatch.consumer_undecided`，`stage FINANCIAL_CONTROL_JUDGMENT, reason FINANCIAL_CONTROL_NOT_FORMED`；未决整笔回滚，任务行与可达性判断都是零行。
- 所以 16 分诊裁定第 4 条的前提「作用域、金额两停在任务上都记 `FINANCIAL_CONTROL_NOT_FORMED`」不成立：任务上什么都不落。作用域停与金额停在适配器里各有原因码，到 `AdvanceFinancialControlJudgmentHandler` 都折成 `FINANCIAL_CONTROL_NOT_FORMED`；16 只能靠未入库的探针分出停在哪一格。
- 停点前移之后，两处带日期的文档补记写的仍是 16 之前的状态：[开发主线](../../../docs/product/PARCEL-NETWORK-FIRST-RELEASE-DEVELOPMENT-BASELINE.md) 2026-09-30 补记（票 psb/06 第 1 项）末句「演示种子的可达性格采用了这一形态，财务控制格没有」；[合成演示动线](../../../docs/design/synthetic-demo-journey-script.md)「墙三」下 2026-10-10 补记（票 rfc/11）写的停点 `FINANCIAL_CONTROL_AS_OF_NOT_CONFIGURED`。

## 待分诊

- 消费方未决整笔回滚出自哪条规则（先找 CONTEXT 或 ADR 的出处）；若是有意为之，「看不出停在哪」靠什么补——任务尝试留痕、只进日志，还是读面另答。
- `assessment.Reason` 要不要带进续办，让作用域停与金额停在任务上分得开。
- 那两处文档补记怎么跟上：补一条新的带日期补记，还是归某张已有的文档票。
- 与 [psb/06](../../product-strategy-boundary/issues/06-ps-acceptance-and-label-selection-judgment-methods.md) 第 2 项（`Amounts` 的估价方法）的先后：越过格 5 之前，演示动线都停在这一格。
