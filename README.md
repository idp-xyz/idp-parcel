# idp-parcel

IDP Parcel 是独立的国际小包网络运营系统代码与文档仓库，采用 Go 单仓库、模块化单体和显式领域边界。

当前仓库处于产品与代码基线建立阶段：产品边界、领域模型、PN-02 至 PN-08 的产品级业务/治理交接已经形成；`PN-02` 已建立不依赖真实参数的建单前领域内核，用于验证作用域身份、来源重放/冲突、最小委托候选和同租户提交批次边界。真实试点参数、可持久化业务切片、PN-08 治理实现、真实 `idp-bento-go` 候选依赖和消费者证明仍未创建。

## 基线

- Go module：`go.idp.xyz/idp-parcel`
- 工具链：Go `1.26.5`
- 数据库：PostgreSQL major 16
- 持久化：`pgx/v5` + 显式 SQL
- 共享技术框架：`go.idp.xyz/idp-bento-go` 精确不可变候选；当前未绑定
- 客户端应用：顶层 `apps/`（每端一个子目录，见 ADR-0018/0021）；`apps/admin-web` 租户管理台需要 Node 20+ 与 pnpm 10+，安装依赖见其 [README](./apps/admin-web/README.md)

## 工作方式

- [给人与 Agent 的开工入口（AGENTS.md）](./AGENTS.md)：权威阅读顺序、编码红线、改文档规则与 skills 路由

## 文档入口

- [产品与领域文档](./docs/README.md)
- [国际小包网络运营首发产品基线与开发主线](./docs/product/PARCEL-NETWORK-FIRST-RELEASE-DEVELOPMENT-BASELINE.md)
- [领域上下文地图](./docs/domain/CONTEXT-MAP.md)
- [`PN-08` 端到端试点与阶段准入开发交接](./docs/design/pn-08-end-to-end-pilot-and-stage-admission-development-handoff.md)
- [产品主线下的 Go 首个消费者技术切片实施决策简报](./docs/design/parcel-go-first-consumer-slice-decision-brief.md)
- [架构决策记录](./docs/adr/README.md)
