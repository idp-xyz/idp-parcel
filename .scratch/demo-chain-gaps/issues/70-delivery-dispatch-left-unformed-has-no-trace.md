# 70 要求缺失入账之后不留痕：待形成的派送查不到，补拍全凭人记

Category: enhancement
Status: in-progress——完工待评审与进 main，分支 claude-dcg70（2026-10-21 作者子代理，见文末「完成记录」；票 129 同批同分支）。此前 in-progress——2026-10-21 认领。此前 ready-for-agent——2026-10-20 上线就绪评估归 R1（上线前必修），裁定与批次见 [triage-2026-10-19](../triage-2026-10-19.md)「上线就绪评估（2026-10-20）」。此前 needs-triage——2026-10-19 票 [60](./60-delivery-dispatch-is-triggered-in-process-from-the-handover-envelope.md) 的作者子代理立（用户「你作为业务和系统专家自决……解决所有的问题」授权）。出自 [ADR-0216](../../../docs/adr/0216-delivery-dispatch-is-triggered-in-process-from-the-handover-envelope-per-object.md) 越权风险点 1「要求缺失不会自愈」。
Blocked by: 无

## 现象

- 派发进程随交接信封发起派送（ADR-0216）。三件派送要求里有一件，所有者答「没有」时，执行器答`要求缺失`，信封照决定三入账、不重投。
- 任务保持待形成（ADR-0114 决定三），但没有任何东西记下这一格：哪个对象、在哪个段、缺哪几件、什么时候答的，只出现在那一次答复里。派发进程对入账的答复不打日志，inbox 一行只说「已处理」。
- 所有者日后补齐了（PS 补收件地点、NR 计划带上交付段、PC 登了交付条件），也没有任何东西回头再发起。补拍口 `/transport-fulfillment-delivery-dispatch-triggers` 在，但谁该补拍、补哪几件，运营无从查起。

## 要裁的

- **留不留痕，留在哪**：TF 是否立一本「派送待形成」册（对象、段、入场依据、缺哪几件、最近一次答复时刻），由派送发起那一拍同笔写，任务形成时随之了结；查阅口挂运营查阅族。
- **补齐后谁来再发起**：
  - 只开查阅面，补拍仍靠人；
  - TF 对册上待形成的对象按拍重问。这不是 ADR-0216 否掉的「按拍批量发起」：重问只针对已知待形成的那几件，拍频仍要有人选；
  - 所有者补齐时交门铃。ADR-0114 Alternatives 否过「所有者推派送要求」，门铃只说「这一件补了」、不带内容，与那条否决不同，要另裁。
- 三类：留痕与查阅是机制；再发起的形态是产品策略；若取按拍重问，拍频是租户取值。

## 规模

M

## 代决复核追加（2026-10-20）

出处：[owner-review-20261020](../../owner-review-20261020/report.md) 的 M2、M8。

- **M2**：派送补拍口今天挂在场外作业范围的作业事实面上，调度员要先有一台场外设备才能补拍。本票裁「所有者补齐要求后，谁来再发起」时一并定这一口归哪一族：若由管理台调度员补拍，出新 ADR 把这一口移到运营决定能力面（加一个决定种类），并部分停用 ADR-0151 决定六末句与 ADR-0216 决定四那句；若只由一线设备补拍，在 ADR-0151 补复核记录改写风险点 7 的前提。
- **M8**：派送待形成册对「同一段同一对象的替代参与」按票 129 的同一认法处理，不另立任务。本票排在 129 之后或同批。

去向：两条都在本分支办，见文末「完成记录」；129 并给本分支同批做（协调方 2026-10-21 派单）。

## 完成记录（2026-10-21，作者子代理，分支 claude-dcg70）

基线 `640f67de`（main = origin/main）。授权：用户原话「你作为业务和系统专家自决……解决所有的问题」；裁定照 [triage-2026-10-19](../triage-2026-10-19.md)「上线就绪评估（2026-10-20）」关于 70 的那一条落，代决复核 M2、M8 与票 129 同批（协调方 2026-10-21 派单）。新 ADR 是 [ADR-0230](../../../docs/adr/0230-delivery-dispatch-left-unformed-is-kept-on-a-pending-register-and-reasked-through-the-same-executor.md)，裁定之外的细节按下文「代决」自决。

### 提交

