# `parcel-shipment` 解析键登记面不收 `CUSTOMER_SERVICE_RULE`：0007 / 0008 的必需依据白名单与 `commercialKindFrom` 名集各少一格——ADR-0136 让索赔资格规则按接受时闭包选用，这条缝今天在生产上到不了「已登记」

Category: bug
Status: resolved——**已进 main，2026-09-12 00:2x**（通道 1 接管会话重放：`main 1b06bb18` 之上**纯 ff 不换号** `7348bce6` / `9fa2ddc6` + 清点 `590282b0` + 本簿记笔；**证据层级如实记：本次没有非作者评审**——推送方接管会话即作者（原通道 3），全网无第二个会话，用户 00:2x 裁「作者两轴自评进 main、后补非作者评审」，自评全文与后继见「进 main 记录」）。此前 resolved——2026-09-12 00:1x 通道 3 作者完工（task-1c76951b；分支 `mcp3-psr09` 基远端 main `1b06bb18`，认领 `7348bce6`，代码 + 本完成记录同笔 `9fa2ddc6`）：迁移 0022 重加白名单收 `CUSTOMER_SERVICE_RULE`、`commercialKindFrom` 加一格、真库两例、ve-claims/04 绊线翻回两态皆经 PS 登记面；带 DSN 六包全 ok，0007 / 0008 零 diff；「必登」未动（归 PS owner，看法见完成记录）。等非作者评审 → 推送方重放进 main。此前 in-progress——2026-09-11 23:5x 通道 3 认领（树 `D:/tops/idp-parcel-mcp3-psr09`；迁移序号钉 0022）。此前 ready-for-agent——2026-09-11 23:3x 通道 1 推送方立票并直接转 ready（机制半边：加一格枚举与一道迁移，不涉任何租户实例）；取证锚 main `262e8c0a`；「要裁的」一条归 PS owner，不阻机制半边
Blocked by: 无（硬）。~~软阻：ve-claims/04 正在动手~~ **ve-claims/04 已 23:5x 进 main（通道 1 推送方记），软阻解除，可派**；它那条「已登记」态装配用例的负断言由本票做法 4 翻回真走 PS 登记面

## 缺口（取证于 `262e8c0a`，逐符号名）

- [ADR-0136](../../../docs/adr/0136-claim-rule-resolution-key-is-the-acceptance-time-commercial-resolution-reference.md) 决定三：`CustomerServiceRuleObject` 必须在接受时闭包的必需依据里，恢复动作是「去 PS 解析键登记面把它列进必需依据」；越权风险点 2 写的是 PS 今天「只在请求结算依据时强制合同维，要不要改『必登』归 PS owner」——**前提是可登、只是不必登**。
- 实测不成立，是**不可登**，两道闸：
  1. `migrations/parcel_shipment/0007_commercial_resolution_key.sql` 立的 CHECK `commercial_resolution_key_registration_bases_closed`、`0008_resolution_key_settlement_selector.sql` 重加的同名约束，必需依据白名单里没有 `CUSTOMER_SERVICE_RULE`（`git grep CUSTOMER_SERVICE_RULE -- migrations/parcel_shipment` 零）。
  2. `internal/parcelshipment/adapters/partycommercial/commercial_resolution_keys.go` 的 `commercialKindFrom` 名集里没有 `CustomerServiceRuleObject`（同目录 `git grep` 零）——即便行进了库，接受流 `FormResolutionKey` 也报 unknown kind。
- 后果：在 PS owner 开闸之前，没有任何租户能让接受时闭包采用客户服务规则版本，VE 索赔资格两维（[ve-claims/04](../../ve-claims-read-seams/issues/04-rule-resolution-key-source-needs-a-registration-face.md) 做法 2 的结果代数）在生产上永远停在「未采用客户服务规则 → 未登记」。ve-claims/04 的「已登记」态装配用例因此只能把键直给 PC `ResolveCommercialBasisHandler` 解、不经 PS 登记面（通道 1 23:3x 裁 (a)，见该票「裁决」）。
- 发现人：通道 3（ve-claims/04 中途报，23:4x 机时）；通道 1 复核两处 grep。

## 要做的（机制半边）

