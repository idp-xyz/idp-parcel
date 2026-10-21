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

基线 `640f67de`（main = origin/main），评审之后变基到 main `c1828dd1`（下列 SHA 都是变基之后的）。授权：用户原话「你作为业务和系统专家自决……解决所有的问题」；裁定照 [triage-2026-10-19](../triage-2026-10-19.md)「上线就绪评估（2026-10-20）」关于 70 的那一条落，代决复核 M2、M8 与票 129 同批（协调方 2026-10-21 派单）。新 ADR 是 [ADR-0230](../../../docs/adr/0230-delivery-dispatch-left-unformed-is-kept-on-a-pending-register-and-reasked-through-the-same-executor.md)，裁定之外的细节按下文「代决」自决。

### 提交

- `95ae2613` 认领；`e8ee6394` 票 129 并入、照抄代决复核追加一节。
- `276e81ea` 票 129：期望交接与沿替代链认前版任务（完成记录见票 129）。
- `a23f0ec7` 派送待形成册：迁移 0045、端口、PG 适配器、执行器写册、两个进程的装配、用例。
- `678c6db9` TF 查阅页 `registry=delivery-dispatch-pending`。
- `9d5d269c` 运营决定新种类「派送重问」，access_identity 迁移 0019（变基时由 0020 改号，CHECK 以 main 上 0018 为底）。
- `a80620f9` 派送重问间隔册：迁移 0046、领域、登记编排、HTTP 登记口、查阅页 `registry=delivery-dispatch-reask-cadence`。
- `2054e0cd` 按册重问用例与 `/transport-fulfillment-delivery-dispatch-reasks`；补拍口改挂运营决定；parcel-api 新特性文件与真库用例。
- `ca41f99e` 演示种子新一节 `455-access-delivery-dispatch-reask.sh`（变基时由 460 改号）。
- `a37f8270` ADR-0230；ADR-0216、0114、0151 前向指针；TF CONTEXT、UC-TF-006、参数登记册 `PAR-NET-19`、演示动线补记。
- `a645e534` 立票 152、153；`a3fac644` AT 覆盖盘点追记。
- `bf45a240` 本路哨兵分类用例、注释去计数；`232d8f99` 改族后过期的两处注释改口；`d4a55fe4` 两张票的完成记录。
- 评审之后：
  - `c3080387` ADR-0230 改指迁移 0019 与种子 455；
  - `1b306b19` 三处过期注释、票 129 Status、演示补记；
  - `11d043e9` 要求缺失不改回已形成；
  - `d533e777` 按最近一次问排队、remaining；
  - `e8df75c5` 确定性拒绝归缺陷；
  - `dade3c30` 形成锁；
  - `192e4f7f` 两条真库用例；
  - `4efd5c43` 间隔带登记依据；
  - `08512b57` ADR、UC、CONTEXT、PAR、AT 随之改写；
  - 本笔：评审记录。

### 落点

- TF：
  - 执行器 `application/trigger_delivery_dispatch.go` 多必备依赖 `Pending`，答复得出后由 `keepPendingRegister` 写册，与这一拍同笔：要求缺失立或刷新、已形成与已在册了结、对象不在段里或已离场作罢。册写不动整拍不答（`ErrDeliveryDispatchPendingUnavailable`）；装配没交册另一个错误（`ErrDeliveryDispatchPendingRegisterNotWired`）。
  - 端口 `ports/delivery_dispatch_pending.go`、`ports/delivery_dispatch_reask_cadence.go`；PG `adapters/postgres/delivery_dispatch_pending.go`、`delivery_dispatch_reask_cadence.go`。
  - 用例 `application/reask_pending_delivery_dispatches.go`、`register_delivery_dispatch_reask_cadence.go`；领域 `domain/delivery_dispatch_reask_cadence.go`。
  - HTTP：`reask_delivery_dispatches.go`、`register_delivery_dispatch_reask_cadence.go`、查阅页两格；补拍口 Intake 从 `WorkFactCommandIntake` 移到 `OperatorDecisionIntake`。
