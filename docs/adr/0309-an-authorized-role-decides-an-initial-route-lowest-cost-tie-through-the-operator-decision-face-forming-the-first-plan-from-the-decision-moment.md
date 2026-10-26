# ADR-0309：授权角色经运营决定口在初始路由最低成本并列的候选之间择一、形成这件包裹的首个计划——未形成登记加读法，所择候选按此刻的证据重判，计划从决定的时刻起生效，授予与授权改路同一种

Status: Accepted（2026-10-26，Claude Code 会话代 NR owner 决：用户授权协调方「你作为业务和系统专家自决」，协调方分诊升 R1 并认领，作者读码后确认「要裁的」三问的倾向，其中商业解析标识一问改为不取，见决定三；票 [docs-consistency/code/97](../../.scratch/docs-consistency/code/97-initial-route-tie-has-no-authorized-decision-face.md)。部分停用（再扩一格）[ADR-0075](./0075-customer-address-is-carried-with-the-routing-request.md) 决定一被 [ADR-0215](./0215-a-handover-into-a-node-triggers-route-reassessment-and-reads-the-baseline-geo-projection.md) 停用、又被 [ADR-0284](./0284-an-authorized-role-decides-a-current-reroute-suggestion-through-an-operator-decision-face.md) 扩过一格的适用场景，再加一格「授权角色在初始路由并列的候选之间择一」，被停原句与适用场景见「对已接受记录的作用」；ADR-0075 正文不改，只在其 Status 行与 Links 加前向指针。越权风险点见文末，待 NR owner 复核，PS owner 会签读面那一格；授予粒度待 PC owner 在角色模型（票 product-strategy-boundary/07）里会签。）<!-- 索引注记 -->｜**读法**：未形成登记按判断键、按（租户，包裹）读回最近一次停住的原因与并列的那几家。**口**：`POST /route-initial-route-decisions` 挂操作者渠道「运营决定」能力面，与授权改路同一种授予（`AUTHORIZED_REROUTE`，不新增决定种类、不新增 access_identity 迁移），未授予答 `OPERATOR_NOT_GRANTED`；决定方取信封里认证出的操作者，载荷里自报身份一律 400；请求指名包裹、所择候选与决定依据引用。**答复**：判断库已有这件包裹的初始路由答已有结果；登记行不存在或最近一次停住的不是并列答没有并列登记；所择候选不在并列的那几家里答不在并列之列；此刻重判被有效硬限制淘汰、被其余层次淘汰或不在候选空间、证据未知各自具名拒；证据未配置答未配置；只有依赖读不动答未决。**投影与标识**：所携地理投影经消费侧适配器取受理基线那一版；商业解析标识不取——适用性在并列入账时已判过，重判所择候选不用它。**生效**：首个计划从决定的时刻起生效，判断时刻同取。**落册**：计划落判断库 `initial_route`，裁决证据落 `initial_route_tie_decision`（network_routing 0030），与发布意图同一条提交边界，在判断键锁里串行。**读面**：`GET /route-initial-route-ties` 列并列、全部候选、打平的金额与三格状态
Date: 2026-10-26

## Context

- 初始路由遇最低成本并列入账为`路由未形成`（`CANDIDATES_TIED`）：候选可行，只是选不出唯一一条。NR CONTEXT「路由形成与选择」写并列「交授权角色裁决」，UC-NR-001 结果「路由未形成」写它等「授权角色的裁决」。票 demo-chain-gaps/155 之后并列落 `network_routing.initial_route_not_formed`，全部候选、并列的那几家与各自打平的金额都在，可没有任何生产代码读这一册，也没有任何口能作这个裁决：包裹一直没有初始路由，之后每一次复核都因「没有路由历史」入账未判。成本单维择优最容易打平，并列是常见情形，所以分诊定为挡上线（票 code/97）。
- ADR-0284 为复核建议备好了授权决定面，其决定七把初始路由并列划出去，归本票：重判初始路由要的两样东西，地理投影与商业解析标识，初始路由原本都随接受决定信封携带（`CreateInitialRouteCommand` 的 `Carried`、`Resolution`），NR 不落投影本体（ADR-0075 决定三），登记行里也没有，所以决定面要另有取法。
- 读码确认这两样要的东西并不一样：
  - 地理投影是证据与候选成本的输入（关务适用性问两端国家，服务区域解析与交付范围取投影，绑了分区目录的价卡要邮编路线），重判所择候选离不开它。
  - 商业解析标识只喂 `AssessRoutingApplicability`，那是服务级判断：产品要不要求网络路由。证据视图与候选成本取数侧的签名里都没有它。登记行存在，就说明并列入账那一刻适用性已判为要求网络路由。
