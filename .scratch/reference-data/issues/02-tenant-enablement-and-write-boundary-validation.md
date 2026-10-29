# 02 租户启用哪些国家 / 地区与币种没有登记处，新写入入口也不认内置目录

Category: enhancement
Status: in-progress——2026-10-29 作者完工，待隔离评审与进 main，分支 claude-refdata2a（代码最后一笔 `a3215649b`）。此前：in-progress——2026-10-29 作者认领，分支 claude-refdata2a（起点 `b8fe76a42`）。
Blocked by: 无（承接 [01](./01-countries-currencies-and-units-are-product-built-in-reference-data.md) 的后续轮次）

## 现象

取证于 main `b8fe76a42`：基础数据目录（ADR-0335）已内置，但某租户运营哪些国家 / 地区、哪些币种没有地方登记，基础数据页只能查目录；各写入口对国家码、币种码只验形状（两位 / 三位大写，或只验非空），目录外的码照常被接受。

## 做什么

设计与取舍在 [ADR-0345](../../../docs/adr/0345-tenant-enablement-of-countries-and-currencies-is-a-tenant-value-registered-in-party-commercial-and-new-write-entries-accept-only-catalogue-codes.md)（会话代产品 owner 接受、待复核）。

1. `party-commercial` 的启用范围登记册：只追加、修订连续、生效时点不早于前一修订，当前状态由“生效时点不晚于此刻的最新一行”导出；码取自内置目录；登记责任方取自认证信封。迁移 `0043_reference_enablement.sql`。
2. 两个端点：`POST /reference-data/enablement-registrations`（登记册配置写，不新增授予种类）、`GET /reference-data/enablement?kind=`（主数据与运营查阅读）。
3. 新写入入口只收内置目录里的码：法人注册国家、服务区域覆盖国家、结算政策 / 结算账户 / 供应商应付账户 / 越权升级上限 / 价卡 / 参照序列 / 分户账的币种；构造函数与读回不动。
4. 管理台“基础数据”页国家和地区、币种两签加“启用状态”列与“启用 / 停用”动作。

## 完成判据

- 登记册：幂等重放、同键异内容冲突、最新生效导出、停用后再启用、目录外码被拒、租户隔离（领域、编排、真库）。
- 读口与写口 HTTP：未授予 403、他租户不可见、操作者落库取自信封。
- 每个被收紧的入口各一条“目录外的码答业务格、目录内的码照常”的用例；不为过测试放宽任何一道门。
- `internal/architecture` 门禁过；前端纯逻辑单测。
- 不做写入时按启用范围强制拦截。

## 完成记录（2026-10-29 作者）

起点 `b8fe76a42`；分支 `claude-refdata2a`。提交（后到先排）：`a3215649b`（未受理原因写成中文，真令牌自验时发现）、`5c30be45e`（管理台）、`866780454`（新写入入口收紧与夹具改真码）、`cf2d520d7`（两个端点与生产装配）、`1793d8138`（登记册：领域、编排、真库、迁移 0043）；文档（ADR-0345、CONTEXT、UC-PC-001、AT 盘点、本票面）在其后单独一笔。

**改了什么**
- `internal/partycommercial`：`domain/reference_enablement.go`（种类、状态、码、来源单据、登记修订、`FollowsFrom`、`EnablementStandingAt`）、`ports/reference_enablement.go`、`application/register_reference_enablement.go`（登记与读侧编排）、`adapters/postgres/reference_enablement_registry.go`、`adapters/http/reference_enablement.go`（Intake 方法、登记端点、读回端点）；`migrations/party_commercial/0043_reference_enablement.sql`。读回的重建不核目录成员（`RehydrateReferenceEnablementRegistration`）。
- `cmd/parcel-api`：登记行入 `endpoints_commercial_registration.go`，读回行入 `endpoints_commercial_catalogue.go`，事务壳与装配在 `assemble_commercial_registration.go`。**没有新增授予种类，没有 `access_identity` 迁移**。
- 收紧：`refdata.CheckCountry` / `CheckCurrency`；PC 应用层 `checkLifetimeNumbers`（法人注册国家）、`NewCatalogueCurrencyCode`（结算政策币种：发布载荷、受控批文、商业选择键）；NR 应用层 `normalizeServiceAreaCoverage` 与新增拒绝理由 `COVERAGE_COUNTRY_NOT_IN_CATALOGUE`（枚举末尾追加，不挪既有数值）；SA registrationjson 的结算账户、供应商应付账户、越权升级上限；PP 的价表导入币种格与参照序列载荷；CR 的新开分户账。
- `apps/admin-web`：`EnablementControls.tsx`、`enablement.ts`、`use-catalogue.ts`、页面两签、词典（ENABLED、DISABLED）、导航摘要；网络登记拒绝理由补一句说法。
- 文档：ADR-0345、`party-commercial/CONTEXT.md`（启用范围登记册词条与两条规则、所有权）、UC-PC-001（`AT-PC-073`–`077`）、`.scratch/at-coverage-inventory.md` ① 桶补五行与一条追记。

