# 22 PS 客户面 HTTP 契约没按已接受的 ADR 落：`!found` 答 500 而非 404，`/v1` 与问题详情没落地

Category: bug
Status: in-progress——完工待评审与进 main，分支 claude-code22
Blocked by: 无
出处：[audit/ps.md](../audit/ps.md) PS-07、PS-24，取证 [audit/ps-code.md](../audit/ps-code.md) 第 3、12 条。两条同一上下文、同一根因——客户面 HTTP 适配器的契约形状没跟上已接受的契约 ADR——合成本票（审计钉 `6b0074d0`；2026-10-21 按 main `67438a0f` 复核，现象仍在）。

## 现象

1. **`!found` 不是 404**（PS-07）：客户面撤回、资料修订、受控补充等命令口把「查无委托」与其余没形成答案的失败一并答 5xx `NO_ANSWER_FORMED`；逐包裹取消口在 `!found` 时交 `CancellationNotAccepted`，即 `200` + `REQUEST_NOT_ACCEPTED`，把「查无」与「字段缺」并成一格。
2. **`/v1`、问题详情、端点描述符都没落地**（PS-24）：提交口仍是 `/shipment-requests`，以 `application/json` 写出自造的 `problemResponse`；仓内没有契约生成器，也没有 `problem+json`；ADR-0140 接受后没有立实施票。

## 证据

- 决定：
  - ADR-0022 Decision：「**`!found` 一律映射为对应上下文的`统一不可见结果`，用 `404`。**」，Consequences：「接 HTTP 时由适配器映射为 `404`，不得原样 500。」这一条不在 ADR-0022 已部分停用的范围内。
  - ADR-0140 决定一（端点描述符与生成器）、决定二（`/v1`）、决定三（`application/problem+json`）；后果节：「首方族就位后，客户面提交口的路径变为 `/v1/shipment-requests`」。
- 代码：
  - `internal/parcelshipment/adapters/http/withdraw_shipment_request.go` 头注：「其余没形成答案的失败一并回 5xx `NO_ANSWER_FORMED`——不细分」；`amend_customer_source_data.go`、`form_new_submission_version.go`、`complete_manual_review.go` 同款。头注把 `404` 读成了「拆开统一不可见结果」——统一不可见要的是「不存在」与「他人的」同答，`404` 本身就是那个同答。
  - `application/cancel_parcel.go`：`!found` 时 `CancelParcelResult{outcome: CancellationNotAccepted}`。
  - `cmd/parcel-api/endpoints_submission.go`：`"/shipment-requests"`；`adapters/http/submit_shipment_request.go` 写 `problemResponse`。
  - 撤回、资料修订等口今天在生产装配下仍以 `UnconfiguredIntake{}` 起步，首方 Intake 逐口换上时即生效。

## 三类

机制（HTTP 适配器契约）。产品策略：契约版本与问题详情的形状（ADR-0140 已定）。租户取值：无。

## 规模

M–L：路径加 `/v1`（旧路径是否在过渡期双挂见「要裁的」）；问题详情改 `problem+json` 并带 `requestId`；客户面命令口 `!found` 统一 `404` 加上下文统一不可见码；ADR-0140 决定一的描述符与生成器可拆成后续一笔，但路径与问题体须在基线前落。

## 最小的诚实修法

1. 客户面命令口（撤回、资料修订、受控补充、逐包裹取消）`!found` 一律 `404` + 上下文统一不可见码；取消口把「查无」与「字段缺」分开。改掉那几处头注。
2. 客户面提交口及同族口迁到 `/v1/...`，问题体改 `application/problem+json`。
3. 登记进 [first-party-api/spec.md](../../first-party-api/spec.md) 的子票表（ADR-0140 的实施票），已在那份规格补指针。
4. 操作者族（`403 OPERATOR_NOT_GRANTED` 的探针同答，ADR-0100）不在本票改；若那是有意的，修订票 docs-consistency/06 在 ADR-0022 的 Status 行补注适用范围。

## 要裁的

- 旧路径 `/shipment-requests` 在基线前直接改名，还是双挂一段过渡期。首发前没有真实集成方，推荐直接改名（管理台提交页随改），作者可代决。
- 描述符与生成器（ADR-0140 决定一）是否与路径、问题体同票：推荐拆为后续票，本票只做基线前必须定形的两件。

## 文档自洽 06 复核补记（2026-10-21）

