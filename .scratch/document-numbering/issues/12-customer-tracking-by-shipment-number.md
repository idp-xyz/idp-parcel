# 12 客户追踪按运单号（委托的单据号）查

Category: enhancement
Status: in-progress——2026-11-02 作者实现完工、自验、推分支 `claude-docnum12`（完成记录见文末）；文档半边的评审修订已在同一分支；待非作者评审与进 main。此前 ready-for-agent——同日用户过目 ADR-0356 草案并同意，ADR-0356 已接受
父票：[spec](../spec.md)
Blocked by: 无（[ADR-0356](../../../docs/adr/0356-customer-tracking-locates-by-the-shipment-document-number-and-lists-the-current-effective-parcels-of-an-accepted-shipment.md) 已接受）
出处：票 [10](./10-wrap-up-references-labels-and-tracking-by-shipment-number.md)「第 4 条停在哪（向协调方报）」一节查清「机制做得到、挡住的是 ADR 级取舍」；用户 2026-11-02 裁定「客户追踪支持按运单号查，先写 ADR」。

## 要做的

规则全在 ADR-0356 与 [UC-VE-008](../../../docs/application/visibility-exception/UC-VE-008-PROVIDE-CUSTOMER-END-TO-END-TRACKING-VIEW.md)，本票不复述，只列落点。

1. **parcel-shipment：按（租户，货主客户账户，单据号）的读。**
   - 端口、应用读口与 pg 实现：答委托所处阶段（PS 自己的状态）；已接受（含其后的已取消、已完成）时，按稳定次序答当前有效包裹，每件带客户包裹件号与来源关系，可按页取。
   - 客户账户放进同一条查询的条件里，不先按号读出再比账户（ADR-0356 越权风险点 1）。
   - 号逐字比，不做大小写或空白折算。
   - 谱系今天没有模型：当前有效包裹取接受基线的声明成员，次序同首个提交版本，来源关系恒空。端口形状要留得下谱系落地后的后继与来源关系，不为今天的简化定形。
2. **visibility-exception：消费方端口与消费侧适配器。**
   - `ports` 立一口，用 VE 的语言答整单级四种（逐件列出带当前有效包裹 / 待接受 / 没有服务 / 查无此单）。
   - `adapters/parcelshipment` 实现它（ADR-0025），PS 的每个委托状态都有明确落点、不留兜底，适配器有自己的测试。
3. **visibility-exception：按号查的编排与首方读口。**
   - 编排：先问委托；逐件列出时，对本页每一件复用按包裹查的判据（视图读，读不到再问一次归属，ADR-0246 决定二）。判据不复制第二份，抽成两口共用的一处。
   - 新 Intake（首方客户族，新能力词 `CUSTOMER_SHIPMENT_TRACKING_QUERY` 的授予——一口一词，不复用按包裹查的 `CUSTOMER_TRACKING_VIEW_QUERY`（ADR-0356 决定一），查询串封闭：`documentNumber` 恰一格、`after` 至多一格）与处理器：`GET /v1/customer-shipment-tracking`。
   - 游标：照 ADR-0144 的编法，编进本次的号与末件位置；与本次号不符或解不开答 `400`，不读库。页大小 50 由 Intake 按渠道契约定。
   - 任一读不动，整页 `500 NO_ANSWER_FORMED`，不出半页。
   - `cmd/parcel-api` 首方族端点表加一行。
4. **首方契约与演示。**
   - [首方客户 API 契约](../../../docs/api/first-party-api-contract.md)：「首方族今天开着的口」加一行，状态码一节「客户追踪视图是例外」处补按号查的四格与逐件；「重放」一节「没有另开按委托号读回的口」那句按 ADR-0356 改。
   - 描述符若已落（ADR-0140 决定一），照 ADR-0356 写进新口。
   - 能力词：`accessidentity` 的客户面能力词表加 `CUSTOMER_SHIPMENT_TRACKING_QUERY`，迁移里那条 CHECK 同笔改；演示种子的客户渠道册给 `SYN-` 渠道授这一项（租户取值，证据只记 `S`）。
   - [演示动线](../../../docs/design/synthetic-demo-journey-script.md)「客户追踪视图」一节加按运单号查的一步。`scripts/demo-seeds/submit-one-shipment.sh`（或同族脚本）在委托接受后按答复里的 `documentNumber` 查一次，断言 `PARCELS_LISTED` 且件号对上；接受前查一次断言 `AWAITING_ACCEPTANCE`。
