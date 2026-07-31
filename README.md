# idp-parcel

IDP Parcel 是独立的国际小包网络运营系统代码与文档仓库，采用 Go 单仓库、模块化单体和显式领域边界。

当前仓库处于首个切片实施阶段：`PS-W1-S1 来源保全并提交委托` 已经实现，接受、拒绝、接受基线和预计承诺尚未实现，`BD-PS-001` 至 `BD-PS-008` 继续保持未决。

## 基线

- Go module：`go.idp.xyz/idp-parcel`
- 工具链：Go `1.26.5`
- HTTP 入站：`chi/v5 v5.3.1`
- 数据库：PostgreSQL major 16
- 持久化：`pgx/v5 v5.10.0` + 显式 SQL
- 迁移：`tern/v2 v2.4.1`
- 共享技术框架：`go.idp.xyz/idp-bento-go v0.1.0-rc.1`（精确不可变候选，无 `replace`）

## 代码结构

```text
cmd/parcel-api/                           HTTP API 进程
cmd/parcel-migrate/                       显式迁移命令
internal/parcelshipment/domain/           委托、声明包裹、提交版本与来源记录
internal/parcelshipment/application/      PS-W1-S1 用例编排
internal/parcelshipment/ports/            语义化 Repository 端口
internal/parcelshipment/adapters/postgres/  显式 SQL、扫描与行模型
internal/platform/migrate/                框架 + 业务迁移计划与历史
internal/platform/pgtest/                 集成测试数据库供给
internal/architecture/                    模块依赖门禁
tests/bentocontract/                      Bento 候选消费者合同（仅测试依赖）
```

## PS-W1-S1 边界

已实现：

- 原始内容、摘要、来源发生时间和系统接收时间先形成不可变来源记录，后续失败不删除它。
- 无法建立最小身份时记录`输入未受理`，不创建占位委托，也不写成委托拒绝。
- 每份委托在独立事务中创建`已提交`、当前提交版本和声明包裹，并在同一事务写入“委托已提交”Outbox 信封。
- 同键同摘要返回原委托和原 EventID；同键不同摘要返回接入冲突且不覆盖原内容；不同租户或客户账户即使外部键相同也隔离。
- 一份委托失败不回滚同批其他已经合法形成的`已提交`事实。

未实现且不得被伪装为已实现：

- `已接受`、`已拒绝`、接受基线和预计承诺。数据库约束限制 `lifecycle_state` 只能为 `SUBMITTED`。
- 商业资格、可达性、信用、余额或资金冻结结果。
- `BD-PS-001` 至 `BD-PS-008` 的任何推荐默认值。

## 数据库

迁移由 `parcel-migrate` 显式执行，应用进程启动只做只读 schema 检查。

```bash
go run ./cmd/parcel-migrate -plan
IDP_PARCEL_POSTGRES_DSN=... go run ./cmd/parcel-migrate
```

## 本地门禁

```bash
gofmt -l .
go vet ./...
IDP_PARCEL_POSTGRES_DSN=postgres://user:pass@127.0.0.1:5432/postgres?sslmode=disable \
  go test -race -count=1 ./...
go build ./...
```

未设置 `IDP_PARCEL_POSTGRES_DSN` 时 PostgreSQL 集成门禁在本地跳过；`CI` 环境下缺少该变量直接失败。

## 文档入口

- [产品与领域文档](./docs/README.md)
- [领域上下文地图](./docs/domain/CONTEXT-MAP.md)
- [Go 首个消费者切片实施决策简报](./docs/design/parcel-go-first-consumer-slice-decision-brief.md)
- [架构决策记录](./docs/adr/README.md)
