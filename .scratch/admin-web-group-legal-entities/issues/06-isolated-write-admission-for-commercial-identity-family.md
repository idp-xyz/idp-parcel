# 06 ADR-0091 逐口放行：`/commercial-*` 身份族在隔离形态下放行

Category: enhancement
Status: resolved
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

## 完成记录（2026-09-16，分支 `mcp4-pc-identity-isolated-write`，基 `a608536d`，19:0x rebase 至 `a7bdd666`）

作者：五笔为通道 4 前会话所作（17:57–18:08，分支原 SHA `7900ab12` / `ab6e7a48` / `d0814692` / `52e1cd93` / `ee4fb886`），会话断于完成记录之前；19:0x 由通道 4 新会话（无在途记忆）`git rebase origin/main` 重放为 `b6f99b6e`（legal-entity 首口）/ `0a95ba6f`（business-party）/ `89771bc1`（customer-account）/ `515b7c14`（party-relationship）/ `835690b8`（identity-deactivation，tip），零冲突，`--force-with-lease` 已推。本节由通道 4 新会话按 `git diff a7bdd666 835690b8`（8 件 +1095/−35）代写，未改一行代码。

对完成判据：

- Go：八件 `gofmt -l` 空；`go build ./...` 0 / `go vet ./...` 0；带 DSN `go test -p 1 -count=1 ./internal/partycommercial/adapters/http/ ./cmd/parcel-api/ ./internal/architecture/...` 三包 ok、0 FAIL（19:01:26→19:01:34；反向依赖用 `go list -f '{{.ImportPath}} {{.Deps}}'` 反查，`partycommercial/adapters/http` 的生产反向依赖只有 `cmd/parcel-api`）；`-v` 探针 `TestIsolatedLegalEntityRegistrationLandsAgainstARealDatabase` PASS 非 SKIP（0.39 s；`cmd/parcel-api` 全 `-v` 下 `--- SKIP` 为 0）。全仓全量按现行规矩由推送方在重放 tip 跑一次，本票不重跑。
- 开关未设，五口照旧 403：`TestEveryAssembledEndpointAnswersUnconfigured`（读 Intake 与两个写 Intake 都为 nil，五口连同全部端点 403 `ACCESS_CHANNEL_NOT_CONFIGURED`、无 outcome）；只设读开关也开不了写行：`TestIsolatedReadAdmissionCannotOpenTheAdmittedCommandLines` 对六口（`/shipment-requests` + 身份族五口）各钉 403。
- 开关设 `SYN-TENANT-01`：`legal-entity` 一口对合规载荷 201、outcome `REGISTERED`（`TestIsolatedLegalEntityRegistrationLandsAgainstARealDatabase`——走 `buildIsolatedWriteAdmission` 取 Intake、`buildCommercialRegistrationOrchestration` 取编排、`pgtest` 真库，参与方先经编排直登再登法人）；带 `tenantId` 的载荷 400 `MALFORMED_REQUEST` 且无 outcome（同用例）；传输面另有 `commercialhttp` 包 `TestIsolatedPartyIdentityIntakeRefusesSelfReportedTenant`（另一租户 / 与开关同值 / `null` 三格）。
- 「其余四口在各自那笔落地前仍 403」：二分表 `expectedWriteAdmittedLines` 每笔加一行、`TestIsolatedPartyIdentityIntakeServesOnlyAdmittedLines` 每笔翻一格（五笔的 `--stat` 各含 `isolated_write_test.go` 与 `isolated_write_intake_test.go`）；`TestIsolatedWriteAdmissionSwitchesOnlyTheAdmittedCommandLines` 名单内 400 / 名单外 403。tip 上五口齐，发布口与产品渠道口仍 403 且在类型上装不进隔离身份 Intake。
- 演示形态从票 02 表单登 `SYN-LE-02`、列表重取可见：**未验**——收尾会话未起 parcel-api 进程与前端栈；载荷形状由 `TestIsolatedPartyIdentityIntakeTranslatesLegalEntityRegistrationWithInjectedTenant` 按票 02 的 `legalEntities[0]` 五格钉住（RFC 3339 带偏移按绝对时刻比，请求头与查询串里的自报租户无视）。

