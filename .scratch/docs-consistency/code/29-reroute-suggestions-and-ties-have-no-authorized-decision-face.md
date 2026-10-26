# 29 NR 改路建议与最低成本并列没有授权决定面：建议只能看，原计划失效的包裹停在无当前有效路由

Category: enhancement
Status: in-progress——完工待评审与进 main，分支 claude-code29（2026-10-26，见文末「完成记录」；记录前置由协调方裁定接受、给号 [ADR-0284](../../../docs/adr/0284-an-authorized-role-decides-a-current-reroute-suggestion-through-an-operator-decision-face.md)）。此前 needs-info——2026-10-26 协调方分诊 R2（上线后尽快，见「分诊」）；作者开工即停在记录前置：决定面要按受理基线那一版地理投影重判所择候选、组新计划，NR 读这份投影的口今天只开给受理之后、不由小包托运信封触发的复核，扩到授权改路决定要先有一份记录（见「停工记录」），号与裁定归协调方。修法第 3 条另立 [code/97](./97-initial-route-tie-has-no-authorized-decision-face.md)。分支 claude-code29。此前 needs-triage——建议 R2（上线后尽快）：看得见的卡住。复核在原计划失效后形成改路建议（不满足自动条件、改善不过阈值、最低成本并列、揽收时不在可控节点），初始路由遇最低成本并列交授权角色裁决；CONTEXT 写「授权角色接受建议 → 形成改路决定」，可没有任何口能作这个决定，包裹停在无当前有效路由，直到下一次控制触发碰巧判成。绕法：等包裹交进下一个节点。原计划已失效，那时包裹处于无当前有效路由，交接复核走「原先无路由」那条线，候选评估选出唯一合格候选即形成首个当前有效路由计划（不比改善阈值）；这条绕法依赖的「失效之后的复核走无路由那条线」已由 [code/52](./52-reassessment-reads-the-initial-route-row-instead-of-the-current-plan-or-no-route.md) 补齐（2026-10-21）；评估未决或最低成本并列时仍无路由，没有绕法。复核不取候选成本、失效线上的建议在生产上到不了那一格，已由票 [demo-chain-gaps/115](../../demo-chain-gaps/issues/115-route-reassessment-never-loads-candidate-costs.md) 补齐（2026-10-21），本票随之暴露。2026-10-21 文档自洽 05 立（用户「你作为业务和系统专家自决」授权；审计 L3 承接，02 完成记录「交修订票」一表交 05 判要不要立），分支 claude-dc05。分级由协调方定稿。
Blocked by: 无（ADR-0284 已接受）。此前：一份待接受的记录：把 ADR-0075 决定一被 ADR-0215 决定二部分停用的适用场景扩到「授权角色据复核建议形成改路决定」（号由协调方给，要点见「停工记录」）。此前：无（排在 demo-chain-gaps/115 之后，115 已补齐）
出处：[audit/nr-no.md](../audit/nr-no.md) L3「人工改路：NR 生命周期『授权角色接受建议 → 形成改路决定』」；UC-NR-001「给开发的交接」并列一行、UC-NR-003「给开发的交接」授权决定一行。

## 现象

复核形成的改路建议与初始路由的最低成本并列都要授权角色裁决，可没有任何口能作这个决定，领域里备好的「授权角色」决定方式在生产路径上无人构造。

## 证据

- 领域已备好决定方式：`internal/networkrouting/domain/reroute.go` 的 `RerouteDecisionMode` 有 `AuthorizedRoleReroute`，`FormRerouteDecision` 对人工方式要求指名授权角色、`禁行`下拒。
- 没有调用方：应用层只在 `rerouteAfterLapse`、`tryImprovementReroute` 里以 `AutomaticReroute` 形成决定；没有命令口、没有运营决定面、没有授予名，`AuthorizedRoleReroute` 在生产路径上无人构造。
- 初始路由的最低成本并列同样「交授权角色裁决」（NR CONTEXT「路由形成与选择」、UC-NR-001 结果「路由判断未决」），也没有口；并列候选今天还不落库（demo-chain-gaps/155）。