- 授权决定的先例与形状全在 ADR-0284：一种运营决定、一个口，操作者从会话取，答复按恢复动作分格，重判所择候选，判断键锁里串行，计划从决定的时刻起生效。本记录照它，只换对象：那里是复核建议里的候选，这里是并列登记里的候选；那里原计划已失效，这里这件包裹根本没有计划。

## Decision

**一、未形成登记加读法。**（机制）

- 端口 `InitialRouteNotFormedRead`：`FindNotFormed` 按判断键、`FindLatestNotFormedOfParcel` 按（租户，包裹）读回一行的判断键、最近一次停住的原因、那次交接的关联，以及并列时并列的那几家。后者取入账时刻最晚的一行：同一包裹换基线重新受理才会有第二行，那时最近入账的才是在等裁决的。
- 登记行答的是「最近一次停住的原因」，旧的已被改写（ADR-0273 决定二），所以读回的就是最新那一次；决定面据此认并列，不认历史上停过并列。
- 读回只带决定面要用的件。全部候选、打平的金额是给人看的，走读面（决定六）的行模型，不重建成领域对象。

**二、一个口，答复分格。**（机制、产品策略）

- `POST /route-initial-route-decisions` 按授权改路那一种认证，铸信封前判准入（能力 `OPERATION_DECISION`、事实类型取决定种类），不覆盖答 `OUTSIDE_ADMISSION_SCOPE`。请求指名包裹、所择候选与决定依据引用；载荷封闭，不收租户与决定方，载荷里自报身份键一律 400。决定方取信封里认证出的操作者，记成 `OPERATOR/<发行方>#<sub>`（ADR-0303 的操作证据写法）。
- 答复按恢复动作分格（[ADR-0029](./0029-retrieval-failure-algebra-is-partitioned-by-recovery-action.md)）。除成立与未决外各格都是确定性答复，重发不会变：
  - 已有结果：判断库已有这件包裹的初始路由（先成的裁决、并列之后重判选出了唯一一条，或无路由），答 `INITIAL_ROUTE_ALREADY_FORMED` 并交回那一行。先于登记的判别，因为计划形成后登记行仍留作当时为何没形成的出处。
  - 没有并列登记：这件包裹没有未形成登记，或登记最近一次停住的不是并列，答 `NO_TIE_REGISTERED`；后者带它停住的原因，授权角色据此知道卡在哪一格。
  - 不在并列之列：所择候选不在并列的那几家里，答 `CANDIDATE_NOT_TIED`。哪怕它此刻合格也拒——选并列之外的候选不是裁并列，是另一种决定，不在这一口（越权风险点 3）。
  - 此刻重判不成立。按此刻的证据重走候选评估管线（服务区域、路由要求、路径可执行、硬约束、时效），所择候选：被有效硬限制淘汰答 `CANDIDATE_RESTRICTED`，续办是限制的来源责任方解除，人工决定不得绕过；被其余层次淘汰或此刻的候选空间里已没有它答 `CANDIDATE_NOT_QUALIFIED`；证据未知答 `CANDIDATE_EVIDENCE_UNKNOWN`。各带依据。判法与授权改路决定同一份（`refusalOf`），不另写一套。
  - 证据未配置：判断时点没有适用的网络证据，答 `INITIAL_ROUTE_EVIDENCE_NOT_CONFIGURED`，等租户登记（[ADR-0052](./0052-network-evidence-catalogue-has-an-unconfigured-grade.md)）。
