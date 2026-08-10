# IDP Parcel Go 首个消费者切片实施决策简报

Status: Confirmed

## 目标

把 `idp-parcel` 的技术代码基线和首个 `idp-bento-go` 真实消费者切片固定到可以直接建立仓库、包结构、迁移和合同测试的程度，同时严格保留 [`UC-PS-001`](../application/parcel-shipment/UC-PS-001-SUBMIT-SHIPMENT-REQUEST.md) 中尚未确认的业务参数。

首个切片只证明以下闭环：

- 原始提交及来源语义先被不可变保全。
- 完整拟受理范围先取得显式生产归属；未明确由本产品承接时不建立 `parcel-shipment` 委托。
- 一份已建立最小身份的委托进入`已提交`，但不冒充已经接受或拒绝。
- 委托、声明包裹、提交版本、接受判断任务和“委托已提交”事件发布意图原子提交。
- 重复、同键冲突、事务回滚、提交结果不确定和 Outbox 重试有明确行为。
- Parcel 的真实 Repository、事务、迁移和 Outbox 使用同一不可变 Bento 候选运行消费者合同。

本文不补全真实客户、合同、线路、字段、金额、阈值、角色、超时、重试次数或 SLA，也不宣布生产接单已经完成。

## 证据边界

- `idp-parcel` 是独立产品，拥有独立领域模型、业务数据库、运行和发布边界，见 [ADR-0001](../adr/0001-autonomous-product-domain-boundary.md) 和 [ADR-0002](../adr/0002-independent-data-runtime-release-boundary.md)。
- `parcel-shipment` 已确认拥有提交批次、委托、声明包裹、接受决定和接受基线；客户提交与委托接受是两个业务事件。
- `UC-PS-001` 已确认“委托已提交”的业务语义，以及业务状态、决定记录和事件发布意图的原子提交要求。
- `BD-PS-001` 至 `BD-PS-008` 的接单机制已经确认，但不扩大本简报首个“来源保全并进入已提交”子切片的范围。真实合同规则、角色、金额、阈值、判断时点和试点归属参数仍阻塞生产接受。
- `idp-bento-go` 的正式 module path、Git 远端、DNS、TLS 和 vanity metadata 已建立；当前仍没有可用于消费者基线的不可变 `v0.1.0-rc.N` tag、私有读取身份和空缓存下载证明。
- Parcel 本地工作区已经初始化 Git `main`、`go.idp.xyz/idp-parcel` 单 module、Go `1.26.5` toolchain、`chi/v5 v5.3.1` 基础 API 和只读权限基础 CI，并建立了建单前的作用域身份、来源重放/冲突、最小委托候选与同租户提交批次候选领域内核；WSL race test、本机 `go test`、`go vet` 与 build 已通过。当前仍没有首个提交、远端、可持久化业务切片或 Bento 候选依赖。

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
| `P-12` | 首个真实切片为 `UC-PS-001` 的“来源保全、取得生产归属并进入已提交”子切片；只有本产品已被明确选为当前生产权威时才建立委托，不形成接受、拒绝、预计承诺或财务控制结果 | `CONFIRMED` |
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
    domain/                   委托、声明包裹、提交版本、接受判断任务与领域事件
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
- `internal/platform` 只放进程级共享技术设施；业务 HTTP 端点一律落在 `internal/<context>/adapters/http`，不因就近而写入 `platform/httpapi`。
- 新增进程入口与对外交付形态（额外 `cmd/` 二进制、前端或其他客户端）是加法：它们在 `internal/` 之外落位，不改变本节包布局，因此不为将来的多端形态提前重排目录或预建空壳。
- 面向人的客户端应用落在顶层 `apps/`，每端一个子目录；端之间的共享物落在顶层 `packages/`。判据是它服务谁——服务租户自己作业与治理人员的端属于产品的一部分，与后端同仓同版本发布；租户的锚点货主客户走 [`PAR-INT-01`](../product/PILOT-PARAMETER-REGISTER.md) 登记的租户现有渠道，那不是本仓要建的界面。已确认的两个端是租户管理界面与一线作业客户端；完整取舍、判据与被否决方案见 [ADR-0021](../adr/0021-frontline-operations-client-is-part-of-the-product.md)，本节不另立口径。两条按该记录不落在这里：开发方自用的跨租户运维后台不属于产品；后台 worker 是 Go 进程入口，仍落上表的 `cmd/`。两个目录都等第一个真实端落地时才建，按上一条不预建空壳；端的存在也不倒推领域或应用层改动，因为用例层按 [application/README](../application/README.md) 刻意不绑定 API、文件或门户等传输方式。

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

子切片名称：`PS-W1-S1 来源保全、生产归属并提交委托`。

它实现 `UC-PS-001` 步骤 1、2、3A、3B、3C 的最小闭环：

