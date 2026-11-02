# ADR-0356：客户追踪可按委托的单据号（运单号）查——经 `parcel-shipment` 按（租户，客户账户，单据号）找委托，号查无与属别的账户同答；委托已接受才逐件列当前有效包裹，每件照按包裹查的单件三格作答；另开一口，不改按包裹定位

Status: Accepted（2026-11-02，Claude Code 会话起草，用户经会话过目并同意：四项选择——另开新口、拒绝与撤回并一格、被替代的来源包裹不单列而后继带 `replaces`、任一读失败整页 500——逐项同意，其余各格（待接受不列包裹、一页 50 件、每件带回追踪答复等）未提异议、照草案；代 VE、PS owner 决（代决），用户过目不是 owner 复核。此前 Proposed——草案，同日起草；起草出处是用户 2026-11-02 裁定客户追踪支持「按运单号（委托的单据号）查」、先写 ADR。票 [document-numbering/12](../../.scratch/document-numbering/issues/12-customer-tracking-by-shipment-number.md)，出处是票 document-numbering/10「第 4 条停在哪（向协调方报）」一节的调查。部分停用 [ADR-0223](./0223-the-first-party-submission-answer-names-each-parcel-and-the-tracking-view-locates-by-it.md) 决定四一句与 [ADR-0246](./0246-the-customer-tracking-query-answers-no-visible-result-for-a-parcel-attributed-to-the-requesting-account-before-its-view-forms.md) 决定一一句，回答 ADR-0246 越权风险点 2 与 ADR-0223 越权风险点 6 的后继一半（被停原句与适用场景见两者的 Status 行）；两者正文不改，只在其 Status 行与 Links 加前向指针。越权风险点见文末，待 VE owner 复核，第 2、4 点同时待 PS owner 会签。）<!-- 索引注记 -->｜**新口**：`GET /v1/customer-shipment-tracking?documentNumber=<运单号>`，首方客户族，另立能力词 `CUSTOMER_SHIPMENT_TRACKING_QUERY`（一口一词）。**整单四格**：`PARCELS_LISTED`（委托已接受，含其后派生的已取消、已完成）、`AWAITING_ACCEPTANCE`（已提交未决定）、`CLOSED_WITHOUT_SERVICE`（已拒绝或已撤回）、`SHIPMENT_NOT_FOUND`（号查无、属别的账户、别的租户，没有单据号的委托按号也落这里，同一副字节）。**逐件**：只列当前有效包裹，被替代的来源包裹不单列、后继带 `replaces`；每件 `parcelId` 与提交答复同号，答复与同一时刻按包裹查逐字相同；游标分页，一页至多 50 件。**隔离在键上**：账户是 `parcel-shipment` 读口的查询条件，「存在但不是你的」这一位不出提供方；单据号与成员关系不复制进 `visibility-exception`
Date: 2026-11-02

## Context

取证于 main `060a6e72f`，只记 `S`。

- **客户手里的号是运单号。** 委托的单据号在客户面前叫运单号（[ADR-0355](./0355-a-document-number-is-separate-from-internal-identity-and-is-assigned-in-the-issuing-transaction-under-built-in-rules-a-tenant-may-override.md) 决定二、十一），首方提交答复已带 `documentNumber`。它是客户、收件方与租户客服之间传的那个号；`parcelId` 是 26 位随机内部标识，只交给提交方的系统。
- **今天首方客户追踪只按包裹定位。**
  - `GET /v1/customer-tracking-view?parcel=`，查询串只认 `parcel` 一格（`CustomerTrackingViewIntake`），是 [ADR-0223](./0223-the-first-party-submission-answer-names-each-parcel-and-the-tracking-view-locates-by-it.md) 决定三承诺的查询标识。
  - 答复是单件的封闭三格：`CURRENT_VIEW` / `NO_VISIBLE_RESULT` / `VIEW_NOT_FOUND`（[ADR-0246](./0246-the-customer-tracking-query-answers-no-visible-result-for-a-parcel-attributed-to-the-requesting-account-before-its-view-forms.md) 决定一）。
  - [UC-VE-008](../application/visibility-exception/UC-VE-008-PROVIDE-CUSTOMER-END-TO-END-TRACKING-VIEW.md)「查询标识」一行不含委托的单据号。
