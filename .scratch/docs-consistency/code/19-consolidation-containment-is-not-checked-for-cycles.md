# 19 NO「集运包含无循环」未落代码，连单元装进自己也不拦

Category: bug
Status: resolved——2026-10-25 进 main（见文末「进 main 记录」）。此前 in-progress——完工待评审与进 main，分支 claude-code19（2026-10-21）。此前 ready-for-agent——R2（上线后尽快）：数据完整性。集运移入已在生产路径上（`cmd/parcel-frontline-import` 的集运导入），袋套袋是常态；成员关系可以成环。下游 TF 出发门禁展开成员时有环检测（`application/read_departure_gate.go` 记 `ConsolidationMembershipCycle`），不会放行或死循环，只会让那一袋停住——看得见的卡住；绕法是移出成环的成员。2026-10-21 文档自洽 02 立（用户「你作为业务和系统专家自决」授权），分支 claude-dc02。
Blocked by: 无
出处：[audit/nr-no.md](../audit/nr-no.md) M6（审计钉 `6b0074d0`；2026-10-21 按 main `67438a0f` 复核，现象仍在；下游环检测是复核时新读到的，降低了后果）。

## 现象与证据

- 文档：NO CONTEXT「集运包含关系必须无循环；每个作业实物同一时点最多一个直接物理父级」；GLOSSARY「集运单元」：「全部包含层级不得形成循环」；UC-NO-003 `AT-NO-039`：「集运单元嵌套形成循环 | 拒绝成员关系，原关系不被覆盖」。
- 代码：
  - `internal/nodeoperations/domain/consolidation_unit.go` 的 `ConsolidationUnit.AddMember` 只拒已关闭、已封装与重复成员。
  - `application/consolidate_parcels.go` 的移入只问成员自己的 `Containment.CurrentParent`，不查目标单元的祖先链，也不拒「成员就是单元本身」。
  - 嵌套成员确实存在（`ports.ConsolidationMemberAt.NestedUnit`）；`migrations/node_operations` 没有对应约束；`AT-NO-039` 没有测试。

## 三类

机制。无产品策略、无租户取值。

## 规模

S–M：移入时沿目标单元的父级链上溯（含自身），撞到成员即拒；与「同一时点最多一个直接父级」同一读口；补 `AT-NO-039`。

## 最小的诚实修法

移入编排在同一事务里读目标单元的当前祖先链；成员是目标自身或其祖先之一即答「包含循环」并拒绝，原关系不变。并发下两袋互装的竞态靠同一事务的行锁或按单元排序加锁挡住。

## 要裁的

无。

## 完成记录（2026-10-21，作者会话，分支 claude-code19，基于 main `bedb374b2`）

### 做了什么

- 领域 `domain.ContainmentLineage`（目标单元的直接父级链路：自身在先、逐级到根）与 `ClosesCycle`：成员是目标自身或某一级祖先即成环，交回从目标到那一级的那一段；成员与单元按同串认（ADR-0214 决定二）。
- 端口 `ports.ContainmentIndex` 加 `LineageForUpdate`：在事务里读链路，并取（租户）的事务级咨询锁把本租户此后的移入排到本事务之后；事务外拒；册已成环交回 `ports.ErrContainmentLineageCyclic`。
- 编排 `ConsolidateParcelsHandler.AddMember`：受理闸之后先取锁读链路、再读单元；判环先于单父级、先于三相；新结果格 `ContainmentCycle`（`CONTAINMENT_CYCLE`）带 `Lineage()`；链路读不动交回 error（ADR-0186 决定三同形）。
- 端口 `ports.ConsolidationStore` 加「读来改」`FindByIDForUpdate`（修订轮，见「评审记录与修订轮」）：在事务里读单元并持单元行锁到事务结束，事务外拒；移入、移出、封装、开封、关闭读单元一律走它，只读的 `FindByID` 不带锁。适配器取 `SELECT … FOR NO KEY UPDATE`。
- 适配器 `ConsolidationUnits.LineageForUpdate`：`pg_advisory_xact_lock(hashtext('node_operations.containment_current.tenant'), hashtext(tenant))` + 带 path 的递归 CTE（册已成环时停并报错，不死循环）。
- 迁移 `migrations/node_operations/0022_containment_current_no_self_containment.sql`：`CHECK (member_id <> unit_id)`——环里一行之内判得出的那一种。
- 一线导入 `consolidationRowDisposition` 加 `CONTAINMENT_CYCLE` → 被拒，报文按 ⊂ 逐级点名链路与断开它的动作（`REMOVE_MEMBER`）；装进自己另一句。
- 文档：UC-NO-003 阶段 3、`AT-NO-039` 行与「给开发的交接」表去掉缺口注，「已实现的是」补上查环一句；AT 盘点 `AT-NO-039` 由②移入①、文末追记，总览计数未改（推送方重算）。NO CONTEXT 与 GLOSSARY 早已写成现行，不动。

