# 末端面单渠道能力形状盘点

Category: chore
Status: resolved——只读盘点，已被消费为本目录子票 `02`..`17`（spec 2026-09-02 MCP-4 Comment：「两份盘点的建议一并落成 `02`..`17`」）；本文此后只作取证出处，不再维护。状态由通道 2 于 2026-09-04 代簿记

取证基线 `origin/main` = `957e768`（2026-09-02 取证）。断言一律锚到文件与符号名；本文写证据不写结论。

**不做什么**：不写产品代码，不改 `docs/**`，不改任何既有票面（含本目录 `spec.md`）。每段末尾的「票粒度」是建议不是票面，立票归 MCP-1。不填任何实例取值——渠道字段名、体积系数、金额、账号一个都不出现（[ADR-0088](../../docs/adr/0088-label-channel-service-enters-the-first-release-service-forms.md) Decision 五）。不打开 `docs/wooolink/` 或 `docs/reference/xls/` 下的客户文件。不替产品拍板，不写方案设计。

盘的链条：客户下单 → 取末端渠道面单 → 面单 PDF 返回/修改 → 轨迹回传 → 结算。**轨迹那一段不在本文**，另见[轨迹源接收缝盘点](./tracking-source-seam-inventory.md)；本文到「渠道结果落在面单交易上」为止。

## 一、面单交易域：聚合、状态、持久化、读面、端点

### 已有形状

**聚合**在 `internal/parcelshipment/domain/label_transaction.go`。`LabelTransaction` 是独立聚合，键为租户加 `LabelTransactionID`。建立时固定项由 `EstablishLabelTransactionSpec` 逐项列出并在 `EstablishLabelTransaction` 里全部过构造门：`ChannelAccountReference`、`ChannelAccountHolderReference`、`ChannelServiceProviderReference`、`SettlementCounterpartyReference`、`ChannelContractReference`、`ChannelRateReference`、`ResponsibilityBasisSnapshotReference` 各是独立值类型，没有任何方法能在建立之后改它们。覆盖范围经 `fixCoveredParcels` 复制入聚合。

**状态**是封闭集合 `LabelTransactionState`：`LabelTransactionEstablished`、`LabelTransactionSubmitted`、`LabelTransactionResultUncertain`、`LabelTransactionSucceeded`、`LabelTransactionPartiallySucceeded`、`LabelTransactionFailed`。转移方法三个：`SubmitToChannel`、`MarkResultUncertain`、`RecordChannelResult`。两层结果一次写下（`RecordChannelResultSpec` 同时收交易级 `Outcome` 与逐包裹 `ParcelResults`），逐包裹结果经 `matchResultsToCoverage` 对齐覆盖范围。定案是派生谓词 `Finalized()`，无存储字段。后续动作走追加式 `AppendFollowUpAction`，`FollowUpActionKind` 三格（`ChannelVoidAction`、`ChannelRefundAction`、`ChannelReplacementAction`）。重试/替代关系由 `EstablishPriorLabelTransactionLink` 在**新**交易出生时建立，且要求原交易已定案。

**持久化**在 `internal/parcelshipment/adapters/postgres/label_transaction.go`（`LabelTransactions` 的 `FindByID`/`Insert`/`Save`）与 `migrations/parcel_shipment/0010_label_transaction.sql`。行模型是键 + `revision` + `state` + `snapshot jsonb`；`state BETWEEN 1 AND 6` 的 CHECK 镜像领域封闭集；无 `finalized` 列。读回路径经 `internal/parcelshipment/domain/label_transaction_rehydration.go` 的 `RehydrateLabelTransaction`。写入代数在 `internal/parcelshipment/ports/ports.go`：`LabelTransactionInsertOutcome`（`LabelTransactionInserted`/`LabelTransactionAlreadyExists`）与 `LabelTransactionSaveOutcome`（`LabelTransactionSaved`/`LabelTransactionRevisionConflict`）分立。

**读面**是 `ports.LabelTransactionViews.ListLabelTransactions(ctx, tenant, limit)`，记录类型 `LabelTransactionRecord` 与「交易 × 包裹」行 `LabelTransactionParcelRow`（`HasResult` 与 `Accepted` 分开、`FollowUpKinds` 并列不改写 `Accepted`、`ContinuedAttemptOpen` 派生自一段真实为空的决定历史）。实现在 `adapters/postgres/label_transaction_views.go`，HTTP 在 `adapters/http/query_label_transactions.go`。

