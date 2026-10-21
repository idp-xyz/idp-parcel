# 123 全程追踪把出发的一袋当一件包裹投，袋里的成员拿不到出发里程碑

Category: enhancement
Status: in-progress——完工待评审与进 main，分支 claude-dcg123（2026-10-21 认领，基于 main `e013fef6e`）。此前 needs-triage——2026-10-20 票 [42](./42-departure-gate-reading-does-not-expand-consolidation-unit-members.md) 的作者子代理立（用户「你作为业务和系统专家自决……解决所有的问题」授权）。出自 [ADR-0205](../../../docs/adr/0205-departure-gate-is-a-pulled-reading-recorded-with-the-movement-fact-not-a-refusal.md) 文末补记越权风险点 20（原指票 42，票 42 只修了门禁读数与阻断信号）与 [ADR-0225](../../../docs/adr/0225-the-departure-gate-reading-asks-through-consolidation-unit-members-at-the-moment-of-departure.md)「不在本记录」第二条、越权风险点 9。
Blocked by: 无
地盘：VE 移动事实投影（`internal/visibilityexception/adapters/transportfulfillment/derive_on_movement_fact.go` 的 `objectsOf`，以及补派生 `backfill_movement_facts.go`）。

## 现象（读码钉票 42 的分支，只记 `S`）

- 全程追踪消费移动事实已形成信封时按键回读，对象取声明的段里事实发生那一刻在控的参与（ADR-0205 文末补记「展开口径」），逐对象派生投影。段里在控的是一袋时，投影按袋的引用立一件「追踪包裹」，袋里的成员拿不到这一条出发——客户与运营在成员的追踪上看不到 `DEPARTED`。
- 票 42 之后出发读数里已有袋的成员各一格（ADR-0225），阻断信号也落在成员上；但移动与到达不带读数，投影不能借读数展开，得像出发读数那样问节点作业那一刻的成员。
- 补派生（ADR-0224）沿同一口径，同病。

## 三类

机制（投影按成员展开：袋按事实发生那一刻的直接成员逐件派生，嵌套到包裹；袋本身投不投由 VE owner 定）。规模 M。

## 要裁的

- 袋本身还要不要作为一件追踪对象被投（今天它被当成包裹投，追踪页上多一件不是包裹的条目）。
- 成员取事实发生那一刻（与出发读数同口径），还是消费那一刻。
- 成员身份不成立、读不动时，投影入账不派生还是记未决重投（后者要先分确定性与暂时，见票 114）。

## 既有票

- [42](./42-departure-gate-reading-does-not-expand-consolidation-unit-members.md)、[53](./53-movement-facts-recorded-before-their-segment-never-reach-tracking.md)（补派生）、[114](./114-deterministic-failures-answered-as-retryable-undecided-across-consumers.md)

## 文档自洽审计复核（2026-10-21）

审计条目：[audit/ve.md](../../docs-consistency/audit/ve.md) VE-10（归 C，审计已点名本票，不另立）：段里在控的袋或集运单元被当成一件包裹投影，与 VE CONTEXT「集运、装载或同批关系不合并包裹身份和客户轨迹」相反；出处 ADR-0205 文末补记越权风险点 20、ADR-0225 越权风险点 9。按 main `67438a0f` 复核，现象仍在；票 146、94 只改门禁读数，没有改投影展开。

分级建议（文档自洽 02）：R1——整袋出发是第一天的常态，袋里每件成员的 `DEPARTED` 永久缺失、追踪页多一件不是包裹的条目，属静默丢失，与已按 A 组处理的票 [53](./53-movement-facts-recorded-before-their-segment-never-reach-tracking.md)（`DEPARTED` 永久丢失）同一后果类。由协调方定稿。文档侧（UC-VE-002 `AT-VE-046` 附近注一句当前缺口）由修订票 docs-consistency/04 做。

## 完成记录（2026-10-21，Claude Code 作者子代理，分支 claude-dcg123，基于 main `e013fef6e`）

### 做了什么

