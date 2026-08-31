# acceptance-review 接线切片——范围裁定与完成判据

Status: superseded——被 admin-skeleton-closure-batch/09 取代（2026-08-31 18:15 MCP-5
按频道 5 用户指示认领**全量**实现：读面 + 复核完成/主动拒绝命令面 + 页面，含迁移
0009/0010 地盘占号）。本切片开工在其广播抵达之前，读面半成品按票 01 Comments 的
清单移交 MCP-5 吸收；命令面票 02 的内容并入 09 号票范围。

## 事实基线（开工时点 2026-08-31）

- 页 `acceptance-review` 是工作台仅存两块骨架之一（另一块 `label-transactions` 归
  admin-skeleton-closure-batch/08 draft，人工 ADR 待裁，不进本切片）。
- parcel-shipment 域上「等待人工复核」已成话语：`Decide` 在全过、无未决、但规则要求
  复核而复核未完成时写 `waitingOn=MANUAL_REVIEW`（domain/acceptance_decision.go 341
  行），三个终态转移各自清零；复核完成留痕 `CompleteManualReview` 域转移在；任务文档
  三字段（waitingOn / processingAttempts / reviewCompletion）随快照落库
  （adapters/postgres/shipment_request.go taskDocument）。
- 判断记录读口 `ports.RecordedJudgmentReader` 有真库适配器（`AcceptanceJudgments`，
  parcel-dispatch 已在消费）。
- 应用层没有「完成人工复核」编排，HTTP 没有复核命令端点——决定面今天不存在。
- 委托查阅读面（`/shipment-request-views`）就位：Intake、作用域、统一不可见、隔离读
  准入（ADR-0078）全套可复用。

## 范围裁定

**进**：复核队列查阅面（读）——列「当前停在等待人工复核」的委托；详情连判断任务、
复核留痕与已记录权威判断一并如实呈现；页 `acceptance-review` 接真该读面并入
`liveIds`。

**出**：复核完成的命令面（决定按钮背后的一切）。理由：`CompleteManualReview` 域转移
虽在，应用编排、授权判定（BD-PS-002 复核角色目录未确认）与复核后续办（重跑 Decide
的触发）三样都不存在；查阅面收决定等于让读口长出第二种「处置」语义。单列票 02
（draft）承载，重启条件写在票面。

## 硬约束

- **队列 = 事实**。「等待人工复核」以快照任务文档 `waitingOn` 为准，不在读侧重推域
  判断；复核已录完成但未续办的行如实列出并标示，不折成「已完成即出队」。
- **作用域纪律与 `/shipment-request-views` 同套**：过滤在 SQL 键上、统一不可见、
  Intake 复用 `ShipmentRequestViewsIntake`，隔离读准入随同一变量换值，不另造第二种
  准入形。
- **判断记录照登记转写**：可达性逐包裹、财务控制零值即「尚未形成」、采用解析零值即
  「尚无」——不代拟、不折并。
- **装配纪律照批 admin-skeleton-closure**：endpoints.go 一行、探针表一行、隔离读
  放行表一行、unwired 占位齐方法表；提交态验证 = go build/vet/gofmt + 含真库全仓
  测试 + admin-web pnpm build。

## 完成判据

1. `GET /acceptance-review-queue` 列表与详情（`?shipmentRequestId=`）真库读面就位，
   真库测试证过滤（等待复核之外不出现、跨租户不可见）、排序（老的在前）与字段逐项。
2. 页 `acceptance-review` 接真：队列、详情、判断三组、复核留痕如实呈现；两个决定
   按钮如实禁用并注明命令面票 02；空态句「读取入口已配置，登记册为空……」；
   `liveIds` 收 `acceptance-review`，工作台骨架档降至 1（`label-transactions`）。
3. 提交态验证绿；票 01 resolved、票 02 draft 立着，本 spec 收口。
