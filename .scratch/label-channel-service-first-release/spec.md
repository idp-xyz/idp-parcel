# 面单渠道服务首发机制半边

Category: feature
Status: draft——目录与本 spec 已建，子票待 MCP-1 据两份盘点立

## 这个 feature 是什么

[ADR-0088](../../docs/adr/0088-label-channel-service-enters-the-first-release-service-forms.md) 把面单渠道服务从「首发范围裁剪掉的形态」改为**首发对客销售的独立服务形态**。范围落文已完成（PILOT-SCOPE、参数登记册 `PAR-COM-04`/`PAR-COM-12`/`PAR-INT-02`、验收矩阵 `CPS-06`/`CPS-07`、PC CONTEXT 首发范围硬句、`UC-PC-001`/`UC-PC-002` 验收行、[渠道适配缝备忘](../../docs/design/channel-adapter-seams-design-note.md)的范围口径都已随之修订）。

**范围落文不等于机制就位。** ADR-0088 的 Consequences 列出了「机制半边要补的能力形状」，并明说本 ADR *不预判各项在代码中的现状*，盘点归本目录。本 feature 承载的就是那次盘点及其派生的工程票。

盘点产物两份，均为只读、不改产品代码。**两份都尚未写出**，下列链接暂为悬空：

- [末端面单渠道能力形状盘点](./capability-shape-inventory.md)——客户尾程流「下单 → 取末端渠道面单 → 面单 PDF 返回/修改 → 轨迹回传 → 结算」这条链在主线上今天有哪些形状、缺哪些。
- [轨迹源接收缝盘点](./tracking-source-seam-inventory.md)——外部轨迹源（17track 与承运商直连）「拉取/接收 → 事实 → 里程碑映射 → 内部投影/客户可见」这条链同样逐段量。

## 边界

**在本 feature 范围内**：机制半边。端口形状、聚合与状态、执行器（应用层用例）、装配接线、迁移与读面——凡是不需要真实租户参数就能立起来的骨架。

**不在本 feature 范围内**：

- **实例半边的任何取值。** 渠道产品与账号（`PAR-INT-02`）、逐份渠道报价表及其价表族/体积系数/进位分段（`PAR-SET-03`）、对客价卡（`PAR-COM-10`）、面单服务终局规则版本（`PAR-COM-05`/`PAR-COM-07`）——本 feature 一个都不填，也不写技术默认值顶替。ADR-0088 Decision 五：机制里不内置任何渠道字段名、体积系数或金额。
- **采购过程。** 不向渠道询价、竞价、授予。PILOT-SCOPE 两条既有约束原样适用于面单渠道服务。
- **对客户的价格承诺。** 渠道候选择优是运营企业内部成本决策，首发只按成本单维（`PAR-NET-16`）。
- **领域语言的新造与改写。** ADR-0088 Decision 二：面单渠道服务按 PC/PS CONTEXT 既有定义进入首发，首发范围的变化只发生在产品文档。要改领域语言得先回 `CONTEXT.md`，走 [AGENTS.md 的「改文档」](../../AGENTS.md#改文档)。
- **范围决定本身。** 已由用户裁定并落 ADR-0088，本 feature 不重开。
- **网络服务主链路的试点约束。** 一条线路、一个锚点客户、三阶段执行与 `PN-08` 治理不变；面单渠道服务按同一 `PN-08` 治理走，不另设阶段模型。

## 与既有票族的关系

- [渠道适配缝备忘](../../docs/design/channel-adapter-seams-design-note.md)（票 `product-story-and-demo/02` 的产物）已经把六类渠道数据各指到所有权上下文与既有缝位，并写明「本文是缝位登记，不是工作包」，触发条件是「能力范围判据点名真实商业模式时按本文缝位立票」。**ADR-0088 就是那次点名**，因此本 feature 的票按备忘的缝位落，不重做缝位判断。
- [`tenant-implementation-01/requirement-mapping.md`](../tenant-implementation-01/requirement-mapping.md) 尾程流表的「待建」项是本次盘点的起点；该表锚在 `2ab7f89`，两份盘点重取了证据，判定以盘点为准。

## 子票

暂无。立票归 MCP-1：两份盘点各段末尾给了建议的票粒度，是建议不是票面。

盘点建议按「一类数据一张票、落在所有权上下文的 `adapters/` 或 `application/` 下」切分（渠道适配缝备忘的消费方式）；具体编号、阻塞边与 `Status:` 由 MCP-1 按 [issue-tracker 约定](../../docs/agents/issue-tracker.md) 落。

## Comments

- 2026-09-02 MCP-6：建目录与本 spec。本目录此前不存在而 ADR-0088 已引它作为盘点归处，这次补上那处悬空引用。取证基线 `origin/main` = `8c32d78`。未立任何子票，未改 `docs/**`，未改任何既有票面。
- 2026-09-02 MCP-1：MCP-6 在写完 spec 后随第三次并行崩溃停摆，两份盘点未开始；原 Comment 自称「并出两份只读盘点」与事实不符，已改。本 spec 原样取进主线是为了保住它，不是为了宣布盘点已办——盘点重派 MCP-6。