- **挡路的是已接受记录里的取舍，不是机制。**
  - ADR-0223 决定四：「不另开按委托号读回逐件号的首方读面。」它的 Alternatives 否决了这条路，理由是「号在答复形成时已在，多一个读口是同一事实的第二条路；答复丢失靠原样重发拿回」。按运单号查、答这单下的包裹，答复必然逐件带出包裹号，正是这条路。
  - ADR-0246 决定一：「客户追踪查询的业务结果是三格。」一单多件要另一种答复形。
  - ADR-0246 决定五与越权风险点 2：首方客户今天没有别的口读得到委托是否已接受，`NO_VISIBLE_RESULT` 是唯一一条路；「PS 将来若给首方客户开读委托接受状态的口，两处语义得对齐」。
- **ADR-0223 的否决理由管的是另一种需要。** 它要解决的是提交答复丢失后怎么取回号，原样重发就够。本记录要解决的是：只持有运单号的一方怎么查到这单下的包裹。例如客户的客服、只记了运单号的客户系统、收件方转来的询问。还有拆分合并后的后继包裹：它们的号从来没有答复可带（ADR-0223 越权风险点 6「拆分合并不是提交，没有答复可带」）。
- **机制上做得到**（票 document-numbering/10「第 4 条停在哪（向协调方报）」一节的调查）：
  - `visibility-exception` 已有问 `parcel-shipment` 的消费侧适配器（`adapters/parcelshipment` 的 `ParcelCustomerAccountLookup`，[ADR-0025](./0025-cross-context-adapters-live-on-the-consumer-side.md)）。
  - 委托上存着单据号与无号原因（`domain.ShipmentDocumentNumbering`，逐字保存、不做大小写或空白折算）。（租户，类型代码，单据号）唯一（ADR-0355 决定八）。
  - 操作者族的委托查阅已能按 `?documentNumber=` 查，作用域是隔离读注入的可见账户。
- **号可猜。** 委托的默认模板 `P{yyyy}{mm}{dd}{seq:6}` 是明文流水（ADR-0355 决定五、越权风险点 2）。ADR-0246 越权风险点 6 论证「这一格不构成枚举口」，靠的是包裹号由服务端随机签发、只交回提交方，按运单号查时这一条不再成立：同一租户的另一个客户能顺着自己的号试邻号。隔离只能落在查询键上，不能指望号难猜。
- **包裹身份谱系今天领域里没有模型**（`parcelshipment/ports` 的 `delivery_place_reference_view.go` 头注「谱系未建模的今日形状」）。今天一份已接受委托的当前有效包裹就是接受基线的声明成员。
- 首方族的限流（[ADR-0140](./0140-first-party-api-contract-is-generated-from-endpoint-descriptors-with-url-major-version-and-problem-details.md) 决定三，票 first-party-api/02）冻结到首个租户上线之后。
- 渠道册一行绑定唯一（租户，客户账户），首发 1:1（票 first-party-api/01）。请求方账户只取自绑定（[ADR-0139](./0139-first-party-customer-api-channel-is-a-product-owned-access-channel-family.md) 决定四）。

## Decision

**一、首方客户族新开一口：按委托的单据号查。**（机制）

