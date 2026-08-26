# 合规限制页收放行门禁：目录与认定两表

Category: enhancement
Status: ready-for-agent

自[票 04 的导航裁决](./04-registered-but-unreadable-rows-need-a-nav-ruling.md)。这一格连裁都不算裁——`customs-restrictions` 在 `moduleInfoById` 里的主责句结尾原文就是「放行门禁核对」，与 CONTEXT 116 的术语条同词。页面早认领过，只是没实现读面。

## 事实（实读代码，锚 `dc611a3`）

1. `gate_condition_catalog` / `gate_condition_finding`（0008）都带 `tenant_id`；主键含 `(scope_ref, action, boundary_ref)`，`action` 是封闭四值（`OUTBOUND_RELEASE`、`LOADING_DEPARTURE`、`CROSS_CUSTOMS_MOVEMENT`、`FINAL_DELIVERY`）——0008 自注：动作在主键里不是附属列，因为门禁判断绑定动作与边界、不能复用于其他动作（CONTEXT 硬句 216）。
2. 点读适配器 `GateConditionView.LoadPreconditionFindings` 已在，按 `(tenant, scope, action, boundary)` 取一份认定集，交回领域对象——为编排而设，不是目录上列。
3. 种子灌过一份门禁目录与一条已满足的门禁认定（`08-gate-catalog.json` / `09-gate-finding-met.json`）。

## 要做什么

与票 05 同形（同一个上下文、同一批共享文件），因此**两票串行、不要并行派**：

- `ports` 加伴生列表读端口，不拓宽 `LoadPreconditionFindings`。
- `adapters/postgres` 列表适配器，父子一条语句取回。
- `adapters/http` 查询处理器，复用既有两种 Intake。
- `cmd/parcel-api` 装配 + 放行面枚举加行。
- `CustomsRestrictionsPage` 接真 + `liveIds` 加一行。

## 两条形状约束

1. **目录未登记与登记了空清单必须分得开**，且两者含义与关闭义务那一格**相反**——0008 自注原话：门禁目录未登记 → 未决，没有清单的门禁判断无从复核；而登记了却空清单是「此动作在此边界本就不受门禁」的如实答案（领域折叠为「不适用」）。页面不能把这两态说成同一句话，也不能照抄票 05 的说法。
2. **认定的封闭三值刻意没有「未知」格**（0008 自注：判断不出来的前置条件根本不该进折叠）。读回集外取值即坏数据，上抛，不折成第四格。

页面上门禁按 `(scope, action, boundary)` 三元组成组显示；动作用 CONTEXT 原词的中文化说法，封闭四值进词表（照 `pages/party/presentation.ts` 的形状）。

## 完成标准

同票 05：`200` + 非空册、未启用准入答 `403`、真库测试钉住租户隔离与两态可分辨、全仓测试绿（含真库）、页面层 DOM 取证。