**代决与越权风险点**：ADR-0345（会话代产品 owner 接受、待复核）越权风险点 0–8。本票层面的代决：
1. **ADR 号取 0345、迁移号取 0043、AT 号取 `AT-PC-073`–`077`**：取号前对 origin/main 与全部 `origin/claude-*` 分支核过，这三个号都没有被引用。
2. **“400 级业务答复”按 ADR-0022 落成 HTTP 200 + 响应体里的封闭理由**：目录外的码在登记口答 200 `NOT_ACCEPTED` / `CODE_NOT_IN_CATALOGUE`，不是 4xx；4xx 只留给“请求本身畸形”（自报租户、集外种类 / 状态、缺修订 / 生效时点、读回缺 `kind`）。各收紧入口答各自既有的业务格。
3. **写面用登记册配置写，不是运营决定**：启用范围是租户配置，不是对某个对象作的运营决定（ADR-0151 那一族）；因此不用 launch-wiring/01 的“access_identity 迁移取全集加一词”做法。
4. **来源单据可不填**（派单“按同类册对依据的写法”与“表单可不填”相抵时取后者）：同类册的依据格必填，这里放宽，写进风险点 6。
5. **生效时点不得早于前一修订**是本票加的不变量（风险点 5），用来避免“最新一行”读成没人想表达的事；管理台取“此刻”与“册上最晚生效时点”中较晚的一个，本机钟慢不会被拒。
6. **收紧范围的取舍**：事实类入口（记账、金额事实、资金事实、供应商账单、评价请求）与注册号类型目录登记的国家键不收紧，写进风险点 7。商业选择键（受控 CLI `register-resolution-key`）的结算币种算登记入口，收紧了。

**盘点清单与改动的种子 / 测试**
- **演示种子**（`scripts/demo-seeds/data`、`steps`、`demo-run`）：登进库的国家码只有 `CN`、`SG`，币种码只有 `CNY`、`SGD`，**全在目录内，种子一行没改**；一次性库灌种子退 0（见自验）。
- **测试夹具里目录外的码**（取证于 `b8fe76a42`，`grep` 全仓 `*.go`：`"XA"` 185 处、`"XB"` 43 处、`"XC"` 2 处、`"XK"` 1 处、`"XTS"` 48 处、`"SYN"` 57 处、`"XXX"` 8 处、`"ZZZ"` 1 处；JSON、脚本里没有）。这些值里走到被收紧入口的才改，其余不动：
  - `XA` → `CN`、`XB` → `SG`（法人注册国家、服务区域覆盖国家相关用例；共 20 个测试文件，另 `cmd/parcel-commercial/register_registration_number_types_test.go` 的 `XA` → `CN`，因为 `register_parties_test.go` 引用它的批文夹具）。涉及 `internal/partycommercial/{application,adapters/postgres,adapters/http}`、`internal/networkrouting/{application,adapters/http,adapters/postgres}`、`cmd/parcel-api`（三个文件）、`cmd/parcel-commercial`（三个文件）、`cmd/parcel-network-register`。`XC` 留作“寄件国不在始发区域”的地址侧输入（解析与读回不查目录）。
  - `XTS` → `CNY`（结算账户、供应商应付账户、越权升级上限的登记用例）：`internal/settlementaccounting/adapters/registrationjson` 的测试与 `cmd/parcel-settlement-register` 的全部测试文件；二者的币种在同一包内一致，`CNY` 与 `XTS` 一样落 2 位小数，行为不变。
  - `SYN` → `CNY`：`cmd/parcel-commercial/translate_test.go` 的结算选择键夹具，并反过来加了一条“`SYN` 被拒”。
  - **未动**：领域包单元测试（构造函数仍只验形状）、注册号类型目录登记用例的国家键（`XA`）、`internal/networkrouting/adapters/postgres/network_catalog_coverage_test.go` 里直接写 SQL 的 CHECK 用例、资金事实 / 评价请求 / 关务等事实类入口的 `XTS`、`XXX`（作“目录外的码”的反例）。
  - 没有为了让测试通过放宽任何一道门。

