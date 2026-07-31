# IDP Parcel Go 首个消费者切片实施决策简报

Status: Confirmed

## 目标

把 `idp-parcel` 的技术代码基线和首个 `idp-bento-go` 真实消费者切片固定到可以直接建立仓库、包结构、迁移和合同测试的程度，同时严格保留 [`UC-PS-001`](../application/parcel-shipment/UC-PS-001-SUBMIT-SHIPMENT-REQUEST.md) 中尚未确认的业务参数。

首个切片只证明以下闭环：

- 原始提交及来源语义先被不可变保全。
- 一份已建立最小身份的委托进入`已提交`，但不冒充已经接受或拒绝。
- 委托、声明包裹、提交版本和“委托已提交”事件发布意图原子提交。
- 重复、同键冲突、事务回滚、提交结果不确定和 Outbox 重试有明确行为。
- Parcel 的真实 Repository、事务、迁移和 Outbox 使用同一不可变 Bento 候选运行消费者合同。

本文不补全真实客户、合同、线路、字段、金额、阈值、角色、超时、重试次数或 SLA，也不宣布生产接单已经完成。

## 证据边界

- `idp-parcel` 是独立产品，拥有独立领域模型、业务数据库、运行和发布边界，见 [ADR-0001](../adr/0001-autonomous-product-domain-boundary.md) 和 [ADR-0002](../adr/0002-independent-data-runtime-release-boundary.md)。
- `parcel-shipment` 已确认拥有提交批次、委托、声明包裹、接受决定和接受基线；客户提交与委托接受是两个业务事件。
- `UC-PS-001` 已确认“委托已提交”的业务语义，以及业务状态、决定记录和事件发布意图的原子提交要求。
- `BD-PS-001` 至 `BD-PS-008` 当前仍待业务确认。它们不阻塞提交骨架和技术合同，但阻塞真实接受、拒绝、资金/信用、试点归属及生产规则。
- `idp-bento-go` 的正式 module path、Git 远端、DNS、TLS 和 vanity metadata 已建立。首个不可变候选 `v0.1.0-rc.1` 已创建，指向框架提交 `af68525`，并已通过空缓存、无 `replace`、`GOPROXY=direct` 的下载证明。
- Parcel 已建立首个提交并绑定远端 `https://github.com/idp-xyz/idp-parcel`（private）。仓库使用 `go.idp.xyz/idp-parcel` 单 module、Go `1.26.5`、`chi/v5 v5.3.1`、`pgx/v5 v5.10.0`、`tern/v2 v2.4.1`，并锁定 Bento `v0.1.0-rc.1`。
- `PS-W1-S1 来源保全并提交委托` 已实现，PostgreSQL 16 集成门禁与 `PBC-01`、`PBC-05`、`PBC-06`、`PBC-08` 通过。`PBC-09` 的证明 JSON、WORM/DSSE 证据和受保护 CI 仍未建立。

## 已确认决定

