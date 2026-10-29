# ADR-0335：国家 / 地区、币种、计量单位是产品内置的只读基础数据目录，随代码发布；单位换算保留精度，暂不接入计价

Status: Accepted（2026-10-28，Claude Code 会话代产品 owner 决（代决）：用户要补「国家 / 地区、币种、计量单位」的基础数据维护，设计由协调方代决、作者落笔；用户未逐条过目，越权风险点待产品 owner 复核。）<!-- 索引注记 -->｜**三份目录**：国家 / 地区（ISO 3166-1 alpha-2 全量）、币种（ISO 4217 常用集带小数位）、计量单位（四类别、基准单位加精确系数）都是产品内置的只读 Go 数据，放共享的 `internal/platform/refdata`，不入库、不建上下文；**不动校验与计价语义**：现有两位 / 三位形状校验原样，`Convert` 不接计价评价，只有 `Currency.MinorUnitScale` 改查目录且行为不变。｜**部分停用**：其决定四「本记录**不把 `Convert` 接进计价评价**：重量单位不一致仍报 `ErrWeightUnitMismatch`，因为自动换算会改金额，接不接、接在评价的哪一步、哪些口径允许，是单独的裁决。」一句，与越权风险点 1「也不让任何计价路径自动换算单位」半句，已由 [ADR-0346](./0346-evaluation-converts-input-weight-and-dimensions-to-the-units-the-plan-declares-by-the-built-in-unit-catalogue-and-records-the-factor-it-used.md) 决定一停用——评价里输入实重与尺寸的单位与方案声明的单位不同时，按本目录的精确系数换算后再走原有取整规则。**适用场景**：仅限重量与长度两类单位在计价评价内的换算；体积、件数单位与特征判定里的单位比较不在此列；其余各条不变。

Date: 2026-10-28

## Context

取证于 main `f9dc00a6a`，只记 `S`。

- **币种**只有 `parcelpricing/domain` 里的 `Currency`：三位大写字母的形状校验，加 `MinorUnitScale` 里内置的两张封闭小表（0 位一批、3 位一批，其余按 2 位答）。没有名称、没有符号，别的上下文要显示币种名只能自己写一份。
- **国家 / 地区**没有目录：`networkrouting/domain` 的 `isCountryCodeShape` 与 `partycommercial/domain` 的 `RegistrationCountryCode` 各自验两位大写的形状，注释都写“不内置码表”。管理台的公司主体页把“CN”原样摆给操作者看。
- **计量单位**是 `parcelpricing/domain` 里的 `WeightUnit`、`LengthUnit` 两个字符串类型与各处字面量（`KG`、`G`、`LB`、`OZ`、`CM`、`IN`、`PCS`、`PIECE`、`M3`、`CBM`）；重量单位不一致时评价报 `ErrWeightUnitMismatch`，没有换算，也没有一处写明某个单位叫什么、属哪一类。
- 三者都没有页面，操作者没处查“LB 是什么”“THB 小数几位”。
- 三者的内容都是**公开标准与物理常数**：ISO 3166-1、ISO 4217 由标准组织发布，磅等于 0.45359237 千克是定义值。它们不随租户而异，不属于 [ADR-0146](./0146-product-strategy-is-a-third-class-between-mechanism-and-tenant-values.md) 所说的租户取值，也不是产品策略（没有“怎么判断”可选）——是随产品发布的数据。

## Decision

**一、三份目录是产品内置的只读数据，随代码发布，不入库。** Go 里的包级只读表，目录的版本就是代码的版本；不建库表、不要迁移、没有写口，租户与操作者都改不了它。ISO 修订（新币种、旧码退役、国家更名）靠发版跟进。

