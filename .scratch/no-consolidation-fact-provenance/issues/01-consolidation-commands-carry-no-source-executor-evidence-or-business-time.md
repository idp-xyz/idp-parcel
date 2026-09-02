# 集运三口不收来源、执行方、证据与业务时间——UC-NO-003 结果契约与 ADR-0023 在实现里没有落点

Category: bug
Status: done——三笔落主线（`f9c04da` 机制、`3f304c9` 真库取证、`98e1752` 读面到页面）；完成判据逐条对过，见 Comments
Blocked by: 无

取证于 `8acacfe`（2026-09-02）。发现者是一线过渡导入票（`frontline-transition-import/01`）：要让导入
进来的封签事实与设备扫描事实可区分，先得有一格能装「来源」，结果发现集运用例根本没有这一格。

## 事实

`internal/nodeoperations/application/consolidate_parcels.go` 的三个作业口，签名里只有租户、单元、
载具/成员/封签与作业依据：

- `ConsolidateParcelsHandler.Open(ctx, tenant, id, asset)`
- `ConsolidateParcelsHandler.AddMember(ctx, tenant, id, member)`
- `ConsolidateParcelsHandler.Seal(ctx, tenant, id, seal, basis)`

`ConsolidateParcelsDeps` 只有 `Store` / `Containment` / `Downstream` / `Clock`。封装与开封的时间取的是
`handler.deps.Clock.Now()`——处理时的系统时钟，不是现场事实自带的业务时间；`ConsolidationUnit.Seal`
与 `Unseal` 的 `at` 形参由此而来。聚合 `domain.ConsolidationUnit` 是成员集合 + 快照 + 封签的状态机，
没有任何字段表达谁做的、依据哪条来源、证据是什么。

对照同一上下文的收寄口 `ReceiveDeliveredUnitCommand`：它有 `SourceID`、`DeliveredBy`、`Evidence`、
`OccurredAt`，来源身份还做了幂等键（`ports.ReceptionKey`）。两口在同一个包里，一个把来源当一等公民，
一个完全没有。

## 它欠的是哪几句

- [UC-NO-003](../../../docs/application/node-operations/UC-NO-003-PROCESS-CONSOLIDATE-AND-SEAL-PARCELS.md)
  结果契约「作业事实已形成」一行的最低语义：「任务、对象、动作、发生时间、位置、**执行方、来源和证据**」；
  输入契约「作业来源」一行：「扫描、测量、位置、状况、移入/移出、封装、开封、封签和装卸事实——**来源先
  保存，不覆盖历史**」；`AT-NO-043`：「同一来源事实重复或内容冲突：重复返回原结果；冲突保留双方」——没有
  来源身份，这条验收在集运口上无从判定。
- [node-operations CONTEXT](../../../docs/domain/node-operations/CONTEXT.md)「封签记录」词条：「针对一次
  封装保存的封签标识、状态、**施封依据和观察历史**」；受控开封一条要求「节点保存开封前状况、原封签、
  授权来源、**执行人、时间**……」。
- [ADR-0005](../../../docs/adr/0005-source-facts-effective-events-derived-state.md)：来源事实、有效事件与
  派生状态分离——今天集运口直接改派生状态，没有来源事实那一层。
- [ADR-0023](../../../docs/adr/0023-work-fact-identity-and-time-are-minted-by-the-device.md)：作业事实的
  身份与时间由设备铸造——`Clock.Now()` 恰好是它禁止的那种「服务端代铸时间」。

## 不这么做

- **不把来源塞进 `WorkBasisReference` 或 `ConsolidationUnitID` 当前缀。** 一线过渡导入票里有人提过这
  条捷径（B1），MCP-1 2026-09-02 已否决：读的人会把它当作业依据，是「接错看着像接对」那一类。
- 不在导入 CLI 侧自造一张「谁导入了什么」的旁表来绕——来源属于事实本身，归拥有事实的上下文。
- 不动 `parcel-shipment`、不动 `transport-fulfillment` 的交接语义；本票只补 `node-operations` 集运口自己
  欠的那一层。

## 要做的

