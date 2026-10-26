# 143 NR 读回整份委托聚合只为取受理基线地理维

Category: enhancement
Status: in-progress——2026-10-25 评审阻断已修、修订轮完，待评审复核与进 main，分支 `claude-dcg142` tip `7e78cb155`（修订轮末笔；本票面一笔随后）。此前 in-progress——2026-10-25 作者完工，待隔离评审与进 main，分支 `claude-dcg142` tip `ff13558ed`（代码末笔；本票与 142 的完成记录随后一笔）。此前 in-progress——2026-10-25 会话 `tops-20`（Claude Code）认领，与 142 同批，分支 `claude-dcg142`（worktree `idp-parcel-dcg142`）。此前 ready-for-agent——2026-10-20 代决复核立（上线后尽快），见 [owner-review-20261020](../../owner-review-20261020/report.md)「应修订 → 去向」与「须你裁定 → 已代决」
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
- **没动**：`docs/domain/**`、`docs/application/**`、迁移（PS 迁移末号仍是 0029，本票不取号）。（修订轮补注：`docs/domain/parcel-shipment/CONTEXT.md`「地址要素」词条末句已在 `7e78cb155` 按评审改成现行规则，见「修订轮」。）

### 代决

1. **要新 ADR，取号 0301。** ADR-0075 决定一的部分停用早已落成文：ADR-0215 Consequences 写了，ADR-0075 Status 行注记也挂了。本票不改它的范围与理由。被本票改掉的是 ADR-0215 决定二里三句：读委托聚合、整份读回、「小包托运不为此新开端口」。改已接受 ADR 的句子，照 AGENTS.md「改文档」要用新 ADR 部分停用。ADR-0075 与 ADR-0279 判为无条款停用，理由写在 ADR-0301「对已接受记录的作用」。
2. **读口实现不过重建门，只取快照两段。** 复用 `findAcceptedRequestCoveringParcel`（整份读回再取投影）更省事，但聚合照旧整份进 NR 所在的进程，R6 就落空了。
3. **读口连判断键引用与接受时刻一起交**，不只交投影：判断键各维、ADR-0279 的接受时刻与投影须出自同一份基线；分两口读，「两次读之间委托在动」的窗口又回来。
4. **新哨兵 `ErrAcceptanceBaselineInconsistent` 放在 PS 领域。** 三格自相矛盾原先由 NR 适配器在整份聚合上判出、响亮失败。判法挪进 PS 读口之后，NR 靠这个哨兵分「响亮失败」与「重投」，各格落法与之前相同。原先「反查与聚合指着两份委托」那一格随单条 SQL 消失；「反查命中而聚合读不回」那一格同样消失。（修订轮补注：「各格落法与之前相同」只在适配器层成立。端到端看，旧路先撞重建门：无基线、无时刻、版本不在这三格当时落 `ErrAcceptedParcelUnreadable`、重投到放弃；新路由 PS 读口答矛盾，NR 响亮失败、不进哨兵。修订轮又把基线挂历史版本、成员收窄、时刻与决定不一三形也收进这一格，同样由重投变为响亮失败。方向合 UC-NR-003 `AT-NR-067`。）
5. **NR 适配器直接收 PS 公开的窄读口**，不在适配器里另声明接口。先例：同为 NR 适配器的 `adapters/parcelpricing` `DeclaredParcelFacts` 收 `psports.DeclaredMeasurementView`。`AcceptedRequestSource` 初始路由还在用，留着。
6. **原读法留着**：`FindCurrentAcceptedByParcel` 与 `FindBySourceIdentity` 都还有别的调用方。初始路由仍整份读回聚合，不在 R6 范围（ADR-0301 越权风险点 3）。
7. **`docs/domain` 不改。** NR CONTEXT、CONTEXT-MAP 与 PS CONTEXT 写的都是「消费侧适配器按（租户，包裹）取受理基线那一版提交的同一份投影、只取用地理维」，读口换了照旧成立。PS CONTEXT「地址要素」词条末句的主语是 NR 适配器「按（租户，包裹）反查当前已接受委托」；反查此后在 PS 读口里做，措辞可以收紧，但不改也不错。要不要改由协调方定，候选措辞报在作者汇报里。（修订轮补注：评审采纳改 PS CONTEXT 那一句，已在 `7e78cb155` 落；NR CONTEXT、CONTEXT-MAP、GLOSSARY 仍未动。）
8. 真库用例里有两格夹具直写快照造出（基线所指版本不是当前版本、接受基线收窄成部分成员）。领域不让已接受委托造出这两格；造它们是为了证 SQL 认的是基线、不是当前版本或投影列。

