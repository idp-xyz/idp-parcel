# 126 TF 运营决定各口不记提交操作者

Category: bug
Status: in-progress——2026-10-26 作者完工，待隔离评审与进 main，分支 `claude-dcg126` tip `58eec6559`（其后一笔只改本票面），见文末「完成记录」。此前 in-progress——2026-10-26 会话 `tops-20`（Claude Code）认领（code/01 已进 main），分支 `claude-dcg126`（worktree `idp-parcel-dcg126`）；新 ADR 取 0303，TF 迁移取 0051。此前 ready-for-agent——2026-10-20 代决复核立（上线前必修），见 [owner-review-20261020](../../owner-review-20261020/report.md)「应修订 → 去向」与「须你裁定 → 已代决」
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