## 三类

机制：授权角色在建议或并列候选里择一、形成改路决定或首个计划的那道口与它的门（不得绕过硬限制、商业服务范围与他上下文的既有责任）。产品策略：运营决定面的形状照已有的运营决定口（ADR-0227、ADR-0236 一族）。租户取值：谁持有这一种运营决定的授予，未登记答未配置。

## 规模

M：运营决定口（照操作者渠道运营决定族）、授予名、应用编排（读回建议、核建议仍是当前那一版、候选仍合格、形成 `AuthorizedRoleReroute` 决定与新计划、替代原计划适用性）、管理台读面、用例。

## 最小的诚实修法

1. 运营决定口：指名判断键与建议（或并列判断），从建议列出的候选里选一件，带决定依据引用；操作者从会话取（照 ADR-0236 的取法）。
2. 编排：重读当前有效计划与适用性；建议已被更晚的复核取代、候选已不合格或被限制淘汰时拒，答出原因；成立时形成 `AuthorizedRoleReroute` 决定与新计划，生效边界取当前或下一可控节点（揽收复核形成的建议从首个节点起，ADR-0249）。
3. 初始路由并列同口：在 demo-chain-gaps/155 让并列候选落库之后接上。
4. 授予未登记答未配置，不默认给任何角色。

## 要裁的

- 建议过时的判据（被更晚的复核取代即过时，还是另看策略版本、证据修订是否变了）：分诊时定，倾向前者——与 ADR-0235 的当前控制位置同一把尺。

## 分诊（2026-10-26，协调方；用户授权协调方作为业务和系统专家自决）

- 分级 R2（上线后尽快）。
- 「要裁的」建议过时判据：被更晚的复核取代即过时，与 ADR-0235 的当前控制位置同一把尺；策略版本或证据修订变了而没有新复核的，不另判过时（新复核会来）。
- 修法第 3 条（初始路由最低成本并列同口）：先核 demo-chain-gaps/155 之后并列候选是否已落库、读得回；读得回就同一笔接上，读不回就不做这一条，立新票写清依赖。

## 停工记录（2026-10-26，作者子代理，分支 claude-code29，基于 main `1fd7f8e7b`）

### 停在哪

决定面成立时要做的两件事都离不开受理基线那一版提交的地理投影：

1. **按此刻的证据重判所择候选。** CONTEXT「人工决定同样不得绕过硬限制、商业服务范围或其他上下文拥有的既有事实与责任」。建议形成之后才生效的限制，只有重判看得见；不重判，授权角色就能选中一条此刻已被限制淘汰的候选。硬约束里的关务适用性逐候选问两端国家，国家只取自所携投影（`customsFindings`）；服务区域解析同样取投影（`resolveCandidateServiceAreas`）。
2. **组新计划。** 段链取自此刻证据里那条候选的路径，交付段止于覆盖收件地的那一版服务区域（`deliveryScopes`，ADR-0217）；计划带选中候选的逐段成本出处，要向候选成本取数侧取，绑了分区目录的卡要随请求的邮编路线（`postalRouteOf`，ADR-0189）。复核册里建议的候选只存标识、结局与理由（`candidateRow`），没有路径，组不出计划。

NR 不落投影本体（ADR-0075 决定三），能拿到它的只有两处：

- 随小包托运信封携带：初始路由、收寄采用触发的复核。
- 经消费侧适配器 `AcceptedCarriedGeoSource` 问小包托运的受理基线地理投影读口（ADR-0215 决定二、ADR-0301）。这一处的适用场景，ADR-0075 Status 行注记写作「仅限受理之后、不由小包托运信封触发的路由复核」，ADR-0301「对已接受记录的作用」写作「受理之后、不由小包托运信封触发的复核取所携地理投影」，CONTEXT「客户地址继续由 `parcel-shipment` 保存」一条同此。

