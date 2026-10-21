# 42 出发门禁读数不展开集运单元成员：一袋被扣包裹的境内出发读成不受门禁

Category: enhancement
Status: in-progress——完工待评审与进 main，分支 claude-dcg42。2026-10-20 认领。此前 ready-for-agent——2026-10-19 分诊为 A（挡演示/上线），批次见 [triage-2026-10-19](../triage-2026-10-19.md)。此前 needs-triage——2026-10-19 Claude Code 会话立（用户「你作为业务和系统专家自决……解决所有的问题」授权）。出自 [32](./32-movement-facts-declare-segment-and-record-departure-gate-reading.md) 的隔离评审（非阻断项，评审建议进 main 前写明后果、另立票）。
Blocked by: 无（NO 那一侧的端口在另一会话 idp-parcel-9b 的地盘里变动，落地前要与那边对齐读口形状）
出处：[ADR-0205](../../../docs/adr/0205-departure-gate-is-a-pulled-reading-recorded-with-the-movement-fact-not-a-refusal.md) 越权风险点 5；[ADR-0206](../../../docs/adr/0206-customs-action-admission-judgement-folds-three-sources-and-judges-cross-zone-by-node-customs-zones.md) 越权风险点「对象只认包裹」（「展开成员由消费方做」）。

## 现象

出发门禁读数逐对象问关务，对象取段里出发那一刻在控的参与。段里在控的是袋或集运单元时，关务按包裹引用解析不到它：跨区出发答未决，境内出发答不受门禁。于是一袋里装着被扣包裹的境内出发读成不受门禁。那是一格假的干净读数，不是未决，VE 据阻断派生异常时也看不见它。

## 证据（读码）

- `internal/transportfulfillment/domain/departure_gate_reading.go` 的 `ObjectsUnderControlAt`：只取段的参与对象，不看对象是什么。
- `internal/transportfulfillment/adapters/customscompliance/departure_gate.go` 的 `ReadDepartureGate`：载运对象引用按同一串译成关务的包裹引用。
- `internal/customscompliance/application/judge_action_admission.go`：不在册的对象，跨关务区域移动答未决、其余动作答不受门禁（ADR-0206 决定四）。
- `internal/transportfulfillment/domain/offsite_pickup.go` 的 `CarriedObjectReference`：引用不分包裹与集运单元，本上下文此刻判不出「这一件是袋」。

## 三类

机制（集运单元在出发那一刻的成员怎么取、成员与单元本身怎样各记一格）。规模 M。

## 最小的诚实修法

TF 新开消费侧端口读 NO 的集运成员关系（`internal/nodeoperations/ports/catalogue_read.go` 里有成员关系册的读面，按出发时点取当时成员）。出发读数对集运单元按成员逐件问关务，单元本身那一格取成员里最重的一格，成员各自一格存进逐对象明细。成员读不动时，单元那一格记读不动、依据指名 NO 那一口。

在那之前若要先止损，可以让「本上下文判不出这一件是不是包裹」的对象落未决而不是照收关务的不受门禁；但这要先让载运对象带上种类，比展开成员的改动更宽，不建议单做。

## 要裁的

- 成员取出发那一刻的成员，还是封签那一刻的成员快照（CONTEXT「封签连续且相符的封闭集运单元……成员随单元受控」）。
- 嵌套单元（袋套袋）展开到哪一层。
- 归 TF owner，并要 NO owner 会签读口形状。

## 既有票

none

## 完成记录（2026-10-20，Claude Code 作者子代理，分支 claude-dcg42，基于 main `8bf7a231`）

### 做了什么