- 只有依赖读不动答 `UNDECIDED`，带原因与续办引用：判断键锁、登记、判断库、受理基线、网络证据、候选成本读不动，取号故障。整笔回滚，重发即可。
- 受理基线读回的判断键各维须与登记行的判断键一致，不一致或读成「不是当前已接受包裹」是不变量破了，响亮报错，不进未决。

**三、编排：判断键锁里串行，重判，成立时形成首个计划。**（机制）

- 先由包裹读到登记行，取得判断键，再取与复核、授权改路决定同一把判断键锁（`LatestTriggerAt`，ADR-0235 决定四），锁里重读判断库与登记行再判。两个授权角色同时裁同一件包裹，只有一笔成立，另一笔等在锁上，读到先成的那一份，答已有结果；决定与新一次复核并发，按取锁先后。初始路由自己的交接编排不取这把锁，它与决定并发时靠判断库主键撞键翻成「已有记录」：决定落库撞上，读回赢家，答已有结果。
- **所携地理投影经消费侧适配器取受理基线那一版。**（回答「地理投影从哪里来」）决定编排按（租户，包裹）经 `AcceptedCarriedGeoSource` 问小包托运的受理基线地理投影读口，只取地理维，随证据请求与候选成本请求交出，与交接触发的复核、授权改路决定同一读法。理由同 ADR-0215、ADR-0284：受理基线一经固定不再改写，读回的那一版与判断键上的基线同版，ADR-0075 防的异版窗口在这里不存在。本条再扩 ADR-0075 决定一被停用的适用场景一格。
- **商业解析标识不取。**（回答「商业解析标识从哪里来」，改了倾向）证据视图与候选成本取数侧的签名里没有它，它只喂服务级的适用性判断；并列登记行存在，即入账时适用性已判为要求网络路由，接受决定的商业解析一经接受不再改写，重问一遍得到同一个答案。决定面不重问，也就不必回 PS 取接受决定。不重问的代价见越权风险点 2。
- **首个计划从决定的时刻起生效，判断时刻同取。**（回答「生效时刻」）与 ADR-0284 决定四一致，不取委托接受的时刻：决定没有别的业务时间，做出决定的那一刻就是它的业务时间。委托接受到决定之间包裹没有当前有效路由，这一段照留在路由历史里，不追认；取委托接受的时刻，会把这一段抹掉，让一份当时不存在的计划显得那时就已生效。这是 CONTEXT「路由计划适用性」首条的又一种来处，不改初始路由自动形成的口径（仍取委托接受的时刻）。
- 成立时：形成计划，落判断库 `initial_route`（与自动形成的首个计划同一册、同一键，复核取判断起点读的就是这一行），落裁决证据（决定四），同笔交发布意图，意图带并列入账那次交接的关联。**同一条提交边界**：意图交不出去，整笔报错、事务回滚，重发即可。这与初始路由自动形成不同——那边失败只留续办引用，是因为一封交接里别的包裹已判成，不能连坐；这里一次只有一件包裹。
- 不得绕过硬限制、商业服务范围或其他上下文拥有的既有责任：硬限制由重判那一步守；商业服务范围体现在服务区域与路由要求的评估里，同在重判之内；他上下文的既有责任，在这个口上今天没有：这件包裹从未有过路由计划，没有订舱、装载或申报挂在一份不存在的计划上。

**四、裁决证据册。**（机制）

- 计划本体落 `initial_route`，不另立一份；`network_routing.initial_route_tie_decision`（迁移 0030）只记「谁凭什么把并列裁给了谁」：所择候选、计划版本、决定依据、决定方与决定时刻。没有这一行的 `initial_route` 计划是自动形成的。
- 主键取完整判断键，一件包裹至多一次裁决。外键指向同键的 `initial_route` 行（先有计划后有证据）与 `initial_route_not_formed` 行（没有并列登记的裁决是凭空的人工选路）。登记行以后会被同键的又一次交接改写，所以库上只钉行在，不钉它当前的原因；原因由编排在锁里判。
- 写口要事务（`RequireExecutor`）：证据、计划、意图同笔。同一判断键第二次记证据是编排缺陷，响亮报错，不翻成业务答案。

