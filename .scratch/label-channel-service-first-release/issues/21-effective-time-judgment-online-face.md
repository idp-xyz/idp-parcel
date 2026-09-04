# 21 有效时间显式判断的在线面：所有者今天没有地方就一条事实说「从何时起有效」

Category: enhancement
Status: resolved（2026-09-04，通道 5 新会话接手收口；六笔均在分支 `mcp5-lc19-21`，已 rebase 到 main `d41f73da`，待 MCP-1 重放进 main）
Blocked by: 无（`16` 已 resolved）

## 缺口

ADR-0102 决定三给有效时间两种来源，第一种是「所有者就这一条显式给出」。票 `16` 落了它的应用入口
`JudgeEffectiveTimeHandler.Judge`（指名事实 + 有效时间 → 新版本回指前版 → 交 VE），**但没有在线面**：
没有端点、没有管理台写面，也没有一个列出「待判断事实」的读面让人知道该判哪几条。与票 `17` 同一条理由
先有事实再谈面——事实现在有了。

## 做什么

1. 读面：按（租户，轨迹源）列出有效时间待判断的事实（`effective_basis = 'PENDING'` 且未被回指的当前版），
   带源事件、状态词、发生时间、接收时间，供判断人看。落 TF `adapters/http` 与管理台页，形状照既有的
   TF 读面（`query_transport_fulfillment_records.go`）。
2. 写面：一条事实一次判断，调 `JudgeEffectiveTimeHandler.Judge`；结果代数四格（已判断／已按同值判过／未受理／
   未决）各有 HTTP 落点。操作者身份走 ADR-0100 的管理台接入面。
3. 载荷形状按 ADR-0101 由本票裁（逐条判断，不是批量导入）。

## 红线

- 面上不解释状态词，不建议一个「推荐的有效时间」——那是替所有者判断。
- 已判断过的版本可以再判（形成新版本），但面上要把「这一条已经判过、按哪个依据判的」摆出来。
- 不动 `16` 的领域与应用层；端点表在 `cmd/parcel-api` 加行时与占号的会话互报。

## 完成判据

读面与写面各有实现与测试，能在管理台上把一条待判断事实判成已判断并看到它进 VE 投影；
`gofmt -l` 空、`go build`/`go vet` 退 0、`go test -count=1 ./...` 绿。

## 参照

ADR-0102 决定三；票 `16` 完成记录「刻意留下的三格」第 3 条；
`internal/transportfulfillment/application/judge_external_tracking_effective_time.go`；ADR-0100、ADR-0101。

## Comments

- 2026-09-04 18:2x，通道 5（重启后的新会话）接手：旧 MCP-5 会话 crash 后现场经通道 6 接管（票 `19` 由它收口），
  用户又把通道 6 调去裁决批，本票随 MCP-1 改派单 `task-dcc78dcf` 回到通道 5。接手时分支 tip `4b045ef7`、
  worktree 干净，本票一行未动；MCP-1 要求「不重开 worktree，不重做设计」，据此在同一分支续做。
- 途中按 MCP-1 广播在本 worktree `git rebase main`（`d41f73da`）一次：只冲机制清点这一份生成物，取 main 版、原
  `12a733c` 对 main 成空笔跳过、收尾在新 tip 重生成；rebase 后带 DSN 的 TF + parcel-api 用例全绿。票 `19`
  完成记录里的四个分支 SHA 因此改号（见该票 Comments）。
- 载荷形态按 ADR-0101 决定八自裁：**逐字段表单**——一次判断只有「哪条事实、从何时起有效」两件事，逐条判不
  批量导入；不走模板导入与草稿。读面在票面第 1 条之外多给了一格 `view=current`（该源全部当前版），理由是
  红线第二条：再判一条已判断的事实之前，面上要摆得出「它已经判过、按哪个依据判的」，而只列待判断的读面
  连那条事实都找不到；`current` 与 `pending` 同一读口同一条 SQL 只差一个过滤，端口把过滤做成封闭两格而不是
  布尔，缺席不猜默认。

## 完成记录（2026-09-04，通道 5；六笔均在分支 `mcp5-lc19-21`，SHA 为 rebase 到 `d41f73da` 之后的分支 SHA，待 MCP-1 重放进 main 后另记 main SHA）