| 编号 | 决定 | 状态 |
|---|---|---|
| `P-01` | Parcel 使用独立 Git 仓库、单仓库、模块化单体和单 `go.mod`；限界上下文不因此共享领域模型或数据库表 | `CONFIRMED` |
| `P-02` | 正式 module path 为 `go.idp.xyz/idp-parcel`；实际 Git 远端和 vanity 路由在建仓后单独绑定 | `CONFIRMED` |
| `P-03` | 首个代码基线锁定 Go `1.26.5`；后续升级必须显式变更并通过完整回归 | `CONFIRMED` |
| `P-04` | HTTP 入站适配器使用 `chi/v5 v5.3.1`，领域与应用代码不依赖 HTTP 类型 | `CONFIRMED` |
| `P-05` | 业务权威存储使用 PostgreSQL major 16、`pgx/v5` 和显式 SQL，不引入 GORM 或其他通用 ORM | `CONFIRMED` |
| `P-06` | 数据库迁移命令使用 `tern/v2 v2.4.1`；应用进程启动时只检查 schema，不自动执行或修复 DDL | `CONFIRMED` |
| `P-07` | 通过精确 SemVer 候选依赖 `go.idp.xyz/idp-bento-go`；生产 `go.mod` 禁止本地 `replace`，仓库不得复制框架源码 | `CONFIRMED` |
| `P-08` | Parcel 拥有语义化 Repository Port；Bento 的泛型 Repository 只提供最小合同，不提供表级 CRUD、Registry 或业务聚合 | `CONFIRMED` |
| `P-09` | 写 Repository 通过 Bento `postgres.DB.RequireExecutor(ctx)` 严格取得事务执行器；缺少事务立即失败，不静默回退连接池 | `CONFIRMED` |
| `P-10` | 业务写入与 `eventing.OutboxWriter.Enqueue` 在同一个 `application.Transactor` 事务 context 中执行 | `CONFIRMED` |
| `P-11` | Outbox 使用 At-Least-Once；Parcel 发布进程拥有运行生命周期、超时、退避和失败预算，消费方使用 Inbox 或等价业务幂等抑制重复副作用 | `CONFIRMED` |
| `P-12` | 首个真实切片为 `UC-PS-001` 的“来源保全并进入已提交”子切片，不形成接受、拒绝、预计承诺或财务控制结果 | `CONFIRMED` |
| `P-13` | Bento `testkit` 只能被 `_test.go` 和专用消费者合同包导入，任何生产包导入都由 CI 阻断 | `CONFIRMED` |
| `P-14` | 当前消费者与框架治理如实使用 `SOLO_BOOTSTRAP`；Bento 证明标记 `NO INDEPENDENT HUMAN APPROVAL` 和 `OWNER-BYPASSABLE` | `CONFIRMED` |

## 代码与模块边界

首个仓库采用以下方向；目录是所有权边界，不把每个目录部署为微服务：

```text
cmd/
  parcel-api/                 HTTP API 进程
  parcel-migrate/             显式迁移命令
  parcel-outbox/              Outbox 发布进程
internal/
  platform/
    bootstrap/                依赖装配、配置与生命周期
    postgres/                 连接池、Bento DB 与 schema 检查
    observability/            slog 与 OpenTelemetry SDK 装配
  parcelshipment/
    domain/                   委托、声明包裹、提交版本与领域事件
    application/              命令、结果和用例编排
    ports/                    语义化 Repository 与相邻上下文端口
    adapters/postgres/        显式 SQL、扫描和映射
    adapters/http/            chi 请求/响应映射
  partycommercial/            后续按真实切片建立，不预建空模型
  networkrouting/             后续按真实切片建立，不预建空模型
migrations/
  parcel_shipment/            Parcel 自有不可变业务迁移
tests/
  bento_contract/             仅测试依赖的真实消费者合同
```

约束如下：

- `internal/parcelshipment/domain` 只依赖标准库和必要的 Bento `domain` 事件合同，不依赖 `pgx`、HTTP、日志或环境配置。
- `application` 只依赖领域类型、消费者拥有的端口，以及 Bento `application` / `eventing` 最小合同。
- PostgreSQL 行模型、SQLSTATE 翻译和扫描留在适配器，不进入领域对象。
- `party-commercial`、`network-routing` 和 `settlement-accounting` 的判断通过应用端口取得；首个子切片不创建它们的空领域模型或伪造结果。
- 模块不能直接读取其他模块拥有的表。将来同进程调用也必须通过应用端口或已确认事件语义。

## Bento 使用边界

