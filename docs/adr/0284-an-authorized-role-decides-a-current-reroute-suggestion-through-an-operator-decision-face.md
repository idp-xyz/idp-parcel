# ADR-0284：授权角色经运营决定口据当前的改路建议择一候选、形成改路决定——建议被不早于它的复核取代即过时，所择候选按此刻的证据重判，计划从决定的时刻起生效

Status: Accepted（2026-10-26，Claude Code 会话代 NR owner 决：用户授权协调方「你作为业务和系统专家自决」，协调方接受作者停工记录里的要点并给号；票 [docs-consistency/code/29](../../.scratch/docs-consistency/code/29-reroute-suggestions-and-ties-have-no-authorized-decision-face.md)。部分停用（扩成）[ADR-0075](./0075-customer-address-is-carried-with-the-routing-request.md) 决定一被 [ADR-0215](./0215-a-handover-into-a-node-triggers-route-reassessment-and-reads-the-baseline-geo-projection.md) 决定二停用的适用场景，再加一格「授权角色据复核建议形成改路决定」，被停原句与适用场景见「对已接受记录的作用」；ADR-0075 正文不改，只在其 Status 行与 Links 加前向指针。越权风险点见文末，待 NR owner 复核，PS owner 会签读面那一格；决定种类与授予粒度待 PC owner 在角色模型（票 product-strategy-boundary/07）里会签。）<!-- 索引注记 -->｜**口**：`POST /route-reroute-decisions` 挂操作者渠道「运营决定」能力面，决定种类 `AUTHORIZED_REROUTE`（access_identity 0023），铸信封前判准入；谁持有是租户取值，未授予答 `OPERATOR_NOT_GRANTED`；决定方取信封里认证出的操作者，记成 `OPERATOR/<发行方>#<sub>`；请求指名包裹、建议所在复核的触发关联、所择候选与决定依据引用。**过时与重判**：同一判断键上另有一次触发业务时间不早于建议的复核即过时（同一时刻也算）；已有决定答已决定；所择候选不在建议里合格答不在建议之列；此刻重判被有效硬限制淘汰、被其余层次淘汰或不在候选空间、证据未知各自具名拒；证据未配置答未配置；这些都是确定性答复，只有依赖读不动答未决。**投影**：所携地理投影经消费侧适配器取受理基线那一版。**生效**：计划从决定的时刻起生效，生效节点是新计划首节点；不推当前控制位置。**落册**：一份建议至多一份决定（network_routing 0029，主键租户加建议所在复核的触发关联），与复核共用判断键锁；复核取起点与段窗口读口并读这一册的新计划。**读面**：`GET /route-reroute-suggestions` 列建议、候选、阻塞与决定状态
Date: 2026-10-26

## Context

- NR CONTEXT 生命周期「受控改路」：不满足自动条件形成改路建议，「授权角色接受建议 → 形成改路决定，从明确的当前或下一可控节点及未执行段生效」；授权角色未形成决定前，原计划已经失效时保持无当前有效路由。规则「不满足自动改路条件时，只形成改路建议，由适用授权的运营角色决定。人工决定同样不得绕过硬限制、商业服务范围或其他上下文拥有的既有事实与责任」。
- 复核在原计划失效后形成改路建议：自动改路形态未声明、改善不过阈值、最低成本并列，以及揽收时不在可控节点（[ADR-0249](./0249-offsite-pickup-reassessment-checks-hard-constraints-only-and-does-not-establish-a-controllable-node.md) 决定二）。生产上没有任何口能作这个决定，领域里备好的 `AuthorizedRoleReroute` 无人构造，包裹停在无当前有效路由（票 code/29，读码钉 main `1fd7f8e7b`，只记 `S`）。
- 决定成立要做两件事：
  - 按此刻的证据重判所择候选。建议形成之后才生效的限制，只有重判看得见；不重判，授权角色就能选中一条此刻已被限制淘汰的候选。
  - 组新计划。段链、交付段（[ADR-0217](./0217-route-plan-carries-a-delivery-leg-after-the-delivery-node.md)）与选中候选的逐段成本出处都取自此刻的证据；复核册里建议的候选只存标识、结局与理由。