1. 接入适配器解析显式集团租户、货主客户账户、来源和来源请求键。
2. 原始内容、内容摘要、来源发生时间、系统接收时间和关联信息先形成不可变来源记录。
3. 无法建立客户范围、委托边界或最小成员身份时记录`输入未受理`，不创建占位委托。
4. 已建立最小身份时，先取得完整拟受理范围的版本化生产归属结果。只有 `idp-parcel` 已被明确选为当前唯一生产权威时才继续建单；其他权威或归属未决只保留接入、归属和安全续办记录。
5. 本产品取得生产归属后，每份委托在独立事务中创建`已提交`状态、当前提交版本、一个或多个声明包裹和该版本的接受判断任务；任务建立不表示已经接受。
6. 同一事务写入 Parcel 业务记录和“委托已提交”Outbox 信封。
7. 返回生产归属结果，或者返回`已提交 / 尚未决定`及稳定查询关联；不执行接受条件判断。

明确排除：

- `已接受`、`已拒绝`、接受基线和预计承诺。
- 商业资格、可达性、信用、余额或资金冻结结果。
- `BD-PS-001` 至 `BD-PS-003`、`BD-PS-005` 至 `BD-PS-008` 的接受机制执行；`BD-PS-004` 的真实准入规则、权威系统映射和跨系统交接集成。首切片只消费显式生产归属结果，不能假定本产品天然拥有生产权威。
- 面单交易、初始路由、关务建案、节点收寄、履约或结算。

### 输入与幂等

应用命令必须显式携带强类型作用域：

- `TenantID`
- `CustomerAccountID`
- `Source`
- `SourceRequestKey`
- `PayloadDigest`（规范化业务内容摘要；包含 `requestEffectiveAt` 的值及缺失/显式存在状态，不包含 `occurredAt`/`receivedAt`）
- `SubmissionBatchID`
- `ShipmentRequestID`
- 一个或多个 `DeclaredParcelID`
- 来源发生时间（`occurredAt`）、系统接收时间（`receivedAt`）和客户请求生效时间（`requestEffectiveAt`）

`SourceRequestKey` 的真实组成由各接入适配器依据 `PAR-INT-01` 绑定，不能直接把客户参考号当作全局幂等键。Parcel 持久化唯一性至少覆盖租户、客户账户、来源和来源请求键：

- 同键、同摘要：返回已有来源记录和原接入处理结果；即使 `occurredAt`/`receivedAt` 不同也仍是重放，并只追加本次观察；已经形成生产归属时返回原归属结果，只有已经建单时才附已有委托结果。
- 同键、不同摘要：返回接入冲突，保留原内容，不覆盖、不再建单；`requestEffectiveAt` 的值或缺失/显式存在状态变化必须通过摘要变化进入该路径。
- 不同租户或客户账户：即使外部键相同也必须隔离。
- 提交结果不确定：使用稳定请求关联查询原结果；只有已证明业务幂等时才能自动续办，不能盲目重放事务。

### 事务边界

来源保全和业务提交分为两个明确事务，生产归属必须在二者之间形成并在业务提交时仍然有效：

1. **来源保全事务**：保存原始内容或不可变引用、摘要及接入处理状态。后续解析或依赖失败不能删除该记录。
2. **委托提交事务**：仅在本产品拥有当前生产权威时，创建提交批次关联、委托、当前提交版本、声明包裹和接受判断任务，随后 Enqueue 同一事件信封；任一步失败全部回滚。

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
3. 来源保全、同键重复、同键冲突、输入未受理、非本产品生产归属、生产归属未决和每份委托独立提交均有测试；未取得本产品生产归属时不得创建委托、接受判断任务或“委托已提交”事件，取得后任务与委托提交必须同时成立或同时回滚。
4. 委托业务记录与“委托已提交”Outbox 意图满足同事务成功/回滚证明。
5. 任何输出、日志、事件和测试夹具不泄露真实客户、地址、联系人、货物或申报资料。
6. 未实现代码不返回`已接受`或`已拒绝`，不创建接受基线、预计承诺或财务结果。
7. 所有未决业务参数继续显式未配置；测试使用命名夹具，不把示例值变成生产默认值。
8. 真实候选出现后，`PBC-01` 至 `PBC-09` 对同一版本和 checksum 通过，才可把该 Parcel 提交登记为 `B-06` 候选基线。

## 实现准入检查

本节只记录当前代码可以越过哪一道实现闸门，不复制参数值或证据正文。任何解锁都必须绑定可追溯的 Parcel 代码基线和不可覆盖决定；文档完成、测试通过或候选文件存在不能自动解锁。

闸门按阻断理由的性质分别裁决，依据 [ADR-0017](../adr/0017-admission-gates-judged-by-blocking-cause.md)；下表的「当前结论」与「仍禁止范围」按该记录重述，本节不另立口径。

