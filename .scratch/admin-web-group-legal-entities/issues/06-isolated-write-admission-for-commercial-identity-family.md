# 06 ADR-0091 逐口放行：`/commercial-*` 身份族在隔离形态下放行

Category: enhancement
Status: ready-for-agent
Blocked by: 无
地盘：`cmd/parcel-api/assemble_isolated_write.go`、`cmd/parcel-api/endpoints.go`（身份族五行的 Intake 参数）、
`internal/partycommercial/adapters/http/`（隔离 Intake 实现 + 测试）

## 为什么

本机演示形态开了 `IDP_PARCEL_ISOLATED_WRITE_TENANT=SYN-TENANT-01`，但 ADR-0091「命令面按端点逐口放行，不是一次全开。
首批只有 `/shipment-requests`」——`/commercial-legal-entity-registrations` 等五口仍挂字面量 `UnconfiguredIntake{}`，
管理台「登记法人」签（票 02 的表单）在任何环境下都答 403。演示动线要能从页面登一个法人再在列表里看见它，这一口得放。

## 要做的

按 ADR-0091 与 ADR-0055 预留的那条缝，**一次只换一口，每换一口该口的「未配置即拒」测试改写为放行测试，未换的口答复不变**：

1. `commercialhttp` 加 `IsolatedPartyIdentityIntake`：只在隔离写开关设了且带 `SYN-` 前缀时构造；租户格填开关值（合成），
   行内容只从载荷取（`legalEntities[0]` 等，形状照票 02 载荷）；载荷里若带 `tenantId` **拒**（`MALFORMED_REQUEST`）——
   在线口不采信自报租户，隔离形态也不例外。
2. 首放 `legal-entity` 一口；`business-party`、`customer-account`、`party-relationship`、`identity-deactivation` 各自成笔，
   每笔改写各自的未配置测试。
3. `buildIsolatedWriteAdmission` 返回结构加身份族 Intake 一格；`assembleBusinessEndpoints` 对 nil 的处理与之前逐字节同形。
4. 启动日志那条「Isolated write admission enabled」补一句放行了哪几口——放行必须出声（ADR-0078 决定三、ADR-0091 决定四）。

## 不做

- 不放发布口（`/commercial-publications`）与产品渠道口：它们有自己的治理（审批职责规则、ADR-0126 载体路径），另票。
- 不动读面开关。

## 完成判据

- `go build ./...`、`go vet ./...`；`internal/partycommercial/adapters/http` 与 `cmd/parcel-api`（带 DSN）`go test -count=1`。
- 开关未设：五口照旧 403。开关设 `SYN-TENANT-01`：`legal-entity` 一口对合规载荷答 201 `PARTY_IDENTITY_REGISTERED`，
  对带 `tenantId` 的载荷答 400；其余四口在各自那笔落地前仍 403。
- 演示形态下从票 02 的表单登一个 `SYN-LE-02`，列表重取后可见。

## Comments