授权改路决定由操作者发起，不是复核触发，也不经复核编排。让它走后一处，就是把 ADR-0075 决定一被停用的适用场景再扩出一格，属于派单规矩里的「扩成」，照规矩停下回报，不自取 ADR 号。票 demo-chain-gaps/155 完成记录的代决对初始路由续办核得同一处，并入了票 154 的「要裁的」。

绕开它的取法都不成立：

- 揽收触发的建议，投影随收寄采用信封来过，NR 没留。
- 交接触发的建议，按触发关联回到交接册再走一遍复核，会撞「已有结果」。
- 拿建议那一刻的评估结果直接组计划，组不出段链，也挡不住建议之后才生效的限制。
- 让操作者在请求里带投影：判断输入与判断键上的基线不再同版，正是 ADR-0075 要防的。

### 提给协调方的记录要点（草案，号待给）

1. **一种运营决定、一个口。** 操作者渠道「运营决定」能力面加一种决定（暂名 `ROUTE_REROUTE_DECISION`），端点挂运营决定面，铸信封前判准入（能力 `OPERATION_DECISION`、事实类型取决定种类）。有这一种是产品策略；谁持有是租户取值，没登记答 `OPERATOR_NOT_GRANTED`，不默认给任何角色。决定方取信封里认证出的操作者，记成 `OPERATOR/<发行方>#<sub>`，同 ADR-0236 决定三的写法。
2. **过时与重判**（分诊已定的那把尺）。建议所在的那次复核不是这一判断键上业务时间最晚的已判复核，即过时；同一时刻另有一次复核，同样按过时答，不靠处理时钟分先后。这一份建议已有决定，答已决定，交回原决定。所择候选不在建议的候选里，或建议时就没合格，按「不在建议之列」拒。此刻重判被淘汰、不合格或证据未知，各自具名拒。这几格与授予缺席都是确定性答复，不进未决（ADR-0029）。证据读不动、取号故障才答未决。
3. **地理投影取法（部分停用）。** ADR-0075 决定一被 ADR-0215 决定二停用的适用场景加一格：授权角色据复核建议形成改路决定，经 `AcceptedCarriedGeoSource` 取受理基线那一版投影，只取地理维、不落本体。理由同 ADR-0215：基线一经固定不再改写，读回的那一版与判断键上的基线同版。ADR-0215 决定二、ADR-0301 的适用场景随之挂前向指针，CONTEXT 那一条同笔改。
4. **生效。** 决定形成的计划从决定的时刻起生效：服务端时钟就是这一决定的业务时间。原计划失效到决定之间的无当前有效路由照留，与 CONTEXT「授权角色未形成决定前……保持无当前有效路由」一致；取触发的业务时间会把这一段抹掉。生效节点是新计划的首节点，揽收触发的建议即包裹交进的首个节点（ADR-0249 决定二）。决定不是控制触发，不推当前控制位置。ADR-0272 决定一只管复核编排形成的计划，不停用；为免按口解，在记录里写明。
5. **落册与并发。** NR 新册（迁移 `network_routing/0029`）主键是租户加建议所在复核的触发关联，一份建议至多一份决定。决定、新计划与判断键锁同笔，锁与复核同一把（`LatestTriggerAt` 那一把）。两个授权角色同时决定，只有一笔成立，另一笔答已决定；决定与新复核并发，按取锁先后：复核先到，决定答过时；决定先到，复核从新计划起判。复核取起点时（`FormedPlans`）并读这一册的新计划。
6. **初始路由并列不在本记录。** 见下节。

越权风险点草案：NR 读小包托运的面多出一格（PS owner 会签）；决定时刻作生效时刻；授权角色可以选建议里比最低成本贵的合格候选，CONTEXT「加价改路由授权角色决定」认这一格；决定不另发信封，与复评新计划同缺（demo-chain-gaps/34）；授予粒度待角色模型（product-strategy-boundary/07）。

### 修法第 3 条：不在本票做