- **查清走同一读口的路**：出发登记（`RecordMovementFactHandler` 之前的 `DepartureGateReader.Read`）与设备出发前问询（`PreviewDepartureGateHandler.Preview`）共用 `readIntent`，两条一并修；同内容重投回显在册读数、不重问，不涉及。交出与交付门禁读数是另两套读法（各一格），不在本票，立票 122。
- **NO 那一侧**（只加新文件）：ports 新开 `ConsolidationMembershipAtView`（`consolidation_membership_at.go`），postgres 新只读查询 `ConsolidationMembershipsAt`：按（租户，对象，时点）答是不是本租户的集运单元、那一刻的直接成员（移入、移出事实按业务发生时间取最后一条，同刻按记录时刻、来源标识定序）、成员本身是不是单元；时点之前已关闭的实例不装东西；读不动作为错误交回。成员身份照旧问已发布的 `IntakeIdentityView`、由 `JudgeIdentityBasis` 认。
- **TF**：新消费侧端口 `ports/consolidation_membership.go`、新适配器包 `adapters/nodeoperations`（两口 NO 读面 → 成员种类：包裹、嵌套单元、身份不成立、身份读不动）；`DepartureGateReadDeps` 多一口 `Members`、缺即构造拒；`readIntent` 的逐对象问改为 `departureGateExpansion`：先问 NO 是不是单元，不是按包裹问关务，是就按成员逐件问、逐层展开到包裹，每件只问一次只一格，沿路径防环；说不出是不是单元的那一件记读不动（`CONSOLIDATION_MEMBERSHIP_UNAVAILABLE` / `_UNTRANSLATABLE`），不按包裹问关务。
- **领域**：`domain/consolidation_gate_finding.go`——单元那一格（不带关务判断标识、指名在同一份读数里各有一格的成员），四种缘由（身份不成立、无成员答未决；身份读不动、成环答读不动）记依据、不按同格筛；`FormConsolidationUnitGateFinding` 推答复，`RehydrateConsolidationUnitGateFinding` 照存的重建；出发读数重建门在整份读数上核单元与成员对得上、不成环；交出与交付读数拒单元那一格。
- **存取**：逐对象那一格加可缺的 `members` 键（单元才有，空单元写空数组），存量读数照读，**不迁移**（TF 0045 未用）。
- **线格式**：出发与问询答复的 `gateReading.objects[]` 在单元上带 `members`；成员那几格照旧只有对象与格。
- **VE**：`signalBlocked` 跳过单元那一格——信号落在袋里被扣的那一件上，不给袋另立一次。现有派生本就逐对象明细取阻断，展开之后成员的阻断自然进得去；只需挡住袋那一格。
- **装配**：`cmd/parcel-api/assemble_movement_fact.go` 的 `buildDepartureGateReader` 接 NO 两口真适配器（只改这个既有装配文件，未动 endpoints / main / assemble / routes）。
- **文档**：ADR-0225；ADR-0205（部分停用决定一、决定二各一句，仅限单元那一格）、ADR-0206（越权风险点 8 出发那一半已答）、ADR-0219（读数多两源）的 Status 行与 Links 加前向指针与索引注记，正文不改，ADR 索引未手改；TF CONTEXT「出发门禁读数」词条与 Rules 各补一句；CONTEXT-MAP `node-operations ↔ transport-fulfillment` 补一句；UC-TF-005 立 `AT-TF-276`–`280`、UC-VE-004 立 `AT-VE-183`、UC-NO-003 立 `AT-NO-129`–`130`；AT 盘点加行与追记（计数未改）；立票 122（交出读数不展开）、123（追踪把袋当一件包裹投）。

### 代决（用户原话「你作为业务和系统专家自决……解决所有的问题」）

1. **成员取出发那一刻，不取封签快照**：按集运成员关系事实（`consolidation_fact`）的业务发生时间取。理由：成员关系是 CONTEXT 定义的「适用时间内直接位于」的包含关系；快照只在封装那一刻冻结，开放单元没有、重封单元有多份，封签异常时 CONTEXT 不许依快照推成员；单元封着时两者本就一致。也不取此刻容纳索引（离线补登时袋可能已拆，同 ADR-0205 决定四）。备选：快照（否）、此刻索引（否）。
2. **嵌套展开到包裹为止、不设层数上限**：停在第 N 层，被扣包裹在 N+1 层就回到假干净读数；沿当前路径防环（环上那一边记缘由、答读不动），数据有限必收尾；对象与成员定序走，同源同读数。备选：一层、设上限（否）；NO 一口答完全部间接成员（不作本次形状，记越权风险点 2）。
3. **判「是不是单元」靠问 NO 单元册**（判据同 ADR-0214 决定二，同串按单元判），不给载运对象引用加种类（那要改一线入口与各口契约，更宽）。说不出种类时那一件读不动、不按包裹问——把读不动答成「不是单元」就是那一格假干净。
4. **问不到关务的成员不另立一格**，记成单元那一格的依据：作业实物标识不是包裹引用，进了逐对象那几格 VE 会拿它当包裹立信号，与包裹号同串时还会撞格。
5. **单元那一格的缘由不按同格筛**（与 ADR-0206 决定五不同）：这几件成员没有自己的一格，筛掉就哪里都查不到。
6. **空单元与已关闭单元答未决**（缘由「单元无成员」），与「段里没有在控对象」同格。
7. **成员身份取此刻**（不取出发那一刻）：身份是「是哪件包裹」的事实，重核与处置纠正的是先前认错。
8. **袋那一格不形成阻断信号**，信号只落在问过关务的那几格。
9. **线格式加 `members`**：执行方要知道被挡的那一件在哪一袋里。
10. **存量读数不迁移**：新键可缺，缺即问关务问来的那一格。

