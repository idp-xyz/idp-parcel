# 文档自洽审计：NR（network-routing）+ NO（node-operations）

- 审计版本：只读 worktree `/home/tops/workspace/idp-parcel-audit`，detached @ `6b0074d0`。
- 口径：`.scratch/docs-consistency/README.md`「审计口径」（类别 1–6、A/B/C、严重度）。位置一律用文件路径加引文，不用行号。
- 范围：`docs/domain/network-routing/CONTEXT.md`、`docs/domain/node-operations/CONTEXT.md`；UC-NR-001～003、UC-NO-001～006；与 NR/NO 相关的 ADR（0052、0053、0064、0068、0075、0094、0095、0131、0148、0156、0172～0175、0184、0186、0189、0195、0196、0199、0201、0210～0217、0220、0225、0235 等）；CONTEXT-MAP、GLOSSARY 中的 NR/NO 条目；`internal/networkrouting/**`、`internal/nodeoperations/**` 与 `cmd/parcel-dispatch` 的装配。
- 合计 26 条：高 3，中 8，低 15。

---

## 高

### H1　初始路由：一件包裹停在持久未决，整封接受决定就回滚重投；同委托其余包裹已形成的计划跟着回滚，并列候选与并列理由不留存

- **类别**：4（文档与代码一致），兼 2
- **位置**：
  - `docs/domain/network-routing/CONTEXT.md`「路由形成与选择」：「初始路由遇并列既不形成计划也不形成无当前有效路由，保留全部候选与并列理由并保持可续办」。
  - `docs/application/network-routing/UC-NR-001-CREATE-INITIAL-ROUTE.md`「应用流程」表后：「某个包裹未决或无路由不回滚其他包裹已经合法形成的计划，也不能被一个委托级“已路由”状态掩盖。」；`AT-NR-012`「三个包裹分别保留计划、无路由和未决结果；任一结果不回滚或掩盖其他结果」；元数据「未决结束」行：「……本次没有形成路由计划或无路由判断，但可以安全续办」；结果表「路由判断未决」：「最低成本并列时另交全部候选与并列的那几家」。
- **证据**：
  - `internal/networkrouting/adapters/parcelshipment/route_on_acceptance.go` 的 `outcomeToConsumption`：只要有一件包裹是 `ParcelRouteUndecided`，就交 `ErrRouteHandoffUndecided`，消费门整封回滚重投（`inboxconsume`「消费入账与处理方同一事务……失败回滚」）。
  - `internal/networkrouting/application/create_initial_route.go` 把 `domain.RankingTied` 落成未决 `RouteCandidatesTied`，并列候选只放在内存结果里。排序形态未声明、成本待判断、不可计价、币种不齐、证据未配置、成本源未配置也一样落未决。
  - `adapters/postgres/initial_route.go` 的 `InitialRoutes.Save` 用 `RequireExecutor`，与消费门同在一个事务里。整封回滚时，同一委托里已经判成的包裹，计划也一起撤掉。
  - 这几格重投多少次结果都不变。信封最后落 `ABANDONED`：判成的包裹没有计划，并列候选与理由一样也没留下。
  - ADR-0235 决定六与票 `demo-chain-gaps/155` 只点名了「未配置类」，没算进最低成本并列和成本类各格。CONTEXT 也承认首发形态下「单维恰恰最容易打平」，所以并列会是常见情形。
- **归类**：C（文档是现行规则，代码与它相反）
- **严重度**：高。读者会以为并列可以续办、兄弟包裹不受连坐；演示时实际看到的是整单都没有路由。
- **建议修法**：扩充票 155，或另立一张代码票：初始路由的持久未决（并列、各未配置格、成本待判断、不可计价、币种不齐）照 ADR-0235 决定一、二的形状入账并留下原因，并列候选与理由落库；逐包裹提交，不连坐。文档不用改；如果先修文档，在 UC-NR-001「未决结束」后注明「今天仅依赖类可续办，见票 155」。

### H2　场外揽收触发的复核把揽收地点当成计划节点来比，地点不是计划节点，原计划就判失效

- **类别**：4，兼 1（CONTEXT 内部两句相抵）
- **位置**：
  - `docs/domain/network-routing/CONTEXT.md`「路由形成与选择」：「有效网络收寄结果形成后，必须依据当时已知的实际接货位置、控制依据、当前限制、截单和网络可用性重新校验。……场外揽收时使用 `transport-fulfillment` 的揽收交接结果，不要求尚未发生的到站或节点实测。」
  - 同节下一条：「拒收、待确认与交给运输方的交接不建立可控节点，不复核。」
  - 「路由稳定性与受控改路」：「已执行前缀只由计划段链和当前可控节点切出；可控节点只认节点收寄或权威运输交接。」
  - `docs/application/network-routing/UC-NR-003-REASSESS-ROUTE-AFTER-NETWORK-INTAKE.md` `AT-NR-034`：「场外揽收地点与计划接货范围一致，尚未到节点 | 使用实际接货位置复核，不虚构节点实测或到站」。
