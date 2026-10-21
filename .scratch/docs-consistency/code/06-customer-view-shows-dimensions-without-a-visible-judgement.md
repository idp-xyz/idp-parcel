# 06 VE 客户视图的 ETA 维在没有任何客户可见 ETA 判断时也答 `SHOWN`，里程碑维同病较轻

Category: bug
Status: ready-for-agent——R1（上线前必修）：对客答错。客户视图在生产路径上，而 ETA 形成编排 `NewFormETAHandler` 在 `cmd/` 下没有调用方，生产上从不形成 ETA；披露策略只要把 ETA 维登成展示（演示策略对 `SYN-ACCOUNT-01` 正是如此），客户第一天读到的就是「ETA：展示」，背后没有任何 ETA。没有绕法（租户只能把 ETA 维整个登成不展示）。2026-10-21 文档自洽 02 立（用户「你作为业务和系统专家自决」授权），分支 claude-dc02。
Blocked by: 无
出处：[audit/ve.md](../audit/ve.md) VE-02（审计钉 `6b0074d0`；2026-10-21 按 main `67438a0f` 复核，现象仍在）。

## 现象

客户视图的 ETA 一维只翻译披露策略的答复，不看有没有获准对客展示的 ETA 判断。终局一维有 `domain.FinalDimensionOver` 按投影兜底，ETA 一维没有。里程碑一维：投影里只有未归类条目时（例如只有初始路由那一份），策略照样答 `SHOWN`。

## 证据

- 文档：
  - VE CONTEXT「客户可见性与通知」：「客户全程追踪视图只基于当前有效的全程追踪投影、获准对客展示的 ETA、`parcel-shipment` 提供的客户服务终局，以及已经批准披露的客户可见异常形成。」
  - Lifecycles「客户全程追踪视图」：「某一维信息待确认或不可披露 → 只对该维返回待确认、暂不可用或不展示，不虚构里程碑、ETA、终局或异常说明。」
  - UC-VE-008 主流程第 4 步：「只有达到适用质量与合同门槛的 ETA 才进入视图」；`AT-VE-158`：「不展示 ETA，也不使用计划或承诺时间填充」。
- 代码与配置：
  - `internal/visibilityexception/application/derive_customer_view.go` 的 `dimensionsFrom`：`eta, err := dimensionFrom(answer.ETA)`，终局一维才过 `domain.FinalDimensionOver(final, projection)`。头注自承「ETA 一维要的是 `UC-VE-003` 的对客可见判断（`AT-VE-158`），那份判断不在投影里，不在这里补」。
  - `NewFormETAHandler` 在 `cmd/` 下无调用方。
  - 演示策略 `scripts/demo-seeds/data/visibility/07-disclosure-policy-v1.json` 与 `14-disclosure-policy-v2.json` 给 `SYN-ACCOUNT-01` 的 `eta` 都是 `SHOWN` / `SYN-VE-CONTENT-ETA-BAND-V1`。
  - HTTP 响应体 `dimensionBody` 只带 `contentRef`，不带 ETA 值。

## 三类

机制：维度派生按「有没有可见判断」兜底。产品策略：无。租户取值：披露策略照旧由租户登，不在本票改。

## 规模

S–M：ETA 维加一道与 `FinalDimensionOver` 同形的兜底（读 UC-VE-003 的对客可见判断；读不到即待确认）；里程碑维在投影没有已归类条目时答待确认；补 `AT-VE-158` 的用例。

## 最小的诚实修法

1. ETA 维：没有当前有效的客户可见 ETA 判断，就答待确认，不答展示。今天生产上没有 ETA，等于 ETA 维一律待确认——这是如实的答案。
2. 里程碑维：投影里没有已归类为标准追踪里程碑的条目时答待确认。
3. 两处都只收窄展示，不放宽：策略答不展示的维照旧不展示。

## 要裁的

- `contentRef` 在 UC-VE-008 结果语义里指什么（策略允许展示的内容集，不是内容本身）——属文档，交修订票 docs-consistency/04 写明。
- ETA 维兜底读 UC-VE-003 判断的端口形状：从 ETA 库按（包裹，里程碑）取当前对客可见的那一版。依赖 [07](./07-eta-store-overwrites-history-and-mixes-sources.md) 修好 ETA 存储之后读法才稳；在那之前读不到即待确认，本票不必等 07。