1. **迁移**：新迁移 `migrations/parcel_shipment/00NN_resolution_key_bases_accept_customer_service_rule.sql`（序号取当时 PS 最大 + 1，派单时钉），`DROP CONSTRAINT` + `ADD CONSTRAINT commercial_resolution_key_registration_bases_closed` 把 `CUSTOMER_SERVICE_RULE` 加进白名单——照 0008 改 0007 的形，**不改已施加的 0007 / 0008 一字**（业务迁移 checksum 按文件内容算）。
2. **名集**：`commercialKindFrom` 加 `CustomerServiceRuleObject` 一格，与 PC `pcdomain.CommercialObjectKind` 的 `String()` 对齐；反向 `String()`/译回若有同表一并补。
3. **用例**：PS `adapters/postgres` 真库一例——登一行必需依据含 `CUSTOMER_SERVICE_RULE` 与 `CUSTOMER_CONTRACT`，读回逐字同；PS `adapters/partycommercial` 一例——`FormResolutionKey` 对该行形成的键 `RequiredBases` 含 `CustomerServiceRuleObject`。`parcel-commercial` CLI 登记路径若有白名单 / 提示词封闭集（`registrationjson` 一族），同步加一格并把提示句原词对上。
4. **翻 ve-claims/04 的绊线**：`cmd/parcel-api/assemble_claims_test.go` `TestTheWiredClaimsReadTheRuleAdoptedAtAcceptanceThroughParcelShipment` 里对真登记面钉的负断言「含 `CUSTOMER_SERVICE_RULE` 的一行登不进去」在本票开闸后会红——那是有意的（通道 1 23:4x 裁留）：本票同笔把「已登记」态的两份闭包改为**经 PS 登记面登含 CSR + 合同的键、走接受形成回指与闭包**，删掉重建门造的那两份与负断言；用例头注里「PS 登记面收下客户服务规则那天…」那段随之改口。这是本票地盘的一部分，不算越界。
5. **不做**：不改「必登」——见要裁的。

## 红线

- 只加一格，不改既有格的语义；不动 `internal/partycommercial/**`、`internal/visibilityexception/**`。
- 不为任何租户预填含 CSR 的解析键（实例半边归登记册）。
- 注释中文、不写行号不写跨文件计数；迁移头注引 ADR-0136 决定三原句。

## 完成判据

1. `git grep -n CUSTOMER_SERVICE_RULE -- migrations/parcel_shipment` 命中新迁移；`0007` / `0008` 零 diff。
2. `commercialKindFrom("CUSTOMER_SERVICE_RULE")` 返回 `pcdomain.CustomerServiceRuleObject`、无 error；未知名仍报 error（既有用例不动）。
3. 真库一例登含 CSR 的行并读回；`FormResolutionKey` 一例形成含 CSR 的键。带 DSN PASS，无 DSN SKIP。
4. `gofmt -l` 空、`go build ./...` / `go vet ./...` 退 0；`go test -count=1` PS `adapters/postgres` + PS `adapters/partycommercial` + `cmd/parcel-commercial`（带 DSN）+ `./internal/architecture/...`；机制清点 tip 重生成（迁移 +1）。
5. 完成记录随最后一笔代码同笔；ve-claims/04 票面「裁决」末条的后继句由本票进 main 时改口（推送方代）。

## 地盘

`migrations/parcel_shipment/`（新文件一个）、`internal/parcelshipment/adapters/partycommercial/commercial_resolution_keys.go` 及其测试、PS `adapters/postgres` 解析键登记册测试一例、`cmd/parcel-commercial` 登记路径若有封闭集则一处、**`cmd/parcel-api/assemble_claims_test.go` 一条用例的夹具与负断言（做法 4，共享接线测试文件，动前占号）**。**不动** `0007` / `0008`、`internal/partycommercial/**`、`internal/visibilityexception/**`、`cmd/parcel-api/assemble_claims.go`。

## 要裁的

1. **CSR 是否随合同维一并「必登」**（ADR-0136 越权风险点 2 原题，本票只让它「可登」）：PS `commercial_resolution_keys.go` 今天只在请求结算依据时强制合同维（`validateSettlement`）。选项 (a) 不必登——租户没登 CSR 时索赔两维如实答未登记（今天 ve-claims/04 落的就是这一格）；(b) 与合同维同强度必登——没登就拒登整行，索赔两维在接受时就有答案。归 PS owner；不阻做法 1–3。

## 参照

ADR-0136 决定三 / 越权风险点 2；[ve-claims/04](../../ve-claims-read-seams/issues/04-rule-resolution-key-source-needs-a-registration-face.md)「裁决」与 23:3x 中途裁；[07](07-commercial-resolution-reference-by-parcel-read-face.md)（同族：PS 为 VE / TF 开的回指读口）；[syn-wall-door-audit 03](../../syn-wall-door-audit/issues/03-network-definition-register-no-writer-no-resolver.md) 件 2（`commercial_resolution_key` 登记面经 `parcel-commercial` CLI）；`internal/platform/migrate`（已施加迁移 checksum 纪律）。

## 完成记录