- **证据**：
  - `adapters/parcelshipment/reassess_on_intake.go` 的 `triggerSpecFor` 取 `source.Place()`（揽收地点）作复核位置；`controlKindFor` 把 `OffsitePickupSource` 译成 `TransportHandoverControl`。
  - `domain.ReviewPlanApplicability` 拿这个位置去比 `plan.Nodes()`，不在里面就答 `PLAN_ORIGIN_MISMATCH/<地点>`，即 `PlanNoLongerApplicable`。
  - 计划段只有节点间段与交付段（ADR-0217），没有「计划接货范围」或揽收段可比。TF 领域测试里揽收地点的取值是 `customer-warehouse-2` 这类客户场所，不是节点码。
  - `structuralAutoReroute` 把揽收也当成「位于可控节点」。NR 没有 `AT-NR-034` 的测试。
  - 结果：只要揽收地点不是节点码，每一次揽收复核都会让初始计划失效，这与 AT-NR-034 期望的「保留原路由」相反。CONTEXT 一句要求揽收复核，另一句又说交给运输方的交接不建立可控节点，两句对揽收算不算可控节点各执一词。
- **归类**：B（CONTEXT 两句相抵，代码两头都不完全对；计划里没有揽收那一段的模型）
- **严重度**：高。场外揽收的包裹会被误判为原计划失效、无当前有效路由，或被改路。
- **建议修法**：立新 ADR 代决三件事：揽收触发复核时，位置与计划怎样比（首节点的服务区域，或计划带一段揽收段，或只复核硬约束、不比节点）；揽收算不算可控节点（能不能切前缀、能不能自动改路）；它与「交给运输方不建立可控节点」那一句的关系。随后改 CONTEXT 这三句与 AT-NR-034，再立代码票。

### H3　复核没有候选成本取数口：只要有合格候选，排序就报 `ErrInvalidRanking`。原先无路由的包裹因此响亮失败，失效之后的候选评估一律记「未决」

- **类别**：4
- **位置**：
  - `docs/domain/network-routing/CONTEXT.md`：「此前处于无当前有效路由时，候选评估完成且有合格候选则形成首个当前有效路由计划」；「复核遇并列只形成改路建议」；「首版形态是成本改善：差额严格大于已登记阈值才自动切换」；「首次收寄复核使用当时已经取得的物理事实，到站复核使用当前有效物理实测」。
  - UC-NR-003 `AT-NR-037`、`AT-NR-039`、`AT-NR-053`、步骤 6B/7B/7C。
  - ADR-0148 决定四第 7 条（复核的计价输入按物理事实取）。
- **证据**：
  - `application/reassess_route.go` 的 `ReassessRouteDeps` 里没有成本取数口。
  - 生产证据视图 `application/catalog_initial_route_evidence.go` 的 `LoadInitialRouteEvidence` 不填 `CandidateCosts`。只有 `CreateInitialRouteHandler` 经 `deps.Costs.LoadCandidateCosts` 补上（`evidence.CandidateCosts = costs.Facts`），cmd 只给初始路由接了 `routeCosts`。
  - `domain.RankRouteCandidates` 遇到合格候选却没有成本事实时，按装配缺陷答 `ErrInvalidRanking`。
  - 由此三条路的表现：`reassessNoRoute` 上抛错误，整封信封重投到 `ABANDONED`；`reviewCandidates` 把错误吞成 `CANDIDATE_REVIEW_UNDECIDED`，失效之后永远不形成改路决定或建议；`tryImprovementReroute` 在声明了自动改路时同样上抛。
  - 应用测试用 `routableEvidence`、`cheaperAlternative` 直接往证据里塞 `CandidateCosts`，所以测试是绿的。演示种子 `scripts/demo-seeds/data/network/23-route-strategy-cn-sg-v2.json` 声明了 `COST_SINGLE_DIMENSION`。
  - ADR-0235 越权风险点 12 写「今天两种都到不了」，这是第三种，而且今天就能走到。
- **归类**：C
- **严重度**：高。CONTEXT 写的首个计划、受控改路、改路建议，在生产接法上都走不通；失效之后的候选评估一律报「未决」，误导运营。
- **建议修法**：立代码票，给复核编排接上与初始路由同一套候选成本取数（包裹事实按 CONTEXT 区分收寄与到站）；并补一条 cmd 层用真证据跑「先前无路由 → 首个计划」的用例（AT-NR-039）。

---

## 中

### M1　UC-NO-006 的归入条件与门铃重核对象漏了「冲突来由是只因歧义信号」

- **类别**：2
- **位置**：`docs/application/node-operations/UC-NO-006-ESTABLISH-A-VERIFIABLE-DELIVERY-SCOPE.md`
  - 输入契约「归入」行：「实物在同一节点有当前收寄，此刻待识别、不带关联、不记身份冲突」；
  - 应用流程步骤 7：「对交接单名下当前有效版本仍为待识别、不带关联、不记身份冲突的作业实物重核」；
  - `AT-NO-075`：「名下已关联或记身份冲突的实物不受影响」。
