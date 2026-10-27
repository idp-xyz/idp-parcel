# 74 身份后来才成立的集运成员，永远拿不到它随袋走过的那条移动事实

Category: enhancement
Status: in-progress——完工待评审与进 main，分支 claude-code74。原状态 needs-triage——R2（上线后尽快）：袋里一件待识别或身份冲突的实物随袋出发，识别成功之后追踪上永远缺这一条出发。2026-10-21 协调方据票 demo-chain-gaps/123 隔离评审立（作者子代理代笔）。
Blocked by: 无（「要裁的」第一条裁为另立 ADR-0299，已先接受）
上下文：VE（全程追踪）、NO（集运成员、收寄身份）。
出处：票 [demo-chain-gaps/123](../../demo-chain-gaps/issues/123-tracking-projects-a-departing-bag-as-one-parcel-and-its-members-get-no-departure.md) 完成记录越权风险点 4。

## 现象

票 123 之后，移动事实那一路与补派生对段里在控的集运单元按事实那一刻的成员展开（`internal/visibilityexception/adapters/transportfulfillment/consolidation_members.go` 的 `trackedParcelsAt`）。成员身份此刻不成立（待识别、身份冲突、没有收寄记录）的那一件没有可投的追踪包裹，入账不派生、不挡其余成员。之后那件实物识别成功、或身份冲突经处置成立，节点作业形成新的收寄版本，但没有任何东西回头把它当时随袋走过的移动事实补给它：那件包裹的追踪上永久缺这条出发、移动或到达。证据只记 `S`（读码）。

## 三类

机制。规模 M。

## 最小的诚实修法（形状仿 ADR-0224）

- 在识别成功、或身份冲突处置成立的那一封 NO 信封上挂一路补派生：取那件作业实物的成员关系事实，找它在哪几件单元里、各在哪段时间；按单元找它在控的段（TF 按对象列段那一口），按段列出声明了这一段的在册移动事实；凡事实发生那一刻它正是那件单元的直接成员（逐层向上到段里在控的那一件）、那件单元在段里在控的，就在新认出的包裹上补派生。
- 派生走 `deriveMovementFactOnto`，同一把键 `movement-fact/<包裹>/<事实>`，与正常路径、ADR-0224 补派生谁先谁后都只留一份。
- 只补投影，不补信号、不重问关务，同 ADR-0224。

## 要裁的

- **扩 ADR-0224 还是另立 ADR**：ADR-0224 定的是「控制事实让对象进段」时的补派生，触发方与判据都在 TF；这一路的触发方是 NO 的身份事实，判据要沿成员关系向上找段，不是同一条决定的简单延伸。倾向另立一份、引 ADR-0224 的判据与幂等键。
- **向上找的口径**：按成员关系事实的业务时间逐层向上到段里在控的单元（与票 123 向下展开同一口径的逆向），还是只认直接装在段里在控单元里的那一层。
- **触发信封**：识别成功（收寄新版本形成）与身份冲突处置成立是不是同一个信封类型，各挂一路还是共用。
- **身份后来又被纠正成另一件包裹**：先前补到旧包裹上的那一份怎么处置（VE 不自行使源事实失效）。
- **扩到交接与交付**：[code/75](./75-tracking-projects-control-facts-on-a-consolidation-unit-as-one-parcel.md) (b) 落地、交接与交付投到单元成员上之后，本票的补派生从移动事实扩到交接与交付；那时身份后来才成立的成员一并由本票补。

## 既有票

- [demo-chain-gaps/123](../../demo-chain-gaps/issues/123-tracking-projects-a-departing-bag-as-one-parcel-and-its-members-get-no-departure.md)；[demo-chain-gaps/53](../../demo-chain-gaps/issues/53-movement-facts-recorded-before-their-segment-never-reach-tracking.md)（ADR-0224 补派生）；[114](../../demo-chain-gaps/issues/114-deterministic-failures-answered-as-retryable-undecided-across-consumers.md)

## 完成记录（2026-10-27，Claude Code 作者子代理，分支 claude-code74，基于 main `cc6744830`）

### 做了什么

