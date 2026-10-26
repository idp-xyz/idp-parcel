# 126 TF 运营决定各口不记提交操作者

Category: bug
Status: resolved——2026-10-26 进 main `039abf840`（见文末「进 main 记录」）。此前 in-progress——2026-10-26 评审无阻断、修订轮完，待进 main，分支 `claude-dcg126` tip `560c46c88`（其后一笔只改本票面），见文末「评审记录」「修订轮」。此前 in-progress——2026-10-26 作者完工，待隔离评审与进 main，分支 `claude-dcg126` tip `58eec6559`（其后一笔只改本票面），见文末「完成记录」。此前 in-progress——2026-10-26 会话 `tops-20`（Claude Code）认领（code/01 已进 main），分支 `claude-dcg126`（worktree `idp-parcel-dcg126`）；新 ADR 取 0303，TF 迁移取 0051。此前 ready-for-agent——2026-10-20 代决复核立（上线前必修），见 [owner-review-20261020](../../owner-review-20261020/report.md)「应修订 → 去向」与「须你裁定 → 已代决」
Blocked by: 无
出处：用户 2026-10-20「要我裁定的部分，你作为业务和系统专家，请你自决吧」；本会话代决复核报告 [owner-review-20261020](../../owner-review-20261020/report.md) 的 M1；ADR-0151 越权风险点 5

## 现象与修法

现象、证据与修法的完整写法以复核报告 M1 一条为准（单一权威），此处只记要点：

把信封里的提交操作者作为操作证据记在各运营决定记录上，待遇照 ADR-0187（只进审计，不进重放比对，也不进任何门）；须新 ADR 或在 ADR-0151 补复核记录后落地。

## 完成记录（2026-10-26，Claude Code 作者子代理，会话 `tops-20` 派单，分支 `claude-dcg126`）

起点：`origin/main` `3e40b51e0`（含认领）。

| 提交 | 内容 |
|---|---|
| `5f2d6cd53` | 代码、TF 迁移 `0051` 与各层用例 |
| `58eec6559` | ADR-0303；ADR-0151 Status 注记与 Links；UC-TF-004 / 005 / 006 立 `AT-TF-335`–`341`；AT 盘点①七行；补执行器形成的任务不记人一条用例，两处 Covers 逐号点名 |
| 完成记录那一笔 | 本节与 Status 行 |

### 接票时核实（读码于 `3e40b51e0`）

M1 的现象成立：`tfhttp.OperatorDecisionIntake` 对关段、建派送任务、装载分配、终止参与、有效时间判断（`/transport-fulfillment-effective-time-judgments`）、承运商首次有效收寄判断（`/transport-fulfillment-carrier-first-effective-pickup-judgments`）与运输委托提交七口走 `tenantFor`，认证出的操作者在 Intake 里丢掉。M1 写的「两个判断口」就是这两口。已经记人的是费用发生登记（`registered_by`）、发生项修订、解除采用与委托协议换版。另核得：同在运营决定能力面、同样只取租户的还有订舱提交、取消订舱、取消运输委托、履约段采用运输委托、容量预占、容量释放与派送补拍、按册重问，M1 没列，见代决 1、2。

### 做了什么