- **证据**：
  - NO CONTEXT「送站交接单」生命周期：「同一节点上当前有效版本属于重核对象（待识别、不带关联、不记身份冲突或冲突来由是只因歧义信号）的作业实物可以归入」。
  - ADR-0212 决定三：「经 ADR-0211 放宽到冲突来由是只因歧义信号的冲突」。
  - CONTEXT-MAP `parcel-shipment → node-operations`（外部标识关联已形成的交接）。
  - 代码 `application/delivery_handover_note.go` 的 `assignableJudgments`：`contested := record.IdentityConflict && record.IdentityConflictCause != ports.ConflictByAmbiguityOnly`。`adapters/postgres/reception_handover_recheck_lookup.go` 的 SQL：`(NOT r.identity_conflict OR r.identity_conflict_cause = 'AMBIGUITY_ONLY')`。
- **归类**：A
- **严重度**：中。照 UC 写前端或测试的人，会把只因歧义信号的冲突排除在可归入、可重核之外。
- **建议修法**：三处都改成「不记身份冲突，或冲突来由是只因歧义信号」，并引 NO CONTEXT 的同一句。

### M2　UC-NR-003 仍用「未决」去写重投推不动、或根本不是触发的格；结果表缺「触发未受理」

- **类别**：2
- **位置**：`docs/application/network-routing/UC-NR-003-REASSESS-ROUTE-AFTER-NETWORK-INTAKE.md`
  - 启动条件：「路由策略、网络、日历、截单、限制和资格版本可以按实际业务时点解析；缺失时保持未决。」
  - 应用流程步骤 3「无权威依据时保持未决」，步骤 5「依据不完整时未决」。
  - `AT-NR-040`：「只有普通扫描或卸载，没有控制依据 | 复核未决，不建立当前可控节点」。
  - 「目标与边界」首段：结果列到「……改路建议或判断未决结果为止」；结果语义表没有「触发未受理」一格。
- **证据**：
  - ADR-0235 决定一：证据未配置答`未判`、入账，不是未决。
  - 代码 `domain.NewReassessmentTrigger` 对 `SourceHint` 交 `ErrSourceHintDoesNotTrigger`，`ReassessRouteHandler.Handle` 答 `TRIGGER_NOT_ACCEPTED`（`ReassessTriggerNotAccepted`），不重投。
  - 证据未配置走 `ports.NotJudgedEvidenceNotConfigured`。
- **归类**：A
- **严重度**：中。照这几句实现的人，会把重投推不动的格做成重投，正是 ADR-0235 修掉的那个问题。
- **建议修法**：启动条件与步骤 5 改为「依赖读不动时未决；判断时点证据未配置时入账`触发未判`」；步骤 3 与 AT-NR-040 改为「不受理为复核触发，不建立当前可控节点」；结果表补「触发未受理」，首段补「触发未判」。

### M3　「当前位置」同词异义：NR 指最近一次已判控制触发，GLOSSARY 与 NO 指节点内的作业位置

- **类别**：6
- **位置**：
  - NR CONTEXT「路由稳定性与受控改路」：「这次触发只入账为「触发早于当前位置」」。
  - UC-NR-003 结果表「触发未判」行：「（触发早于当前位置）」，以及 `AT-NR-079`、`AT-NR-082`「当前位置不倒退」。
  - ADR-0235 决定四：「「当前位置」是这一判断键在册复核里业务时间最晚的那一次已判控制触发」。
- **证据**：
  - GLOSSARY「作业位置与当前位置」：「当前位置是依据当前有效位置和集运包含关系派生的结果」，所有者是 `node-operations`；NO CONTEXT「作业位置」词条：「当前位置依据有效位置和包含关系事实派生」。
  - NR 自己已有「当前可控节点」一词。代码码值是 `TRIGGER_PRECEDES_CURRENT_POSITION`。
- **归类**：A（NR 的 Language 未定义这个词，与 GLOSSARY 撞名）
- **严重度**：中。跨上下文读者会把它读成节点内库位那个「当前位置」。
- **建议修法**：NR CONTEXT 与 UC-NR-003 改用「当前控制位置」，或直接写「最近一次已判控制触发」，并在 NR Language 下定义一次，注明不同于 GLOSSARY「作业位置与当前位置」。码值不必改。

### M4　原计划仍适用那条线从不形成改路建议：自动改路未配置、有阻塞、最低成本并列，都静默只保留原计划

- **类别**：4
- **位置**：NR CONTEXT「复核遇并列只形成改路建议」；「不满足自动改路条件时，只形成改路建议，由适用授权的运营角色决定」；「形态或阈值没声明时只形成建议，答未配置」。UC-NR-003 步骤 7C，结果「改路建议已形成」。
- **证据**：
  - `application/reassess_route.go` 的 `tryImprovementReroute`：`authority != domain.AutomaticRerouteAllowed` 时直接 `return record`；`selection.ranking.Outcome() != domain.RankingSelected` 时同样 `return record`，都不挂建议。
  - 只有 `rerouteAfterLapse`（失效那条线）形成建议。`AT-NR-042` 的测试只覆盖失效线。
- **归类**：C
- **严重度**：中。照 CONTEXT 写运营界面的人，会以为仍可执行时也会出现「有更优候选，待授权」的建议。（修 H3 之后这条才会真正暴露。）
- **建议修法**：立代码票，在仍适用且未越过冻结边界时，对「改善大于阈值但权限是仅建议」与「改善候选并列」形成建议；或者由 NR owner 裁定仍适用一路不出建议，再把 CONTEXT 那几句限定到失效那条线。二选一。