**二、放 `internal/platform/refdata`，不新建限界上下文。** 它是纯 Go 数据与查询：不依赖 HTTP、`pgx` 或任何上下文，所以各上下文的领域包可以直接引用（`Currency.MinorUnitScale` 就查它），方向正常（上下文导入 `platform`，反向才被门禁拒）。限界上下文表达的是领域语言与数据所有权；这三份目录没有生命周期、没有会被违反的不变量、没有写入它的业务动作，也就没有语言和所有权要表达。读口的传输层同放 `internal/platform/refdata/refdatahttp`，认证件由 `cmd/parcel-api` 组合根接上（platform 不得导入 `accessidentity`）。目录不替任何上下文做判断：一个码在目录里，只说明它是标准里的一个码。

**三、目录内容。**
- **国家 / 地区**：ISO 3166-1 官方已分配的全部 alpha-2 码（用户指派码如 `XK` 与已撤销码不收）。每项 `Code`、`NameZh`、`NameEn`、`Region`；`Region` 是封闭七词 `ASIA`、`EUROPE`、`AFRICA`、`NORTH_AMERICA`、`SOUTH_AMERICA`、`OCEANIA`、`ANTARCTICA`，归属照联合国 M49 地理分区大洲层（塞浦路斯、土耳其、外高加索三国归亚洲，俄罗斯归欧洲，中美洲与加勒比归北美洲），只供检索分组，不是任何租户的区域维。
- **币种**：ISO 4217 常用集，覆盖跨境小包会遇到的现行币种（含 `CNY` `USD` `EUR` `GBP` `JPY` `KRW` `SGD` `HKD` 等），不含已被取代的旧码、基金与贵金属码。每项 `Code`、`NameZh`、`NameEn`、`Symbol`、`MinorUnitScale`（封闭在 0..3）。
- **计量单位**：每项 `Code`、`NameZh`、`Category`、`BaseUnit`、`FactorToBase`。`Category` 封闭四词 `WEIGHT`、`LENGTH`、`VOLUME`、`COUNT`；基准单位分别是 `KG`、`CM`、`M3`、`PCS`，系数是“1 个本单位等于多少基准单位”的精确十进制文本（`LB` 为 `0.45359237`，`OZ` 为 `0.028349523125`，`IN` 为 `2.54`，`IN3` 为 `0.000016387064`）。`CBM` 之于 `M3`、`PIECE` 之于 `PCS` 是同值别名，两行都留——计价里两种写法都在用，目录不替数据挑一个。

**四、单位换算保留精度，暂不接入计价。** `refdata.Convert(value, from, to)` 的 `value` 与结果是精确有理数（`*big.Rat`）：不取整、不截断、不经浮点；跨类别报 `ErrUnitCategoryMismatch`，目录外的单位报 `ErrUnknownUnit`。结果取有理数而不是定标十进制，因为商可能是无限小数（1 千克折多少磅），落成几位小数是调用方在自己业务位置上的取整决定。**不用仓里已有的 `Decimal`**：它在 `parcelpricing/domain`，platform 不得导入业务上下文。本记录**不把 `Convert` 接进计价评价**：重量单位不一致仍报 `ErrWeightUnitMismatch`，因为自动换算会改金额，接不接、接在评价的哪一步、哪些口径允许，是单独的裁决。

**五、不收紧任何现有校验。** `NewCurrency` 与两处国家码校验仍只验形状，目录外但形状合法的码（演示租户的合成数据、历史数据）照常被接受；`Currency.MinorUnitScale` 改为查目录，目录外的码仍按 2 位答，目录内每个币种的小数位与改查之前的两张小表逐码一致（`TestCurrencyMinorUnitScaleMatchesTheTableItReplaced` 钉着）——行为不变，只是数据搬了家。