**端点**已进装配：`cmd/parcel-api/endpoints.go` 挂 `GET /label-transactions`，Intake 走 `shipmenthttp.LabelTransactionQueryIntake`，默认 `UnconfiguredIntake{}`，隔离读放行时由 `cmd/parcel-api/assemble_isolated_read.go` 的 `isolatedReadIntakes.labelTransactions` 换值；`cmd/parcel-api/main.go` 交入真实 `pspostgres.NewLabelTransactionViews(db)`。管理台页面在 `apps/admin-web/src/pages/shipment-request/LabelTransactionsPage.tsx`。

### 缺口

- **无任何写侧执行器。** `internal/parcelshipment/application/` 下一个面单交易用例都没有：该目录的执行器全部是委托/受理/取消/终局侧（`submit_shipment_request.go`、`form_acceptance_decision.go`、`cancel_parcel.go`、`form_parcel_final.go` 等）。`internal/architecture/production_wiring_baseline.txt` 把 `EstablishLabelTransaction` 与 `EstablishPriorLabelTransactionLink` 显式登为「无生产调用方」，并注明这是设计而非遗漏。因此「委托提交 → 建立交易 → 提交渠道 → 记录结果 → 追加后续动作」这条链上，聚合方法齐备而**编排一环没有**。
- **渠道返回的面单载荷（PDF/ZPL）在领域与库里都没有落点。** 聚合上与结果有关的字段只有 `ChannelParcelIdentifier`（包裹级标识）与 `ChannelResultReasonReference`（结果原因）；`LabelTransactionParcelResult` 只有 `parcel`/`accepted`/`identifier`/`reason` 四项。读面 `LabelTransactionParcelRow` 同形，也无载荷字段。全仓 `internal/` 下 `PDF`、`ZPL`（含小写）**零命中**——这个数本身是论点，实测于 `957e768`。`0010_label_transaction.sql` 的列与快照文档同样没有载荷或其引用。
- **`面单继续尝试决定`登记册未建**（ADR-0084 决定六，聚合注释显式点名本切片不建），因此 `LabelTransactionParcelRow.ContinuedAttemptOpen` 今天派生自空历史。
- **包裹终局跨交易判断不在聚合内**（同一处注释点名），需要跨该包裹全部相关交易与实际承运商收寄事实，今天没有承接它的执行器。

### 半边归属

写侧执行器、载荷落点形状、继续尝试决定登记册、包裹终局跨交易判断——四项全属**机制半边**，不需要真实租户参数就能立。载荷的**具体格式取值**（哪家渠道给 PDF、哪家给 ZPL、DPI 多少）属实例半边，见第二段。

### 建议的票粒度

一票补写侧执行器链（建立/提交/记录结果/追加后续动作，落 `internal/parcelshipment/application/`）；一票单独定「渠道返回载荷」在聚合与库上的落点形状；继续尝试决定登记册与包裹终局各一票（ADR-0084 决定六已点名前者另票）。

## 二、末端渠道取面单的适配器缝

### 已有形状

端口只有两个，都在 `internal/parcelshipment/ports/ports.go`，都朝**内**：`LabelTransactionRepository`（存取聚合）与 `LabelTransactionViews`（读面）。`LabelTransactionRepository` 的文档注释自己写明「本口今天没有生产写入方」。

[渠道适配缝备忘](../../docs/design/channel-adapter-seams-design-note.md)把「面单 / 标签获取」指到 `parcel-shipment` 的面单交易，并写明「每家渠道的取面单适配器接在此处」。

### 缺口

