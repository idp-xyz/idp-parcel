# PS 生产归属目录：隔离写开关退场的前件

Category: enhancement
Status: resolved——2026-10-26 进 main `7dc590793`（见文末「进 main 记录」；第 4 步写开关退场随 operator-channel/53）。此前 in-progress——2026-10-26 作者完工，待隔离评审与进 main，分支 claude-fpa04（代码最后一笔 8e34dff89）。此前 in-progress——2026-10-26 会话 `tops-20`（Claude Code）分诊为 R1（上线前必修：生产形态下真实租户一单也提交不进来，见 [分诊报告](../../needs-triage-sweep-2026-10-26.md)）并认领，分支 `claude-fpa04`；新 ADR 取 0308。此前 needs-triage——2026-10-19 票 [00](./00-first-party-customer-intake.md) 立。
Blocked by: 无
出处：[ADR-0180](../../../docs/adr/0180-admission-scope-tenant-mapping-is-an-access-identity-register-and-self-authority-is-a-deployment-parameter.md) 决定三「目录本身归 PS 生产归属那一票」与越权风险点「PS 生产归属的能力与事实类型词不在本记录内」；[ADR-0150](../../../docs/adr/0150-synthetic-tenant-is-treated-as-a-real-tenant-and-isolated-form-retires-per-face.md) 决定一、三；[ADR-0141](../../../docs/adr/0141-integrator-sandbox-is-a-deployment-form-of-the-first-party-api-on-synthetic-data.md) 决定四「不许写一个直接返回已确定的假目录」。

## 现状（票 00 落地后）

- 提交口挂首方客户渠道 Intake，隔离写开关不放行任何命令面。写开关只管 ADR-0091 决定二的「墙二」：给提交编排的生产归属适配器注入合成治理坐标（`IsolatedGovernanceScopeDirectory`）与合成自身权威串，且只答写开关那个租户的拟受理范围（ADR-0091 决定六；票 00 评审修补）。
- 生产形态下 `buildProductionOwnership` 的 `Directory` 留空，`governanceScope` 先于读册早退，任何租户经首方口提交都停在 `OWNERSHIP_UNRESOLVED`。演示要走到`已提交`仍得设写开关。
- 首方 Intake 给编排的拟受理范围是（租户、客户账户）快照，形状 `PS-CUSTOMER-ACCOUNT-ADMISSION-SCOPE/1` 的构造与认回在 PS 领域（`NewCustomerAccountAdmissionScope` / `AdmissionScope.CustomerAccount`，票 00 代决）。生产目录从快照认（租户、客户账户）就调后者，不另解析引用串；目录还没有一处把它换成治理坐标。

## 做什么

1. 立 PS 的生产归属目录（`pspilot.GovernanceScopeDirectory` 的生产实现），读册换坐标、不代拟：候选是读接入身份的准入范围对照册（ADR-0180 决定一、二）按（租户、能力、事实类型）取对象范围，试点范围版本由谁给要一并裁。
2. 定 PS 委托受理在治理登记册上的能力词与事实类型词（ADR-0180 越权风险点「PS 生产归属的能力与事实类型词不在本记录内」；决定四那一类，产品策略）。
3. 演示种子按定下的词另登区间与对照，`SYN-` 合成坐标随写开关退场。
4. 目录接上之后，写开关在 ADR-0150 决定三说的「最后一口」那一笔与读开关一起删（读开关归 operator-channel/53）；`isolatedWriteAdmission`、`IsolatedGovernanceScopeDirectory` 与两开关一致性校验同笔删。

## 完成判据

- 生产形态（不设写开关）下，演示租户经首方口提交一单答 `SUBMITTED`；对照或区间没登答 `OWNERSHIP_UNRESOLVED`（不代拟）。
- 证据只记 `S`。

## 完成记录

起点 `b6f0c10ba`；代码提交 `8e34dff89`（分支 `claude-fpa04`）。本轮做「做什么」第 1–3 步；第 4 步（写开关退场）不在本轮，归 operator-channel/53，写开关代码未删。没有迁移（对照册与区间册现成，PS 迁移号 0032 未取用）。

### 各层改了什么

- 领域/接入身份：`accessidentity.ShipmentRequestSubmissionAdmission()` 是委托提交那一对词的唯一定义；`checkAdmissionRequirement` 单独认下它（登得进对照），`customerCapabilitiesAdmittedAtMint` 不动，铸造侧仍不判它。
- PS 适配器：`internal/parcelshipment/adapters/accessidentity` 新增 `ProductionGovernanceScopeDirectory`（实现 `pspilot.GovernanceScopeDirectory`；放这里是 ADR-0025 消费方侧位置，不让 `pilotgovernance` 适配器多导入第三个上下文）；`pilotgovernance` 新增 `NewGovernanceScopeOfObjectScope`（试点范围版本取对象范围本身）。
- 装配：`buildProductionOwnership` 在 `isolated == nil` 时装生产目录；设写开关仍装注入式合成目录。
- 种子：`data/access/17`（对照）、`data/governance/52`（区间）、`steps/478-access-shipment-request-submission.sh`；`scripts/demo-seeds/README.md` 第 5 条改写。
- 文档：ADR-0308；ADR-0180 的 Status 索引注记与 Links 加前向指针（正文未动）；`docs/adr/README.md` 加 0308 行并给 0180 行加后续注记；UC-PS-001 验收表与 `.scratch/at-coverage-inventory.md` ①分桶表补 `AT-PS-117..119`（总览计数未重算，由推送方重算）。未改任何 `docs/domain` 下 CONTEXT。

