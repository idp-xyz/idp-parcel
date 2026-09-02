# 02 PS/NR 侧：ADR-0014、ADR-0061 与 ADR-0068 在 main 里有没有执行器

Category: chore
Status: resolved——取证完成，答案见「答案」节；去留决定归集成方，本票不代定
Assignee: MCP-6（已失去响应，2026-09-01 由 MCP-4 代做，见 Comments）

**只读取证票。零代码改动，不合并、不 cherry-pick、不拆树、不删分支。**

## 要答的问题

与[票 01](./01-pc-side-adr-executors-in-main.md) 同型、同方法，换三笔。[父清单](../spec.md)第三类里有五笔实现已接受 ADR 的提交不在 main；`git cherry` 只答「这一笔的 patch 不在 main」，**不答「这件事在 main 里没做」**。本票取证 PS/NR 侧三笔：

| 分支 | SHA | ADR |
|---|---|---|
| `t14-payload-digest` | `321c841` | [ADR-0014](../../../docs/adr/) 载荷规范化摘要的版本化形状（提交信记 PSC-1） |
| `ps-rehydrate-accepted` | `2f7e144` | [ADR-0061](../../../docs/adr/) 已接受委托重建门 |
| `nr04-catalog-registration` | `6cf6c89` | [ADR-0068](../../../docs/adr/0068-versioned-network-catalog-structure-precedes-rule-content.md) 网络目录结构先于规则内容 |

ADR-0014 与 ADR-0061 的文件名本票不写死，按 `docs/adr/` 下编号自取——写错一个文件名比不写更费事。

逐 ADR 答三问：

1. **main 里有没有这条 ADR 决定的执行器？** 执行器指真正让那条决定生效的类型、函数或约束，不是同名文件、不是端口声明、不是注释里提到 ADR 编号。
2. **有的话，它与分支那笔是什么关系？** 等价实现 / 只覆盖一部分（说清哪一格没覆盖）/ 换了做法（说清换成什么）。
3. **没有的话，缺的是哪一格？** 按 ADR 的 Decision 逐条对，不要整篇打包判。

`6cf6c89` 另带一个进程入口 `cmd/parcel-network-register`，`2f7e144` 触及 `cmd/parcel-dispatch` 与 `internal/architecture`。**这两处要单独看一眼 main 里在不在**——机制件在而进程入口不在，与两者都不在，是不同的答案。

## 取证方法上的两个坑，务必避开

**一、「文件在」不等于「规则有执行器」。** 父仓已经栽过同型的跟头：只写测试的契约（`*_contract_test.go`）被读成实现，会高估就绪度；判据要的是**规则有执行器**。端口声明是形状不是执行。

**二、「接错看着像接对」。** 一个按租户过滤的读口去查一张没有租户的表，编得过、测得过、页面也出得来，只是答的不对，没有任何测试会红。判「有执行器」时要往下走一层，看它实际约束住了什么。

## 写法红线

- **写证据不写结论**：每个断言锚住取证 SHA（本轮基线 `main = aeeb709`）与取数命令。命中数不作数，要打开看。
- **不用行号、不用计数引用别处**；数本身就是论点时必须锚 SHA。
- **中文**。改中文文件不要用 `Set-Content`（会写成乱码让 `go build` 报 `illegal UTF-8`）。
- 「查不到」要多问半句：是真查过，还是没想到能查。

## 地盘与禁止

- **只写 `.scratch/unmerged-branch-inventory/issues/02-*.md`**（本文件，答案追加到 `## 答案` 一节）。票 01 归 MCP-4，不要碰。
- 不改任何 `.go` / `.sql` / `.tsx`；不动 `docs/**`；不改任何票面 `Status:`（本文件自己的除外）。
- 不跑 `go mod tidy`、全树 `gofmt -w` 或任何扫全树再写回的命令。
- 不合并、不 cherry-pick、不拆 worktree、不删分支。父清单里三棵有未提交内容的 worktree 一律不碰——其中 `%TEMP%/idp-parcel-mcp1-t14` 正是 `t14-payload-digest` 那棵，本票只读 main 与分支提交，不进那棵树。
- 不提交，不推送。做完在频道报 MCP-2。

## 答案

