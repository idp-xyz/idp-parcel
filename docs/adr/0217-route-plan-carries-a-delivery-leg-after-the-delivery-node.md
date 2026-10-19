# ADR-0217：路由计划在交付节点之后带一段交付段——终点是交付范围，窗口是离开交付节点加上派送时长；段带种类，已执行前缀与运输准备只取节点间段

Status: Accepted（2026-10-19，Claude Code 会话代决：用户原话「你作为业务和系统专家自决……解决所有的问题」；方案 [handover-dispatch-plan](../../.scratch/demo-chain-gaps/handover-dispatch-plan.md)「代决」C11，票 [demo-chain-gaps/64](../../.scratch/demo-chain-gaps/issues/64-route-plan-carries-a-delivery-leg.md)。越权风险点见文末，待 NR owner 复核，TF owner 会签。在 [ADR-0131](./0131-planned-leg-is-referenced-by-plan-version-and-ordinal-and-the-delivery-window-seam-answers-content-not-applicability.md) 决定一的引用形之内；部分停用 [ADR-0175](./0175-service-calendar-content-folds-absolute-time.md) 决定一、决定四各一句与 [ADR-0201](./0201-schedule-templates-materialize-concrete-schedules-by-date-and-preparation-answers-not-configured-on-an-empty-register.md) 决定二末句，被停原句与适用场景见 adr/README 各该行，其余各条不变。）
Date: 2026-10-19

## Context

- NR CONTEXT「网络结构与适用范围」写明网络服务的路由计划覆盖揽收、始发、转运、关务衔接、干线、尾程注入和交付段；「计划履约段」词条也说段可以表达交付范围。计划生成却只按网络连接成段：候选段链就是线路的连接序列（ADR-0175 决定四），演示计划三段止于新加坡末端节点，没有末端到收件人的那一段。
- 派送任务的时间窗由 TF 按登记方在参与关系上声明的计划段引用取（ADR-0131 决定一）。演示末端交接只能声明第三段，即到末端节点为止的那一段，任务窗口因此是「到达交付节点」的窗口，冒充派送窗口（票 06「更正」）。
- ADR-0114 决定一禁止从段位置推「尾程」：哪一段是派送段由控制事实登记方声明。ADR-0131 决定三：没有那一段就答`要求缺失`，NR 不给兜底窗口。
- 服务日历内容今天是截单、处理时长、衔接缓冲三格（ADR-0175 决定一），没有一格说「派送要多久」。
- TF 运输准备消费初始路由信封，逐段从窗口依据里读出连接代码（ADR-0201 决定二、七）；读不出连接的段被判为译不出，信封成毒丸。候选成本按线路段号取成本依据，一段没有依据，整条候选就停在待判断。

## Decision

**一、交付段（机制）。** 计划在交付节点之后至多带一段交付段，且只在段链末尾。起点是交付节点，即候选的末个节点。终点是交付范围：这条候选的服务区域解析覆盖收件地址所用的服务区域版本，多版并列时照解析依据同一种写法。交付范围不是网络节点，不进计划节点序列。目录没有末端责任方列，交付段的责任引用暂指同一服务区域版本，与节点间段先指连接版本（ADR-0175 决定四）同一个「不编造法人」的退法。交付段没有外部不透明形。

**二、窗口与派送时长（窗口折法是机制，时长是租户取值）。** 窗口是［离开交付节点的时刻，离开时刻＋派送时长］。离开时刻照 ADR-0175 决定二折出，含截单与处理时长。派送时长是交付节点服务日历上新加的一格：分钟，可空，空是没登，登 0 才是零分钟，不补默认（迁移 `network_routing/0020_service_calendar_delivery_minutes.sql`）。窗口依据指交付节点的那一版日历。没登派送时长，或这条候选没有交付范围，就不成交付段：计划止于交付节点，与本记录之前相同；登记方再声明交付段之后的序位，TF 窗口缝照 ADR-0131 决定三答 `REQUIREMENT_MISSING`。成了交付段，候选的时间投影延到交付段的终点，时间可行性按它与预计承诺比：预计承诺说的是交到收件人。