- `GET /v1/customer-shipment-tracking?documentNumber=<运单号>[&after=<游标>]`。`documentNumber` 就是首方提交答复里的那一格。
- 能力授予另立一个词：客户追踪按运单号查询（`CUSTOMER_SHIPMENT_TRACKING_QUERY`）。客户面能力词表一口一词（票 first-party-api/00「代决」一节「客户面能力词表一口一词」），按包裹查的 `CUSTOMER_TRACKING_VIEW_QUERY` 钉在 `/v1/customer-tracking-view` 那一口上，新口要新词。两口同属 UC-VE-008，授予各格的形状相同（`401`、`403`、`503`），只有按包裹查授予的客户端调本口答 `403 CUSTOMER_CLIENT_NOT_GRANTED`。词本身是产品策略，随产品的能力词表发布（ADR-0139 决定二）；哪个客户端被授予它是租户取值（客户渠道册，`PAR-INT-13`），演示租户以 `SYN-` 授予，证据只记 `S`。
- 租户与货主客户账户只取自客户渠道册的绑定。查询串封闭：`documentNumber` 恰一格，`after` 至多一格；自报账户、租户或多给任何一格都答 `400`（同 `AT-VE-171`）。
- 号逐字比，不做大小写或空白折算：号是编号规则渲染出来的，折算就是另出了一个号。值立不起来答 `400`。这一判只看输入，与号存不存在无关。
- **另开一口，不在 `/v1/customer-tracking-view` 上加参数。** 两种定位答的是两种形状：单件一格，与整单一格加逐件列表。一个端点一份成功响应形状、一套 `outcome` 词表（ADR-0140 决定一的描述符）。按包裹定位那一口的路径、参数、三格与字节一个不动，ADR-0223 决定三对它的承诺照旧成立。

**二、整单级答复四格，按客户系统的续办分（[ADR-0029](./0029-retrieval-failure-algebra-is-partitioned-by-recovery-action.md)）。**（产品策略）

| `outcome` | 何时 | 带什么 | 续办 |
|---|---|---|---|
| `PARCELS_LISTED` | 号指名请求方账户下的一份委托，委托已接受，含其后由包裹结果派生的已取消、已完成 | `parcels`、`page` | 逐件看 |
| `AWAITING_ACCEPTANCE` | 委托已提交，接受或拒绝尚未决定 | 只有 `outcome` | 稍后再查 |
| `CLOSED_WITHOUT_SERVICE` | 委托已拒绝或已撤回（PS CONTEXT「`已撤回`、`已拒绝`的委托没有服务」） | 只有 `outcome` | 这个号不会有可追踪的包裹，停止追踪；客户另提交了关联新委托的，按新委托的运单号查 |
| `SHIPMENT_NOT_FOUND` | 其余一切：本租户没有这个号；号指名的委托属别的货主客户账户；号属别的租户 | 只有 `outcome`，同一副字节 | 核对号 |

- **没有单据号的委托按号查不到，落 `SHIPMENT_NOT_FOUND`。** 无号有两种情形：ADR-0355 上线前开立，或开立时单据编号时区未配置。这类委托的提交答复已带 `documentNumberAbsence` 说明无号，客户系统手里有同一份答复的 `parcels[].parcelId`，照旧按包裹查（ADR-0223 决定三）。不为它另开一格：服务端拿一个号去找，找不到一份没有号的委托，与找不到任何委托是同一回事。
- **待接受不列包裹。** 接受之前声明的包裹不是正式包裹（PS CONTEXT），本上下文没有客户归属可核，逐件只会全是 `VIEW_NOT_FOUND`（ADR-0246 决定五）。客户系统要的声明件号已在提交答复里。
- **拒绝与撤回并一格。** 对追踪查询，两者的续办相同：停止追踪这个号。拒绝原因不是追踪查询的内容。
- **整单级不带委托完成汇总。** 已完成、已取消归 `parcel-shipment` 的汇总，也只是派生；每一件的服务终局已在各件视图的终局一维。
- **读不动答未形成。** 委托读不动，或成页所需的任一读不动，整页答 `500 NO_ANSWER_FORMED`（ADR-0022；UC-VE-008 `AT-VE-168`），不降成 `SHIPMENT_NOT_FOUND`，也不降成某一件的业务格，不出半页。理由：依赖故障的续办是整页退避重发；逐件另立「未形成」一格，客户系统就要多处置一种逐件状态。UC-VE-008 规则「批量查询逐对象返回结果，一个对象冲突或失败不扩大其他对象授权，也不回滚合法结果」，本口在读失败上是它的例外：成页所需的任一读失败整页未形成，不按对象分开答；授权与冲突这些业务结果仍逐件独立，下文决定三照办。UC-VE-008 那句规则同时写明这一例外。

