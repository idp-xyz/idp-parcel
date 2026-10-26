# 07 撤回口经首方渠道接通

Category: enhancement
Status: in-progress——2026-10-26 作者完工，待隔离评审与进 main，分支 claude-fpa07（代码最后一笔 f34131d90）。此前 in-progress——2026-10-26 会话 `tops-20` 认领（前置 code/36、code/44 已进 main），分支 `claude-fpa07`。此前 ready-for-agent——R1（上线前必修，用户 2026-10-26 定首发要这一口）。
Blocked by: 06；docs-consistency/code/36、code/44（tops-26）
出处：用户 2026-10-26「都需要」（首个租户上线时客户经首方 API 要能撤回、取消、修订、补充、索赔）、「每个客户系统只代表它自己的账户（直客），只考虑这个吧」、「首发不要客户状态读口」、「首发不做限流」；技术取舍由会话 `tops-20` 代决（用户「你帮我决定吧」）。 计划见 [spec](../spec.md)「五口上线」与 [摸底报告](../customer-faces-scoping-2026-10-26.md)。

## 做什么

首方 `IntakeWithdrawal` 实现、`endpoints_withdrawal.go` 换真 Intake、装配；他账户标识答找不到不泄露存在；演示走一次撤回。

## 完成判据

- [x] 首方客户凭证经 `/v1` 该口提交，演示租户答成功；他账户目标答找不到、不泄露存在；未授予答 403；重放答同一结果（生产装配真库用例）。
- [x] 新 AT 行与 AT 盘点①表行；Go 门禁；全量用 `scripts/test-full.sh`；种子 `seed.sh` 退 0。

## 规模

M

## 完成记录

起点 `3f34ded92`，分支 `claude-fpa07`，代码提交 `f34131d90`。

### 改了什么
- `CustomerWithdrawalIntake`（`customer_withdrawal_intake.go`）：令牌、铸造（`MintWithdrawal`）、封闭解码、译 `WithdrawShipmentRequestCommand`；`NewCustomerWithdrawalHandler` 在编排前按原提交身份找委托、补齐委托号与当前提交版本、核对客户带的委托号。
- `DirectCustomerWithdrawalAuthorizer`（PS `adapters/accessidentity`）：直客 1:1 授权，授权引用 `FIRST-PARTY-DIRECT-CUSTOMER-BINDING/<租户>/<账户>`。
- `CanonicalizeCustomerWithdrawal`（`PWD-1`）；`endpoints_withdrawal.go` 换真 Intake、face 为 `customerClientFace(SHIPMENT_REQUEST_WITHDRAWAL)`；`buildCustomerWithdrawalOrchestration` 与 `buildWithdrawalOrchestration` 共用 `assembleWithdrawalOrchestration`；旧无前缀路径出 `customerFaceCommandPatterns`、进 `TestTheUnversionedFirstPartyPathsAreGone`。
- 契约「委托撤回」一段（含重放语义与撤回后读回）；UC-PS-005 验收 `AT-PS-134` 至 `137`；AT 盘点①四行加追记。管理台 `api.ts` 撤回路径改发 `/v1`（架构门禁要求发向端点表上的路径，先例同提交口）。无迁移，无 PS 应用层改动。

### 本口形状字段（PWD-1）
`accessRequestId`、`originalAccessRequestId`、`reason`（结构化原因代码，必填）、`shipmentRequestId`（可不带）。摘要含原提交标识、委托号（带与不带不同）、原因。

### 代决与越权风险点
1. 载荷不收 `submissionVersionId`：提交版本由 Handler 取委托当前版本（释放本就放各版占用）。
2. 不带委托号时靠 Handler 补；委托查不到或号不符在进编排前直接答找不到，因此这一格不留来源保全（本就是调用方的错，编排里也随事务回滚）。委托读不动答 5xx，原样重放。
3. 授权器不问任何外部权威，只能被首方撤回口装配使用；代理人 1:N、运营侧要自己的授权器。风险点：被别处误复用即等于默认放行。
4. Handler 读委托与编排内再读之间提交版本可能换代（并发补充），只影响授权询问与续办引用里的版本号，不影响撤回与释放。
5. 提交回执不带委托状态，撤回后原提交重放仍答 `EXISTING_RESULT` 原版本（用例固化，契约写明）；撤回态经撤回重放读回。
6. 管理台撤回页的草案形状（`originalRequestKey`、`requesterReference` 等）与首方载荷不同，本票只改了路径，页面接首方载荷是后续事（该页此前也走不通：口挂未配置）。
7. AT 号 134–137，已核全仓与各 claude- 分支不撞。
8. 契约已写：未获授权、不允许、等复核的请求条件变了原样重放可能形成决定；撤回未决可重放；已决态重放答已有决定；并发输的一路答 `STALE_SHIPMENT_REQUEST_REVISION` 未决、需再重放。

### 判据对用例
`AT-PS-136`（`TestACustomerClientWithdrawsThroughTheFirstPartyLineAgainstARealDatabase`）：成功 201、重放 200 同一撤回、换标识再撤读回原撤回、他账户与不存在 404 同字节、委托号不符 404、未授予 403、自报字段与缺原因 400、来源冲突三格；`AT-PS-137` 同函数末段；`AT-PS-134`/`135` 为单元。

### 自验
含 PG：`go build ./...`、`go vet`（cmd/parcel-api、parcelshipment）、`gofmt` 无输出；`go test -count=1 -p 1` 过 `./internal/parcelshipment/...`、`./internal/accessidentity/...`、`./cmd/parcel-api/...`、`./internal/architecture/...`、`./migrations/...`、`./tools/...`。`at-coverage-counts -check` 只报生成物过期。未改种子，未跑 seed.sh；未跑管理台前端测试。

### 评审收尾（tops-20 评审，无阻断）
- 防误装：`TestTheDirectCustomerWithdrawalAuthorizerIsBuiltOnlyByTheFirstPartyWiring`（internal/architecture）断言 `DirectCustomerWithdrawalAuthorizer{` 在非测试源码里只由 `cmd/parcel-api/assemble_withdrawal.go` 构造；选门禁不选非导出，因为首方装配在 cmd 包、授权器在 PS 适配器包，非导出过不了包界。
- 契约补一句：重放须逐字节同载荷，补带 `shipmentRequestId` 答 `SOURCE_CONFLICT`。
- 越权风险点 9：管理台撤回页草案形状与首方载荷不符，本票只改路径，页面接首方载荷归收尾票 12 或运营侧另议。
