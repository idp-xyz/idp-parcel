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

## 已被取代决策

被取代的记录保留正文不改写，只作历史；现行依据以取代它的记录为准。

- [ADR-0011：在 idp-parcel 内建立小包计价上下文](./0011-parcel-pricing-context-within-idp-parcel.md) → 由 [ADR-0012](./0012-parcel-pricing-context-within-idp-parcel.md) 取代：同一边界决策改写为不依赖外部参考设计目录的自洽形式。
