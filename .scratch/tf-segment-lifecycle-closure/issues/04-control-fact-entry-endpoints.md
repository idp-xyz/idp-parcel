# 控制事实入口：交接与揽收的 HTTP 端点、装配与进段带段引用

Category: enhancement
Status: in-progress——MCP-4 2026-09-03 认领，在隔离 worktree 里做 TDD，共享树上不留 red 中间态
Blocked by: 无

## 从哪里来

票 [03](03-parcel-api-wiring-for-segment-orchestrations.md) 决策简报第 1–3 行（MCP-3 2026-09-03 逐行同意）：
`RegisterTransportHandover`（含 `Correct`）、`RegisterOffsitePickup`、`PerformOffsitePickup` 三条是
CONTEXT 成立边界的来源事实，**外部端点，第一优先**。它们是关键路径：接上之前，进段那道门
（`enterFulfillmentSegment`）在生产上没有任何路走到。

端点表**按事实分**（裁决 2）：交接一组（登记 + 更正）、揽收一组（单对象登记 + 多对象执行）。

## 要做什么

三层，全部沿 `parcel-api-remaining-endpoint-wiring/02`（TF 交付端点）那一笔的形状，不另发明。

### 一、HTTP 适配器（`internal/transportfulfillment/adapters/http/`，新文件）

每个入口一份 Intake 接口 + 一份 Handler 接口 + 一个封闭响应形状，照 `register_effective_delivery.go`：

| 路径 | 构造函数 | Intake / Handler | 编排 |
|---|---|---|---|
| `POST /transport-fulfillment/handovers` | `NewRegisterTransportHandoverEndpoint` | `HandoverIntake.IntakeRegistration` / `HandoverHandler.Register` | `RegisterTransportHandoverHandler.Register` |
| `POST /transport-fulfillment/handover-corrections` | `NewCorrectTransportHandoverEndpoint` | `HandoverIntake.IntakeCorrection` / `HandoverHandler.Correct` | `RegisterTransportHandoverHandler.Correct` |
| `POST /transport-fulfillment/offsite-pickups` | `NewRegisterOffsitePickupEndpoint` | `PickupRegistrationIntake` / `PickupRegistrationHandler.Register` | `RegisterOffsitePickupHandler.Register` |
| `POST /transport-fulfillment/offsite-pickup-attempts` | `NewPerformOffsitePickupEndpoint` | `PickupAttemptIntake` / `PickupAttemptHandler.Handle` | `PerformOffsitePickupHandler.Handle` |

首登与更正分两个端点，理由同交付端点：命令形状与恢复动作不同，合并就得靠请求体里的模式字段
分路。单对象登记与多对象执行同组不同口：命令类型互不相同，合成一口就得在 Intake 里先认形状。

**结果代数 → 响应，逐条映 ADR-0022**：

- 应用层返回 `error` → 5xx `NO_ANSWER_FORMED`，**不带 `outcome`**。
- 应用层返回结果 → `outcome` 取应用枚举原名（`HANDOVER_REGISTERED`、`EXISTING_VERSION`、
  `SOURCE_CONFLICT`、`HANDOVER_CORRECTED`、`SOURCE_NOT_ACCEPTED`、`HANDOVER_UNDECIDED`；揽收两口同理），
  传输层不合并、不改名、无「其他」格。
- **`未决` 不带 `error` 时是形成了的答案**：200，带 `continuationReference` 与 `undecidedReason`。
  这是 ADR-0022「镜像应用签名」的字面，与同包交付端点、pricing 复核端点的既有测试一致；
  MCP-3 裁决 4 的速记「未决→5xx」指的是 error 那一支，不是 outcome 那一支（已在票 03 注明）。
- 新落一版用 201：`HANDOVER_REGISTERED`、`HANDOVER_CORRECTED`、`PICKUP_REGISTERED`、`ATTEMPT_RECORDED`；
  其余已形成答案 200。
- **段那一半的欠账必须透出**：单对象两口的 `SegmentContinuationReference`、多对象口的
  `SegmentEntries`（逐对象）、以及 `HandoverHandoffReference` / `PickupHandoffReference`。这些是
  编排刻意与 `Outcome` 分开交回的东西——来源保全成立、派生一侧欠着——传输层吞掉任何一个，
  调用方就没法续办。

