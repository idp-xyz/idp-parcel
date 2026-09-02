# 轨迹源接收缝盘点

Category: chore
Status: draft——只读盘点，取证完成待立票

取证基线 `main` = `9e6d53a`（2026-09-02 取证；该时点 `origin/main` = `398a148`，本文引的代码行两者一致）。断言一律锚到文件与符号名；本文写证据不写结论。**凡出现计数的地方，数本身是论点，一律锚在上述 SHA**。

**不做什么**：不写产品代码，不改 `docs/**`，不改任何既有票面。每段末尾的「票粒度」是建议不是票面。不填任何实例取值——渠道账号、承运商字段名、轮询频率、里程碑目录内容一个都不出现（[ADR-0088](../../docs/adr/0088-label-channel-service-enters-the-first-release-service-forms.md) Decision 五）。不新造也不改写领域语言（本目录 `spec.md`「边界」节）。不打开 `docs/wooolink/` 或客户文件。

盘的链条：**外部轨迹源（17track 聚合、承运商直连）拉取/接收 → 事实 → 里程碑映射 → 内部投影 / 客户可见**。面单交易本体与取面单那一段不在本文，见[末端面单渠道能力形状盘点](./capability-shape-inventory.md)。

本文要回答的核心问题由 ADR-0088 Consequences 提出：「末端渠道轨迹回传的事实收编（按渠道适配缝备忘：先由事实所有者收编，不直插投影）」——这句话里有两件事，**一件今天已经守住，另一件整段缺席**，见第一、二段。

## 一、拉取/接收：外部轨迹源的入站缝

### 已有形状

**无。** 本仓今天没有任何从外部系统取数据的代码。

### 缺口（零命中即结论，故逐条写明搜法）

- **外部轨迹源字面零命中。** 按 `17[Tt]rack|17TRACK|aftership|AfterShip|trackingmore|[Ww]ebhook` 扫全仓，命中全部落在 `docs/` 与 `.scratch/` 下的文档（PILOT-SCOPE、参数登记册、PRD、竞品事实台账、若干 `UC-CC-*`、本目录 `spec.md` 与另一份盘点等），**`internal/`、`cmd/`、`migrations/`、`apps/` 四处一个都没有**。
- **出向 HTTP 调用零命中。** 按 `http\.Client|http\.Get|http\.Post|http\.NewRequest|http\.DefaultClient` 扫全仓 `*.go`，**零匹配**。本仓的 `net/http` 用法全部在服务端一侧（各上下文 `adapters/http/` 的处理器与 `cmd/parcel-api`）。也就是说不只是「没有轨迹源客户端」，而是**没有任何出向 HTTP 客户端**，连一个可供照抄的先例都没有。
- **朝外的拉取端口不存在。** 按 `^type \w*(Gateway|Client|Provider|Carrier|Courier|Source|Feed|Puller|Fetcher|Subscriber|Channel)\w* interface` 扫 `internal/`，命中分三类，没有一类是「向外部轨迹源取数据」：
  - 大量 `*Source` 接口全部在各上下文的 `adapters/<邻接上下文>/` 下（如 `parcelshipment/adapters/partycommercial/` 的 `ResolutionKeySource`、`IntakeContentSource`，`networkrouting/adapters/parcelshipment/` 的 `AcceptedRequestSource`）——它们是**防腐层朝内的入参口**，读的是兄弟上下文已有的答案，不出进程。
  - `visibilityexception/ports.NotificationChannelGateway`——朝外，但方向是**发通知**不是取轨迹，且其文档注释自称「今天没有实现，唯一实现是测试替身」。
  - `accessidentity.ChannelRegistry` 与 `ChannelCredentialProof`——**入向**接入渠道（谁能调我），不是出向。
- **inbox 机制承接的是内部上下文之间的事件，不是外部入站。** `internal/platform/inboxconsume/consume.go` 与各上下文 `adapters/inbox/` 的消费者配对的是 `outboxintent` 写出的信封；VE 侧 `adapters/inbox/` 下十一个 `*_consumer.go`（实测于 `9e6d53a`）逐个对应一个**兄弟上下文**的事件，无一来自进程外的第三方系统。

### 半边归属