5. **管理台**：没有客户视角的追踪页（操作者控制台不持客户令牌，票 first-party-api/05），本票不加页。若实现时发现已有客户视角的演示页，跟上按号查。
6. **文档收口**：UC-VE-008「给开发的交接」里本票那一行改成「已补」并写清落在哪（`AT-VE-235` 若仍等谱系，照实写）；AT 盘点 `AT-VE-231` 至 `AT-VE-236` 按实际点名移行，不手改总览计数。

## 不做

- 不按委托标识、客户单号或其他外部标识定位（后者归票 demo-chain-gaps/119）。
- 不在 visibility-exception 存单据号或委托–包裹映射。
- 不改 `/v1/customer-tracking-view`、提交答复与操作者族的委托查阅读口。例外：实现评审查出按包裹查对含 NUL 或非法 UTF-8 的 `parcel` 答 `500`，与新口同一缺陷，本票一并修成 `400`、不读库——缺陷修复，契约形状不变（ADR-0223、ADR-0246 本就要求坏输入答 `400`）。
- 不建包裹身份谱系模型。`AT-VE-235` 等谱系落地那一票；本票只让端口与答复形状留得下 `replaces`。
- 不另设限流（首方族限流在票 first-party-api/02，冻结中）。

## 完成判据

- **PS 真库用例**：按号读本账户已提交、已接受、已拒绝、已撤回各一份；别的账户的号、别的租户的同号与查无三者同答，且同一条查询；当前有效包裹的次序与分页；无号委托按号不命中。
- **VE 适配器测试**：每个 PS 委托状态的落点；PS 读不动译成未形成，不译成查无。
- **VE 编排与 HTTP 用例**：
  - `AT-VE-231` 至 `AT-VE-234`、`AT-VE-236` 逐条点名。
  - 逐件答复与按包裹查逐字相同（同一夹具两口对比）。
  - `SHIPMENT_NOT_FOUND` 几种情形同一副字节。
  - 坏请求不读委托。
  - 认证、授予各格的形状与按包裹查一致（`401` / `403` / `503`），词不同；只有按包裹查授予（`CUSTOMER_TRACKING_VIEW_QUERY`）的渠道调新口答 `403 CUSTOMER_CLIENT_NOT_GRANTED`。
- **`cmd/parcel-api` 真库测试**：新口挂在首方族、带 `/v1`、要 `CUSTOMER_SHIPMENT_TRACKING_QUERY` 授予。
- **门禁**：Go 受影响包加 `internal/architecture`、`cmd/...` 真库测试过；`go build ./...`、`go vet ./...` 退 0。
- **演示**：一次性库灌种子后跑动线脚本，按运单号查得 `PARCELS_LISTED` 与接受前的 `AWAITING_ACCEPTANCE`，证据只记 `S`。
- 照「进 main 的步骤」里「作者在自己的分支或 worktree 上做完」那一步交非作者评审。

## 工作方式

只做 `docs/agents/workflow.md`「进 main 的步骤」里「作者在自己的分支或 worktree 上做完」那一步：作者在自己的 worktree 与分支上做完、自验、推分支、写完成记录；非作者评审与进 main 由协调方做。注释中文、无行号与跨文件计数；不手改 `docs/adr/README.md`、AT 盘点总览计数、机制清点。

## 完成记录（2026-11-02，作者，分支 `claude-docnum12`，从 `00bc4fe68` 起）

### 提交

| 提交 | 内容 |
|---|---|
| `ea21863fc` | 文档半边评审修订（协调方转来，钉 `00bc4fe68`）：阻断两条——UC-VE-008 批量规则为按单据号查的整页让出读失败例外；新口另立能力词——与非阻断六条 |
| `054ba8a03` | parcel-shipment：EffectiveParcel 与谱系落地前的当前有效包裹、按（租户，客户账户，单据号）的端口、应用读口与 pg 实现 |
| `8abcfc847` | access-identity：能力词 `CUSTOMER_SHIPMENT_TRACKING_QUERY`、迁移 `access_identity/0038`、演示种子 `steps/488` 与 `data/access/38` |
| `545e65016` | visibility-exception：整单端口、消费侧适配器、单件判据抽到 application 两口共用、按号查编排、新 Intake 与处理器、认证适配；`cmd/parcel-api` 端点表一行与真库装配用例 |
| `3e2bc443d` | 首方契约、演示动线与两只脚本、种子 README、UC 交接行、AT 盘点移行；ADR-0356 与评审修订对齐的两句 |
| 本笔 | 演示实走记录与本完成记录 |

