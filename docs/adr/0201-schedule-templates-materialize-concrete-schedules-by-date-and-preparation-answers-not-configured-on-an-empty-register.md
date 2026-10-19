# ADR-0201：运输机会——周期安排按日期物化成班次，初始路由形成后逐段准备，空册答未配置

Status: Accepted（2026-10-19，Claude Code 会话代决：用户原话「你作为业务和系统专家自决……解决所有的问题」；票 [demo-chain-gaps/16](../../.scratch/demo-chain-gaps/issues/16-transport-schedule-templates-and-concrete-schedules-are-registered.md) 起草并落决定一至五，决定三的容量声明与决定一的容量池由票 17 落，决定六至八由票 18 落。越权风险点见文末，待 TF owner 复核，决定二与决定七待 NR owner 会签。）

Date: 2026-10-19

## Context

- UC-TF-003 要求「计划履约段 → 运输机会 → 班次 / 资源 → 容量准备」分别建模，周期模板不直接成为班次（步 5、`AT-TF-027`），没有合格机会时不建占位班次（`AT-TF-028`）。CONTEXT「班次」生命周期写的是「周期安排或临时需求 → 建立具体班次」。
- 落地前的班次只有身份、一格自由串方向与出发时刻（迁移 `0004_alternate_journey_schedule_capacity.sql`），唯一的写口 `PrepareTransportOpportunityHandler.EstablishSchedule` 在 cmd 下零调用，表里没有生产行。自由串方向无从与 NR 的计划履约段比对。
- 计划履约段只带起讫节点与时间窗口，窗口依据写作 `CONNECTION/<代码>@<版本>`（NR `calendar_projection.go` 的 `plannedLeg`）。演示网络的三条连接随参考配置 `network-routing/network-catalog/SYN-CN-SG@1` 发布，演示租户采用。
- 本批代决的单一权威是 [tf-chain-plan](../../.scratch/demo-chain-gaps/tf-chain-plan.md)「01 运输机会与班次」D01-1 至 D01-10 与「跨票通则」X2、X3、X6；本记录把其中的裁定写成决定，计划此后只作索引。

## Decision

**一、周期安排是 TF 自有的登记册（机制）。** 周期安排、具体班次与容量池走登记册配置写（ADR-0151 定族判据：不搬包裹、不形成生产事实），同一个登记用例同时出受控 CLI 与在线端点（ADR-0085）：CLI 是 `parcel-transport-register`，端点挂 `operatorRegistries.transport`。容量预占、释放与再准备是运营决定，不在本册。NR 服务日历不当班次用。容量池的登记口由票 17 落。

**二、方向身份是 NR 连接代码加起讫节点（机制）。** 班次与周期安排都固定（连接代码〔不带版本〕、起点节点、终点节点）三格，按 NR 的拼写原样保存，本上下文不解析、不核它们在 NR 目录里在不在。自由串方向退役：迁移 0024 在班次表有行时拒绝施加，不替任何人把自由串猜成连接代码。与计划履约段的比对只在 TF 的 `adapters/networkrouting` 里做，对的是 `PlannedLeg.From/To` 与窗口依据里的连接代码（票 18）。

**三、周期文法只内置一种（产品策略）。** 按周循环（星期集合，MON…SUN），当地出发时刻（HH:MM），历时（分钟），IANA 时区，方向，适用服务范围引用，运行日期的生效区间（起点含、终点不含、终点可缺）。到达用历时表达，不收到站当地时刻：起讫可能跨时区，历时只有一种读法。可选的逐维容量声明（维度、单位、硬上限）由票 17 加进文法，物化时随班次建池且幂等。

**四、两本册子的登记代数（机制）。** 同代码（同班次）同内容答`已存在`，交回当前版；异内容成下一版，版本号由册子按当前版加一给出，不由登记方指名。册上各版只插不改，当前版是版本号最大的那一行。周期安排换版不改写已登记的班次——那一班是册上自己的版本；要跟上新的一版，就按同一（代码、运行日期）再物化，班次成下一版。物化出处的周期安排版本不进班次的内容摘要：按更新的一版物化出一模一样的班次，答`已存在`。

**五、具体班次的身份与物化发生在哪里。**
- 派生身份（机制）：周期安排不是班次；具体班次的身份由（周期安排代码、运行日期）确定性派生，写作 `<周期安排代码>/<YYYY-MM-DD>`（不用 `@`，免得与 NR 的版本引用混读），同一（代码、日期）不论在哪里物化都是同一班。不出自周期安排的临时班次另可登记；临时班次不得取派生形身份，否则同一身份有两种来历。
- 物化只在两处（产品策略）：准备时评估候选（只算不存，见决定六），以及显式「登记具体班次」。不按滚动窗口预生成——窗口是租户取值，现在没有；这是内置的一种物化形态，要按租户窗口预生成就另行内置一形。