| 笔 | 内容 |
|---|---|
| `bd44a0d0` | `ports.ExternalTrackingFactReviewRead`（`ports/external_tracking_fact_review.go`，新文件，既有端口一格未动）：按（租户，轨迹源）上列**当前版**，`EffectiveTimeReviewFilter` 封闭两格 `PendingEffectiveTimeOnly` / `EveryCurrentVersion`；行是照实转写，待判断行 `EffectiveAt` 为 nil。postgres 实现挂在 `ExternalTrackingFacts` 上（`adapters/postgres/external_tracking_fact_review.go`），「当前」照 `FindCurrent` 的派生法；真库用例 2 条 |
| `7ba40b04` | `GET /transport-fulfillment-external-tracking-facts?source=…&view=pending\|current`（`adapters/http/query_external_tracking_facts.go`）：源与视图必备、缺席或集外 400 且不触读口；两格各有 outcome 词 `PENDING_EFFECTIVE_TIME_FACTS_LISTED` / `CURRENT_EXTERNAL_TRACKING_FACTS_LISTED`；走 `CatalogueQueryIntake`（ADR-0077/0078）；`effectiveAt` / `effectiveRule` / `supersedes` 只在判断过时在场 |
| `d2d270fa` | `POST /transport-fulfillment-effective-time-judgments`（`adapters/http/judge_effective_time.go`）：载荷 `{fact, effectiveAt}` 严格解码、无身份键、租户由 Intake 信封交进；时刻解不出 400、空着交编排答未受理 200；`EFFECTIVE_TIME_JUDGED` 201，`ALREADY_JUDGED_AS_GIVEN` / `INPUT_NOT_ACCEPTED` / `JUDGMENT_UNDECIDED` 200；带记录的答案把那一版含 `effectiveBasis`、`effectiveAt`、规则版本、`supersedes` 原样透出，`handoffReference` 只在意图没交出去时在场；`UnconfiguredIntake` 加 `IntakeEffectiveTimeJudgment` |
| `0a05dced` | `cmd/parcel-api`：两行进端点表（读行挂 `transportCatalogueIntake`，写行挂字面量 `UnconfiguredIntake{}`）、`buildEffectiveTimeJudgment`（事实登记册 + `ResultVersions` + `OutboxExternalTrackingFactHandoff` + 时钟，事务边界归装配点）、探针两行、unwired 两占位、隔离读放行面加读行；真库装配用例：判成 `EFFECTIVE_TIME_JUDGED` 回指前版、同值重放 `ALREADY_JUDGED_AS_GIVEN`、待判断视图清空、全部当前版带依据、Outbox 按主题认领到「外部承运轨迹已判断」信封 |
| `e7076574` | `apps/admin-web`：新页 `effective-time-judgment`（`pages/operations/EffectiveTimeJudgmentPage.tsx`），导航「作业与履约」区、`liveIds` 登记；源输入 + 两格视图切换 + 行动作「判断」开表单（事实引用与 RFC 3339 时刻两格，不预填时间、不收判断人）；判读与行转写在 `effective-time-judgment.ts`，node:test 12 条 |
| `0be8b4fd` | 机制清点在 `e7076574` 干净检出上重生成：TF 生产 106→116、http 16→19、迁移 14 份、接入面端点 90→93、端口声明 328→330，`EffectiveTimeRules` 从两份「缺」名单消失 |

**完成判据逐条**：读面与写面各有实现与测试——传输层用例（`query_external_tracking_facts_test.go`、`judge_effective_time_test.go`，编排是真处理器接内存替身）与真库用例（读口 2 条、装配 1 条）；「能在管理台上把一条待判断事实判成已判断并看到它进 VE 投影」——管理台页面已接两口，判成已判断这一段在真库装配用例上复现到**Outbox 里那一封意图**（`transport-fulfillment.external-carrier-tracking.judged`，按主题认领得到），VE 投影的派生由派发进程消费该信封完成，那一半是既有链路（票 `16` 的交接口），本票不重证；今天写口挂 `UnconfiguredIntake{}`，页面上提交必答 403「接入渠道未配置」，机制在、墙也在，页面文案分得开。`gofmt -l` 空、`go build ./...` / `go vet ./...` 退 0；无 DSN `go test -count=1 ./...` 95 包 ok；带 DSN `-v` 于 `internal/transportfulfillment/...` + `cmd/parcel-api/...` + `internal/architecture/...` + `migrations/...` **1358 PASS / 0 SKIP / 0 FAIL**，探针一正一反（`TestTheWiredEffectiveTimeJudgeAnswersHonestlyAgainstARealDatabase` 带 DSN PASS、去 DSN SKIP）；前端 `tsc --noEmit` 退 0、`node scripts/run-tests.mjs` 61 条全过；均在 `0be8b4fd` 的干净 detached 检出上跑（本机门禁容器 55432）。夹具全为合成 `S`，不含任何真实轨迹源取值。

**刻意留下的**：写面的准入换真随 ADR-0100 操作者渠道那批，本票只挂缝；批量按规则重判归票 `22`（它要的「按源列待判断当前版」读口即 `bd44a0d0` 那个端口，`PendingEffectiveTimeOnly` 一格）；读面按源上列、不做跨源汇总——「哪些源有待判断」是另一个问题，等真源出现再问。