- `7946b053` 认领；`5f0a29ad` 票 129 并入、照抄代决复核追加一节。
- `692cb407` 票 129：期望交接与沿替代链认前版任务（完成记录见票 129）。
- `591bfe7a` 派送待形成册：迁移 0045、端口、PG 适配器、执行器写册、两个进程的装配、用例。
- `57bf0b42` TF 查阅页 `registry=delivery-dispatch-pending`。
- `8260fb28` 运营决定新种类「派送重问」，access_identity 迁移 0020。
- `7beaaf5c` 派送重问间隔册：迁移 0046、领域、登记编排、HTTP 登记口、查阅页 `registry=delivery-dispatch-reask-cadence`。
- `4249a08d` 按册重问用例与 `/transport-fulfillment-delivery-dispatch-reasks`；补拍口改挂运营决定；parcel-api 新特性文件与真库用例。
- `1e7fd088` 演示种子新一节 `460-access-delivery-dispatch-reask.sh`。
- `9d258d69` ADR-0230；ADR-0216、0114、0151 前向指针；TF CONTEXT、UC-TF-006、参数登记册 `PAR-NET-19`、演示动线补记。
- `2277bcec` 立票 152、153；`dea49639` AT 覆盖盘点追记。
- `452f43fd` 本路哨兵分类用例、注释去计数；`57af00db` 改族后过期的两处注释改口。

### 落点

- TF：
  - 执行器 `application/trigger_delivery_dispatch.go` 多必备依赖 `Pending`，答复得出后由 `keepPendingRegister` 写册，与这一拍同笔：要求缺失立或刷新、已形成与已在册了结、对象不在段里或已离场作罢。册写不动整拍不答（`ErrDeliveryDispatchPendingUnavailable`）；装配没交册另一个错误（`ErrDeliveryDispatchPendingRegisterNotWired`）。
  - 端口 `ports/delivery_dispatch_pending.go`、`ports/delivery_dispatch_reask_cadence.go`；PG `adapters/postgres/delivery_dispatch_pending.go`、`delivery_dispatch_reask_cadence.go`。
  - 用例 `application/reask_pending_delivery_dispatches.go`、`register_delivery_dispatch_reask_cadence.go`；领域 `domain/delivery_dispatch_reask_cadence.go`。
  - HTTP：`reask_delivery_dispatches.go`、`register_delivery_dispatch_reask_cadence.go`、查阅页两格；补拍口 Intake 从 `WorkFactCommandIntake` 移到 `OperatorDecisionIntake`。
- 接入身份：`DecisionDeliveryDispatchReask`，access_identity 迁移 0020。
- parcel-api：新 `endpoints_delivery_dispatch_reask.go`（两行：按册重问挂运营决定、重问间隔登记挂登记册配置写）、`assemble_delivery_dispatch_reask.go`；`endpoints_delivery_dispatch.go` 那一行改面；`assemble_delivery_dispatch.go` 交真册；`unwired_orchestration.go` 只加查阅页两口占位。没改 endpoints.go、main.go、endpoints_test.go、assemble_operator_decisions.go。
- parcel-dispatch：`assemble_delivery_dispatch_on_handover.go` 交真册、本路未决哨兵名单加 `ErrDeliveryDispatchPendingUnavailable`；没加路由，没改 assemble.go、routes.go。
- 迁移：transport_fulfillment `0045`、`0046`；access_identity `0020`。

### 代决与越权风险点

裁定之外，本票自决（理由与反方都在 ADR-0230）：

1. **册在执行器里写，不包在外面**：三条再发起的路都调同一个执行器，册只有一处定义。
2. **册键（租户，段，对象），不带入场依据**（M8）：同段同对象只有一条参与替代链，更正长出的替代参与仍是同一条，入场依据记最近一次。
3. **册是待办台账，状态随最近一次答复改写，不留答复历史**；最近一次答复时刻按写入先后取。越权风险点 1、2。
4. **作罢只在重问那一拍发生**：参与结束的三条编排不同笔写册。越权风险点 3。
5. **`引用被占`、`不受理`不动册**。越权风险点 4。
6. **册写不动整拍不答、不答一格未决**：补拍口那一笔会让已形成的任务随空答复提交。
7. **再发起的形态**：内置运营按册批量重问（`ALL` / `DUE`），一次最多 200 件（产品常量），每件自成一笔事务，一件给不出答复时整口报错。越权风险点 5、6。
8. **重问节奏 = 重问间隔**：最近一次答复之后多久到期，租户登记（登记册配置写），最近登记的那一版生效，未登记到期重问答 `REASK_CADENCE_NOT_CONFIGURED`，产品与演示租户都不登。自动驱动另立票 [152](./152-delivery-dispatch-reask-has-no-in-process-driver.md)：它要动 parcel-dispatch 的 main 与装配，不在本票地盘。越权风险点 9、10、11。
9. **M2 取「由管理台调度员补拍」**：补拍口改挂运营决定。补拍与按册重问共用一种决定「派送重问」（同一种权力），线格式与答复不变。并进 ADR-0230（决定七），没另立 ADR-0234。部分停用 ADR-0216 决定四一句、ADR-0151 决定六末句派送发起口那一项。越权风险点 7、8。
10. **门铃另立票** [153](./153-delivery-requirement-owners-ring-no-doorbell-when-they-fill-in.md)。越权风险点 14。
11. **演示种子另起一节**（460），不改 450 与 400 的数据文件；不登重问间隔。

