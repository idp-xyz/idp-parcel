# 05 主数据与运营目录查阅面逐口换操作者 Intake

Category: enhancement
Status: resolved——2026-10-14 通道 1 重放进 main：`mcp4-oc05` 自 `da788a3b` 快进，SHA 不改（`4a7e0e63` 认领、`caec812a` 代码、`7b7ce47c` 清点、`605e0743` 完成记录）。评审无阻断，Spec 非阻断一条不挡。此前：完工，待评审与重放——2026-10-14 通道 4。分支 `mcp4-oc05`，基 `da788a3b`，代码 tip `caec812a`，清点 `7b7ce47c`。认领见完成记录。
Blocked by: 03
父票：[psb/15](../../product-strategy-boundary/issues/15-operator-channel-per-adr-0100.md) 甲轨
地盘：`cmd/parcel-api` 端点表里 ADR-0077 那一族目录查阅端点的装配行及其装配测试。
出处：[ADR-0100](../../../docs/adr/0100-operator-identity-is-a-product-owned-access-channel-family.md) 决定四；[ADR-0078](../../../docs/adr/0078-isolated-environment-operations-reads-admit-by-assembly-injection.md) 隔离读放行。

## 做什么

1. 目录查阅面逐口换操作者 Intake（能力面：主数据与运营查阅读），三格测试同 04。
2. 与隔离读放行并存：隔离读入参非 nil 时照旧换隔离行，两者是装配点上的两行，互不替代（ADR-0100 决定五第二条）。

## 完成判据

- 已换各口对合成操作者答查阅结果、三格各答其格；已换各口的隔离读放行在同一笔撤下，其用例改写为真渠道答复格用例。（2026-09-24 随 [ADR-0150](../../../docs/adr/0150-synthetic-tenant-is-treated-as-a-real-tenant-and-isolated-form-retires-per-face.md) 决定三改：原句「隔离读放行的既有用例不改一字仍绿」停用。）

## 完成记录

认领：2026-10-14 13:36 通道 4（派单 task-fc4339a6 ← 通道 1），`4a7e0e63`。开工时远端 main 仍是 `da788a3b`。此前 ready-for-agent——2026-09-24 拆法经用户授权通道 4 自决认可。

各笔（已推 `origin/mcp4-oc05`，未进 main）：

- `4a7e0e63` 认领
- `caec812a` 计价、网络、关务、商业四族目录查阅换 `OperatorOperationsReadIntake`；`cmd/parcel-api` 不再让隔离读盖这四只变量；隔离读用例里这几口改回未配置，新用例 `TestSwappedCatalogueReadFacesAnswerFromTheOperatorChannel` 钉真渠道三格
- `7b7ce47c` 机制清点，干净检出钉 `caec812a`

证据层级只记 `S`。用例里的操作者、租户都是 `SYN-`。

### 判据

- 三格各答其格 ✅ `TestSwappedCatalogueReadFacesAnswerFromTheOperatorChannel`：发行方未配置答 `403 ACCESS_CHANNEL_NOT_CONFIGURED`；不带令牌答 `401 OPERATOR_CREDENTIAL_REJECTED`；只有登记写授予答 `403 OPERATOR_NOT_GRANTED`。四族点名的查阅口都走这三格，探针沿 `businessEndpointProbes` 的传输形状（网络目录带 `family`，口岸册带 `registry`，解析口带 `at`）。
- 合成操作者答查阅结果 ◑ 同一用例里授予齐备的合成操作者在各口到达未接线读口，答 `500 NO_ANSWER_FORMED`，不再是未配置。租户取自信封、页大小 200、不读请求体，由 `TestOperatorCatalogueQueryTakesTheTenantFromTheEnvelope` 钉。空目录的 2xx 成格是各端点换口前的既有用例，本票没有用操作者 Intake 再跑一遍真库空册。
- 隔离读放行同笔撤下 ✅ 装配不再把 `isolatedRead` 的计价、网络、关务、商业四格盖上查阅变量。`TestIsolatedReadAdmissionSwitchesOnlyOperationsReadLines` 里这几口改回 `403 ACCESS_CHANNEL_NOT_CONFIGURED`；`TestSwappedCatalogueReadFacesAnswerFromTheOperatorChannel` 在隔离读打开时仍答操作者渠道的格。自报身份那条探针改打仍在放行面上的 `/tracking-projections`。

不做 operator-channel/10 的更正口与撤写放行。节点、运输、追踪、代收、结算、治理的查阅仍走隔离读。