（通道 3 · task-1c76951b · 2026-09-11 23:5x–2026-09-12 00:1x · 树 `D:/tops/idp-parcel-mcp3-psr09`，分支 `mcp3-psr09` 基远端 main `1b06bb18`，未 rebase。）

**逐笔**：认领 `7348bce6`（Status → in-progress）；代码 + 本完成记录同一笔（SHA 见完工报）。

**动过的文件**：新增 `migrations/parcel_shipment/0022_resolution_key_bases_accept_customer_service_rule.sql`（照 0008 改 0007 的形 DROP + ADD `commercial_resolution_key_registration_bases_closed`，白名单加 `CUSTOMER_SERVICE_RULE` 一格；头注引 ADR-0136 决定三原句「客户合同版本与客户服务规则版本都必须在接受时闭包的必需依据里」，单行逐字；`migrations/migrations.go` 按目录 `embed` 收，不动）；`internal/parcelshipment/adapters/partycommercial/commercial_resolution_keys.go` `commercialKindFrom` 名集加 `CustomerServiceRuleObject` 一格 + 头注（同文件无反向译回表——`Register` 用 `kind.String()` 写行，PC 的 `String()` 就是反向）；新增 `internal/parcelshipment/adapters/postgres/commercial_resolution_key_store_test.go`（真库一例：登含 `CUSTOMER_CONTRACT` + `CUSTOMER_SERVICE_RULE` 的行、读回 `required_bases` 逐字同、其余维照登记值、结算 / 信用维空）；`commercial_resolution_keys_test.go` 加一例（`FormResolutionKey` 对该行形成的键 `RequiredBases` 含 `CustomerServiceRuleObject` 与 `CustomerContractObject`，最小身份成立，结算 / 信用选择器空）；`cmd/parcel-api/assemble_claims_test.go` 只动 `TestTheWiredClaimsReadTheRuleAdoptedAtAcceptanceThroughParcelShipment` 一条（做法 4，占号后动）。**第三道闸不存在**：`cmd/parcel-commercial/translate.go` 的同名 `commercialKindFrom` 早已含 `CustomerServiceRuleObject`，`registrationjson` 一族无封闭提示词，CLI 侧零改。

**判据逐项**：
1. ✓ `git grep -n CUSTOMER_SERVICE_RULE -- migrations/parcel_shipment` 只命中 0022；`git diff 1b06bb18 -- 0007 0008` 零。
2. ✓ `commercialKindFrom("CUSTOMER_SERVICE_RULE")` → `CustomerServiceRuleObject`（经 `FormResolutionKey` 用例证）；未知名仍 error，既有用例（含 `PRICE_RULE` 拒、集外拒）一字未动、全 ok。
3. ✓ 真库两例带 DSN PASS、无 DSN SKIP（`pgtest.Pool` 跳过）。
4. ✓ `gofmt -l` 空；`go build ./...` / `go vet` 退 0；带 DSN `go test -p 1 -count=1`：`internal/platform/migrate`（0022 随全套迁移重跑）、PS `adapters/postgres`、PS `adapters/partycommercial`、`cmd/parcel-commercial`、`cmd/parcel-api`、`internal/architecture/...` 全 ok（00:07，55432 占 / 释已报通道 1）。机制清点由推送方在干净检出重生成（迁移 +1），本记录不预报数字。
5. ✓ 本记录随代码同笔；ve-claims/04「裁决」末条的后继句归推送方进 main 时改口。

**做法 4 · 翻 ve-claims/04 的绊线**：负断言「含 `CUSTOMER_SERVICE_RULE` 的一行登不进去」删；「键直给 PC 真解析器」那段删。解析键登记面按（租户，客户账户）一行，两种登记行因此要两个货主客户账户：委托二 / 索赔二改属 `SYN-CUSTOMER-2`（`secondSubmissionCommand` 换客户），客户一登含客户合同 + 客户服务规则的行、客户二只含客户合同；两态都经 `FormResolutionKey` 成键 → PC 真 `ResolveCommercialBasisHandler` 解出并固定闭包 → PS 真 `ShipmentRequests` Decide → Save。用例头注「PS 登记面收下客户服务规则那天…」改口为两态皆经登记面、点名 0022 与 `commercialKindFrom` 各加一格并引本票路径。VE 两本册按（租户，合同）作答，两个客户共用同一合同声明，无需加登。`cmd/parcel-api/assemble_claims.go` 零改。

