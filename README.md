# idp-parcel

IDP Parcel 是独立的国际小包网络运营系统代码与文档仓库，采用 Go 单仓库、模块化单体和显式领域边界。

当前仓库处于代码基线建立阶段：产品、领域和首个消费者切片设计已经确认，业务实现、真实 `idp-bento-go` 候选依赖和消费者证明尚未创建。

## 基线

- Go module：`go.idp.xyz/idp-parcel`
- 工具链：Go `1.26.5`
- 数据库：PostgreSQL major 16
- 持久化：`pgx/v5` + 显式 SQL
- 共享技术框架：`go.idp.xyz/idp-bento-go` 精确不可变候选；当前未绑定

## 文档入口

- [产品与领域文档](./docs/README.md)
- [领域上下文地图](./docs/domain/CONTEXT-MAP.md)
- [Go 首个消费者切片实施决策简报](./docs/design/parcel-go-first-consumer-slice-decision-brief.md)
- [架构决策记录](./docs/adr/README.md)