### 验证

工作树 `idp-parcel-mcp4-oc05`。`go build ./...` 退 0；`go vet ./...` 退 0。

带 `IDP_PARCEL_POSTGRES_DSN=postgres://parcel:parcel@127.0.0.1:55432/postgres?sslmode=disable`：`go test -count=1 -p 1 ./cmd/parcel-api/ ./internal/parcelpricing/adapters/http/ ./internal/parcelpricing/adapters/accessidentity/ ./internal/networkrouting/adapters/http/ ./internal/networkrouting/adapters/accessidentity/ ./internal/customscompliance/adapters/http/ ./internal/customscompliance/adapters/accessidentity/ ./internal/partycommercial/adapters/http/ ./internal/partycommercial/adapters/accessidentity/ ./internal/architecture/...` 退 0。未跑全仓。

## 判断项

1. **做什么第 2 条仍写「与隔离读放行并存」，完成判据和派单按 ADR-0150 决定三改为同笔撤下。** 本票照完成判据。ADR-0100 决定五第二条对这四族已换的口不再适用。
2. **作用域引用取能力面名字 `MASTER_DATA_AND_OPERATIONS_READ`，页大小取 200。** 两者都是产品常量：引用是审计回溯、不参与行过滤（ADR-0077 决定二）；页大小由接入面定、不采信请求。不是租户取值。四个上下文各写各的常量，不共享类型。
3. **换的是共享 Intake 变量上的全部查阅口，** 含 ADR-0077 决定六当时排除、后来挂上同一变量的法人、参与方、合同、协议、注册号类型与资料修订。只换其中几口会装出「同一个 Intake 两种答案」。
4. **查阅读不带准入要求。** 与试算同一理由：目录查阅不形成生产事实。准入范围桥不在这条入口上判。

## 越权风险点

1. **页大小与作用域引用写成产品常量。** owner 若要把页大小或审计引用收成租户取值，要改 `operatorCatalogueReadLimit` 与 `operatorCatalogueScopeReference`，四包同形。
2. **「答查阅结果」没有真库空册。** 装配用例只证授予齐备的请求到达读口。读口自己的空目录成格不在本票重跑。

## Comments

**评审 ← 通道 1 · 钉 `605e0743` · 13:42**（基 `da788a3b`，隔离检出 `/tmp/idp-review-oc05-mcp1`。作者是通道 4。用户此前令通道 1 直接评审，本票照办，不另派。）

**Standards**

阻断：无。

非阻断：无。

无发现：四族都铸 `CapabilityMasterDataAndOperationsRead`，不带准入要求。页大小 200 与作用域引用是产品常量，判断项 2 写明不共享类型。租户取自信封，`TestOperatorCatalogueQueryTakesTheTenantFromTheEnvelope` 钉请求体不读。证据只记 `S`。

**Spec**

阻断：无。

非阻断：完成判据「答查阅结果」票面自标 ◑。`TestSwappedCatalogueReadFacesAnswerFromTheOperatorChannel` 里授予齐备到达未接线读口，答 `NO_ANSWER_FORMED`，不是查阅结果。空目录 2xx 没有用新 Intake 再跑。读口仍是各端点原有的第二参，不挡合入。越权风险点 2 已记下。

无发现：三格与撤隔离读放行有名。`TestSwappedCatalogueReadFacesAnswerFromTheOperatorChannel` 钉未配置、无令牌、无查阅读授予，以及隔离读打开时这几口仍走操作者渠道。`endpoints.go` 不再把隔离读的计价、网络、关务、商业四格盖上查阅变量。未做 operator-channel/10。

### 进 main 记录（推送方 · 通道 1）

- **门**：上面这份评审无阻断。非阻断不挡。
- **重放**：分支自 `da788a3b` 起，远端 main 仍是它时快进，SHA 不改：`4a7e0e63`、`caec812a`、`7b7ce47c`、`605e0743`。
- **验证**：钉 `605e0743` 的 Go。`gofmt -l` 对改动的 `.go` 无输出。带 DSN 单跑 `TestSwappedCatalogueReadFacesAnswerFromTheOperatorChannel` 为 PASS、非 SKIP。带 DSN `go test -p 1 -count=1 ./...` 退 0：140 个包 `ok`、0 FAIL、14 个包无测试文件，没有 `(cached)`。本笔只改票面。推送条件：远端 main 仍是 `da788a3b`。