- **VE 消费侧端口**（新文件）：`internal/visibilityexception/ports/consolidation_membership.go` 的 `ConsolidationMembershipView`——按（租户，对象引用原文，时点）答是不是集运单元、那一刻的直接成员；成员三种：认出的包裹、嵌套单元、身份不成立。调用失败原样交回；节点作业答了却立不成成员包住确定性哨兵 `ErrConsolidationMembershipUntranslatable`。
- **VE 适配器**（新文件）：`internal/visibilityexception/adapters/nodeoperations/consolidation_membership.go`，读 ADR-0225 已发布的 NO `ConsolidationMembershipAtView` 与 `IntakeIdentityView`，身份由 NO `JudgeIdentityBasis` 认。**NO 一个文件也没改、也没新加**：两口读面票 42 已立，本票只消费。
- **展开**（新文件）：`internal/visibilityexception/adapters/transportfulfillment/consolidation_members.go` 的 `trackedParcelsAt`——段里事实那一刻在控的每件先问是不是单元，不是即一件包裹；是就按那一刻的直接成员逐层到包裹，每个引用只问一次、只投一次（直接在段里又在袋里、几袋同指、成环都收成一份）；身份不成立的成员跳过；先全部展开再派生。
- **正常路径与补派生同一处**：`DeriveOnMovementFactAdapter` 与 `BackfillMovementFactsAdapter` 各多一口成员关系读面（缺即构造拒），对象经 `trackedParcelsAt` 再交 `deriveMovementFactOnto`；后者与 `movementProjectionCommand` 改收追踪包裹引用，事实引用仍是 `movement-fact/<包裹>/<事实>`，成员取自己的包裹号，与它直接在段里时同一把键。
- **哨兵**：`ErrConsolidationMembershipNotVisible`（读不动，续办）进移动事实那一路与补派生两路的未决名单；`ErrConsolidationMembershipInconsistent`（答复立不成成员，确定性）不进，各名单头注写明。
- **装配**：只改特性文件——`cmd/parcel-dispatch/assemble_movement_fact.go`（新 `projectionConsolidationMembers` 接 NO 两本只读册）、`assemble_movement_fact_backfill.go`、`routes_movement_fact_backfill.go`。未动 endpoints / main / assemble / routes。无新迁移；接线棘轮与类型可达棘轮基线不受影响（没有新增领域工厂）。
- **文档**：VE CONTEXT「追踪投影与里程碑」加一条规则（集运单元按事实那一刻的直接成员投、单元本身不投、身份不成立入账不派生、读不动未决），「入账不派生」的「事实没有可投的追踪对象」一格补「在控的集运单元那一刻没有身份成立的成员」；UC-VE-002 立 `AT-VE-196`–`198`，`AT-VE-046` 的现状缺口注改为移动事实那一半已补；AT 盘点只加三行与一条追记，总览计数未改。ADR 正文与 Status 行都没动。

### 代决（用户原话「你作为业务和系统专家自决……解决所有的问题」）

1. **袋本身不投**：单元不立追踪投影，只投到成员上。依据 VE CONTEXT「集运、装载或同批关系不合并包裹身份和客户轨迹」与追踪对象是包裹；同串的包裹号按单元认（ADR-0214 决定二、ADR-0225 决定四同一判据）。备选「袋与成员都投」否：追踪页多一件不是包裹的条目，且会给袋另立客户轨迹。
2. **成员取事实发生那一刻**，不取消费那一刻：与出发门禁读数展开成员同一口径（ADR-0225 决定二），离线补登或迟到消费时袋可能已拆。三种事实与补派生都这样取。
3. **不借出发读数展开**：读数里有成员，但移动、到达没有读数，存量出发与读不动的读数也说不出成员；一条路问节点作业，三种事实与补派生同形。
4. **成员身份不成立（待识别、身份冲突、没有收寄记录）→ 入账不派生，不挡同袋其余成员**；成员关系或收寄身份**调用失败 → 整封未决**；**身份链判不下来、答复译不回、成员种类越界 → 发布失败**（确定性，不进未决名单）。票面第三问按票 114 的分法先分了确定性与暂时。备选「身份不成立也记未决」否：整封停住，袋里其余成员的出发一起停，而识别不会回头让这封信重投。
5. **嵌套逐层到包裹、不设上限，成环跳过不报错**：读数要给环记一格缘由；投影只要成员集合，环上那一边不多出成员，按「每个引用只问一次」收尾。
6. **端口落在 VE 自己的 ports，不复用 TF 的 `tfports.ConsolidationMembershipView`**：VE 要区分重投与否（TF 读数把身份链说不准与读不动同记一格），且跨上下文翻译留在消费方（ADR-0025）。
7. **信号不变**：阻断信号仍只看读数里问过关务的那几格（ADR-0225 决定七）；补派生仍只补投影。
8. **不立新 ADR**：读码判断本票不与已接受 ADR 相抵——ADR-0205 文末补记「展开口径」定的是在控对象取 `ObjectsUnderControlAt`，本票照取、展开在其后，回答的正是它的越权风险点 20；ADR-0263 决定四约束的是运输履约的参与与控制判断，投影不形成参与或控制、不改段册，也不改节点控制。
9. **补派生对象若是单元同样展开**：对象凭交接进段时是一袋，补到袋那一刻的成员身上。

### 越权风险点

