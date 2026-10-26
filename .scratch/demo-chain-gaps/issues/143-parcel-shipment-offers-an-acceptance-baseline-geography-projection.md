# 143 NR 读回整份委托聚合只为取受理基线地理维

Category: enhancement
Status: in-progress——2026-10-25 作者完工，待隔离评审与进 main，分支 `claude-dcg142` tip `ff13558ed`（代码末笔；本票与 142 的完成记录随后一笔）。此前 in-progress——2026-10-25 会话 `tops-20`（Claude Code）认领，与 142 同批，分支 `claude-dcg142`（worktree `idp-parcel-dcg142`）。此前 ready-for-agent——2026-10-20 代决复核立（上线后尽快），见 [owner-review-20261020](../../owner-review-20261020/report.md)「应修订 → 去向」与「须你裁定 → 已代决」
Blocked by: 无
出处：用户 2026-10-20「要我裁定的部分，你作为业务和系统专家，请你自决吧」；本会话代决复核报告 [owner-review-20261020](../../owner-review-20261020/report.md) 的 R6；ADR-0215 越权风险点 1

## 现象与修法

现象、证据与修法的完整写法以复核报告 R6 一条为准（单一权威），此处只记要点：

代决「是，作为过渡」：PS 开「受理基线地理投影」端口，NR 进程不再加载收件人身份与地址本体；NR 适配器只换协作者。

## 完成记录（2026-10-25，Claude Code 作者子代理，协调方 `tops-20`；分支 `claude-dcg142`，worktree `idp-parcel-dcg142`，起点 `origin/main` `f8b25837b`）

授权：用户 2026-10-20 原话「要我裁定的部分，你作为业务和系统专家，请你自决吧」。裁定的单一权威是复核报告 R6；落地的决定写成 [ADR-0301](../../../docs/adr/0301-parcel-shipment-offers-an-acceptance-baseline-geo-projection-and-reassessment-no-longer-reads-the-whole-request.md)，本节只记落地事实与 ADR 之外的代决，不复述 ADR。

### 提交

| SHA | 内容 |
|---|---|
| `9c2e171df` | docs(adr)：ADR-0301；ADR-0215 的 Status 行注记与 Links 前向指针 |
| `ff13558ed` | feat(parcel-shipment,network-routing)：PS 读口、NR 适配器换协作者、parcel-dispatch 两处装配 |
| 随后一笔 | docs(scratch)：142、143 完成记录 |

### 找到了什么（取证于 `f8b25837b`）

- **NR 今天怎样读 PS**：`internal/networkrouting/adapters/parcelshipment` 的 `AcceptedCarriedGeoSource` 收两个协作者，都由 `*pspostgres.ShipmentRequests` 满足：
  - `FindCurrentAcceptedByParcel`：只读投影列；
  - `FindBySourceIdentity`：整份快照读回、过重建门。
- 装配两处，都在 `cmd/parcel-dispatch`：交接一路 `reassessOnHandoverConsumerOver`，收寄采用一路 `networkIntakeConsumer`。收寄采用一路只用它取接受时刻与判断键各维（ADR-0279），投影随信封来。
- 快照不带地址文本与收件人身份（寄收件条目只进载荷摘要）。整份读回时 NR 经手的是申报测量、接受决定与商业依据、结算条款、客户包裹件号与接受后的资料修订。
- 初始路由（`RouteOnAcceptanceAdapter`）另按来源身份整份读回聚合，取接受基线与接受决定，不取地理维。

### 做了什么

- **PS**（全是新文件）：
  - 领域 `acceptance_baseline_geo_projection.go`：值 `AcceptanceBaselineGeoProjection`（委托与客户账户引用、基线所指那一版、基线固定时刻、那一版的地理投影），哨兵 `ErrAcceptanceBaselineInconsistent` 与 `ErrInvalidAcceptanceBaselineGeoProjection`。
  - 端口 `ports/acceptance_baseline_geo_projection_view.go`：`AcceptanceBaselineGeoProjectionView`。
  - 实现 `adapters/postgres/acceptance_baseline_geo_projection_view.go`：`ShipmentRequests.LoadAcceptanceBaselineGeoProjection`，一条 SQL 只取快照的接受基线与基线那一版的地址要素子段（ADR-0301 决定二）。
  - 用例：领域一个，真库八个。