### 代决

1. **判环的锁粒度取租户，不按链上的行加锁。** 要锁的行走完链才知道是哪几行；边走边锁的次序随链而异，一笔向上走、另一笔正在闭环的会互等，只能靠 PG 判死锁让一方出错。把本租户的移入串成一队就够，前提是新边只由移入加：移出、封装、开封、关闭都经 `ConsolidationUnits.Update` 把单元的成员整份重新写进容纳索引，它们读单元持单元行锁（代决 8），写回的就只是当时已提交的那几条边，删边只会缩短链路。这条前提要靠代决 8 才成立——不持锁的读会把别的事务刚删的边写回，写回不判环、不在租户锁下（评审 B1）。无死锁、无重试，代价见越权风险点 1。
2. **「包含循环」单立结果格，不并入 `NOT_ACCEPTED`。** 恢复动作不同（核对是否装反、断开链上一级 vs 开封、换实例），且要带链路——与 `MemberElsewhereContained` 带对方标识同形（ADR-0029 按恢复动作分格）。形状照既有拒绝格：结果格、不交 error、不进未决哨兵。
3. **判环先于单父级。** 一只已在别处的袋被装进自己或装进它装着的袋，答「先从那里移出」会引人做一件做完仍成环的事；ADR-0029 要分的正是恢复动作。既有次序「结构事实先于三相」不变。
4. **锁序：移入先取租户锁、后取目标单元的行锁；移出、封装、开封、关闭只取单元行锁、从不取租户锁。** 租户锁在前，在租户队里排着的移入不占着单元行，别的口不陪着这一队等；别的口从不取租户锁，集运各口的锁序只有这一条，没有互等。一笔事务只锁一只单元的行：袋套袋时成员单元的行不读不写，它那一侧的边在容纳索引里按成员键记；判环读祖先链不锁行；一线导入一行一笔、一笔只调一口。同一单元两笔并发移入既在租户锁上、也在单元行锁上串起（评审 N1）。容纳索引按成员键的唯一约束也会让插入等在途事务，但移入取到行锁之前不写任何东西，别的口只动本单元的那几条边，等待只会指向持着行锁或租户锁的那一笔，不成环。
5. **库级约束只加自装 CHECK，不写触发器。** CHECK 引用不了别的行，递归关系守不住；本仓迁移里有 SQL 函数、没有用触发器表达业务不变量的先例，环由编排与锁守，库面只钉行内形状。
6. **册已成环时读链路交回 error、不答任何格。** 绕过适配器写出的两行互装是册坏了（ADR-0022「返回 error 就是没形成」），不是这一次移入能判的东西；哨兵与读不动分开，让日志分得出「重试不会好」。
7. **一线导入先于 UC 结果契约表。** UC-NO-003 结果契约表没有「成员在别处」一行，本票也不给「包含循环」加行：两者都是移入口对 CONTEXT 硬句的拒绝，验收表的 `AT-NO-039` 一行已经是它的出处。
8. **改单元的各口读单元一律持单元行锁；端口另开「读来改」`FindByIDForUpdate`，只读的 `FindByID` 不带锁。** 根上消掉单元行的丢失更新（评审 B1 的病根）：`Update` 整行改写、成员整份写回容纳索引，持着行锁读到的才是写回那一刻仍然成立的那一版。比改单元的各口都取租户锁少把同租户全部集运作业串成一队——移出、封装、开封、关闭只与同一只单元上的作业排队。只读那一问（开启撞已有实例的读回、已有结果的读回、导入报告、各读面）不取锁，不平白排队；读来改另起一口而不是给 `FindByID` 加参数，调用方一眼看得出这一读要交给 `Update`。
9. **行锁取 `FOR NO KEY UPDATE`，不取 `FOR UPDATE`。** 取的正是随后那条 `UPDATE`（不改键列）本来就要取的那一把，只是提前到读；读来改之间同样互斥，READ COMMITTED 下等到前一笔提交后读到的是提交后的那一版。`FOR UPDATE` 另外会挡住别的事务插入引用这只单元的行时做的外键核对（`FOR KEY SHARE`；容纳索引与来源事实登记都有外键指向单元），单元的键从不改，挡了换不来什么。协调方裁定原话写的是 `SELECT … FOR UPDATE`，这里取同一族里更窄、对本场景等效的那一把。
10. **N1 的守法随 B1 的修法换了。** 修前同一单元两笔并发移入只靠「租户锁先于单元那一读」串起，挪动次序（变异 M4）就让后一笔按旧读数盖掉前一笔；修后单元行锁自己就把它们串起，单挪次序不再丢成员，真库的同单元用例只在「挪次序且去掉行锁」时红。次序仍定为租户锁在前（代决 4 的理由），由编排用例按到达次序钉住（`TestUnitChangingStepsReadTheUnitForUpdateAndAddingQueuesOnTheTenantFirst`），M4 由它杀。