- **ADR-0299**（接上 ADR-0224 决定三的唤醒面，无条款停用）：节点收寄形成信封上另起一路移动事实补派生。ADR-0224 的 Status 行与 Links 加前向指针，正文不动。
- **节点作业一侧只加读，不动写与已发布读口**：端口 `ConsolidationUnitsHoldingMemberView.ReadUnitsEverHoldingMember`（`internal/nodeoperations/ports/consolidation_units_holding_member.go`），postgres 实现 `ConsolidationUnitsHoldingMember`，取自集运作业来源事实层（不取单元行成员列与容纳索引：那两样是此刻的派生状态，看不到已出袋或已关闭的单元）；**迁移 `node_operations/0023_consolidation_fact_by_member.sql`** 加 `(tenant_id, member_id, unit_id) WHERE member_id IS NOT NULL` 索引。
- **VE 消费侧端口**（新文件）`internal/visibilityexception/ports/identified_consolidation_member.go` 的 `IdentifiedMemberView`：信封键回读收寄是否认出了包裹、成员曾装进哪几件单元；确定性问题包 `ErrFormedIntakeUntranslatable` / 既有 `ErrConsolidationMembershipUntranslatable`。适配器 `adapters/nodeoperations/identified_member.go`。
- **补派生处理方** `adapters/transportfulfillment/backfill_identified_member_movement_facts.go` 的 `BackfillIdentifiedMemberAdapter`：先逐层向上列候选单元（宽度优先、每件一次、成环走得完），再对每件候选单元走与控制事实信封上补派生**共用**的 `movementBackfill`（`backfill_movement_facts.go` 里把原来 `BackfillMovementFactsAdapter` 的段扫描与按段补抽出，行为不变）；`onlyParcel` 过滤让展开到的别的成员不被碰。
- **inbox 消费门** `adapters/inbox/movement_fact_backfill_on_node_intake_consumer.go`（名 `visibility-exception/backfill-movement-facts-on-node-intake`，与投影、PS 来源采用分账，译码复用 `decodeFormedNodeIntake`，信封与载荷一字不动）。
- **装配**（ADR-0222，只加特性文件）：`cmd/parcel-dispatch/assemble_movement_fact_backfill_on_node_intake.go`、`routes_movement_fact_backfill_on_node_intake.go`（名次 `positionProjection + 10`，排在投影之后、来源采用之前）。未动 endpoints / main / routes / `cmd/parcel-api`。
- **文档**：VE CONTEXT 三处规则句；UC-VE-002 立 `AT-VE-218`–`221`；UC-NO-003 立 `AT-NO-132`；AT 盘点入①五行与一条追记（总览计数未改）。

### 代决（用户原话「你作为业务和系统专家自决……解决所有的问题」）

1. **扩 ADR-0224 还是另立**：另立 ADR-0299。ADR-0224 的触发方与判据都在运输履约，本路触发方是节点作业的身份事实、判据要沿成员关系向上找袋；引它的决定四（同一判据同一幂等键）、决定五（只补投影）、决定六（失败分格），接上唤醒面、无条款停用。
2. **向上找的口径**：逐层到段里在控的单元，不止直接那一层；实现为「候选放宽、展开收紧」而非另算向上的时间区间——候选取节点作业来源事实层里曾装过它的单元（不问哪一刻），再对候选单元用 `trackedParcelsAt` 按事实那一刻展开，展开到这件包裹才补。这是票 123 向下展开口径的逆向，且保证正反向永远用同一条成员口径（同刻按记录时刻、单元关闭、嵌套、成环、身份取此刻），不在反向再实现一遍。备选「另算两个时间区间求交」否：两处各自演化迟早分叉。
3. **触发信封**：识别成功（收寄首次形成、待识别经重核形成）与身份冲突处置成立是**同一个信封类型**（`node-operations.node-intake.formed`，三处都经同一个 `handOffIntake`），共用一路，不各挂一路。**不新造信封**，载荷不加字段，NO 生产方与别的订阅方（VE 投影、PS 来源采用）合同一字不动。没认出包裹的信封（待识别、未形成、形成了却没有关联）入账收工，不等识别（投影那一路才等）。
4. **身份后来又被纠正成另一件包裹**：展开取此刻的收寄身份（ADR-0225 决定二），旧信指的包裹此刻已不是这件实物时什么也不补，也不替新认出的那件补（它有自己的信封）；先前已补到旧包裹上的**不撤**（VE 不自行使源事实失效；移动事实在运输履约仍成立）。撤销需要 NO 把身份纠正交出，见越权风险点 3。
5. **扩到交接与交付**：不做（本路补的是段声明的移动事实；交接与交付尚不投到成员上，等 code/75 (b) 与 demo-chain-gaps/80）；ADR-0299 决定八记明届时候选寻找一口可原样复用。
6. **只补这一件包裹**：展开到的同袋别的成员不补（已识别的早已投过，仍待识别的有各自信封）；只补投影，不补信号、不重问关务。
7. **读口落点**：新增 NO 反向读口 + 索引，不借 `ConsolidationMembershipAtView` 的页式/时点读法；VE 端口另立，不复用 TF 的 `tfports.ConsolidationMembershipView`（理由同票 123 代决 6）。
8. **失败分格**：收寄读不回、反向读/成员关系读不动、列段列事实读不动、派生未决 → 未决名单；收寄与键不符、答复立不成成员或单元标识、段册事实不变量已破 → 发布失败，不进名单（ADR-0029）。新增哨兵 `ErrFormedIntakeNotVisible`（续办）、`ErrFormedIntakeInconsistent`（确定性）。

### 越权风险点