**五、授予与授权改路同一种。**（机制、产品策略、租户取值）

- 不新增决定种类、不新增 access_identity 迁移：持 `AUTHORIZED_REROUTE` 授予的角色可以裁并列。CONTEXT 里两件事用同一个词：授权角色接受建议形成改路决定，授权角色裁并列；复核遇并列形成的建议本来就在授权改路决定面上被裁，初始路由的并列在授权上另立一种，只会让「裁并列」这同一件事按触发它的是复核还是初始路由分成两种授予。
- 谁持有是租户取值；授予未登记答 `OPERATOR_NOT_GRANTED`，不默认给任何角色。授予粒度（是否要把「裁初始路由并列」从「改路」里分出来）是 PC owner 在角色模型里要会签的事，现在分开只是预设答案（越权风险点 4）。
- 演示种子无需改动：配置员已持授权改路授予。

**六、读面。**（产品策略）

- `GET /route-initial-route-ties` 挂主数据与运营查阅读，列本租户未形成登记最近一次停在并列的包裹：判断键、全部候选与各自依据（入账那一刻的）、并列的那几家与打平的金额与币种、状态。状态三格：待裁决（判断库还没有这件包裹的初始路由）、已裁决（带裁决证据）、已另行形成（判断库有计划却不是经决定面形成的，并列之后重判选出了唯一一条）。读面只答「此刻能不能裁」的大概，是否成立仍由决定口在锁里判。
- 管理台路由计划页加「最低成本并列」一册，与「改路建议」一册同形：逐行可在并列的那几家里择一、填决定依据提交。

## 候选与反方

- **另立一种决定种类与授予。** 否决：见决定五，同一件事不按触发来处分两种授予；分开的代价是租户要多登一种授予，收益只是一个还没人提出的职责分离。需要分开时加一种决定种类即可，不影响已授予的角色。
- **随接受决定信封重驱，让初始路由编排自己重判。** 否决：信封已入账，生产上没有工具重发；重放撞「已有结果」或同关联同指纹的重放分界；投影随信封来，NR 没留。
- **决定口收投影或候选成本。** 否决：判断输入与判断键上的基线不再同版，正是 ADR-0075 要防的；操作者自报的地址也不可信。
- **回 PS 接受决定取商业解析标识再问一遍适用性。** 否决：见决定三，取到的只会是同一个答案，却多一条 NR 读 PS 的面和一个读不动的未决格。
- **生效时刻取委托接受的时刻。** 否决：见决定三，抹掉无当前有效路由那一段。
- **所择候选可以是并列之外的合格候选。** 否决：这个口裁的是并列，选更贵的是改路的 CONTEXT 认可「加价由授权角色决定」，那是复核建议的口（ADR-0284 越权风险点 3）；初始路由没有「原计划」可比较，放开等于让并列的登记失去意义。
- **计划与裁决证据写进同一行。** 否决：`initial_route` 是自动形成与人工裁决共用的判断结果册，复核、查阅面与交接意图都读它，人工裁决的出处是它的附注，不是它的列。
- **意图交不出去只留续办引用，不回滚。** 否决：见决定三，一次只有一件包裹，没有要保的同伴；留续办引用要有人去重发，回滚重发更直接。

## 对已接受记录的作用