### 做了什么

1. **parcel-shipment。**
   - 领域：`EffectiveParcel` 带内部标识、位次、客户包裹件号与 `Replaces`。`CurrentEffectiveParcelsOfBaseline` 立谱系落地之前的当前有效包裹：接受基线成员，次序与位次取首个提交版本（同首方提交答复，`submittedParcelsOf` 同一个版本），`Replaces` 恒空。基线为空、成员集合与首个版本不等、件号配到基线外，都报 `ErrInconsistentEffectiveParcels`。
   - 端口 `CustomerShipmentByDocumentNumberView`：按（租户，客户账户，单据号）答状态，已接受时另答当前有效包裹全集。
   - 应用读口 `ReadCustomerShipmentByDocumentNumberHandler`：按位次切页，答 `Total`、`More`。
   - pg：`ShipmentRequestViews.FindCustomerShipmentByDocumentNumber`。一条语句，租户、号、账户同在 WHERE；号逐字比；已接受才解快照，不经聚合重建。
2. **access-identity。** 新词进封闭词表；`MintShipmentTrackingQuery` 核这一词、不判准入。迁移 0038 取此前各份并集重建 CHECK。
3. **visibility-exception。**
   - ports：`CustomerShipmentTrackingView` 答整单四格，可按页取；`ShipmentTrackingStanding` 是本上下文的词；`CurrentCustomerViewReader` 是两口共用的视图读口。
   - 适配器 `CustomerShipmentLookup`（ADR-0025）：已提交 → 待接受；已接受 → 逐件列出；已拒绝、已撤回 → 没有服务；查无 → 查无此单；词表外的状态包 `ErrCustomerShipmentUntranslatable`，不兜底；PS 读不动原样上抛。
   - application：`TrackCustomerParcel` 是单件三格判据唯一的一处，按包裹查改走它；`TrackCustomerShipment` 先问整单，逐件走单件判据，任一读不动整页交回错误。
   - http：`CustomerShipmentTrackingIntake`（查询串只认 `documentNumber` 一格、`after` 至多一格，页大小 50）与 `NewQueryCustomerShipmentTrackingEndpoint`。
   - 认证：`CustomerShipmentTrackingAuthenticator` 核新词。
   - `cmd/parcel-api`：`/v1/customer-shipment-tracking` 挂在客户追踪那一特性里，`face` 是新词。
4. **契约与演示。**
   - 首方契约：开着的口加一行；状态码一节补四格与逐件；重放一节那句改准；新口的请求、逐件、分页、无号委托各一条。
   - 演示：动线加一步；`submit-one-shipment.sh` 提交后按号断言 `AWAITING_ACCEPTANCE`，末行带出运单号；`chain.sh` 的 `ch_submit` 接受后断言 `PARCELS_LISTED`、件号对上。
   - 描述符（ADR-0140 决定一）未落，契约照处理器响应结构体写。
5. **管理台**：查过，没有客户视角的追踪页（`pages/visibility/api.ts` 只在注释里提到 `/v1/customer-tracking-view`），本票不动。
6. **文档收口**：UC-VE-008 交接行改为已补，写清落点；`AT-VE-235` 照实写仍等谱系。AT 盘点 `AT-VE-231` 至 `AT-VE-234`、`AT-VE-236` 移入①并逐条点名用例，`AT-VE-235` 留④，追记一行，总览计数未重算。

### 关键取舍与理由

