# 面单交易：全仓没有这张表，缺的是领域建模不是接线

Category: feature
Status: in-progress
Session: MCP-5（2026-08-31 认领，重启范围见文末 Comments）
Blocked by: 无（但与本批其余各票性质不同，不并线）

## 为什么单列

其余十三张骨架页缺的是读面或数据，**这一张缺的是建模**。取证于 `65b6cf2`：全仓
`migrations/` 下搜不到任何 `label_transaction` 表，`parcel_shipment` 的十二张表里没有一张承载
面单交易（依次是 `acceptance_adopted_resolution`、`acceptance_financial_control`、
`acceptance_processing_attempt`、`acceptance_reachability_judgment`、
`commercial_resolution_key_registration`、`customer_source_data_version`、`final_outcome`、
`intake_adoption`、`parcel_cancellation`、`shipment_request`、`source_submission`、
`source_submission_observation`）。

所以这一票不是「补读面」，是从领域语言开始：域模型 → 迁移 → 写入方 → 读面 → 端点 → 页。
工作量与性质都与票 02–06 不是一回事，混进那三条并行线会把它们的完成判据搅浑。

## 已有的领域语言（出处在，形状未定）

`apps/admin-web/src/navigation.ts` 的 `moduleInfoById['label-transactions']` 指向
`docs/domain/parcel-shipment/CONTEXT.md`，其中已列出这些词：**面单交易**、**交易级与包裹级
渠道业务结果**、**面单交易定案**、**包裹级关闭或重开请求与决定**、**渠道角色与责任依据快照**。
词在，但没有任何代码或迁移实现它们。

## 为什么现在不派

三条理由，任一成立都够：

1. **难逆转取舍要走 ADR**。面单交易的聚合边界（交易级与包裹级如何分、定案后如何封口、关闭与
   重开的决定如何留痕）是结构性决定，按 AGENTS.md 要新 ADR，不能在实现票里顺手定。
2. **写入方在墙后面**。面单交易随委托提交产生，而接入渠道墙拦着上游——即便建完模，这一页
   仍是空册。它不比票 02–06 更急。
3. **地盘冲突**。`internal/parcelshipment/**` 此刻有他会话的在途未提交改动
   （`adapters/inbox/shipment_request_submitted_consumer.go`、
   `application/advance_acceptance_chain.go` 及其测试）。按 parallel-sessions，地盘要由人在开工
   时分派，越界前先在频道说一声、对方让位再动。

## 重启条件

上述三条各自解开：①一份定聚合边界的 ADR 已落；②承接会话由人指定且 `internal/parcelshipment/**`
的在途改动已落地或已让位。届时本票范围改写为完整一条从建模到接线的线。

## 参照

`docs/domain/parcel-shipment/CONTEXT.md`（面单交易一族术语）；
[本批 spec](../spec.md) 的事实基线；`docs/agents/parallel-sessions.md` 的地盘一节。

## Comments

- 2026-08-31 · MCP-5：**重启并认领。** 频道指示「这两个页面骨架（acceptance-review、
  label-transactions）完整实现」。三条不派理由逐一核销（基线 `4b35815`）：
  1. ADR——聚合边界 ADR 作为本票第一件交付先落（编号以 `docs/adr/README.md` 下一空位为准），
     不在实现里顺手定；
  2. 写入方在墙后——**维持成立，且首发基线明写「独立面单渠道服务不进入首发生产」**。因此本票
     范围按机制半边裁：域模型、迁移、仓储写入机制（构造与不变式由测试驱动）、读端口、真库读
     适配器、HTTP 查阅端点、隔离读准入（ADR-0078 读行）、页面接真。**不造任何伪写入方**：
     渠道墙未降前登记零行，页面如实呈现空册（同批 05/06 阶段二的先例）。写编排（随委托提交
     产生面单交易）等渠道墙降后另票；
  3. 地盘——`internal/parcelshipment/**` 的在途改动已随 `4b35815` 批收口落地，工作树此刻
     除他人 `docs/wooolink/`、`.scratch/admin-write-faces/` 外干净。已在频道广播认领
     `internal/parcelshipment/**`、`cmd/parcel-api/**`、`apps/admin-web/src/**`、
     `migrations/parcel_shipment/**` 与本批票面。
  同批新开 [09-acceptance-review-wiring](./09-acceptance-review-wiring.md) 承接另一页，两票
  分开提交。
