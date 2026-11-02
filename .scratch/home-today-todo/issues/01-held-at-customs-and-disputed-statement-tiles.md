# 01 首页今日待办补「卡关」「待对账」两格：各补一个只读计数口，点进去落到筛好的列表

Category: enhancement
Status: in-progress——2026-11-01 作者完工，待隔离评审与进 main，分支 claude-hometodo（最后一笔见「完成记录·提交」）。本票动了 Go，是后端票：作者只做「进 main 的步骤」第 1 步
父票：无（设计源「九、怎么落地」第三步：今日待办是待受理、待处置、卡关、异常、待对账各一格，点进去就是筛好的列表）
Blocked by: 无
地盘：`internal/customscompliance/{ports,adapters/postgres,adapters/http}` 的判断册读面、`internal/settlementaccounting/{ports,adapters/postgres,adapters/http}` 的对账单读面、`cmd/parcel-api` 两个特性文件及其未接线占位、`apps/admin-web` 首页与清关单、对账单两页、UC-CC-013 与 UC-SA-003 各一行验收、AT 盘点两行、本票。
出处：用户 2026-11-01「不要留下尾巴」；协调方派单。上一轮首页改版没做这两格，理由是「今天没有直接回答这两问的读口」。

## 要做的

1. 先按 CONTEXT 定义两格各数什么，再找读口；读口答不如实就在主责上下文补只读查询。
2. 首页加两格，点进去落到对应页并带上筛选。
3. 定下今日待办是八格还是替换某格，写明理由。

## 完成记录

**起点** `origin/main` `b2c1505c7`。分支 `claude-hometodo`，只推分支不推 main。

### 两格各数什么（先定义，再取数）

**卡关 = 最近一次动作准入判断不让动作过门的对象。**

- 出处：CC CONTEXT「动作准入判断」词条——对明确对象、一次动作意图与动作时点，三源折成「放行、阻断、不受门禁、未决或未配置」的封闭答复；`domain.AdmissionAnswer` 的压制次序注释：「未配置压过未决——缺配置的那一格重投不会自己好，要人去配；未决压过放行——还缺东西就不能说能走」。所以「过门」只有放行与不受门禁两格（新加 `AdmissionAnswer.Admits`），未决、未配置、阻断都是还卡着、要人处理的（等限制解除或放行、等补齐、等配好）。
- 「最近」按动作时点取（同一时点再按判断时刻）：执行方晚报的一次早先动作不该盖过之后那次动作的答复。CONTEXT「来源变化不后台重算」，所以放行到了而没人再过门的对象仍在列——过门要再问一次，这正是它还在等的事；UC-CC-013 目标与边界「阻止或撤销已经发生的物理动作……归动作执行方（ADR-0205）」，被挡后照样动了的对象也在列，同样要人看。
- 没有用「清关中的案件」（进行中的关务案件大多在正常走，不是卡住），也没有用「多久没动算卡住」——那要一个时长，属租户取值，本格不需要它。监管限制、内部合规限制没有列表读口（冻结清单 2026-10-28 那条），而它们挡人的效果已经折进准入判断，从判断册取就够。

**待对账 = 挂着未了结争议的已发布客户对账单。**

