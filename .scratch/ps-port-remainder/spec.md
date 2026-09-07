# PS 端口余口：三口的机制半边各能立什么

Category: chore
Status: in-progress——十问已由 MCP-1 代裁（owner 授权，2026-09-07，逐票 Comments）：01、03 转 blocked 等 PC 半边（pc-gaps 批，MCP-3）；04（资料修订生产入口机制半边）已 resolved（通道 2，分支 `mcp2-ps-ports`：`38c2aa82` + `a1a1d16f` + `baff5fdd`）；02 的 PS 半边中不依赖 PC 的那段由通道 2 按 task-d5558bc6（接管 task-b77525c9）在同一分支实施中。此前 draft（通道 2 于 2026-09-07 按 task-0c472fed 立三票：每口一次 `/domain-modeling`，机制半边的头一半都在 PC，PS 单方面无可落代码）

## 从哪里来

[completion-assessment-2026-09-04](../completion-assessment-2026-09-04.md)「剩余机制缺口」第 1 项：端口精确口径缺 12，其中 PS 三口记为「建模未决」——`ports.LabelValidityRuleView`、`ports.SourceDataRuleDeclaration`、`ports.SourceDataAmendmentAuthorizer`（`BD-PS-009` / `PAR-COM-13`）。三口今天在仓内**只有测试替身实现**（`grep JudgeLabelLapsed|DeclareSourceDataAmendment|AuthorizeSourceDataAmendment` 非测试命中只在 ports 与两处编排调用点），且两条消费编排都没有生产调用方：`NewAmendCustomerSourceDataHandler` 与 `NewJudgeLabelServiceFinalHandler` 在 `cmd/` 零命中。

## 三口一览

| 票 | 端口 | 一句话裁决 | 头一半归谁 |
|---|---|---|---|
| [01](issues/01-label-validity-rule-is-a-lapse-declaration-on-the-final-rule.md) | `LabelValidityRuleView` | 「接受时固定的有效期规则」是接单规则包版本下**面单服务终局规则**的一格声明（PC CONTEXT 把「接受时固定规则下的不可逆失效」写在终局规则那句里）；PS 只消费 | PC：`FinalRuleContent` 长一格有效期声明 + 读口 |
| [02](issues/02-source-data-amendment-allowance-is-a-third-stage-content-declaration.md) | `SourceDataRuleDeclaration` | 允许矩阵是接单规则包版本下的**第三族阶段内容声明**（照 ADR-0058：归拥有规则对象，产品与合同只采用）；PS 要先长出「资料修订阶段」这个词并把阶段带进查询 | PC：声明表族 + 读口；PS：阶段词条与阶段判断 |
| [03](issues/03-source-data-amendment-authorization-needs-a-pc-action-kind-and-a-decider.md) | `SourceDataAmendmentAuthorizer` | PC 授权动作封闭集没有「资料修订」这一格、裁定结果不带实际决定方（合同委派无执行器）；PS 适配器照撤回/主动拒绝那两只 | PC：动作格 + 委派→实际决定方；实例半边 `BD-PS-009` |
| [04](issues/04-amendment-production-entry-mechanism-half.md) | （入口，不是端口） | 资料修订编排的生产入口机制半边：端点 + `UnconfiguredIntake{}` + 两层边界壳，接上后编排如实停在授权未决 / 矩阵未登记 | PS（已 resolved，通道 2；清点里两口出缺口名单是未配置适配器被计为实现，PC 半边仍开） |

## 共同结论

- **三口本身不立新 ADR。** 三口的归属都由既有记录覆盖：ADR-0058（阶段内容声明按拥有规则对象归属）、ADR-0025（跨上下文适配器落消费方）、ADR-0062（采用版本从已接受解析回指）。PC 落各自正文时若改领域形状，是否记 ADR 归 PC owner（ADR-0104 的先例）。原写「PS 预留的 `0118` 号本批不取」，02-Q4 代裁改了：CC→PS、NO→PS 两条新消费箭头（阶段事实进 PS）要 ADR，取 `0118`——见下一条与 02 票 Comments。
- **不动硬句、不填任何取值。** 无声明/无规则/无动作格时三口各自停在既有的诚实格：不失效、`NotDeclared`→待复核、`AuthorizationRulesNotConfigured`→未决。
- **PS 侧对三口本身今天没有可单独落地的代码。** 消费适配器没有可翻译的对象就是一层空壳（与 `auto-reroute-demo-reachability/02` 那次的判据同一条：绿着的替身看着像接线）。等 PC 半边落地后 01、03 与 02 的适配器段转 ready-for-agent，PS 适配器我可接。
- **不依赖 PC 而先做的两件**（代裁后）：02 的「资料修订阶段」词条 + `Stage` 维 + 阶段判断（CC/NO 输入缝走消费侧读口，ADR-0118）；04 的生产入口机制半边。

## 边界

本目录只写票面，不改 `docs/**`、不改 CONTEXT（提议的词条写在票里标「提议」）、不动 `internal/**`。PC 侧地盘当前归 MCP-3（pc-gaps/07 批），三票的 PC 半边由 MCP-1 派或 PC owner 认领，不在此越界。