| 闸门 | 评审 `asOf` 与 Parcel 代码基线 | 权威证据引用及版本 | 当前结论 | 精确解锁范围 | 仍禁止范围 | 失效或复评触发 | 决定记录引用 |
|---|---|---|---|---|---|---|---|
| `PN02-W01/W02` 业务语义 | 当前本地未提交基线；首次评审须固定提交 ID 和评审时间 | 参数登记册、W01/W02 完成结论及其受控证据版本、ADR-0016、ADR-0017 | 机制半边放行；实例半边保持阻断：W01 仅有待核验候选，W02 直接参数仍待提供 | 已解锁 `parcel-shipment` 应用编排与命令处理、Parcel 自有语义端口接口、`已提交`聚合的领域形态及其确定性测试替身 | 真实客户、合同、线路、金额、阈值、角色或时限取值进入代码；任何租户流量进入生产接单；端口的 PostgreSQL 适配器（属下一道闸门） | 范围、版本、`asOf`、权威身份、交接语义或证据状态变化 | [ADR-0017](../adr/0017-admission-gates-judged-by-blocking-cause.md) |
| Bento 持久化技术 | 当前本地未提交基线；首次评审须固定提交 ID、RC 和 checksum | ADR-0009、不可变 Bento RC、空缓存下载和适用 `PBC-*` 消费者证明 | 保持阻断：尚无可用不可变 RC 或消费者证明 | 本闸门通过后才解锁 PostgreSQL Repository、迁移、事务与 Outbox 实现 | 本地框架替身、`replace`、浮动分支、伪事务和未证明的发布基线 | RC、checksum、消费者合同、迁移或 Parcel 依赖基线变化 | [ADR-0017](../adr/0017-admission-gates-judged-by-blocking-cause.md) |

两道闸门相互独立。业务语义的机制半边放行不解锁任何持久化技术；Bento 技术候选存在也不解锁 W01/W02 的实例半边。Parcel 自有端口的确定性内存替身替的是 Parcel 的端口而非 Bento 的框架合同，不属「本地框架替身」。

## 下一步

1. 已完成本地 Git `main`、`go.idp.xyz/idp-parcel` 单 module、LF 规则、基础 API/CI 和可测试包；首个提交与远端仍未绑定。
2. 按 [`PN-02` 真实参数取证与开发交接](./pn-02-real-parameter-evidence-and-development-handoff.md)并行取得锚点商业、接入归属、接单财务和可达性证据；未确认值继续保持显式未配置。
3. 建单前领域内核已经覆盖作用域身份、来源重放/冲突、最小委托候选和同租户提交批次候选。按 [ADR-0017](../adr/0017-admission-gates-judged-by-blocking-cause.md)，在此之上推进应用编排、命令处理与 Parcel 自有语义端口接口，并以确定性内存替身验证；不实现任何端口的 PostgreSQL 适配器，不写入未确认取值。
4. 先按 [`PN02-W01`](./pn-02-w01-anchor-commercial-scope-evidence-request.md) 和 [`PN02-W02`](./pn-02-w02-ingress-production-ownership-evidence-request.md) 核验完整范围、版本、时点、当前权威和安全交接语义，再形成业务语义闸门**实例半边**的决定；机制半边不等待该决定。
5. 等真实不可变 Bento RC、checksum 和适用消费者证明存在，且业务语义闸门同时通过后，再建立 PostgreSQL 16 Repository、迁移、事务、Outbox 及其集成合同；不得提前建立本地替身。
6. 等 TMS 与 Parcel 都形成适用最小切片提交后，再绑定 `B-06`、执行空缓存下载和双消费者证明。

## 链接

- [小包托运上下文](../domain/parcel-shipment/CONTEXT.md)
- [UC-PS-001：客户提交国际小包请求并取得生产归属或接单结果](../application/parcel-shipment/UC-PS-001-SUBMIT-SHIPMENT-REQUEST.md)
- [UC-PS-001 生产接单业务决策简报](../application/parcel-shipment/UC-PS-001-BUSINESS-DECISION-BRIEF.md)
- [领域上下文地图](../domain/CONTEXT-MAP.md)
- [ADR-0002：国际小包采用独立数据、运行与发布边界](../adr/0002-independent-data-runtime-release-boundary.md)
- [ADR-0005：由来源事实形成有效事件并派生状态](../adr/0005-source-facts-effective-events-derived-state.md)
- [ADR-0009：采用 Go 模块化单体并复用版本化 Bento 技术合同](../adr/0009-go-modular-monolith-and-versioned-bento-contracts.md)
- [ADR-0016：产品交付与租户试点作为两条并行验收轨道](../adr/0016-product-delivery-and-tenant-pilot-as-parallel-tracks.md)
- [ADR-0017：实现准入闸门按阻断理由分别裁决](../adr/0017-admission-gates-judged-by-blocking-cause.md)