- 出处：SA CONTEXT「费用争议」词条「结算一方针对明确费用及金额范围提出、等待核实或处置的异议」；UC-SA-003「争议处理形成接受、部分接受、拒绝或待复核结果」；领域 `StatementDispute.Resolve`「待复核可再处理；终局三格不可再改」（新加 `DisputeResolutionKind.Final`，`Resolve` 改用它，语义不变）。所以未处理与待复核的争议还在等人，接受、部分接受、拒绝算处理完。作废的单照列：作废不了结争议。
- **不是对账状态**。票 [admin-skeleton-closure-batch/04](../../admin-skeleton-closure-batch/issues/04-settlement-accounting-http-read-faces.md) 撤对账状态一栏的理由（「异议不是对账状态」）照旧成立：本格按单上已记的争议筛，不说哪张单「已对账」。
- **设计源写的「待确认应收」答不出**：UC-SA-003 结果契约的对账状态有相对方未确认、部分确认、已确认三格，但本产品不记相对方确认；把未作废的单都当待确认是拿「没有记录」冒充「客户还没确认」，不数。另查到**争议的提出与处理在生产上也没有口**：编排有 `OpenDispute`、`ResolveDispute`，`cmd/` 下无人调用，生产库上争议只能是空的，这一格在生产上如实答 0。两条立票 [02](./02-statement-confirmation-and-disputes-have-no-production-face.md)，记进 UC-SA-003「给开发的交接」。

### 为什么补后端读口，不用现有的

现有判断册口（`/customs-action-admission-judgements`）与对账单册口（`/settlement-statements?registry=customer-statement`）都固定页大小 200、不带总数与截断标志（冻结清单 2026-10-28 已记）；对账单册还按单号升序，新单先被截掉。前端在这一页里数，过了两百就少报，判断册还会把后来已放行的运单算进去——不如实。于是各补一口：

| 口 | 答什么 | 件数 |
|---|---|---|
| `GET /customs-unadmitted-objects` | 每个对象最近那份判断不过门的，卡得最久的在前，行形与判断册同一份 | `unadmittedCount` + `truncated` |
| `GET /settlement-disputed-statements` | 挂着未了结争议的已发布单，按单号，行形与对账单册同一份 | `disputedCount` + `truncated` |

件数用窗口计数与这一页在同一条语句里求（窗口在 LIMIT 之前算），不分两次读。分层照各上下文现有读口：`ports` 加答案类型与读口接口，`adapters/postgres` 在既有读面适配器上加方法（判断册与对账单册的行解码各抽成一处共用），`adapters/http` 加端点（读不回答 500 `NO_ANSWER_FORMED`，不伪装成 0），`cmd/parcel-api` 在既有两个特性文件里各加一行、挂运营查阅读面与各自上下文的 Intake，未接线占位补齐。领域只加两个判读方法，不依赖 HTTP 与 pgx。

### 首页与两页

- **八格，不替换**：设计源列待受理、待处置、卡关、异常、待对账各一格。现有六格里待处置是三张各有主人的队列（受限运单、收寄后处置、资金释放），合一格就要替三个读口加总、点进去也落不到一张列表；审批与发布不在设计源那一列里，但同样是在等人处理的队列。所以卡关、待对账加进来、不替换哪一格，按设计源次序排（待受理 → 待处置三格 → 卡关 → 异常 → 待对账 → 审批与发布），栅格改四列成两行。
- 格子标题照旧取导航页名，后面标筛的是哪一类：「清关单 · 卡关」「对账单 · 待对账」，与页上开关同名。
- 点进去落在筛好的那一签：`#/customs-cases?tab=admission&only=unadmitted`、`#/reconciliation?only=disputed`。两页的签与开关读自地址、写回地址（做法同商业规则页的 `usePoliciesPlace`），本签已开着时跟着 hashchange 走；清关单页「只看卡关」换读卡关口，筛选摘要写全部件数与是否截断；对账单页「只看待对账」同理。`only` 不与保存视图的 `view`、检索词的 `q` 撞键。

### 判据对用例

新立 `AT-CC-456`（UC-CC-013）与 `AT-SA-227`（UC-SA-003），核过 origin 各分支最大号为 455 / 226；AT 盘点 ① 桶各加一行，总览计数留给进 main 时重算（`at-coverage-counts -check` 只报生成物过期、行齐）。

