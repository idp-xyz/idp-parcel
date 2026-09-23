# ADR-0143：客户对账单号由平台编号能力在发布事务内分配，外部工件先过闸门

Status: Proposed（草案，2026-09-21；接受前不改变当前命令、依赖或迁移，接受与否归本仓维护者）  
Date: 2026-09-21

## Context

`settlement-accounting` 已经决定草稿发布后保存固定对账单号，发布后的单号、费用范围和金额快照不可覆盖；同一发布意图重放必须返回原结果，不能生成第二个单号。这些业务不变量属于本上下文，现由 `StatementNumber` 非空值对象和 `(tenant, statement_number)` 的持久化键表达。

当前实现没有单号生成机制。`PublishStatementCommand` 要求调用方提供 `Number`，应用先按（租户，单号）查重，再把这个单号写入对账单和后续交接。`StatementNumber` 不拥有格式或序列算法，结算迁移也没有编号技术表。发布写入已经要求 Bento 事务上下文，但发布编排本身尚未拥有事务壳。

`idp-platform-go` 在 `master` 提交 `f9a86f8` 提供了跨产品 `numbering` 能力：调用方显式传入租户范围，在消费者事务内原子分配序列，并由消费者执行库拥有的技术迁移。Parcel 的已确认用例是 `CUSTOMER_STATEMENT` 的 `STMT-{yyyy}{mm}-{seq:5}`，按月重置。该仓库当前仍带开发期 `replace`，没有可锁定的发布版本与 checksum，因此本仓尚不能把它作为正式 Go 模块依赖。

直接把 `assigner.Assign` 插入现有 `Publish` 也不成立。现有幂等查询依赖调用方给出的单号；如果每次重放都在查询前分配，新号会绕过“同一发布意图返回原号”的规则。平台当前不提供预留/认领机制，发布意图与已分配单号的关系必须由 Parcel 自己持有。

平台库拥有编号代码及其不可变技术迁移，Parcel 拥有对账单业务数据和业务迁移执行。把平台 SQL 复制到 `settlement_accounting`，或把平台实现放进 Parcel，都会产生第二套编号权威。现有迁移计划也没有 `numbering` 模块或技术 schema 的落点。

## Decision

本仓计划在外部工件满足闸门后，消费 `go.idp.xyz/idp-platform-go` 的正式、精确版本；在此之前不修改 `go.mod`，不添加 `replace`、`go.work`、源码副本或本地替身。

编号职责按以下边界落位：

- `settlement-accounting` 继续拥有对账单、`StatementNumber` 的业务不变量、发布/作废/替代生命周期以及租户隔离；平台库不拥有结算数据，也不从 `context` 推断租户。
- 结算应用通过消费侧适配器调用平台 `numbering`，显式传入租户和 `CUSTOMER_STATEMENT` 类型。适配器只依赖平台公开 API，不触碰平台 `internal` 或 `testkit`。
- 计划使用已确认的 `STMT-{yyyy}{mm}-{seq:5}` 机制，初始方案不启用 `{seqx}`。若将来启用加扰，密钥只从实例配置注入，不进入仓库，也不提供生产默认值。

单号分配必须在发布事务内完成：通过稳定的 Parcel 发布意图先查找或登记已分配单号，再在同一个 Bento 事务上下文中分配号码、保存不可变对账单并写入适用的发布交接。分配失败或事务回滚不得留下对账单或已提交号码。发布意图到单号的登记和查询由 Parcel 拥有，不能依赖平台的预留/认领能力；`StatementKey` 仍可按（租户，单号）寻址已发布对账单，作废替代单继续使用新身份。具体发布意图字段、登记表和端口形状由实现票依据本决定落地，但不得删除这条幂等边界。

平台技术迁移由 Parcel 执行而不改写：取得正式版本后，迁移计划须把平台编号表作为独立的平台技术模块/schema 接入，保留其来源、版本和 checksum，并与平台公开 API、Bento 事务合同在同一消费者变更中验证。它不归入 `settlement_accounting` 业务迁移，也不复制平台 SQL。迁移资产交付形态、schema 名称、模块 loader、embed/plan 接线和回滚检查在实现前必须由平台发布物确认。