### 越权风险点

1. **租户级锁把同一租户内全部集运移入串成一队**（NO owner）。今天唯一生产装配是一线导入，一行一笔、本就串行；日后在线作业端多节点并发移入若在这把锁上排队，可改为按链上单元加行锁并接受 PG 死锁判定出错重试，端口 `LineageForUpdate` 的契约不变。
2. **更长的环没有库面约束**（NO owner）。造出环的来路有两种，都要等读链路时才报错——报的是读的人，不是写的人；TF 出发门禁的 `ConsolidationMembershipCycle` 仍是下游最后一道。一是绕过适配器直接写容纳索引，两行互装各看各的都合法。二是经适配器的 `Update`，交进去的单元不是 `FindByIDForUpdate` 读的：日后新写一口、或别的调用方拿只读的 `FindByID` 读了去改，旧读数照样把别的事务刚删的边写回，`Update` 自己分不出单元是怎么读来的。端口注释写明了契约、编排用例逐口钉住走读来改，但适配器不拦；要拦得给单元行加版本、`Update` 比版本（票 no-consolidation-fact-provenance/02 要裁的第 2 条），本票不扩。
3. **既有数据若已成环，本票不修也没有处置面**（NO owner）。装进那一袋的移入从此答 error（一线导入记 `NO_ANSWER_FORMED`、重跑不会好），要人直接断开容纳索引里的一边。产品尚无租户，开发库若撞上照迁移 0003 的办法清掉试验行。
4. **新结果格 `CONTAINMENT_CYCLE` 要每个翻译集运结果的消费方都有落点**（ADR-0025 全函数）。今天只有一线导入，已加；parcel-api 的集运端点落地时要照加，否则落进它的 `default` 支。
5. **`Lineage()` 是拒绝那一刻的读数**。锁随事务释放后链路可能已变；报文是给人核对纸单的，不是给机器据以续办的。
6. **锁序靠调用方一笔事务只调一口守着**（NO owner）。一笔做多步有两种互等：一是别的口之后再移入（比如先从一袋移出、再移入另一袋），锁序成了先单元行锁、后租户锁，与另一笔移入互等；二是两笔都不含移入、按相反次序推进同两只单元（一笔 `Seal U1` 再 `Seal U2`，另一笔 `Seal U2` 再 `Seal U1`），在两只单元的行锁上互等。PG 判死锁让一方出错回滚，不坏册，但那一笔没形成答案（一线导入记 `NO_ANSWER_FORMED`）。今天唯一装配是一线导入，一行一笔、一笔一口。parcel-api 的集运端点或在线作业端日后要一笔做多步时，拆成多笔，或先经 `LineageForUpdate` 取租户锁再推进——后者只有每一笔多步事务都先取租户锁时才同时救这两种：多步事务之间由租户锁串成一队，不在行锁上交叉；单步的移入仍是租户锁在先，单步的别的口只持一把行锁、不等第二把。调用约定写在 `ConsolidateParcelsHandler` 类型头注（`consolidationImporter` 头注的「一行一笔」引它），不写进 ports。
7. **票 no-consolidation-fact-provenance/02 的现象被收掉一半**（NO owner）。同一来源两份异内容并发写同一只单元时，单元行锁让输家那一步落在赢家之后，不再「盖掉赢家刚写进去的成员」；输家那一步仍随提交落册、答来源冲突，那张票的撤回问题不变。那张票的现象一节仍写着「可能盖掉」，本票不改别的票面，留给它的作者或推送方。