### 越权风险点（详见 ADR-0225 文末）

1. **NO owner 会签读口形状**：新接口 `ConsolidationMembershipAtView`、新只读查询与 `AT-NO-129`–`130`（及 UC-NO-003 两行）落在 NO 地盘（原归会话 idp-parcel-9b，已不在线）；NO 既有文件一个未改。成员按业务时间取、同刻定序、关闭后无成员、嵌套按单元册认，都是代 NO 定的读法。
2. 「是不是单元」的 EXISTS 判据与 `ResolveHandoverObject` 写在两处（判据相同）；间接包含由消费方逐层派生（NO owner）。
3. 成员按业务发生时间取，受设备时钟偏差影响（同 ADR-0205 越权风险点 10）。
4. 成员身份取此刻：问询与随后出发之间身份被重核时两份读数不同。
5. 每件段对象多一次 NO 读、每件成员多一次收寄读；NO 成员事实那一本坏了，连纯包裹的段也每件读不动（运维）。
6. 成环、身份链说不准与读不动同记读不动，确定性与暂时未分（ADR-0205 越权风险点 2 同一缺口，票 114）。
7. 单元那一格依据不按同格筛，读者要按缘由词认所属的格（TF owner）。
8. 执行方看得到袋的成员（同 ADR-0205 越权风险点 13）。
9. VE：追踪仍把袋当一件包裹投（票 123）；袋那一格不立信号是代 VE owner 定的。
10. 交出读数仍不展开（票 122）。
11. 空单元、已关闭单元答未决：回空载具若登成开放实例随车走会带来未决噪音。

### 用例

- 领域：`internal/transportfulfillment/domain/consolidation_gate_finding_test.go`（取最重、缘由各格、嵌套、重建门核对与形状门、交出交付不收单元那一格）。
- 编排：`internal/transportfulfillment/application/read_departure_gate_consolidation_test.go`（袋不问关务、成员逐件问；说不出种类与成员身份问题不读成不受门禁；嵌套只问一次、成环收尾、空袋未决；封闭集外成员上抛；问询与出发同读数）；既有 `read_departure_gate_test.go` 补缺口构造门一条。
- 适配器：`internal/transportfulfillment/adapters/nodeoperations/consolidation_membership_test.go`（身份经 NO `JudgeIdentityBasis` 译、时点按登记精度、读不动不读成不是单元）。
- 线格式：`internal/transportfulfillment/adapters/http/departure_gate_consolidation_test.go`（袋那一格带 `members`、问询逐字节相同）。
- VE：`internal/visibilityexception/adapters/transportfulfillment/derive_on_movement_fact_consolidation_test.go`（`AT-VE-183`）。
- 真库：NO `internal/nodeoperations/adapters/postgres/consolidation_membership_at_test.go`（`AT-NO-129`–`130`）；TF `internal/transportfulfillment/adapters/postgres/movement_fact_consolidation_gate_reading_test.go`（往返、被改不装回）；`cmd/parcel-api/assemble_movement_fact_consolidation_test.go`——**境内 SHA→SZX 出发，袋里一件被限制装载出发、一件干净：读数阻断而不是 NOT_GUARDED，袋那一格取阻断、列出两件成员，成员明细可见，库面存成员，问询同一份**；收寄册读不动时袋那一格读不动、依据指名那件作业实物；成员事实读不动时那一件读不动、关务判断册不多一行；**笼套袋逐层到阻断；空单元未决；没有收寄记录的成员让袋未决**。证据一律 `S`。