票 155 之后，并列候选、并列的那几家与打平金额落了 `network_routing.initial_route_not_formed`，可读不回：端口 `InitialRouteNotFormedRegister` 只有写法，生产代码与管理台都没有读这一册的地方；候选行同样没有路径。初始路由的重判要随接受决定信封携带的投影与商业解析标识，NR 都不落本体，取法正是票 154「要裁的」第二条。照分诊不做这一条，另立 [code/97](./97-initial-route-tie-has-no-authorized-decision-face.md)。UC-NR-001「给开发的交接」并列一行的承接，随本票恢复实施那一笔改指 code/97。

### 恢复之后照做的形

- 新特性文件 `cmd/parcel-api/endpoints_reroute_decision.go`，配 `assemble_reroute_decision.go` 与同名测试文件里的未接线占位；`operatorDecisionIntakes` 加 NR 一格；不动 `endpoints.go`、`main.go`、`endpoints_test.go`、`routes.go`。
- 决定种类加进 `accessidentity`，迁移 `access_identity/0023`（取号前再核）。
- 编排在 NR 应用层，依赖复核册、适用性册、证据视图、成本取数侧、取号、时钟与 `AcceptedCarriedGeoSource` 那一类读口；`FormRerouteDecision` 以 `AuthorizedRoleReroute` 形成决定。
- 管理台 NR 页加建议读面与决定签，`pages/network/api.ts` 只加函数。
- UC-NR-003 步骤 7C 与「给开发的交接」改、加 AT 行，AT 盘点分桶表同笔加行；NR CONTEXT 生命周期补决定的生效。
- 失败用例先红；变异至少三处：过时建议仍被接受、已被限制淘汰的候选仍被选、授予缺席时默认放行。

## 完成记录（2026-10-26，作者子代理，分支 claude-code29，基于 origin/main `51aca18cb`）

协调方接受「停工记录」里的记录要点与草案，给号 ADR-0284，裁定恢复实施。修法第 1、2、4 条做完；第 3 条照分诊另立 [code/97](./97-initial-route-tie-has-no-authorized-decision-face.md)。

### 做了什么

- **记录与规则**：
  - 立 [ADR-0284](../../../docs/adr/0284-an-authorized-role-decides-a-current-reroute-suggestion-through-an-operator-decision-face.md)，部分停用（扩成）ADR-0075 决定一被 ADR-0215 停用的适用场景；ADR-0075 的 Status 注记与 Links 挂入向指针，正文不动；ADR 索引不手改。
  - NR CONTEXT 改三处：客户地址那一条、不满足自动改路条件那一条、路由计划适用性首条。
  - UC-NR-003 改步骤 7C 与 `AT-NR-042`、`084` 的注，新立 `AT-NR-095`–`099`。「给开发的交接」去掉授权决定一行，加建议与决定之间晚到的触发一行（code/99）。
  - UC-NR-001 并列一行改指 code/97。
- **领域与端口**：
  - `RouteCandidate.EliminatedByRestriction`：限制淘汰的原因前缀收成一个常量。
  - `RerouteDecisionBasisReference`。
  - 端口 `AuthorizedRerouteDecisionStore`（含过时一问 `SuggestionOvertaken`）、`AcceptanceBaselineGeoSource`、`RerouteSuggestionCatalogueRead`。
- **应用**：`DecideRerouteHandler`。
  - 评估管线与成本取法从复核编排里抽成包级函数，两处共用。
  - 复核册测试替身加一个钩子，替身的 `FormedPlans` 能并读决定册替身。
- **postgres**：
  - 迁移 `network_routing/0029`：决定册，主键是租户加建议关联，外键指回复核行。
  - `AuthorizedRerouteDecisions`、`RerouteSuggestionCatalogue`。
  - `RouteReassessments.FormedPlans` 与 `PlannedLegWindows` 并读决定册。
- **接入**：
  - `accessidentity.DecisionAuthorizedReroute`，迁移 `access_identity/0023`。
  - NR `OperatorDecisionAuthenticator`；networkhttp `OperatorDecisionIntake`、`NewDecideRerouteEndpoint`、`NewQueryRerouteSuggestionsEndpoint`。
  - `AcceptedCarriedGeoSource.LoadAcceptanceBaselineGeo`。