### 测试

- 先红：域测（`ContainmentLineage` 未定义）；编排用例答 `MEMBER_ADDED`；库面 CHECK 用例直插自装行成功（`err=<nil>`）；导入用例新格落入 `UNKNOWN(6)` 未决。
- 新用例：`internal/nodeoperations/domain/consolidation_lineage_test.go::TestAMemberAlreadyOnTheLineageClosesACycle`；`internal/nodeoperations/application/consolidate_parcels_test.go::TestNestingThatWouldCloseACycleIsRefusedAndTheOriginalRelationsStay`（另在 `TestConsolidationOwnStoreFailuresFormNoAnswer` 加「祖先链读不动」）；`internal/nodeoperations/adapters/postgres/consolidation_lineage_test.go` 的自装 CHECK、链路到根、册已成环报错、第二笔等第一笔提交与 `TestConsolidationWritesRefuseToRunOutsideATransaction` 加 `LineageForUpdate` 的无事务拒（PBC-08）；`cmd/parcel-frontline-import/consolidation_cycle_test.go::TestConcurrentMutualNestingAdmitsOnlyOneSideOnTheDatabase`（生产装配并发互装只成立一方）、`consolidation_vertical_test.go::TestFrontlineConsolidationImportRejectsWithoutRecording` 加装反与装进自己两行。全部 `-count=3` 绿。
- 变异（都被杀）：A 去掉租户锁 → 适配器并发用例「第二笔没有等第一笔提交就读到了链路 "bag-b"」、导入并发用例「第二笔…答了 MEMBER_ADDED」；B 判环跳过自身 → 域测、编排用例、导入用例（装进自己那行答 `MEMBER_ELSEWHERE_CONTAINED`）；C 去掉册已成环报错 → `TestAnAlreadyCyclicIndexIsAnErrorNotALineage`；D 单父级先于环 → 编排用例、导入用例同一行。
- `scripts/test-full.sh`：改动包 + `go list -test` 反向依赖共 65 包，加 `./internal/architecture/...`、`./migrations/...`，退 0（64 包 ok、2 包无用例）。`go build ./... && go vet ./...` 干净，`gofmt -l` 空。
- 一次性库 `c19seed` 灌演示种子成功，`containment_current_not_self_contained` 在位，用后已删。
- 机制清点与 AT 盘点计数未重生成（workflow「进 main 的步骤」归推送方）。

## 评审记录与修订轮

### 隔离评审（钉 `94051c962`，基 `bedb374b2`）

结论：不可合入。一条阻断、三条建议（经协调方转达）：

