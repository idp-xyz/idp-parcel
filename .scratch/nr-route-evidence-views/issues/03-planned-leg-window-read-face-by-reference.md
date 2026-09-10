# 计划履约段引用与按引用取段窗口的窄读口（`network-routing` 侧，供 TF 派送时间窗口缝消费）

Category: enhancement
Status: resolved——2026-09-10 11:5x 通道 5 收口（同通道两任会话：前一任四笔中三笔成笔、一笔留现场后崩，本任按 git 现场接手成笔并验；分支 `mcp5-nr03` 基 `84e89dc7`，每笔已推 origin；完成记录见文末，进 main 记录归推送方）。此前：in-progress——2026-09-10 11:1x 通道 5 认领（单 task-a8cbfc83-3174-46be-b46c-5117d772d087），分支 `mcp5-nr03` 基 `84e89dc7`。此前：ready-for-agent——由 [ADR-0131](../../../docs/adr/0131-planned-leg-is-referenced-by-plan-version-and-ordinal-and-the-delivery-window-seam-answers-content-not-applicability.md) 决定一派生（2026-09-09，通道 5 代裁，task-8f6d8f94）；票 [tf-segment-lifecycle-closure/13](../../tf-segment-lifecycle-closure/issues/13-delivery-window-seam-network-routing.md) 以本票为阻塞边
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

## 完成记录（2026-09-10 11:5x，通道 5；分支 `mcp5-nr03` 基 `84e89dc7`，每笔已推 origin 的 SHA——推送方重放进 main）

| 笔 | SHA | 会话 | 内容 |
|---|---|---|---|
| ① | `c3f711df` | 通道 5 前一任 | 票面→in-progress |
| ② | `6edb83b6` | 通道 5 前一任 | 第 1 步：`domain.PlannedLegReference`（版本 + 序位）、`NewPlannedLegReference` 构造门拒空版本与非正序位、`String` / `ParsePlannedLegReference` 往返相等；`InitialRoutePlan.LegAt` 按序位取段、越界答没有不 panic；领域测试四例 |
| ③ | `4ace04d6` | 通道 5 本任成笔（内容为前一任 11:19–11:22 所写，未改一字） | 第 2–4 步：`ports.PlannedLegWindowRead`；postgres `PlannedLegWindows`（初始路由行 `plan` 列 → 复核行 `new_plan` 列两处、经 `rebuildPlan` 整份重建后 `LegAt` 取段）；真库五例；`ParsePlannedLegReference` 进 `production_wiring_baseline.txt` 一行 |
| ④ | `f32311ea` | 通道 5 本任 | 票面→resolved + 本记录初稿 |
| ⑤ | `90c0595c` | 通道 5 本任 | 机制清点在 `f32311ea` 干净 detached 检出重生成：networkrouting 生产 52→55 / 测试 48→50 / 端口 13→14，合计与端口声明数（369→370）随之而动；单独一笔，本笔单独作用于其父提交 |
| ⑥ | 本笔 | 通道 5 本任 | 本表补 ④⑤⑥ 与清点一句 |

**拼写形状（供票 13 的 TF 适配器逐字对）**：`<路由计划版本标识>#<序位>`，分隔符 `#`，例 `RPV-000000000007#2` 指该版本的第二段。**序位自首段起计、首段为 1**（不从 0 起）。解析从**最后一个** `#` 切（版本标识是自由字串、不禁止含 `#`；序位在末尾只由数字组成），序位只认 `strconv.Itoa` 写得出的形状——前导零、正号、空白一律拒，保证 `Parse(String(r)) == r` 且 `String(Parse(s)) == s`。TF 侧**只调 `domain.ParsePlannedLegReference` / `String`，不自己拼不自己切**。与 ADR-0131 措辞对照：ADR 只定「版本标识 + 段在段链中的序位（自首段起计）」、拼写由 NR 值类型一处定义，未定分隔符——分隔符由本票定为 `#`，与 ADR 无出入，无需回改 ADR；票 13 照此。

**触及**（8 件，`git diff --stat 84e89dc7..4ace04d6` +551/−1 再加本笔票面）：`internal/networkrouting/domain/{planned_leg_reference.go（新）,planned_leg_reference_test.go（新）,initial_route_plan.go（只加 `LegAt`）}`、`internal/networkrouting/ports/planned_leg_window_read.go`（新）、`internal/networkrouting/adapters/postgres/{planned_leg_window.go（新）,planned_leg_window_test.go（新）}`、`internal/architecture/production_wiring_baseline.txt`（network-routing 段新增一行 + 理由注）、本票面。**未碰**：`internal/transportfulfillment/**`；`PlanApplicability` 与 `InitialRouteStore` / `ReassessmentStore` 两个判断口的签名与语句；`InitialRoutePlan` 构造不变量（`FormInitialRoutePlan` 未动）；`PlannedLeg` 未加身份字段；无 HTTP 端点；ADR-0131 正文；无迁移。机制清点已在 `f32311ea` 干净检出重生成随 ⑤ 提（`f32311ea` 之前的完工报误写「未重生成」，以 ⑤ 为准；推送方在 tip 再兑一次照旧）。