### M5　节点监管协作承接决定缺「待补充」一格

- **类别**：4
- **位置**：
  - NO CONTEXT「节点监管协作承接决定」：「决定可以是已承接、部分承接、待补充或拒绝」；规则「对象、动作、节点、授权依据或必要作业条件不足时保持待补充」；生命周期「已承接、部分承接、待补充或拒绝」。
  - GLOSSARY 同名词条。
  - UC-NO-001 结果「节点协作待补充」、`AT-NO-002`、`AT-NO-003`。
- **证据**：`internal/nodeoperations/domain/customs_collaboration.go` 的 `AcceptanceDecisionKind`，注释写「封闭三值：接受、拒接、部分承接」（`ACCEPTED` / `DECLINED` / `PARTIALLY_ACCEPTED`）；`application/accept_collaboration.go` 的结果里也没有待补充。
- **归类**：C
- **严重度**：中
- **建议修法**：立代码票补「待补充」（带缺口与补充入口）；在补上之前，UC-NO-001 状态行注明「待补充未实现」。

### M6　「集运包含无循环」未落代码，连单元装进自己也不拦

- **类别**：4
- **位置**：NO CONTEXT「集运包含关系必须无循环；每个作业实物同一时点最多一个直接物理父级」；GLOSSARY「集运单元」：「全部包含层级不得形成循环」；UC-NO-003 `AT-NO-039`：「集运单元嵌套形成循环 | 拒绝成员关系，原关系不被覆盖」。
- **证据**：
  - `domain/consolidation_unit.go` 的 `AddMember` 只拒已封装、已关闭与重复成员。
  - `application/consolidate_parcels.go` 的 `AddMember` 只问成员自己的 `Containment.CurrentParent`，不查祖先链，也不拒「成员就是单元本身」。
  - 嵌套成员确实存在（`ports.ConsolidationMemberAt.NestedUnit`）。`migrations/node_operations` 里没有对应约束；`AT-NO-039` 没有测试。
- **归类**：C
- **严重度**：中。成员关系会出现环，下游按嵌套展开（ADR-0225 出发门禁）时可能出错。
- **建议修法**：立代码票，在移入时沿目标单元的父级链检查（含自身），撞环就拒，并补 AT-NO-039 测试。

### M7　「方向性作业受阻」：UC-NO-003 写的是系统挡下，NO CONTEXT 与代码是照记不拒

- **类别**：2，兼 1
- **位置**：`docs/application/node-operations/UC-NO-003-PROCESS-CONSOLIDATE-AND-SEAL-PARCELS.md`
  - 启动条件：「方向性作业必须取得当前有效路由指令；集运还必须取得成员级集运兼容判断。」
  - 结果「方向性作业受阻」。
  - `AT-NO-031`「阻止方向性分拣与集包」；`AT-NO-033`「只对兼容成员实际集运」。
- **证据**：
  - NO CONTEXT「集运、封装与载具」：「已经发生的移入（一线过渡导入、离线回传）照记为集运成员关系事实，不因方向依据缺失或读不动而拒收、悬置或回写」。
  - 同一份 CONTEXT「作业授权与执行」又写：「分拣到下一方向、集包出运、装载和交出等方向性作业必须暂停，直至取得有效依据」。括注是只限这两种来路，还是举例，没有说清。
  - 代码：`ConsolidateParcelsHandler.AddMember` 只要求 `basis.Recorded()`，在线口也照落；`adapters/networkrouting/routing_basis.go` 的 `UnconfiguredRoutingBasis` 对任何实物都答`未配置`；`ConsolidationOutcome` 里没有「受阻」一格，也没有设备在动作之前问的读口（出发门禁那边有 ADR-0219）。
- **归类**：B（CONTEXT 两句的适用边界没裁，UC 与代码各取一边）
- **严重度**：中
- **建议修法**：立 ADR 代决：「暂停」落在设备在动作之前问（照 ADR-0219），服务端对已经发生的移入一律照记、只记方向依据。然后改 CONTEXT 那条的括注与 UC-NO-003 的启动条件、结果表与 AT-NO-031、AT-NO-033。

### M8　「适用性看不清」被归为重投推得动，但它的来源里有重投推不动的格

- **类别**：2
- **位置**：
  - ADR-0235 决定一：「重投推得动的照旧是未决……复核登记册或路由历史读不回、证据读不动、适用性看不清、取号依赖故障」。
  - UC-NR-003 `AT-NR-067`：「复核停在依赖类未决（……适用性看不清……）」。
- **证据**：
  - `domain.ReviewPlanApplicability` 遇 `ConstraintStatusUnknown` 答 `PlanReviewUndecided`；`reassessPlan` 一律折成 `PlanReviewInconclusive`，回滚重投。
  - 关务取数侧 `adapters/customscompliance/customs_source.go` 的 `unknownGap` 有两种来源：`CUSTOMS_PORT_PATH_CATALOG_EMPTY`（租户没登目录，等登记）与 `CUSTOMS_ENDPOINT_COUNTRY_MISSING/<端>`（受理基线的地理投影缺国家，而基线一经固定不再改写）。
  - 这两格都是重投推不动的，与 ADR-0235 自己的判据「重投推不动的答`未判`」相抵。