- **NR**：`accepted_carried_geo.go` 的 `AcceptedCarriedGeoSource` 构造只收 `psports.AcceptanceBaselineGeoProjectionView`；判断键、接受时刻与投影的取值逐格不变；删掉只为旧读法存在的 `CurrentAcceptedParcelLookup` 与 `baselineCovers`。`accepted_carried_geo_test.go` 改用 PS 读口的替身。NR 领域、复核编排与未决哨兵名单没动。
- **`cmd/` 装配**：`cmd/parcel-dispatch/assemble.go`（收寄采用一路）与 `assemble_reassess_on_handover.go`（交接一路）各一行改接；`reassess_business_time_test.go` 夹具同改。
- **ADR**：新 ADR-0301；ADR-0215 只加 Status 行注记与 Links 前向指针，正文不改。ADR 索引没重生成，留给推送方；本地试跑过 `go run ./tools/adr-index`，生成成功、只多出 0301 一行与 0215 那一行的注记，随即复原。
- **没动**：`docs/domain/**`、`docs/application/**`、迁移（PS 迁移末号仍是 0029，本票不取号）。

### 代决

1. **要新 ADR，取号 0301。** ADR-0075 决定一的部分停用早已落成文：ADR-0215 Consequences 写了，ADR-0075 Status 行注记也挂了。本票不改它的范围与理由。被本票改掉的是 ADR-0215 决定二里三句：读委托聚合、整份读回、「小包托运不为此新开端口」。改已接受 ADR 的句子，照 AGENTS.md「改文档」要用新 ADR 部分停用。ADR-0075 与 ADR-0279 判为无条款停用，理由写在 ADR-0301「对已接受记录的作用」。
2. **读口实现不过重建门，只取快照两段。** 复用 `findAcceptedRequestCoveringParcel`（整份读回再取投影）更省事，但聚合照旧整份进 NR 所在的进程，R6 就落空了。
3. **读口连判断键引用与接受时刻一起交**，不只交投影：判断键各维、ADR-0279 的接受时刻与投影须出自同一份基线；分两口读，「两次读之间委托在动」的窗口又回来。
4. **新哨兵 `ErrAcceptanceBaselineInconsistent` 放在 PS 领域。** 三格自相矛盾原先由 NR 适配器在整份聚合上判出、响亮失败。判法挪进 PS 读口之后，NR 靠这个哨兵分「响亮失败」与「重投」，各格落法与之前相同。原先「反查与聚合指着两份委托」那一格随单条 SQL 消失；「反查命中而聚合读不回」那一格同样消失。
5. **NR 适配器直接收 PS 公开的窄读口**，不在适配器里另声明接口。先例：同为 NR 适配器的 `adapters/parcelpricing` `DeclaredParcelFacts` 收 `psports.DeclaredMeasurementView`。`AcceptedRequestSource` 初始路由还在用，留着。
6. **原读法留着**：`FindCurrentAcceptedByParcel` 与 `FindBySourceIdentity` 都还有别的调用方。初始路由仍整份读回聚合，不在 R6 范围（ADR-0301 越权风险点 3）。
7. **`docs/domain` 不改。** NR CONTEXT、CONTEXT-MAP 与 PS CONTEXT 写的都是「消费侧适配器按（租户，包裹）取受理基线那一版提交的同一份投影、只取用地理维」，读口换了照旧成立。PS CONTEXT「地址要素」词条末句的主语是 NR 适配器「按（租户，包裹）反查当前已接受委托」；反查此后在 PS 读口里做，措辞可以收紧，但不改也不错。要不要改由协调方定，候选措辞报在作者汇报里。
8. 真库用例里有两格夹具直写快照造出（基线所指版本不是当前版本、接受基线收窄成部分成员）。领域不让已接受委托造出这两格；造它们是为了证 SQL 认的是基线、不是当前版本或投影列。