**三、段带种类（机制）。** 计划履约段有两种：节点间段与交付段。种类是本上下文对计划内容的描述，不是 TF 的段服务动作。ADR-0114 决定一照旧：对象在执行哪一段、那一段是不是末端派送，仍由登记方声明计划段引用；消费方不得按种类推「尾程」。种类出现之前落库的计划行没有这一格，读回即节点间段，不回填、不迁移计划 jsonb；落库时只给交付段写种类，节点间段与旧行同形。

**四、只取节点间段的各处（机制）。**
- 计划节点序列、已执行前缀与冻结边界的剩余段数：交付段之后没有可控节点；演示策略「剩余不超过一段即冻结」若把交付段也数进去，会在倒数第二个节点提前冻结。
- 计划适用性复核里「实际位置在计划上」的比对：交付范围不是节点。
- TF 运输准备：交付段不走网络连接、没有班次可备，准备只为节点间段形成结果；跳过末段不动前面各段的序位。
- 候选比价：成本依据按线路段号挂，交付段没有成本依据。候选路径把交付段另放一格，不混进与线路段号逐一对应的那一串。

**五、引用与读面（机制）。** 计划履约段引用的序位按整条段链数，交付段是末段，仍是 ADR-0131 决定一的「版本＋序位」。NR 的段窗口窄读口照答交付段的窗口。TF 派送窗口缝不按种类分支：声明交付段答交付段的窗口，声明节点间段答那一段的窗口。路由计划查阅口的成计划行逐段列出引用、种类、起讫、责任、窗口与依据，列之前按判断口的装载纪律把整份计划过构造门重建，不从 jsonb 里抠字段。登记方要声明哪一段，得先看得见有哪几段。

**六、演示租户（租户取值，证据 `S`）。** 新加坡末端日历修订 2 照修订 1 重登处理时长 240 分钟，另登派送时长 240 分钟，合成值。演示末端交接声明交付段 `#4`，派送任务窗口取它。

三类：决定一、三、四、五与决定二的窗口折法是机制；派送时长的取值是租户取值，生产不登就不成段；决定六是演示租户取值。

## 候选与反方

- **交付段终点取收件地址或地理投影。** 客户地址归 PS，NR 只存解析依据（CONTEXT「网络结构与适用范围」、ADR-0075）。否决。
- **TF 窗口缝在末段窗口上自行加派送时长。** TF 推导 NR 的计划内容，派送时长也无处登。否决。
- **派送时长登在服务区域上。** 区域更贴近「末端到收件人」的距离，但处理时长、截单已在节点日历，同一族分两处登要对两边；一个交付节点服务多个区域时差别留作风险点 3。
- **交付段并进候选路径的节点间段，由成本取数侧跳过。** 段号与线路段号的逐一对应被打乱，取数侧要学会认种类。否决，另放一格。
- **种类缺席即判坏。** 已落库的计划全部读不回。否决。
- **没登派送时长时补一个默认时长。** 违 ADR-0175 决定一「不写默认时长」。否决。
- **TF 窗口缝只认交付段。** 等于按种类推「尾程」，ADR-0114 决定一禁的正是这种推法。否决。
- **为交付段另铸段身份。** ADR-0131 决定一已裁「版本＋序位」完整。否决。

## Consequences