拉取/接收的**形态选择本身**（轮询拉取 vs 回调接收 vs 两者都要）、出向端口形状、凭证与重试的表达位置——**机制半边**。轮询频率、账号、密钥、各源的字段名——实例半边（`PAR-INT-02`）。

需要点明的一格：**这一段与另一份盘点第二段（取面单出向端口）是同一类新东西**——本仓至今没有任何出向集成，两者会同时撞上「出向调用的失败、超时、重试与幂等在本仓怎么表达」这个尚无先例的问题。两票若各答一次，答案会分叉。

### 建议的票粒度

先一票只定「本仓的出向集成缝长什么样」（失败/超时/重试/幂等的表达，不接任何真实源），取面单与轨迹两处共用；其后轨迹源的拉取或接收端口一票。**不要在轨迹票里顺手发明出向调用的通用形状**——那会让取面单票拿到一份没参与讨论的既成事实。

## 二、事实：由所有者收编为「已接受源事实」

### 已有形状

**事实模型齐备且严谨。** `internal/visibilityexception/domain/tracking_projection.go`：

- `AcceptedSourceFact` 与它的构造输入 `AcceptedSourceFactSpec`，经 `NewAcceptedSourceFact` 逐项过门。
- **三个时间分立**：`OccurredAt`（业务发生）、`EffectiveAt`（有效）、`ReceivedAt`（接收），构造门要求三者皆非零。类型注释写明合并成一个时间字段就再也分不出「什么时候发生」与「什么时候才知道」——迟到轨迹与更正轨迹正是踩这一格。
- **取代关系只登记不裁决**：`Supersedes` 由源上下文随更正给出，`Supersedes()` 第二个返回值区分首登与更正；自指名前身（`Supersedes == Version`）被构造门拒绝，注释点明「沿用原版本号就是覆盖，不是更正」。
- `SourceFactKind` **刻意不封闭**（`requiredValue` 包装的引用），注释写明映射按「源上下文 + 事实类型」版本化登记、不得按单条事实引用建目录。

**收编的边界在类型上就守住了。** `SourceContext` 是封闭五值：`SourceParcelShipment`、`SourceNetworkRouting`、`SourceNodeOperations`、`SourceTransportFulfillment`、`SourceCustomsCompliance`。该类型注释的原话是「投影只消费这五处的事实——**来源消息、原始扫描或外部状态码未经业务所有者接受，在类型上就没有入口**」。持久化侧同形：`adapters/postgres/accepted_fact.go` 的译码只认这五值，集外取值上抛；登记侧同形：`adapters/registrationjson/translate.go` 的 `sourceContextFromToken` 逐词比对这五值，未知词交错。

**九个事实译装器在册**（实测于 `9e6d53a`，`adapters/*/derive_on_*.go` 非测试文件）：`customscompliance/derive_on_customs_case.go`、`customscompliance/derive_on_declaration_submission.go`、`networkrouting/derive_on_initial_route.go`、`nodeoperations/derive_on_node_intake.go`、`parcelshipment/derive_on_final_outcome.go`、`transportfulfillment/derive_on_effective_delivery.go`、`transportfulfillment/derive_on_exception_journey.go`、`transportfulfillment/derive_on_offsite_pickup.go`、`transportfulfillment/derive_on_transport_handover.go`。每个都把某兄弟上下文已接受的事实译成 `AcceptedSourceFactSpec` 并填上自己那一格 `Source`。

**存储与代数**：`ports.AcceptedFactStore`（`FindByParcel` 交回该租户该包裹全部已接受事实，是投影派生的输入；事实只增不删，来源更正是新版本新键），写入代数 `FactSaved` / `FactAlreadyRecorded`。

### 缺口