| Bento 包 | Parcel 使用方式 | 禁止方式 |
|---|---|---|
| `domain` | `Event`、`EventType`、`EventVersion`、`EventBuffer` | 让框架拥有委托、包裹、值对象或通用领域错误 |
| `application` | `Handler`、`Clock`、`IDGenerator`、`Transactor`、`Transactional` | Command Bus、Repository Registry、自动重试、嵌套新事务 |
| `repository` | 为 Parcel 强类型键和聚合运行 Loader/Inserter/VersionedUpdater 合同 | 暴露任意表 CRUD、隐式租户、跨聚合保存 |
| `eventing` | 版本化 Envelope、单条 Publisher、Outbox/Inbox 技术合同 | 开放 Metadata map、Exactly-Once 声明、从 context 补全租户 |
| `postgres` | `DB.RequireExecutor`、`Transactor`、`Migrations`、`RenderMigration`、`CheckSchema` | 业务包直接取得 `pgx.Tx`、应用启动自动迁移、静默连接池写入 |
| `testkit` | 确定性 Clock/ID、记录式 Publisher、真实 Repository/Outbox/Inbox 合同 | 生产依赖、通用内存 Repository、用假事务替代 PostgreSQL 验证 |

当前本地 Bento 工作副本和任意普通提交都不是消费者依赖基线。只有真实不可变候选存在后，Parcel 才在 `go.mod` 锁定该精确版本，并在隔离环境中证明 module checksum 与候选 manifest 一致。

## 首个真实子切片

### 业务范围

子切片名称：`PS-W1-S1 来源保全并提交委托`。

它实现 `UC-PS-001` 步骤 1、2、3A、3B 的最小闭环：

1. 接入适配器解析显式集团租户、货主客户账户、来源和来源请求键。
2. 原始内容、内容摘要、来源发生时间、系统接收时间和关联信息先形成不可变来源记录。
3. 无法建立客户范围、委托边界或最小成员身份时记录`输入未受理`，不创建占位委托。
4. 已建立最小身份时，每份委托在独立事务中创建`已提交`状态、当前提交版本和一个或多个声明包裹。
5. 同一事务写入 Parcel 业务记录和“委托已提交”Outbox 信封。
6. 返回`已提交 / 尚未决定`及稳定查询关联；不执行接受条件判断。

明确排除：

- `已接受`、`已拒绝`、接受基线和预计承诺。
- 商业资格、可达性、信用、余额或资金冻结结果。
- `BD-PS-001` 至 `BD-PS-008` 的任何推荐默认值。
- 面单交易、初始路由、关务建案、节点收寄、履约或结算。

### 输入与幂等

应用命令必须显式携带强类型作用域：

- `TenantID`
- `CustomerAccountID`
- `Source`
- `SourceRequestKey`
- `PayloadDigest`
- `SubmissionBatchID`
- `ShipmentRequestID`
- 一个或多个 `DeclaredParcelID`
- 来源发生时间与系统接收时间

`SourceRequestKey` 的真实组成由各接入适配器依据 `PAR-INT-01` 绑定，不能直接把客户参考号当作全局幂等键。Parcel 持久化唯一性至少覆盖租户、客户账户、来源和来源请求键：

- 同键、同摘要：返回已有来源记录和已有委托结果。
- 同键、不同摘要：返回接入冲突，保留原内容，不覆盖、不再建单。
- 不同租户或客户账户：即使外部键相同也必须隔离。
- 提交结果不确定：使用稳定请求关联查询原结果；只有已证明业务幂等时才能自动续办，不能盲目重放事务。

### 事务边界

来源保全和业务提交分为两个明确事务：

1. **来源保全事务**：保存原始内容或不可变引用、摘要及接入处理状态。后续解析或依赖失败不能删除该记录。
2. **委托提交事务**：创建提交批次关联、委托、当前提交版本和声明包裹，随后 Enqueue 同一事件信封；任一步失败全部回滚。

每份委托独立提交，提交批次不形成全批原子事务。一个委托失败不得回滚同批其他委托已经合法形成的`已提交`事实。

Repository 写入必须在 `Transactor.WithinTransaction` 派生的 context 中调用 `DB.RequireExecutor(ctx)`。Outbox Writer 使用同一 context；禁止 Repository 或 Outbox 在取不到事务时改用连接池。