### 越权风险点

ADR-0301 文末四条：读口不过重建门；SQL 按名取快照的几个键；初始路由一路仍读整份聚合；坏行仍按读不动重投。此处不复述。另一条：

5. **票面与 R6 的说法要读准**（协调方知会）。票面写「NR 进程不再加载收件人身份与地址本体」，而快照今天本就不带这两样（ADR-0301 Context），本票收窄的是聚合其余部分。派发进程里初始路由一路仍整份读回，所以「NR 进程不再加载整份委托聚合」只对交接与收寄采用两路复核成立。

### 完成判据 → 用例

| 判据（R6 与票面） | 用例 |
|---|---|
| PS 开「受理基线地理投影」端口，只读受理基线的地理维 | PS 真库：`TestTheBaselineGeoProjectionIsWhatTheWholeAggregateWouldGive`（两成员各问一次，答案与在整份聚合上取的逐格相同）、`TestAnAdoptedAmendmentAfterAcceptanceDoesNotMoveTheBaselineGeo`、`TestABaselineFixedOnAPriorVersionIsInconsistent`（修订轮由 `TestTheBaselineVersionIsReadEvenWhenItIsNotTheCurrentVersion` 翻转改名）、`TestABaselineVersionWithoutElementsStillCarriesAProjection`、`TestAnObjectThatIsNotACurrentAcceptedMemberHasNoBaselineGeo`（从未声明、他租户、投影列有而基线没有）、`TestTwoAcceptedRequestsClaimingOneParcelAreAmbiguousForTheBaselineGeo`、`TestAnInconsistentAcceptanceBaselineIsNamed`（三格；修订轮扩到六形）、`TestTheBaselineGeoLookupRequiresTenantAndParcel`；PS 领域：`TestAnAcceptanceBaselineGeoProjectionNeedsEveryPart` |
| NR 不再为取地理维加载整份委托聚合 | PS 真库 `TestTheProjectionDoesNotReadTheRestOfTheAggregate`：聚合别处坏到仓储读不回，读口照答。NR 适配器构造只收这一读口（`TestTheGeoSourceNeedsThePSView`），类型上已拿不到聚合 |
| NR 适配器只换协作者，领域与判断不变 | NR 适配器：`TestAnAcceptedParcelAnswersItsJudgmentKeyAndTheBaselineVersionsGeo`（AT-NR-064、087 的取数半边，投影摘要不变）、`TestABaselineWithoutAddressElementsStillCarriesAnEmptyProjection`、`TestAnObjectThatIsNotACurrentAcceptedParcelAnswersNotFound`（AT-NR-066）、`TestAnUnreadableShipmentAnswerIsRetryableAndAContradictionIsLoud`（AT-NR-067）。两路真库端到端照旧绿：`cmd/parcel-dispatch` 的 `reassess_on_handover_test.go`、`reassess_business_time_test.go` 等走生产装配的用例 |

### 自验（实测于 `ff13558ed`）

- `go build ./...`、`go vet ./...` 全仓过；新写与改动的 `.go` 都已 `gofmt -w`，`gofmt -l` 无输出。
- 带 DSN（`127.0.0.1:55432`，含 PG）`go test -count=1 -p 4`：`./internal/networkrouting/...`、`./internal/parcelshipment/...`、`./cmd/parcel-api/`、`./cmd/parcel-commercial/`、`./cmd/parcel-dispatch/`、`./cmd/parcel-frontline-import/`、`./cmd/parcel-network-register/`、`./cmd/parcel-settlement-register/`、`./internal/architecture/...`、`./migrations/...`，40 个包全 ok、退 0。真库包确实跑了：PS `adapters/postgres` 15.5 秒，NR `adapters/postgres` 7.5 秒，`cmd/parcel-dispatch` 33.5 秒，`cmd/parcel-api` 24.6 秒。
- `go run ./tools/adr-index -check` 只报 README 生成物与文件不一致（预期，留给推送方）。
- 全量留给推送方。

## 评审记录