- **parcel-api**：
  - 新特性文件 `endpoints_reroute_decision.go`、`assemble_reroute_decision.go`，同名测试文件里登未接线占位。
  - `operatorDecisionIntakes` 加 `networkRouting` 一格（`assemble_operator_decisions.go`）。
  - `endpoints.go`、`main.go`、`endpoints_test.go`、`routes.go` 都没动。
- **管理台**：
  - 路由计划页加第三个 chip「改路建议」（`RerouteSuggestionsView.tsx`）。
  - 纯逻辑放在 `reroute-decision.ts`，配 node:test。
  - `pages/network/api.ts` 只加两个函数与它们的类型。
  - 没碰 `pages/party/**`。
- **演示种子**：`steps/477-access-authorized-reroute.sh`，数据是 `data/access/15`、`16` 与 `data/governance/51`；README 加一段。
- **票与盘点**：
  - 立 [code/97](./97-initial-route-tie-has-no-authorized-decision-face.md)（初始路由并列）、[code/99](./99-a-control-trigger-between-a-suggestion-and-its-decision-reviews-a-plan-not-yet-effective.md)（ADR-0284 越权风险点 6）。code/98 已被另一会话取用。
  - AT 盘点分桶表加五行，`AT-NR-084` 那一行补点名，另写一段追记；总览计数未动。

### 代决（用户授权协调方「你作为业务和系统专家自决」，协调方接受要点 1–6；以下是落地时作者另定的格）

1. **命名**：决定种类 `AUTHORIZED_REROUTE`，与领域的 `AuthorizedRoleReroute` 同词根；决定口 `POST /route-reroute-decisions`，读面 `GET /route-reroute-suggestions`，与 `/route-plans` 同一前缀。
2. **请求形状**：指名包裹、建议所在复核的触发关联、所择候选与决定依据引用，四格都必填，载荷封闭。指名的包裹不是建议判断键上那一件时答建议不存在：同一租户里，拿另一件包裹的建议关联不能改这件的路。
3. **过时的判法**：同一判断键上，建议所在那次以外有一次复核，它的触发业务时间不早于建议的，即过时；这一问在判断键锁里问库。迁移 0022 之前没有业务时间的旧建议一律按过时答。
4. **答复的先后**：建议不存在 → 已决定 → 过时 → 不在建议之列 → 原计划须已失效 → 受理基线 → 证据 → 重判 → 成本 → 取号 → 落册。先答已决定、后答过时：一份已决定、后来又被取代的建议，答已决定更有用。
5. **此刻重判三格的分法**：
   - `CANDIDATE_RESTRICTED` 只收硬约束那一层留下的淘汰，续办是限制的来源责任方解除；
   - `CANDIDATE_NOT_QUALIFIED` 收其余层次的淘汰与「此刻候选空间里已没有它」（依据 `NOT_IN_CANDIDATE_SPACE`），续办是另择一条；
   - `CANDIDATE_EVIDENCE_UNKNOWN` 收证据未知，续办是等登记。
6. **成本**：与复核同一口取；排序形态未声明不问。读不回答未决；取数侧答用不了或没接，响亮报错。理由：计划要带选中候选的逐段出处，口径同复核形成的计划。
7. **三格不变量破了响亮报错，不折业务答案**：
   - 建议不在失效线上（`ErrUnsupportedRerouteSuggestion`）；
   - 原计划的适用性不是已失效（`ErrInconsistentRouteStart`）；
   - 受理基线与建议的判断键对不上，或读成「不是当前已接受包裹」（`ErrSuggestionBaselineInconsistent`）。
8. **事务壳**：成立以外的各格整笔回滚，与委托协议换版口同形。
9. **读面口径**：读面状态与决定口的过时判据同一条；只列最近 200 份（运营查阅读的常量页大小），不翻页。
10. **管理台入口**：放在路由计划页第三个 chip，不新开导航项，不动共享面。
11. **演示种子**：配置员持这一种授予。这是演示租户的取值，产品不默认给任何角色。

### 越权风险点

ADR-0284 文末七条为准，这里不复述正文，只列条目：