**验收对照**（票面完成判据逐项）：`PlannedLegReference` 往返相等、越界序位与空版本被构造门拒 ✓（②，`planned_leg_reference_test.go`）；窄读口对初始计划与改路新计划两处的段都交回 `PlannedTimeWindow` 含 `Basis` ✓（③，`TestAPlannedLegWindowIsReadFromTheInitialPlanByReference` / `…FromTheReroutedNewPlanByReference`，后者同时钉「被替代版本的引用仍交回它自己那一段」即 ADR-0131 决定二）；版本不存在 / 序位越界 / 跨租户三情形 `found=false && err=nil` ✓（③ 三例）；`go build ./... && go vet ./...` 退 0 ✓；`internal/networkrouting/...` 带 DSN 全过 ✓；`internal/architecture/` 门禁绿 ✓（`ParsePlannedLegReference` 进基线并写理由，`NewPlannedLegReference` 按 `New` 前缀被门禁跳过，`LegAt` 是方法不在网内）；完成记录写清拼写与序位起点 ✓（上段）。边界三条 ✓。

**验证强度**（本任，隔离树 `D:/tops/idp-parcel-mcp5-nr03`，工作树与 ③ 提交内容逐字相同——四件全部入笔、`status --untracked-files=all` 空）：`gofmt -l ./internal/networkrouting` 零输出；`go build ./...`、`go vet ./internal/networkrouting/... ./internal/architecture/...` 退 0；**带 DSN** 探针 `go test ./internal/networkrouting/adapters/postgres/ -run TestPlannedLegWindowsAreInvisibleAcrossTenants -count=1 -v` → `--- PASS`（非 SKIP）；**带 DSN** `go test -p 1 -count=1 ./internal/networkrouting/... ./internal/architecture/... ./cmd/...` → 19 包 ok、0 FAIL（NR 七包 + ports 无测试、architecture、cmd 十一包），约 30 秒。未跑全量（作者范围口径，2026-09-08 裁定）、未跑 `-race`。前一任 11:2x 那轮 55432 占号未见释号广播、进程已不在，本任 11:4x 广播视为已释并重占一轮、跑完释号。

**与 main 碰面**（`git fetch` 后 `git merge-tree --write-tree origin/main mcp5-nr03`，origin/main = `6a387b89`，干跑未动树）：退 0 无冲突——main 自 `84e89dc7` 起只多一笔 tasks.md 簿记，两侧无同文件。

**判断题**（给评审与推送方，都不阻断）：

1. **复核行那条语句里的 `new_plan->>'version' = $2`** 与票面第 3 步「不写新的 jsonb 抠字段 SQL」怎么对：它**只用来定位行**，不抠任何字段交出去——`route_reassessment` 表没有新计划版本的平铺列（迁移 `0002` 里 `new_plan` 是整份 jsonb，版本标识只住在它里面），定位到的行仍整份过 `rebuildPlan` → `FormInitialRoutePlan` 构造门，重建后再核一次 `plan.Version() == 定位键`，键名即 `planRow` 的 json 标签、由读改路新计划的用例钉住。替代路是给复核表加一列 `new_plan_version` 再迁移，会动复核判断口的写语句与 `PlanApplicability` 地盘，超出本票「不动两个判断口」的边界；若评审判该加列，另立票。
2. **同版本多行响亮报错而不挑一行**（`planByVersion`）：版本由 `RouteIdentityFactory` 走 `route_plan_version_seq` 序列签发、租户内唯一，多行是唯一性被破坏，挑一行等于把两份计划的内容当一份交出去。
3. **`found=false` 不区分「版本不存在」与「属于另一个租户」**：端口头注已写明，ADR-0003 租户条件进每条语句，跨租户零行与不存在同形是有意的。
4. **接手方式**：本任无原会话记忆，按 git 现场（HEAD = origin = `6edb83b6`，四件未提交、mtime 停在 11:22:51、无 go 进程）接手；四件全文读过、未改一字、未读出与 ADR-0131 / 票面相反的语义。测试与实现同为前一任所写（作者常规写法，不是「先读实现再写镜像测试」那格）；本任没有另写自己的 red——评审请以代码为准，不以本记录为准。

## Comments

- 2026-09-09 · 通道 5（task-8f6d8f94，代裁 tf/13）：由 ADR-0131 决定一派生立票，只写票面，未动代码。
- 2026-09-10 11:4x · 通道 5 新会话接手（用户报全部会话 crash；前一任 11:13 认领、11:17 推第 1 步、11:19–11:22 写第 2–4 步四件后崩）：按 git 现场接、不另起；带 DSN 验绿后成笔 `4ace04d6`；完成记录见上。通道 1 11:4x 点名已应答「在做 nr/03」。