**六、读面：三个运营查阅口。** 挂操作者渠道的“主数据与运营查阅读”能力面（[ADR-0077](./0077-master-data-catalogue-read-follows-the-operations-read-pattern.md)、[ADR-0100](./0100-operator-identity-is-a-product-owned-access-channel-family.md) 决定四），只读，不新增能力面：
- `GET /reference-data/countries`，可选 `?q=`（按码或中英文名称检索，不分大小写、子串匹配）与 `?region=`（封闭七词）；答 `{"outcome":"COUNTRIES_LISTED","countries":[{code,nameZh,nameEn,region}]}`。
- `GET /reference-data/currencies`，可选 `?q=`；答 `{"outcome":"CURRENCIES_LISTED","currencies":[{code,nameZh,nameEn,symbol,minorUnitScale}]}`。
- `GET /reference-data/units`，可选 `?q=`（按码或中文名称）与 `?category=`（封闭四词）；答 `{"outcome":"UNITS_LISTED","units":[{code,nameZh,category,baseUnit,factorToBase}]}`。
- 空检索答全表，不分页（全表是几百行的常量）；未知的 `region` 或 `category` 是调用方的错，答 400，不答空表。租户只取自认证信封（操作者在操作者册上绑的那一个），请求自报的租户一概不采信；目录本身不分租户，信封在这里只证明“这是个有授予的操作者”。未配置、凭证被拒、未授予、依赖不可用各答各的码，同其余目录读口（[ADR-0055](./0055-business-endpoint-intake-has-an-unconfigured-grade.md)）。

**七、管理台：只读页与译名显示件。** 导航“设置”组新增“基础数据”页（id `reference-data`），三签：国家 / 地区、币种、计量单位；只读，没有写入口。再加一个前端共享件，把国家码、币种码译成“中国（CN）”“人民币（CNY）”，**查不到原样显示**，并接进现在直接摆国家码或币种码的少数列表页。这只改显示，不改任何数据，也不改请求或表单里的值。

## Alternatives considered

- **入库，租户可维护。** 否决：内容是公开标准与物理常数，不随租户而异；让租户能改小数位或换算系数，等于让同一份价卡在不同租户算出不同的钱，还要为此建表、迁移、初始化与版本化。ISO 变更靠发版比靠每个租户各自维护更可控。
- **新建 `reference-data` 限界上下文。** 否决：上下文是领域语言与数据所有权的边界，这里两者皆无（见决定二）；新建一个只会让 CONTEXT-MAP 多一个没有不变量的格子，还要为三个纯查询配一整套 domain / application / ports。
- **放进 `parcelpricing/domain`。** 否决：国家码被 `networkrouting`、`partycommercial` 使用，币种被 `settlementaccounting` 使用；让它们引 `parcelpricing` 违反 [ADR-0025](./0025-cross-context-adapters-live-on-the-consumer-side.md) 的跨上下文方向，而计价并不拥有“国家叫什么”。
- **引第三方库（如 `golang.org/x/text/currency`）或运行时下载 ISO 表。** 否决：库的版本节奏与 ISO 修订不同步，且没有中文名与小包场景需要的小数位口径；运行时下载把一份公开常量变成对外部服务的运行依赖。
- **同轮把 `Convert` 接进计价，自动统一重量单位。** 否决：改金额，是计价 owner 的单独裁决（见决定四与越权风险点 6）。
- **同轮把国家码、币种码校验收紧到目录成员。** 否决：演示租户（[ADR-0150](./0150-synthetic-tenant-is-treated-as-a-real-tenant-and-isolated-form-retires-per-face.md)）与既有数据里有目录外的码，收紧会让它们无声被拒；某租户启用哪些国家 / 币种是租户取值，属后续轮次。
- **`Convert` 返回 `Decimal` 或浮点。** 否决：`Decimal` 不可达（见决定四）；浮点会让 0.45359237 这类定义值经二进制走一圈，往返不再精确。

## Consequences

- 新增 `internal/platform/refdata`（目录、查询、换算）与 `internal/platform/refdata/refdatahttp`（三个读口的处理器）；`cmd/parcel-api` 登记三行端点并自建认证件；`parcelpricing/domain` 因 `MinorUnitScale` 导入 `platform/refdata`。无迁移、无库表。
- 币种名称、符号与小数位、国家名称、单位名称与换算有了唯一来源；今后任何上下文需要这些，查目录，不再各写一份。
- 目录完整性由单元测试钉：码唯一且大写、币种小数位与旧表一致、国家数等于 ISO 已分配数、币种常用集覆盖、每个单位的基准单位存在且同类别。
- 管理台新增“基础数据”页与译名显示件。
- ISO 修订之后目录过期，直到发版；发版之前新币种或新国家码仍是形状合法的码，只是目录答不出名称。
- 租户启用范围（某租户只收哪些国家 / 币种）、单位自动换算、码校验收紧到目录成员，都是后续轮次，本记录没有为它们留任何半成品入口。