### 越权风险点

ADR-0301 文末四条：读口不过重建门；SQL 按名取快照的几个键；初始路由一路仍读整份聚合；坏行仍按读不动重投。此处不复述。另一条：

5. **票面与 R6 的说法要读准**（协调方知会）。票面写「NR 进程不再加载收件人身份与地址本体」，而快照今天本就不带这两样（ADR-0301 Context），本票收窄的是聚合其余部分。派发进程里初始路由一路仍整份读回，所以「NR 进程不再加载整份委托聚合」只对交接与收寄采用两路复核成立。

### 完成判据 → 用例

| 判据（R6 与票面） | 用例 |
|---|---|
| PS 开「受理基线地理投影」端口，只读受理基线的地理维 | PS 真库：`TestTheBaselineGeoProjectionIsWhatTheWholeAggregateWouldGive`（两成员各问一次，答案与在整份聚合上取的逐格相同）、`TestAnAdoptedAmendmentAfterAcceptanceDoesNotMoveTheBaselineGeo`、`TestTheBaselineVersionIsReadEvenWhenItIsNotTheCurrentVersion`、`TestABaselineVersionWithoutElementsStillCarriesAProjection`、`TestAnObjectThatIsNotACurrentAcceptedMemberHasNoBaselineGeo`（从未声明、他租户、投影列有而基线没有）、`TestTwoAcceptedRequestsClaimingOneParcelAreAmbiguousForTheBaselineGeo`、`TestAnInconsistentAcceptanceBaselineIsNamed`（三格）、`TestTheBaselineGeoLookupRequiresTenantAndParcel`；PS 领域：`TestAnAcceptanceBaselineGeoProjectionNeedsEveryPart` |
| NR 不再为取地理维加载整份委托聚合 | PS 真库 `TestTheProjectionDoesNotReadTheRestOfTheAggregate`：聚合别处坏到仓储读不回，读口照答。NR 适配器构造只收这一读口（`TestTheGeoSourceNeedsThePSView`），类型上已拿不到聚合 |
| NR 适配器只换协作者，领域与判断不变 | NR 适配器：`TestAnAcceptedParcelAnswersItsJudgmentKeyAndTheBaselineVersionsGeo`（AT-NR-064、087 的取数半边，投影摘要不变）、`TestABaselineWithoutAddressElementsStillCarriesAnEmptyProjection`、`TestAnObjectThatIsNotACurrentAcceptedParcelAnswersNotFound`（AT-NR-066）、`TestAnUnreadableShipmentAnswerIsRetryableAndAContradictionIsLoud`（AT-NR-067）。两路真库端到端照旧绿：`cmd/parcel-dispatch` 的 `reassess_on_handover_test.go`、`reassess_business_time_test.go` 等走生产装配的用例 |

### 自验（实测于 `ff13558ed`）

- `go build ./...`、`go vet ./...` 全仓过；新写与改动的 `.go` 都已 `gofmt -w`，`gofmt -l` 无输出。
- 带 DSN（`127.0.0.1:55432`，含 PG）`go test -count=1 -p 4`：`./internal/networkrouting/...`、`./internal/parcelshipment/...`、`./cmd/parcel-api/`、`./cmd/parcel-commercial/`、`./cmd/parcel-dispatch/`、`./cmd/parcel-frontline-import/`、`./cmd/parcel-network-register/`、`./cmd/parcel-settlement-register/`、`./internal/architecture/...`、`./migrations/...`，40 个包全 ok、退 0。真库包确实跑了：PS `adapters/postgres` 15.5 秒，NR `adapters/postgres` 7.5 秒，`cmd/parcel-dispatch` 33.5 秒，`cmd/parcel-api` 24.6 秒。
- `go run ./tools/adr-index -check` 只报 README 生成物与文件不一致（预期，留给推送方）。
- 全量留给推送方。