以下照抄协调方转来的 `review-142-143.md`（二级标题降为三级），142、143 两票同一份。

评审 ← tops-20 隔离子代理 · 钉 `6fc41e9ad` · 23:48

结论：有阻断 1 条（Spec 轴，143 新读口）；142 无发现。

### Standards
阻断：无。
非阻断：
1. PS CONTEXT「地址要素」末句：不改也不与 ADR-0301 相抵（按租户+包裹、取基线那一版、同一份投影仍成立），但 AGENTS「改文档」要求「同一笔把 CONTEXT 里被停的句子改成现行规则」；ADR-0301 Consequences 转述成「适配器…取…」，漏了 PS CONTEXT 原有的「反查当前已接受委托」。若改，在本分支落，并同改 Consequences 里「CONTEXT…不改」那句（合入后 ADR 正文冻结）。
2. ADR-0279 候选「在 PS 委托仓储或 PS 读口上加一法取接受时刻」的否决前提「NR 已有的 PS 读口…就交得出它」已被 ADR-0301 换掉；ADR-0301 正文交代了但不是逐字引。可照 ADR-0008 先例在 ADR-0279 注记挂「理由限定」；不挂不算相抵。
无发现：红线（NR 只经 `psports.AcceptanceBaselineGeoProjectionView` 读，SQL 在 PS adapters/postgres，architecture 探针绿）；ADR-0215 正文未动、三句引文逐字在决定二；去向/来向分写合 adr/README；adr-index -check 只报生成物；注释中文无行号。

### Spec
阻断：
1. `ShipmentRequests.LoadAcceptanceBaselineGeoProjection` 把重建门会拒的委托当好答案交出。下面三形 `FindBySourceIdentity` 都拒：
   - 基线挂在历史版本：found=true，交出历史版的投影。
   - 基线时刻 ≠ 决定时刻：found=true，`AcceptedAt` 是错值，ADR-0279 的业务时间下限随之错。
   - 成员收窄：被剔的成员答 found=false。
   旧路先撞重建门，落 `ErrAcceptedParcelUnreadable` 重投至放弃，看得见；新路静默作答。PS CONTEXT「已提交 → 已接受」把基线固定在当前提交版本，第一形自相矛盾，UC-NR-003 `AT-NR-067` 要求响亮失败；`TestTheBaselineVersionIsReadEvenWhenItIsNotTheCurrentVersion` 反把这一形钉成正确答案；ADR-0301 越权风险点 1「本口守它读的两段上」的自相矛盾与事实不符。
   修法：基线版本 ≠ `currentVersion.versionId` 时答 `ErrAcceptanceBaselineInconsistent`，翻转那个用例；时刻、成员两条至少写进越权风险点 1。
非阻断：无基线、无时刻、版本不在三格，端到端由「重投」变为「响亮失败」，方向合 `AT-NR-067`；但票面代决 4「各格落法与之前相同」只在适配器层成立，宜注一句。
无发现：判断顺序与旧适配器逐格相同；键名漂移不会静默答错；分格与两路哨兵名单不变；ADR-0301 如实写明初始路由一路仍读整份聚合；142 两条变异（多格连接、`@`→`:`）都让用例变红，代决 1 只守一半不与 R4 相抵；无票面外改动。

修订轮重跑探针：在检出根目录带 DSN 跑
`go test -count=1 -v -overlay <o.json> -run TestReviewProbeGateVersusView ./internal/parcelshipment/adapters/postgres/`
探针源：/tmp/claude-1000/-home-tops/916d5bf9-1a44-4ef9-92dc-171db91f1686/scratchpad/probe143/probe_gate_vs_view_test.go；overlay：同目录 o.json（Replace 键写死评审检出路径，换树要改）。

## 修订轮（2026-10-25，同一作者子代理，同分支）

### 各条怎么收