### 验证

- `go build ./...`、`go vet ./...`、`gofmt -l` 干净；逐笔代码提交（`7016f6ff`、`1358d25c`、`a58f6d4b`、`b6928f6e`、`5479126e`）各自单独 `go build ./... && go vet ./...` 通过（临时 worktree 逐笔检出验）。
- `scripts/test-full.sh`（专用一次性实例、走 socket）跑：触到的包及其反向依赖（含 `cmd/parcel-api`、`cmd/parcel-dispatch`、`cmd/parcel-frontline-import`、`cmd/parcel-node-operations-register`、`cmd/parcel-transport-register`，NO / TF / VE 各适配器与应用包，NR、PS、PP、SA 的 TF / NO 消费侧适配器）、`./internal/visibilityexception/...`、`./internal/architecture/...`、`./migrations/...`、`./tools/...`：全部 ok，0 FAIL。未跑全量。
- 未重生成 ADR 索引、AT 计数与机制清点（`adr-index -check` 与 `at-coverage-counts -check` 此刻如预期报不一致，留给进 main 时重生成）。

## 评审记录（2026-10-20，隔离评审，无阻断项；修订在同一分支 claude-dcg42，不 rebase，main 已到 `2a7cab9c`，进 main 时由协调方重放）

1. **展开缓存按对象引用共用，成员包裹走 `parcel()` 不认种类**——已修（`0f61f99d`）。成员认出的包裹号恰与段里另一件单元同串、且袋先被展开时，原先按包裹问关务记下一格，那件单元随后取到它就永不展开，可能读出假干净，违背 ADR-0214 决定二。取的修法：每个对象引用（段里的与成员认出的）都先经 `object()` 问节点作业是不是单元，再问关务；`parcel()` 只在认出「不是单元」之后调。不取「缓存按（种类，对象）分开」：读数里一个对象引用只能有一格，分开就会出现同一引用两格、读数立不起来。代价是每件成员多一次成员关系读，ADR-0225 决定四、后果与越权风险点 5 随之改（`23c9f3d6`，ADR 尚未进 main，正文就地改）。用例 `TestAMemberParcelNamedLikeAUnitIsStillReadAsTheUnit`（`AT-TF-278`）：去掉修法即失败。
2. **AT-NO-129 补恰在时点的三条边界**——已修（`5513c5f5`）。`TestFactsAtExactlyTheMomentCount`：业务时间恰等于问询时点的移入算在、移出不在、关闭之后不装任何东西（关闭前一微秒照成员事实答），与 `ObjectsUnderControlAt` 含起点的口径一致；UC-NO-003 那一行写明（`23c9f3d6`）。
3. **TF `MovementFacts.FindByKey` 重建失败与读不动同形，VE 包成 `ErrMovementFactNotVisible` 进未决名单**——已登记到票 [114](./114-deterministic-failures-answered-as-retryable-undecided-across-consumers.md)（`ed36a5db`），点名本票新增的几种只能由越权写入造成的装不回情形，修法方向「`FindByKey` 重建失败也包 `ports.ErrMovementFactRecordUnreadable`，VE 归记录不一致」；本票不改。
4. **AT-TF-275 空号**——取成本低的一种：AT 盘点本票追记写明 275 空着（票 53 号段 273–277 只用了 273–274，本票从 276 起），不补编（`ed36a5db`）。
5. **NO 适配器「是不是单元」的 EXISTS 写了两遍**——按评审意见不修，已记 ADR-0225 越权风险点 2。

修订后验证：`go build ./...`、`go vet ./...`、`gofmt -l` 干净；两笔修订代码提交各自单独 build/vet 通过；`scripts/test-full.sh` 覆盖触到的包及其反向依赖（含 `cmd/parcel-api`、`cmd/parcel-dispatch`、`cmd/parcel-frontline-import`、`cmd/parcel-node-operations-register`、`cmd/parcel-transport-register`）、`./internal/visibilityexception/...`、`./internal/architecture/...`、`./migrations/...`、`./tools/...`，52 个包 ok、0 FAIL。

