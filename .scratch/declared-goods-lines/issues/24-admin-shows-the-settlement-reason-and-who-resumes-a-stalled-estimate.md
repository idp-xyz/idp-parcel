# 24 管理台：估价停因带结算原因与服务产品，写明谁来续办

Category: enhancement
Status: ready-for-agent
Blocked by: 无（字段名在票 23 里钉死，可并行；联调等 23）

依据：[ADR-0380](../../../docs/adr/0380-an-estimate-the-price-card-side-cannot-form-waits-for-operator-registration-and-the-attempt-records-why.md) 决定五；票 21 的停因译法（`shipment-request/stall-reasons.ts`）。

## 要做的

- 运单详情「受理进度」与「受理前财务控制」两处：`lastAttemptSettlementReason` 在场时，用它译出细原因。服务产品在场时一并写，例如「估价没形成：没有在用价卡（服务产品 SYN-PROD-…）」。
- 按 `lastAttemptContinuation`（续办方）写谁来动手：
  - 等运营登记写「在价卡登记里登记后自动续办」，能链就链到价卡目录；
  - 内部重试写「系统会自动重试」；
  - 等客户补充照旧。
- 细原因的码复用票 21 已有的 `SA-` 译表：去掉前缀查同一张表，不另立一份；对照 Go 源的契约测试照旧。

## 完成判据

三道门通过；纯函数单测覆盖带与不带细原因、带与不带产品、三种续办方。

## 不做的

后端（票 23）。