本决定不改变当前生产准入状态。只有同时具备以下证据后，才可把本记录从 Proposed 提升为 Accepted 并开始正式接入：

1. 平台正式 module 版本、`go.sum` checksum 和可复跑的空缓存下载/编译证明；
2. 平台 assigner 与本仓 Bento 事务版本的消费者合同证明；
3. 平台技术迁移的不可变资产、独立 schema/module 归属和 Parcel 迁移计划接线证明；
4. 发布意图重放、并发、租户隔离、月度重置、分配失败及事务回滚测试通过；
5. `settlement-accounting` CONTEXT、`UC-SA-003` 和 PN-07 交接同步记录单号归属与分配时点，且未把实例密钥或租户参数写成默认值。

## Consequences

- 当前代码继续接受调用方单号；本记录不会把未发布的外部库伪装成已接入能力。
- 将来接入需要同时改应用端口/适配器、事务装配、发布意图持久化和迁移计划，不能只增加一个 `go.mod` 行或在现有 handler 前调用一次 assigner。
- 平台序列的技术表由平台维护，Parcel 负责自己的数据库执行和证据记录；两个产品仍保持独立数据与发布边界。
- 纯序列模板无需实例密钥；加扰方案的密钥管理和真实租户绑定仍属于实例半边，未取得证据时保持未配置。
- 直到外部版本和上述证明齐备，PN-07 的真实账期与生产准入仍不因本草案改变。

## Alternatives considered

- **现在直接在 `go.mod` 引入 `master` 或本地 `replace`。** 否决：版本不可锁定、checksum 不可证明，且违反 ADR-0009/0026 的依赖闸门。
- **把平台编号实现或 SQL 复制进 Parcel。** 否决：会让 Parcel 拥有第二套算法或技术表，破坏“库拥有代码、应用拥有数据”的边界。
- **继续要求调用方永久传入单号。** 否决：保留现状可以暂时运行，但没有为跨应用编号能力划定归属，也无法兑现已确认的 Parcel 编号用例。
- **在发布事务外预先分配，或在每次重放前再次分配。** 否决：无法保证回滚释放和同一发布意图只得到一个号码。
- **默认启用 `{seqx}` 并在仓库放置密钥。** 否决：密钥是实例半边敏感参数，不能由产品代码猜测或提交默认值。

## Links

- [ADR-0001：国际小包采用自治产品与领域边界](./0001-autonomous-product-domain-boundary.md)
- [ADR-0002：国际小包采用独立数据、运行与发布边界](./0002-independent-data-runtime-release-boundary.md)
- [ADR-0009：采用 Go 模块化单体并复用版本化 Bento 技术合同](./0009-go-modular-monolith-and-versioned-bento-contracts.md)
- [ADR-0017：实现准入闸门按阻断理由分别裁决](./0017-admission-gates-judged-by-blocking-cause.md)
- [ADR-0025：跨上下文调用的适配器落在消费侧，翻译职责由它独占](./0025-cross-context-adapters-live-on-the-consumer-side.md)
- [ADR-0026：为产出消费者证明而写的持久化实现先于闸门通过，发布基线登记仍在闸门后](./0026-persistence-written-for-consumer-proof-precedes-gate-passage.md)
- [ADR-0031：自有仓储端口的写入结果是封闭代数而不是 error](./0031-owned-repository-write-outcome-is-a-closed-algebra-not-an-error.md)
- [ADR-0043：发布意图由结果标识认领，重放重发同一份](./0043-publish-intent-claimed-by-result-identity.md)
- [结算与经营核算上下文](../domain/settlement-accounting/CONTEXT.md)
- [UC-SA-003：截单、发布并处理客户对账单](../application/settlement-accounting/UC-SA-003-CUT-OFF-PUBLISH-AND-RECONCILE-CUSTOMER-STATEMENT.md)
- [Go 首个消费者切片实施决策简报](../design/parcel-go-first-consumer-slice-decision-brief.md)
- [idp-platform-go `f9a86f8`](https://github.com/idpxyz/idp-platform-go/tree/f9a86f8)：编号能力、迁移归属和消费者使用约束的外部审阅基线
