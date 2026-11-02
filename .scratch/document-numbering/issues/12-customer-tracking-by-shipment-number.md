# 12 客户追踪按运单号（委托的单据号）查

Category: enhancement
Status: blocked——2026-11-02 文档半边已起草（ADR-0356 草案，UC-VE-008、VE / PS CONTEXT、CONTEXT-MAP 与 AT 盘点同笔），分支 `claude-docnum12`；实现等 ADR-0356 接受
父票：[spec](../spec.md)
Blocked by: ADR-0356 待用户接受（[ADR-0356](../../../docs/adr/0356-customer-tracking-locates-by-the-shipment-document-number-and-lists-the-current-effective-parcels-of-an-accepted-shipment.md) 现为 Proposed，用户过目后由协调方改为 Accepted）
出处：票 [10](./10-wrap-up-references-labels-and-tracking-by-shipment-number.md) 第 4 条查清「机制做得到、挡住的是 ADR 级取舍」；用户 2026-11-02 裁定「客户追踪支持按运单号查，先写 ADR」。

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
   - 新 Intake（首方客户族，同一项 `CUSTOMER_TRACKING_VIEW_QUERY` 授予，查询串封闭：`documentNumber` 恰一格、`after` 至多一格）与处理器：`GET /v1/customer-shipment-tracking`。
   - 游标：照 ADR-0144 的编法，编进本次的号与末件位置；与本次号不符或解不开答 `400`，不读库。页大小 50 由 Intake 按渠道契约定。
   - 任一读不动，整页 `500 NO_ANSWER_FORMED`，不出半页。
   - `cmd/parcel-api` 首方族端点表加一行。
4. **首方契约与演示。**
   - [首方客户 API 契约](../../../docs/api/first-party-api-contract.md)：「首方族今天开着的口」加一行，状态码一节「客户追踪视图是例外」处补按号查的四格与逐件；「重放」一节「没有另开按委托号读回的口」那句按 ADR-0356 改。
   - 描述符若已落（ADR-0140 决定一），照 ADR-0356 写进新口。
   - [演示动线](../../../docs/design/synthetic-demo-journey-script.md)「客户追踪视图」一节加按运单号查的一步。`scripts/demo-seeds/submit-one-shipment.sh`（或同族脚本）在委托接受后按答复里的 `documentNumber` 查一次，断言 `PARCELS_LISTED` 且件号对上；接受前查一次断言 `AWAITING_ACCEPTANCE`。
5. **管理台**：没有客户视角的追踪页（操作者控制台不持客户令牌，票 first-party-api/05），本票不加页。若实现时发现已有客户视角的演示页，跟上按号查。
6. **文档收口**：UC-VE-008「给开发的交接」里本票那一行改成「已补」并写清落在哪（`AT-VE-235` 若仍等谱系，照实写）；AT 盘点 `AT-VE-231` 至 `AT-VE-236` 按实际点名移行，不手改总览计数。

## 不做

- 不按委托标识、客户单号或其他外部标识定位（后者归票 demo-chain-gaps/119）。
- 不在 visibility-exception 存单据号或委托–包裹映射。
- 不改 `/v1/customer-tracking-view`、提交答复与操作者族的委托查阅读口。
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
  - 认证、授予各格与按包裹查一致（`401` / `403` / `503`）。
- **`cmd/parcel-api` 真库测试**：新口挂在首方族、带 `/v1`、要追踪查阅授予。
- **门禁**：Go 受影响包加 `internal/architecture`、`cmd/...` 真库测试过；`go build ./...`、`go vet ./...` 退 0。
- **演示**：一次性库灌种子后跑动线脚本，按运单号查得 `PARCELS_LISTED` 与接受前的 `AWAITING_ACCEPTANCE`，证据只记 `S`。
- 照「进 main 的步骤」第 1 步交非作者评审。

## 工作方式

只做 `docs/agents/workflow.md`「进 main 的步骤」第 1 步：作者在自己的 worktree 与分支上做完、自验、推分支、写完成记录；非作者评审与进 main 由协调方做。注释中文、无行号与跨文件计数；不手改 `docs/adr/README.md`、AT 盘点总览计数、机制清点。