越权风险点逐条见 ADR-0230 文末，待 TF owner 复核，第 7、8 点同时待接入身份 owner。

### 各判据对应用例（真库标 ★）

- 交接触发答要求缺失，与 inbox 同笔落册、重投不重复写册：
  - ★ `cmd/parcel-dispatch/delivery_dispatch_pending_test.go::TestARequirementMissingOnTheHandoverLaneIsRecordedWithTheInboxAndSettledByTheRetick`：inbox 一行、册一条、两次投递册仍一条。
  - ★ 同文件 `::TestAnUndecidedOrFormedBeatLeavesThePendingRegisterAlone`：未决那一拍 inbox 与册都不落，顺利形成的不进册。
  - 同文件 `::TestThePendingRegisterBeingUnwritableIsRetriedAndItsAbsenceIsADefect`：本路哨兵分类。
- 补拍后任务形成、册上那一条了结：★ 上一条用例的后半；`application/delivery_dispatch_pending_test.go::TestAPendingEntryIsSettledWhenTheTaskForms`。
- 运营查阅页列出待形成清单，按租户隔离：
  - ★ `adapters/postgres/delivery_dispatch_pending_test.go::TestTheReviewCatalogueListsOnlyThisTenantsPendingDeliveries`。
  - ★ `cmd/parcel-api/delivery_dispatch_reask_test.go::TestOperatorsSeeThePendingDeliveriesAndReaskThemWithoutDuplicates`：经生产端点与操作者查阅渠道，别的租户那一条不出现。
- 批量重问复用执行器、幂等、不重复形成任务：
  - ★ 上一条 parcel-api 用例：两件形成、清单清空；再问零件，任务仍两项；再补拍答已在册。
  - `application/reask_pending_delivery_dispatches_test.go` 三条。
- 重问节奏未登记答未配置：
  - ★ `cmd/parcel-api/delivery_dispatch_reask_test.go::TestReaskingWhatIsDueNeedsTheTenantsRegisteredCadence`：未登记答未配置、所有者一次也不被问；登记 201、重放、冲突、不受理；登记后按最近一版筛，换长间隔后零件；查阅页列两版。
  - `application/...::TestReaskingWhatIsDueWithoutARegisteredCadenceAnswersNotConfigured`。
- 同段替代参与是同一条（M8）：`application/delivery_dispatch_pending_test.go::TestASupersedingParticipationIsTheSamePendingEntry`；★ 票 129 的两条派发真库用例。
- AT：UC-TF-006 `AT-TF-281`–`291`，逐条落点见 `.scratch/at-coverage-inventory.md` 本票追记。

### 验证

- `go build ./...`、`go vet ./...` 退 0；`gofmt -l cmd internal migrations tools` 无输出。
- `scripts/test-full.sh` 跑改过的包、它们的反向依赖（含全部 cmd 包）、`./internal/architecture/...`、`./migrations/...`：65 个包，61 ok、0 FAIL，其余 4 个没有测试文件。清单在 scratchpad `dcg70-revdeps.txt`。代码定在 `57af00db`。
- 演示种子：55432 上一次性库 `dcg70seed` 跑 `seed.sh` 退 0。新一节三行答 `REGISTERED` / `RECORDED`；授予、对照落库，重问间隔册 0 行。用完 `DROP ... WITH (FORCE)`。
- 每一笔提交单独可编译：中间几笔在提交时都对工作树跑过 build 与 vet；`7beaaf5c` 另经 `git archive` 导出单独 build、vet 通过。
- 没跑全量，没重生成机制清点、ADR 索引与 AT 计数。`tools/adr-index -check` 只报 README 过期（预期）；在 scratch 副本上生成，0230 一行与三份前向注记都解析得出。

### 没做与去向

- 按间隔自动重问的驱动：票 152。
- 所有者门铃：票 153。
- 待确认更正为`已交接`不进段的根：票 130（main 上已立）。在它之前，补拍与按册重问不核期望交接（ADR-0230 越权风险点 12）。
- 演示动线没在一次性库上逐步实走，只写了补记；取证靠上列真库用例。
- 管理台页面（apps/admin-web）没做：查阅格与两口都已在 API 上。
- 进 main 时要协调方处理：
  - access_identity 迁移 0020 的号与 CHECK 并集。并行票 psb18 占了 0018、0019，若它们也重建这条 CHECK，后落地的一份要取并集。
  - 演示种子 `data/access/09`、`10`、`data/governance/42` 与 460 一节的号，可能与并行票撞。
  - 参数登记册 `PAR-NET-19` 的号。
  - 票面与 ADR 指向票 130 的链接，要 main 上的票 130 文件才连得上。

