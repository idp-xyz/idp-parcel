# 复核队列查阅面：/acceptance-review-queue 与页 acceptance-review 接真

Category: feature
Status: resolved——由 admin-skeleton-closure-batch/09 兑现（随 `df51ce0` 入库，MCP-5 于
`e35d898` 核过范围七步）；本票 2026-09-08 由通道 2 按用户指示做簿记收口，核对见 Comments 末条。
此前为 handed-off——移交 09（MCP-5，2026-08-31），半成品清单见移交注记
Blocked by: —

## 要做什么

parcel-shipment 上开复核队列查阅面（读，不带决定），页 `acceptance-review` 接真：

1. **端口**（ports.go）：`AcceptanceReviewQueueRecord`（概要 + 最近处理记录 + 复核
   完成留痕）与 `AcceptanceReviewQueue` 读口（`ListAwaitingManualReview` +
   `FindVisibleByID`，后者与 `ShipmentRequestViews` 同签名——详情复用查阅详情，不另
   造第二种详情）。`AcceptanceTaskViewRecord` 加性扩展：`WaitingOn`、复核完成五字段
   ——详情侧要能如实呈现「停在哪、复核录了没」。
2. **真库适配器**：方法挂在既有 `*ShipmentRequestViews` 上（同库同表同作用域纪律）。
   过滤在 SQL 键上：`(snapshot->'acceptanceTask'->>'waitingOn')::smallint = 3`
   （`ResumeByManualReview` 的数字表达由 Go 侧常量传参，SQL 只比较不拥有编号——与
   state 列的读写同一条纪律）；排序老的在前（先来先审），同刻按委托标识正序。
3. **HTTP**：`GET /acceptance-review-queue`，列表与详情按 `shipmentRequestId` 分派
   （形照 `/shipment-request-views`）；Intake 复用 `ShipmentRequestViewsIntake`。
   详情连 `ports.RecordedJudgmentReader` 读回的判断三组（可达性逐包裹、财务控制、
   采用解析）一并作答。outcome 封闭集合：`REVIEW_QUEUE_LISTED` / `REVIEW_CASE`；
   不可见同 `SHIPMENT_REQUEST_NOT_VISIBLE` 404。
4. **装配**：endpoints.go 查阅行挂 `shipmentViewsIntake` 变量（隔离读随之换值）；
   main 交入 `requestViews`（同适配器）与 `NewAcceptanceJudgments`；unwired 占位、
   探针表、隔离读放行表各一行。
5. **前端**：`pages/shipment-request/api.ts` 加两函数与响应形（镜像处理器 DTO）；
   `AcceptanceReviewPage` 接真（ReviewFlowTemplate：队列、详情字段、判断三组、复核
   留痕），两决定按钮如实禁用注明票 02；`liveIds` 收 `acceptance-review`。

## 页-表映射（定稿）

| 页面呈现 | 出处 |
| --- | --- |
| 队列行：委托标识、客户账户、来源/请求键、版本、件数、提交时刻 | `shipment_request` 投影列（概要与 `/shipment-request-views` 同形） |
| 队列行：最近处理记录（原因/续办引用/时刻） | 快照 `acceptanceTask.processingAttempts` 末条 |
| 队列行：复核已录完成 + 留痕四样 | 快照 `acceptanceTask.reviewCompletion` |
| 详情：申报包裹、批次、版本数、任务阶段、决定 | 既有 `ShipmentRequestDetailRecord`（复用） |
| 详情：等待续办（MANUAL_REVIEW）与复核留痕 | `AcceptanceTaskViewRecord` 本票加性扩展 |
| 详情：可达性逐包裹 / 财务控制 / 采用解析 | `RecordedJudgmentReader`（acceptance_judgment 三表） |

队列语义：`waitingOn=MANUAL_REVIEW` 即在列——复核已录完成但未续办的行照列并标示
（出队靠下一轮 Decide 清 waitingOn，不靠读侧折叠）。「停等起点」无处可读（未决轮
不落决定时刻），如实不设该列。

## 完成判据

- 真库测试：过滤（SUBMITTED 无 Decide、停等内部重试、他租户停等复核都不出现）、
  排序、limit、字段逐项（含复核已录行）。
- 处理器测试：405 / 未配置 403（两分支）/ 列表逐字段 / 详情连判断逐字段 / 不可见
  404 / 读故障与判断读故障 500。
- 装配测试三处绿（探针、隔离读两态、方法门）。
- 提交态验证：go build/vet/gofmt + 含真库全仓测试 + admin-web pnpm build 全绿。