- **B1**：用旧读数写回单元的几口能把删掉的边写回，绕过判环与租户锁造出环；评审在真库复现（封装读完 A{B} 停住 → 导入把 B 从 A 移出 → 导入把 A 装进 B，B 的链路只有自己，答`已移入` → 封装按旧读数写回 B→A → 册上 A、B 互装）。失实前提在适配器 `LineageForUpdate` 头注、代决 1（「移出、关闭只删边」）与 `AddMember` 头注。
- **N1**：「锁先于单元那一读」无用例守着；变异 M4（`AddMember` 里把 `LineageForUpdate` 挪到读单元之后）全绿存活。
- **N2**：`domain.ContainmentLineage.ClosesCycle` 头注引 ADR-0214 决定二，那条比的是 PS 正式包裹号与单元号，不是这里的一对。
- **N3**：`AddMember` 头注「五口」只列名四个，且是计数引用；代决 4 同句。

### 修订轮（2026-10-25，接手的作者会话，分支 claude-code19）

先 `git rebase main` 到 `d19eeaafc`，退 0、无冲突（main 这一段没碰 `internal/nodeoperations`、`cmd/parcel-frontline-import`、`migrations/node_operations`）；原作者的提交随重放换了 SHA（`5b8bb2af2`、`4d86edae7`、`153c5e549`），内容未动。修订另起新提交。

**B1（阻断）——根上消掉单元行的丢失更新。**

- 修前复现：评审用例放进 `cmd/parcel-frontline-import/` 在重放后的尖上跑，红——封装答 `SEALED`、移出答 `MEMBER_REMOVED`、反向移入答 `MEMBER_ADDED`，事后 `CurrentParent(A)=B`、`CurrentParent(B)=A`，读 A 的链路报 `ErrContainmentLineageCyclic`。
- 修法（照协调方裁定）：端口 `ports.ConsolidationStore` 加「读来改」`FindByIDForUpdate`；适配器 `ConsolidationUnits.FindByIDForUpdate` 走 `RequireExecutor`、`SELECT … FOR NO KEY UPDATE`（代决 9）；编排移入、移出、封装、开封、关闭读单元一律换成它，开启撞已有实例的读回与 `existingResult` 仍走只读的 `FindByID`。移入仍先 `LineageForUpdate` 取租户锁读链判环，再取单元行锁（代决 4、8）。
- 锁序论证（写进 `AddMember` 头注与代决 4）：
  - 移入是「租户咨询锁 → 目标单元行锁」；移出、封装、开封、关闭只取单元行锁、从不取租户锁。全序只有这一条，等待图里没有「持行锁等租户锁」的边，不会互等。
  - 一笔事务只锁一只单元的行：移入只锁目标单元；成员本身是单元时它的单元行不读不写（那一侧的边在容纳索引里按成员键记）；读祖先链不锁行。一线导入一行一笔、一笔一口。一笔做两步的调用方会把锁序倒过来，记为越权风险点 6。
  - 容纳索引主键上的插入等待：移入取到行锁之前不写任何东西；别的口只插、删本单元的边，而本单元的边只由持本单元行锁的事务改，撞不上别人在途的同键元组。等待只指向持着行锁或租户锁的那一笔。
  - 环为什么守得住：单元 U 的边（`unit_id = U` 的容纳行）只由持 U 行锁的事务改，写回的一定是当时已提交的边；新边只由移入加，移入都在租户锁下读链判过环；删边只缩短链路。
- 改正的失实说法：适配器 `LineageForUpdate` 头注、端口 `LineageForUpdate` 注释（锁排的是此后的移入，不是「成员关系写入」）、`AddMember` 头注、`recorded` 头注里票 no-consolidation-fact-provenance/02 那句的成因、迁移 0022 注释、本票代决 1、4 与越权风险点 2（补上经适配器 `Update` 的那一种来路）；`Update` 头注写明只收读来改的单元。
- 回归用例：评审复现收成 `cmd/parcel-frontline-import/consolidation_cycle_test.go::TestAStaleUnitReadCannotWriteBackARemovedEdgeIntoACycle`。停住的替身改成只读、读来改两问都停，封装改回只读也停在同一处；改用池外一条连接轮询 `pg_locks` 看移出是否等在锁上，不睡固定时长（用例池只有两条连接，正被两笔在途事务占着——第一版从池里取，挂住了）。修后断言：移出等过锁，封装 `SEALED`，移出 `NOT_ACCEPTED`（读到的已是封装态），反向移入 `CONTAINMENT_CYCLE`；A 已封装且只装 B，B 空，A 无父级，读 A 的链路不报成环。收成的用例在「各口全改回只读」（即修前的编排）下红、修后绿。
- 另加：适配器 `TestFindByIDForUpdateHoldsTheUnitUntilCommitWhilePlainReadsDoNotWait`（第二笔读来改等第一笔提交、读到提交后那一版；只读那一问不等），编排 `TestUnitChangingStepsReadTheUnitForUpdateAndAddingQueuesOnTheTenantFirst`（逐口走读来改；移入先读链、后读来改），`TestConsolidationWritesRefuseToRunOutsideATransaction` 加读来改的无事务拒（PBC-08）。