- **归类**：B
- **严重度**：中。这两格会照旧占着交接分区重投到 `ABANDONED`；AT 行把它们写成依赖类，误导运维去查一个没坏的依赖。
- **建议修法**：立 ADR（或在票 154 同族里）代决：把「看不清」按缺口来源再分，目录为空、端点国家缺入账`未判`并留下原因，只留真正的依赖故障重投；随后收窄 AT-NR-067。

---

## 低

### L1　NO CONTEXT 两处「重核对象」口径不一；还缺「信号没了、只缺范围」那一支

- **类别**：1
- **位置**：
  - NO CONTEXT 生命周期「待识别实物与身份关联」：「未核对、没有候选、缺可验证交付范围、或只因歧义信号形成身份冲突的待识别实物 → 重核」。
  - 「送站交接单」：「重核对象（待识别、不带关联、不记身份冲突或冲突来由是只因歧义信号）」。
  - 规则：「只因歧义信号形成的那一格另有一条路：……答出唯一命中即建立关联，冲突随之解开；仍带歧义信号时判断不变。」
- **证据**：
  - 第一处的枚举没有覆盖 UC-NO-002 `AT-NO-066`（有范围、范围内零候选、范围外有候选），也没有覆盖处置「都不是」之后的待识别，而 CONTEXT 自己说它们「此后可再经重核」。
  - 代码 `RecheckPendingIdentityHandler.recheck` 只看「待识别、无关联、无硬冲突、来源已保全」，与 ADR-0212 决定六一致。
  - 缺的一支：ADR-0212 决定六「重算后信号没了……追加一版`待识别实物已建立`：不记冲突」，代码 `RecheckAmbiguityGoneScopeUnverified`；UC-NO-002「控制、身份与后续作业」已经写了这一支，CONTEXT 没写。
- **归类**：A
- **严重度**：低
- **建议修法**：重核对象只在 CONTEXT 里定义一处（取第二处的写法），第一处改为引用；规则那句补上第三支。

### L2　过时的实现现状句

- **类别**：4
- **位置**：
  - UC-NR-003「给开发的交接」：「当前仓库没有 `network-routing` 代码、真实网络配置、计划持久化或事件闭环。」
  - UC-NO-002「给开发的交接」：「……尚无 `node-operations` 代码、持久化、事务或事件闭环。」
  - UC-NO-003「给开发的交接」：「当前代码没有 `node-operations` 模块、设备接入、作业持久化或事件闭环。」
  - UC-NO-006 状态行：「稳定业务骨架已定（ADR-0212），待实现」。
  - UC-NO-002 结果段：「身份核对读不动在 ADR-0199 决定五所列前提齐备之前照 ADR-0186 不形成答案，齐备之后建立待识别实物并记未核对。」
- **证据**：`internal/networkrouting/**`、`internal/nodeoperations/**` 都已在库，预报与交接单口（`application/delivery_pre_advice.go`、`delivery_handover_note.go`）已实现；`ErrParcelIdentityUnreadable` 的注释写「收寄据此落身份核对态未核对的待识别（决定五）」，翻转已经落地；ADR-0186 Status 行写着「复核记录（2026-10-19，ADR-0199 决定五部分停用两处）」。
- **归类**：A
- **严重度**：低
- **建议修法**：删去或改写这几句的现状描述；UC-NO-002 那句只留翻转之后的现行答法。

### L3　AT 行与规则以现行口吻写着尚无代码的能力，没有任何标注

- **类别**：4
- **位置**：
  - 路由指令：UC-NR-003 步骤 7B「形成改路决定、新计划和新路由指令」，`AT-NR-037`「形成新计划和指令」。
  - 路由偏离：`AT-NR-035`「形成偏离和剩余旅程重评」。
  - 集运兼容判断：NR CONTEXT「封装和装载前必须由 `network-routing` 按成员当前有效路由计划形成……集运兼容判断」，UC-NO-003 `AT-NO-033`。
  - 实测触发：UC-NR-003 元数据「触发」里的「当前有效实测」，`AT-NR-036`/`037`，UC-NO-003 步骤 5、`AT-NO-030`。
  - 人工改路：NR 生命周期「授权角色接受建议 → 形成改路决定」。
- **证据**：NR 没有路由指令、偏离、集运兼容与人工改路决定的模型；NO 的 `adapters/networkrouting/routing_basis.go` 注释写「NR 今天没有路由指令与成员级集运兼容判断的模型」；NO 没有实测代码；复核只有收寄采用与交接两种触发。
- **归类**：C（文档写的是目标规则，代码没有实现）
- **严重度**：低
- **建议修法**：各 UC「给开发的交接」列出这几块尚未实现及票号；或者在 AT 行加「待实现（票 …）」标注。

### L4　AT-NR-033、AT-NR-036 没写冻结形态已声明这个前提，与 AT-NR-076 是同一场景两套结果