1. 给集运三口（以及 `RemoveMember` / `Unseal` / `Close`，同一条契约管着）补来源表达：来源身份（幂等键，
   照 `ReceptionKey` 的形状）、执行方、证据引用、业务发生时间由调用方带入，`Clock.Now()` 只保留给
   「记录时刻」这类确属服务端的时间。形状照收寄口，不另造第二套词。
2. 聚合与持久化留下这些字段——封签记录与封装快照各自带上来源与执行方；读面（节点作业读面）能显示出来。
3. `AT-NO-043` 在集运口上可判：同一来源身份同内容返回已有结果，同身份不同内容形成冲突。
4. 现有调用方（`cmd/parcel-api` 装配、种子、测试）跟上；`internal/architecture` 门禁不得红。

## 完成判据

- 三个作业口的命令带来源身份、执行方、证据、业务时间，缺来源身份即不受理（不是默认填系统时钟）。
- 真库往返用例证明来源与执行方落库并能读回；`AT-NO-043` 两向各一例。
- `gofmt -l` 无输出、`go build`、`go vet`、`go test -count=1 ./...` 绿且注明含不含真库。
- 完成后在 `frontline-transition-import/01` 的 Comments 记一条：集运子命令的阻断解除。

## Comments

- 2026-09-02 MCP-1：立票。触发是 MCP-5 在一线过渡导入票里报的边界 B，裁定 B3（本期不导集运）+ 立本票；
  取证由本会话在 `8acacfe` 上复核（签名、`Clock.Now()` 两处、收寄口对照）。
- 2026-09-02 MCP-1：机制落主线（`f9c04da`）。内容取自第三次并行崩溃的封存分支
  `mcp3-consolidation-provenance@87011d0`，五笔合成一笔——原第一笔是「在建」快照，其后三笔只是让
  测试跟上同一处改动，逐笔落主线会让中间那几个提交编译不过。合并前在隔离 worktree 上复核过：
  `gofmt -l` 与 `go build` / `go vet` 干净，全仓 `go test` 绿。
  路上有一格值得记：该 worktree 的 `.sql` 与两个 `.go` 在工作副本里是 CRLF，`gofmt -l` 列了文件、
  迁移的 `TestEmbeddedMigrationAssetsCarryNoCarriageReturnOrBOM` 也红。但 `.gitattributes` 定了
  `*.sql text eol=lf`，提交进库的 blob 本来就是 LF——删掉文件重新检出后两项都干净。**这是工作副本
  的产物，不是提交的问题**；照着 `gofmt -l` 的输出去改文件反而会把真内容改坏。
- 2026-09-02 MCP-1：完成判据逐条对过，补齐两处欠账后收口。
  - 判据 3「`AT-NO-043` 两向各一例」与判据 2 的真库往返，进来时只有应用层替身与「无事务即拒」
    一例，适配器层缺着——同上下文的收寄口与协作口都有这两例。`3f304c9` 补上，另把迁移新增的
    成员/封签在场 CHECK 也钉住（此前一条测试都没有），并在五条拒绝之后补一行合法插入，否则
    那五条可能都是被外键拒的，说明不了是这两条 CHECK 在起作用。
  - 判据 2「读面能显示出来」只做到端口与 Postgres：`ConsolidationUnitCatalogueRow` 带了两组来源，
    HTTP 响应体却把它们丢掉，页面看不到——而 `catalogue_read.go` 的注释已写着「页面据此分得开
    导入与扫描」。`98e1752` 接通响应体与「开启来源」「封装来源」两列，那句话现在为真。
  - 取证强度：四个新真库用例设 DSN 各 PASS、不设 DSN 各 SKIP（探针两向，不是只看一行 `ok`）；
    读面那条做过非空洞性检查——把 `review_catalogue.go` 的 `Scan` 两组来源对调，
    `TestReviewCatalogueListsConsolidationUnitsByIdentity` 当场 FAIL，随即还原。
  - 全仓：`gofmt -l` 空、`go build` / `go vet` 退 0、`go test -count=1 ./...` 绿（含真库，DSN 指
    门禁容器 55432）；`apps/admin-web` 以仓内 typescript 5.6 跑 `tsc --noEmit` 退 0。
  - 判据 4 的 `cmd/parcel-api` 装配未动：本票没有新增端点或新增依赖，`internal/architecture` 门禁绿。