四条逐条：① `commercialhttp.IsolatedPartyIdentityIntake`（`isolated_write_intake.go`），只在 `buildIsolatedWriteAdmission` 的 `SYN-` 前缀门与读写开关同值门之后构造；租户格取开关值，行内容只从载荷取；外壳 `partyIdentityBatchDocument` 与 CLI `register_parties.go` 的 `partyBatchDocument` 五种项文档逐字段同形，`tenantId` 一格改 `json.RawMessage`、只为拒（`refuseSelfReportedTenant`：键在场即拒）。② 一口一笔，SHA 见上；每笔 = 接口断言一行 + `Intake*` 方法一个 + 装配点换一行 + 二分表加一行 + `ServesOnlyAdmittedLines` 翻一格。③ `isolatedWriteAdmission` 加 `partyIdentity` 一格，`partyIdentityIntake()` 对 nil 接收者交 nil；`assembleBusinessEndpoints` 新入参为具体类型 `*IsolatedPartyIdentityIntake`（不是某个 Intake 接口——类型实现了哪几口才换得了哪几行），nil 时五个局部变量维持 `UnconfiguredIntake{}`，五行答复与之前同；源码上那五行由字面量改为变量，「逐字节同形」指答复，由 `TestEveryAssembledEndpointAnswersUnconfigured` 钉。④ `main.go` 启动日志加 `admittedCommandLines` 属性（`isolatedWriteAdmittedCommandLines` 六行），`TestIsolatedWriteAdmissionNamesTheAdmittedCommandLines` 钉日志名单与二分表条数、成员一致。

判断项（票面没写死、代码替它判了的）：

1. 线上 outcome 名是 `REGISTERED`（`PartyIdentityRegistered.String()`），不是票面写的 Go 常量名 `PARTY_IDENTITY_REGISTERED`；与票 01 / 02 评审所量一致，以代码为准，票面完成判据那句按此读。
2. 一次一项：每口恰收本口一项，零项 / 多项 400；同一外壳里夹带别的口的项也 400（`exactlyOne` + `itemCount`），不无声丢弃。批走受控 CLI。
3. `DisallowUnknownFields`：封闭形状，多余键 400。
4. `"tenantId": null` 也拒——键在场即自报，不看值；值与开关相同也拒。
5. 停用口独立外壳 `deactivationBatchDocument`（镜像 CLI `deactivate-party-identity`），不与登记外壳合一；`kind` 封闭三值 `BUSINESS_PARTY` / `LEGAL_ENTITY` / `CUSTOMER_ACCOUNT`，`PARTY_RELATIONSHIP` 拒（关系的终止不叫停用）。
6. 关系口 `role` 为 `domain.PartyRole` 封闭集的名称镜像（按 `String()`），集合外 400；`effectiveEndsAt` / `approval` 缺席即开区间 / 候选关系，用指针表达缺席、不造零值顶上。
7. 字段不裁首尾空白（与 CLI 一致；票 02 表单层已裁）。
8. 形状级失败包 `ErrMalformedRequest`（400）；修订连续性、参与方在册与届时已生效、`effectiveFrom` 缺席为零值等一律进用例答`未受理`，Intake 不预判——票 02「表单不裁任何门」在服务端 Intake 同样成立。
9. 放行名单 `isolatedWriteAdmittedCommandLines` 是独立 `var`，不从装配表派生；与装配点的同步靠两道测试，不靠结构。`/shipment-requests` 也在这份名单里，而提交口 Intake 由 `buildIsolatedSubmissionIntake` 在开池后另构造——两者失配只会在启动失败（进程退出）时出现，不会与运行中的进程并存。
10. `SYN-` 前缀门与读写开关同值门在 `buildIsolatedWriteAdmission`，`NewIsolatedPartyIdentityIntake` 只校验 `TenantID` 立得住——「只在开关设了且带 `SYN-` 时构造」由调用点位置保证，不由构造函数保证。
11. 真库用例直接对端点构造函数 `ServeHTTP`，不经路由；路由表那一格由 `TestIsolatedWriteAdmissionSwitchesOnlyTheAdmittedCommandLines` 另钉（用例注释已写明）。

清点预报（在 `835690b8` 干净树上跑 `tools/mechanism-inventory -out` 至临时路径，与已提交文件 `--numstat` 3/3）：`partycommercial` 生产 126→127 / 测试 140→141 / http 适配器 28→29，合计随之；`cmd/` 测试 89→90。按现行流程由推送方在重放 tip 重生成、单独成笔，本笔不含。

**进 main 记录**：待推送方重放后补；评审另派非作者通道。

## Comments
