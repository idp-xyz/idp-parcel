# 架构决策记录

本目录记录国际小包网络运营系统中难以逆转、存在真实替代方案且无法仅从实现理解的持久架构决策。

## 已接受决策

一条记录只有个别条款被后续记录停用、整体仍然有效时，标为**部分停用**：改被停用记录的 `Status` 行与 Links 节各加一条前向指针，并在本节该行后写明**被停条款**（引原句）与「其余各条不变」。若停用理由本身带前提，还要写明**适用场景**——理由有范围，结论就继承范围，不写等于把一次有条件的停用读成无条件的。两者是不同的东西，别并成一个「范围」。同一机制也用于标注正文中已失效的引用坐标：`Status` 行注明坐标已移位、按符号名与引文定位。改整份记录的去向走下面「已被取代决策」。

三者都不改写正文——红线护的是「谁在何时、凭什么作了什么决定」可审，而承载它的是 Decision 与 Alternatives，不是文件字节不变（`Status` 行在本仓被取代时本就会改写，见 0011、0018、0019、0020）。

跨文件引用的写法以 [AGENTS.md 的「改文档」节](../../AGENTS.md#改文档)为准，本节不复述。这里只记它在 ADR 上为何格外咬人：**一份 ADR 引的往往正是它自己要改的那份代码，因此它在自己被实现的那一刻就腐。** ADR-0027 的 Context 就是实例——它按行号引 `acceptance_basis.go`，而实现它的那次改动在被引位置上方插了近百行，坐标当场全部移位，被引的那句注释本身却一字未改。

- [ADR-0001：国际小包采用自治产品与领域边界](./0001-autonomous-product-domain-boundary.md)
- [ADR-0002：国际小包采用独立数据、运行与发布边界](./0002-independent-data-runtime-release-boundary.md)
- [ADR-0003：采用集团租户、法人责任与货主客户账户三级边界](./0003-group-tenant-legal-entity-customer-account.md)
- [ADR-0004：分离客户承诺、计划旅程与实际事实](./0004-separate-commitment-plan-actual.md)
- [ADR-0005：由来源事实形成有效事件并派生状态](./0005-source-facts-effective-events-derived-state.md)
- [ADR-0006：场站节点管理小包网络作业而非完整 WMS](./0006-parcel-node-operations-not-full-wms.md)
- [ADR-0007：分离物流运营结算与法定财务账](./0007-separate-operational-settlement-from-statutory-finance.md)
- [ADR-0008：试点证据采用自建 MinIO S3 对象存储](./0008-self-hosted-minio-evidence-store.md)
- [ADR-0009：采用 Go 模块化单体并复用版本化 Bento 技术合同](./0009-go-modular-monolith-and-versioned-bento-contracts.md)
- [ADR-0010：以小包网络运营闭环作为首发核心](./0010-parcel-network-operations-as-product-core.md)
- [ADR-0012：在 idp-parcel 内建立小包计价上下文](./0012-parcel-pricing-context-within-idp-parcel.md)
- [ADR-0013：由计价拥有版本化的外部数值序列并在评价内完成币种换算](./0013-pricing-owns-versioned-external-reference-series.md)
- [ADR-0014：为内容摘要的规范化形状引入版本号](./0014-versioned-canonicalization-shape-for-content-digest.md)
- [ADR-0015：以封闭文法与开放词汇实现多承运商通用计价](./0015-closed-grammar-open-vocabulary-for-multi-carrier-rating.md)
- [ADR-0016：产品交付与租户试点作为两条并行验收轨道](./0016-product-delivery-and-tenant-pilot-as-parallel-tracks.md)
- [ADR-0017：实现准入闸门按阻断理由分别裁决](./0017-admission-gates-judged-by-blocking-cause.md)
- [ADR-0021：一线作业客户端属于产品，范围限于本产品拥有的节点作业](./0021-frontline-operations-client-is-part-of-the-product.md)
- [ADR-0022：HTTP 状态码只回答「有没有形成答案」，业务判别一律进响应体](./0022-http-status-carries-answer-formed-not-business-verdict.md)｜**部分停用**：其 Decision 中「两项未决期间，业务端点可以存在并由测试替身驱动，但不进 `cmd/parcel-api` 的装配」的末段「但不进 `cmd/parcel-api` 的装配」已由 [ADR-0055](./0055-business-endpoint-intake-has-an-unconfigured-grade.md) 停用——业务端点改为带「未配置即拒」Intake 进装配，未配置自成一格如实作答。该句前段（端点可存在并由测试替身驱动）与同段「禁止在两项未决前落地任何默认实现」不变，其余各条不变。
- [ADR-0023：作业事实的身份与发生时间由设备签发，服务端不重签、不校正、不按接收顺序定序](./0023-work-fact-identity-and-time-are-minted-by-the-device.md)
- [ADR-0024：方向性作业的依据随对象下发并携带有效区间，出区间等同无有效依据](./0024-directional-work-basis-carries-a-validity-interval.md)
- [ADR-0025：跨上下文调用的适配器落在消费侧，翻译职责由它独占](./0025-cross-context-adapters-live-on-the-consumer-side.md)｜**部分停用**：其 Decision 中「适配器为此需要的实例半边协作者，其接口定义在适配器包内，不进消费方 `ports`」一句已在跨上下文多步协议场景由 [ADR-0027](./0027-multi-step-cross-context-protocol-state-held-by-the-provider.md) 停用——该协作者已由提供方以第二阶段用例提供，适配器直接调用，不另定接口。其余各条不变。
- [ADR-0026：为产出消费者证明而写的持久化实现先于闸门通过，发布基线登记仍在闸门后](./0026-persistence-written-for-consumer-proof-precedes-gate-passage.md)
- [ADR-0027：跨上下文多步协议的中间状态由提供方按解析标识保留，消费方端口按协议阶段分方法](./0027-multi-step-cross-context-protocol-state-held-by-the-provider.md)｜**部分停用**：其 Decision 中「不一致按`输入未受理`处理」一句所指定的取值已由 [ADR-0029](./0029-retrieval-failure-algebra-is-partitioned-by-recovery-action.md) 改指为`依据未解析`——该句要防的「解析标识不得成为一张能力凭证」不但保留且被加强，原先只挡读取，现在连回答也不泄。**适用场景**：仅限「按标识取回原状态失败」这一步；该句其余部分与本记录各条不变。
- [ADR-0028：聚合的重建与构造分属两扇门，重建只校验不重算；聚合携带版本，保存按预期版本写入](./0028-aggregate-rehydration-is-a-separate-door-that-validates-without-recomputing.md)
- [ADR-0029：按标识取回原状态失败时，结果代数按消费方的恢复动作分格，不按提供方的失败原因分格](./0029-retrieval-failure-algebra-is-partitioned-by-recovery-action.md)
- [ADR-0030：聚合重建按状态逐个开门，一个状态只有在快照能表达它可达的全部字段时才准进](./0030-rehydration-admits-one-state-at-a-time-by-snapshot-expressiveness.md)
- [ADR-0031：自有仓储端口的写入结果是封闭代数而不是 error，预期版本由聚合携带](./0031-owned-repository-write-outcome-is-a-closed-algebra-not-an-error.md)
- [ADR-0032：商业依据解析在采用之前必须确认正文里指名的引用，跨上下文的那一半只收所有方的答复](./0032-resolution-confirms-references-named-in-the-adopted-content.md)
- [ADR-0033：`依据未解析` 在消费方端口自占一格，既不并进`已失效`，也不在消费侧拆回两格](./0033-basis-not-resolved-is-its-own-value-on-the-consumer-port.md)
- [ADR-0034：计价闭包经商业价格政策采用，结果带回方向与定价方案绑定](./0034-pricing-closure-adopts-via-price-policy.md)
- [ADR-0035：发布时「批准角色未确认」与「批准依据字段不全」分格](./0035-publication-approval-role-is-separate-from-incomplete-basis.md)
- [ADR-0036：发布前必须确认正文指名引用已发布，未确认不得建立生产引用](./0036-publication-requires-named-references-published.md)
- [ADR-0037：发布批次是逐对象折叠，不是全有或全无的聚合](./0037-publication-batch-is-per-item-not-all-or-nothing.md)
- [ADR-0038：有效性更正保留原版本与更正关系，不改正文](./0038-validity-correction-keeps-original-and-relationship.md)
- [ADR-0039：渠道账号使用授权发布必须有显式业务授权，技术可用不能顶替](./0039-channel-account-use-authorization-requires-business-grant.md)
- [ADR-0040：商业版本身份键携带 TenantID，跨租户同号互不可见](./0040-commercial-version-key-carries-tenant-id.md)
- [ADR-0041：业务参与方与货主客户账户携带 TenantID](./0041-business-party-and-customer-account-carry-tenant-id.md)
- [ADR-0042：接受内容声明按对象归属建模，解析后经独立只读端口读取](./0042-acceptance-content-declarations-by-owning-object.md)
- [ADR-0043：发布意图由结果标识认领，重放重发同一份——事务发布落地前的统一缝形](./0043-publish-intent-claimed-by-result-identity.md)
- [ADR-0044：结算政策依据经政策采用，结果带回预付/账期方式与适用范围](./0044-settlement-basis-adopts-via-settlement-policy.md)
- [ADR-0045：同一委托的新提交版本——当前版本单指针加历史追加，判断任务随版本重立](./0045-new-submission-version-keeps-history-and-reestablishes-the-task.md)
- [ADR-0046：候选评估规则归 network-routing 领域，证据端口退为取数](./0046-route-candidate-evaluation-lives-in-the-domain.md)
- [ADR-0047：账期方式的接受前控制形成信用暴露，不冒充资金冻结](./0047-terms-control-forms-credit-exposure-not-a-freeze.md)
- [ADR-0048：声明测量是成员级保真画像，不换算不推导](./0048-declared-measurements-are-verbatim-member-profiles.md)
- [ADR-0049：集成事件的发布通道首发采用进程内直投，消息中间件等负载证据](./0049-publish-channel-is-in-process-delivery-until-load-evidence.md)
- [ADR-0050：服务产品形态随解析闭包可观察](./0050-service-product-form-observable-through-the-resolution-closure.md)
- [ADR-0051：资格审核答复含等待补充；一次性只约束终局格](./0051-eligibility-screen-awaits-supplement.md)
- [ADR-0052：网络证据端口增设「未配置」格，网络定义登记册的模式属机制半边](./0052-network-evidence-catalogue-has-an-unconfigured-grade.md)｜**部分停用**：其 Decision 四中「它要能承载 `NetworkEvidence` 与 `InitialRouteEvidence` 两个结构今天已声明的全部事实族」一句已由 [ADR-0053](./0053-network-fact-families-are-derived-not-registrable.md) 停用——九族以每次判断生成的 `CandidateID` 为轴、且判断键里没有目的地维，它们是推导结果不是可登记的目录行。其余各条（三格端口、专格未决原因、修订由登记册派生、空册与空集合可分辨）不变。
- [ADR-0053：网络事实族是推导结果不可登记；登记册只登定义存在，无解析层时一律答未配置](./0053-network-fact-families-are-derived-not-registrable.md)｜**部分停用**：其 Decision 三中「现在不建表」半句已由 [ADR-0068](./0068-versioned-network-catalog-structure-precedes-rule-content.md) 停用——目录**结构**由 CONTEXT 硬句定死不依赖取值，先行落表；「不从事实族倒推」「内容形态不替租户拟」与其余各条不变。
- [ADR-0054：接受前财务控制策略视图增设「未配置」格](./0054-pre-acceptance-control-policy-view-has-an-unconfigured-grade.md)
- [ADR-0055：业务端点 Intake 增设「未配置」格，端点装配属机制半边](./0055-business-endpoint-intake-has-an-unconfigured-grade.md)
- [ADR-0056：区间更正只增多条，同版本写入以版本行锁串行化](./0056-validity-correction-append-only-serialized-by-version-row.md)
- [ADR-0057：价格政策册保全发布期邻接答复，不重查也不绕过绑定校验](./0057-price-policy-preserves-publish-time-adjacent-replies.md)
- [ADR-0058：阶段内容声明按拥有规则对象归属，产品与合同只采用](./0058-stage-content-owned-by-rule-objects.md)｜第三条「不得从 `SourceIdentity` 发明采用版本」不变；从已接受快照回指闭包的路径由 [ADR-0062](./0062-adopted-stage-owner-from-accepted-resolution.md) 补上
- [ADR-0059：规则包五维适用性照存但不参与选择](./0059-rule-package-applicability-stored-not-selected.md)
- [ADR-0060：包裹反查走当前快照投影列，不另表、不扫 JSONB](./0060-parcel-lookup-uses-current-snapshot-projection.md)
- [ADR-0061：已接受委托的重建门按快照表达能力开门](./0061-accepted-shipment-request-rehydration-by-snapshot-expressiveness.md)
- [ADR-0062：采用规则版本从已接受解析标识回指提供方持有的闭包](./0062-adopted-stage-owner-from-accepted-resolution.md)
- [ADR-0063：收寄硬资格证明由消费侧窄口取证，商业上下文只声明开放引用](./0063-intake-qualification-proof-is-a-consumer-side-evidence-port.md)
- [ADR-0064：初始路由适用性闭包标识从已接受解析标识回指](./0064-initial-route-applicability-closure-from-accepted-resolution.md)
- [ADR-0065：追踪投影版本只增不改写；替代关系由源上下文给出，不进冲突裁决](./0065-projection-versions-are-append-only-and-supersession-is-source-given.md)
- [ADR-0066：多载运对象信封在消费侧按成员循环拆分；成员维进事实引用，不进事实类型](./0066-multi-object-envelope-unrolls-per-member-on-the-consumer-side.md)
- [ADR-0067：预期成本纠错版本整组重述一个评价的计价结果，同币种两额相等对所有版本成立](./0067-cost-correction-restates-the-whole-evaluation-result.md)
- [ADR-0068：版本化网络目录结构先行——七表由 CONTEXT 硬句推导，内容列与折叠规则等 PAR-NET-14](./0068-versioned-network-catalog-structure-precedes-rule-content.md)
- [ADR-0069：关务案件链乱序由重读与重试消化，不由分区保证；关闭信封携关闭周期维](./0069-customs-case-chain-ordering-absorbed-by-reread-and-retry.md)
- [ADR-0070：关务规则登记册分记录侧与选择侧；解释规则的选择侧违反硬句 191，案件要求规则的版本维随同一模型决定裁](./0070-customs-rule-registries-split-recording-from-selection.md)
- [ADR-0071：visibility-exception 的目录与策略视图把租户放进方法签名，不在构造期绑定](./0071-catalogue-views-carry-tenant-in-the-method-signature.md)｜**草案**：Status 为 Proposed，尚非依据
- [ADR-0072：接入渠道与凭据验证归共享接入身份能力，登记册形状等真渠道证据](./0072-access-channel-capability-is-shared-and-registry-shape-awaits-channel-evidence.md)｜**部分停用**：其 Decision 2「登记册表结构与凭据形态在 `PAR-INT-01` 最低证据（该租户渠道的现行流程）到位前不立」一句已由 [ADR-0100](./0100-operator-identity-is-a-product-owned-access-channel-family.md) 收窄——**适用场景**限客户接入渠道的登记行与凭据；管理台运营操作者渠道的登记册结构与凭据形态由产品定义、现在就立。Decision 1（能力归 `internal/accessidentity`）、Decision 3 与其余各条不变
- [ADR-0073：申报单元是持久化聚合，案件关联落在单元上且成立即定](./0073-declaration-unit-is-a-persisted-aggregate-holding-its-case.md)
- [ADR-0074：TF 载运对象与 VE 包裹是两个排队主体，TF 对象链分区键带口名段](./0074-tf-object-partitions-carry-a-port-segment-apart-from-ve-parcel-partitions.md)
- [ADR-0075：客户地址随判断请求在请求期携带过界，network-routing 不建读取端口](./0075-customer-address-is-carried-with-the-routing-request.md)
- [ADR-0076：运营追踪查阅走独立读口消费投影库，不复用客户视图端点；运营作用域是租户级、无客户维](./0076-operations-tracking-read-is-a-separate-endpoint-on-the-projection-store.md)
- [ADR-0077：主数据登记目录查阅沿运营读口通例——独立查询端点、每上下文自立租户级作用域、未配置即拒；空目录如实答空](./0077-master-data-catalogue-read-follows-the-operations-read-pattern.md)
- [ADR-0078：隔离环境运营查阅按装配注入放行——合成租户显式入参、缺省朝拦，写路径与客户查阅面维持未配置即拒](./0078-isolated-environment-operations-reads-admit-by-assembly-injection.md)｜**部分停用**：其 Decision 四中「按环境选择的只有装配点上查阅行的 Intake 一件事」一句已由 [ADR-0091](./0091-isolated-form-extends-to-the-write-path-by-graded-switches.md) 停用——适用面由枚举改为入格判据（注入值全为 `SYN-` 合成、不采信自报身份、生产装配无此路径），写路径的命令面 Intake 与生产归属范围目录据此入格。同条另两句（不得据此再添 demo/mode 类全局开关或第二个 main；pgtest 环境处理通例不变）与其余各条不变。**适用场景**：仅限满足那三条判据的缝；`/customer-tracking-view` 的排除判据（客户维是调用方自己的身份主张）不因判据化而松动
- [ADR-0079：接受前控制策略视图凭商业解析回指提问——消费方只回显标识，提供方从已固定闭包取合同；坏回指是 error 不是未登记](./0079-pre-acceptance-control-policy-view-asks-by-commercial-resolution-reference.md)
- [ADR-0080：引用闭包先解合同再据以解结算政策——合同维是结论不是输入，前提未解析自成一格](./0080-commercial-closure-resolves-the-contract-first-and-keys-settlement-by-it.md)
- [ADR-0081：接受判断由信封驱动——提交落库即交出「委托已提交」，推进落在派发一拍；提交事务随之改两段边界](./0081-acceptance-judgment-is-envelope-driven.md)
- [ADR-0082：代收分户账按四维立键、余额只由追加式记账派生，回汇批次形成即冻结——分配守恒由记账形状交付，不靠事后对平](./0082-collection-subledger-is-keyed-by-four-dimensions-and-posted-append-only.md)
- [ADR-0083：试点治理读面按登记册实有维度成形——无租户维是设计；隔离读放行沿用同一开关，注入不带租户的产品级作用域；呈现面留在管理台并明示实例级作用域](./0083-pilot-governance-read-face-carries-registry-dimensions-only.md)
- [ADR-0084：面单交易是独立聚合——建立即固定覆盖与依据，双层结果一次记录不许互推，定案是派生谓词；继续尝试决定单列登记册不进聚合](./0084-label-transaction-is-an-independent-aggregate-with-two-level-results.md)
- [ADR-0085：登记册配置写面进端点表带未配置格——写表单属产品能力，CLI 保留为受控批量口；写准入不另立形，与其余命令面同等真渠道证据](./0085-registry-write-faces-enter-the-endpoint-table-with-unconfigured-grade.md)｜**部分停用**：其 Decision 二补记「两族各自要过 `PAR-INT-01` 的真证据门」一句的前半，与 Decision 二「本记录新增的登记端点也在被拦之列」一句，已由 [ADR-0100](./0100-operator-identity-is-a-product-owned-access-channel-family.md) 停用——**适用场景**限管理台操作者族与登记册配置写面；「不得互相顶替」后半保留，客户业务面照旧过 `PAR-INT-01` 并被 ADR-0055 Decision 五两项拦着。Decision 三原句不变，票 01 评论对它「翻译属渠道接入契约」的引申由 [ADR-0101](./0101-operator-facing-registration-payload-shape-is-product-defined.md) 收窄为客户渠道载荷。其余各条不变
- [ADR-0086：等待人工复核是入账暂停——暂停与等待态同事务落库，续办由「复核已完成」信封另行驱动；细分 ADR-0081 的未决语义，重投留给会自己回来的依赖](./0086-manual-review-wait-is-a-committed-pause-resumed-by-completion-envelope.md)
- [ADR-0087：结算登记册补齐硬句所要求的可核对事实——确认费用固定八项且收付方向自立词表，客户费用调整单列追加册（含唯一创建用例门），经营组成项带按口径分组的角色维](./0087-settlement-registers-carry-the-facts-their-hard-sentences-require-checking.md)
- [ADR-0088：面单渠道服务进入首发对客服务形态——`PAR-COM-12` 由范围裁剪改为纳入，首发形态成为网络服务与面单渠道服务的明确组合；领域语言一字不改，「一条线路」约束只作用于网络服务主链路](./0088-label-channel-service-enters-the-first-release-service-forms.md)
- [ADR-0089：一线作业过渡走受控批量导入并内置结构性拆除期限——ADR-0021 的带期限例外，管理台一个作业动词都不加；导入事实带来源标记以与设备事实可分辨](./0089-frontline-transition-controlled-import-with-structural-sunset.md)｜**带期限例外**：它不停用 [ADR-0021](./0021-frontline-operations-client-is-part-of-the-product.md) 与 [ADR-0023](./0023-work-fact-identity-and-time-are-minted-by-the-device.md) 任何一条，只在一线作业客户端开工之前开一个到 `2026-12-31T23:59:59+08:00` 为止的口子；两记录各条不变，延长期限须 supersede ADR-0089。**适用场景**：仅限过渡期受控批量导入这一条路径灌入的作业事实，标记为例外的印记；经设备签发的作业事实仍全数照 ADR-0023 办
- [ADR-0090：出向集成的结果代数按调用方的恢复动作分格，「有没有形成答案」由适配器判定且不得看 HTTP 状态码；未配置即拒，不设重试默认值](./0090-outbound-integration-result-algebra-partitioned-by-recovery-action.md)
- [ADR-0091：隔离形态从查阅面扩到写路径，按分级开关放行——入格判据取代枚举；写面有持久化，可分辨物由 `SYN-` 前缀承担](./0091-isolated-form-extends-to-the-write-path-by-graded-switches.md)
- [ADR-0092：渠道面单载荷在领域与库里只落引用，本体存放是一条未配置的出向缝；载荷记录追加不可覆盖](./0092-channel-label-payload-lands-as-a-reference-and-the-body-store-is-an-unconfigured-outbound-seam.md)｜**摘要与定位符不对称**：摘要必备（证明收到过这份件）、定位符可缺（回答此刻在哪），合成一格会让「收到了但没处放」表达不出来，而在文件组件就位前那是唯一走得到的分支。**适用场景**：渠道交回的面单件；本体存放端口未配置期间 `BodyStored` 恒为假，那是真话不是默认值
- [ADR-0093：渠道账号使用授权不是商业版本，走自己的修订式登记册；撤销是状态取值、自然到期由区间导出，撤销不回溯](./0093-channel-account-use-authorization-is-not-a-commercial-version.md)｜**归族判据**：判一个对象进不进 `CommercialObjectKind`，看它的生命周期是不是版本演进（有没有「新版本取代旧版本、旧版本留在册上」），持不持有 `CommercialVersion` 只是这件事在代码里的表现，不是判据本身。**适用场景**：`party-commercial` 新增对象的归族；票 04 的客户服务规则版本须按同一判据重问一次，且很可能得出相反答案
- [ADR-0094：未决重投与否由领域的 `ResumePath` 决定，并增设「等运营登记」第四格](./0094-undecided-retry-is-decided-by-resume-path-with-a-fourth-grade-for-operator-registration.md)｜**分格判据**：看恢复动作，不看缺了什么——重试、客户补件、人工复核都推不动、只有一次登记动作推得动的，是新的第四格。**适用场景**：接受判断链的未决处置；新增未决原因时先答它的恢复动作，消费门不再持有原因字面量
- [ADR-0095：未决的停站与原因分两层出声——不自愈那格靠入账留痕，自愈那格靠装配方注入的失败观察口](./0095-undecided-stage-and-reason-surface-in-two-layers.md)｜**两层对应**：会自愈的进日志、不自愈的进库，是按恢复动作分格那条判据在可观测性上的投影。**适用场景**：派发逐条失败的观察；本记录只补观察不补告警，「未决多久算异常」属实例半边
- [ADR-0096：实际履约段的身份由控制事实登记方显式声明，不从承运商、计划段或交接范围推导；未声明即段不成立且不铸默认号](./0096-a-fulfillment-segment-identity-is-declared-not-derived.md)｜**不推导判据**：一个成立时可能缺席的值（承运商可待确认、计划段可无）当不了身份；而操作组织单位（交接范围、揽收任务）与「共同控制责任范围」重合与否是租户的运营事实，隐式等同会静默合并两个控制范围。**适用场景**：接控制事实到段的编排；「两次控制事实何时算同一个段」的粒度政策属实例半边，留空拒默认
- [ADR-0097：段成立之后按动作分三个窄写口（加入 / 逐对象离场 / 关段），不开通用 Update](./0097-segment-evolution-writes-through-narrow-doors-not-a-general-update.md)｜**结构判据**：窄口的价值不在窄，在于「回写为未发生」「整段覆盖成员差异」在这个口上**表达不出来**；整段重写口挡住倒退靠的是调用方纪律，且并发下会静默丢成员。**适用场景**：逐对象演进的聚合怎么开写口；`ErrSegmentStillActive` 那条跨行不变量仍靠纪律不靠结构，已在记录里如实标出
- [ADR-0098：失败尝试费发生项不在揽收编排里形成，采购上下文显式给出而不推导；自营揽收失败不形成发生项是正确答案不是缺席](./0098-a-failed-attempt-charge-occurrence-is-not-formed-inside-the-pickup-orchestration.md)｜**位置判据**：CONTEXT 禁「任一对象的成功、失败或取消被压缩成其他对象的状态」，而把派生塞进一个必须照常成功的登记编排，会造出静默不发生且无人重试的一步（同 ADR-0094 那一类）。**适用场景**：一个派生事实该在哪个编排里形成；「不适用」与「未决」「报错」三格恢复动作不同必须分开；揽收↔委托的连线显式留空待租户实证
- [ADR-0099：价卡绑定序列标识而不是序列版本；在用序列版本按评价形成时刻从复核记录派生，解析结果冻结进评价的版本清单](./0099-price-card-binds-series-identity-and-in-force-version-is-derived-from-review.md)｜**两个时点判据**：计价基准时点决定期次，评价形成时刻决定版本——合成一个日期就是 SAP 式用当前表回看，重放随之失效。**适用场景**：方案对参考序列的引用与评价用例的解析；序列版本的复核是追加记录不是状态列，在用版本是派生结论不是可维护指针
- [ADR-0100：管理台运营操作者身份是产品自有的接入渠道族，属机制半边——信任锚、校验方式与操作者—租户授权模型由产品定义，不等 `PAR-INT-01`；登记册配置写面的真 Intake 据此现在就立](./0100-operator-identity-is-a-product-owned-access-channel-family.md)｜**归族判据**：一族身份的证据由谁出，看它的三件（信任锚、校验方式、授权模型）取决于谁——取决于产品的就是机制半边；`PAR-INT-01` 最低证据列里的每一件都是客户渠道的属性，容不下操作者。**适用场景**：管理台登记册配置写面与目录查阅面的真 Intake，答复按恢复动作分三格（发行方未配置 / 令牌无效 / 无授予）；客户业务命令面照旧过 `PAR-INT-01` 并被 ADR-0055 Decision 五两项拦着，隔离形态的 `SYN-` 写开关与之是装配点上的两行
- [ADR-0101：运营操作者面的登记载荷形状由产品定义，属机制半边——「渠道原始载荷 → 登记快照」的翻译只在客户渠道上属渠道契约；价卡首例：版本化导入模板、持久化草稿、校验与发布共用一份摘要、批准是独立操作者动作、审批职责规则缺省朝拦](./0101-operator-facing-registration-payload-shape-is-product-defined.md)｜**形状归属判据**：载荷形状归它唯一的来源——客户渠道的报文是客户系统的既成事实，操作者面的载荷只有产品这一个来源，产品不定义就没人定义。**适用场景**：管理台各登记签的形态由实施票按「登记频次 × 操作者角色 × 载荷结构」逐册裁；价卡首例走模板导入 → 草稿 → 批准 → 交既有 `RegisterPriceCard` 发布，CONTEXT 生命周期的草稿、已校验、已批准、已发布由此各有载体；蓝图「价卡导入与治理」节其余门禁按计价规则模型最终设计的范围判据另裁
- [ADR-0102：外部事实的三个时间按归属分铸——接收时间由本仓铸，发生时间只能由源给且缺则不收编，有效时间由所有者显式判断；这是 ADR-0023 在外部源上的适用解释，此后每一个外部源都继承它](./0102-external-fact-three-times-are-minted-by-ownership.md)｜**归属判据**：一个时间是谁的事实就由谁铸——接收时间是本仓的、发生时间是源的、有效时间是所有者的一次判断；「不代铸」只禁替别人铸，不禁铸自己的。**适用场景**：任何上下文收编任何外部报文；`OccurredAt` 缺失即不收编且无例外，`EffectiveAt` 不得默默等于 `OccurredAt`，无规则时留为「待判断」不提供给投影；状态词在收编这一步不解释，外部承运轨迹事实因此在状态词规则到位前只能未归类

## 已被取代决策

被取代的记录保留正文不改写，只作历史；现行依据以取代它的记录为准。

- [ADR-0011：在 idp-parcel 内建立小包计价上下文](./0011-parcel-pricing-context-within-idp-parcel.md) → 由 [ADR-0012](./0012-parcel-pricing-context-within-idp-parcel.md) 取代：同一边界决策改写为不依赖外部参考设计目录的自洽形式。
- [ADR-0018：产品内客户端应用与后端共享发布边界并落在顶层 `apps/`](./0018-product-clients-share-release-boundary-under-apps.md) → 由 [ADR-0019](./0019-product-ships-tenant-facing-operator-clients.md) 取代：落位结论有效但立在了低一层，改写为先决定产品是否自带界面，并据此排除跨租户运维后台。
- [ADR-0019：产品自带面向租户的作业与治理界面，跨租户运维后台不属于产品](./0019-product-ships-tenant-facing-operator-clients.md) → 由 [ADR-0020](./0020-tenant-admin-client-in-product-workers-are-processes.md) 取代：`worker` 被误读为一线作业人员，据此写入的「自带一线作业面」不成立；更正为 worker 即后台进程、落 `cmd/`，一线作业界面退回未决。
- [ADR-0020：产品自带租户管理界面；后台 worker 是进程入口，一线作业界面未决](./0020-tenant-admin-client-in-product-workers-are-processes.md) → 由 [ADR-0021](./0021-frontline-operations-client-is-part-of-the-product.md) 取代：所留未决项已按能力范围判据与行业既有做法作出判断，一线作业客户端确认属于产品并划定范围。