- **ADR-0303**（新）：定「记」，待遇照 ADR-0187 三条；部分停用 ADR-0151 决定二「TF 各口：信封给租户，其余照各口既有载荷。」一句（只限七口），回答其越权风险点 5。ADR-0151 只改 Status 行末尾的索引注记（加一段来向）与 Links（加一行），正文不动。ADR-0187 不动。
- **迁移** `migrations/transport_fulfillment/0051_operation_decisions_record_the_submitting_operator.sql`：`actual_fulfillment_segment.closed_by`、`fulfillment_participation.terminated_by`、`dispatch_task.opened_by`、`load_assignment.decided_by`、`external_carrier_tracking_fact.effective_judged_by`、`carrier_first_effective_pickup.judged_by`、`transport_commission.submitted_by`，各一道 CHECK（不空白；关段人只在已关闭的段上、终止人只在 `CONTROL_TERMINATED` 的参与上、判断人只在 `JUDGED_EXPLICITLY` 的版本上）。
- **领域**：`domain/operation_evidence.go`（`OptionalRegistrantReference`、`RegistrantReference.Present`）；段聚合 `CloseSegment(at, by)`、`EndParticipationWithTermination(…, by)`、`ClosedBy()`、`TerminatedBy()`，重派生承继终止人；重建规格 `ClosedBy`、`TerminatedBy` 与错位拒收。
- **端口**：`DispatchTaskRecord.OpenedBy`、`LoadAssignmentRecord.DecidedBy`、`ExternalTrackingFactRecord.EffectiveJudgedBy`、`CarrierFirstEffectivePickupRecord.JudgedBy`、`TransportCommissionRecord.SubmittedBy`；段登记册 `CloseSegment` 多收关段人；`ExternalTrackingFactReviewRow.EffectiveJudgedBy`。
- **PostgreSQL 适配器**：七张表读写新列（`adapters/postgres/operation_evidence.go` 一处 NULL 落法）；段登记册 `EndParticipation` 写终止人、`Supersede` 的 `lockedInheritedEnd` 连同终止人按锁后的在册值承继；`Replace` 不碰提交人。
- **编排**：七个命令各加一格（`ClosedBy`、`TerminatedBy`、`OpenedBy`、`DecidedBy`、`JudgedBy`×2、`SubmittedBy`），只做形状核（空白即缺席、NUL 答未受理）后挂到记录上；重放、比对与各门一行未改。
- **HTTP**：七口 Intake 改走 `identityFor`，七个 `…Payload` 的 `Command`（终止口是 `Termination`）改收 `OperatorIdentity`；写面回执不变；收寄链查阅面 `judgedBy`、轨迹事实查阅面 `effectiveJudgedBy`（omitempty）。两处注释（`OperatorIdentity`、TF 认证方）改成现行口径。
- **装配**（`cmd/parcel-api`）：无改动——七口原本就挂运营决定 Intake，操作者随认证结果自然进来。`GET /transport-fulfillment-records` 各册不展示这七种记录，不改。管理台不改。
- **TF CONTEXT 未改**（一句都没动）：它没有一句说运营决定只收租户或不记人，规则写在 ADR-0303。
- **UC**：UC-TF-004 立 `AT-TF-335`（委托提交）、`AT-TF-336`（装载分配）；UC-TF-005 立 `AT-TF-337`（关段）、`AT-TF-338`（明确终止）、`AT-TF-339`（收寄判断）、`AT-TF-340`（有效时间判断）；UC-TF-006 立 `AT-TF-341`（建派送任务）。号段全仓与在途分支核过：`AT-TF-330` 归 147、`331`–`334` 归 127，`335` 起无人用。AT 盘点①按号补七行；文末未加追记（避开与 127 同处追加）；总览计数未动。

### 代决与越权风险点（用户 2026-10-20「要我裁定的部分，你作为业务和系统专家，请你自决吧」）

1. **范围取 M1 列名的七口**（机制）。订舱提交与取消、取消委托、段采用委托、容量预占与释放同一判据也该记，本票不落：订舱与容量池两组的存储与读面正由在途分支 `claude-dcg127` 改（`capacity_pool.go`、`review_catalogue.go`、`query_transport_fulfillment_records.go`、订舱读面），并进来就与它改同一批文件。ADR-0303 决定五与越权风险点 1 写明「另票落地、同为上线前必修」。**请协调方立后继票**（编号未占）。owner 若要求本票一并落：同一套落法（命令一格、记录一格、`0051` 再加列或新迁移、查阅面随订舱读面一起带出）。
2. **派送补拍与按册重问不记在任务上**（机制）。读了 ADR-0216 决定四、ADR-0230 决定四至七：两口只让执行器再问一次所有者，形成的任务内容与进程内那一拍同源，不是这位操作者的决定；执行器形成的任务 `opened_by` 留空（`TestATaskFormedByTheDispatchTriggerRecordsNoOpener` 钉）。按重问的人归访问留痕，ADR-0100 族另裁。owner 若要记：派送待形成册「最近一次问」旁加一列，或执行器命令带一格只落在由补拍形成的任务上。
3. **形状取 TF 既有的「发行方#sub」，不取 PS 的 `OPERATOR/` 前缀**（机制）。照票面「照 `RegisteredBy` 的形状，别另立第二套」。
4. **写面回执不带操作者**（机制）。照 ADR-0187 决定四；与费用发生登记回执回显登记者（ADR-0207 越权风险点 7）口径不一，ADR-0303 越权风险点 2。
5. **两种落点**（机制）。关段与终止挂在段聚合上（终止人要随替代版本承继），其余五种挂在 `ports.*Record` 上（与登记时刻并列，领域门结构上读不到）。ADR-0303 决定一、越权风险点 6。
6. **列可空、不回填、不立 NOT NULL**（机制）。存量无源可填；同表有执行器、规则、素材到达与交付/交接结束形成的行。读作「没记下是谁」。
7. **NUL 操作者答未受理**（机制）。存储形状门，与各身份值同一条纪律；不是按人放行。空白读作缺席，编排层可不带操作者照样形成——这是「不进任何门」的可测形状。
8. **CONTEXT 不写**（产品策略边界）。ADR-0303 越权风险点 7：owner 若认定它属领域语言，写进 TF CONTEXT「运营决定」相关句。
9. **没读的**：UC-TF 各篇除验收示例外的正文，ADR-0100、0202、0203、0204、0232 正文（ADR-0303 越权风险点 8）。