### 二、端点表与占位（`cmd/parcel-api/`，共享接线文件，动前占号）

- `endpoints.go`：四行，Intake 一律字面量 `tfhttp.UnconfiguredIntake{}`（命令面，隔离读准入换不了写行）。
  `UnconfiguredIntake` 要补实现新 Intake 接口的方法（同一个类型，每个方法都答
  `ErrAccessChannelNotConfigured`）。
- `unwired_orchestration.go`：`unwiredHandover`（一型顶两口，同 `unwiredDelivery`）、
  `unwiredPickupRegistration`、`unwiredPickupAttempt`，只供装配测试。
- 端点表测试补四个 Pattern。

### 三、装配（`cmd/parcel-api/assemble_control_facts.go`，新文件）

事务包装照 `transactionalDelivery`。依赖缝逐条：

- 交接：`tfpostgres.NewTransportHandovers`、`NewFulfillmentSegments`（`Segments` 可缺席但生产**要接**——
  接上它进段才有路走到，这正是本票是关键路径的理由）、`NewOutboxTransportHandoverRegistrationHandoff`、
  `systemClock{}`。
- 揽收登记：`NewOffsitePickupRegistrations`、`NewFulfillmentSegments`、`NewResultVersions`（担
  `PickupIdentityFactory`）、`NewOutboxOffsitePickupRegistrationHandoff`、时钟。
- 揽收执行：`NewPickupAttempts`、`NewFulfillmentSegments`、`NewResultVersions`、`NewOutboxOffsitePickupHandoff`、时钟。

装配测试对真库实跑，照 `assemble_delivery_test.go`：首登提交、重放答 `EXISTING_VERSION`、
**带 `Segment` 的交接登记后段登记册里能读到那个段**（进段这道门第一次在生产装配上被证明走得到）。

## 端点测试要专钉的（票 03「与票 08 那个缝同形」）

1. **三入口各一条**：Intake 交出的命令里 `Segment` / `PlannedSegment` 原样到达 Handler；
   `PerformOffsitePickup` 那条还要钉整次 `Segment` 与逐对象 `PlannedSegment` 两层都在。
   HTTP 适配器若在中间立一个自己的请求形状而漏掉那一格，就是「编排接上了、端点没让它接上」。
2. 响应透出 `segmentContinuationReference`（两个单对象口）与 `segmentEntries`（多对象口）。
3. 通用纪律：非 POST → 405 带 `Allow`；未配置 → 403 `ACCESS_CHANNEL_NOT_CONFIGURED`；
   `ErrMalformedRequest` → 400；其他 Intake 错 → 500 `INTAKE_FAILED`；编排 `error` → 500
   `NO_ANSWER_FORMED` 且响应体无 `outcome`；空 outcome → 500 `UNNAMED_OUTCOME`。
4. 201 / 200 分法逐格。

## 边界与未做

- **揽收更正口**：裁决 2 写「揽收含更正口」，但 `RegisterOffsitePickupHandler` 只有 `Register`，
  应用层没有更正编排。本票**不造编排**（TF application 不在本票地盘），端点表只挂登记口；
  要不要立更正编排由 MCP-3 裁后另立票。
- **「段已关闭」单开一格**（裁决 3）：要动 `enterFulfillmentSegment`，不在本票；等 MCP-1 答复后
  单立票或并入，见票 03 Comments。
- Intake 只有未配置格；真实接入渠道的认证属 `PAR-INT-01` 实例半边，不写任何取值。
- 不改任何既有导出签名；不碰 TF domain / ports / application / adapters/postgres。

## 完工判据

四个端点在表上、Intake 未配置即拒在编排之前、装配测试含真库 PASS（`-v` 下非 SKIP）、
上面四条专钉测试各自红过再绿、gofmt / build / vet / `go test -count=1 ./...` 在临时 detached
worktree 上绿、提交带 pathspec。落库后向 MCP-3 报 SHA 与验证种类。