- 「最小的诚实修法」第 4 条操作者族一问：不需要在 ADR-0022 的 Status 行补注。`403 OPERATOR_NOT_GRANTED` 是接入身份层在指名任何对象之前的答复（ADR-0100 决定四：操作者不在册、不绑这个租户或无此能力面的授予），ADR-0022 的 `!found` 一条管的是通过接入之后、按对象指名查不到的那一格，两者不在同一层。
- 资料修订口同族的另几格：委托未接受、修订意图未声明、内容与范围不配，今天同样以错误上抛、答 5xx `NO_ANSWER_FORMED`；它们是调用方的错，照 ADR-0022 属 4xx。随本票第 1 条一起改，UC-PS-002「给开发的交接」已指本票。
- 首方契约文档写明不收客户声明的来源发生时间、信封以接收时刻记（[ADR-0258](../../../docs/adr/0258-the-first-party-family-declares-no-source-occurrence-time-and-records-the-receipt-instant.md)），随本票第 2 条的契约形状一起落；代码取法今天已是如此，不改。
- 取消口「查无与字段缺分开」一条：UC-PS-006 新立的结果语义契约把 `REQUEST_NOT_ACCEPTED` 限到字段缺，查无、跨租户、跨客户与成员出界归统一不可见，「给开发的交接」已指本票。

## code/05 补记（2026-10-21）

- 客户追踪视图口（`/customer-tracking-view`）的响应体已由 [code/05](./05-customer-tracking-view-overwrites-published-versions.md) 删去 `basedOnProjection`：对客契约删字段，客户的采用版本只由 `version` 承担（code/05 代决 1、越权风险点 1）。首方契约在基线之前定形，本票落契约文档时按删后的形状写，不再列这一格。

## 完成记录（2026-10-21，分支 claude-code22）

用户授权原话「你作为业务和系统专家自决……解决所有的问题」；下列「代决」由作者会话作出，部分经协调方裁定（标「协调方裁」），其余待 PS、VE、NO owner 复核。`cmd/parcel-api/endpoints_test.go` 与 `endpoints.go` 那一句注释的改动经协调方放行（ADR-0222 不让动它们是为避免并行撞文件，本票期间 `cmd/parcel-api` 只有本会话在动），只改了放行的那几处。

### 做了什么

- **`!found` 答 404（最小修法第 1 条）。** 应用层立具名错误 `ErrShipmentRequestNotVisible`（仍是 `domain.ErrInvalidShipmentRequest` 的一种），撤回、资料修订、受控补充、逐包裹取消四个客户面命令口与复核完成、主动拒绝、授权处置三个操作者命令口查无的委托都上抛它，HTTP 答 `404 SHIPMENT_REQUEST_NOT_VISIBLE`（与委托查阅同一枚）。取消口的查无、编号不符、跨租户与成员出界从 `200 REQUEST_NOT_ACCEPTED` 拆出，那一格只剩字段缺。资料修订受理阶段的三格调用方之错各立具名错误：意图未声明、内容与范围不配答 `400 MALFORMED_REQUEST` 带 `detail`，委托未接受答 `409 SHIPMENT_REQUEST_NOT_ACCEPTED`。
- **`/v1`（第 2 条）。** 首方客户族五口在各自的特性文件里改成 `/v1/shipment-requests`、`/v1/customer-tracking-view`、`/v1/node-operations/delivery-pre-advices`、`/v1/node-operations/delivery-pre-advice-withdrawals`、`/v1/node-operations/delivery-handover-conclusions`；旧路径直接改名、不并挂，答 404。新装配测试断言首方族每行带 `/v1`、其余各族一行都不带。
- **问题详情（第 2 条）。** `internal/platform/httpapi` 收 `WriteProblem` / `WriteProblemWithDetail`：`application/problem+json`，成员 `type`（`urn:idp-parcel:problem:<code>`）、`title`（由码机械派生）、`status`、`code`、`requestId`（取 `middleware.RequestID`）；`detail` 只随 4xx，403、404 与 5xx 交来也不写出。首方族五口与 PS 客户面四个命令口改用它；PS 与 NO 包里各格映射只留一份，形状由处理器构造时选（`problemWriter`），操作者族与作业事实族各口照旧。
- **`endpoints_test.go`（放行的三处）。** `problemCode` 两种问题体都认；`TestUnconfiguredDoesNotSwallowTheMethodGate`、`TestAssembledEndpointsIgnoreSelfReportedIdentity` 改用 `/v1/shipment-requests`，后者比较前去掉逐请求的 `requestId`。`endpoints.go` 只改那句会过时的注释。
- **admin-web。** 提交页改发 `/v1/shipment-requests`；三处 exchange 的错误码解析统一经 `src/pages/problem-body.ts`，两种形状都认；取消结果与两枚新码的说明随改。
- **演示与文档。** `scripts/demo-seeds` 两个提交脚本与 README、`docs/design/synthetic-demo-journey-script.md`（带日期的实走记录不改）、业务模块指南的首方口改指 `/v1`。新立 [首方客户 API 契约](../../../docs/api/first-party-api-contract.md) 并登 `docs/README`：写明 ADR-0258「首方契约不收客户声明的发生时间」、ADR-0140 决定五「未识别 `outcome` 交人」与决定六的弃用窗口；客户追踪响应体按 code/05 删后的形状写。UC-PS-001、002、005、006 指向本票的缺口注改为已补。
- **后续票。** 操作者族、作业事实族与集成族迁问题详情，集成族 `/v1`，端点描述符、生成器与 `/docs` 入口，立为 [code/80](./80-operator-and-integration-families-still-answer-the-legacy-problem-shape-and-the-contract-is-not-generated.md)（R3）。