- **类别**：2
- **位置**：UC-NR-003 `AT-NR-033`「实际节点与初始计划一致 | 记录复核依据并保留原路由」；`AT-NR-036`「当前有效实测仍满足原计划硬限制 | 原计划继续适用」。
- **证据**：`AT-NR-076`「原计划仍可执行，策略版本没声明冻结形态 | `触发未判`，原因 `FREEZE_FORM_UNCONFIGURED`」；代码 `freezeOf` 在仍适用时先判冻结，未声明就落未判、不落复核。
- **归类**：A
- **严重度**：低
- **建议修法**：给 AT-NR-033、AT-NR-036 的场景补前提「策略版本已声明冻结形态」。

### L5　失效之后的自动改路仍要新候选比失效计划便宜且超过阈值，AT-NR-037 没写这个条件

- **类别**：2
- **位置**：UC-NR-003 `AT-NR-037`「当前有效实测使原线路不再合格，存在替代候选且允许自动改路 | 形成新计划和指令」；NR CONTEXT 把改善阈值写在「当前路由仍可执行时，轻微……改善只有在冻结边界前且超过适用策略的改善阈值时，才允许自动切换」那一句里。
- **证据**：ADR-0173 决定三「失效后同样按本形态折自动条件」；代码 `rerouteAfterLapse` 拿新候选与失效计划比 `costImprovement`，不达阈值就只形成建议 `IMPROVEMENT_BELOW_THRESHOLD`；测试 `TestAllConditionsMetFormsAnAutomaticRerouteDecision` 用的是 `cheaperAlternative`。原计划本是最便宜的，替代候选通常更贵，所以失效之后几乎不会自动改路。
- **归类**：B（CONTEXT 的限定与 ADR-0173 决定三两种读法）
- **严重度**：低
- **建议修法**：NR owner 裁定失效之后还比不比改善；维持现状就把条件写进 CONTEXT 与 AT-NR-037。

### L6　NR CONTEXT「币种不齐时停下不比」没提比较币种

- **类别**：1
- **位置**：NR CONTEXT「路由形成与选择」：「已计价候选币种不齐时停下不比」。
- **证据**：ADR-0148 决定四第 2、4 条：比较币种是策略版本上的租户取值，换算由 PP 完成，没登比较币种时才只在同币种下比；代码 `adapters/parcelpricing/cost_source.go` 按 `HasComparisonCurrency` 换算。
- **归类**：A
- **严重度**：低
- **建议修法**：补半句：「策略版本登了比较币种时，候选成本按比较币种合成；没登时，已计价候选币种不齐即停下不比」。

### L7　NR CONTEXT 在现行规则节里写历史经过

- **类别**：1
- **位置**：NR CONTEXT「路由形成与选择」：「并列出口此前按候选标识升序收尾。`label-channel-service-first-release/01` 裁定渠道择优不得那样收尾时，判路由侧仍可保留……」。
- **证据**：目标 1「ADR 只作取舍的历史与理由」。
- **归类**：A
- **严重度**：低
- **建议修法**：只留现行一句「最低成本并列不收尾、交授权角色，不损幂等」；把经过挪进 ADR 或票里引用。

### L8　CONTEXT-MAP 的 NR 条目只写 `BUY` 评价，还说「成本维度」

- **类别**：1，兼 6
- **位置**：`docs/domain/CONTEXT-MAP.md` NR「拥有」：「择优消费 `parcel-pricing` 对每个候选形成的 `BUY` 评价作为成本维度，按版本化策略裁决」；`parcel-pricing → network-routing` 边：「计价对每个候选分别形成 `BUY` 评价」。
- **证据**：ADR-0148 决定四第 1 条：自营段经内部价格政策评价；NR CONTEXT 说首发是「成本单维」；代码 `nrpartycommercial.NewInternalPlanResolver` 已接入内部政策。
- **归类**：A
- **严重度**：低
- **建议修法**：改成「`BUY` 评价或内部价格政策评价（ADR-0148 决定四）」，去掉「成本维度」这个说法，或注明首发只有成本单维。

### L9　UC-NR-002 输入契约写「读取」客户地址与申报资料

- **类别**：2
- **位置**：`docs/application/network-routing/UC-NR-002-ASSESS-PARCEL-REACHABILITY.md` 输入契约「包裹声明」行：「客户地址、声明测量、货物、申报原始资料及各自版本 | 只按判断所需最小范围读取」。
- **证据**：ADR-0075 决定一（不建读地址的端口、随请求携带）与决定二（只有地理投影过界）；ADR-0148 决定一（随请求携带的只有地理投影、服务要求与承诺上界）；代码 `ports.RequestCarriedContent` 只有 `Geo` 与 `Commitment`。
- **归类**：A
- **严重度**：低
- **建议修法**：改为「随判断请求携带地理解析投影（ADR-0075），NR 不回读、不留存地址本体」；货物与申报一格改成「经关务适用性判断引用」。

### L10　GLOSSARY 与 NR/NO CONTEXT 同词重复定义，且有出入

