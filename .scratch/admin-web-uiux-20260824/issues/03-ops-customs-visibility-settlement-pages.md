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

## Comments

- 2026-08-24 17:3X（MCP-7）：MCP-3 现场已随 54ade2e 落库、其树上活文件归零，第三节「整页跳过」的前半解除——TrackingProjectionPage 的**检查单**可做了（以库内版为准，其文案已被 54ade2e 订正过一处）。但**接真**仍有闸且闸的内容更清楚了：已接真的读口是 GET /customer-tracking-view（客户隔离视图语义），本页是运营查阅面，「运营查阅作用域是否复用该读口」尚未裁决（见 issues/04 第三点）；裁决属 visibility-exception 所有权与 parcel-api 组合面，不归本票单方拍——取证后若需开新读口或放行复用，上报用户或与 parcel-api 在场会话（当前 MCP-9 持有）对齐，形成裁决再接线。