| AT | 判据 | 用例 |
|---|---|---|
| CC-456 | 先挡后放行不列、先放行后挡列、未决与未配置列、不受门禁不列；晚报的早先动作不盖过之后那次；卡得最久的在前；件数不受页大小截断；他租户不进来（真库） | `TestUnadmittedObjectsAreThoseWhoseLatestJudgementDoesNotAdmit` |
| CC-456 | 件数是全部、页短标截断、行形同判断册、租户只取自接入面；0 件与读不回分开；未授予 403；非 GET 405 | `TestUnadmittedObjectQueryCarriesTheWholeCountAndMarksTruncation`、`TestUnadmittedObjectQueryAnswersZeroOnlyWhenTheReadAnswersZero` |
| SA-227 | 未处理与待复核算、终局三格不算；一单多项只算一张；作废照列；无争议不列；张数不受页大小截断；他租户不进来（真库） | `TestDisputedStatementsAreThoseWithADisputeAwaitingAFinalResolution` |
| SA-227 | 张数是全部、页短标截断、行形同对账单册；0 张与读不回分开；并入目录口拒绝共用组（未授予、坏形状不触读口） | `TestDisputedStatementQueryCarriesTheWholeCountAndMarksTruncation`、`TestDisputedStatementQueryAnswersZeroOnlyWhenTheReadAnswersZero` |

### 自验

- Go（含 PG，DSN `127.0.0.1:55432/postgres`）：`go build ./...`；`go vet ./cmd/parcel-api/ ./internal/customscompliance/... ./internal/settlementaccounting/...`；`gofmt -l cmd internal` 无输出；`go test -count=1 -p 1 ./internal/customscompliance/... ./internal/settlementaccounting/... ./internal/architecture/... ./cmd/parcel-api/...` 全绿（新真库用例以 `-v` 核过跑了、未 SKIP）。
- 前端：`npx tsc -b`、`pnpm test`（1014 过 / 0 败）、`npx vite build` 通过。
- 浏览器（5185，自编 parcel-api 起在 19085，一次性库 `demo_hometodo` 经 `demo-run/up.sh` 灌种子、跑 `10-main-chain` 两遍与 `30-settlement`，验完 `down.sh --drop-db` 删库、停进程）：
  - 真链路数据：主链那件包裹交进深圳口岸、交给干线承运商时判断答未决（不在关务册、跨区），之后在新加坡交出与交付答不受门禁——卡关口如实答 0 件（先挡后放行不算）；对账单已发布、无争议——待对账答 0 张。
  - 非零核验：生产上没有争议的写入口，在一次性库里直接写了一项未处理的争议（`SYN-DSP-HOMETODO-01`），另写两份合成判断（阻断、未配置）。首页显示「清关单 · 卡关 2」「对账单 · 待对账 1」，琥珀色；点卡关格落到 `#/customs-cases?tab=admission&only=unadmitted`，表头「共 2 件卡关」，两行依据 `RELEASE_NOT_RECEIVED`、`GATE_CATALOG_NOT_REGISTERED`；取消勾选回到放行记录全表、地址去掉 `only`；签已开着时再从首页点进来，开关跟着地址重新勾上；点待对账格落到 `#/reconciliation?only=disputed`，「共 1 张有未了结异议」，那一行状态「有异议待裁」；切到供应商签地址变 `?tab=supplier`、清掉 `only`。全程无页面错误、无写请求。
- 没碰 `:5173`、`:19080` 与库 `demo_live`；没动 ADR 索引、AT 总览计数与机制清点。

### 留给协调方

- 票 [02](./02-statement-confirmation-and-disputes-have-no-production-face.md)：相对方确认不记、争议的提出与处理没有生产口。后者意味着首页待对账在生产上恒为 0（真 0，但原因是争议进不来）。要不要做、谁来提出与确认，请定。
- 卡关的定义取「最近一次动作准入判断不过门」，包含未决与未配置，不只阻断；放行到了而没人再过门的仍算。若要只数阻断，或要按「放行事实已到」自动出列（那要后台重算，与 CONTEXT「来源变化不后台重算」相悖），请定。