- **朝外的取面单端口不存在。** 全仓没有任何以渠道/承运商为对象的出向接口——按 `^type \w*(Gateway|Client|Channel|Carrier|Courier|Provider)\w* interface` 扫 `internal/`，命中的是 `visibilityexception/ports.NotificationChannelGateway`（通知渠道，注释自称唯一实现是测试替身）、`partycommercial/ports.ProductChannelMappingRegistry` 与 `ProductChannelMappingCatalogueRead`（商业映射登记与读面）、`accessidentity.ChannelRegistry` 与 `ChannelCredentialProof`（**入向**接入渠道，`ChannelRegistry` 注释写明本仓无生产实现）。没有一个是「向末端渠道发起取面单请求」。
- **一家渠道的实现或合成替身都没有。** `internal/parcelshipment/adapters/` 下的目录是 `adoptconsume`、`finalconsume`、`http`、`identity`、`inbox`、`networkrouting`、`nodeoperations`、`parcelpricing`、`partycommercial`、`pilotgovernance`、`postgres`、`settlementaccounting`、`transportfulfillment`——全是内部上下文与传输/持久化，无渠道方向。

### 六家末端渠道的公开接入形态差异（公开资料，不涉客户账号）

以下只用公开开发文档，用途是判断端口形状会不会被形态差异撑破；不写任何客户名、客户的渠道账号或报价。

| 渠道（公开承运商名） | 取面单形态 | 载荷形态 | 对端口形状的影响点 |
|---|---|---|---|
| FedEx（Ship API，含 Ground / Ground Economy） | 一次 ship 调用同时回单号与面单 | `labelSpecification.imageType` 取 PDF/PNG/ZPLII/EPL2/DPL；载荷在 `ShippingDocumentPart.Image` | 格式是**请求参数**，同一渠道可回栅格或指令流两类载荷 |
| UPS（Shipping API） | 一次 ship 调用回结果 | `LabelImageFormat.Code` 取 GIF/ZPL/EPL/SPL（Label Recovery 另有 PDF）；`GraphicImage` 为 base64；GIF 另回 `HTMLImage`，部分场景另回 `internationalSignatureGraphicImage`、`pdf417` | **一件包裹可能回多份图件**，单一「载荷」字段装不下 |
| USPS（Domestic Labels API v3，含 Ground Advantage） | `POST /labels/v3/label` 回面单；另有 `DELETE /labels/v3/label/{trackingNumber}` 作废 | `imageInfo.imageType` 取 PDF/TIFF/JPG/PNG/GIF/SVG/ZPL203DPI/ZPL300DPI/`LABEL_BROKER`/`NONE` | `LABEL_BROKER` 回的是二维码而非面单，`NONE` 不回图件——「取了面单」与「回了图件」不是同一件事；作废是**渠道侧独立调用** |
| UniUni（Platform Client API） | **两步**：`POST /client/shipments/create` 落 DRAFT，再 `POST /client/shipments/{orderNumber}/purchase` 才出 `trackingId`；面单另经 `GET /client/label/{id}?labelType=shipping\|batching` 取 | base64 PDF（`data.body`），带 `Content-Type`/`Content-Disposition` 头 | 三处影响：①「已建立」与「已提交渠道」在渠道侧是两次调用；②面单是**第三次**调用才拿到，不随提交应答返回；③`labelType=batching` 的面单粒度是**批**不是包裹；④HTTP 一律 200，成败在 body 的 `code` 上（0 成功、1002 无效、1014 未找到），**HTTP 状态码不能用来判结果不确定** |
| GOFO | 公开资料未取得直连开发文档（17TRACK 的承运商页明写「当前公开官方 API 认证方式未确认」）；公开可见的是经 4Seller / ShipWise / ShipHero 等第三方平台接入，其中 4Seller 的说明提到「官方运费接口」与授权流程按 EntryPort 逐仓库重复授权 | 未取得 | 存在「经第三方平台间接接入」这一形态；且第三方侧的 GOFO 说明写明「一件包裹一张面单，多件走 bagging 归组」「不支持自动退货面单」 |
| FedEx Ground SP | 未单独取证。公开资料里 SP（SmartPost 谱系）与 Ground Economy 同属 FedEx Ship API 的服务类型维，不是另一套接入 | 同 FedEx | 若成立则它是**服务类型**差异而非接入形态差异；此判断未坐实，按未取证记 |

差异落在四处，都指向端口形状而不是取值：**取面单与下单是否同一次调用**（FedEx/UPS/USPS 同次，UniUni 分三次）、**一次结果是否只有一份图件**（UPS 明确不是）、**面单粒度是否恒为包裹**（UniUni 批面单不是）、**成败信号在哪一层**（UniUni 在 body 而非 HTTP 状态）。

### 半边归属

