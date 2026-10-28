# 05 收寄后服务处置请求信封无下游消费

Category: enhancement
Status: in-progress——2026-10-28 作者完工 TF 一半，余下需 NO、NR 所有者新模型，待协调方裁与隔离评审，分支 claude-lw-pid；此前：2026-10-28 会话 tops-20（idp-parcel-91）立并认领（用户「现在就接线」）
Blocked by: 无

## 做什么

ADR-0332 的范围化请求信封入队后今天没有消费者。按 ADR-0332 请求表接下游（code/37 完成记录「下游承接」一段）：

- TF：拦截、改送、退运、服务终止的运输请求——消费信封，以 `POST_INTAKE_DISPOSITION/<包裹>/<版本>` 作服务处置决定依据启动备用/退运旅程（UC-TF-007，`StartAlternateJourneyHandler`，code/43 已接成口）。
- NO：拦截扣住、退运按退运装载、服务终止扣住。
- NR：改送、退运重新规划，服务终止释放。
- 承接结果读回（`AT-PS-088`）；监管退运转 UC-TF-001（`AT-PS-084`）。

规模大：作者先按各所有者现有能力出切分，能在本票内接的接，需要所有者新模型才接得上的写明并回报协调方，不自造他上下文的模型。

## 完成判据

- 演示租户上做一个「退运」决定，TF 旅程、NO 扣住/装载、NR 规划各有可见落点（真库进程用例）；其余去向同理或写明去处。
- `AT-PS-083`、`084`、`088` 移出④，或写明还差什么。

## 完成记录

起点 `2d0193336`。

**切分（每去向 × 每所有者，今天有没有现成编排可消费信封）**

| 去向 | TF | NO | NR |
|---|---|---|---|
| 退运 | 已接：另起退运旅程（`StartAlternateJourneyHandler`） | 差：按退运装载并交出，NO 无对应模型 | 差：规划退运路径，NR 只有初始路由与重评（`ReassessRoute`），无「按去向规划」 |
| 改送 | 已接：另起备用旅程 | — | 差：按目的地重新规划，目的地引用由交付去向所有者解读，NR 无此入口 |
| 拦截 | 差：「不再安排向前移动」，TF 无扣住模型 | 差：在控制处扣住，NO 无扣住模型 | — |
| 服务终止 | 同拦截 | 同拦截 | 差：释放尚未消耗的计划，NR 无释放入口 |

NO 三项、NR 三项、TF 两项扣住，全需所有者先立模型（扣住事实与解除、退运装载、按去向规划、计划释放），本票不自造。按派单，做完 TF 可接的一半即停。

**改了什么**
- `internal/transportfulfillment/adapters/inbox/post_intake_disposition_consumer.go`：消费门，译载荷，缺键维或时刻为毒丸。
- `internal/transportfulfillment/adapters/parcelshipment/post_intake_disposition_journey.go`：只承接 TF 的 `START_ALTERNATE_JOURNEY`/`START_RETURN_JOURNEY`，调与运营决定口同一只 `StartAlternateJourneyHandler`，依据取信封的 `POST_INTAKE_DISPOSITION/<包裹>/<版本>`、依据种类恒为服务处置决定。
- `cmd/parcel-dispatch/routes_post_intake_disposition.go`、`assemble_post_intake_disposition.go`（新文件，`assemble.go` 未动）。
- `cmd/parcel-dispatch/post_intake_disposition_journey_test.go`：真库进程用例，退运决定经生产交接口入队、派发后落退运旅程；重入队同信封 ID 不开第二条；下一版改送的原旅程指向退运旅程；拦截入账不开旅程且无失败码。
- UC-PS-006「给开发的交接」一行改写已接与未补。

**代决与越权风险点**
1. 新旅程身份取 `JRN/<包裹>/PID-<决定版本>`（确定性派生，仿 ADR-0218 的 `JRN/<对象>`），是作者的产品策略取舍，待 TF owner 复核。
2. 原旅程取 `ActualFulfillmentJourney` 在决定时刻对包裹的判定（旅程册无记录时为 `JRN/<包裹>`），排除以同一依据启动的旅程以保重投稳定；信封本身不带原旅程标识（ADR-0332 载荷只带关系），故由 TF 一侧派生，待 TF owner 复核。
3. 成员取 `[包裹]`：信封不带成员，整件包裹作为一个成员；集运单元等成员展开未做。
4. 启动时刻取决定生效时刻（信封 `decidedAt`），启动人留空（不经运营口）。
5. 旅程册未决或意图未交出时返回哨兵整笔回滚重投；依据种类不是服务处置决定、未受理、来源冲突为非未决，保持 publish_failed。
6. 拦截与服务终止信封到 TF 路原样入账，没有可做动作；其余所有者尚无路，信封只有 TF 一路消费。

**判据对用例**：TF 一半达成（演示租户 `SYN-TENANT-01` 同租户身份、真库进程用例，证据 `S`）；NO 扣住/装载、NR 规划无可见落点，未达。`AT-PS-083` 仅 TF 半边，`AT-PS-084`（监管退运转 UC-TF-001）、`088`（承接结果读回）未动，仍在④。

**自验**：`go build ./...`、`go vet`（cmd/parcel-dispatch、transportfulfillment）、`gofmt -l` 无输出；带 PG DSN `go test -count=1 -p 1` 跑 `./cmd/parcel-dispatch`、`./internal/transportfulfillment/...`、`./internal/architecture/...`、`./internal/platform/...` 全绿；`at-coverage-counts -check` 无报（未新增 AT 号）。未取迁移、ADR、种子号。

**留给协调方**
- 余下 NO（扣住、退运装载）、NR（按去向重新规划、退运路径、计划释放）、TF（扣住）需各所有者新模型，各立票；`AT-PS-088` 承接结果读回依赖各所有者的承接事实。
- 演示动线上「退运」从决定到 TF 旅程可见的是派发进程这一段；管理台页与决定追加口的演示灌入未碰。