**N1——收进同单元并发移入用例。** 评审用例收成同文件的 `TestTwoConcurrentAddsIntoOneUnitBothLand`：改用 `pg_locks` 看第二笔已在等，再放第一笔；另断言 X、Y 的容纳行都指向 A。修前的尖上施 M4，评审原用例 `-count=3` 三次全红（「A 的成员 = "SYN-HU-Y"……同一单元两笔并发移入有一笔被覆盖」）。修后单元行锁自己就把同单元两笔串起，单施 M4 它不再红（代决 10）；M4 由编排次序用例杀，同单元用例在「M4 且移入读单元不持锁」下红。

**N2——`ClosesCycle` 头注改引。** 改为：容纳索引以 `member_id` 为键，链路沿 `member_id = unit_id` 上溯；按时点答成员的读口按 `unit_id = member_id` 判 `NestedUnit`（ADR-0225 决定二及其 Context）。不再引 ADR-0214 决定二。

**N3——`AddMember` 头注不计数。** 随 B1 整段重写，直接点名移出、封装、开封、关闭，不带数；代决 4 同句改写。

**可选两格（自决）。**

- 「成员是目标的后代 → 不成环」补进编排用例 `TestNestingThatWouldCloseACycleIsRefusedAndTheOriginalRelationsStay`：A⊃B⊃C 时把 C 装进 A，答 C 在别处（B）、不带链路。领域层不另补：链路只装祖先，后代在领域用例里与「另一只袋」同形，补了只是重复那一格。
- 报文补开封：`describeContainmentCycle` 写成「先把 X 从 Y 移出（REMOVE_MEMBER；Y 若已封装，先开封 UNSEAL_UNIT）」。报文这一层不读 Y 的三相，条件句对封没封两种情形都说真话；`TestFrontlineConsolidationImportRejectsWithoutRecording` 加断言。

**变异（修后的尖，`scripts/test-full.sh -run …`），都被杀：**

| 变异 | 杀它的用例与报文 |
|---|---|
| 封装改回只读 `FindByID` | 回归用例「册上成环……移出等过锁=false」；编排次序用例「"find:unit-1"，要 "for-update:unit-1"」 |
| 移出改回只读 `FindByID` | 回归用例「第 2 行 factRef=SYN-WORK-B-OUT-OF-A → MEMBER_REMOVED [已落地]，要 NOT_ACCEPTED [被拒]」（移出盖掉了封装）；编排次序用例 |
| 各口全改回只读 | 回归用例（成环）；编排次序用例逐格全红 |
| M4：移入先读来改、后读链 | 编排次序用例「"for-update:unit-1,lineage:unit-1"，要 "lineage:unit-1,for-update:unit-1"」；真库的回归、同单元、互装用例不红，见代决 10 |
| M4 且移入读单元不持锁 | 同单元用例「A 的成员 = "SYN-HU-Y"」；编排次序用例 |
| 适配器读来改去掉 `FOR NO KEY UPDATE` | 适配器用例「第二笔读来改没有等第一笔提交就读到了成员 "hu-1"」；回归用例（成环） |
| 适配器读来改 `RequireExecutor` 换 `ReadExecutor` | `TestConsolidationWritesRefuseToRunOutsideATransaction`「无事务 FindByIDForUpdate 应返回 ErrTransactionRequired，实得：<nil>」 |
| 只读 `FindByID` 也带锁 | 适配器用例「只读那一问等在了行锁上」 |

