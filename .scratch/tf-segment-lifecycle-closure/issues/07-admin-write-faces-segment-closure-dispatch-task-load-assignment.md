# admin 写面四格：关段、建派送任务、装载分配、明确终止参与

Category: enhancement
Status: ready-for-agent
Blocked by: 无

## 从哪里来

票 [03](03-parcel-api-wiring-for-segment-orchestrations.md) 决策简报第 6–8 行与第 5 行的终止一路
（MCP-3 2026-09-03 同意）：

| 编排 | 为什么是写面不是回传口 |
|---|---|
| `CloseFulfillmentSegment` | 「不再接受新对象」是运营决定，由人做（CONTEXT 生命周期④） |
| `OpenDispatchTask` | UC-TF-006 触发之一「授权角色建立派送任务」 |
| `FormLoadAssignment` | UC-TF-003/004 运输准备：分配本体在消耗之前、由人定 |
| `EndFulfillmentParticipation`（终止一路） | 它指向本上下文之外的处置决定，依据只能由调用方给 |

写面沿 ADR-0085：Intake 接口 + 处理器接口 + 封闭响应形状，与既有命令端点同款，装配以字面量
`UnconfiguredIntake{}` 起步；写准入不另立形（ADR-0085 决定二）。

## 要做什么

### HTTP 适配器（`internal/transportfulfillment/adapters/http/`，新文件）

路径照实说动词，不套 `-registrations`：这四件里只有派送任务与装载分配是「登记一个新东西」，
关段与终止是往已有对象上落一个决定。

| 路径 | 构造函数 | 编排 | 201 那一格 |
|---|---|---|---|
| `POST /transport-fulfillment-segment-closures` | `NewCloseFulfillmentSegmentEndpoint` | `CloseFulfillmentSegmentHandler.Close` | `SEGMENT_CLOSED` |
| `POST /transport-fulfillment-dispatch-task-registrations` | `NewOpenDispatchTaskEndpoint` | `OpenDispatchTaskHandler.Open` | `DISPATCH_TASK_OPENED` |
| `POST /transport-fulfillment-load-assignment-registrations` | `NewFormLoadAssignmentEndpoint` | `FormLoadAssignmentHandler.Form` | `LOAD_ASSIGNMENT_FORMED` |
| `POST /transport-fulfillment-participation-terminations` | `NewTerminateFulfillmentParticipationEndpoint` | `EndFulfillmentParticipationHandler.End`（`Source` 固定为 `ParticipationEndedByTermination`） | `PARTICIPATION_ENDED` |

路径前缀取读面册名 `transport-fulfillment-`（与 `/transport-fulfillment-records` 同源），不取
控制事实那组的 `/transport-fulfillment/...`——两组是两种口：一组是接入方回传事实，一组是运营写决定。

**终止口的 Intake 只能铸终止那一路**：`Source` 由端点固定，不从请求体收。交付一路与交接一路是
内部触发（票 06），从这个口进来就是让接入方替 TF 做步骤 7；`NextSegment` 在终止一路也收不下
（`carriesControlOnward`），Intake 不给那一格。

结果代数逐条映 ADR-0022（映法同票 04）。关段的 `SEGMENT_STILL_ACTIVE` 与 `INPUT_NOT_ACCEPTED`
两格、结束参与的四格（`PARTICIPATION_ALREADY_ENDED` / `OBJECT_NOT_IN_SEGMENT` / `SEGMENT_NOT_FOUND` /
`CONTROL_FACT_NOT_FOUND`）各自成格，续办动作两两不同，传输层不合并。

### 端点表、占位、装配

- `endpoints.go` 四行，字面量 `UnconfiguredIntake{}`；`UnconfiguredIntake` 补四个 Intake 方法。
- `unwired_orchestration.go` 四个占位类型分立（方法同名 `Handle`/命令类型不同，一个类型装不下）。
- 装配 `assemble_segment_operations.go`：
  - 关段：`tfpostgres.NewFulfillmentSegments`。
  - 派送任务：`NewDispatchTasks`、时钟。
  - 装载分配：`NewLoadAssignments`、时钟。
  - 终止：`NewFulfillmentSegments`、`NewTransportHandovers`、`NewEffectiveDeliveries`、时钟
    （`EndFulfillmentParticipationDeps` 四缝全接真——终止一路不读后两个，但 Deps 是一份）。
  - 事务包装照 `transactionalDelivery`。
- 装配测试真库实跑：立段 → 进一个对象 → 关段答 `SEGMENT_STILL_ACTIVE` → 终止那条参与 → 关段答
  `SEGMENT_CLOSED` → 重放答 `SEGMENT_ALREADY_CLOSED`。这一串把票 01 的三条完工判据第一次在装配点
  上串起来走一遍。

## 边界与未做

- **不自动关段**——本票建的是让人做那个决定的口，不是替人做。
- 简报第 7 行的另一半「到达事实 → 内部触发建派送任务」不在本票（编排间触发，归属待 MCP-3 裁，
  见票 03 Comments）。
- 管理台表单页（ADR-0085 决定三）不在本票：页接真次序照两阶段纪律，上下文侧交活后由装配点持有者
  广播，页另立票。
- 不改既有导出签名；不碰 TF domain / ports / application / adapters/postgres。

## 完工判据

同票 04：四个端点在表上、未配置即拒在前、装配测试含真库 PASS 且那一串关段序列走通、专钉测试
红过再绿、临时 worktree 全绿、提交带 pathspec、向 MCP-3 报 SHA 与验证种类。