| 评审条目 | 收法 | 提交 |
|---|---|---|
| Spec 阻断 1：读口把重建门会拒的委托当好答案交出 | 读口照重建门（`acceptedProductsValidForRehydration`）里关于接受基线的各条自判，先后相同：基线在、指当前提交版本、有固定时刻、成员与当前提交版本一致、时刻等于接受决定时刻；哪条不成立都答 `ErrAcceptanceBaselineInconsistent`。评审点名的三形都判得出，没有判不出的。为此 SQL 多取当前提交版本的标识与成员、接受决定时刻三格，聚合其余内容仍不取；基线既须指当前版本，去掉了在历史版本里找那一段。先判整份委托的基线，再看问的这件在不在基线里 | `1a552ad1c` |
| 同上：翻转用例、每形补真库用例 | `TestTheBaselineVersionIsReadEvenWhenItIsNotTheCurrentVersion` 翻转改名为 `TestABaselineFixedOnAPriorVersionIsInconsistent`。`TestAnInconsistentAcceptanceBaselineIsNamed` 扩到六形：没有基线、不指当前版本、没有固定时刻、成员收窄、时刻与决定不一、决定没有时刻。每形先证 `FindBySourceIdentity` 确实拒，再证两个成员都答矛盾。「投影列声明了而基线里没有它」改为只改投影列造出——基线与当前版本成员一致之后，只剩这一种能落「不是」 | `1a552ad1c` |
| 同上：ADR-0301 越权风险点 1 不实 | 改写成「只判重建门里关于接受基线的各条」，并列出不判的其余各条（接受决定是否为接受、商业依据与预计承诺、判断任务、资料版本、撤回）。决定一答格、决定二与注记随之改，风险点 2 的键名补全 | `7e78cb155` |
| Standards 非阻断 1：PS CONTEXT 那一句 | 改成现行规则：`network-routing` 适配器经本上下文的受理基线地理投影读口取，反查在读口里做，只交投影、引用与接受时刻，接受基线自相矛盾时如实答矛盾。ADR-0301 Consequences 原写「CONTEXT…不改」，改为写明 PS CONTEXT 原句（主语是 NR 适配器、反查也是它做）与改法；NR CONTEXT、CONTEXT-MAP、UC-NR-003 不改，理由照实写。NR CONTEXT、CONTEXT-MAP、GLOSSARY 未动 | `7e78cb155` |
| Standards 非阻断 2：ADR-0279 理由限定 | 挂了：照 ADR-0008 先例，在 ADR-0279 Status 行注记加「理由限定」，引其候选那一条的未采纳理由原句，Links 加前向指针，正文不动。ADR-0301 前段写去向，「对已接受记录的作用」改为「理由限定」一条 | `7e78cb155` |
| Spec 非阻断：代决 4「各格落法与之前相同」 | 已在代决 4 末尾补注：只在适配器层成立；端到端三格（修订后连同新收进的三形）由重投变为响亮失败 | 本票面一笔 |

变异检查：新加的三条判法——成员一致、时刻等于决定时刻、指当前版本——逐条改成恒放行，各自对应的子用例都红，改回即过。

ADR 索引仍没重生成。本地试跑 `go run ./tools/adr-index` 能生成：0215、0279 两行注记变化，加 0301 一行；跑完已复原。

### 重跑

实测于修订后的 tip `7e78cb155`（含 PG，DSN `127.0.0.1:55432`）：

- **评审探针**：探针源原样不动，另起 overlay `o-dcg142.json`，Replace 键换成本树路径，`-overlay … -run TestReviewProbeGateVersusView ./internal/parcelshipment/adapters/postgres/`，ok。三形 × 两成员共六行，每行都是：`FindBySourceIdentity` 拒（`invalid rehydrated shipment request`），读口 `found=false`，错误包 `ErrAcceptanceBaselineInconsistent`，原因依次为
  - 基线在历史版本：「baseline is fixed on version "version-1", not on the current submission version」；
  - 成员收窄：「baseline members differ from the current submission version」；
  - 时刻不一：「baseline fixed time differs from the acceptance decision time」。
  
  没有一行交出投影；修订前三形分别答 found=true、found=true、parcel-2 found=false。
- `go build ./...` 与 `go vet ./...` 全仓通过；改动的 `.go` 都已 `gofmt -w`，`gofmt -l` 无输出。
- 带 DSN `go test -count=1 -p 4 ./internal/parcelshipment/... ./internal/networkrouting/... ./cmd/parcel-dispatch/... ./internal/architecture/...`：34 个包全 ok，退 0。真库包确实跑了：PS `adapters/postgres` 16.5 秒，NR `adapters/postgres` 7.9 秒，`cmd/parcel-dispatch` 39.4 秒，`architecture` 8.4 秒。
- 全量留给推送方。