端口形状、替身、以及「一次调用/多次调用」「一件/多件图件」「包裹/批粒度」这三格在类型上分不分得开——**机制半边**。每家渠道的账号、授权、字段名、报价表——实例半边（`PAR-INT-02`、`PAR-SET-03`）。

### 建议的票粒度

先一票只定取面单出向端口的形状（把上表四处差异做进类型，不接任何真渠道）；再一票补一个合成替身把「结果不确定」「部分成功」「批粒度面单」三条走通。真渠道适配器按备忘「一类数据一张票」逐家落，不在本轮。

## 三、渠道候选择优（`PAR-NET-16`）

### 已有形状

**`BUY` 侧评价齐备。** `internal/parcelpricing/domain/value_objects.go` 的 `PricingDirection` 有 `PricingDirectionBuy`，与 `PricingPurposeSupplierCost` 配对（`plan.go` 的配对门）。装载按方向隔离：`adapters/postgres/price_card_catalog.go` 的 `LoadApplicable(ctx, tenant, direction, scope, asOf)`。评价用例在 `application/evaluate_pricing.go`，价卡登记在 `application/register_price_card.go`。计价重与体积系数机制在 `domain/weight_rounding.go`、`domain/dimensions.go`、`domain/volumetric_test.go`；价表族在 `domain/rate_families.go`。

**候选择优的比较器齐备，但在 `networkrouting` 且是通用的。** `internal/networkrouting/domain/route_ranking.go` 的 `SelectRouteCandidate(candidates, scores, priority)` 在**合格**候选中按策略声明的准则序做字典序比较，全平时按候选标识升序收尾，无合格候选交 `ErrNoQualifiedCandidate`。准则 `RankingCriterion` 刻意是引用不是枚举，分值 `CriterionScore` 越小越优、量纲与折算由策略版本拥有。调用方是 `application/create_initial_route.go` 与 `application/reassess_route.go`。

**「本服务不要求网络可达性判断」这一格已存在。** `internal/networkrouting/domain/network_eligibility.go` 的 `NetworkJudgmentNotRequired` 必须携带 `EligibilityBasisReference`；测试里用的依据字符串就是 `LABEL_ONLY_CHANNEL_SERVICE`（`network_eligibility_test.go`、`application/assess_parcel_reachability_test.go`）。

### 缺口

- **`PAR-NET-16` 在生产代码里零命中**（实测于 `957e768`：全仓该字符串只出现在 `docs/` 下的登记册、`ADR-0014`、`ADR-0015`、`ADR-0088` 与两份 design 备忘）。没有任何执行器把「候选集合 → 逐候选 `BUY` 评价 → 按计费重后的总价比较 → 选出一条并给落选者留痕」串起来。
- **候选来源没有接线。** 候选按 ADR-0088 Consequences 来自产品—渠道映射与渠道约束，那两样在 `internal/partycommercial`（`ProductChannelMappingRegistry`、`ProductChannelMappingCatalogueRead`）；`SelectRouteCandidate` 收的是已经装好的 `[]RouteCandidate` 与 `[]CandidateScores`，没有任何适配器从产品—渠道映射生成它们。
- **成本分值与 `BUY` 评价之间没有桥。** `CriterionScore` 明写「折算发生在事实形成处」，而「把某候选的 `PricingEvaluation` 总价折成 `COST` 准则分值」这一步今天没有实现方；`parcelpricing` 侧也没有「一次输入对多份 `BUY` 价卡逐份评价」的批量口。
- **落选留痕的对象不清。** `nrdomain.RouteCandidate` 带 `CandidateOutcome` 与 `CandidateReason`（`visibilityexception/adapters/networkrouting/derive_on_initial_route.go` 消费它），但那是**路由**候选；渠道候选是否复用它、还是归 `parcel-shipment` 面单交易侧，没有代码可指。
- **`PAR-NET-16` 的两条硬约束在机制上无守卫**：「候选评价为待判断或不可计价时该候选出局，不得以零金额顶替」与「并列且无法选出唯一一条时为冲突，交人工裁决，不得任选」。前者在 `parcelpricing` 有 `EvaluationPending` 这一格（`domain/evaluation_test.go` 的 `DIMENSIONS_REQUIRED`/`RATE_NOT_FOUND`），但没有消费它的择优方；后者与 `SelectRouteCandidate` 现行行为**相反**——它全平时按候选标识升序收尾（为了幂等），而 `PAR-NET-16` 要的是「冲突交人工」。这一处不是缺实现，是两条口径在同一个函数上撞了，需要裁决归属再动。

