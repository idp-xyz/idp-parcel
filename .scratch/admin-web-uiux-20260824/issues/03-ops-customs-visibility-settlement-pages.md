# 03 作业/关务/追踪/结算/代收区 10+2 页与追踪接真（MCP-9）

Category: enhancement
Status: ready-for-agent

地盘：`apps/admin-web/src/pages/operations/**`、`pages/customs/**`、`pages/visibility/**`、`pages/settlement/**`、`pages/collection/**`。通用检查单与协作纪律见 ../spec.md。

## 一、现有 10 页按检查单过

- operations（2）：node-operations-review → docs/domain/node-operations/CONTEXT.md；transport-fulfillment-review → docs/domain/transport-fulfillment/CONTEXT.md
- customs（2）：customs-cases、customs-restrictions → docs/domain/customs-compliance/CONTEXT.md
- visibility（此轮 2 页）：exception-cases、claims-recovery → docs/domain/visibility-exception/CONTEXT.md（tracking-projection 整页跳过，理由见第三节）
- settlement（2）：charges-billing、operating-metrics → docs/domain/settlement-accounting/CONTEXT.md
- collection（1）：cod-ledger → docs/domain/CONTEXT-MAP.md 的 collection-remittance 一节

## 二、补齐两个规划占位页（落 pages/customs/）

- customs-ports-paths「口岸与申报路径」：出处 CONTEXT-MAP 的 customs-compliance ↔ network-routing（合规候选区域、口岸、申报路径、限制及解除）；
- compliance-rules「合规规则库」：出处 customs-compliance CONTEXT.md 规则化合规判断（规则版本、依据、决定方式）。

列定义从出处小节取词；用 ListPageTemplate 现有 props；未配置态如实；主责与出处从 moduleInfoById 只读导入（两个 id 已登记在 navigation.ts）。经 customs/index.ts 导出后**把导出名 send_to_session 7**——page-registry.tsx 登记由 7 号落，不在你地盘。

## 三、追踪视图接真（本轮作废，改续办）

**TrackingProjectionPage.tsx 此轮整页跳过（检查单也不做）**：MCP-3 于 2026-08-24 17:00 前后广播，该文件上留有其未提交活现场（hash 路由轮的文案订正），且其通道无提交授权、现场落库时点未定。现场落库前任何人改这页都会与之纠缠（parallel-sessions.md「未提交现场」）。

续办条件：MCP-3 广播该文件已落库后，按下述原案另行续办（届时先按**已提交状态**读 `git show HEAD:cmd/parcel-api/endpoints.go` 取证 VE 追踪视图查询端点形状；形状清楚才接真，四态完整、403 语义按 ADR-0022 不吞；liveIds 登记送 7 号；存疑则只写取证结论进 Comments）。

## 完成

同 02：票面 Status: resolved + 提交 SHA 与验证结果；report_task 标 done；send_to_session 7 简报。
