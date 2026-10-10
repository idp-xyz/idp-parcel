# 11 集成客户端册与客户端凭据校验：外部结果与资金事实的信任入口

Category: enhancement
Status: 完工，待评审与重放——2026-10-10 通道 4 交卷（接手 task-201a7d4a；代码 tip `a259ffde`）。此前 in-progress——2026-10-09 通道 4 认领（通道 1 派单 task-9c89948b），隔离 worktree `idp-parcel-mcp4-oc11`、分支 `mcp4-oc11`（基 `187dccd2`）。此前 ready-for-agent——2026-09-24 随 ADR-0149 立（用户授权通道 4 自决）
Blocked by: 02
父票：[psb/15](../../product-strategy-boundary/issues/15-operator-channel-per-adr-0100.md) 丙轨实施
地盘：`internal/accessidentity`（集成客户端册、客户端凭据令牌校验、集成客户端信封）与其迁移、受控登记 CLI、参数登记册增「集成客户端与凭据」一行。
出处：[ADR-0149](../../../docs/adr/0149-business-command-faces-split-into-frontline-operator-and-integration-client-families.md) 决定三前两条。

## 做什么

1. 集成客户端册：客户端标识绑定唯一租户与一个来源身份，授予按事实类型登记、可撤销、带生效区间；凭据本体不入库，只存凭据引用。
2. OAuth 2.0 client credentials 令牌校验，方式同 02；可选 mTLS 绑定。
2a. 外部资金事实的更正口 `/settlement-external-funds-fact-correction-registrations` 随原事实归本族（2026-09-24 随 [ADR-0151](../../../docs/adr/0151-unassigned-command-faces-get-their-families.md) 决定四补）：与首登口同一份认证与授予。
3. 集成客户端信封：租户、客户端、来源身份、授予集；与操作者信封、客户来源信封分型，编译期不可互换。

## 完成判据

- 客户端未登记、授予不含该事实类型、区间外、令牌无效各答其格（带测试）；参数登记册那一行已增。
- 随 [ADR-0150](../../../docs/adr/0150-synthetic-tenant-is-treated-as-a-real-tenant-and-isolated-form-retires-per-face.md) 决定三（2026-09-24 补）：换上集成客户端 Intake 的各口（今天经写开关放行的外部结果、监管凭证登记、外部资金事实等）同一笔撤下隔离放行，隔离放行用例改写为真渠道答复格用例。

## 完成记录（2026-10-10，通道 4，分支 `mcp4-oc11`；待非作者评审与重放；证据只记 `S`）

`b6825dc2` 封存现场自验保留：`go test -count=1 ./internal/accessidentity/...`（含 `adapters/postgres`，DSN `127.0.0.1:55432`）通过，未再改那一笔。

| 笔 | 内容 |
|---|---|
| `3d3e7442` | 领域、令牌校验与信封铸造。客户端未登记、授予不含该事实类型、区间外与已撤销同答 `ErrIntegrationClientNotGranted`；令牌无效答 `ErrCredentialRejected` |
| `6c15a27a` | 册落库（`access_identity` 0003）、适配器与 `parcel-access-register` |
| `87a14270` | 参数登记册 `PAR-INT-09`。当前登记保持「待提供」；凭据与客户端实例不填，答未配置 |
| `b6825dc2` | 封存：两族授予的生效判断收成 `effectiveAt`。自验后保留 |
| `a259ffde` | 四口换集成客户端 Intake，并撤下其中三口的隔离放行 |

换上的口：`/customs/external-results`、`/customs-regulatory-credential-registrations`、`/settlement-external-funds-fact-registrations`、`/settlement-external-funds-fact-correction-registrations`（更正与首登同一份 `EXTERNAL_FUNDS_FACT` 授予）。前三口从 `isolatedWriteAdmittedCommandLines` 撤下；隔离放行的真库业务结果用例改为真渠道答复格用例。发行方三件未设时这四口仍答 `403 ACCESS_CHANNEL_NOT_CONFIGURED`。令牌无效答 `401 INTEGRATION_CLIENT_CREDENTIAL_REJECTED`；未登记、授予不含该类、区间外答 `403 INTEGRATION_CLIENT_NOT_GRANTED`；授予在、准入对照未登答 `403 OUTSIDE_ADMISSION_SCOPE`。

`a259ffde` 自验（DSN 同上）：`go build ./...`、`go vet ./...`；`go test -count=1` 跑了动过的包（关务与结算的 `adapters/http`、`adapters/accessidentity`）、`./cmd/...`、`./internal/architecture/...`。自 `b6825dc2` 以来动过 `.go`，没有动 `.sql`。评审不在本票自评。

## Comments

**评审 ← 通道 2 · 钉 `3de118e7` · 11:28**（基 `187dccd2`，隔离检出；`b6825dc2` 封存笔照常看过。非作者。）

**Standards**

阻断：无。

非阻断：无。

无发现：凭据与客户端实例没有写死成生产默认。`buildIntegrationClientCredentialVerifier` 三件环境变量齐备才建校验器，缺一件拒启动，三件都空答未配置。`integrationClientBindingFrom` 见 `certificateBoundTokenRequired` 缺席即拒，不代填成不要证书绑定。迁移 `0003_integration_client_register.sql` 不种行，`certificate_bound_token_required` 无列默认，密钥与证书本体不入库。`PAR-INT-09` 当前登记保持待提供。集成客户端信封与操作者信封分型。

**Spec**

阻断：无。

非阻断：无。

无发现：完成判据各格有测试。`a259ffde` 眼见：`isolatedWriteAdmittedCommandLines` 已撤下 `/customs/external-results`、`/customs-regulatory-credential-registrations`、`/settlement-external-funds-fact-registrations`。这三口与更正口都接 `integrationClients`，写开关换不了。更正与首登都走 `AuthenticateExternalFunds`，请求的 `FactType` 是 `FactExternalFunds`。`TestExternalFundsClientWithoutTheGrantIsNotGranted` 用关务外部结果授予打这只认证，答未授予。装配测试里持有资金授予的四口都落到准入未登，而不是未授予。