### 修法 → 用例

| 修法（报告 M1）/ 派单要求 | 用例 |
|---|---|
| 信封里的提交操作者进命令，载荷不收 | `adapters/http/operation_evidence_test.go::TestEachOperationDecisionFaceCarriesTheEnvelopeOperatorIntoItsCommand`（七口逐口；载荷自报即 400） |
| 各口记录加列、读回带出 | 领域 `domain/operation_evidence_test.go` 两条；编排 `application/operation_evidence_test.go` 七口各一条；真库 `adapters/postgres/operation_evidence_test.go` 三条（段与参与、三种登记册记录、两种判断记录） |
| 重放与内容冲突比对不含操作者（同一决定另一操作者重放仍答已有），每口一条 | 编排：关段`早已关闭`、终止`已离场`、任务`已在册`、分配`已在册`、有效时间`已按同值判过`、收寄判断`已在册`与`内容冲突`、委托`已有结果`与`冲突`，在册操作者都不变；真库：各表撞键 / 窄口写空不顶替；生产装配真库 `cmd/parcel-api/operation_decision_operator_test.go::TestEachOperationDecisionRecordsTheOperatorWhoMadeItAgainstARealDatabase`（两个令牌、七口各投两次，库里那一列是首投者，回执不含操作者） |
| 任何门不读它 | 编排各条「不带操作者照样形成」；领域「带与不带走出同一个终点 / 关闭」 |
| 存量为空 | 真库各子用例「不带操作者落 NULL、读回没记下」；重建门照收存量 |
| 查阅面带出 | 收寄链 `::TestTheCarrierPickupChainShowsTheJudgeAndTheJudgmentReceiptDoesNot`；轨迹事实 `::TestTheTrackingFactReviewShowsTheExplicitJudge`；真库 `ListCurrentExternalTrackingFacts` |
| 终止人随替代版本承继 | 领域 `TestTheClosingAndTerminatingOperatorsAreKeptWithoutGatingTheSegment`；真库 `TestTheSegmentRegistryKeepsTheClosingAndTerminatingOperators`（替代口按锁后的在册终点承继） |

改前核对（临时改坏、跑、改回）：`lockedInheritedEnd` 不按锁后的在册值承继终止人 → `TestTheSegmentRegistryKeepsTheClosingAndTerminatingOperators` 红（替代版本终止人为 NULL）；关段窄口去掉 `closed = false` → 同一用例红（另一位操作者再关写中一行）；关段 Intake 丢掉操作者 → `TestEachOperationDecisionFaceCarriesTheEnvelopeOperatorIntoItsCommand/segment_closure` 与 `cmd/parcel-api` 真库用例红（关段人 NULL）；委托重放按提交人分冲突 → `TestACommissionSubmissionRecordsItsSubmitterAndAReplayByAnotherKeepsIt` 红（答 COMMISSION_CONFLICT）。四处改回后全绿。

### 自验

于 `58eec6559`（其后一笔只改本票面）：`gofmt -l cmd internal migrations tools` 无输出；`go build ./...`、`go vet ./...` 退 0；带 DSN（`127.0.0.1:55432`，pgtest 每例自建自删库）`go test -count=1` 跑 `./internal/transportfulfillment/...`、`./internal/architecture/...`、`./migrations/...`，以及 `go list` 反查（含测试导入）依赖 TF 的 `./cmd/parcel-api/...`、`./cmd/parcel-dispatch/...`、`./cmd/parcel-transport-register/...` 与 NR / NO / PP / PS / SA / VE 六个 `adapters/transportfulfillment` 包，全部 ok。新真库用例 `-v` 核过是 PASS 不是 SKIP（`adapters/postgres` 全包 338 条 PASS、零 SKIP；`cmd/parcel-api` 新用例六个子用例 PASS）。`go run ./tools/adr-index -check`、`go run ./tools/at-coverage-counts -check` 只报生成物过期（ADR 索引、总览计数，归推送方）；票 139 那一版 `at-coverage-counts` 对本树跑，`AT-TF-335`–`341` 不在缺行与多行之列。全量 `scripts/test-full.sh` 未跑（归协调方）。

