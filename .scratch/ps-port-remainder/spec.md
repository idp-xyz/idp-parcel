# PS 端口余口：三口的机制半边各能立什么

Category: chore
Status: draft——三张子票皆 draft（每口一次 `/domain-modeling` 裁完：机制半边的**头一半都落在 party-commercial**，PS 侧只剩消费适配器与一个阶段判断，都要等 PC 那一半先立；无一口今天能在 PS 单方面落代码），待 owner 答各票「要你答的问题」后转 ready-for-agent；通道 2 于 2026-09-07 按 MCP-1 派单 task-0c472fed 立

## 从哪里来

[completion-assessment-2026-09-04](../completion-assessment-2026-09-04.md)「剩余机制缺口」第 1 项：端口精确口径缺 12，其中 PS 三口记为「建模未决」——`ports.LabelValidityRuleView`、`ports.SourceDataRuleDeclaration`、`ports.SourceDataAmendmentAuthorizer`（`BD-PS-009` / `PAR-COM-13`）。三口今天在仓内**只有测试替身实现**（`grep JudgeLabelLapsed|DeclareSourceDataAmendment|AuthorizeSourceDataAmendment` 非测试命中只在 ports 与两处编排调用点），且两条消费编排都没有生产调用方：`NewAmendCustomerSourceDataHandler` 与 `NewJudgeLabelServiceFinalHandler` 在 `cmd/` 零命中。

## 三口一览

| 票 | 端口 | 一句话裁决 | 头一半归谁 |
|---|---|---|---|
| [01](issues/01-label-validity-rule-is-a-lapse-declaration-on-the-final-rule.md) | `LabelValidityRuleView` | 「接受时固定的有效期规则」是接单规则包版本下**面单服务终局规则**的一格声明（PC CONTEXT 把「接受时固定规则下的不可逆失效」写在终局规则那句里）；PS 只消费 | PC：`FinalRuleContent` 长一格有效期声明 + 读口 |
| [02](issues/02-source-data-amendment-allowance-is-a-third-stage-content-declaration.md) | `SourceDataRuleDeclaration` | 允许矩阵是接单规则包版本下的**第三族阶段内容声明**（照 ADR-0058：归拥有规则对象，产品与合同只采用）；PS 要先长出「资料修订阶段」这个词并把阶段带进查询 | PC：声明表族 + 读口；PS：阶段词条与阶段判断 |
| [03](issues/03-source-data-amendment-authorization-needs-a-pc-action-kind-and-a-decider.md) | `SourceDataAmendmentAuthorizer` | PC 授权动作封闭集没有「资料修订」这一格、裁定结果不带实际决定方（合同委派无执行器）；PS 适配器照撤回/主动拒绝那两只 | PC：动作格 + 委派→实际决定方；实例半边 `BD-PS-009` |

## 共同结论

- **不立新 ADR。** 三口的归属都由既有记录覆盖：ADR-0058（阶段内容声明按拥有规则对象归属）、ADR-0025（跨上下文适配器落消费方）、ADR-0062（采用版本从已接受解析回指）。PC 落各自正文时若改领域形状，是否记 ADR 归 PC owner（ADR-0104 的先例）。PS 预留的 `0118` 号本批不取。
- **不动硬句、不填任何取值。** 无声明/无规则/无动作格时三口各自停在既有的诚实格：不失效、`NotDeclared`→待复核、`AuthorizationRulesNotConfigured`→未决。
- **PS 侧今天没有可单独落地的代码。** 消费适配器没有可翻译的对象就是一层空壳（与 `auto-reroute-demo-reachability/02` 那次的判据同一条：绿着的替身看着像接线）。等 PC 半边落地后三张票各转 ready-for-agent，PS 适配器我可接。
- **顺带量到、不在本批**：资料修订编排没有生产入口（`BD-PS-009` 的机制半边——端点 + `UnconfiguredIntake{}` + 边界壳，形照 ADR-0106 决定四）。它不依赖三口，可另立票；列在 03 票末。

## 边界

本目录只写票面，不改 `docs/**`、不改 CONTEXT（提议的词条写在票里标「提议」）、不动 `internal/**`。PC 侧地盘当前归 MCP-3（pc-gaps/07 批），三票的 PC 半边由 MCP-1 派或 PC owner 认领，不在此越界。