- 两件都离不开受理基线那一版提交的地理投影：关务适用性逐候选问两端国家，服务区域解析与交付范围取投影，绑了分区目录的卡要邮编路线。NR 不落投影本体（ADR-0075 决定三）；经消费侧适配器取受理基线那一版投影的适用场景只到「受理之后、不由小包托运信封触发的路由复核」（ADR-0075 Status 注记、ADR-0215 决定二、[ADR-0301](./0301-parcel-shipment-offers-an-acceptance-baseline-geo-projection-and-reassessment-no-longer-reads-the-whole-request.md)）。授权改路决定由操作者发起，不是复核触发。
- 运营决定口的先例：[ADR-0227](./0227-customs-case-and-declaration-submission-enter-production-through-operator-decision-faces.md)、[ADR-0236](./0236-operators-adopt-external-funds-facts-and-regulatory-credentials-through-operator-decision-faces.md)——新决定种类追加进 `accessidentity` 与迁移、端点行挂 `decisionFace`、准入照 ADR-0149 决定四判、操作者从会话取。
- 复核的业务时间排序：当前控制位置是这一判断键在册复核里业务时间最晚的那一次已判控制触发（[ADR-0235](./0235-route-reassessment-settles-what-a-retry-cannot-change-and-does-not-judge-triggers-older-than-the-current-position.md) 决定四），读它即取判断键事务锁。复核形成的计划从所判触发的业务时间起生效（[ADR-0272](./0272-a-plan-formed-by-route-reassessment-takes-effect-at-the-trigger-business-time.md) 决定一）。

## Decision

**一、一种运营决定、一个口。**（机制、产品策略）

- 操作者渠道「运营决定」能力面加一种决定 `AUTHORIZED_REROUTE`：授权角色据改路建议形成改路决定。迁移 `access_identity/0023` 重建决定种类那条 CHECK。
- `POST /route-reroute-decisions` 按这一种认证，铸信封前读生产权威区间（能力 `OPERATION_DECISION`、事实类型取决定种类），不覆盖答 `OUTSIDE_ADMISSION_SCOPE`。合成租户与真实租户同一条命令面。
- 有这一种决定是产品策略；谁持有是租户取值，没有授予答 `OPERATOR_NOT_GRANTED`，不默认给任何角色。
- 请求指名包裹、建议所在那次复核的触发关联、所择候选与决定依据引用；载荷不收租户与决定方。决定方取信封里认证出的操作者，记成 `OPERATOR/<发行方>#<sub>`，与 ADR-0236 决定三的操作证据同一写法，落在决定上作授权角色引用。

**二、建议过时与所择候选重判。**（机制）

答复按恢复动作分格（[ADR-0029](./0029-retrieval-failure-algebra-is-partitioned-by-recovery-action.md)）。下列各格都是确定性答复，重发不会变：

- 建议不存在：这一租户下没有这个触发关联的复核，那次复核没有建议，或建议的判断键不是请求指名的包裹。答 `SUGGESTION_NOT_FOUND`，不区分不存在与他租户。
- 已决定：这份建议已有决定，答 `SUGGESTION_ALREADY_DECIDED` 并交回原决定。重放与另一位授权角色的第二次决定同答。
- 过时：同一判断键上另有一次复核，其触发业务时间不早于建议所在的那一次，答 `SUGGESTION_SUPERSEDED`。同一时刻另有复核也按过时答，不靠处理时钟分先后。这把尺与当前控制位置同一把：建议所在的那次复核已不是业务时间最晚的那一次，包裹已在不早于它的位置上判过。策略版本或证据修订变了而没有新复核的，不另判过时。
- 不在建议之列：所择候选不在建议的候选里，或建议形成时就没合格，答 `CANDIDATE_NOT_SUGGESTED`。
- 此刻重判不成立。决定按此刻的证据重走候选评估管线（服务区域、路由要求、路径可执行、硬约束、时效），所择候选：
  - 被有效硬限制淘汰，答 `CANDIDATE_RESTRICTED`，带淘汰依据——续办是限制的来源责任方解除，人工决定不得绕过；
  - 被其余层次淘汰，或此刻的候选空间里已没有它，答 `CANDIDATE_NOT_QUALIFIED`，带依据——续办是另择一条；
  - 证据未知（地址信息不足、限制状态未知），答 `CANDIDATE_EVIDENCE_UNKNOWN`，带依据——续办是等登记。
- 证据未配置：判断时点没有适用的网络证据，答 `REROUTE_EVIDENCE_NOT_CONFIGURED`，等租户登记（[ADR-0052](./0052-network-evidence-catalogue-has-an-unconfigured-grade.md)）。