**跑过的测试。**

- 并发与相关真库用例 `-count=5` 绿：cmd 的回归、同单元、互装、导入被拒；适配器读来改、无事务拒、链路各条。
- `scripts/test-full.sh`：改动包与 `go list -test` 算出的反向依赖，加 `./cmd/parcel-frontline-import/...`、`./internal/architecture/...`、`./migrations/...`，退 0（64 包 ok、2 包无用例）。`go build ./... && go vet ./...` 干净，`gofmt -l` 空。
- 迁移 0022 只改了注释，字节随之变了：一次性库 `c19seed` 灌演示种子退 0，`containment_current_not_self_contained` 在位，用后已删。0022 未进 main；施加过分支旧版 0022 的库校验和会对不上，那只会是一次性库，重建即可。
- 收尾核过：门禁库上无 `parcel_test_*` 库残留、无悬挂事务，专用测试实例都已停掉。

### 第二轮评审与收尾（2026-10-25）

第二轮评审：可合入，无阻断。协调方另要一小轮，只改注释与票面，新提交：

- 调用约定提到类型头注：`ConsolidateParcelsHandler` 加头注，写「一笔事务只调一口；要一笔做多步，先经 `LineageForUpdate` 取租户锁再推进，或拆成多笔」与锁序上的理由（两种互等）；`AddMember` 头注那句改为指向它；`consolidationImporter` 头注「一行一笔」的理由补上锁序这一层。越权风险点 6 补第二种互等（两笔都不含移入的多步事务在行锁上交叉），并写明先取租户锁只有每一笔多步事务都先取时才同时救这两种。
- 小处：适配器 `FindByIDForUpdate` 与 `Update` 头注里的「端口注释」改写成符号名 `ports.ConsolidationStore.FindByIDForUpdate`；回归用例头注改成现在时；端口「单元不在册时……」改为「不锁任何行」。
- `git rebase main` 到 `9ea060682`：`.scratch/at-coverage-inventory.md` 文末追记区与 main 的 code/31 那段冲突（原作者那笔与修订轮那笔各一次），两段都留、main 那段在前；总览计数不手算，留给进 main 时用工具重算。

## 进 main 记录（2026-10-25，协调方）

- 原协调会话派出的隔离评审在开工时因额度中断；接手的协调方重派评审。分支 `claude-code19` 经两次 rebase（作者到 `9ea060682`，协调方到含 code/25 的 main）后快进，tip `b476b1427`；迁移 NO 0022。
- 第一轮隔离评审一条阻断：B1 移出、封装、开封按旧读数经 `ConsolidationUnits.Update` 把别的事务刚删的边写回，绕过判环造出互装环（真库复现）。协调方裁定：会改单元的各口在事务内读单元持单元行锁，移入保留租户咨询锁判环。修订轮另派作者（原作者会话不可续）落 `ports.ConsolidationStore.FindByIDForUpdate`（`FOR NO KEY UPDATE`，代决 9）、收评审复现为回归用例，并收 N1–N3。第二轮评审可合入，N1（「一笔事务只调一口」的调用约定提到 `ConsolidateParcelsHandler` 类型头注、补行锁交叉那一种互等）在收尾轮改完。
- 协调方单独提交重算的 AT 计数与重生成的机制清点；ADR 索引无变化；检查均退 0。
- 与 code/25 同批验：全量 `scripts/test-full.sh` 退 0、158 个包 ok；`go build ./...`、`go vet ./...` 干净；演示种子在一次性库上灌通，用后删库。
- 去向：无新票。第二轮评审的真库探针说明票 no-consolidation-fact-provenance/02「可能盖掉赢家」那一半已随本票不成立，另测到同成员异证据并发移入答 `MEMBER_ALREADY_CONTAINED` 一格——协调方同一笔在 02 现象一节末尾追记。