### 事件信封基线

首个 Parcel 自有事件定义为：

| 字段 | 基线 |
|---|---|
| 业务语义 | 委托已提交 |
| `Type` | `idp.parcel.shipment-request.submitted` |
| `Version` | `1` |
| `Source` | `go.idp.xyz/idp-parcel/parcel-shipment` |
| `Scope` | 显式租户与货主客户账户复合标识 |
| `Subject` | 明确委托标识 |
| `PartitionKey` | 同一租户、客户账户和委托的稳定复合键 |
| Payload | 委托、提交批次、提交版本、声明包裹标识和必要关联；不含地址、联系人、货物或申报明文 |

EventID 在首次命令处理中生成并与业务结果一起保存。重复请求复用原 EventID，不创建语义相同的新事件。Envelope 的记录时间与领域发生时间分别填写，不能用当前时间覆盖来源发生时间。

## Repository Port

Parcel 不提供一个可以操作任意聚合的通用业务 Repository。首个子切片至少拥有：

- `SourceSubmissionRepository`：按显式来源作用域保存和查询不可变提交，处理同键同摘要与同键不同摘要。
- `ShipmentRequestRepository`：以包含 `TenantID`、`CustomerAccountID` 和 `ShipmentRequestID` 的强类型键加载、插入和版本化更新委托聚合。
- `SubmissionBatchRepository`：只拥有批次归组和逐委托结果引用，不取得委托服务责任。

这些语义端口可以组合 Bento `repository.Loader`、`Inserter` 和 `VersionedUpdater` 以运行公共合同，但 Parcel 自有方法仍使用领域语言。所有 SQL 必须显式包含租户和客户账户条件；`ErrNotFound` 不得泄露其他作用域是否存在对象。

## 迁移与 schema

- 首版使用一个 Parcel PostgreSQL 数据库；`parcel_shipment` schema 由该模块拥有，Bento 技术表使用独立 `bento` schema。
- `parcel-migrate` 按顺序执行 Parcel 业务迁移和锁定候选提供的 `Migrations()` / `RenderMigration` 结果，并记录迁移 ID、规范 checksum、框架版本和实际 schema。
- 生产 API 与 Outbox 账号不持有 DDL 权限。部署前显式运行迁移，进程启动调用只读 schema 检查。
- 业务迁移只使用显式 SQL。Repository 的 SQL、扫描和行模型必须由 Parcel 测试直接覆盖。

## 消费者合同

真实 `bento-contract` 至少包含：

| ID | 证明 |
|---|---|
| `PBC-01` | 从正式 module path 和精确候选版本编译，不使用 `replace`、`go.work` 或源码副本 |
| `PBC-02` | Parcel 强类型复合键 Repository 通过 Bento Repository 合同，覆盖插入、加载、版本冲突和作用域隔离 |
| `PBC-03` | 业务 Repository 与 Outbox 使用同一个事务；成功时二者同时可见，回滚时二者都不可见 |
| `PBC-04` | 同键同摘要返回原结果，同键不同摘要冲突，并发重复不能创建第二份委托或第二个 EventID |
| `PBC-05` | Envelope v1 校验、Payload 最小化、同委托分区顺序和 At-Least-Once 重投符合框架合同 |
| `PBC-06` | 框架迁移按 checksum 渲染，PostgreSQL 16 上 `CheckSchema`、Outbox 和 Inbox 合同通过 |
| `PBC-07` | `application.ErrCommitUncertain` 不触发无条件自动重放，调用方能够按稳定请求关联查询原结果 |
| `PBC-08` | 生产依赖图不包含 `testkit`，且没有业务包导入 `pgx.Tx` 或绕过 `DB.RequireExecutor` |
| `PBC-09` | 证明 JSON 绑定 Parcel 精确提交、候选版本、module checksum、合同版本和 `PASS`，并显式记录单人治理状态 |

