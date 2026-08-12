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
- [ADR-0022：HTTP 状态码只回答「有没有形成答案」，业务判别一律进响应体](./0022-http-status-carries-answer-formed-not-business-verdict.md)
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

## 已被取代决策

被取代的记录保留正文不改写，只作历史；现行依据以取代它的记录为准。

- [ADR-0011：在 idp-parcel 内建立小包计价上下文](./0011-parcel-pricing-context-within-idp-parcel.md) → 由 [ADR-0012](./0012-parcel-pricing-context-within-idp-parcel.md) 取代：同一边界决策改写为不依赖外部参考设计目录的自洽形式。
- [ADR-0018：产品内客户端应用与后端共享发布边界并落在顶层 `apps/`](./0018-product-clients-share-release-boundary-under-apps.md) → 由 [ADR-0019](./0019-product-ships-tenant-facing-operator-clients.md) 取代：落位结论有效但立在了低一层，改写为先决定产品是否自带界面，并据此排除跨租户运维后台。
- [ADR-0019：产品自带面向租户的作业与治理界面，跨租户运维后台不属于产品](./0019-product-ships-tenant-facing-operator-clients.md) → 由 [ADR-0020](./0020-tenant-admin-client-in-product-workers-are-processes.md) 取代：`worker` 被误读为一线作业人员，据此写入的「自带一线作业面」不成立；更正为 worker 即后台进程、落 `cmd/`，一线作业界面退回未决。
- [ADR-0020：产品自带租户管理界面；后台 worker 是进程入口，一线作业界面未决](./0020-tenant-admin-client-in-product-workers-are-processes.md) → 由 [ADR-0021](./0021-frontline-operations-client-is-part-of-the-product.md) 取代：所留未决项已按能力范围判据与行业既有做法作出判断，一线作业客户端确认属于产品并划定范围。