只有依赖读不动答未决（`UNDECIDED`，带原因）：复核册、决定册、适用性册读不回，小包托运读不动，证据或候选成本读不动，取号依赖故障。整笔回滚，重发即可。

建议所在复核的结论只认`已失效`那一线：今天只有失效线形成建议。

**三、所携地理投影经消费侧适配器取受理基线那一版。**（机制）

- 决定编排按（租户，包裹）经 `AcceptedCarriedGeoSource` 问小包托运的受理基线地理投影读口，取那一版的地理维，随证据请求与候选成本请求交出，与交接触发的复核同一读法。
- 只取用地理维，投影本体不落 NR。读回的判断键各维须与建议的判断键一致，不一致或读成「不是当前已接受包裹」是不变量破了，响亮报错，不进未决。
- 理由同 ADR-0215：受理基线一经固定不再改写，读回的那一版与判断键上的基线同版，ADR-0075 防的异版窗口在这里不存在。本条扩了 ADR-0075 决定一被停用的适用场景，见「对已接受记录的作用」。

**四、决定形成的计划从决定的时刻起生效。**（机制）

- 新计划的生效时刻与判断时刻都取决定的时刻（服务端时钟）：决定没有别的业务时间，做出决定的那一刻就是它的业务时间。
- 原计划失效到决定之间的无当前有效路由照留，与 CONTEXT「授权角色未形成决定前……保持无当前有效路由」一致；取建议那次触发的业务时间会把这一段从适用性时间线上抹掉，让一份当时不存在的计划显得那时就已生效。
- 生效节点是新计划的首节点（`RerouteDecision.EffectiveFromNode`）；揽收触发的建议即包裹交进的首个节点（ADR-0249 决定二）。
- 决定不是控制触发，不推当前控制位置：当前控制位置仍只认已判的复核。
- 决定方式 `AUTHORIZED_ROLE`，原计划引用取建议所在复核的被复核计划，触发引用取建议的触发；原计划的适用性已是`已失效`，不再动。

**五、落册与并发。**（机制）

- 决定落 NR 新册 `network_routing.authorized_reroute_decision`（迁移 0029），主键是租户加建议所在复核的触发关联：一份建议至多一份决定。一行记判断键各维、所择候选、原计划、新计划本体与版本、决定方、决定依据引用与决定时刻。
- 决定编排先读建议所在的那次复核，再按它的判断键取与复核同一把判断键锁（`LatestTriggerAt`），锁里读已有决定、判过时、重判、落册，到事务结束。由此：
  - 两个授权角色同时对同一份建议作决定，只有一笔成立，另一笔等在锁上，读到先成的那一份，答已决定；
  - 决定与新一次复核并发，按取锁先后：复核先到，决定答过时；决定先到，复核从新计划起判。
- 新计划进路由历史：复核取判断起点（`FormedPlans`）与计划履约段窗口读口都并读这一册的新计划。

**六、读面。**（产品策略）

- `GET /route-reroute-suggestions` 挂主数据与运营查阅读，列本租户的改路建议：判断键、触发关联、被复核计划、阻塞、候选（标识、结局、依据）与状态。状态三格：待决定（建议所在复核仍是判断键上业务时间最晚的那一次、没有决定）、已决定（带决定）、已过时。读面只答「此刻能不能决定」的大概，是否成立仍由决定口在锁里判。
- 管理台路由计划页加「改路建议」一册，逐行可择一合格候选、填决定依据提交。

**七、初始路由最低成本并列不在本记录。** 重判初始路由还要随接受决定信封携带的商业解析标识，取法与票 demo-chain-gaps/154「要裁的」第二条同一问，归票 [docs-consistency/code/97](../../.scratch/docs-consistency/code/97-initial-route-tie-has-no-authorized-decision-face.md)。

## 候选与反方

- **建议形成那一刻的评估直接组计划。** 否决：建议里的候选只有标识与结局，组不出段链；也挡不住建议之后才生效的限制，人工决定就绕过了硬限制。
- **操作者在请求里带投影。** 否决：判断输入与判断键上的基线不再同版，正是 ADR-0075 要防的；操作者自报的地址也不可信。
- **交接触发的建议按触发关联回到交接册，再走一遍复核。** 否决：同一触发再判撞「已有结果」；揽收触发的建议投影随收寄采用信封来过，NR 没留，回不去。
- **生效时刻取建议那次触发的业务时间。** 否决：抹掉无当前有效路由那一段，见决定四。
- **决定推当前控制位置。** 否决：当前控制位置是路由历史里已判控制触发的事实（ADR-0235 决定四），塞进决定会改它的定义；代价见越权风险点 6。
- **决定落复核册一行。** 否决：复核册是触发的判断结论册，主键是触发关联，逐结论在场件矩阵钉死`已改路`只出自自动改路；另立一册，一份建议一份决定由主键承担。
- **过时另看策略版本或证据修订。** 否决：分诊定了同一把尺；证据变了由重判所择候选接住，不必让整份建议过时。
- **授予默认给配置员一类角色。** 否决：谁持有是租户取值。

