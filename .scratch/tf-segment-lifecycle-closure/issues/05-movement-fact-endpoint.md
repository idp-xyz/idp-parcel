# 移动事实端点：只收自营执行方

Category: enhancement
Status: resolved——2026-09-03 MCP-4，代码一笔 `4c48bac`（若并回前再 rebase，以票 Comments 末行为准），验证见 Comments
Blocked by: 无

## 从哪里来

票 [03](03-parcel-api-wiring-for-segment-orchestrations.md) 决策简报第 4 行（MCP-3 2026-09-03 同意）：
`RecordMovementFact` 对应 UC-TF-005「执行方/伙伴接入：接收出发、移动、到达、中断」，**外部端点，
只收自营执行方**。外部承运轨迹**不走它**——那一路走 `TrackingSource` 入站口的采纳执行器
（`label-channel-service-first-release/15` 已落端口、`16` 在做）。两条来源两个口，别让 HTTP 端点
顺手替轨迹采纳做判断。

## 要做什么

### HTTP 适配器（`internal/transportfulfillment/adapters/http/record_movement_fact.go`，新文件）

| 路径 | 构造函数 | Intake / Handler | 编排 |
|---|---|---|---|
| `POST /transport-fulfillment/movement-facts` | `NewRecordMovementFactEndpoint` | `MovementFactIntake` / `MovementFactHandler.Record` | `RecordMovementFactHandler.Record` |

结果代数逐条映 ADR-0022（映法同票 04，不复述）：`MOVEMENT_FACT_RECORDED` 201；
`MOVEMENT_FACT_VERSION_EXISTS`、`DEPARTURE_GATE_BLOCKED`、`INPUT_NOT_ACCEPTED`、`MOVEMENT_FACT_UNDECIDED`
（带 `continuationReference`）200；编排 `error` 5xx `NO_ANSWER_FORMED`。

`DEPARTURE_GATE_BLOCKED` 与 `INPUT_NOT_ACCEPTED` 是两格不是一格：前者去取放行结果（门禁判断属
customs-compliance），后者去改输入——编排把它们分开的理由在 `MovementFactOutcome` 自注，传输层
不许合回去。

响应透出 record 里的事实引用、班次、种类、版本与发生时刻；`GateClearance` 只在出发那一格有意义，
响应照实透出不做判断。

### 端点表、占位、装配

- `endpoints.go` 一行，字面量 `tfhttp.UnconfiguredIntake{}`；`UnconfiguredIntake` 补 `MovementFactIntake` 方法。
- `unwired_orchestration.go`：`unwiredMovementFact`。
- 装配：`buildMovementFactOrchestration(db)`，缝两条——`tfpostgres.NewMovementFacts`、`systemClock{}`；
  事务包装照 `transactionalDelivery`。装配测试真库实跑：登记一条到达事实、重放答 `MOVEMENT_FACT_VERSION_EXISTS`。

## 端点测试要钉的

- Intake 交出的命令（含 `GateRequired` / `GateClearance`）原样到达 Handler。
- 通用纪律同票 04 第 3 条。
- 端点文件头注释写明：**外部轨迹不从这里进**。这句是产品判断的落点，不写在代码里就只活在票面上。

## 边界与未做

- 「自营执行方」与「外部伙伴」的来源判定属 Intake 认证结果（`PAR-INT-01` 实例半边），本票只立
  未配置格，不写任何判定逻辑。
- 简报第 7 行「到达事实 → 内部触发建派送任务」不在本票：那是编排间触发，归内部触发那一族
  （见票 03 Comments 的归属待裁项）。
- 不改既有导出签名；不碰 TF domain / ports / application / adapters/postgres。

## 完工判据

同票 04：端点在表上、未配置即拒在前、装配测试含真库 PASS、专钉测试红过再绿、临时 worktree 全绿、
提交带 pathspec、向 MCP-3 报 SHA 与验证种类。

## Comments

2026-09-03 · MCP-4 完成记录（隔离 worktree `mcp4-tf03`）：

- 代码一笔：`record_movement_fact.go`（Intake / Handler / 端点 / 封闭响应）、`UnconfiguredIntake` 补格、
  端点表一行一参、`unwiredMovementFact`、`assemble_movement_fact.go`（登记册 + 时钟，无意图交付）、
  `main.go` 交入、探针表一行、真库装配测试。
- 专钉的红绿：端点构造函数与探针行各红过一次；`gateRequired`/`gateClearance` 到编排那条以门禁那道领域门
  被触到为判据（无放行 → `DEPARTURE_GATE_BLOCKED`，带放行 → 201 透出放行），不是命令镜像。
- 「外部轨迹不从这里进」写在端点文件头与端点表行注释两处。
- 验证：全仓 gofmt / build / vet / `go test -count=1 ./...` 在最终 SHA 的干净检出上跑，含真库（PASS 非 SKIP）。
- 机制清点已重生成（TF 生产 +1 / 测试 +1 / http +1 / 端点 +1）。

未做：简报第 7 行「到达事实 → 建派送任务」的内部触发半边（归属待 MCP-3 裁，见票 03 Comments）。
