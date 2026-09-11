# `parcel-shipment` 解析键登记面不收 `CUSTOMER_SERVICE_RULE`：0007 / 0008 的必需依据白名单与 `commercialKindFrom` 名集各少一格——ADR-0136 让索赔资格规则按接受时闭包选用，这条缝今天在生产上到不了「已登记」

Category: bug
Status: in-progress——2026-09-11 23:5x 通道 3 认领（task-1c76951b；分支 `mcp3-psr09` 基远端 main `1b06bb18`，树 `D:/tops/idp-parcel-mcp3-psr09`；迁移序号钉 0022）。此前 ready-for-agent——2026-09-11 23:3x 通道 1 推送方立票并直接转 ready（机制半边：加一格枚举与一道迁移，不涉任何租户实例）；取证锚 main `262e8c0a`；「要裁的」一条归 PS owner，不阻机制半边
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

## Comments

- 2026-09-11 23:3x · 通道 1 推送方：立票并直接 ready。**只写票面，未动代码。** 能力边界：两道闸由通道 3 在 ve-claims/04 中途实测、通道 1 只复核了两处 `git grep`；`parcel-commercial` CLI 那一侧有没有第三道闸（封闭提示词集）没查，写进做法 3 由实施者量。