- **部分停用（再扩一格）** ADR-0075 决定一「`network-routing` 不建立读取 `parcel-shipment` 地址的端口」一句：它被 ADR-0215 决定二停用的适用场景「仅限受理之后、不由小包托运信封触发的路由复核」，已被 ADR-0284 再加一格「授权角色据复核建议形成改路决定」；本记录再加一格——授权角色在初始路由最低成本并列的候选之间择一，经同一个消费侧适配器取受理基线那一版提交的地理投影（决定三）。**适用场景**：只限 NR 授权决定编排（授权改路与并列裁决两种）；可达性判断、初始路由自己的编排与收寄采用触发的复核照旧随请求携带。停用理由与 ADR-0215、ADR-0284 同一前提：受理基线一经固定不再改写。决定二、决定三与其余各条不变。
- ADR-0284：无条款停用。决定七把并列划给 code/97，本记录接下这一格；其「过时」与「建议」一套不适用于并列（初始路由没有建议，也没有复核触发可比较），决定四的生效时刻、决定五的判断键锁与授权取同一种，这里照搬而非引用其条款。
- ADR-0215、ADR-0301：无条款停用。读口与「只交地理维」不变，今天多一个调用方。
- ADR-0273：无条款停用。决定二的未形成登记「重投推不动、等授权角色裁决」一句由本记录落地；登记行的写法与「最近一次停住的原因」不变，只加读法。
- ADR-0272：无条款停用，也不是按口解。它的决定一说复核形成的计划从触发的业务时间生效；初始路由自动形成的计划取委托接受的时刻不变。本记录为授权裁决形成的首个计划另定生效时刻，不借它的条款。
- ADR-0235：无条款停用。判断键锁借同一把，当前控制位置的定义不变。裁决不是控制触发，不推当前控制位置。
- ADR-0227、ADR-0236、ADR-0303：无条款停用，本记录照它们的形，决定方记成操作证据。

## Consequences

- 代码随票 docs-consistency/code/97：
  - 端口 `InitialRouteNotFormedRead`、`InitialRouteTieDecisions`、`InitialRouteTieCatalogueRead`；
  - 应用 `DecideInitialRouteHandler`；
  - postgres `InitialRoutes.FindNotFormed` 与 `FindLatestNotFormedOfParcel`、`InitialRouteTieDecisions`；
  - networkhttp `NewDecideInitialRouteEndpoint`、`NewQueryInitialRouteTiesEndpoint`，`OperatorDecisionIntake.IntakeInitialRouteTieDecision`；
  - parcel-api 特性文件 `endpoints_initial_route_tie.go` 与 `assemble_initial_route_tie.go`；
  - 管理台路由计划页「最低成本并列」一册。
- 迁移 `network_routing/0030`；无 access_identity 迁移。
- NR CONTEXT 客户地址那一条与路由计划适用性首条各补一句；UC-NR-001 结果「路由未形成」一格、候选判断规则末句、「给开发的交接」两行随之改，新增验收行 `AT-NR-100`–`AT-NR-103`。
- 票 demo-chain-gaps/154「要裁的」第二条：本记录给的是决定面的答案；初始路由续办重驱取投影可援引同一前提，但那是另一格，由 154 自己裁，本记录不替它扩适用场景。

## 越权风险点

