# 45 管理台运输履约查阅页：没有运输准备结果区

Category: enhancement
Status: needs-triage——2026-10-19 Claude Code 会话立（用户「你作为业务和系统专家自决……解决所有的问题」授权），出自 [18](./18-transport-preparation-consumes-the-initial-route.md) 的完成记录。
Blocked by: [18](./18-transport-preparation-consumes-the-initial-route.md)（查阅口 `transport-preparation` 一册随它进 main）
地盘：`apps/admin-web/**`，照 [workflow.md「前端切片」](../../../docs/agents/workflow.md#前端切片一人在-main-上直接做)在 `main` 上直接做。
出处：方案「等 peer 的」admin-web 一行；[ADR-0201](../../../docs/adr/0201-schedule-templates-materialize-concrete-schedules-by-date-and-preparation-answers-not-configured-on-an-empty-register.md) 决定七与文末「补记：票 18 落地时的补充裁定」。

## 现象

票 18 进 main 之后，`GET /transport-fulfillment-records?registry=transport-preparation` 答 `TRANSPORT_PREPARATIONS_LISTED`，`preparations` 每行带段引用、包裹、规则版本、五值结论、方向与段窗口、需求量快照（来源、锚、逐维数量）、选中的班次与逐候选依据（候选的版本为 0 即只在评估时物化、没有登记；逐池的可用量、换成池单位的需求量与判词）。管理台运输履约查阅页没有这一区，演示提交之后看不到三段准备结果，只能直接调 API。

## 三类

机制（查阅页如实呈现结果册）。

## 做什么

- `records-api.ts`：册名加 `transport-preparation`，答案加 `TRANSPORT_PREPARATIONS_LISTED` 与行形。
- 新区：每行一段——段引用、包裹、结论（五值原词示文）、连接与起讫、段窗口、需求量逐维、选中的班次；候选依据可展开，标出未登记的那一班（要订舱或装载分配先登记它）与每一班出局或够不够的原因。
- 空册照页上「登记册为空」的分词呈现；需求量缺的那一维示「待补充」，不代填零。

## 完成判据

- 演示提交一单后，页上列出三段，首段示运输机会已准备并指出选中的那一班。
- 前端单测覆盖五值结论与候选依据的呈现。

## 规模

S