1. NR 读小包托运的面多出一格（PS owner 会签）。
2. 决定的时刻作生效时刻。
3. 授权角色可以选比最低成本贵的合格候选。
4. 决定不另发信封（demo-chain-gaps/34）。
5. 授予粒度（product-strategy-boundary/07）。
6. 建议与决定之间的控制触发晚到时响亮失败（code/99）。
7. 仍适用线上的建议（code/17）。

本票另有四条：

8. **装法两份**（NR owner）。parcel-api 的证据、关务与成本取数侧照 parcel-dispatch 复核那一份重写了一遍：两个进程各是一个 package main，不共用代码。改一边忘了另一边，决定与复核会按不同的取法判同一份证据；头注写了要对照。
9. **取号在拒之后不回收**（NR owner）。取号之后才失败的那几格（落册写不进），计划版本序列已经前进，不回收；与复核同。
10. **读面不翻页**（产品）。建议多于 200 份时只见最近的那些；要翻页随运营查阅读的翻页票一并做。
11. **派发夹具里收寄采用信封所携投影与受理基线读口读出的不同**（NR、PS owner）。只在测试夹具里观察到：`intakeAt` 拼的采用记录不带投影。决定与交接复核两路取到的是同一份，用例按这两路比；生产上收寄采用信封带不带投影，归 PS 交接适配器，本票未核。

### 用例与变异

- **先红后绿**：领域、应用、受理基线投影适配器、HTTP 口、NR 认证与决定种类授予各层，先写失败用例（编译不过），再实现。postgres 决定册、parcel-api 生产装配与派发真库三处用例写在实现之后，判别力用下面的变异确认。
- **变异**：每处改坏都跑相应用例，确认变红，再恢复。
  1. 过时建议仍被接受：去掉过时那一格的返回，`TestASuggestionOvertakenByAnotherReassessmentIsSuperseded` 两个子用例都红。
  2. 候选此刻已被限制淘汰仍被选：把限制淘汰当合格，`TestACandidateRestrictedSinceTheSuggestionIsRefused` 红。
  3. 授予缺席默认放行：决定口按另一种决定（`TRANSPORT_COMMISSION_SUBMISSION`）认证，只持那一种授予的人进得来，`TestSwappedDecisionFacesRefuseAGrantOfAnotherDecisionKind` 与 `TestAnAuthorizedOperatorDecidesARerouteSuggestionOnTheDatabase` 都红。
  4. 两人同时决定、决定与新复核并发：去掉判断键锁，`TestTwoConcurrentRerouteDecisionsFormOnlyOne` 与 `TestARerouteDecisionWaitsForTheJudgmentKeyAndLosesToAConcurrentReassessment` 都红，报「决定没有等判断键锁」。这两条用另开一笔事务持锁取证：确认决定真的等在锁上，再放行。
- **PBC-08**：`TestTheDecisionRegisterRefusesToWriteOutsideATransaction`。

### 验证

- `go build ./...`、`go vet ./...` 干净；`gofmt -l cmd internal migrations tools` 为空。
- `TMPDIR=… scripts/test-full.sh`，范围是改动包、`go list -test` 算出的反向依赖、`./cmd/parcel-api/`、`./cmd/parcel-dispatch/`、`./internal/architecture/...`、`./migrations/...`：退出码 0，34 个包 ok，无 FAIL。
- 新用例另跑 `-count=3`，全绿。
- 演示种子：在一次性库 `c29seed` 上灌完，退出码 0；`477` 那一节的授予、对照与权威区间都落了，迁移 0029 的表与五条约束都在。灌完删库。
- 管理台三道门：`tsc -b --noEmit`、`run-tests`（675 过）、`vite build`，都过。软链与产物（`dist`、`.tmp-test`、`tsconfig.tsbuildinfo`）用后删掉。
- `go run ./tools/adr-index -check` 只报索引过期；`go run ./tools/at-coverage-counts -check` 退 1（总览过期，没有缺行）。两样都由推送方重生成。
- 证据只记 `S`。

迁移落号 `network_routing/0029`、`access_identity/0023`。
