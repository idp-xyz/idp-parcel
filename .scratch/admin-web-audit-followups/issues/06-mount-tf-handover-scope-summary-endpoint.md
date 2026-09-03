# 06 挂载 TF 交接范围汇总端点，并在运输履约查阅页给它一签

Category: enhancement
Status: ready-for-agent——**等 `tf-unwired-seven/03` 提交后再认领**
Blocked by: `.scratch/tf-unwired-seven/issues/03-handover-scope-summary-read-face.md`（MCP-3 在途）

## 缺什么

取证 `0d492b8`：`internal/transportfulfillment` 四层俱在——`ports.HandoverScopeView`、
postgres 适配器（`transport_handover_registry.go`）、应用 `SummarizeHandoverScopeHandler`、
HTTP `NewQueryHandoverScopeSummaryEndpoint`（`GET /transport-fulfillment-handover-scope-summary?scope=`）。
`cmd/parcel-api/endpoints.go` 没有它的行，`main.go` 也没构造它——全仓 76 个端点构造函数里
唯一一个未挂载的。

## 做什么（后端）

1. `main.go` 构造 `SummarizeHandoverScopeHandler`（读口用已有的 TF 读适配器）。
2. `endpoints.go` 加一行，Intake 复用 `transportCatalogueIntake` 变量（查阅行，随 ADR-0078 隔离读
   开关换值；写行的字面量纪律不受影响）。
3. `endpoints_test.go` 的未配置面断言与 `isolated_read_test.go` 的放行表各加一格；
   `unwired_orchestration.go` 加占位（方法表对 `HandoverScopeSummarizer`）。

## 做什么（前端）

运输履约查阅页加「范围汇总」签：一个 `scope` 输入 + 四格 outcome 呈现。**`不成立汇总`那格不显示
三个零**——后端刻意不带 `summary` 键，页面照它说「这个范围还没有交接」。

## 为什么等

MCP-3 的票面写着 HTTP 读面与适配器是它「余下」的两层；今天树上两层文件已在但票未收口。先动
`endpoints.go` 会与它的收口撞同一行。

## 完成判据

`go test ./cmd/parcel-api/ -count=1` 绿；前端签对 `/api/transport-fulfillment-handover-scope-summary`
发请求，未配置态如实呈现。

## Comments