### 半边归属

择优执行器归哪个上下文、候选装配缝、成本分值桥、落选留痕对象、以及「并列即冲突」与现行「标识升序收尾」的口径归属——全属**机制半边**（最后一项还需要一次归属裁决）。候选集合本身、各渠道价卡与体积系数——实例半边（`PAR-NET-16` 登记为待提供，`PAR-SET-03` 逐份取证）。

### 建议的票粒度

第一票先只裁「渠道候选择优归哪个上下文执行」并把「并列即冲突」与现行升序收尾的冲突结清（够不上 ADR 的话至少要一条票面裁决）；其后候选装配、成本分值桥、落选留痕各一票。

## 四、`ServiceProductForm` 第二取值缺席的波及面

### 已有形状与波及点

`internal/partycommercial/domain/service_product.go` 的 `ServiceProductForm` 只有 `NetworkServiceForm` 一个有效取值；`valid()` 直接写成 `form == NetworkServiceForm`，`String()` 只有一格。注释写明「独立面单渠道服务是长期产品形态，`PAR-COM-12` 明确它对首发不适用，所以这里有意不列出它」——而 `PAR-COM-12` 已随 ADR-0088 改为纳入，注释的依据句已经过期。

缺席在代码里波及以下各处（只列文件与符号，不改）：

- `internal/partycommercial/domain/service_product.go`：`ServiceProductForm.valid()`、`String()`、`NewServiceProduct` 的 `!form.valid()` 门。
- `migrations/party_commercial/0008_service_product_form.sql`：`service_product_form_closed` CHECK 为 `form IN ('NETWORK_SERVICE')`，注释明写「扩展先改领域封闭集，再改这一条」。
- `internal/partycommercial/adapters/postgres/service_product_form.go`：`serviceProductFormFrom(raw)` 的 `switch` 只认一格，集外取值上抛。
- `internal/partycommercial/adapters/postgres/commercial_resolution.go`：`rehydrateServiceProduct` 的 `switch` 同形，未知取值响亮失败；快照文档 `adoptedDocument.Form` 用 `omitempty`。
- `internal/networkrouting/adapters/partycommercial/form.go`：`translateForm` 的 `switch` 只把 `NetworkServiceForm` 译成 `NetworkJudgmentRequired`，`default` 交 `ErrUntranslatableAnswer`。**这是波及面里最实的一处**——面单渠道服务恰恰是「不要求网络可达性判断」那一格（依据引用 `LABEL_ONLY_CHANNEL_SERVICE` 已在 NR 侧测试里出现），第二取值一旦落地，这个 `switch` 是必改的一格，而它今天会把该形态判成「翻译不出来」。
- `internal/partycommercial/application/register_product_channel.go`：`RegisterServiceProductFormCommand.Form` 与 `RegisterServiceProductForm` 的写路（形态册挂在 `ports.ServiceProductFormRegistry` 上）。
- 守卫侧（不是波及，是会**变红**的一处）：`internal/partycommercial/domain/service_product_test.go` 的 `TestNoServiceProductCanTakeAnIndependentWaybillChannelForm` 扫完整个 `uint8` 值域，任何第二取值构造得出服务产品它就红；`TestServiceProductFormIsAFacetNotASeparateCatalog` 断言 `ServiceProductForm(2).String() == ""`。两条用例的 `Covers` 指的是 `AT-PC-015` 与 `AT-PC-030`，而这两条验收行已随 ADR-0088 修订。
- `internal/partycommercial/adapters/postgres/service_product_form_test.go` 的文件头注释登记了两条「今天够不着因此没有用例」的防御分支（内容冲突、形态取值不认识），并写明「`PAR-COM-12` 解封、第二种形态落地那一天，两条同时变得够得着，届时补用例」。

### 缺口

第二取值本身缺席，且它不是「加一个枚举值」——上列 SQL CHECK、两处 `switch`、一处翻译表与两条守卫用例是同步扩展的一组；`translateForm` 那处还要求先有「面单渠道服务对应哪种网络资格」的口径。