- 接入身份：`DecisionDeliveryDispatchReask`，access_identity 迁移 0019。
- parcel-api：新 `endpoints_delivery_dispatch_reask.go`（两行：按册重问挂运营决定、重问间隔登记挂登记册配置写）、`assemble_delivery_dispatch_reask.go`；`endpoints_delivery_dispatch.go` 那一行改面；`assemble_delivery_dispatch.go` 交真册；`unwired_orchestration.go` 只加查阅页两口占位。没改 endpoints.go、main.go、endpoints_test.go、assemble_operator_decisions.go。
- parcel-dispatch：`assemble_delivery_dispatch_on_handover.go` 交真册、本路未决哨兵名单加 `ErrDeliveryDispatchPendingUnavailable`；没加路由，没改 assemble.go、routes.go。
- 迁移：transport_fulfillment `0045`、`0046`；access_identity `0019`。

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
11. **演示种子另起一节**（455，数据 `data/access/09`、`10`、`data/governance/45`），不改 450 与 400 的数据文件；不登重问间隔。

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
  - access_identity 迁移号：变基后落 0019，CHECK 以 main 上 0018 为底取并集（0019 词集 = 0018 词集 + 1）。
  - 演示种子：变基后落 455 一节、`data/governance/45`；`data/access/09`、`10` 在 main 上是空号。
  - 参数登记册 `PAR-NET-19`：main 上是空号。
  - 指向票 130 的链接：变基后 main 上有票 130，连得上。

## 评审记录（2026-10-21，隔离评审，无阻断项）

变基到 main `c1828dd1`，之后按评审逐条修。

### 变基

1. **access_identity 迁移**：0020 改为 0019。CHECK 以 main 上 0018 那一份为底，加 `DELIVERY_DISPATCH_REASK`；`decisionKinds` 取并集，`grep` 核过 0019 词集 = 0018 词集 + 1（25 → 26）。授予读回用例加一种 0018 的词。
2. **演示种子**：`460-access-delivery-dispatch-reask.sh` 改为 455，`data/governance/42` 改为 45；`data/access/09`、`10` 不撞。README 授予表那一行取两边并集。55432 上一次性库 `dcg70seed2`：`seed.sh` 全程退 0，新一节三行 `REGISTERED` / `RECORDED`，关务三种与派送重问的授予都在库里；用完 `DROP ... WITH (FORCE)`。
3. **PAR-NET-19**：main 上没被占，照用。
4. **票 129、130**：129 票面以 main 那份为底，只多认领、完成、评审三节；指向票 130 的链接能连上。
5. **文本冲突**：票 70 的追加节、README 授予表都取并集；AT 盘点、TF CONTEXT 与 ADR 自动合并。

### 必修

1. **三处过期注释改口**（`1b306b19`）：`assemble_delivery_dispatch.go` 的事务包装头注、`trigger_delivery_dispatch.go` 文件头注、`delivery_dispatch_task_reference_test.go` 头注。
2. **票 129 Status** 改为「in-progress——完工待评审与进 main，分支 claude-dcg70（与 70 同批）」（`1b306b19`）。
3. **按册重问队头卡死**（`d533e777`）：
   - 迁移 0045 加 `last_asked_at`，每问一次就记、不论答复。答复动不了状态的那几格走新口 `MarkAsked`，只记问过。排队与到期都按它。
   - 队列口加 `CountPending`，结果与答复带 `remaining`；查阅页多一列 `lastAskedAt`。
   - 真库用例 `cmd/parcel-api/...::TestUndecidedEntriesDoNotStarveTheRestOfTheReaskQueue`：队头两件一直未决、上限 2，第一次答 `remaining=1`，第二次先问到后面那一件并形成。另有编排与 PG 用例。
   - ADR-0230 决定四、五与越权风险点 4、5、9 改写。
4. **`RecordMissing` 不改回已形成**（`11d043e9`）：`DO UPDATE ... WHERE state <> 'FORMED'`，替身同规则，PG 用例证 task_ref 不被清。

