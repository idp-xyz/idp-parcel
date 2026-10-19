# 38 管理台运输履约查阅页：班次区「运行方向」列读空，也没有周期安排区

Category: enhancement
Status: needs-triage——2026-10-19 Claude Code 会话立（用户「你作为业务和系统专家自决……解决所有的问题」授权）。出自 [16](./16-transport-schedule-templates-and-concrete-schedules-are-registered.md) 的隔离评审。
Blocked by: [16](./16-transport-schedule-templates-and-concrete-schedules-are-registered.md)（查阅口的新行形与 `transport-schedule-template` 一册随它进 main）
地盘：`apps/admin-web/**`，照 [workflow.md「前端切片」](../../../docs/agents/workflow.md#前端切片一人在-main-上直接做)在 `main` 上直接做。
出处：[ADR-0201](../../../docs/adr/0201-schedule-templates-materialize-concrete-schedules-by-date-and-preparation-answers-not-configured-on-an-empty-register.md) 越权风险点「查阅口班次行换键」；方案「等 peer 的」admin-web 一行。

## 现象

票 16 进 main 之后，`GET /transport-fulfillment-records?registry=transport-schedule` 的行去掉了 `direction` 一键，方向写作 `connection`、`fromNode`、`toNode` 三格，另加 `version`、`arrivesAt`、`serviceScope` 与来历各键（`origin`、`template`、`templateVersion`、`runningDate`），只列当前版；查阅口新增 `registry=transport-schedule-template`（答 `SCHEDULE_TEMPLATES_LISTED`）。

管理台运输履约查阅页（`apps/admin-web/src/pages/operations/TransportFulfillmentReviewPage.tsx`）的班次区仍读 `record.direction`，「运行方向」列读空；`records-api.ts` 的 `TransportScheduleRecord` 还是旧形；页上没有周期安排区。

## 三类

机制（查阅页如实呈现登记册）。

## 做什么

- `records-api.ts`：`TransportScheduleRecord` 换成新行形；册名加 `transport-schedule-template`，答案加 `SCHEDULE_TEMPLATES_LISTED`。
- 班次区：「运行方向」一列换成连接、起点、终点三列，加到达时刻、服务范围、版本与来历（临时 / 出自哪份周期安排哪个运行日期）。
- 新周期安排区：代码、版本、连接与起讫、星期、当地出发时刻、历时、时区、服务范围、生效区间。星期按 MON…SUN 原词示文，不在集合内的词原样示文。
- 空册照页上「登记册为空」的分词呈现，不代填占位班次（ADR-0201 决定六）。

## 完成判据

- 三道门退 0。
- 灌过演示种子的库上，周期安排区列出三条 SYN 周期安排；显式登一个班次后班次区看得到连接与起讫三格、出处与运行日期。

## 规模

S
