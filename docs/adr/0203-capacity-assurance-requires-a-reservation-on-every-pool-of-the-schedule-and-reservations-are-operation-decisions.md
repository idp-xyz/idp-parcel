# ADR-0203：容量保障——班次挂了容量池，装载分配与确认订舱就要对每个池引用有效预占并声明分配量；预占与释放是两种运营决定，声明需求量与来源，硬上限按预占整段判、带余量拒绝、不可审批越过

Status: Accepted（2026-10-19，Claude Code 会话代决：用户原话「你作为业务和系统专家自决……解决所有的问题」；票 [demo-chain-gaps/20](../../.scratch/demo-chain-gaps/issues/20-capacity-reservation-and-release-decision-faces.md) 起草并落决定三至六，决定一、二由票 21 落装载分配那一半、票 37 落确认订舱那一半。越权风险点见文末，待 TF owner 复核，access-identity owner 知会。）

Date: 2026-10-19

## Context

- TF CONTEXT 写「确认订舱或装载分配需要容量保障时，必须引用、建立或转换明确的容量预占范围」，没写什么时候算「需要」。票 03 把这一格列为要裁的：做成内置策略，还是作租户取值。
- 容量池已能登记（票 17，ADR-0201 补记「池身份」「登记口不改池」）：池身份由（班次、维度）派生，一池一维、一个硬上限。预占、释放、消耗的编排早已在，在 cmd 下零调用，没有入口。
- 编排里的预占只有三量与有效期：说不出这份量从哪来（CONTEXT 要每次容量申请固定「采用的测量/数量版本、各容量维数量、单位和判断时点」），也不记从哪一刻起占。余量按判断时点那一刻算，而预占没有起点：一份声明在后天、彼时别的预占都已过期的预占，按后天的余量放行，今天就超了硬上限。
- 释放没有身份，重交一遍就再还一份。池按值换存、读池不锁：两笔并发预占各读到同一个旧池、各按旧余量放行，后换存的那一笔把先到者整份删掉。
- 本批代决的单一权威是 [tf-chain-plan](../../.scratch/demo-chain-gaps/tf-chain-plan.md)「03 装载分配与容量」D03-3 至 D03-6 与「跨票通则」X2、X3；本记录把其中的裁定写成决定，计划此后只作索引。

## Decision

**一、保障判据（产品策略；票 21 落装载分配，票 37 落确认订舱）。** 装载分配或确认订舱所在的具体班次只要登记了至少一个容量池，就必须对该班次的每个池引用一份当前有效预占，并逐池声明分配量；没有登记容量池的班次不要求。判据只看班次挂没挂池，不看任何租户取值。已登记的池是硬限制，绕过它就是超配；不要保障的租户不登池即可，不需要第二个开关。

**二、分配关系（机制；票 21 落）。**

- 在预占上建分配关系，守恒为「已分配 ≤ 预占 − 释放 − 消耗」；分配撤回或改版时归还。形成分配时不调消耗：消耗是永久占用，撤回时还不回来。
- 池所在的班次必须等于分配所在的班次。
- 分配量逐池逐维度声明并带来源，来源与决定三的需求来源同一组封闭词；只有件数维的池可以用成员数（UC-TF-003 禁止用对象数顶替别的维度）。
- 新格：`CAPACITY_RESERVATION_REQUIRED`（挂池班次缺预占引用）、`RESERVATION_NOT_ON_SCHEDULE`（引用的预占不在本班次的池上）、`ALLOCATION_EXCEEDED`（超出预占剩余，带余量）。

**三、预占与释放是两种运营决定，都声明需求量（本票）。**

- 决定种类 `CAPACITY_RESERVATION`、`CAPACITY_RELEASE`，挂操作者渠道的「运营决定」能力面（ADR-0151 定族判据：为一个业务范围占住一段运力是生产决定，不是登记册配置）。授予的粒度等于权力的粒度：能占的不因此能放。种类是产品角色模型；谁被授予、准入对照与权威区间是租户取值，演示租户以 `SYN-` 登记。建池仍是登记册配置写（ADR-0201 决定一），不在这两口。
- 需求量是数量、单位与来源三格。来源是封闭的两种：`OPERATOR_DECLARATION`（运营人员声明的一版需求）与 `PREPARATION_DEMAND_SNAPSHOT`（运输准备结果固定下来的需求快照），引用原样保存。释放的来源说的是为什么不再需要这些量。
- 单位必须等于池的单位，按字面比，不换算、不归一大小写；不符答 `UNIT_MISMATCH` 并交回池。单位要声明出来再比，不从池上默认：把「5」默认成池的单位，声明方心里的吨就成了千克。维度不声明：一池一维，落到哪个池就是哪一维。
- 判断时点与释放时点由声明方给（ADR-0023）。

**四、硬上限按预占整段判（产品策略），带余量拒绝，不可越过。**