### 代决

1. **旧路径直接改名，不留并行期**（协调方裁）。ADR-0140 决定五：首版发布前没有基线；决定六：弃用窗口只对已发布端点。
2. **问题体成员与 `detail` 边界**（协调方裁）。`type` 取 URN 而不是 https 地址：文档页落在哪个域名今天没人定过，先写一个域名就是替部署做决定，日后换域名又成破坏性变更。`title` 由码机械派生，不另立码到短语的表。
3. **`problem+json` 只及首方族五口与 PS 客户面四个命令口**（协调方裁）。形状随处理器、前缀随装配族：四个命令口今天挂未配置即拒、不属首方族，不带 `/v1`，但问题体已是首方契约的形状，首方 Intake 换上时只差前缀。
4. **客户追踪视图查无照旧答 `200 VIEW_NOT_FOUND`**（协调方裁）。ADR-0246 决定一定的是业务结果格，不改成 404。
5. **操作者三口（复核完成、主动拒绝、授权处置）的查无一并改 404。** 票面第 1 条只列客户四口，但证据点了复核完成，ADR-0022 的 `!found` 一条不分族。
6. **资料修订「委托未接受」答 409。** 仓内此前没有 409；答 400 会与 `MALFORMED_REQUEST`（改请求才会好）并成一格，而这一格要换的是路（受控补充）。意图、内容两格用 `MALFORMED_REQUEST` 加一句固定英文 `detail`，不另立码。
7. **契约文档手写横切规则，不写逐端点字段。** ADR-0140 否决的是手写 OpenAPI 作第二事实源；本文只把已接受 ADR 的横切规则写成调用方读得懂的一份，字段以处理器结构体为准，生成物落地后以生成物为准（code/80）。
8. **`form_continued_attempt_decision` 查无仍答 5xx**（协调方裁：不在本票）。它查的是租户级登记册，与委托不是同一对象。

### 越权风险点

1. `type` URN 与 `title` 的写法进首个兼容性基线；改它是破坏性变更。
2. 资料修订的 409 与两句 `detail` 措辞进基线；客户端要多认一个状态码（处置动作照 4xx 出队交人不变）。
3. admin-web 复核完成、主动拒绝、授权处置三页此前对查无拿到 noAnswer（500），现在拿到 callerProblem（404）。
4. 同一包里两种问题体并存到 code/80 落地：PS、NO、VE 三包的客户面与操作者面形状不同；测试辅助与 admin-web 的解析两种都认，code/80 时删旧分支。
5. 首方 Intake 换上四个客户面命令口时路径要迁到 `/v1/shipment-requests/...`；`TestOnlyTheFirstPartyFamilyCarriesTheMajorVersion` 会在挂上首方族而没加前缀时红。
6. 契约文档里「今天开着的口」一表与能力授予码是手抄的，生成器落地前没有测试读它（code/80 第 3 条收）。

### AT

- `AT-PS-106` 由④移入①；`AT-PS-075`、`AT-PS-090` 的①行补客户面命令口真库用例。总览计数未改。

### 验证（分支 claude-code22，rebase 到 main `be6b5ad20` 之后）

- `go build ./...`、`go vet ./...`、`gofmt -l` 干净。
- `TMPDIR=/home/tops/.claude/jobs/1be2e64f/tmp scripts/test-full.sh ./internal/parcelshipment/... ./internal/visibilityexception/... ./internal/nodeoperations/... ./cmd/parcel-api/ ./cmd/parcel-dispatch/ ./internal/architecture/... ./migrations/... ./internal/platform/httpapi/... ./internal/accessidentity/...`：55 个包 ok，退出码 0。
- 新用例另跑 `-count=3`（含真库 `TestCustomerCommandsNamingAnAbsentRequestAnswerNotFoundOnTheWiredOrchestrations` 与 `TestEveryAdminWebPathIsOnTheParcelAPIEndpointTable`）：全过。
- admin-web 三道门：`tsc -b --noEmit` 退 0；`node scripts/run-tests.mjs` 601 过 0 失败；`pnpm exec vite build` 成功。
- 一次性库 `code22seed`：`seed.sh` 退 0；起 `parcel-api` 与一只私有发行方后，`submit-one-shipment.sh` 经 `/v1` 答 201 并从列表与详情读回，`submit-one-pre-advice.sh` 答 `DELIVERY_PRE_ADVICE_REGISTERED`；旧路径答 404；坏令牌、自报账户、撤一份不存在的预报各答 401、400、404，媒体类型 `application/problem+json`、带 `requestId`。只记 `S`；库与私有发行方已删。
