# 01 国家 / 地区、币种、计量单位没有基础数据目录

Category: enhancement
Status: in-progress——2026-10-29 作者完工，待隔离评审与进 main，分支 claude-refdata（代码最后一笔 `469a2ef0e`）。此前：in-progress——2026-10-28 作者认领，分支 claude-refdata（起点 `f9dc00a6a`）。
Blocked by: 无

## 现象

取证于 main `f9dc00a6a`：币种只有 `parcelpricing/domain` 的 `Currency`（三位大写形状加 `MinorUnitScale` 内置小表）；国家只有 `networkrouting/domain` 的 `isCountryCodeShape` 与 `partycommercial/domain` 的两位大写形状校验，注释写“不内置码表”；计量单位是 `parcelpricing/domain` 的 `WeightUnit`、`LengthUnit` 与各处字面量，重量单位不一致报 `ErrWeightUnitMismatch`、没有换算。三者都没有名称、没有表、没有页面，操作者无处查“LB 是什么”“THB 小数几位”，管理台把国家码、币种码原样摆出来。

## 做什么

设计与取舍在 [ADR-0335](../../../docs/adr/0335-countries-currencies-and-units-are-product-built-in-read-only-reference-data-released-with-the-code-and-unit-conversion-keeps-precision-and-is-not-yet-wired-into-pricing.md)（会话代产品 owner 接受、待复核）。

1. `internal/platform/refdata`：国家 / 地区（ISO 3166-1 alpha-2 全量）、币种（ISO 4217 常用集带小数位）、计量单位（四类别、基准单位加精确系数）三份只读目录；`Country`、`Currency`、`Unit`、`Convert`。`Currency.MinorUnitScale` 改查目录，行为不变。
2. 三个运营查阅读口 `GET /reference-data/countries`、`/currencies`、`/units`（`?q=`、`?region=`、`?category=`）。
3. 管理台“设置”组新页“基础数据”（id `reference-data`），三签只读；译名显示件 `countryLabel` / `currencyLabel` 接进三个现在直接摆码的列表页。

## 完成判据

- 目录完整性单测：码唯一、全大写、币种小数位与旧表一致、国家数等于 ISO 已分配数、币种常用集覆盖、每个单位的基准单位存在且同类别。
- `Convert` 单测：LB→KG、KG→G、跨类别拒绝、往返精度。
- 读口 HTTP 用例：检索、分类过滤、租户必取自信封、未授予 403、空检索答全表。
- `internal/architecture` 门禁过；前端 `countryLabel` / `currencyLabel` 与页面纯逻辑单测。
- 不收紧任何现有校验，不把 `Convert` 接进计价。

## 完成记录（2026-10-29 作者）

起点 `f9dc00a6a`；分支 `claude-refdata`。提交：`fee2eef2b`（目录与 `MinorUnitScale` 改查、ADR-0335、票面）、`234b52195`（三个读口与组合根装配）、`7dd95b173`（管理台新页与译名显示件）、`469a2ef0e`（结算政策币种译名的用例）。

**改了什么**
- `internal/platform/refdata`：国家 / 地区 249 项（ISO 3166-1 alpha-2 官方已分配全量，带中英文名与大洲七词）、币种 152 项（ISO 4217 常用集，带名称、符号、小数位 0..3）、计量单位 23 项（重量 `KG`、长度 `CM`、体积 `M3`、件数 `PCS` 四类，系数为精确十进制文本）；`Country`、`Currency`、`Unit`、列表与检索、`Convert`（精确有理数，跨类别报 `ErrUnitCategoryMismatch`，未知单位报 `ErrUnknownUnit`）。
- `parcelpricing/domain`：`Currency.MinorUnitScale` 改查目录，目录外的三位码仍按 2 位答；原两张小表逐码钉进用例。`networkrouting` 与 `partycommercial` 两处“码表不内置”的注释改成指向 ADR-0335 决定五（只改注释，校验未动）。
- `internal/platform/refdata/refdatahttp` 三个读口，`cmd/parcel-api` 登记 `/reference-data/countries`、`/currencies`、`/units` 三行（`operationsReadFace`、`ownIntake`），认证件自建并把接入身份的答复格译成读口认的四格；契约形状写在 `refdatahttp` 的包注释里。
- `apps/admin-web`：设置组新页“基础数据”（id `reference-data`，三签只读，Workbench 形态，大洲 / 小数位 / 类别胶囊带计数，换算写成“1 磅 = 0.45359237 千克”）；`refdata-names.ts`（纯函数 `countryLabel`、`currencyLabel`）、`use-refdata-names.ts`（整表读一次、全站共用、读不全不缓存）、`refdata-labels.tsx`；词典 `scopedStatusVocabulary` 加 `refdata-region` 与 `refdata-unit-category` 两册。接进三页：公司主体（注册国家）、代收货款（币种列）、商业规则的结算政策册（币种列）——只改显示。