- 预占从判断时点起占到有效期满；释放从它落下的那一刻起归还；过期后只剩已消耗的那一部分。
- 新预占要在 [判断时点, 有效期] 整段的每一刻都不超硬上限。占用只在某份预占的判断时点上涨，所以最紧的那一刻不是起点就是段内另一份预占的判断时点，逐一比过即得整段最小余量。
- 超出答 `CAPACITY_EXCEEDED`（HTTP 200），带整段最紧那一刻的余量（单位同池）与池。命令与载荷都没有任何越过上限的格；带审批、越过一类键的载荷在封闭解码下是坏报文（400）。软容量的受控超配仍归 psb/09，不在本记录。
- 读池在写事务里锁住池那一行（`FOR UPDATE`），判余量与换存同一笔：两笔并发预占串行判，后到者读到含先到者的池。

**五、身份与重放（机制）。**

- 预占引用在池内唯一。同一引用整份相同（数量、单位、来源、判断时点、有效期，时刻按登记册精度比）答 `EXISTING_RESERVATION` 并交回那一份；任一格不同答 `RESERVATION_CONFLICT` 并交回在册那一份，不改写。
- 释放有自己的引用，在池内唯一，不只在一份预占内唯一。同一引用、同一份预占、同内容答 `EXISTING_RELEASE`，不再还一份；改了数量、来源、时点或挪到另一份预占上答 `RELEASE_CONFLICT`，交回在册那一次。
- 重放判在单位与上限之前：池此刻已满时原样重交仍答已有，它占的就是那一份。

**六、分格与次序（机制）。**

- 预占：载荷过构造门（不成立答 `INPUT_NOT_ACCEPTED`，不读池）→ 读池（读不动答未决 `POOL_STORE_UNAVAILABLE`；不在本租户册上答 `POOL_NOT_FOUND`，他租户的池同此）→ 重放与冲突 → 单位 → 硬上限 → 落池。
- 释放：构造门 → 读池 → 释放引用的重放与冲突 → 预占在册（不在答 `RESERVATION_NOT_FOUND`）→ 单位 → 过期（`RESERVATION_EXPIRED`）与守恒撞线（`RESERVATION_OVERDRAWN`），这两格交回那一份预占，三量在上面 → 落池。
- 池不在册、预占不在册、单位不符各成一格，续办分别是先登池或改指池、改指预占、按池的单位重述数量，不并进`未受理`（ADR-0029）。容量编排的`未受理`线上名改为 `INPUT_NOT_ACCEPTED`，与 TF 其余运营决定口与登记口同名，容量池登记口随之改名。
- 只有新预占、新释放用 201，其余都是形成了的答案（200，ADR-0022）。编排答未决时装配整笔回滚再交回答复。
- 预占与释放不入队信封：此刻没有消费者（方案 X3）。不核班次在册：池建立时已核过，池身份由班次派生。

三类：决定一的判据、决定三的来源种类与单位判法、决定四的整段判法是产品策略（内置一种）；决定三的决定种类是产品角色模型，授予、对照、区间是租户取值；硬上限不可越过是 CONTEXT 规则；决定二、五、六是机制。演示租户的预占取值只写在演示种子 README，证据只记 `S`。

## 候选与反方

- **保障做成租户可开关的内置策略。** 让租户关掉保障就是允许绕过已登记的硬上限；不要保障就不登池。否决。
- **判据看池紧不紧张。** 同一笔分配今天要预占、明天不要，判据不可复现。否决。
- **预占与释放共用一种决定。** 能占的就能放，授予粒度大过权力粒度。否决。
- **单位不声明，默认取池的。** 声明方的量纲错在任何一处都不会响。否决。
- **只按判断时点那一刻判余量**（此前的实现）。见 Context：声明在后天的预占绕过今天的上限。否决。
- **判余量取服务端时钟的此刻。** 与 ADR-0023「业务时间由提交方给」相抵，且按整段判已经不需要时钟。否决。
- **释放不带身份，按「同一预占、同量、同时点」认重放。** 同一刻对同一预占合法地放两次各 10，会被吞成一次。否决。
- **乐观并发（池上加版本列，换存时比版本）。** 要改容量池端口的签名（方案 X4 不改 `ports.go`），撞上时也只能答未决让人重交；行锁让后到者直接读到最新池作答。否决。
- **预占时回读运输准备结果，核需求快照在不在。** 准备结果册由同波票 18 立，此刻没有读口。延后，记为风险点 3。
- **给硬上限留一个带审批的越过格。** CONTEXT 明文「触及硬限制时拒绝新的预占或分配，不能以审批绕过」。否决。

## Consequences

