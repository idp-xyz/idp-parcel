# ADR-0009: 采用 Go 模块化单体并复用版本化 Bento 技术合同

Status: Accepted
Date: 2026-07-31

## Context

`idp-parcel` 已被确认是独立产品，拥有独立领域模型、业务数据库、运行和发布边界。项目目前只有文档，需要建立首个代码基线，并以 `UC-PS-001` 的真实业务骨架验证事务、Repository、事件发布和消费者兼容性。

直接按限界上下文拆分微服务会在团队只有一人、业务参数仍待绑定时提前引入网络一致性、部署和跨服务版本成本。复用 TMS 领域模型或运行时会违反 Parcel 的自治边界。完全自建事务、Outbox、Inbox 和兼容性治理则会重复 `idp-bento-go` 已由 TMS 与 Parcel 共同需要的业务无关能力。

## Decision

- Parcel 主后端使用 Go，首版采用独立单仓库、单 `go.mod` 和模块化单体。
- 限界上下文映射为代码所有权边界，但不默认映射为微服务、独立数据库或独立发布单元；模块不能直接修改其他模块拥有的数据。
- Parcel 使用独立 PostgreSQL 业务数据库，持久化采用 `pgx/v5` 和显式 SQL，不引入通用 ORM。
- Parcel 通过正式 module path 和精确 SemVer 候选复用 `go.idp.xyz/idp-bento-go` 的业务无关事务、Repository 最小合同、Outbox/Inbox、迁移和测试原语。
- Bento 不拥有 Parcel 的委托、包裹、合同、路由、履约、关务或结算语义，也不能成为跨产品共享业务表或运行时业务服务的通道。
- 首个真实消费者切片使用 `UC-PS-001` 已确认的“来源保全并进入已提交”骨架；未确认业务参数不进入生产默认值。

## Consequences

- 一人团队可以在单一代码和部署边界内完成真实纵向切片，同时保留未来按证据拆分服务的路径。
- Parcel 与 TMS 可以共享经过双消费者证明的技术合同，但保持领域、数据、部署和发布自治。
- 显式 SQL、模块边界和消费者合同需要更多样板与集成测试，不能依赖 ORM 或共享数据库隐藏边界。
- 框架升级必须经过 Parcel 自有依赖升级和完整 CI；Bento 发布不自动改变 Parcel 生产依赖。
- 在真实不可变 Bento 候选、读取身份和消费者证明就绪前，可以建立本地合同脚手架，但不能声明形成稳定发布基线。

## Alternatives considered

- **按限界上下文从首版拆分微服务**：部署隔离明确，但当前没有负载、团队或发布节奏证据支撑其一致性与运维成本。
- **复用 TMS 领域模型、数据库或运行时**：减少初期代码，但直接违反 ADR-0001 和 ADR-0002 的产品自治边界。
- **采用 Python `idp-bento` 作为运行时基础**：可以参考其 DDD/EDA 经验，但会引入不同语言运行时，并不能形成 TMS 与 Parcel 的共同 Go 技术合同。
- **Parcel 完全自建事务、Outbox、Inbox 和兼容性工具**：局部控制最大，但重复高风险基础设施，且失去双消费者验证的公司级技术基线。
- **使用通用 ORM 和通用 CRUD Repository**：早期开发较快，但容易隐藏租户条件、事务参与和聚合边界，不适合显式数据隔离与审计要求。

## Links

- [ADR-0001：国际小包采用自治产品与领域边界](./0001-autonomous-product-domain-boundary.md)
- [ADR-0002：国际小包采用独立数据、运行与发布边界](./0002-independent-data-runtime-release-boundary.md)
- [领域上下文地图](../domain/CONTEXT-MAP.md)
- [小包托运上下文](../domain/parcel-shipment/CONTEXT.md)
- [Parcel Go 首个消费者切片实施决策简报](../design/parcel-go-first-consumer-slice-decision-brief.md)
