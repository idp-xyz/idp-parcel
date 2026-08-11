# 架构决策记录

本目录记录国际小包网络运营系统中难以逆转、存在真实替代方案且无法仅从实现理解的持久架构决策。

## 已接受决策

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
- [ADR-0025：跨上下文调用的适配器落在消费侧，翻译职责由它独占](./0025-cross-context-adapters-live-on-the-consumer-side.md)

## 已被取代决策

被取代的记录保留正文不改写，只作历史；现行依据以取代它的记录为准。

- [ADR-0011：在 idp-parcel 内建立小包计价上下文](./0011-parcel-pricing-context-within-idp-parcel.md) → 由 [ADR-0012](./0012-parcel-pricing-context-within-idp-parcel.md) 取代：同一边界决策改写为不依赖外部参考设计目录的自洽形式。
- [ADR-0018：产品内客户端应用与后端共享发布边界并落在顶层 `apps/`](./0018-product-clients-share-release-boundary-under-apps.md) → 由 [ADR-0019](./0019-product-ships-tenant-facing-operator-clients.md) 取代：落位结论有效但立在了低一层，改写为先决定产品是否自带界面，并据此排除跨租户运维后台。
- [ADR-0019：产品自带面向租户的作业与治理界面，跨租户运维后台不属于产品](./0019-product-ships-tenant-facing-operator-clients.md) → 由 [ADR-0020](./0020-tenant-admin-client-in-product-workers-are-processes.md) 取代：`worker` 被误读为一线作业人员，据此写入的「自带一线作业面」不成立；更正为 worker 即后台进程、落 `cmd/`，一线作业界面退回未决。
- [ADR-0020：产品自带租户管理界面；后台 worker 是进程入口，一线作业界面未决](./0020-tenant-admin-client-in-product-workers-are-processes.md) → 由 [ADR-0021](./0021-frontline-operations-client-is-part-of-the-product.md) 取代：所留未决项已按能力范围判据与行业既有做法作出判断，一线作业客户端确认属于产品并划定范围。