- **「不直插投影」这条 ADR-0088 的要求今天已经成立，且是编译期成立**，不是缺口——`TrackingProjection` 的条目类型是 `MilestoneClassification`，而它只能由 `AcceptedSourceFact` 构造（`ClassifyMilestone` / `LeaveUnclassified` 两个构造函数都以 `AcceptedSourceFact` 为首参）；`AcceptedSourceFact` 的 `Source` 又只认封闭五值。**外部数据没有绕过收编写进投影的类型路径。** 这一格记在「已有形状」而不是缺口，是本盘点最该说清的一句：ADR-0088 那句要求的**后半句已经守住，缺的是前半句的执行方**。
- **末端渠道轨迹没有任何收编方。** 按[渠道适配缝备忘](../../docs/design/channel-adapter-seams-design-note.md)的所有权表，「轨迹采集 / 状态同步」归 `transport-fulfillment`（承运/移动/交付）与 `node-operations`（节点事实），备忘原话是「外部轨迹必须先由事实所有者收编为自己的事实；`visibility-exception` 只消费已接受事实形成投影，**不得把渠道轨迹直插投影**」。而 TF 与 NO 两侧今天的事实**全部来自内部登记**：TF 的用例是 `commission_transport`、`prepare_transport_opportunity`、`register_transport_handover`、`register_offsite_pickup`、`register_effective_delivery`、`start_alternate_journey`、`accept_regulatory_disposition`，没有一个接受「外部系统报来的状态」。
- **TF 侧连在线口都只有两个。** `cmd/parcel-api/endpoints.go` 里 TF 的命令端点只有 `/transport-fulfillment/deliveries` 与 `/transport-fulfillment/delivery-proof-corrections`；交接、外场取件两类事实**没有在线登记口**。即便外部轨迹的收编形态定为「译成 TF 既有事实」，也有两类事实今天无口可进。
- **ADR-0023 的约束尚无落点。** 缝备忘那一行点名 [ADR-0023](../../docs/adr/0023-work-fact-identity-and-time-are-minted-by-the-device.md)「设备与外部事实的身份时间不代铸」。外部轨迹源给的时间戳与事件标识由谁认领、本仓能不能代铸，今天没有任何代码或票面表达；而 `AcceptedSourceFact` 要求三个时间齐备，外部源常常只给一个。**这是形状问题不是取值问题**，落在机制半边。

### 半边归属

收编方的归属（TF / NO / 是否要第三方）、外部时间戳到三时间的映射规则形状、TF 两类无口事实要不要补口、以及 ADR-0023 在外部轨迹上的落法——**机制半边**（其中归属与 ADR-0023 落法各需一次裁决）。各源的状态码表、承运商编码、时区口径——实例半边。

### 建议的票粒度

第一票只裁**收编方归属与外部时间戳的认领口径**（够不上 ADR 的话至少一条票面裁决），不写实现——这一票不结，后面每一票都会各自假设一个答案。其后：收编执行器一票（落在裁定的所有者上下文 `application/`）；TF 两类无口事实补在线口一票（若裁定需要）。

## 三、里程碑映射

### 已有形状

**登记册齐备，且已有在线登记口。**

- 条目形状 `ports.MilestoneMappingEntry`：键是（`Source` 源上下文 + `Kind` 事实类型），值是 `Milestone` 标准里程碑引用。注释引 CONTEXT 硬句「一行覆盖此后同类型事实，不得按单条事实引用建目录」。
- 版本形状 `ports.MilestoneMappingRegistration`：抬头 `CatalogVersionHeader` 加**整版条目一次写全**，注释写明不支持事后追加——「事后往已发布版本里塞条目会让『按 vN 判的未归类』这个已作出的判断在事后变成已归类」。
- 登记用例 `ports.CatalogRegistry.RegisterMilestoneMapping`，全部方法在调用方事务内执行（`RequireExecutor` 语义，注释：半版目录比没有目录更坏）。
- 归类结果 `domain.MilestoneClassification`，两个构造函数 `ClassifyMilestone` 与 `LeaveUnclassified`，**都强制携带 `MappingVersionReference`**——连「按哪套话语归不进」都说不出的未归类无从续办。
- 读口 `ports.MilestoneMappingView`：第二个返回值为 `false` 即「映射目录未配置」，注释明写那**不是未决而是整体无法可靠映射，事实按未归类进投影**（CONTEXT：不强行映射）；依赖调不通才作为错误返回。
- 写面已进端点表（票 `admin-write-faces/02` 切片 02d）：`POST /visibility-catalogue-milestone-mapping-registrations`，管理台登记签在 `apps/admin-web/src/pages/visibility/`。

### 缺口