**六、候选、空册与择优（产品策略）。**
- 候选按连接取：该连接上没有任何当前版的周期安排或具体班次，答`未配置`。准备结果记`未配置`、确认消费、不重投（ADR-0094 判据；X6），不建占位班次。登过但这段时间没有合格候选，答`无当前运输机会`，带候选与淘汰原因。
- 候选的来源：周期安排按日期物化出出发时刻落在窗口内的班次；已登记的具体班次按身份顶替同一（代码、日期）的物化结果，改到窗口或连接之外就不再是候选。起讫不符不在读口上滤掉，是淘汰原因。
- 择优只内置一种（票 18）：候选条件是连接代码相同、起讫相符、出发不早于段窗口起点、到达不晚于段窗口终点；按出发最早、同刻按班次标识排；取第一个所有已登池维度容量都够的，答`运输机会已准备`。有候选但没有池或池的维度没有需求量答`容量待确认`；硬容量都不够答`当前机会不可用`；单对象请求不出`部分准备`；不预占。

**七、准备的触发与结果（票 18）。**
- 由初始路由触发（产品策略）：TF 消费 `network-routing.initial-route.formed`：路由表改成扇出给 VE 与 TF，各自一个 inbox、各列未决哨兵（ADR-0049 Consequences：消费者先进路由表）。无路线的判断零写入。
- 结果唯一、不发信封（机制）：准备结果按（租户、计划段引用〔版本加序位〕、包裹、规则版本）唯一：重放答`已有结果`，异内容答`结果冲突`。没有消费者，不入队准备结果信封（X3）。
- 改路后的新计划由 NR 另出信封（票 34），TF 不轮询 NR 计划目录。授权的再准备请求复用同一用例，另立票 36。

**八、需求量（产品策略，票 18）。** 收寄前取 PS 的申报测量（经 TF 消费侧适配器，ADR-0025），快照标来源 `declaration` 与资料版本锚。只认重量、体积（三边乘积）、件数三维，件数只配件数维的池；同维按量纲换算单位，跨维不折算；袋位、托盘位等没有来源的维度答`待补充`。节点实测后的再准备另立票。

三类（照 tf-chain-plan「需要的 ADR」里 ADR-0201 那一块，计划没归的决定四按机制归、决定八按产品策略归）：决定一、二、四是机制；决定五里派生身份是机制，物化只在两处是产品策略；决定七里由初始路由触发、无路线零写入是产品策略，结果唯一、不发信封是机制；决定三、六、八是产品策略；哪条连接开几份周期安排、几点出发、历时多久、哪几天开、服务范围与生效区间，以及各段由谁履约，是租户取值（`PAR-NET-08`），生产保持未配置。演示租户以 `SYN-` 登记三份按日运行的周期安排，证据只记 `S`。

## 候选与反方

- **班次随 NR 网络参考配置发布，或用 NR 服务日历当班次。** 出发时刻依赖具体网络与伙伴，过不了 ADR-0146 参考配置的分界检验；服务日历讲的是截单与处理时长，不是车次。否决（风险点 7）。
- **方向只认节点对。** 两节点间有平行连接时撞成同一个方向。否决。
- **方向带连接版本。** NR 一换版，同一条连接上的班次就全部失配。否决，代价见风险点 6。
- **按滚动窗口预生成具体班次。** 窗口是租户取值，现在没有；预生成还会让没被用到的班次占满册子。否决。
- **同代码异内容答`内容冲突`，由登记方指名新版本号（有效时间规则那一形）。** 周期安排与班次的改动是日常运营，登记方手上没有可指名的版本号；冲突格只会逼他先查再编号。否决。
- **到达登到站当地时刻。** 跨时区时同一行有两种读法。否决（风险点 11）。
- **内置多种周期形态（按月、日历例外）。** 首发没有证据要求；ADR-0146 决定五只要求每个决定点至少一种内置形。不内置（风险点 2）。
- **临时班次身份不设限。** 「代码/日期」形的临时班次会撞上同一（代码、日期）的物化。否决（风险点 10）。

## Consequences

- 代码（票 16）：领域 `domain.ScheduleTemplate`、`RegisterScheduleTemplate`、`MaterializeSchedule`、`ScheduleTemplate.MaterializeDepartingWithin`、`ScheduleDirection`、`RunningDate`；`TransportSchedule` 改为方向三格加到达、服务范围、版本与来历，`FormTransportSchedule` 只立临时班次。端口 `ports.ScheduleTemplateRegistry`、`ports.ScheduleCandidateSource`（`transport_opportunity.go`），班次仍走 `ports.TransportScheduleStore`。编排 `application.RegisterTransportScheduleHandler`（两口同一用例）与 `application.ScheduleCandidatesHandler`（票 18 的准备编排来调）。`PrepareTransportOpportunityHandler.EstablishSchedule` 随自由串方向退役。
- 迁移：TF `0024_schedule_template_and_schedule_direction.sql`（班次表有行即拒绝施加；新立周期安排册）。
- 接口：`POST /transport-fulfillment-transport-schedule-template-registrations`、`POST /transport-fulfillment-transport-schedule-registrations`；查阅口加 `registry=transport-schedule-template`；`registry=transport-schedule` 的行把 `direction` 换成 `connection`、`fromNode`、`toNode`，并加 `arrivesAt`、`serviceScope`、`version` 与来历各键（`origin`、`template`、`templateVersion`、`runningDate`），只列当前版。
- 演示种子 `scripts/demo-seeds/data/transport/` 三份周期安排，方向取演示网络的连接（由 `cmd/parcel-transport-register` 的用例钉住）。
- 文档：TF CONTEXT Language 补「周期安排」，Rules 与 Lifecycles 补派生身份与两处物化；UC-TF-003 补 `AT-TF-100` 至 `AT-TF-104`；参数登记册 `PAR-NET-08` 按 ADR-0146 拆分。