1. **识别成功后出发门禁不补问**（TF owner、VE owner、CC owner）：出发时点这件实物身份未决，读数里那一格答未决（ADR-0225 越权风险点 4），本路只补投影——这件包裹若本该因关务限制而阻断，追踪上显示成一次普通出发、无阻断信号（ADR-0224 越权风险点 3 同一格，那时由 ADR-0305 在控制事实信封上补问接住，成员识别成功这条路没有对应补问）。修法：在 `node-intake.formed` 扇出里再起一路由运输履约消费、按原出发时点替这件包裹再问关务并追加补问读数。独立的产品与领域裁决，**建议协调方立票（R2 候选）**；本票按「不另立后继票」未立（避免与在途分支撞号）。
2. **读量与批量重核**（运维、NO owner）：每封节点收寄形成信封多一次收寄读与一次按成员反查（走新索引）；被装进过袋的实物，每件候选单元多读段、事实与整袋展开（整袋每个成员各读一次收寄身份）。批量重核（按号码、按送站交接单）时读量随件数×袋大小×段上事实数增长。优化方向需 NO 加「这件实物那一刻随哪些单元走」的时点反向读口，本票不取。
3. **身份被纠正后旧包裹上已补的不撤**（VE owner、NO owner）：需要 NO 把身份纠正交出、VE 据此撤销旧包裹上的条目——投影今天不使任何源事实失效，这是它的第一个这类动作。
4. **上线前已识别的成员不自愈**（VE owner）：已定稿的信封不会再投给新 inbox；存量回扫可复用反向读口，本票不做。
5. **补派生停在未决时挡同一封信上排在它后面的 PS 来源采用**（VE owner、PS owner、运维）：节点作业成员关系或收寄册读不动时，新收寄的来源采用一并等；与投影那一路排在来源采用之前同一取舍。信封按收寄键分区，各封互不阻塞。
6. **NO 读口报错不分确定性与暂时**（NO owner，票 114）：反向读口同其余成员读口，一律记未决；能分出的（空白单元标识、收寄与键不符）已记发布失败。
7. **成员按业务发生时间取，受设备时钟偏差影响；段按消费那一刻的段册算**（同 ADR-0225 越权风险点 3、ADR-0205 越权风险点 15）；**更正版改了发生时间时两个时点的成员集合不同**（同票 123 越权风险点 10）；**成环跳过不报错**（同票 123 越权风险点 8）。
8. **新迁移 0023 是非并发建索引**（运维）：`consolidation_fact` 行数大的环境上线时建索引会短暂阻写；此表只在集运作业事实落库时写，演示与起步规模无影响。

### AT

- UC-VE-002 新立 `AT-VE-218`（识别成功补出随袋出发，只补这一件，与两路先后同键）、`AT-VE-219`（笼套袋逐层向上，出发那一刻不在袋里的不补）、`AT-VE-220`（身份被纠正后旧信不补、已补不撤）、`AT-VE-221`（失败分格与消费门分账）；UC-NO-003 新立 `AT-NO-132`（反向读口）。证据一律 `S`。
- 用例：`internal/nodeoperations/adapters/postgres/consolidation_units_holding_member_test.go`；`internal/visibilityexception/adapters/{transportfulfillment/backfill_identified_member_movement_facts_test.go,nodeoperations/identified_member_test.go,inbox/movement_fact_backfill_on_node_intake_consumer_test.go}`；真库 `cmd/parcel-dispatch/movement_fact_backfill_on_node_intake_test.go`（经生产路由一拍：笼套袋、出发时身份不成立的成员识别后补出，已移出、出发后才装进、从没进过单元的不补，重投只答已有结果；识别成功已落册而收寄信封未到时移动事实那一路先到、补派生答已有结果；经生产识别链——PS 接受、门铃、NO 重核、收寄意图——补出；派发失败分区）。

### 验证

- `go build ./...`、`go vet ./...`、`gofmt -l` 干净。
- `TMPDIR=… scripts/test-full.sh` 全仓：159 个包 ok、0 FAIL（含 `./internal/architecture/...`、`./migrations/...`、`./cmd/parcel-api`、`./cmd/parcel-dispatch`）。
- 新用例另跑 `-count=3`：NO postgres、VE 三个适配器包、`cmd/parcel-dispatch` ok（无并发用例）。
- 迁移 0023：一次性库灌演示种子 `seed.sh` 退 0，索引在册，用后 `DROP DATABASE … WITH (FORCE)`。
- 变异（本地验过，备份再拷回并 `cmp` 一致，均未提交）11 处全杀：路由登到别的事件类型（真库三条红）、去掉只补这一件的过滤（适配器两条红）、向上找只走第一层（适配器与真库各一条红）、补派生共用投影的 inbox 账（重放分账用例红）、展开改取记录时刻（适配器三条红）、反向读口把移出算成曾装入（真库读口红）、未决名单去掉收寄读不回（派发分区红）、确定性哨兵误进未决名单（派发分区红）、收寄待识别当成认出了包裹（翻译用例红）、去掉向上找的环防护（成环用例超时红）、补派生派生出与正常路径不同内容（真库笼套袋用例红，幂等键内容一致）。
- `go run ./tools/adr-index -check` 与 `go run ./tools/at-coverage-counts -check` 均只报生成物过期（退 1，非缺行的 2），未提交生成物。