由 MCP-4 代 MCP-6 完成（MCP-6 在本票派发的同一分钟失去响应，交接说明见 Comments）。取证于 `main = aeeb709`（本树 `git rev-parse HEAD main` 两值相同；`.go` 与 `.sql` 在 `git status` 上零改动，故读工作树等同于读 `main@aeeb709`）。

### 总答：三笔的活全部已在 main，三笔都是换基座重放

| 分支笔 | 引入 main 的笔 | 标题关系 |
|---|---|---|
| `321c841`（ADR-0014） | `b51de75` | 逐字同，尾部多一句「票 14 转 resolved（票面回写由抢救笔补，原笔未带）」 |
| `2f7e144`（ADR-0061） | `6228d8e` | 逐字同 |
| `6cf6c89`（ADR-0068） | `e2620ab` | 逐字同 |

三笔各有新增文件，因此 `git log main --oneline --diff-filter=A -- <该笔新增的文件>` 就够用，不必动用只改不增才需要的 pickaxe。三笔的父都与对应 main 笔不同（`321c841^ = 2b68c89` / `b51de75^ = e2620ab`；`2f7e144^ = 6ab9f0c` / `6228d8e^ = 385b421`；`6cf6c89^ = 82eb4e1` / `e2620ab^ = c26b50f`）——patch-id 含上下文行，换基座重放即不等价，`git cherry` 的 `+` 由此而来。

**逐文件比 blob（各自比引入时那一版，不与今天的 main 比）：三笔触及的文件几乎全部相同，只有一个例外。**

例外是 `2f7e144` 与 `6228d8e` 的 `internal/parcelshipment/adapters/postgres/shipment_request.go`。打开看，那处差异整段都是 **ADR-0060** 的当前包裹投影——`current_submission_version_id` 与 `declared_parcel_ids` 两列进 INSERT/UPDATE、新增 `currentParcelProjection`、类型声明上多实现一个 `ports.CurrentAcceptedParcelTargetView`。`6228d8e` 的基座已经有 ADR-0060 而 `2f7e144` 的没有，**与 ADR-0061 无关，且方向是 main 领先分支**。ADR-0061 的内容一格不缺。

顺带一条给下一个人：票面把 ADR-0061 的文件名留给取证方自取是对的。它在 `docs/adr/0061-accepted-shipment-request-rehydration-by-snapshot-expressiveness.md`，编号后面还有 `accepted-` 一段；按不带那一段的名字去 `git cat-file` 会答不存在，那不是「main 没有这份 ADR」。

### ADR-0014：三问

**先分半边，否则整篇打包会判错。** ADR-0014 的 Context 与 Decision 长在 **parcel-pricing** 上（`fingerprint.go` 的 `canonicalPricingPlan`、价表族、评价回放），而 `321c841` 把同一条决定应用到 **parcel-shipment** 的提交载荷摘要。两半各有各的执行器，成色也不同。

**问一（计价半边，非本笔所出，但属这条 ADR 的决定）：执行器在，且三条各有落点。**

- 「规范化形状获得显式版本标识」：`internal/parcelpricing/domain/fingerprint.go` 的 `canonicalizationVersion = "PPC-3"`，进 `canonicalPricingPlan` 与 `canonicalEvaluation` 两个文档的 `Canonicalization` 字段，`CurrentCanonicalizationVersion()` 对外报出。
- 「摘要不一致只有在规范化版本相同时才构成版本内容冲突」：执行器是**比对门的先后次序**——`price_card_catalog.go` 与 `reference_series_register.go` 都先比规范化版本、不同即答 `PriceCardCanonicalizationDiffers` / `ReferenceSeriesCanonicalizationDiffers`，再比摘要才答`版本内容冲突`。两格在 `ports` 上是两个独立取值，不是同一格的两种措辞。这一条最容易被实现成一格，main 上确实分开了。
- 「不携带版本的摘要视为不完整」：`plan_snapshot.go` 与 `registered_price_card.go` 在 `document.Canonicalization != canonicalizationVersion` 时回 `ErrCanonicalizationVersionUnsupported`，不尝试用本构建的形状去重算别的版本的摘要。
- 透出面：`query_evaluations.go` 把 `Canonicalization` 与双摘要照登透出，注释自称是「ADR-0014 的比对列」。

**问一（委托半边，`321c841` 所出）：执行器在，但没有生产调用方。**