### 半边归属

**机制半边**。取值本身不是实例参数——`PAR-COM-12` 登记的是已确认范围决策，不是待提供取值。

### 建议的票粒度

一票落第二取值并同笔改齐领域封闭集、SQL CHECK、两处 `switch`、`translateForm` 的落点与两条守卫用例（少改一处就是一半说谎）；`translateForm` 的落点若需裁决，先在票面里问清。

## 五、面单文件生成与修改的文档适配器

### 已有形状

无。

### 缺口

全仓 `internal/` 下没有任何文档生成件：`PDF`/`ZPL` 零命中（同第一段，实测于 `957e768`），按 `render`/`Render`/`template`/`Template`/`单证`/`文档生成` 扫 `internal/*.go` 的命中全部落在 `internal/platform/migrate/plan.go`（迁移框架模板）与各上下文的 `*_handoff_test.go`（outbox 信封载荷），无一处与单证渲染有关。[`requirement-mapping.md`](../tenant-implementation-01/requirement-mapping.md) 尾程流表把「面单 PDF 生成/修改」判为**待建**，本次取证与该判定一致。

同表把 7501 税单与发票生成也判为待建，本文不盘那两项（不在本任务范围）。

需要一次裁决而非实现的一格：ADR-0088 Consequences 把「面单文件生成与修改的文档适配器」列进机制半边要补的形状，而末端渠道**大多直接返回成品面单**（见第二段公开资料）。因此「生成」与「取回渠道成品后再修改/重排」是两件事，今天没有任何票面或文档区分它们；「面单 PDF 修改」在客户条目里指的是哪一件，本次取证不足以判定，留给 MCP-1。

### 半边归属

适配器缝与「生成 vs 取回后加工」的口径——**机制半边**（后者含一次口径裁决）。纸张尺寸、DPI、模板样式——实例半边。

### 建议的票粒度

先一票只结「生成 / 取回后加工」的口径归属，不写实现；实现票等第二段的取面单端口形状定了再立，两者共用同一个载荷落点。

## 汇总（缺口条目）

| 段 | 缺口 | 半边 |
|---|---|---|
| 一 | 面单交易写侧执行器全缺（`application/` 无一个用例） | 机制 |
| 一 | 渠道返回载荷（PDF/ZPL）在领域、库、读面均无落点 | 机制（格式取值属实例） |
| 一 | `面单继续尝试决定`登记册未建，`ContinuedAttemptOpen` 派生自空历史 | 机制 |
| 一 | 包裹终局跨交易判断无承接执行器 | 机制 |
| 二 | 朝外的取面单端口不存在 | 机制 |
| 二 | 无任何渠道实现或合成替身 | 机制 |
| 二 | 公开形态差异四处（调用次数、多份图件、批粒度面单、成败信号层）未进类型 | 机制 |
| 三 | `PAR-NET-16` 生产代码零命中，无择优执行器 | 机制 |
| 三 | 候选装配（产品—渠道映射 → 候选集合）无适配器 | 机制 |
| 三 | `BUY` 评价 → 成本准则分值无桥；无逐候选批量评价口 | 机制 |
| 三 | 落选留痕对象归属不清 | 机制 |
| 三 | 「并列即冲突」与 `SelectRouteCandidate` 现行升序收尾口径相撞 | 机制（需裁决） |
| 四 | `ServiceProductForm` 第二取值缺席，波及一处 SQL CHECK、两处 `switch`、`translateForm` 与两条守卫用例 | 机制 |
| 五 | 无任何文档生成适配器 | 机制 |
| 五 | 「生成」与「取回渠道成品后加工」口径未分 | 机制（需裁决） |

## Comments

- 2026-09-02 MCP-6：只读盘点，基线 `957e768`。未改产品代码、未改 `docs/**`、未改任何既有票面，未立票。第二段的六家渠道形态取自公开开发文档（FedEx Developer Portal、UPS Ship API 字段参考、USPS Domestic Labels API v3 与 eVS 文档、UniUni Platform Client API 文档、17TRACK 承运商页与第三方平台集成说明），未打开任何客户文件；GOFO 与 FedEx Ground SP 两行按未取证记，没有拿第三方页面的转述当官方形态。