进 main 时请留意：AT 盘点①本票七行与 `claude-dcg127` 的 `AT-TF-331`–`334` 插在同一处（`AT-TF-330` 之后），重放时两段按号顺接；UC-TF-004 与 127 各改不同的行。

## 评审记录（2026-10-26，协调方 `tops-20` 隔离子代理，钉 `87724244a`：两轴无阻断，非阻断 6 条；照抄，小节标题降一级挂在本节下）

评审 ← tops-20 隔离子代理 · 钉 `87724244a` · 01:38

结论：两轴无阻断；非阻断 6 条。

### Standards
阻断：无。
非阻断：
1. `adapters/accessidentity/operator_authenticator.go` `AuthenticateOperatorDecision` 注释「其余运营决定口把它作为操作证据记在决定上（ADR-0303）」不实——订舱提交与取消、取消委托、段采用、容量预占与释放、派送重问仍走 `tenantFor`，操作者在 Intake 丢了。宜改成「关段等七口」。
2. ADR-0303 越权风险点 4 说「只能从接入日志里找」，但产品里没有逐请求接入日志（`internal/platform/httpapi` 无日志中间件，`cmd/parcel-api/main.go` 只记启动配置）。改写成「产品内无处可查」；ADR 合入即冻结，宜在本分支改。
3. 重点 4 不构成单一权威问题（ADR-0187 决定四只管 PS 两口，ADR-0207 决定三只管发生项，ADR-0303 决定三只管七口，各在一处定义）。但越权风险点 2 只点了费用发生登记，漏了同上下文里回执同样回显决定人的两口：委托协议换版（`commissionAgreementRevisionBody.DecidedBy`）与解除采用（`withdrawnBy`），宜补全。
无发现：红线与边界；ADR-0151 只追加注记与 Links、正文未动；去向/来向合 adr/README；注释无行号位置词。

### Spec
阻断：无。
非阻断：
1. 完成判据不缺格（M1 原文恰是七口）。其余六口在 ADR-0303 决定五与越权风险点 1 披露清楚，但后继票（上线前必修）未立，需协调方立。
2. 派送补拍与按册重问「不记在任务上」站得住（ADR-0230 决定七）。但「派送重问」这一决定本身至今哪里都没留下是谁，与 M1 同属「无从补记」，宜并进上一条后继票，不只挂在风险点 4。
3. AT 盘点七行与 main 上 `AT-TF-331`–`334` 插在同一处且未写追记；main 也改过 UC-TF-004。重放时手合。
无发现：七口 Intake 都走 `identityFor`、载荷自报即 400（`TestEachOperationDecisionFaceCarriesTheEnvelopeOperatorIntoItsCommand`）；各口重放与冲突判据都不读操作者（关段 `ErrSegmentClosed`→`segmentAlreadyClosed`；终止 `!current.Active()`；任务、分配按键答已在册；有效时间只比依据与时间；收寄判断按 TFC-1 形状比；委托只比 `ContentDigest`）；生产装配真库用例两令牌各投两次、库里是首投者、回执无操作者。段上两件只经 `ClosedBy()`/`TerminatedBy()` 与持久化读；`lockedInheritedEnd` 交回含 `terminated_by` 的整行；`Supersede` 是 `ON CONFLICT DO NOTHING` 无内容比对。迁移 0051 三值逻辑无漏洞（`closed`、`effective_basis` NOT NULL，`end_kind` 先判 IS NOT NULL；真库用例对在场行写终止人被 CHECK 拒 23514）。ADR-0151 被停句逐字、适用场景只限七口。新号不撞。

## 修订轮（2026-10-26，作者子代理，同一分支）

逐条处理评审非阻断（都在本分支改）：

