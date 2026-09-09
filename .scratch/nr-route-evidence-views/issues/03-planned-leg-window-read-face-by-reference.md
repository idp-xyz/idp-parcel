# 计划履约段引用与按引用取段窗口的窄读口（`network-routing` 侧，供 TF 派送时间窗口缝消费）

Category: enhancement
Status: ready-for-agent——由 [ADR-0131](../../../docs/adr/0131-planned-leg-is-referenced-by-plan-version-and-ordinal-and-the-delivery-window-seam-answers-content-not-applicability.md) 决定一派生（2026-09-09，通道 5 代裁，task-8f6d8f94）；票 [tf-segment-lifecycle-closure/13](../../tf-segment-lifecycle-closure/issues/13-delivery-window-seam-network-routing.md) 以本票为阻塞边
Blocked by: 无

## 缺口

TF 的派送时间窗口缝要按**计划履约段引用**向 `network-routing` 取「这一段的计划时间窗口」（ADR-0131 决定一）。NR 今天没有这样的口：`InitialRouteStore` / `ReassessmentStore` 按完整判断键取单行（TF 不持有那把键），`RoutePlanCatalogueRead` 只上检索列面、头注明写不从 jsonb 抠字段冒充列。`PlanApplicabilityStore.FindByPlan` 已按计划版本取行，说明「计划版本」本就是对外可用的键——缺的是从版本到段内容那一跳。

领域里也还没有「计划履约段引用」这个值：`PlannedLeg` 是 `InitialRoutePlan.Legs()` 有序段链里的值，没有身份字段；ADR-0131 裁定引用 = 路由计划版本标识 + 段在段链中的序位，NR 不另铸段标识，拼写由 NR 一处定义。

## 做法（顺序固定）

1. **领域值**：`internal/networkrouting/domain` 加计划履约段引用值类型（暂名 `PlannedLegReference`：`RoutePlanVersionID` + 序位），构造门拒空版本与非正序位；`String()` 与解析函数往返相等（拼写只在这里定，分隔符由本票定，测试钉往返）。`InitialRoutePlan` 加按序位取段的访问器，序位越界答「没有」不 panic。序位自首段起计，与 CONTEXT「自首段起计」同口径。
2. **端口**：`internal/networkrouting/ports` 加窄读口（暂名 `PlannedLegWindowRead`）：`(ctx, tenant, reference) → (window domain.PlannedTimeWindow, found bool, err error)`。头注写清：交内容不交适用性（ADR-0131 决定二）；「没找到」是业务答案（版本不在本租户下、或序位越出该版本段链）不是错误；不是列表口，不拓宽既有判断口。
3. **postgres 适配器**：按引用先在本租户的初始路由判断行找 `plan_version = 版本` 的 `plan` 列，找不到再在本租户的复核判断行找 `new_plan` 列里同版本的计划（改路成的新版本住在那里，见 `route_reassessment.go` 自注「新计划归 new_plan 列独家拥有」）；两处都按各判断口现有的装载纪律重建 `InitialRoutePlan` 后取段，**不写新的 jsonb 抠字段 SQL**。租户条件进每条语句（ADR-0003）；跨租户零行答「没找到」。
4. **测试**（带 DSN，走 `pgtest`）：找到（初始计划的段 / 改路新计划的段各一例）、版本不存在、序位越界、跨租户零行、引用拼写往返；不写真实节点 / 时间窗取值，夹具全部合成。
5. **不做**：不加 HTTP 端点（消费方是 TF 进程内适配器）；不动 `PlanApplicability` 与两个判断口；不改 `InitialRoutePlan` 的构造不变量；不给 `PlannedLeg` 加身份字段。

## 完成判据

- `PlannedLegReference` 往返相等；越界序位与空版本被构造门拒。
- 窄读口对初始计划与改路新计划两处的段都能交回 `PlannedTimeWindow`（含 `Basis`）；版本不存在 / 序位越界 / 跨租户三种情形答 `found=false` 且 `err=nil`。
- `go build ./... && go vet ./...` 退 0；`internal/networkrouting/...` 带 DSN 全过；`internal/architecture/` 门禁绿（新导出有生产调用点或进基线，按 architecture 包现行规则）。
- 完成记录写清拼写形状（分隔符）与序位起点，供票 13 的 TF 适配器逐字对。

## 边界

- 只动 `internal/networkrouting/{domain,ports,adapters/postgres}` 与本票面；不动 `internal/transportfulfillment/**`（那是票 13）。
- 不改 ADR-0131 正文；拼写定下后若与 ADR 措辞有出入，在本票完成记录写一句并回票 13。
- 不写真实计划、节点、时间窗；`PAR-NET-14` 实例半边不进本票。

## Comments

- 2026-09-09 · 通道 5（task-8f6d8f94，代裁 tf/13）：由 ADR-0131 决定一派生立票，只写票面，未动代码。