### 顺手

5. **确定性拒绝归缺陷**（`e8df75c5`）：
   - PG 写口把没有环境事务、SQLSTATE 23xxx、缺件为空包上 `ports.ErrDeliveryDispatchPendingWriteRefused`。
   - 执行器据它交 `ErrDeliveryDispatchPendingRefused`，不进哨兵；其余写不动仍交 `ErrDeliveryDispatchPendingUnavailable`。
   - 用例：PG、编排、派发哨兵名单各一条。
   - 别的 SQLSTATE 类的确定性错误仍会被当成写不动，记进 ADR-0230 越权风险点 15。
6. **并发窗口**（`dade3c30`）：取「锁」不取「只记风险」。
   - 册口加 `HoldFormation`，按（租户，段，对象）取事务级咨询锁。执行器先占住再读段、查在册，同一件的两拍排成一前一后，替代链两版并发也不铸第二项。
   - tf-face-algebra/09 的并发首拍用例原来要两拍在取件处会合，锁让它们会合不了，改证新语义：一笔 201、一笔已在册、所有者只被问一轮。
   - 手工建任务口不取这把锁，记进越权风险点 16。
7. **两条真库用例**（`192e4f7f`）：
   - 一件给不出答复时整口 500，前面那一件的任务与了结已提交，再按一次接着问（`cmd/parcel-api/...::TestAReaskThatHitsAnUnanswerableEntryKeepsWhatItAlreadyCommitted`）。
   - 册写不动时，在同一笔事务里真执行一条必错语句：inbox、任务与册一起回滚，恢复后重投形成（`cmd/parcel-dispatch/...::TestAnUnwritablePendingRegisterRollsBackTheInboxTheTaskAndTheRegisterTogether`）。
   - 为此派发那一路的册改由装配交入，生产交真册。
8. **重问间隔带登记依据**（`4efd5c43`）：迁移 0046 加 `basis`（非空）；构造门要依据，同版本换依据是冲突；载荷、答复与查阅页带 `basis`。生效区间没加，记进越权风险点 10，交 TF owner。
9. **演示补记**改指「票 demo-chain-gaps/60 那条补记」（`1b306b19`）。

### 未采纳

无。第 6 条在锁与只记风险两条路里选了锁；第 8 条只补依据、不补生效区间，理由见上。

### 验证（评审之后）

- `go build ./...`、`go vet ./...` 退 0；`gofmt -l cmd internal migrations tools` 无输出。
  - 中间一笔 `11d043e9` 的端口注释不合 gofmt，`d533e777` 修正；每一笔单独都能 build 与 vet。
- `scripts/test-full.sh` 跑 `./internal/transportfulfillment/...`、`./internal/accessidentity/...`、改过包的全部反向依赖（含全部 cmd 包）、`./internal/architecture/...`、`./migrations/...`：65 个包，61 ok、0 FAIL，其余 4 个没有测试文件。清单在 scratchpad `dcg70-revdeps2.txt`。
- 种子见上「变基」第 2 条。

### 二轮（2026-10-21，只看修订那一段 `d4a55fe4..9916c6ba`）

一条阻断、两条建议、四条顺手，都在本分支修，没有 rebase。