`internal/parcelshipment/domain/payload_canonicalization.go` 的 `payloadCanonicalizationVersion = "PSC-1"`；`CanonicalizeSubmissionPayload` 的返回是 `NewPayloadDigest(payloadCanonicalizationVersion + ":" + hex...)`——**版本前缀长在摘要串本身上**，不是旁边一列。「已保存摘要必须携带产生它的规范化版本」在这一半靠串自带，摘要与版本没有分家的可能。

没有生产调用方这一点要说准：全仓 `.go` 上 `CanonicalizeSubmissionPayload` 只被它自己的测试调用，`submit_shipment_request.go` 与 `identity.go` 只在注释里提到它。生产侧 `SubmissionIntake` 的唯一实现是 `UnconfiguredIntake`——把渠道原始载荷翻成 `SubmissionPayloadSpec` 词表的规则属 `PAR-INT-01` 实例半边。**分支笔自己就写着「不接线不碰装配点」，所以这不是漏接，是待提供。** 与票 01 里 ADR-0059 那一格同形状，但阻断原因不同：那边是「五维无人读」被 ADR 自己认作显式代价，这边是接入契约未到。

**问二：等价实现**（引入时五个文件 blob 全同）。**问三：不适用。**

### ADR-0061：三问

**问一：Decision 四条逐条有执行器。**

- **决定一（门开到`已提交`与`已接受`，`已拒绝`/`已撤回`仍拒）**：`admitRehydratedState` 对 `ShipmentRequestSubmitted, ShipmentRequestAccepted` 回 `nil`，对 `ShipmentRequestRejected, ShipmentRequestWithdrawn` 回 `ErrRehydrationStateNotSupported`，越界值另走一格印数字。
- **决定二（`已接受`的快照表达）**：`RehydrateAcceptanceDecisionSpec`、`RehydrateAcceptanceBaselineSpec`、`RehydrateExpectedCommitmentSpec` 三个类型在 `rehydration.go` 上，各带自己的 `present()`；`RehydrateShipmentRequestSpec` 把那四项按「`已提交`下必然缺席，`已接受`下决定、基线、承诺必须在场、资料版本可空」写在字段处。
- **决定三（半截的已接受快照是坏数据，不是「本期不支持」）**：`acceptedProductsValidForRehydration` 是这一条的执行器。它对`已提交`逐项拒绝携带产物，对`已接受`要求带齐，而所有拒绝都走 `rehydrationRefusal`——那个函数包的是 `ErrInvalidRehydratedShipmentRequest`，不是 `ErrRehydrationStateNotSupported`。**两个哨兵分开正是这一条的全部内容**（ADR-0029 的分格规则），压成一个的话运维会照着一份完好的数据去等一扇早就开了的门。
- **决定四（PostgreSQL 快照文档按上表增设字段，读回只经公开访问器与领域构造函数）**：`shipment_request.go` 的类型注释写明快照文档形状照 `RehydrateShipmentRequestSpec` 设计、读回逐字段过领域构造函数再进 `RehydrateShipmentRequest`；`Revision` 与 `State` 刻意留在列上不进文档，避免第二个来源。

**问二：等价实现，且 main 领先**——多出 ADR-0060 的当前包裹投影（见上节）。**问三：不适用。**

### ADR-0068：三问

**问一：Decision 里可验的几条都有执行器。**

- **决定二（七表、版本行只增不改、同一身份至多一个未闭区间版本）**：迁移 `migrations/network_routing/0008_network_catalog.sql` 建七张表——物流节点、网络连接、线路、服务区域、服务日历、路由策略六张稳定版本表，加临时可用性调整一张。**六张稳定表各带一个 `WHERE effective_to IS NULL` 的部分唯一索引**，「至多一个未闭区间版本」是库上担保不是约定；`availability_adjustment` **没有**这个索引——它是历史链，正是决定四要的那一半分离。区间自洽由每表的 `CHECK (effective_to IS NULL OR effective_to > effective_from)` 挡。
- **决定四（两版同时适用交回错误）**：`ErrAmbiguousNetworkCatalog` 在 `internal/networkrouting/adapters/postgres/network_catalog.go`；判法是取回排序后逐对比较相邻两行的身份，同身份即报，并把族名与身份带进错误。它兜的是已闭区间的重叠——未闭那一半已由上面的部分唯一索引在库上挡住，两道各管一半，不重复也不遗漏。
- **决定五（修订锚随写推进，事实与修订单语句同取）**：`network_catalog_revision` 每租户一行，推进语句是 `INSERT ... ON CONFLICT (tenant_id) DO UPDATE SET revision = network_catalog_revision.revision + 1`；读侧是一条 `QueryRow`，把 `revision` 与七族适用行一起取回——单语句单快照，事实与修订必然同版。
- **端口与读写分口**：`ports.NetworkCatalogRegistry` 由 `NetworkCatalog` 以 `var _` 断言实现；读侧另立 `ports/catalog_read.go`，其注释明写七个方法与七张版本表一一对应、**不并进写口**。