- **没有缺口是形状上的。** 这一段是整条链上唯一四件齐全（领域、端口、持久化、在线写面）的一段。
- 唯一要点名的是**接续依赖**：映射键的 `Source` 取封闭五值、`Kind` 取源上下文拥有的事实类型词。外部轨迹一旦按第二段收编成某个所有者的事实，它的 `Kind` 取值是**新的**，映射条目要随之登记——那是实例半边（租户登记），但「新 `Kind` 从哪里来、由谁命名」是第二段收编执行器的产物。**本段不缺东西，但它的输入还没有生产者。**

### 半边归属

机制半边已完成。映射条目内容（哪个源事实类型归哪个标准里程碑）、标准里程碑目录本身——实例半边，`domain.MilestoneReference` 的注释原话即「里程碑目录及其映射属实例参数」。

### 建议的票粒度

**不立票。** 本段无缺口；真要动，动的是第二段的产物。若立票只会立出一张「等上游」的空票。

## 四、内部投影

### 已有形状

- 聚合 `domain.TrackingProjection`：`DeriveTrackingProjection` 建首版，`Rederive` 换版并指回原版（原版继续保留），`RehydrateTrackingProjection` 按版本读回历史版（ADR-0065：版本只增不改写）。
- **投影不保存第二套源事实**——类型注释原话「字段里只有引用与归类，没有任何源事实的内容拷贝」；构造门还要求全部条目属同一包裹，注释点明「混入别人的事实就是把两条轨迹拼成一条」。
- 编排 `application/derive_projection.go`：结果代数 `DeriveProjectionOutcome` 五格（`PROJECTION_DERIVED` / `EXISTING_RESULT` / `SOURCE_CONFLICT` / `UNDECIDED` / `NOT_ACCEPTED`），未决原因 `DeriveUndecidedReason` 四格（事实库 / 映射读口 / 投影库 / 身份工厂各自不可用）。
- 端口 `ports.ProjectionStore`（当前版与按版本读回分立，注释：审计问「当时形成过什么」由 `FindByVersion` 作答，重放只能答「今天会派生出什么」）、`ports.OperationsProjectionRead`（运营列面，与写侧接口刻意不合并）、`ports.ProjectionIdentityFactory`、`ports.ProjectionHandoff`（写 Outbox）。
- 读面与页面：`adapters/http/query_tracking_projections.go`、`adapters/postgres/projection.go`。

### 缺口

- **本段形状齐备，缺的仍是输入。** 投影的输入是 `AcceptedFactStore.FindByParcel`，而第二段说明末端渠道轨迹今天进不了那个库。
- 一处值得点名的既有约束：`DeriveTrackingProjection` 要求 `len(entries) != 0`——**没有任何已接受事实的包裹派生不出投影**。面单渠道服务的包裹在首发链路上若只有渠道轨迹一种事实来源，那么在第二段接通之前，这类包裹**连一个投影版本都不会有**，读面上表现为「查不到」而不是「有轨迹但为空」。这两者在页面上要不要分开说，今天没有票面表达。

### 半边归属

机制半边已完成。「无事实包裹」在读面上的呈现口径——机制半边（需一次口径确认，属另一份盘点第一段「渠道结果落在面单交易上」的邻格）。

### 建议的票粒度

**不立实现票。** 若第二段裁定后发现「无事实包裹」的呈现要改，随那一票带；单独立票会立出一张改不动的票。

## 五、客户可见

### 已有形状

- 领域 `domain/customer_view.go` 的 `CustomerTrackingView`；编排 `application/derive_customer_view.go`。
- **视图只基于当前投影形成**——`DeriveCustomerViewHandler.Handle` 的文件注释原话「本编排不读内部案件或原始来源消息——**依赖清单里根本没有那些端口**」。依赖只有四项：`ports.DisclosurePolicyView`、`ports.CustomerViewStore`、`ports.CustomerViewIdentityFactory`、`ports.CustomerViewHandoff`。
- 披露规则未配置的表达是**四维全部待确认**（`DisclosurePolicyView` 第二返回值 `false`），注释写明那不是未决而是如实空白：「不虚构可见性，也不把没人作过的披露决定说成『不展示』」；策略调不通才是未决（`DISCLOSURE_POLICY_UNAVAILABLE`），两者刻意分开。
- 视图版本化：当前版在库，历史由替代关系承担；`CustomerViewStore` 的键含租户与客户账户，注释点明按包裹一个键会让两个客户的授权范围共用一份视图。
- 客户账户反查 `ports.ParcelCustomerAccountView`，多于一个候选交具名错误、绝不按时间或行序任选。