**「必登」的看法**（票面「要裁的」1，归 PS owner，本票未动）：倾向 (a) 不必登。理由：PC CONTEXT 说合同版本恒在，才有 ADR-0133 / 0136 对「闭包未采用客户合同 → error」的判法；客户服务规则是 ADR-0104 决定三「对首发两项都无客户差异就是不登记」的那一类——一个租户可以合法地没有任何客户服务规则版本，登记面若强制必登，接受流会在这类租户身上整行拒登，把「没有差异规则」变成「接受不了委托」；VE 侧对「未采用 → 未登记」已有诚实的停格（ve-claims/04 态一）。反方 (b) 的收益是索赔两维在接受时就有答案，但那要先有「每个租户都必须发布一版客户服务规则」这句商业语言，今天没有。

**判断项**：
1. 两个客户账户共用同一份 VE 索赔资格声明（`RegisterClaimEligibility` 按合同一行）：这是 VE 册的既有键形，本用例只是用到它；若评审认为两客户应各登一份更贴近生产，加一行即可，断言不变。
2. `commercialKindFrom` 头注提到「迁移 0022 同笔」——是本票内的同笔事实，不是跨文件计数；日后若再加格，头注的 ADR 指向仍成立。

## 进 main 记录（通道 1 接管会话 = 作者，2026-09-12 00:1x–00:2x）

- **点名**：00:14 广播、截止 00:17，0 个应答（`list_sessions` 只有通道 1 / 3 在线，两者是同一会话；通道 2 离线）。非作者评审排队给通道 2（task-50cd7049，钉 `9fa2ddc6`）——它上线即接，作为**后补**非作者评审；用户 00:2x 告知「现在只有你自己了」并裁「作者两轴自评、如实记、后补非作者评审」进 main。
- **作者两轴自评（钉 `9fa2ddc6`，与完成记录同一双眼睛，只能证「没漏检」不能证「没盲区」）**：Standards——阻断 0；非阻断 0；无发现：迁移 0022 照 0008 改 0007 的形 DROP + ADD 同名约束，白名单既有八项原样 + `CUSTOMER_SERVICE_RULE`，`PRICE_RULE` 仍不在；0007 / 0008 零 diff；头注引 ADR-0136 决定三首句单行逐字；`commercialKindFrom` 与 `pcdomain.CommercialObjectKind.String()` 对齐；注释中文无行号无跨文件计数；不为任何租户预填键。Spec——阻断 0；非阻断 1：判断项 1（两客户共用同一份 VE 索赔资格声明）是 VE 册按（租户，合同）作答的既有键形，接受；无发现：判据 1–5 逐项如完成记录；做法 4 只动一条用例、`assemble_claims.go` 零改；CLI 无第三道闸（`translate.go` 同名函数含该类，核过）。
- **重放**：`merge-base(main, mcp3-psr09) = main` → 纯 ff 不换号。`%TEMP%\idp-replay-psr09` detached `9fa2ddc6`，清点在其上重生成 `590282b0`（PS 测试 172→173、合计 919→920；迁移 parcel_shipment 21→22、合计 167→168；生产 / 端口 / 消费缝 / 路由表零差）。
- **验证钉 `590282b0`**：`gofmt -l` 空；`go build ./...` / `go vet ./...` 退 0；00:18 占号，带 DSN `go test -p 1 -count=1 ./...` **110 ok / 0 FAIL / 15 无测试 / 0 cached**（00:19:22→00:21:30），00:21 释号。
- **进 main**：本簿记笔（票 09 Status + 本节；ve-claims/04「裁决」5 后继句改口；tasks.md 节）在 `590282b0` 之上；`ls-remote` 核 `1b06bb18` 未动 → 共享树 `merge --ff-only` → `push <sha>:main`。SHA 见推送后 tasks.md。
- **后继**：① 非作者评审后补（task-50cd7049 留在通道 2 队列，钉 `9fa2ddc6`，已进 main；评审结论回落本节）；② 「必登」归 PS owner（「要裁的」1）；③ ADR-0136 越权风险点 2 补 `262e8c0a` 实测一句归 ADR owner。

## Comments

- 2026-09-11 23:3x · 通道 1 推送方：立票并直接 ready。**只写票面，未动代码。** 能力边界：两道闸由通道 3 在 ve-claims/04 中途实测、通道 1 只复核了两处 `git grep`；`parcel-commercial` CLI 那一侧有没有第三道闸（封闭提示词集）没查，写进做法 3 由实施者量。
- 2026-09-12 00:1x · 通道 3（task-1c76951b）：作者完工。迁移 0022 + `commercialKindFrom` 一格 + 真库两例 + ve-claims/04 绊线翻回（两态皆经 PS 登记面，委托二换客户二）；CLI 侧无第三道闸（`translate.go` 同名函数早已含该类）。带 DSN 六包全 ok，0007 / 0008 零 diff。「必登」倾向 (a) 不必登，理由见完成记录。分支已推 origin；等非作者评审后重放。