- 代码：NR 领域 `PlannedLegKind`（`BetweenNodesLeg`、`DeliveryLeg`）、`NewDeliveryLeg`、`PlannedLeg.DeliveryScope`、`InitialRoutePlan.NodeLegs`；`FormInitialRoutePlan` 立交付段的三条不变量；`PlanNodes` 遇交付段答不出。端口 `ServiceCalendarDefinitionVersion.DeliveryMinutes`、`CandidatePath.Delivery` 与 `PlanLegs`、`RoutePlanLegCatalogueRow`。应用层 `projectCandidateTime` 成交付段并延长投影；`foldTimeAndPaths` 按区域解析取交付范围；初始路由与复核形成计划取 `PlanLegs`；复核的已执行前缀取 `NodeLegs`。postgres 计划 jsonb 段行加可缺的 `kind`；目录日历读写带派送时长；`RoutePlanCatalogue.ListInitialRoutes` 重建计划后逐段列出。HTTP `GET /route-plans?register=initial-route` 成计划行多一键 `legs`。TF `PrepareOnInitialRoute` 只译 `NodeLegs`。
- 迁移：`network_routing/0020_service_calendar_delivery_minutes.sql`。计划 jsonb 不迁移。
- 演示种子：`scripts/demo-seeds/data/network/24-calendar-node-sin-lm-v2.json` 与 `seed.sh` 一行。
- 文档：NR CONTEXT「服务日历」与「路由形成与选择」；GLOSSARY「路由计划」；UC-NR-001 补 `AT-NR-054` 至 `AT-NR-058`、`AT-NR-060` 至 `AT-NR-063`，UC-NR-003 补 `AT-NR-059`；UC-TF-003 补 `AT-TF-201`；参数登记册 `PAR-NET-14`；演示脚本。
- 管理台契约夹具暂不含 `legs`，见风险点 7。

## 越权风险点

1. **总窗口变长影响时效可行性**（NR owner）。登了派送时长，候选投影延到交付段终点，原本可行的候选可能变成不可行。演示没有时间承诺。
2. **交付段责任方暂指服务区域版本**（NR、TF owner）。末端外包时责任方应是末端伙伴，目录有末端责任方列之后另裁。
3. **派送时长挂在交付节点日历上**（NR owner）。一个交付节点服务多个区域、派送距离差得多时，一个时长装不下；要按区域分，另立一格。
4. **交付段不进候选比价**（NR owner）。各候选交付节点不同时，末端成本的差别不进排序。
5. **交付段没有不透明形**（NR、TF owner）。末端外包且内部不可观察时，交付段要的是已知交接点与返回控制边界，今天没有来源说它。
6. **路由计划查阅口重建整份计划**（NR owner）。这一册查阅口此前只上检索列，现在成计划行要过构造门；一行坏计划让整页答不出，不跳过那一行。
7. **管理台契约夹具不含 `legs`**（前端 owner）。管理台的响应类型还不认这一键，进夹具会让前端契约测试当场红；类型与夹具随票 [demo-chain-gaps/72](../../.scratch/demo-chain-gaps/issues/72-admin-web-route-plans-shows-planned-legs.md) 一起补。在那之前「改任一侧两侧都红」对这一键不成立。
8. **段种类留在 NR，TF 不据它推派送段**（TF owner）。登记方声明错了段，任务就照那一段的窗口形成，没人核；与 ADR-0131 越权风险点 2「悬空引用同答缺失」同一代价。

## Links

- [ADR-0131](./0131-planned-leg-is-referenced-by-plan-version-and-ordinal-and-the-delivery-window-seam-answers-content-not-applicability.md)：决定一的引用形，决定三的缺失答法
- [ADR-0114](./0114-delivery-dispatch-is-triggered-by-entering-a-declared-delivery-segment-and-pulls-requirements-by-reference.md)：决定一，派送段由登记方声明
- [ADR-0175](./0175-service-calendar-content-folds-absolute-time.md)：日历内容与离开时刻的折法；决定一、决定四各一句由本记录部分停用
- [ADR-0201](./0201-schedule-templates-materialize-concrete-schedules-by-date-and-preparation-answers-not-configured-on-an-empty-register.md)：运输准备；决定二末句由本记录部分停用
- [ADR-0148](./0148-route-evidence-sourcing-candidate-cost-and-first-candidate-generation-form.md)：候选生成与成本口径
- [ADR-0146](./0146-product-strategy-is-a-third-class-between-mechanism-and-tenant-values.md)：三类
- [network-routing CONTEXT](../domain/network-routing/CONTEXT.md)「路由形成与选择」
- 方案 [handover-dispatch-plan](../../.scratch/demo-chain-gaps/handover-dispatch-plan.md)「代决」C11