**关于「有执行器但没有生产写入方」**：ADR-0068 自己的 Consequences 就写着「目录今天没有生产写入方：迁移不种任何默认行，登记入口等真实网络定义（实例半边）」，并预告「会出现一段目录可写可读、尚无人读它产出事实的时期」。所以这一笔的登记入口是 CLI 而非在线写口，是决定本身的安排，不是缺口。

**问二：等价实现**（引入时五个文件 blob 全同）。**问三：不适用。**

### 票面要我单看的两个进程入口

- **`cmd/parcel-network-register` 在 main**，`main.go` 与 `main_test.go` 与分支逐字节相同。它是真的新进程入口：`-kind` 是七值封闭集（`node` / `connection` / `line` / `service-area` / `service-calendar` / `availability-adjustment` / `route-strategy`），退出码三格 `exitRegistered = 0` / `exitUsage = 1` / `exitUndecided = 3`，没有治理格。
- **`2f7e144` 触及的 `cmd/parcel-dispatch` 与 `internal/architecture` 也在 main，但这两处要说准**：它触及的是 `cmd/parcel-dispatch/synthetic_v0_test.go` 与 `internal/architecture/rehydration_gate_test.go`，**两个都是测试文件，这一笔没有新增任何 cmd 二进制**。票面把它与 `6cf6c89` 的进程入口并列问，容易读成同一种东西——「机制件在而进程入口不在」这个分辨在 `6cf6c89` 上有意义，在 `2f7e144` 上不适用，因为它本来就没有进程入口这一格。

### 对父清单的影响（建议，不代改）

第三类可再降三笔，且三笔同属**换基座重放**——与第 9 笔 `mcp4-bento-pbc02` 的「带增删的整体采回」、第 11 笔 `pc01-identity-face` 的「隔数日的独立重写」是三种不同成因，合并成一句「已在 main」会把下一次复核需要的线索抹掉。改不改归父清单地盘。

### 「查不到」的那半句

- 真查过：`CanonicalizeSubmissionPayload` 的调用方在全仓 `.go` 上只命中它自己的测试与两处注释；`SubmissionIntake` 的实现只有 `UnconfiguredIntake` 与测试替身。
- 真查过：三笔各自新增的文件在 `main` 上逐件 `git cat-file -e` 全部命中。
- 没查、也不打算在本票查：`apps/admin-web` 是否有页面读网络目录或展示规范化版本。那不构成 Go 侧执行器，也不在本票路径上。

## Comments

- 2026-09-01 MCP-2：立票并派 MCP-6。
- 2026-09-01 MCP-4：**代 MCP-6 完成**。交接理由：本票任务 `task-ea251125` 派发于 22:17:48，MCP-6 的频道心跳停在 22:17:49 且此后未再写，任务状态一直停在 `pending`（同刻 MCP-2 亦已失去响应，用户 22:2x 告知只余本频道）。原票面 `Status:` 立票时即写作 `resolved` 而答案节为空，这一处不一致本笔按实况改。取证结论：三笔的活全部已在 main（`b51de75` / `6228d8e` / `e2620ab`），三笔都是换基座重放；三条 ADR 的 Decision 逐条有执行器。两处要读窄一格：ADR-0014 的委托半边 `CanonicalizeSubmissionPayload` 无生产调用方（`PAR-INT-01` 待提供，分支笔自己声明不接线），ADR-0068 的目录无生产写入方（该 ADR Consequences 自己写明）。零代码改动，未合并、未 cherry-pick、未拆树、未删分支、未进 `%TEMP%/idp-parcel-mcp1-t14`、未提交。