## 越权风险点

0. 目录随代码发布（决定一）。ISO 3166-1 / 4217 的修订（国家更名、币种改制、小数位变更）靠发版跟进，发版之前目录可能带着过期码；需要更快的更新节奏，要另立决定。归产品 owner。
1. 租户启用范围与自动换算是后续轮次（决定四、五）。本记录不做“某租户只允许这几个国家 / 币种”，也不让任何计价路径自动换算单位；两者都改变业务结果，需要各自的 owner 裁决。归产品 owner 与计价 owner。
2. 国家 / 币种码校验仍只验形状（决定五）。目录外的码继续被各上下文接受，包括合成数据里的码；是否收紧到目录成员、合成码怎么办，属后续裁决。归各校验所在上下文的 owner。
3. 中文名、大洲归属与币种符号取通行写法（如“中国香港”“中国台湾”冠“中国”，塞浦路斯与土耳其归亚洲，加勒比归北美洲，币种符号取常见显示符号，无通行符号的取通行缩写）。这些是会话代 owner 的取舍，只用于显示与检索，不参与任何判断；owner 可改。归产品 owner。
4. 币种常用集的取舍（决定三）：不含已被取代的旧码（如保加利亚、克罗地亚已改用欧元的旧币种）、基金与贵金属码，也不含小数位为 4 的计价单位类码——后者会破坏下游“小数位在 0..3”的封闭假设。目录外的三位码仍是合法币种码，小数位按 2 位答。归产品 owner。
5. 读口挂“主数据与运营查阅读”现有能力面而不新增能力面，且不分页（决定六）。目录本身不分租户，所以这一格的租户只起“操作者须有绑定与授予”的作用。若日后要把某类目录收窄给某些角色，要另开能力面。归访问身份 owner。
6. `Convert` 交有理数（决定四）。日后接入计价时，把有理数落成十进制并取整是计价的业务决定（舍入模式、小数位、取整一次的位置），本记录不替它选。归计价 owner。
7. `CBM` 与 `M3`、`PIECE` 与 `PCS` 作为同值别名并列（决定三）。若日后要把写法归一，是数据迁移加校验收紧，属另一份决定。归计价 owner。

## Links

- [ADR-0146](./0146-product-strategy-is-a-third-class-between-mechanism-and-tenant-values.md)：公开标准与物理常数不是租户取值，是随产品发布的数据
- [ADR-0148](./0148-route-evidence-sourcing-candidate-cost-and-first-candidate-generation-form.md)：决定四按比较币种的最小币单位取整，依赖 `MinorUnitScale` 的 0..3 上界
- [ADR-0077](./0077-master-data-catalogue-read-follows-the-operations-read-pattern.md)、[ADR-0100](./0100-operator-identity-is-a-product-owned-access-channel-family.md)：读口的模式与能力面
- [ADR-0055](./0055-business-endpoint-intake-has-an-unconfigured-grade.md)：未配置即拒的答复格
- [ADR-0025](./0025-cross-context-adapters-live-on-the-consumer-side.md)：跨上下文方向，目录不放进任一业务上下文
- [ADR-0150](./0150-synthetic-tenant-is-treated-as-a-real-tenant-and-isolated-form-retires-per-face.md)：演示租户当真租户对待，目录外的合成码照常被接受
- 票 [reference-data/01](../../.scratch/reference-data/issues/01-countries-currencies-and-units-are-product-built-in-reference-data.md)
- [ADR-0346](./0346-evaluation-converts-input-weight-and-dimensions-to-the-units-the-plan-declares-by-the-built-in-unit-catalogue-and-records-the-factor-it-used.md)：部分停用决定四「不把 `Convert` 接进计价评价」一句——评价内重量与长度的单位换算接上，其余不变
