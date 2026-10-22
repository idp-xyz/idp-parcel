# 19 NO「集运包含无循环」未落代码，连单元装进自己也不拦

Category: bug
Status: in-progress——完工待评审与进 main，分支 claude-code19（2026-10-21）。此前 ready-for-agent——R2（上线后尽快）：数据完整性。集运移入已在生产路径上（`cmd/parcel-frontline-import` 的集运导入），袋套袋是常态；成员关系可以成环。下游 TF 出发门禁展开成员时有环检测（`application/read_departure_gate.go` 记 `ConsolidationMembershipCycle`），不会放行或死循环，只会让那一袋停住——看得见的卡住；绕法是移出成环的成员。2026-10-21 文档自洽 02 立（用户「你作为业务和系统专家自决」授权），分支 claude-dc02。
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
- 适配器 `ConsolidationUnits.LineageForUpdate`：`pg_advisory_xact_lock(hashtext('node_operations.containment_current.tenant'), hashtext(tenant))` + 带 path 的递归 CTE（册已成环时停并报错，不死循环）。
- 迁移 `migrations/node_operations/0022_containment_current_no_self_containment.sql`：`CHECK (member_id <> unit_id)`——环里一行之内判得出的那一种。
- 一线导入 `consolidationRowDisposition` 加 `CONTAINMENT_CYCLE` → 被拒，报文按 ⊂ 逐级点名链路与断开它的动作（`REMOVE_MEMBER`）；装进自己另一句。
- 文档：UC-NO-003 阶段 3、`AT-NO-039` 行与「给开发的交接」表去掉缺口注，「已实现的是」补上查环一句；AT 盘点 `AT-NO-039` 由②移入①、文末追记，总览计数未改（推送方重算）。NO CONTEXT 与 GLOSSARY 早已写成现行，不动。

### 代决

1. **锁的粒度取租户，不按链上的行加锁。** 要锁的行走完链才知道是哪几行；边走边锁的次序随链而异，一笔向上走、另一笔正在闭环的会互等，只能靠 PG 判死锁让一方出错。成环只会由移入造出来（移出、关闭只删边），所以把本租户的移入串成一队就够，无死锁、无重试。代价见越权风险点 1。
2. **「包含循环」单立结果格，不并入 `NOT_ACCEPTED`。** 恢复动作不同（核对是否装反、断开链上一级 vs 开封、换实例），且要带链路——与 `MemberElsewhereContained` 带对方标识同形（ADR-0029 按恢复动作分格）。形状照既有拒绝格：结果格、不交 error、不进未决哨兵。
3. **判环先于单父级。** 一只已在别处的袋被装进自己或装进它装着的袋，答「先从那里移出」会引人做一件做完仍成环的事；ADR-0029 要分的正是恢复动作。既有次序「结构事实先于三相」不变。
4. **锁先于单元那一读。** 同一单元的两笔并发移入随之串行；移出、封装、开封、关闭五口不取这把锁，它们与移入并发时单元行的覆盖写仍是票 no-consolidation-fact-provenance/02 记下的缺口，本票不扩。
5. **库级约束只加自装 CHECK，不写触发器。** CHECK 引用不了别的行，递归关系守不住；本仓迁移里有 SQL 函数、没有用触发器表达业务不变量的先例，环由编排与锁守，库面只钉行内形状。
6. **册已成环时读链路交回 error、不答任何格。** 绕过适配器写出的两行互装是册坏了（ADR-0022「返回 error 就是没形成」），不是这一次移入能判的东西；哨兵与读不动分开，让日志分得出「重试不会好」。
7. **一线导入先于 UC 结果契约表。** UC-NO-003 结果契约表没有「成员在别处」一行，本票也不给「包含循环」加行：两者都是移入口对 CONTEXT 硬句的拒绝，验收表的 `AT-NO-039` 一行已经是它的出处。

### 越权风险点

1. **租户级锁把同一租户内全部集运移入串成一队**（NO owner）。今天唯一生产装配是一线导入，一行一笔、本就串行；日后在线作业端多节点并发移入若在这把锁上排队，可改为按链上单元加行锁并接受 PG 死锁判定出错重试，端口 `LineageForUpdate` 的契约不变。
2. **更长的环没有库面约束**（NO owner）。绕过适配器直接写容纳索引仍能写出两行互装，读链路时才报错——报的是读的人，不是写的人；TF 出发门禁的 `ConsolidationMembershipCycle` 仍是下游最后一道。
3. **既有数据若已成环，本票不修也没有处置面**（NO owner）。装进那一袋的移入从此答 error（一线导入记 `NO_ANSWER_FORMED`、重跑不会好），要人直接断开容纳索引里的一边。产品尚无租户，开发库若撞上照迁移 0003 的办法清掉试验行。
4. **新结果格 `CONTAINMENT_CYCLE` 要每个翻译集运结果的消费方都有落点**（ADR-0025 全函数）。今天只有一线导入，已加；parcel-api 的集运端点落地时要照加，否则落进它的 `default` 支。
5. **`Lineage()` 是拒绝那一刻的读数**。锁随事务释放后链路可能已变；报文是给人核对纸单的，不是给机器据以续办的。

### 测试

- 先红：域测（`ContainmentLineage` 未定义）；编排用例答 `MEMBER_ADDED`；库面 CHECK 用例直插自装行成功（`err=<nil>`）；导入用例新格落入 `UNKNOWN(6)` 未决。
- 新用例：`internal/nodeoperations/domain/consolidation_lineage_test.go::TestAMemberAlreadyOnTheLineageClosesACycle`；`internal/nodeoperations/application/consolidate_parcels_test.go::TestNestingThatWouldCloseACycleIsRefusedAndTheOriginalRelationsStay`（另在 `TestConsolidationOwnStoreFailuresFormNoAnswer` 加「祖先链读不动」）；`internal/nodeoperations/adapters/postgres/consolidation_lineage_test.go` 四条（自装 CHECK、链路到根、册已成环报错、第二笔等第一笔提交）与 `TestConsolidationWritesRefuseToRunOutsideATransaction` 加 `LineageForUpdate` 的无事务拒（PBC-08）；`cmd/parcel-frontline-import/consolidation_cycle_test.go::TestConcurrentMutualNestingAdmitsOnlyOneSideOnTheDatabase`（生产装配并发互装只成立一方）、`consolidation_vertical_test.go::TestFrontlineConsolidationImportRejectsWithoutRecording` 加装反与装进自己两行。全部 `-count=3` 绿。
- 变异（都被杀）：A 去掉租户锁 → 适配器并发用例「第二笔没有等第一笔提交就读到了链路 "bag-b"」、导入并发用例「第二笔…答了 MEMBER_ADDED」；B 判环跳过自身 → 域测、编排用例、导入用例（装进自己那行答 `MEMBER_ELSEWHERE_CONTAINED`）；C 去掉册已成环报错 → `TestAnAlreadyCyclicIndexIsAnErrorNotALineage`；D 单父级先于环 → 编排用例、导入用例同一行。
- `scripts/test-full.sh`：改动包 + `go list -test` 反向依赖共 65 包，加 `./internal/architecture/...`、`./migrations/...`，退 0（64 包 ok、2 包无用例）。`go build ./... && go vet ./...` 干净，`gofmt -l` 空。
- 一次性库 `c19seed` 灌演示种子成功，`containment_current_not_self_contained` 在位，用后已删。
- 机制清点与 AT 盘点计数未重生成（workflow「进 main 的步骤」归推送方）。