## 越权风险点

1. **事件放大**（TF owner；NR owner 会签消费边）。每个包裹每段自动准备一次，放大量归 `PAR-NFR-03`。
2. **只有按周一种形**（TF owner）。按月循环、日历例外（节假日停开、加开）都不内置；要用就只能逐日登临时班次。
3. **用申报量做容量判断**（TF owner；PS、NO 是提供方）。实测偏大时只能靠再准备补救。
4. **`未配置`结果不自动重备**（TF owner）。租户补登周期安排后，已形成的`未配置`要靠票 36 的再准备。
5. **改路缺口**（NR owner）。票 34 落地前，改路后的段没有准备结果。
6. **连接代码跨版本相认**（NR owner）。连接出新版本、改了起讫或含义时，班次仍按代码相认；起讫三格会挡住改了起讫的那一种，挡不住只改含义的。
7. **租户若已在 NR 侧维护车次表，要录两遍**（TF owner）。
8. **登记册写授予没有逐册维度**（access-identity owner）。持登记写授予的配置员既能登班次，也能登凭证与总单。
9. **出发之后仍可成下一版**（TF owner）。本册不读移动事实：首个有效出发之后再登同一班次的新时刻，照样成下一版。CONTEXT 生命周期只写了出发前的再登记成下一版、出发后不得取消；出发后改时刻该不该挡由 owner 定，实现暂不挡。
10. **临时班次身份不得取派生形**（TF owner）。形如 `X/2026-10-21` 的临时身份一律未受理，哪怕租户本来就这样编号。
11. **到达只收历时**（TF owner）。登记方手上是到站当地时刻时要自己折成分钟。
12. **查阅口班次行换键**（前端 owner）。`direction` 一键换成三格之后，管理台运输履约查阅页「运行方向」列在前端切片改列之前读空，也没有周期安排区；改列与补区立为票 [demo-chain-gaps/38](../../.scratch/demo-chain-gaps/issues/38-admin-web-transport-schedule-section-reads-connection-and-nodes.md)。
13. **服务范围只存不判**（TF owner）。候选不按服务范围筛；一条连接上服务于不同范围的班次在准备时同样参选。
14. **择优形态是否开成租户可选**（TF owner）。
15. **准备结果不发信封**（TF / NR owner）。UC-TF-003 步 8 的发布意图暂缺，等 NR「分配失败重评」那条边。
16. **决定四、八的三类是本记录自归**（TF owner）。计划没归这两条：登记代数归机制——同代码异内容成下一版是册子自己的代数，不是给租户选的形态；需求量来源归产品策略——取申报还是实测、认哪几维，产品可以另内置别的形态。owner 若认为登记代数也该是可选形态，要另立 ADR 取代决定四。

## Links

- [ADR-0146：产品策略是机制与租户取值之间的第三类](./0146-product-strategy-is-a-third-class-between-mechanism-and-tenant-values.md)：各决定的三类归属与「至少一种内置形」
- [ADR-0151：未归族的命令口定族](./0151-unassigned-command-faces-get-their-families.md)：决定一的定族判据
- [ADR-0085：登记册配置写面进端点表带未配置格](./0085-registry-write-faces-enter-the-endpoint-table-with-unconfigured-grade.md)：CLI 保留为受控批量口，与端点同一登记用例
- [ADR-0049](./0049-publish-channel-is-in-process-delivery-until-load-evidence.md)：Consequences「生产方上线之前，消费者必须先在路由表里」，决定七的上线次序
- [ADR-0094](./0094-undecided-retry-is-decided-by-resume-path-with-a-fourth-grade-for-operator-registration.md)：`未配置`入账不重投
- [UC-TF-003](../application/transport-fulfillment/UC-TF-003-PREPARE-TRANSPORT-OPPORTUNITY.md)：`AT-TF-027`、`AT-TF-028` 与 `AT-TF-100` 至 `AT-TF-104`
- 计划 `.scratch/demo-chain-gaps/tf-chain-plan.md`「01 运输机会与班次」；票 `.scratch/demo-chain-gaps/issues/16-…`、`17-…`、`18-…`