- **游标记位次，不记下标。** 位次是 PS 给的，一经给出不变：基线成员占首个提交版本里的序号，谱系落地后后继接在全部基线成员之后。被替代的来源包裹离开集合时，后面的件不前移，下一页不跳件（ADR-0356 越权风险点 4）。`More` 由 PS 答，不拿末件位次比 `Total`：谱系落地后位次不连续。
- **PS 端口答全集，应用读口切页。** 当前有效包裹今天出自委托快照的一次读，整份读出再切不多一次往返。谱系成表后可下推到读口，应用读口的签名不变。端口已带 `Replaces` 与位次，不为今天的简化定形。
- **单件判据搬到 application，不留在 HTTP 适配器里抄一份。** 列表的一件嵌入按包裹查同一个 `queryResponse`，经同一个 `newParcelTrackingBody` 写出，逐字相同靠的是同一份结构。按包裹查的既有用例原样全过，字节不变。
- **游标用 ADR-0144 的共用件 `cataloguepage`。**
  - 号作册子选择器进摘要，游标与本次号不符即 400，不读库。
  - 摘要用的是 Intake 立好的那个号，与查询键同源，不重读原始查询串里的号。
  - 排序维 `position` 只住在不透明的游标里。`sort`、`q` 在 Intake 的封闭查询串上就拒掉。
- **认证适配另立一只，铸出的信封共用一型。** 两口各核各的词；铸出的身份同形（租户、账户），另立一型只多一份抄本。
- **读回数据立不起当前有效包裹时答未形成。** 例如已接受却没有基线，或成员集合不等。不答查无此单：那会把一行坏数据说成「没有这单」。
- **演示断言的分工。** 接受前的断言放在 `submit-one-shipment.sh`，与它对详情断言 `SUBMITTED` 是同一个前提（dispatch 下一拍才判接受）。接受后的断言放在已经等接受的 `ch_submit`。末行追加「；运单号 …」，`chain.sh` 里 `REQ`、`PARCEL`、`PREF` 的解析不受影响。

### 验证（证据只记 `S`）

- `go build ./...`、`go vet ./...` 退 0；`gofmt -l cmd internal migrations tools` 无输出。
- `go test -count=1 -p 1 ./internal/parcelshipment/... ./internal/visibilityexception/... ./internal/architecture/... ./internal/platform/... ./internal/accessidentity/... ./cmd/... ./migrations/...` 全过，含 PG（`127.0.0.1:55432`）。测试跑在 `545e65016` 的 Go 内容上，其后两笔不动 Go。
- 新用例 `-v` 抽查，全部 PASS、无 SKIP：
  - PS 真库：`TestACustomersOwnShipmentIsReadByNumberInEachState`、`TestOtherAccountsOtherTenantsAndMissingNumbersAnswerAlikeInOneStatement`（用查询追踪记下每一问恰一条语句、同一段 SQL、账户在参数里）、`TestEffectiveParcelsArePagedInFirstVersionOrderAgainstARealDatabase`。
  - `cmd` 真库：`TestACustomerTracksItsShipmentByNumberAgainstARealDatabase`。
  - 其余新用例分布在 PS 领域与应用、VE 适配器、编排、HTTP、认证、`accessidentity` 与 `cmd` 授予各格。
- `tools/at-coverage-counts -check` 只报总览计数过期，表行与 UC 立号对得上。
- 演示：一次性库 `docnum12_demo` 经 `up.sh`（`:19102`）灌种子退 0，`10-main-chain.sh` 退 0。断言 `AWAITING_ACCEPTANCE` 与 `PARCELS_LISTED`（件号对上），另有几条现场探针，详见演示动线那段实走记录。走完 `down.sh --drop-db`，进程已停、库已删。
- 管理台没动，没跑前端三道门。

### 留给协调方

- **撞号**：迁移 `access_identity/0038`、种子 `steps/488` 与 `data/access/38` 取的是本分支起点上的下一个号。进 main 前别的票若已占用，请改号。
- **ADR-0356 Status 行**：过目记录的措辞没动。`｜` 之后的索引注记里「与按包裹查同一项追踪查阅授予」照评审裁定改成了新词，「第 4 条」改引小节名。ADR 索引与 AT 盘点总览由推送方重生成。
- **ADR-0356 的两句对齐**：评审修订之后，Consequences「无迁移」与越权风险点 1「不加迁移」读起来与新词的那份迁移相抵。已在 `3e2bc443d` 改成「PS 无迁移，唯一的迁移是能力词 CHECK」与「不给唯一约束加账户列」，请评审一并看。
- **仍待落地或复核**：`AT-VE-235` 等包裹身份谱系落模型；端口与答复已留出 `replaces`。ADR-0356 越权风险点待 VE、PS owner 复核，本票未改。