**三、逐件：只列当前有效包裹，每件照单件三格作答。**（产品策略；判据是机制）

- **列哪些。** 列的是委托的[当前有效包裹](../domain/GLOSSARY.md#当前有效包裹)：接受基线成员里没被拆分或合并替代的，加上拆分合并形成的后继包裹。在取消边界前形成取消结果的包裹照列（取消不删除身份）。
- **被替代的来源包裹不单列。** 它不再代表任何实物，单列就是同一件货出现两次。这与委托完成汇总不重复计数来源包裹是同一个理由（PS CONTEXT；GLOSSARY「包裹身份谱系」）。后继一件带 `replaces`，列出它替代的来源包裹号。客户系统手里的旧号（提交答复给的）据此对到新号。谱系今天没有模型，当前有效包裹就是接受基线的声明成员，`replaces` 不出现；谱系落地那一票接上，契约上是新增字段。
- **每件的形状。**
  - `parcelId`：ADR-0223 的定位标识，同一个值可直接用于 `/v1/customer-tracking-view?parcel=`。
  - `customerParcelReference`：该件在委托上声明的客户包裹件号，与提交答复同一格、同一取值（ADR-0223 决定一）。后继包裹与没声明件号的件整键缺席。
  - `replaces`：只在后继包裹上出现。
  - `outcome` 与 `view`：取值是 ADR-0246 决定一的三格，判据照决定二至五，形状与按包裹查完全相同。
- **不变式：同一件、同一时刻，列表里那一格与按包裹查答的逐字相同。**
  - 逐件的视图读与归属反查照 ADR-0246 决定二，不因整单已核过账户就省掉。委托是请求方的，不等于这件包裹此刻唯一归属请求方，例如另有已接受委托同时声明它（标识冲突）。所以请求方自己的已接受委托里，某一件可能答 `VIEW_NOT_FOUND`（ADR-0246 越权风险点 6 同一来由）。
  - 单件的判据只在一处定义。按号查是把按包裹查逐件编排起来，不是第二套判据。
- **次序。** 与首方提交答复的 `parcels` 同序，即首个提交版本的成员次序。接受基线只核成员集合、不定次序：补充形成的新版本只要求成员集合等值，次序可以与首个版本不同，不按它排。后继包裹按谱系形成的先后排在后面。次序是附带承诺，配对靠 `parcelId` 或 `customerParcelReference`。
- **分页。**
  - 照 [ADR-0144](./0144-catalogue-reads-share-one-cursor-pagination-sort-and-filter-contract.md) 的游标形状：请求 `after`，答复 `page`（`size`、`next`、`total`）。不开排序、筛选与 `q`。
  - 页大小由首方渠道契约定，调用方不给：一页至多 50 件（产品策略，随首方契约发布）。`total` 是这份委托此刻的当前有效包裹件数。
  - 游标编进本次的号与末件位置。游标解不开或与本次的 `documentNumber` 不符，答 `400`；这一判不读库，与号是否存在、属谁无关。
  - 每一页都重新按决定二判整单级。所以拿别人的游标换不到别人的委托，读到的是 `SHIPMENT_NOT_FOUND`。

**四、所有权与调用：`visibility-exception` 经消费侧适配器问 `parcel-shipment`，不复制。**（机制）

- 归 `parcel-shipment` 的：委托的单据号、委托所处阶段、委托的客户账户、当前有效包裹集合（含身份谱系）与客户包裹件号。`visibility-exception` 不存这些，也不随视图存（[CONTEXT-MAP](../domain/CONTEXT-MAP.md) 所有权）。
- `visibility-exception` 在自己的 `ports` 立一口，用本上下文的语言：按（租户，货主客户账户，委托单据号）答整单级的封闭四种，已接受的那一种带当前有效包裹，可按页取。实现落在 `internal/visibilityexception/adapters/parcelshipment/`（ADR-0025）。翻译是全函数：`parcel-shipment` 的每个委托状态都有明确落点，不留兜底。
  - 已提交 → 待接受。
  - 已接受、已取消、已完成 → 逐件列出。
  - 已拒绝、已撤回 → 没有服务。
  - 草稿没有单据号，按号不可达。
- `parcel-shipment` 提供对应的读：按（租户，货主客户账户，单据号）答委托所处阶段，已接受时按稳定次序答当前有效包裹及其客户包裹件号与来源关系。
- **账户在键上。** 客户账户是 `parcel-shipment` 那一读的查询条件之一，号属别的账户与查无在提供方口上就是同一个答案。消费方拿不到「存在但不是你的」这一位，也就泄不出去。ADR-0246 决定二在消费方比一位「唯一归属账户等于请求方」；这里连这一位也不出提供方。
- 租户只取自绑定（[ADR-0003](./0003-group-tenant-legal-entity-customer-account.md)）。单据号按租户唯一，别的租户的同号不可见。
- 查询仍不触发派生、披露或通知（UC-VE-008）。

**五、为什么这不泄露存在性，也拿不到别人的包裹号。**

- **号查无、属别的账户、属别的租户，三者同一格、同一副字节、同样的读次数。** 每次合法请求都恰问一次 `parcel-shipment`，键里带请求方账户。只有号属请求方时才有后续的逐件读，而那时请求方本就是委托的主人。
- **列出的每一个 `parcelId` 都是请求方自己那份委托的当前有效包裹。**
  - 接受基线成员是他自己声明的。
  - 后继包裹只由同一委托内的拆分合并形成。PS CONTEXT：「跨委托、跨客户账户或跨合同责任范围的包裹不得形成同一包裹身份谱系」，合并只在「同一委托、同一客户账户、同一合同责任和服务范围内」。
  - 集运、同袋、同班次不构成谱系（同条），同一袋里别的客户的包裹永远不会出现在列表里（`AT-VE-155`）。
- **逐件答复是请求方本可逐件问到的。** 列表不给任何按包裹查给不出的一格。

**六、首方契约：只新增，不改既有。**

- 新端点、它的查询参数与 `outcome` 四格、逐件形状，在 ADR-0140 决定一的描述符立起时照本记录写进首方契约；在那之前以处理器的响应结构体为准。
- `/v1/customer-tracking-view`、提交答复的 `parcels` 与 `documentNumber` 都不改。
- 首方 API 尚未发布兼容性基线（ADR-0140 决定五），今天加一口不破坏任何集成方。

**七、不做的。**

- 不按委托标识（`shipmentRequestId`）定位，不按客户单号或其他外部标识定位。后者归票 demo-chain-gaps/119。
- 不在 `visibility-exception` 存单据号或委托–包裹映射。
- `AWAITING_ACCEPTANCE` 不列声明包裹。`CLOSED_WITHOUT_SERVICE` 不指向关联新委托：新委托有自己的号，答复交给了客户。
- 一次只查一个号，不收号列表。
- 不另设限流。照首方族的限流（ADR-0140 决定三，票 first-party-api/02）。
- 不改操作者族的委托查阅读口。

三类：决定二、三是**产品策略**（整单四格与续办、逐件沿用单件三格、只列当前有效包裹、次序、一页 50 件），决定一里新口的能力词 `CUSTOMER_SHIPMENT_TRACKING_QUERY` 也是；决定一的路径、查询串与键，与决定四，是**机制**；客户渠道册上谁绑哪个账户、谁有按运单号查询的授予，以及单据编号时区，都是**租户取值**（`PAR-INT-13`；ADR-0355 决定七），演示租户以 `SYN-` 登记，证据只记 `S`。

**对已接受记录的作用。** 部分停用 ADR-0223 决定四「不另开按委托号读回逐件号的首方读面。」一句，适用场景是按委托的单据号、经本记录的一口、委托已接受时列当前有效包裹的 `parcelId`；同条按委托标识读回逐件号仍不开，答复丢失照旧靠原样重发取回。部分停用（收窄）ADR-0246 决定一「客户追踪查询的业务结果是三格。」一句，三格收窄为按包裹定位的查询，与按单据号查时列出的每一件；按单据号的查询在整单级另答决定二的四格。回答 ADR-0246 越权风险点 2，两处语义对齐：`PARCELS_LISTED` 与逐件的 `NO_VISIBLE_RESULT` 都只在委托已接受时出现。ADR-0246 决定五「首方客户今天没有别的口读得到委托是否已接受」是写下时的现状；它的决定（接受前按包裹查答 `VIEW_NOT_FOUND`）不变。回答 ADR-0223 越权风险点 6 的后继一半：客户凭运单号能拿到后继包裹的号，凭旧号经 `replaces` 对到新号。

## Consequences

- 代码（票 document-numbering/12）：
  - `parcel-shipment`：按（租户，客户账户，单据号）读委托阶段与当前有效包裹的端口、应用读口与 pg 实现，账户在查询条件里。
  - `visibility-exception`：消费方端口、`adapters/parcelshipment` 的适配器（全函数翻译）、按号查的编排（逐件复用按包裹查的判据，不复制）、新 Intake 与处理器。
  - `cmd/parcel-api`：首方族端点表加一行，挂新能力词的授予。
  - `accessidentity`：客户面能力词表加 `CUSTOMER_SHIPMENT_TRACKING_QUERY`，迁移里那条 CHECK 同笔改；演示种子给 `SYN-` 渠道授这一项。
  - 无迁移。（租户，单据号）唯一约束已在（ADR-0355 决定八）。
- 文档：UC-VE-008 结果语义「查询标识」一行加委托的单据号，另立「按单据号的整单结果」一行与验收行。VE、PS CONTEXT 各一句规则，CONTEXT-MAP 的 PS → VE 关系补一句。首方契约说明（`docs/api/first-party-api-contract.md`）的「首方族今天开着的口」与演示动线「客户追踪视图」一节，随实现票改。
- 管理台没有客户视角的追踪页：操作者控制台不持客户令牌（票 first-party-api/05）。演示走种子脚本。
- 谱系落地之前，当前有效包裹就是接受基线成员，`replaces` 不出现，次序就是提交答复的次序。

## Alternatives considered

- **维持不做，只按包裹查。** 否决（用户裁定）：只持运单号的一方无路可查；后继包裹的号没有答复可带（ADR-0223 越权风险点 6）。
- **在 `/v1/customer-tracking-view` 上加 `documentNumber` 参数，同一路径两种定位。** 否决：两种响应形状、两套 `outcome` 词表挤在一个描述符里，客户端要先看自己发了哪个参数才知道怎么读答复；按包裹那一口的承诺也被搅动。
- **整单级只答整单一格，不列包裹，让客户再逐件按包裹查。** 否决：要解决的正是客户手里没有包裹号，尤其是后继包裹。
- **只列包裹号与件号、不带逐件答复，客户再逐件按包裹查。** 不采：N 件要 N+1 次调用；逐件答复本就与按包裹查相同，带上不新开判据；答复大小由分页约束。
- **号查无与属别的账户分开答。** 否决：CONTEXT「对未授权对象的外部响应不得泄露对象是否存在或属于其他客户」；号可猜，分开答就是枚举口。
- **因为号可猜，要求同时给客户单号或件号作第二因子。** 否决：标识只定位，不是凭证（UC-VE-008），授权来自绑定；隔离已在查询键上，第二因子不加隔离，只加客户的负担。
- **整单级只两格（列出 / 查无），待接受与拒撤都并进查无。** 否决：委托的主人拿对的号会被告知「核对号」。稍后再查、核对号、停止追踪是三种续办（ADR-0029）。
- **拒绝与撤回分两格。** 不采：对追踪查询是同一续办，见决定二。若产品要让客户系统据此区分通知收件人，可在基线发布前加一格（新增 `outcome` 不算破坏，ADR-0140 决定五）。
- **待接受也列声明包裹，逐件答 `VIEW_NOT_FOUND`。** 否决：逐件全是同一格，是噪声；在 `visibility-exception` 里把接受前的声明成员当可追踪对象列出，等于按提交声明认归属，ADR-0246 Alternatives 已否决过。
- **列出被替代的来源包裹，并标注已被替代。** 否决：同一件货出现两次；旧号到新号的对应由后继上的 `replaces` 承载。
- **在 `visibility-exception` 随视图存一份单据号与委托–包裹映射。** 否决：所有权归 `parcel-shipment`；成员关系随谱系变，副本会过期；票 document-numbering/10「不做」已写明。
- **逐件读失败时只那一件答「未形成」，其余照答。** 不采：见决定二末条。

## 越权风险点

1. **号可猜，隔离只靠键**（VE owner）。
   - 默认模板是明文流水，同一租户的任何客户都能试邻号。别人的号、不存在的号、别的租户的号，答复同一副字节、读次数相同。
   - 实现须把账户放进同一条查询的条件里，不得先按号读出再在消费方比账户：那样「存在但不是你的」这一位就出了提供方。
   - 剩下的旁路是时延，没有拉平：（租户，单据号）唯一约束不含账户列，本记录也不加迁移，号属别的账户时比号不存在多一次取行、再按账户筛掉。本记录不加常数时间，与 ADR-0246 越权风险点 3 同一立场。
   - 但这里比 ADR-0246 更值得盯：随机包裹号探中别人对象的概率近于零，明文流水探中的概率很高。介意的租户可登加扰模板（ADR-0355 决定五）。
2. **这一口成了首方客户得知委托接受、拒绝或撤回的路**（VE owner；同时待 PS owner 会签）。
   - `visibility-exception` 不定义委托阶段，只把 `parcel-shipment` 的状态译成三种续办。
   - PS 将来若给首方客户开读委托状态的口，两口要对齐：已提交 ⇔ 待接受；已接受及其后 ⇔ 逐件列出；已拒绝、已撤回 ⇔ 没有服务。
   - 客户系统把 `AWAITING_ACCEPTANCE` 读成「已受理」是读错了。
3. **答复大小与读放大**（VE owner）。
   - 一页 50 件，最多 50 次视图读加 50 次归属反查，外加一次委托读。实现可以批量读，但逐件答复必须与按包裹查逐字相同。
   - 50 是产品策略，随首方契约发布。改它是契约变更，基线之后照 ADR-0140 决定五检查。
4. **分页期间集合会变**（VE owner；同时待 PS owner 会签）。
   - 两页之间若发生拆分合并：后继排在末尾，游标不会跳过它们；被替代的来源包裹从后面的页里消失，前面已读的页里可能还有它；`total` 可能前后不同。
   - 客户要一致快照就从第一页重读。谱系今天没有模型，这一条在谱系落地之前不会发生。
5. **已取消委托的件可能逐件答 `VIEW_NOT_FOUND`**（VE owner）。
   - 委托在视图形成前由已接受转为已取消时，`parcel-shipment` 的当前已接受委托反查不再认它（ADR-0246 越权风险点 6）。这些件在列表里会答 `VIEW_NOT_FOUND`，整单级仍是 `PARCELS_LISTED`。
   - 这是逐件沿用单件判据的直接结果，本记录不另特判。要改就改单件判据，两口一起变。
6. **没有单据号的委托按号查不到**（VE owner）。
   - 上线前开立的委托，与单据编号时区未配置时开立的委托，客户只能按包裹查。ADR-0355 越权风险点 3、5 由此延伸到客户追踪。
   - 提交答复已带 `documentNumberAbsence`，客户系统据它知道不该按号查。
7. **限流冻结期间，枚举只受部署层约束**（VE owner）。首方族限流冻结到首个租户上线之后（票 first-party-api/02）。每次按号探查只花一次带索引的读，不比今天按包裹探查更贵；但号可猜，探查量可能更大。
8. **首方契约加一口**（VE owner）。基线发布前加是无痛的；基线之后，四格、逐件形状与页大小都进兼容性检查。

## Links

- [ADR-0223](./0223-the-first-party-submission-answer-names-each-parcel-and-the-tracking-view-locates-by-it.md)：部分停用决定四「不另开按委托号读回逐件号的首方读面。」一句（适用场景：按委托的单据号、经本记录的一口、委托已接受时列当前有效包裹）；回答越权风险点 6 的后继一半；决定三（按包裹定位的承诺）不变
- [ADR-0246](./0246-the-customer-tracking-query-answers-no-visible-result-for-a-parcel-attributed-to-the-requesting-account-before-its-view-forms.md)：部分停用（收窄）决定一「客户追踪查询的业务结果是三格。」一句（三格只管按包裹定位与按号查时列出的每一件）；回答越权风险点 2；逐件判据（决定二至五）原样沿用
- [ADR-0355](./0355-a-document-number-is-separate-from-internal-identity-and-is-assigned-in-the-issuing-transaction-under-built-in-rules-a-tenant-may-override.md)：委托的单据号与界面词「运单号」；（租户，类型代码，单据号）唯一；默认模板是明文流水；无号的两种情形
- [ADR-0140](./0140-first-party-api-contract-is-generated-from-endpoint-descriptors-with-url-major-version-and-problem-details.md)：首方契约的描述符、`/v1`、兼容性基线、限流；正文不改
- [ADR-0139](./0139-first-party-customer-api-channel-is-a-product-owned-access-channel-family.md)：决定四，客户账户只取自客户渠道册的绑定；决定二，能力词表归产品策略
- 票 first-party-api/00「代决」一节「客户面能力词表一口一词」：新口另立能力词的出处
- [ADR-0025](./0025-cross-context-adapters-live-on-the-consumer-side.md)：跨上下文适配器在消费侧、翻译全函数
- [ADR-0003](./0003-group-tenant-legal-entity-customer-account.md)：租户与货主客户账户的隔离边界
- [ADR-0029](./0029-retrieval-failure-algebra-is-partitioned-by-recovery-action.md)：按续办分格
- [ADR-0022](./0022-http-status-carries-answer-formed-not-business-verdict.md)：状态码只答有没有形成答案
- [ADR-0144](./0144-catalogue-reads-share-one-cursor-pagination-sort-and-filter-contract.md)：游标与 `page` 的形状
- [UC-VE-008](../application/visibility-exception/UC-VE-008-PROVIDE-CUSTOMER-END-TO-END-TRACKING-VIEW.md)：查询标识、按单据号的整单结果、`AT-VE-231` 至 `AT-VE-236`
- [visibility-exception CONTEXT](../domain/visibility-exception/CONTEXT.md)「客户可见性与通知」；[parcel-shipment CONTEXT](../domain/parcel-shipment/CONTEXT.md)「委托」生命周期与身份谱系各条
- [GLOSSARY](../domain/GLOSSARY.md)「单据号」「委托」「当前有效包裹」「包裹身份谱系」「客户归属」
- 票 [document-numbering/12](../../.scratch/document-numbering/issues/12-customer-tracking-by-shipment-number.md)（实现）；出处票 document-numbering/10「第 4 条停在哪（向协调方报）」一节