### 缺口

- **本段形状齐备，缺的同样是输入。**
- 一处与面单渠道服务直接相关的未决：披露策略的适用范围今天按租户与客户账户展开，而面单渠道服务的客户拿到的轨迹**大部分来自末端渠道而非自营网络**。「渠道轨迹对客户的披露口径是否与自营网络事实同一套规则」，今天没有代码也没有票面表达。这是产品口径问题，不是实现缺口——**记在这里是为了让立票的人知道它存在**，不是建议现在答它。

### 半边归属

机制半边已完成。披露四维的取值、渠道轨迹的披露口径——实例半边与产品裁决。

### 建议的票粒度

**不立票。** 披露口径那一问若要答，归产品文档与 `PAR-COM-*` 登记，不归本 feature（本目录 `spec.md`「边界」节把对客承诺与领域语言改写都排除在外）。

## 六、把六段收在一句话上

ADR-0088 那句「末端渠道轨迹回传的事实收编（先由事实所有者收编，不直插投影）」拆开是两件事：

- **「不直插投影」——今天已经守住，且是编译期守住的**（第二段）。`SourceContext` 封闭五值 + `MilestoneClassification` 只能由 `AcceptedSourceFact` 构造，外部数据没有类型路径绕进投影。这一格不需要任何票。
- **「先由事实所有者收编」——整段缺席**（第一、二段）。缺的不只是一个适配器：本仓没有任何出向集成先例（出向 HTTP 全仓零命中），收编方归属未裁（缝备忘指 TF 与 NO，而面单交易在 PS），外部时间戳与 ADR-0023 的关系未表达，TF 有两类事实连在线口都没有。

**链条的下游三段（映射、投影、客户可见）形状齐备、无缺口**——它们在等一个还没有生产者的输入。这一点决定了票的次序：**先裁归属与口径，再补收编，下游三段一行不用改**。

## 汇总（缺口条目）

| 段 | 缺口 | 半边 |
|---|---|---|
| 一 | 外部轨迹源无任何入站缝；全仓无出向 HTTP 客户端，连先例都没有 | 机制 |
| 一 | 「拉取 vs 回调」形态未选；出向失败/超时/重试/幂等的表达无先例 | 机制（与取面单票共用，需先统一） |
| 二 | 末端渠道轨迹无收编方；TF/NO 两侧事实全部来自内部登记 | 机制 |
| 二 | 收编方归属未裁（缝备忘指 TF 与 NO，面单交易在 PS） | 机制（需裁决） |
| 二 | 外部时间戳到 `OccurredAt`/`EffectiveAt`/`ReceivedAt` 三时间的映射规则未表达；ADR-0023「不代铸」在外部轨迹上无落点 | 机制（需裁决） |
| 二 | TF 的交接与外场取件两类事实无在线登记口 | 机制 |
| 三 | 无缺口；映射四件齐全，等第二段产出新的事实类型词 | —— |
| 四 | 无缺口；「无已接受事实的包裹派生不出投影」在读面上的呈现口径未表达 | 机制（口径） |
| 五 | 无缺口；渠道轨迹对客披露是否与自营网络同一套规则，无表达 | 产品裁决（不在本 feature） |

**「不直插投影」不在本表**——它是已成立的机制，不是缺口。

## Comments

- 2026-09-02 MCP-4：只读盘点，基线 `9e6d53a`。未改产品代码、未改 `docs/**`、未改任何既有票面。本文的零命中结论逐条写了搜法（正则与搜索范围），可复算。**未取任何外部轨迹源的公开开发文档**——与另一份盘点第二段不同，本文回答的问题（本仓今天有没有入站缝、事实收编归谁）不需要各源的接口形态就能答完；真要选拉取还是回调、要不要按源分适配器，属第一段建议的那张裁决票，届时再取证，本文不越位替它取。立票原归 MCP-1，2026-09-02 由 MCP-3 改派本会话，见本目录 `spec.md`。