- **类别**：6
- **位置与证据**：
  - NO CONTEXT 的词条叫「实物控制」，GLOSSARY 叫「实物控制关系」，同一概念两个名字。
  - GLOSSARY「待识别实物」：「各实物保持独立并形成身份冲突，正式身份由 `parcel-shipment` 确认」。冲突其实由 NO 的身份冲突处置定（NO CONTEXT「身份冲突处置」），ADR-0210 越权风险点 2 要靠特别读法才圆得过来。
  - GLOSSARY「集运单元」说「把多个包裹临时组合」，而 NO CONTEXT「集运单元实例」写「临时组合包裹或其他允许成员」，代码允许嵌套单元（`NestedUnit`）。
  - NR Language（物流节点、服务区域、网络连接、线路、可达性判断、路由计划、无当前有效路由、路由指令、外部不透明履约段）与 NO Language（作业实物、待识别实物、身份冲突处置、可验证交付范围、送站预报、送站交接单、节点侧交接证据、作业任务、作业批次、在场核对、受控开封、物理状况观察、内部作业标签、节点监管协作承接决定等）在 GLOSSARY 里各有一份全文定义，措辞细节不同。例如「物流节点」两边列的作业种类不一样，「无当前有效路由」一边叫「明确判断」、一边叫「计划适用性判断」。
- **归类**：A
- **严重度**：低
- **建议修法**：跨上下文的词只在 GLOSSARY 定义，CONTEXT 引用；名字统一为一个；改正 GLOSSARY「待识别实物」那半句（写成「正式包裹身份由 PS 给出，实物与包裹的关联与冲突处置归节点」）与「集运单元」的成员范围。横切组统一处理时，按这里列的词条逐一合并。

### L11　NO CONTEXT「可以形成单元控制」以现行口吻写，而单元控制规则尚未确立

- **类别**：1
- **位置**：NO CONTEXT「接收、控制与交接」：「封签连续且相符的封闭集运单元被有效接收后，可以形成单元控制及成员“随单元受控”的派生判断」；GLOSSARY「实物控制关系」有同一句；UC-NO-003 `AT-NO-038`。
- **证据**：同一份 CONTEXT「节点控制」写「单元控制规则确立之前不推进成员的控制」；代码对集运单元交接一律入账 `CONSOLIDATION_UNIT_PENDING_RULE`；UC-NO-005 写「集运单元的单元控制与成员「随单元受控」：规则未立」。
- **归类**：A
- **严重度**：低
- **建议修法**：给那一句加「（规则未立，见 UC-NO-005）」，或者挪到「待定规则」小节；AT-NO-038 同样标注。

### L12　AT-NO-062 一行里叠了两套结果

- **类别**：2
- **位置**：UC-NO-002 `AT-NO-062`：先写「重核追加一版节点收寄关联那件包裹……」，后写「ADR-0212 之后，送站没有可验证交付范围、号码只经客户包裹件号命中时，重核追加的是一版不记冲突的待识别……有范围时照本行」。
- **证据**：目标 2「同一场景只有一套结果」；现行判法见 ADR-0212 决定六，代码 `RecheckAmbiguityGoneScopeUnverified`。
- **归类**：A
- **严重度**：低
- **建议修法**：拆成两行（有范围或渠道件号；无范围且只经客户件号），删掉「ADR-0212 之后」这种按历史叠写的措辞。

### L13　14 份 NR/NO 相关 ADR 的 Status 行把同一次停用写了两遍

- **类别**：3
- **位置**：ADR-0052、0053、0064、0068、0075、0175、0186、0195、0196、0199、0201、0210、0214、0215 的 `Status` 行。被停的事实在 `<!-- 索引注记 -->` 前后各写了一遍。例如 ADR-0215 前段「由此**部分停用**决定四「重投」一条的未决范围……」，索引注记里又写「｜**部分停用**：其决定四……」。
- **证据**：README 目标 3「同一次停用不写两遍（infra-speed/07 并入本轮）」；票 `infra-speed/07`。抽查这 14 份，两边内容一致，没有互相矛盾的。
- **归类**：A
- **严重度**：低
- **建议修法**：交横切组按 infra-speed/07 合并，本组不单独改。

### L14　ADR-0235 落`未判`而不带续办触发，与它援引的先例 ADR-0094 决定四相抵，两者之间没有停用关系

- **类别**：3
- **位置**：ADR-0235 决定六「续办触发不在本记录」、越权风险点 1。
- **证据**：ADR-0094 决定四：「第三格必须与它的续办触发同笔落地，否则不许落地。」ADR-0235 Context 把 ADR-0094 当作「按恢复动作分格」的出处，Links 还写「续办触发同笔落地（决定四）本记录未做」。严格说 ADR-0094 的适用范围是 PS 接受链，不算同域硬矛盾，但两份已接受记录对「入账而没有续办」持相反立场。
- **归类**：B
- **严重度**：低
- **建议修法**：在 ADR-0235 的 Status 行注明「ADR-0094 决定四的适用范围限于 PS 接受链，NR 复核暂不受它约束，续办见票 154」；或立一份记录裁定这条纪律是否跨上下文适用。

### L15　Go 注释与 ADR 相反或已经过时