**判据对用例**
- 登记册：领域 `TestEnablementCodeMustBeInTheBuiltInCatalogueOfItsKind`、`TestEnablementRevisionsFollowOneAnotherInOrder`、`TestEnablementStandingIsTheLatestRowEffectiveByThatMoment`、`TestRehydrationDoesNotRecheckCatalogueMembership`、`TestEnablementSameContentIgnoresTheAuditFacts`；编排 `TestReferenceEnablementRegistersReplaysAndConflicts`、`TestReferenceEnablementDisableThenEnableAgainAppends`、`TestReferenceEnablementRefusalsAreNamedAndWriteNothing`、`TestReferenceEnablementWithoutAnEffectiveRowIsNotConfigured`、`TestReferenceEnablementIsolatesTenants`；真库（`adapters/postgres`）同名一组加 `TestReferenceEnablementWritesRefuseToRunOutsideATransaction`、`TestReferenceEnablementTableGuardsShape`。
- HTTP：`adapters/http/reference_enablement_test.go` 各条（信封取租户与登记责任方、自报拒、业务格、读回导出、读失败不折空册、未配置即拒）；生产装配真库用例 `TestTheOperatorChannelRegistersAndReadsBackReferenceEnablementAgainstARealDatabase`（生产铸造器 + 真库：写授予登记 201、操作者落成“发行方#sub”、只有读授予者登记 403、只有写授予者读回 403、自报租户 400、不带令牌 401、目录外码 200 未受理）。
- 收紧（各一条“目录外答业务格、目录内照常”）：法人 `TestLegalEntityRegistrationCountryMustBeInTheBuiltInCountryCatalogue` 与 `TestAnExistingLegalEntityOutsideTheCountryCatalogueStillReplays`；覆盖 `TestOnlyTheRegistrationEntryChecksTheCoverageCountryAgainstTheCatalogue`（及 `TestServiceAreaCoverageRegistrationRefusesMalformedContentBySlot` 里新增的一格）与读回不动的 `TestAnExistingCoverageOutsideTheCountryCatalogueStillReadsBackAndResolves`；币种：`TestSettlementPolicyCurrencyMustBeInTheBuiltInCatalogue`、`TestSettlementPolicyBodyCurrencyMustBeInTheBuiltInCatalogue`、`TestSettlementSelectorTranslatesWithoutAContractDimension`、`TestSettlementRegisterEntriesTakeOnlyCataloguedCurrencies`、`TestRateTableCurrencyMustBeInTheBuiltInCatalogue`、`TestSeriesPayloadCurrencyMustBeInTheBuiltInCatalogue`、`TestOpeningASubledgerTakesOnlyACataloguedCurrency`；`refdata` 的 `TestCheckAnswersCatalogueMembershipWithoutLeniency`。
- 前端：`enablement.test.ts`（未配置、状态词、页头那句话、动作选择、下一笔载荷与生效时间、答复人话、状态格）；`tsc -b`、全部单测、`vite build` 过。

**自验**（`IDP_PARCEL_POSTGRES_DSN` 指 55432 门禁库，**含 PG**）：`go build ./...`；`go vet ./...`；`gofmt -l` 无输出；`go test -count=1 -p 1` 于 `./internal/partycommercial/...`、`./internal/networkrouting/...`、`./internal/settlementaccounting/...`、`./internal/parcelpricing/...`、`./internal/collectionremittance/...`、`./internal/platform/...`、`./internal/accessidentity/...`、`./internal/architecture/...`、`./migrations/...`、`./cmd/...`（见下文终测结果）；`go run ./tools/at-coverage-counts -check` 与 `go run ./tools/adr-index -check` 只报生成物过期（协调方重生成）。**一次性库灌种子**：建库、`IDP_PARCEL_POSTGRES_DSN=…/<库> scripts/demo-seeds/seed.sh` 退 0（0043 随迁移施加，`reference_enablement_registration` 表在，演示租户未登记任何启用范围，如实是“未配置”）、删库。**真令牌对真进程**：另建一次性库灌种子，起本分支构建的 `parcel-api` 于 `127.0.0.1:19098`（不碰 `:19080`、`:5173`），用演示发行方 Dex 的配置员令牌打：空册读回、登记 CN 启用 201、重放 200 `ALREADY_REGISTERED`、`ZZ` 200 `CODE_NOT_IN_CATALOGUE`、停用 201、读回 `standing: DISABLED` 带两笔历史且 `registeredBy` 是 Dex 的“发行方#sub”、无令牌 401；这次自验发现未受理的 `cause` 是英文哨兵原句，已改成中文（`a3215649b`）。验后停了进程、删了库。**没有开浏览器看管理台页面**：Dex 的管理台客户端回跳地址写死在 5173，不碰 5173 就走不通登录门；页面行为由 `enablement.test.ts`、`tsc -b` 与 `vite build` 证，动作列、确认窗的真实渲染没有人眼核过。

**留给协调方**
- 进 main 时重生成 `docs/adr/README.md`（`go run ./tools/adr-index`）、AT 盘点计数（`go run ./tools/at-coverage-counts`，PC 加 5）、机制清点（新增两条端点与一个 `party-commercial` 登记册）；这三项我没动。
- 演示后端要重新构建部署，管理台“基础数据”页的新列才有数据（此前读口 404，列显示“读不到”，不影响既有页面）。
- 演示种子没有为 `SYN-TENANT-01` 登记启用范围（CN、SG、CNY、SGD）：本票不灌；若要演示页上看到“已启用”，需要一步种子（`parcel-commercial` 还没有登记启用范围的子命令，需要新增或走在线口）。
- 越权风险点 0（强制拦截）是下一轮的核心裁决：“未配置”等不等于“不允许”，会改变业务结果。
- 新发现不挡上线、不立票：目录外的存量数据没有清理手段（风险点 3）；管理台目前不显示“已不在目录里的已登记码”（风险点 2）；启用范围读回暂无历史面。