1. **ADR-0205 越权风险点 20 由本票在码里答了，ADR 上没有前向指针**（VE owner）：ADR 正文与 Status 行按约定未改；协调方进 main 时若要在 ADR-0205 Status 行加一句索引注记，需重生成 ADR 索引。
2. **存量袋投影不清理**（VE owner）：本票之前已把袋当包裹投下的条目照留在投影里，运营按袋号查追踪仍看得到旧的移动条目。
3. **其余各路仍把袋当一件包裹投**（VE owner）：交接、场外揽收、有效交付等投影按控制事实的对象引用立追踪包裹，对象是袋时同病（真库补派生用例里交接那一封就给袋立了一条交接条目）。本票地盘只在移动事实投影，未改，需另立票。
4. **身份后来成立的成员永远拿不到这条移动事实**（VE owner、NO owner）：入账不派生之后没有任何东西回头补；要补，去处是识别成功那一封信上的补派生（同 ADR-0224 的做法）。
5. **NO 读口报错不分确定性与暂时**（NO owner，票 114）：`ConsolidationMembershipsAt` 与 `IntakeIdentities` 对「册上的值立不起来」与连不上库同样报错，本票一律记未决重投。
6. **每条移动事实每件在控对象多一次成员关系读、每件非嵌套成员多一次收寄读**（运维）：节点作业成员事实那一本读不动时，连纯包裹的段的移动投影也整封未决——说不出是不是袋就不能当包裹投。
7. **成员按业务发生时间取，受设备时钟偏差影响**（同 ADR-0225 越权风险点 3）；**段按消费那一刻的段册算、成员按事实那一刻的成员事实算**（同 ADR-0205 越权风险点 15），投到的成员可能与登记那一刻读数问到的不同。
8. **成环跳过不报错**（NO owner）：节点作业成员关系册成了环是坏数据，投影照把能到达的包裹投上，不形成任何提醒。
9. **NO 成员读面的翻译写了两处**（TF、VE、NO owner）：TF `adapters/nodeoperations` 与 VE `adapters/nodeoperations` 各把同两口 NO 读面译成本上下文的成员，判法相同（身份都经 `JudgeIdentityBasis`），只在「身份链判不下来」的落格上不同（理由见代决 4、6）。

### AT

- UC-VE-002 新立：`AT-VE-196`（袋按事实那一刻的成员投、袋本身不投、正常路径与补派生同一把键、重投幂等）、`AT-VE-197`（嵌套、重复、成环只投一次，身份不成立的成员入账不派生，空袋无可投成员）、`AT-VE-198`（成员读不动未决、答复不一致发布失败）。
- 用例：`internal/visibilityexception/adapters/transportfulfillment/derive_on_movement_fact_consolidation_test.go`（四条新用例，`AT-VE-183` 那条改为袋在节点作业那边确是一袋）；`internal/visibilityexception/adapters/nodeoperations/consolidation_membership_test.go`（翻译与读不动两条）；真库 `cmd/parcel-dispatch/movement_fact_consolidation_projection_test.go`——**经生产登记编排登一条出发，段里在控的是一袋与一件散件，生产路由一拍：袋里出发那一刻的两件成员与散件各得一份 movement-departure、归 DEPARTED，袋不立投影，出发前已出袋的、出发后才装进的不投，没有收寄记录的那一件不派生；新 ID 重投只答已有结果**；**出发先于段登记、袋后经生产交接进段，交接那一封补派生到成员上**。派发分区两条用例各加两格。证据一律 `S`。
- 去掉展开（把「是不是单元」恒答不是）时，三条适配器用例与两条真库用例都失败（本地变异验过，未提交）。

### 验证

- `go build ./...`、`go vet ./...`、`gofmt -l` 干净；三笔代码提交各自单独 `go build ./...` 与 `go vet`（VE、parcel-dispatch）通过（临时 worktree 逐笔检出验）。
- `TMPDIR=… scripts/test-full.sh ./internal/visibilityexception/... ./cmd/parcel-dispatch ./cmd/parcel-api ./cmd/parcel-ve-register ./internal/architecture/... ./migrations/...`：20 个包 ok、0 FAIL（`cmd/parcel-ve-register` 是改动包在 VE 之外的反向依赖之一，另两个是 parcel-dispatch、parcel-api；TF 未动）。
- 新用例另跑 `-count=3`：VE 两个适配器包与 `cmd/parcel-dispatch` ok。
- 演示种子不受影响：没有改目录、映射、迁移或种子数据，未灌一次性库。
- 未重生成 ADR 索引、AT 计数与机制清点（`at-coverage-counts -check` 此刻如预期报计数不一致，留给进 main 时重算）。