1. **阻断：「先取锁、再读段、再查在册」没被测试钉住**（`a53b1a4f`）。
   - 真库并发首拍 `TestConcurrentFirstTicksFormOneTaskAndTheOtherReadsTheWinner`：
     - 先到的那一拍问地点时停住，另开一条连接轮询本库 `pg_locks`（`locktype='advisory' AND NOT granted`、只看本库）。看到等锁的事务才放行，10 秒看不到判「后一拍没在形成锁上等」。
     - 其余断言照旧：所有者只被问 3 次、一笔 201、一笔 200、任务 1 行。
     - 看锁另开连接，是因为测试池只有两条连接，两拍各占一条，再从池里取会互等。
   - 编排层 `TestEveryBeatHoldsItsEntryBeforeAnswering`：段替身读段时记下 `len(pending.held)`，断言等于 1。
   - **变异实测**（在 scratchpad 的副本里做，工作树没动）：
     - `HoldFormation` 改成空操作 → 真库用例红：「后一拍没在形成锁上等：先到的那一拍停在取件处 10 秒，库里始终没有等在咨询锁上的事务」（10.40s）。
     - 取锁挪到 `answer()` 之后 → 编排层用例红：「读段时已占过的件数 = [0], want [1]——先占后读」。
   - 「任务口撞键读回赢家」那一道：执行器那几路已由形成锁排开，真库上不可再达。现在由替身层 `application/trigger_delivery_dispatch_supersession_test.go::TestATaskLandingBetweenTheCheckAndTheWriteIsReadBackAsTheWinner` 守着：查在册看不见、任务口撞键，执行器读回赢家答已在册。
2. **建议：替身的 err 让写册失败的用例停在取锁那一步**（`a53b1a4f`）。
   - 替身拆出 `holdErr`；`TestTheBeatGivesNoAnswerWhenThePendingRegisterCannotBeWritten`、`TestARefusedPendingWriteIsADefectNotAWait` 加断言，证真走到了答复之后写册那一步。
   - 新增 `TestABeatThatCannotHoldItsEntryGivesNoAnswer`：取锁读写不动或没有事务时，不读段、不问所有者、不写册。
3. **建议：持续报错的那一件卡队头**（`822c75b8`）：照「出错时另起一笔事务给它记一次 MarkAsked」做了。
   - 按册重问新依赖 `Failed`（`ReaskFailureMarker`），parcel-api 交事务包装的真册；没交时这一口不问。错误带段与对象。
   - 用例：编排 `TestAnUnanswerableEntryIsMovedBehindTheOthers`；真库 `TestAReaskThatHitsAnUnanswerableEntryKeepsWhatItAlreadyCommitted` 扩为三次按：
     - 第一次停在 I，I 排到队尾；
     - 第二次 I 仍报错，但先形成 J；
     - 修好后第三次形成 I。
   - `Reask` 文档注释与 ADR-0230 越权风险点 6 写明：那一笔也记不下（库面整个读写不动）时，它仍停在查阅页队头，修好之前每次重问都先停在它身上。
4. **顺手：锁的形式**（`70a983b0`）：改用本仓双 int 形式——第一键是锁空间名散列，第二键是三件以 chr(31) 拼接的散列。注释写明双 int 与迁移执行器的单 bigint 会话锁不同空间，撞上只让同一空间里两件不相干的派送多等。
5. **顺手：注释口径**（`734ab7c4`）：
   - 端口：写到行的那几口才记问过；`HoldFormation` 不写行，`RecordMissing` 遇已形成的不写。
   - `ErrDeliveryDispatchPendingUnavailable`：取锁失败时还没有答复。
   - HTTP `Remaining`：未受理、未配置、未决时也是零，那时看 outcome。
6. **顺手：「问」的口径**（`1038d1db`）：PAR-NET-19 与 ADR-0230 写明「问」指交执行器跑一拍，不一定问到所有者（迟到的前版信封、缝没接线、任务册读不动也刷新）。
7. **顺手**（`a53b1a4f`、`734ab7c4`）：
   - 并发用例注释去掉变更叙述。
   - `assemble_delivery_dispatch_on_handover.go` 的哨兵名单注释改为按类说明，不再逐个列别的包的错误符号，过长那一行收短。

**未采纳**：无。

**验证**：
- `go build ./...`、`go vet ./...` 退 0；`gofmt -l cmd internal migrations tools` 无输出。
- 本轮每一笔改 Go 的提交都经 `git archive` 单独导出，build 与 vet 通过。
- `scripts/test-full.sh ./internal/transportfulfillment/... ./cmd/parcel-api/ ./cmd/parcel-dispatch/ ./internal/accessidentity/... ./internal/architecture/... ./migrations/...`：21 ok、0 FAIL，另 1 个包没有测试文件。