### 能力与事实类型词、试点范围版本

- 能力 `CUSTOMER_COMMAND`，事实类型 `SHIPMENT_REQUEST_SUBMISSION`，产品词、不带 `SYN-`；与首方客户族已有取法同族（ADR-0180 决定四末项）。
- 试点范围版本由对照的对象范围本身给，不设第二来源。

### 代决与越权风险点（均见 ADR-0308，待 PS owner 复核）

- 代决 1 词（决定一）；代决 2 对照册收下词而铸造侧不判（决定二）；代决 3 目录读对照、不代拟，缺席与故障分格（决定三）；代决 4 试点范围版本 = 对象范围（决定四）；代决 5 写开关形态保留注入式目录（决定五）。
- 越权风险点：1 键粒度是租户不是客户账户；2「试点范围版本 = 对象范围」是假设；3 设写开关时开关租户之外的租户即使登了对照也答`权威未确定`（过渡期）；4 `CUSTOMER_COMMAND` 下混着铸造侧判与不判两类口；5 权威串未设时登得再齐也答`权威未确定`；6 没读 ADR-0139、ADR-0025 全文与暂停迁移。

### 判据对用例

- 不设写开关，演示租户经首方口 `SUBMITTED`：`TestWithoutTheWriteSwitchARegisteredTenantSubmitsThroughTheProductionDirectory`（真库，AT-PS-118）；写开关形态仍成立：`TestTheWriteSwitchLendsItsCoordinatesOnlyToItsOwnTenant`。
- 对照或区间没登答 `OWNERSHIP_UNRESOLVED`：`TestWithoutTheWriteSwitchAMissingMappingOrIntervalLeavesOwnershipUnresolved`（只登区间、只登对照、对照登给别的租户）与 `TestWithoutTheWriteSwitchSyntheticCoordinatesAloneDoNotAdmit`（AT-PS-119）。
- 目录读法（查到、查不到、读不动、重叠）：`production_governance_scope_directory_test.go` 三个用例（AT-PS-117）。
- 证据只记 `S`。

### 自验

含 PG（`IDP_PARCEL_POSTGRES_DSN` 指本机 55432）：`go build ./...`、`go vet`（parcelshipment、accessidentity、cmd）通过，改过的 `.go` 已 `gofmt`；`go test -count=1 -p 1 ./internal/parcelshipment/... ./internal/accessidentity/... ./cmd/parcel-api/... ./internal/architecture/... ./migrations/...` 全绿（架构门禁曾拦下事务回调里的 `t.Fatalf`，已改）。种子灌入一次性库 `fpa04_seed`（`seed.sh` 退出 0，新对照与区间各一行），已 `DROP`。`go run ./tools/at-coverage-counts -check` 只报生成物过期。未跑全量。

## 评审记录

评审 ← tops-20 隔离子代理(sonnet) · 钉 `021618e9d`

- **阻断**：无。
- **非阻断**：1. 作者手改的 `docs/adr/README.md` 中 ADR-0180 行与生成器输出不一致（推送方重生成时覆盖）；ADR-0180 Status 注记拼出「`SYN-`。；**后续**」多一个标点——推送方在落地树上改成全库通行的「｜」分隔并重生成索引（`7dc590793`）。2. 越权风险点 3（设写开关时开关外租户即便登了对照也答权威未确定）可接受：过渡期保守、随开关退场消失，ADR-0308 决定五与 `assemble_production_ownership_directory_test.go` 已钉。3. ADR-0308 决定三（试点范围版本取对照的对象范围本身）站得住：治理册权威区间的对象范围与暂停决定指同一引用，多设来源只多一处可错开。
- **无发现**：`ProductionGovernanceScopeDirectory.FindGovernanceScope` 缺席答 `(零值,false,nil)`、读不动与同键重叠答 error，编排不代拟；放 PS accessidentity 适配器合 ADR-0025，`internal/architecture` 通过；装配两形态互不借坐标；能力与事实类型词只在 `ShipmentRequestSubmissionAdmission()` 一处定义、铸造侧不判；种子对照 17 与区间 52 各一条、不多登；`AT-PS-117`–`119` 钉住判据、不撞号。

## 进 main 记录（2026-10-26，推送方会话 tops-20）

- **main**：`7dc590793`。出处：分支 `claude-fpa04` 代码 `8e34dff89`、票面 `021618e9d`（作者为 Sonnet 子代理）；main 未前进，原样快进。
- **生成物**：ADR 索引 `5991d1493`（并于 `7dc590793` 随 ADR-0180 分隔修正再生成）；AT 盘点总览计数 `665386516`；机制清点 `c73bf0cee`。三件 `-check` 退 0（含票 139 表行守卫）。
- **全量真库测试**：`scripts/test-full.sh` 跑在 `7dc590793`（即所推 SHA）：158 ok / 0 FAIL，退 0。
- **种子**：`scripts/demo-seeds/seed.sh` 灌一次性库（`7dc590793` 上），退 0，用后删库。
- **完成判据**：生产形态（不设写开关）下演示租户经首方口提交答 `SUBMITTED`（`AT-PS-118`）；对照或区间没登答 `OWNERSHIP_UNRESOLVED`（`AT-PS-119`）。证据记 `S`。
- **「做什么」第 4 步（写开关退场）未在本票做**：照 ADR-0150 决定三，它与读开关在「最后一口」同一笔删，读开关归 operator-channel/53；本票之后写开关只在显式设置的演示形态起作用，生产形态已不依赖它。