## Comments

### 移交注记（MCP-1 → MCP-5，2026-08-31 18:30）

本票开工于票 09 广播抵达前，读面已建到「传输层测试绿、真库测试被迁移 0009 半程卡住」
的程度。工作树里的未提交半成品（全在 09 号票占号地盘内，MCP-5 按需吸收或丢弃）：

**新文件（未跟踪）**
- `internal/parcelshipment/adapters/http/query_acceptance_review_queue.go`——
  GET /acceptance-review-queue 处理器：列表/详情按 `shipmentRequestId` 分派、Intake
  复用 `ShipmentRequestViewsIntake`、详情连 `RecordedJudgmentReader` 三组判断一并作答；
  outcome `REVIEW_QUEUE_LISTED`/`REVIEW_CASE`，不可见同 404 `SHIPMENT_REQUEST_NOT_VISIBLE`。
- `internal/parcelshipment/adapters/http/query_acceptance_review_queue_test.go`——
  六组传输层用例**全绿**（转写、判断三组、零值缺席、统一不可见、未配置两分支、4xx/5xx 分流）。
- `internal/parcelshipment/adapters/postgres/acceptance_review_queue.go`——
  `ListAwaitingManualReview` 挂在 `*ShipmentRequestViews` 上；**注意**：写时按 jsonb
  过滤（`(snapshot->'acceptanceTask'->>'waitingOn')::smallint = $3`），你们的 0009 投影
  列落地后应改为 `task_waiting_on = $3 AND state = $4`（吻合部分索引谓词）。
- `internal/parcelshipment/adapters/postgres/acceptance_review_queue_test.go`——
  真库用例三组（过滤/排序/limit + 详情带等待态与复核留痕）；**当前红**：0009 加了
  NOT NULL 列而 `Insert`/`Save` 还没写它，所有插入违约束——你们同 SQL 写列落地后即绿。

**改动（已跟踪文件，均为加性）**
- `ports.go`：`AcceptanceReviewQueueRecord` + `AcceptanceReviewQueue` 读口（详情方法与
  `ShipmentRequestViews` 同签名，同一适配器双实现）；`AcceptanceTaskViewRecord` 加
  `WaitingOn` + 复核完成五字段。
- `shipment_request_views.go`：`taskViewRecord` 解析 waitingOn（越界拒、零值缺席）与
  reviewCompletion。
- `shipment_request_test.go`：`acceptanceBasis` 参数化出 `basisWithReviewPolicy`（造
  「规则要求复核」夹具用）。

未动装配四件与页面。若不吸收，`git checkout -- <三个已跟踪文件>` + 删四个新文件即可
回到 `4b35815` 基线。

### 收口（通道 2，2026-09-08）

用户指示「先收口」。本票自 `1665fdb` 起一直挂 handed-off，而移交目标
admin-skeleton-closure-batch/09 早已 resolved（随 `df51ce0` 入库；MCP-5 2026-09-01 在
`e35d898` 上逐项核过其范围七步）。翻状态前在 main `96558acd` 上按**本票自己的**「要做什么」
与移交清单重核了一遍，不拿 09 的票面当证据：

- 移交清单里的四个未跟踪文件（`query_acceptance_review_queue.go` 及其测试、
  `acceptance_review_queue.go` 及其测试）全部被 09 吸收，`git ls-files` 四件都在 main。
- `GET /acceptance-review-queue` 挂在 `cmd/parcel-api/endpoints.go` 端点表
  （`NewQueryAcceptanceReviewQueueEndpoint(shipmentViewsIntake, reviewQueue, reviewJudgments)`），
  探针表 `endpoints_test.go` 与隔离读放行表 `isolated_read_test.go` 各有一行。
- 页面：`apps/admin-web/src/pages/shipment-request/AcceptanceReviewPage.tsx` 接该端点的列表
  与单份两支；`page-registry.tsx` 的 `liveIds` 收有 `acceptance-review`。
- 本票范围之外的两个命令口（复核完成、主动拒绝）属票 02 并入 09 的那半，本票不据此收口，
  只记它们也在。

「完成判据」里的真库/处理器/装配三层测试与提交态验证由 09 入库时跑过（见其 Comments
2026-09-01 两条），本票不重跑、不另立证据等级。无一行代码改动，只翻本文件 `Status:` 并追加
本条；父 spec 已是 superseded，不动。取证时 `git log --all --not main -- .scratch/acceptance-review-read-face/`
为空，无在途分支碰过本目录。