1. **NR 读小包托运的面多出一个调用方**（NR owner，PS owner 会签）。并列裁决也经受理基线地理投影读口取投影；读口只交地理维与判断键要的引用，面没有变宽。PS owner 若要按调用方收窄，读口换协作者即可。
2. **不重问商业适用性**（NR owner，PS owner 会签）。决定面信并列入账时的适用性判断。若产品在入账之后、裁决之前被撤换成不要求网络路由的服务（今天没有这样的改写口，接受决定的商业解析一经接受不再改写），裁决仍会形成计划。补法是决定编排加一道适用性重问，需要回 PS 取解析标识；有这样的改写口时再加。
3. **并列之外的合格候选不能经这个口选**（NR owner）。授权角色想在并列之外选一条更贵的合格候选，今天没有口：初始路由无计划可改，复核建议的口要求有失效的计划。owner 若要放开，是在决定口放宽「所择在并列里」一道核，同时要定加价是否需要更高一级授权。
4. **授予粒度**（PC owner，票 product-strategy-boundary/07）。一种授予覆盖失效线上全部改路建议与初始路由并列；职责分离（入账的是机器，没有第二个人可分）今天无从谈起。分开成两种授予是加一种决定种类的事，不改已授予角色的权限。
5. **委托接受到决定之间业务发生、决定之后才处理的控制触发**（NR owner）。包裹在无路由期间若仍被交进某个节点，其控制触发的业务时间落在委托接受与决定之间、处理却在决定之后，复核对着从决定时刻才生效的计划判：判出失效或改善替代时，离场早于生效，离场门拒，整封信封响亮失败、按派发预算重投。与 ADR-0284 越权风险点 6 同一类（归票 code/100），补法同要另裁；本记录不吞这一格。
6. **运输准备与查阅面读初始路由那一行**（NR、TF、VE owner）。裁决形成的计划落 `initial_route` 并交发布意图，下游当它是一份普通的初始路由消费；生效时刻在包裹交进首个节点之前的，下游按计划自首节点起执行。
7. **登记行被同键的又一次交接改写**（NR owner）。授权角色看到的并列是读面读到的那一刻；裁决前同一判断键若又一次停在别的原因（比如成本变了），决定口在锁里重读登记，答没有并列登记，不会拿旧并列行形成计划。

## Links

- [ADR-0075](./0075-customer-address-is-carried-with-the-routing-request.md)：前驱——部分停用（再扩一格）决定一被 ADR-0215 停用、又被 ADR-0284 扩过的适用场景，见「对已接受记录的作用」
- [ADR-0215](./0215-a-handover-into-a-node-triggers-route-reassessment-and-reads-the-baseline-geo-projection.md)、[ADR-0301](./0301-parcel-shipment-offers-an-acceptance-baseline-geo-projection-and-reassessment-no-longer-reads-the-whole-request.md)：受理基线地理投影的读法与读口
- [ADR-0284](./0284-an-authorized-role-decides-a-current-reroute-suggestion-through-an-operator-decision-face.md)：同族的先例，授权改路决定面；其决定七把并列划给本记录
- [ADR-0273](./0273-a-settled-wait-for-registration-may-enter-without-a-same-change-resume-and-adr-0094-decision-four-binds-only-the-acceptance-chain.md)：未形成登记
- [ADR-0235](./0235-route-reassessment-settles-what-a-retry-cannot-change-and-does-not-judge-triggers-older-than-the-current-position.md)：当前控制位置与判断键锁
- [ADR-0272](./0272-a-plan-formed-by-route-reassessment-takes-effect-at-the-trigger-business-time.md)：复核形成的计划的生效时刻，不适用于授权裁决形成的计划
- [ADR-0227](./0227-customs-case-and-declaration-submission-enter-production-through-operator-decision-faces.md)、[ADR-0236](./0236-operators-adopt-external-funds-facts-and-regulatory-credentials-through-operator-decision-faces.md)、[ADR-0303](./0303-transport-fulfillment-operation-decisions-record-the-submitting-operator-as-operation-evidence.md)：运营决定口的先例与操作证据写法
- [ADR-0222](./0222-composition-roots-register-endpoints-and-routes-per-feature-file.md)：两口登在 `cmd/parcel-api/endpoints_initial_route_tie.go`
- [ADR-0029](./0029-retrieval-failure-algebra-is-partitioned-by-recovery-action.md)、[ADR-0022](./0022-http-status-carries-answer-formed-not-business-verdict.md)：答复按恢复动作分格、HTTP 状态只答有没有形成答案
- [ADR-0146](./0146-product-strategy-is-a-third-class-between-mechanism-and-tenant-values.md)：机制、产品策略与租户取值三分
- [network-routing CONTEXT](../domain/network-routing/CONTEXT.md)、[UC-NR-001](../application/network-routing/UC-NR-001-CREATE-INITIAL-ROUTE.md)
- 票 `.scratch/docs-consistency/code/97-initial-route-tie-has-no-authorized-decision-face.md`、`.scratch/demo-chain-gaps/issues/154-network-reassessment-not-judged-triggers-resume-after-registration.md`