`PBC-01` 和 `PBC-09` 在真实不可变 RC、私有读取身份和 Parcel Git 提交存在前只能建立脚手架，不能伪造为已经通过。其余合同可以先使用本地 workspace 开发，但本地结果不是 `B-06` 发布证据。

## 发布与治理边界

- Parcel、TMS 和 Bento 是三个独立仓库、CI、版本和发布周期；任何一方都不是其他方宿主。
- 当前一人阶段由 `idp-repo` / user ID `310467973` 承担责任，不用多个账号或角色名称伪造职责分离。
- Parcel 提交给 Bento 的合同证明必须针对同一不可变候选、来自 Parcel 自身安全边界，并携带 `NO INDEPENDENT HUMAN APPROVAL` 与 `OWNER-BYPASSABLE`。
- Bento 自动门禁、TMS/Parcel 双消费者合同、WORM/DSSE 证据、至少 24 小时冷静期和新 CI run 复验全部通过前，不得创建稳定 `v0.x`。
- 认证授权、跨租户隔离、密码学、不可逆数据库收缩和严重安全修复进入外部客户稳定交付前需要外部专业复核。

## 完成门禁

首个 Parcel 消费者切片只有同时满足以下条件才可标记为可执行基线：

1. 独立 Git 仓库、单 `go.mod`、Go `1.26.5`、模块边界和三个命令入口可以在 Linux CI 构建。
2. PostgreSQL 16 上真实迁移、schema 检查、显式 SQL Repository 和事务测试通过。
3. 来源保全、同键重复、同键冲突、输入未受理和每份委托独立提交均有测试。
4. 委托业务记录与“委托已提交”Outbox 意图满足同事务成功/回滚证明。
5. 任何输出、日志、事件和测试夹具不泄露真实客户、地址、联系人、货物或申报资料。
6. 未实现代码不返回`已接受`或`已拒绝`，不创建接受基线、预计承诺或财务结果。
7. 所有未决业务参数继续显式未配置；测试使用命名夹具，不把示例值变成生产默认值。
8. 真实候选出现后，`PBC-01` 至 `PBC-09` 对同一版本和 checksum 通过，才可把该 Parcel 提交登记为 `B-06` 候选基线。

## 下一步

1. 已完成：首个提交、远端、业务模块目录、架构测试、`PS-W1-S1` 来源保全与`已提交`子切片，并锁定不可变候选 `v0.1.0-rc.1`。
2. 已完成：PostgreSQL 16 集成测试和 `bento-contract`，`PBC-01`、`PBC-05`、`PBC-06`、`PBC-08` 通过。
3. 待完成：`PBC-02` 强类型复合键 Repository 合同、`PBC-03` 业务写入与 Outbox 同事务可见性正反证明、`PBC-04` 并发重复抑制、`PBC-07` `ErrCommitUncertain` 查询续办，以及 `PBC-09` 的证明 JSON。
4. 待完成：`parcel-outbox` 发布进程、`chi` 接入适配器，以及与 TMS 一起执行 `B-06` 双消费者证明后再考虑稳定发布。

## 链接

- [小包托运上下文](../domain/parcel-shipment/CONTEXT.md)
- [UC-PS-001：客户提交国际小包委托并取得接受决定](../application/parcel-shipment/UC-PS-001-SUBMIT-SHIPMENT-REQUEST.md)
- [UC-PS-001 生产接单业务决策简报](../application/parcel-shipment/UC-PS-001-BUSINESS-DECISION-BRIEF.md)
- [领域上下文地图](../domain/CONTEXT-MAP.md)
- [ADR-0002：国际小包采用独立数据、运行与发布边界](../adr/0002-independent-data-runtime-release-boundary.md)
- [ADR-0005：由来源事实形成有效事件并派生状态](../adr/0005-source-facts-effective-events-derived-state.md)
- [ADR-0009：采用 Go 模块化单体并复用版本化 Bento 技术合同](../adr/0009-go-modular-monolith-and-versioned-bento-contracts.md)