- **类别**：4（代码注释）
- **位置与证据**：
  - `cmd/parcel-dispatch/assemble_reassess_on_handover.go` 的 `reassessRouteHandlerOver` 里，给 `AutoReroute` 的注释写「登记过的键才走 7B/7C」，说复核按事实目录决定改路。这与 ADR-0173 决定四、NR CONTEXT「自动改路事实目录不再是判断权威，复核不读它」相反，也与 `ReassessRouteDeps.AutoReroute` 字段注释「复核不读它（ADR-0173）」相反；编排确实没有读它。
  - `internal/networkrouting/ports/ports.go` 的 `RequestCarriedContent` 注释写「首版只含地理解析投影；服务要求与承诺上界随 routing-first-cut/08、09 加入」，而结构体已经有 `Commitment`。
- **归类**：C
- **严重度**：低
- **建议修法**：随下一张 NR 代码票改这两处注释，用中文写现行规则，不留变更叙述。

---

## 已核无问题的要点

| 要点 | 结论 |
|---|---|
| ADR-0215 ↔ ADR-0235 停用链 | 两头都在：ADR-0215 的 Status 行、Links 指向 0235；0235 的 Consequences 引的两句原文（决定四「复核停在未决（……）」、决定三「更正之后仍是`已交接`……」）在 0215 正文里逐字存在。 |
| ADR-0075 ↔ ADR-0215 | 两头都在。NR CONTEXT「客户地址继续由 `parcel-shipment` 保存」一条，与 CONTEXT-MAP `parcel-shipment → network-routing`、UC-NR-003「客户地理投影」行，都已写出「受理后、不由 PS 信封触发的复核」这一例外。 |
| ADR-0199 / 0195 / 0184 / 0196 / 0210 / 0211 / 0212 / 0214 / 0220 停用链 | 用脚本核过：各 Status 行引的被停原句在被停记录正文里逐字存在；停用方的作用与被停方的前向指针成对。 |
| NR 复核结论封闭集 | `STILL_APPLICABLE` / `PLAN_LAPSED` / `FIRST_PLAN_FORMED` / `REROUTED`，候选评估三态，与 ADR-0235「结论封闭四格」、UC-NR-003 结果表一致。 |
| 未判原因码 | `NO_ROUTING_HISTORY`、`REASSESS_EVIDENCE_NOT_CONFIGURED`、`FREEZE_FORM_UNCONFIGURED`、`TRIGGER_PRECEDES_CURRENT_POSITION` 与 AT-NR-074～079 一致；迁移 0021、0022 与表名、列名一致。 |
| 按业务时间排、同一时刻照判、交接更正承继判断时刻 | NR CONTEXT、UC-NR-003、ADR-0235 与 `ReassessmentTrigger.PrecedesPosition`、`LatestTriggerAt` 一致。 |
| 冻结边界（剩余段数，`≤` 即越过）、自动改路成本改善（`>` 阈值）、排序成本单维（待判断与不可计价出局、币种不齐停下、并列不收尾） | 与 ADR-0172、0173、CONTEXT 和 `JudgeFreezeBoundary`、`FoldAutoReroute`、`RankRouteCandidates` 一致。 |
| 交付段（ADR-0217）、计划段引用（ADR-0131） | CONTEXT、GLOSSARY、UC-NR-001 AT-NR-054～063 与 `PlanNodes(NodeLegs())` 一致。 |
| 交接触发复核的消费者名与装配 | `network-routing/reassess-on-transport-handover` 与 ADR-0215 决定五一致；收寄与交接两路共用 `reassessRouteHandler`。 |
| 节点控制转出与转入的码值 | `NO_ASSOCIATED_UNIT`、`SEVERAL_ASSOCIATED_UNITS`、`CONSOLIDATION_UNIT_PENDING_RULE`、`NO_ACTIVE_CONTROL`、`SEVERAL_ACTIVE_CONTROLS`、`HANDOVER_PRECEDES_CONTROL`、`CONTROL_SOURCE_CHAIN_PENDING_RECOMPUTE`、`CONTROL_STILL_ACTIVE`、`HANDOVER_PRECEDES_RELEASE`、`HANDOVER_IN`、`mismatchedReleasingNode`、`controlReleasedBy`、`registry=physical-control`，与 UC-NO-005、NO CONTEXT「节点控制」一致；更正链按回指认，与 ADR-0220 一致。 |
| 收寄与身份判断码值 | 收寄结果五格加并入、身份判断表（`UNIQUE` / `UNKNOWN` / `SCOPE_UNVERIFIED` / `AMBIGUITY_ONLY` / `MULTIPLE_CANDIDATES`）、冲突来由三值、交付范围核对态与五个未证原因，与 NO CONTEXT、UC-NO-002 和 ADR-0212 决定四、五一致。 |
| 身份冲突处置七格结果 | 与 UC-NO-004 一致。 |
| 授权交付判断问句五维（含开立节点） | 代码 `AuthorizedDeliveryQuery`、UC-NO-006、CONTEXT-MAP 一致；ADR-0212 决定三写的是四维，已由它 2026-10-19 的复核记录补上「开立节点」。 |
| 方向依据记录 | 三格五值与 NO CONTEXT「方向依据记录」一致。 |
| 引用规范（目标 5） | 本组 11 份文档没有跨文件行号、计数或位置引用；用脚本核过本组文档与相关 ADR 的相对链接与锚点，全部能解析。NR/NO Go 注释里也没有行号引用。 |