- 代码（票 20）：access-identity 决定种类 `DecisionCapacityReservation`、`DecisionCapacityRelease`；TF 领域新 `CapacityDemand`、`CapacityDemandSource`（种类封闭）、`CapacityReleaseReference`、`CapacityRelease`，`CapacityPool.Reserve`、`Release` 改收需求量，新 `AvailableThrough`；编排 `Reserve`、`Release` 带需求来源，新格（`POOL_NOT_FOUND`、`RESERVATION_NOT_FOUND`、`EXISTING_RELEASE`、`RELEASE_CONFLICT`、`UNIT_MISMATCH`）；tfhttp 新预占口与释放口；容量池册读池加行锁。
- 迁移：TF `0030`（预占加来源与判断时点、有效期须晚于判断时点，新释放明细子表、释放引用池内唯一、随预占删；预占表有行即拒绝施加）；access-identity `0012`（决定种类约束重建，列各份重建的并集，另预列 ADR-0202 已定名的 `BOOKING_REQUEST_SUBMISSION`）。
- 接口：`POST /transport-fulfillment-capacity-reservations`、`POST /transport-fulfillment-capacity-releases`，挂操作者渠道的运营决定面；装配另立 `cmd/parcel-api/assemble_capacity_reservation.go`，每次决定一笔事务。
- 演示种子：配置员两种决定的授予、准入对照与权威区间各两行；种子不预登预占。
- 文档：TF CONTEXT「运输委托、订舱、容量与装载」补保障判据一句；UC-TF-004 补 `AT-TF-136` 至 `AT-TF-144`。
- 票 21 在决定一、二之内落装载分配的容量关系；票 37 落确认订舱。

## 越权风险点

1. **无池即无保护**（TF owner）。不登池的班次上，装载分配与确认订舱不受任何容量约束。
2. **需求量与分配量是声明值**（TF owner）。与实测不符（`AT-TF-087`）靠重评处置，本记录不核。
3. **需求来源不回读**（TF owner）。`PREPARATION_DEMAND_SNAPSHOT` 的引用此刻不核准备结果在不在、量对不对；运营声明的版本也只是一个引用。去处是票 [demo-chain-gaps/47](../../.scratch/demo-chain-gaps/issues/47-capacity-demand-from-a-preparation-snapshot-is-not-read-back.md)。
4. **软超配仍归 psb/09**（TF owner）。本记录只守硬上限。
5. **资源与载运位置粒度不支持**（TF owner；方案 D03-8）。池只按（班次、维度）立。
6. **判断时点由声明方给**（TF owner）。整段判法让倒填与前填都越不过硬上限，但倒填的释放会把容量提前还回去；有效期已在过去的预占不拒，它此刻什么也不占。
7. **读池一律加行锁**（TF owner）。事务外读池也会等在途的写；目前读池的只有写编排。
8. **逐份预占没有查阅册**（前端 owner）。查阅口只到池级三个和，管理台看不到每份预占的来源与释放明细。去处是票 [demo-chain-gaps/46](../../.scratch/demo-chain-gaps/issues/46-capacity-reservations-have-no-per-reservation-review-registry.md)。
9. **种类粒度**（psb/07）。两种决定是本记录按「授予粒度等于权力粒度」定的。
10. **缩容之后的余量可为负**（TF owner；票 40）。容量更正把上限缩到既有预占之下时，`CAPACITY_EXCEEDED` 如实报负余量，不截成零。

## Links

- [ADR-0151：未归族的命令口定族](./0151-unassigned-command-faces-get-their-families.md)：决定三的定族判据与「每口一种决定」的先例
- [ADR-0201：运输机会](./0201-schedule-templates-materialize-concrete-schedules-by-date-and-preparation-answers-not-configured-on-an-empty-register.md)：池身份、建池走登记册配置写与「登记口不改池」
- [ADR-0202：运输委托、订舱与承运答复的入口族与协议核对](./0202-transport-commission-booking-and-carrier-answer-entry-families-and-supplier-agreement-check.md)：确认订舱那一半（票 37）所在的订舱面
- [ADR-0022](./0022-http-status-carries-answer-formed-not-business-verdict.md)、[ADR-0029](./0029-retrieval-failure-algebra-is-partitioned-by-recovery-action.md)：各格答 200 且按续办分开
- [ADR-0023](./0023-work-fact-identity-and-time-are-minted-by-the-device.md)：判断时点由声明方给
- [UC-TF-004](../application/transport-fulfillment/UC-TF-004-COMMISSION-AND-ACCEPT-TRANSPORT.md)：步 5，`AT-TF-136` 至 `AT-TF-144`
- [UC-TF-003](../application/transport-fulfillment/UC-TF-003-PREPARE-TRANSPORT-OPPORTUNITY.md)：`AT-TF-030`、`AT-TF-031`
- 计划 `.scratch/demo-chain-gaps/tf-chain-plan.md`「03 装载分配与容量」；票 `.scratch/demo-chain-gaps/issues/20-…`、`21-…`、`37-…`
