# 06 挂载 TF 交接范围汇总端点，并在运输履约查阅页给它一签

Category: enhancement
Status: resolved（2026-09-03，MCP-1，`bd9624e`）
Blocked by: 无（`tf-unwired-seven/03` 的四层已落 `0005897`，票面于 `07c31ff` 收口，见下方 Comments）

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

- 2026-09-03 · MCP-1：**四项全做，装配表上不再有未挂载的端点构造函数。**

  **等的那一步已经过去，但不是等到的。** 上一会话向 MCP-3 问过「你的收口会不会碰
  `cmd/parcel-api/`」，没等到回音就中断了。本轮重新取证：`0005897`（适配器 + HTTP 读面）
  已是 `HEAD` 的祖先，票 03 的完工判据（`SummarizeHandovers` 上生产调用路径、棘轮那行已剪）
  在树上都成立，只有票面仍标 `in-progress`。据此判断 03 余下只剩票面收口、不动
  `cmd/parcel-api/`，广播占号后开工，并在广播里写明「要动就说一声，我让位」。**这是一个
  推断不是一次答复**——写在这里是因为它与「等到了回音」在结果上长得一样，而后来读的人
  分不出。（事后印证：`07c31ff` 就是 03 的票面收口，只动 `.scratch/`。推断对了，但对的是
  推断，不是取证。）

  **后端**：`main.go` 取 `NewTransportHandovers` 当读口（它同时实现写侧幂等存取与
  `ports.HandoverScopeView`）构 `SummarizeHandoverScopeHandler`；`endpoints.go` 一行，Intake
  复用 `transportCatalogueIntake`；`unwired_orchestration.go` 增
  `unwiredHandoverScopeSummary`；`endpoints_test.go` 探针带 `?scope=`（缺席是 400，会盖过
  未配置面的 403）；`isolated_read_test.go` 放行表增一行——它与运输履约查阅共用同一个 Intake
  变量，不列等于断言「同一个 Intake 会给出两种答案」。

  **这一行是装配表上第一个第二参不是读口的查阅端点**：接的是应用读用例。判据没有因此松
  ——那个用例零登记零编辑零披露，隔离读那三条逐条满足；理由记在端点行与
  `tfhttp.HandoverScopeSummarizer`，此处不复述。

  **占位不答零值结果**：`SummarizeHandoverScopeResult{}` 的 outcome 是 `Invalid`，端点会判
  成 `UNNAMED_OUTCOME` 的 5xx，而那条路径本是用来抓「应用层漏了一格没具名」的。占位走上去
  等于把一次未接线伪装成一处结果代数缺口，因此照其余占位交回稳定错误落在
  `NO_ANSWER_FORMED`。

  **前端**：查阅页第六区「交接范围汇总」，不是第六本册而是上一区那本册的派生问答。判读抽成
  `handover-scope-summary.ts`（纯函数，测试不必起 React）。**`不成立汇总`那格在 TS 类型上就
  没有 `summary` 键**——判别联合让「显示三个零」在编译期写不出来，比靠测试守强一级；空态文案
  再把它与「三格都是 0」明说分开。范围入参与输入草稿分两格：合一格会逐键触发真请求，而册区
  的检索是本地过滤、不发请求，两者在同一个输入框里必须行为不同。

  **门禁做过一次证伪，不是只看它绿**：把前端那条路径改成
  `/transport-fulfillment-handover-scope-summaries` 后
  `TestEveryAdminWebPathIsOnTheParcelAPIEndpointTable` 报红并逐字点名该文件与该路径，改回即绿。
  票 03 那条门禁今天恒绿，绿证明不了它看见了这条新路径。

  **未做且要说清**：

  - **没有契约夹具。** 票 04 立的那套（`internal/<ctx>/adapters/http/testdata/*.json` 前后端
    共读）本该覆盖这个响应体，但那要写进 `internal/transportfulfillment/`——MCP-3 的地盘，本轮
    只读不写。前端这份 TS 类型因此仍是手写镜像，后端改键不会让前端红。留给 04 的后续切片。
  - **没有端到端真库调用。** 「生产可达」的证据是编译期装配加适配器自己的真库测试，不是一次
    真跑通的 HTTP 请求——与其余目录查阅行同一水位，不因本票拔高。
  - **棘轮基线零改动**：`SummarizeHandovers` 那行在 `0005897` 已剪，本笔不新增也不剪。

  **验证**（钉在提交前的工作树上，DSN 已设、库可达）：`gofmt -l cmd/parcel-api/` 为空、
  `go build ./cmd/...` 退 0、`go vet ./cmd/parcel-api/` 退 0、`go test -count=1 ./cmd/parcel-api/`
  与 `./internal/architecture/` 绿；`tsc -b` 无输出、`pnpm test` 25/25。真库判据按开关取证而不看
  `ok`：`TestListByScopeReturnsEveryRegisteredVersionOfTheScopeOnly` 在 `-v` 下是 `PASS` 不是
  `SKIP`，所以本笔的绿是**含真库的绿**。`-race` 本笔未跑，归收尾那一批。