## 对已接受记录的作用

- **部分停用（扩成）** ADR-0075 决定一「`network-routing` 不建立读取 `parcel-shipment` 地址的端口」一句：它被 ADR-0215 决定二停用的适用场景「仅限受理之后、不由小包托运信封触发的路由复核」再加一格——授权角色据复核建议形成改路决定，经同一个消费侧适配器取受理基线那一版提交的地理投影（决定三）。**适用场景**：只限 NR 授权改路决定编排；可达性判断、初始路由与收寄采用触发的复核照旧随请求携带。停用理由与 ADR-0215 同一前提：受理基线一经固定不再改写。决定二、决定三与其余各条不变。
- ADR-0215：无条款停用。决定二说的是复核取投影的读法，本记录给另一种判断用同一读法，不改它。
- ADR-0301：无条款停用。决定三「交接与收寄采用两路共用它」说的是读口今天的使用者；本记录让授权改路决定成为第三个使用者，读口、答格与「只交地理维」都不变。
- ADR-0272：无条款停用，也不是按口解。它的决定一写明「适用于复核编排形成的每一份新计划」，授权改路决定不经复核编排，不在其内；本记录为它另定生效时刻（决定四），不借它的条款。代价是它决定三「复核能判的下一次触发都不早于……这一次形成的计划的生效时刻」那条论证对决定形成的计划不成立，见越权风险点 6。
- ADR-0235：无条款停用。当前控制位置的定义与判断键锁照旧；决定借同一把锁，不进当前位置。
- ADR-0249：无条款停用。决定二「授权角色据建议作出的改路决定从下一个可控节点生效，即包裹交进的首个节点」由决定四的生效节点承接。
- ADR-0227、ADR-0236：无条款停用，本记录照它们的形加一种决定。

## Consequences

- 代码随票 docs-consistency/code/29：
  - `accessidentity.DecisionAuthorizedReroute`；
  - 领域 `RouteCandidate.EliminatedByRestriction`；
  - 端口 `AuthorizedRerouteDecision`、`AuthorizedRerouteDecisionStore`、`AcceptanceBaselineGeoSource`、`RerouteSuggestionCatalogueRead`；
  - 应用 `DecideRerouteHandler`；
  - NR 消费侧适配器 `AcceptedCarriedGeoSource.LoadAcceptanceBaselineGeo`、`OperatorDecisionAuthenticator`；
  - postgres `AuthorizedRerouteDecisions`，`RouteReassessments.FormedPlans` 与 `PlannedLegWindows` 并读新册；
  - parcel-api 特性文件 `endpoints_reroute_decision.go` 与 `assemble_reroute_decision.go`，`operatorDecisionIntakes` 加 NR 一格。
- 迁移 `access_identity/0023`、`network_routing/0029`。
- NR CONTEXT「客户地址继续由 `parcel-shipment` 保存」一条、「路由稳定性与受控改路」不满足自动条件那一条与生命周期「路由计划适用性」首条随本记录改；UC-NR-003 步骤 7C、「给开发的交接」与验收行随之改，新增验收行；UC-NR-001 并列一行的承接改指 code/97。
- 演示种子：配置员加这一种授予，准入对照与权威区间各一行。
- 管理台路由计划页加「改路建议」一册。

## 越权风险点