- **Standards 1**：`adapters/accessidentity/operator_authenticator.go` `AuthenticateOperatorDecision` 注释改为「关段等七口把它作为操作证据记在决定上（ADR-0303）」。
- **Standards 2**：ADR-0303 越权风险点 4 改为「在那张票落地之前，按过补拍或重问的人在产品内无处可查——产品没有逐请求的接入日志」。
- **Standards 3**：ADR-0303 越权风险点 2 补上委托协议换版（回执 `decidedBy`）与解除采用（回执 `withdrawnBy`）两口同样回显决定人。
- **Spec 1、2**：立后继票 [180](./180-the-remaining-tf-operation-decisions-and-the-delivery-dispatch-reask-record-the-submitting-operator.md)（ready-for-agent，上线前必修，理由同 M1）：订舱提交、取消订舱、取消运输委托、履约段采用运输委托、容量预占、容量释放六口记提交操作者，并收进「派送补拍、按册重问是谁按的」——不记在任务上，另记在重问决定处（ADR-0230 决定七、ADR-0303 决定五）。立票前核过 180 在 main、各在途分支与各 worktree 都空着。ADR-0303 决定五两条、Status 索引注记、越权风险点 1、4 与 Links 改指 180；决定五第一条原写的「访问留痕归 ADR-0100 族另裁」随之改为归 180，Links 里 ADR-0100 一行去掉「访问留痕的归属」半句。
- **Spec 3**：rebase 时手合，见下。
- 另：rebase 之后票 127 已在 main，ADR-0303 Context、决定五与候选庚里「正由在途票 127 改动」改为「起草时正由票 127 改动」。

rebase 到 `origin/main` `d80e8d8f8`：唯一冲突在 `.scratch/at-coverage-inventory.md` ①——main 上 127 的 `AT-TF-331`–`334` 与本票 `AT-TF-335`–`341` 插在同一处，按号顺接、两边都留；本票没写文末追记，那里无冲突。UC-TF-004 自动合并，127 的 `AT-TF-332`–`334` 与本票 `AT-TF-335`、`336` 都在。迁移 `0051`、ADR 号 0303、`AT-TF-335`–`341` 在新 main 上仍无人占。重放后的提交：`8b1700131`（原 `5f2d6cd53`）、`389d5053a`（原 `58eec6559`）、`410f4c721`（原 `87724244a`）、`560c46c88`（修订轮）。

修订轮自验，于 `560c46c88`（其后一笔只改本票面）：`go run ./tools/at-coverage-counts -check`（main 上已带票 139 的缺行 / 多行核对）只报总览计数过期，无缺行多行；`go run ./tools/adr-index -check` 只报索引过期；`gofmt -l cmd internal migrations tools` 无输出；`go build ./...`、`go vet ./...` 退 0；带 DSN（`127.0.0.1:55432`）`go test -count=1` 跑 TF 全部包、`internal/architecture`、`migrations`、`cmd/parcel-api`、`cmd/parcel-dispatch`、`cmd/parcel-transport-register` 与六个依赖 TF 的 `adapters/transportfulfillment` 包，全部 ok。

## 进 main 记录（2026-10-26，推送方会话 tops-20）

- **main**：`039abf840`。出处：分支 `claude-dcg126` tip `099b6d167`（作者修订轮后自行 rebase；推送方在 main `4bb6c2fa0` 上重放）。rebase 前出处：`5f2d6cd53`、`58eec6559`、`87724244a`。
- **main 上的提交**：`f445c668e`（代码、TF 迁移 0051）、`8d19b6588`（ADR-0303、UC `AT-TF-335`–`341`）、`2ee075747`（完成记录）、`49fde3fa0`（修订轮，立后继票 180）、`d6f902051`（评审记录与修订轮）。
- **生成物**：ADR 索引 `24f581955`；AT 盘点总览计数 `bf368e651`；机制清点 `039abf840`。三件 `-check` 退 0（含票 139 表行守卫）。
- **评审**：隔离子代理钉 `87724244a`，两轴无阻断、非阻断 6 条：Standards 1–3 在修订轮收；Spec 1、2 立 [180](./180-the-remaining-tf-operation-decisions-and-the-delivery-dispatch-reask-record-the-submitting-operator.md)（其余六口与派送重问记提交操作者，上线前必修）；Spec 3（盘点相邻行）由作者 rebase 时按号顺接（见「评审记录」「修订轮」）。修订轮只收非阻断，推送方读了修订 diff，未另派复评。
- **全量真库测试**：`scripts/test-full.sh` 跑在 `039abf840`（即所推 SHA）：158 ok / 0 FAIL，退 0。
- **种子**：`scripts/demo-seeds/seed.sh` 灌一次性库（`039abf840` 上），退 0，用后删库。
- **号**：TF 迁移 `0051`；ADR-0303；`AT-TF-335`–`341`。