**代决与越权风险点**：ADR-0335（会话代产品 owner 接受、待复核）越权风险点 0–7。另有五项本票层面的代决：
1. **ADR 号取 0335**：origin/main 与各分支的号为 0001–0334、0336–0344，0335 在任何 ref、任何 worktree、`.scratch` 与源码里都没有被引用，按“未被占的最小号”取它；第一笔提交就推了分支以便别的会话撞号前看得见。
2. **`Convert` 不用仓里的 `Decimal`**：派单写“用仓里已有的十进制类型”，但它在 `parcelpricing/domain`，`internal/platform` 不得导入业务上下文（`TestSharedPlatformDoesNotDependOnBusinessContexts`），改用标准库 `math/big.Rat`（精确、无需新依赖），已写进 ADR 决定四与替代方案。
3. **读口的处理器放 `platform/refdata/refdatahttp`、认证译层放组合根**：platform 不得导入 `accessidentity`，所以处理器只认自己的四个哨兵，由 `cmd/parcel-api/assemble_reference_data.go` 译；`?region=` 是派单之外多给国家口的一个过滤，成本几行，写进 ADR 决定六。
4. **没有立 AT 号**：基础数据不属任何 UC，`docs/application` 没有验收表可挂；用例的 `Covers` 指 ADR-0335 的决定号，`.scratch/at-coverage-inventory.md` 未动。
5. **三个改显示的页面与派单提示不同**：服务区域页没有国家列（地理覆盖一栏是“尚不存在”的占位），费用与计费的币种在金额串里（`1,000.00 CNY`，金额格式不拆）；实际接的是公司主体的注册国家、代收货款的币种列、商业规则结算政策册的币种列——这三处是现状里直接摆码的列。

**判据对用例**
- 目录完整性：`TestCountryCatalogueIsCompleteAndWellFormed`、`TestCurrencyCatalogueCoversTheCommonSetAndIsWellFormed`、`TestUnitCatalogueIsConsistent`、`TestCurrencyMinorUnitScaleMatchesTheTableItReplaced`（`internal/platform/refdata`）与 `TestCurrencyMinorUnitScale`（`parcelpricing/domain`，含目录外码按 2 位）。
- `Convert`：`TestConvertIsExact`（LB→KG、KG→G、LB→OZ 等）、`TestConvertKeepsPrecisionOnTheRoundTrip`（往返与 1 千克 = 100000000/45359237 磅）、`TestConvertRefusesCrossCategoryAndUnknownUnits`。
- 读口：`TestTenantComesFromTheEnvelopeAndNeverFromTheRequest`、`TestAuthenticationAnswersBeforeParametersAreRead`、`TestListingSearchAndFilters`、`TestUnconfiguredHandlerAndConstructorGuard`（`refdatahttp`）；`TestReferenceDataEndpointsAnswerFromTheOperatorChannel`（真铸造器：发行方未配置、无令牌、只有写授予、有查阅读授予）、`TestReferenceDataEndpointsSearchFilterAndRefuse`、`TestReferenceDataPathsMatchTheEndpointTable`（`cmd/parcel-api`）；装配测试 `TestEveryAssembledEndpointAnswersUnconfigured`、`TestEachLineOpensOnlyOnItsDeclaredAccessFace`、`TestProductionAssemblyMountsEveryDeclaredLine` 覆盖这三行。
- 前端：`refdata-names.test.ts`（`countryLabel` / `currencyLabel`：译名、查不到原样、读不回按码、只读回一本）、`reference-data/rows.test.ts`（检索、大洲 / 小数位 / 类别筛选与计数、换算人话、页面态）、`policy-rows.test.ts` 新增一例（结算政策币种译名）；`navigation.test.ts` 的既有断言（页名、无英文、导航条目与出处）覆盖新条目。
- 不收紧校验、不接计价：`NewCurrency` 与两处国家码校验未改；`Convert` 无生产调用方（`grep` 可证）。

**自验**（`IDP_PARCEL_POSTGRES_DSN` 指 55432 门禁库，含 PG）：`go build ./...`；`go vet` 于 `internal/platform/...`、`cmd/parcel-api`、三个改过的 domain 包；`gofmt -l` 无输出；`go test -count=1 -p 1` 于 `./internal/architecture/...`、`./internal/platform/...`、`./internal/parcelpricing/...`、`./internal/networkrouting/...`、`./internal/partycommercial/...`（全 ok）与 `./cmd/parcel-api/`（ok，42s）；`go run ./tools/at-coverage-counts -check` 退 0，`go run ./tools/adr-index -check` 只报 README 生成物过期（协调方重生成）。`apps/admin-web`：`tsc -b --noEmit` 无输出，`node scripts/run-tests.mjs` 886 个全过，`pnpm exec vite build` 过。
**浏览器**：演示后端 `:19080` 还没有新读口（它是 main 之前构建的），页面直连会读不到——所以另起一次性小进程（不入库、用后删）在 `:19097` 挂三个真处理器，认证借 `:19080` 的 `/operator-binding` 验真 Dex 令牌、其余路径转发 `:19080`，管理台 `pnpm dev --port 5177` 指过去，用参考脚本改写 Dex 回跳登录后看真页面：三签都有数据（国家 249、币种 152、单位 23；大洲计数相加等于 249），单击单位行开详情栏；公司主体页注册身份显“中国（CN）”，代收货款页币种列显“人民币（CNY）”。验后停了自起的 5177 与 19097 两个进程，没碰 5173 与 19080，没写任何数据。商业规则的结算政策册没有演示数据可看，由 `policy-rows.test.ts` 的新例证明。

**留给协调方**
- 进 main 时重生成 `docs/adr/README.md`（`go run ./tools/adr-index`）与机制清点（新增了三条端点与一个 platform 包）；AT 盘点无变化。
- ADR 号 0335 是个空缺号：若别的会话也按“最小空号”取了 0335，以先进 main 者为准、另一方改号（本分支第一笔已推）。
- 演示后端要重新构建部署之后，管理台“基础数据”页与三个译名才有数据；在那之前它们如实显示读不到 / 按码显示，不影响既有页面。
- 后续轮次（ADR-0335 越权风险点 1、2）：租户启用范围、单位自动换算、码校验收紧到目录成员；本轮没有留半成品入口。