1. **NR 读小包托运的面多出一格**（NR owner，PS owner 会签）。授权改路决定也经受理基线地理投影读口取投影；读口只交地理维与判断键要的引用，面没有变宽，多的是一个调用方。PS owner 若要按调用方收窄，读口换协作者即可。
2. **决定的时刻作生效时刻**（NR、TF、VE owner）。新计划从操作者提交那一刻起生效，不是包裹交进某个节点的时刻；揽收建议的决定在包裹交进首个节点之前生效，计划自首节点起执行。运输准备与查阅面今天仍读初始路由那一行（code/52 越权风险点 5，归 demo-chain-gaps/34），看不到决定形成的计划。
3. **授权角色可以选建议里比最低成本贵的合格候选**（NR owner）。CONTEXT「加价改路由授权角色决定」认这一格；并列建议里也可以选并列之外的合格候选，排序只是建议。owner 若要把并列建议限在并列那几家之间，决定口加一道核即可。
4. **决定不另发信封**（NR、TF owner）。与复评新计划同缺（demo-chain-gaps/34）：决定形成的计划落库，不交给运输准备。
5. **授予粒度**（PC owner，票 product-strategy-boundary/07）。一种授予覆盖失效线上全部建议，包括加价的与揽收的；职责分离（建议的复核者与决定者）今天无从谈起，复核是机器。
6. **建议与决定之间的控制触发晚到**（NR owner）。一次控制触发的业务时间落在建议与决定之间、处理却在决定之后（包裹在无当前有效路由期间仍被交进下一个节点，信封又晚到），它不早于当前控制位置，照判，起点却是从决定时刻才生效的新计划：判出失效或改善替代时离场早于生效，离场门拒，整封信封响亮失败、按派发预算重投。今天不吞这一格。补法要另裁，可选的有：这一格入账为未判（扩 ADR-0235 的未判原因）、决定推当前控制位置、或这次触发对着它业务时间那一刻的无路由状态判；归票 [docs-consistency/code/99](../../.scratch/docs-consistency/code/99-a-control-trigger-between-a-suggestion-and-its-decision-reviews-a-plan-not-yet-effective.md)。
7. **仍适用线上的建议**（NR owner）。今天只有失效线形成建议，决定编排只认失效那一线；票 code/17 让仍适用线也形成建议时，决定要替代一份当前有效计划，那时随 code/17 一并定。

## Links

- [ADR-0075](./0075-customer-address-is-carried-with-the-routing-request.md)：前驱——部分停用（扩成）决定一被 ADR-0215 停用的适用场景，见「对已接受记录的作用」
- [ADR-0215](./0215-a-handover-into-a-node-triggers-route-reassessment-and-reads-the-baseline-geo-projection.md)、[ADR-0301](./0301-parcel-shipment-offers-an-acceptance-baseline-geo-projection-and-reassessment-no-longer-reads-the-whole-request.md)：受理基线地理投影的读法与读口，无条款停用
- [ADR-0235](./0235-route-reassessment-settles-what-a-retry-cannot-change-and-does-not-judge-triggers-older-than-the-current-position.md)：当前控制位置与判断键锁
- [ADR-0272](./0272-a-plan-formed-by-route-reassessment-takes-effect-at-the-trigger-business-time.md)：复核形成的计划的生效时刻，不适用于决定形成的计划
- [ADR-0249](./0249-offsite-pickup-reassessment-checks-hard-constraints-only-and-does-not-establish-a-controllable-node.md)：揽收建议的决定从首个节点生效
- [ADR-0227](./0227-customs-case-and-declaration-submission-enter-production-through-operator-decision-faces.md)、[ADR-0236](./0236-operators-adopt-external-funds-facts-and-regulatory-credentials-through-operator-decision-faces.md)：运营决定口的先例
- [ADR-0222](./0222-composition-roots-register-endpoints-and-routes-per-feature-file.md)：两口登在 `cmd/parcel-api/endpoints_reroute_decision.go`
- [ADR-0029](./0029-retrieval-failure-algebra-is-partitioned-by-recovery-action.md)、[ADR-0022](./0022-http-status-carries-answer-formed-not-business-verdict.md)：答复按恢复动作分格、HTTP 状态只答有没有形成答案
- [ADR-0146](./0146-product-strategy-is-a-third-class-between-mechanism-and-tenant-values.md)：机制、产品策略与租户取值三分
- [network-routing CONTEXT](../domain/network-routing/CONTEXT.md)、[UC-NR-003](../application/network-routing/UC-NR-003-REASSESS-ROUTE-AFTER-NETWORK-INTAKE.md)
- 票 `.scratch/docs-consistency/code/29-reroute-suggestions-and-ties-have-no-authorized-decision-face.md`；后继票 `.scratch/docs-consistency/code/97-initial-route-tie-has-no-authorized-decision-face.md`、`.scratch/docs-consistency/code/99-a-control-trigger-between-a-suggestion-and-its-decision-reviews-a-plan-not-yet-effective.md`
