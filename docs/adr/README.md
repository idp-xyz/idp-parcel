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
- [ADR-0072：接入渠道与凭据验证归共享接入身份能力，登记册形状等真渠道证据](./0072-access-channel-capability-is-shared-and-registry-shape-awaits-channel-evidence.md)
- [ADR-0073：申报单元是持久化聚合，案件关联落在单元上且成立即定](./0073-declaration-unit-is-a-persisted-aggregate-holding-its-case.md)
- [ADR-0074：TF 载运对象与 VE 包裹是两个排队主体，TF 对象链分区键带口名段](./0074-tf-object-partitions-carry-a-port-segment-apart-from-ve-parcel-partitions.md)
- [ADR-0075：客户地址随判断请求在请求期携带过界，network-routing 不建读取端口](./0075-customer-address-is-carried-with-the-routing-request.md)
- [ADR-0076：运营追踪查阅走独立读口消费投影库，不复用客户视图端点；运营作用域是租户级、无客户维](./0076-operations-tracking-read-is-a-separate-endpoint-on-the-projection-store.md)
- [ADR-0077：主数据登记目录查阅沿运营读口通例——独立查询端点、每上下文自立租户级作用域、未配置即拒；空目录如实答空](./0077-master-data-catalogue-read-follows-the-operations-read-pattern.md)
- [ADR-0078：隔离环境运营查阅按装配注入放行——合成租户显式入参、缺省朝拦，写路径与客户查阅面维持未配置即拒](./0078-isolated-environment-operations-reads-admit-by-assembly-injection.md)
- [ADR-0079：接受前控制策略视图凭商业解析回指提问——消费方只回显标识，提供方从已固定闭包取合同；坏回指是 error 不是未登记](./0079-pre-acceptance-control-policy-view-asks-by-commercial-resolution-reference.md)
- [ADR-0080：引用闭包先解合同再据以解结算政策——合同维是结论不是输入，前提未解析自成一格](./0080-commercial-closure-resolves-the-contract-first-and-keys-settlement-by-it.md)
- [ADR-0081：接受判断由信封驱动——提交落库即交出「委托已提交」，推进落在派发一拍；提交事务随之改两段边界](./0081-acceptance-judgment-is-envelope-driven.md)
- [ADR-0082：代收分户账按四维立键、余额只由追加式记账派生，回汇批次形成即冻结——分配守恒由记账形状交付，不靠事后对平](./0082-collection-subledger-is-keyed-by-four-dimensions-and-posted-append-only.md)
- [ADR-0083：试点治理读面按登记册实有维度成形——无租户维是设计；隔离读放行沿用同一开关，注入不带租户的产品级作用域；呈现面留在管理台并明示实例级作用域](./0083-pilot-governance-read-face-carries-registry-dimensions-only.md)
- [ADR-0085：登记册配置写面进端点表带未配置格——写表单属产品能力，CLI 保留为受控批量口；写准入不另立形，与其余命令面同等真渠道证据](./0085-registry-write-faces-enter-the-endpoint-table-with-unconfigured-grade.md)

## 已被取代决策

被取代的记录保留正文不改写，只作历史；现行依据以取代它的记录为准。

- [ADR-0011：在 idp-parcel 内建立小包计价上下文](./0011-parcel-pricing-context-within-idp-parcel.md) → 由 [ADR-0012](./0012-parcel-pricing-context-within-idp-parcel.md) 取代：同一边界决策改写为不依赖外部参考设计目录的自洽形式。
- [ADR-0018：产品内客户端应用与后端共享发布边界并落在顶层 `apps/`](./0018-product-clients-share-release-boundary-under-apps.md) → 由 [ADR-0019](./0019-product-ships-tenant-facing-operator-clients.md) 取代：落位结论有效但立在了低一层，改写为先决定产品是否自带界面，并据此排除跨租户运维后台。
- [ADR-0019：产品自带面向租户的作业与治理界面，跨租户运维后台不属于产品](./0019-product-ships-tenant-facing-operator-clients.md) → 由 [ADR-0020](./0020-tenant-admin-client-in-product-workers-are-processes.md) 取代：`worker` 被误读为一线作业人员，据此写入的「自带一线作业面」不成立；更正为 worker 即后台进程、落 `cmd/`，一线作业界面退回未决。
- [ADR-0020：产品自带租户管理界面；后台 worker 是进程入口，一线作业界面未决](./0020-tenant-admin-client-in-product-workers-are-processes.md) → 由 [ADR-0021](./0021-frontline-operations-client-is-part-of-the-product.md) 取代：所留未决项已按能力范围判据与行业既有做法作出判断，一线作业客户端确认属于产品并划定范围。
