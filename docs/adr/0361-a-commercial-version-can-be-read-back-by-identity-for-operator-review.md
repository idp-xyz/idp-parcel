# ADR-0361：商业版本可由运营操作者按身份读回一条，答与该类别目录行同形的一份

Status: Accepted（2026-11-04，Claude Code 会话代 owner 决，用户授权代决；票 commercial-version-object-pages/01。裁决能力边界：读过 CONTEXT「商业版本」、ADR-0077 与三个目录读口（`ServiceProductCatalogueRead`、`CommercialRelationCatalogueRead`、`CommercialPolicyCatalogueRead`）及其 Postgres 实现、ADR-0314 的载体读回端点；越权风险点见文末。）<!-- 索引注记 -->｜**入口**：服务产品、客户合同、供应商协议与各政策册只有列表读口，一个版本没有自己的地址——管理台的对象页深链不到「这一版」，只能带检索词落到目录。**取法**：一个按身份读回的读口 `GET /commercial-version-readbacks/{kind}/{objectId}/{version}`，挂操作者查阅读面；身份是（租户、类别、对象、版本），答与该类别目录行同形的一份；三个目录读口各加 `Find…`，Postgres 实现是各自 `List…` 的投影（同一条语句多两格键过滤）。
Date: 2026-11-04

## Context

- CONTEXT 把商业版本定义为具有稳定身份、不可覆盖的对象：一经发布，（租户、类别、对象、版本）四格钉住它，内容不再变。库上 `party_commercial.commercial_version` 的主键正是这四格；范围（`scope_ref`）是壳上的一列，不是键。
- ADR-0077 立了目录查阅的读面：每本册一个列表读口，租户内按发布时刻倒序、页大小由接入面裁为固定上限；目录上列不触发判断。管理台的行详情在工作台右栏，只能从已取回的列表里找行——列表翻不到的那一版，右栏就打不开；引用链接只能带检索词落到目录，另开标签、分享、从别处深链都指不到「这一版」。
- ADR-0314 已为待批准载体立了单份读回（`/commercial-publication-draft-readbacks/{kind}/{objectId}/{version}`）：路径三格就是身份，Intake 先于路径，不在册与跨租户同形。已发布的版本没有同形的口。
- 各类别的目录行形状互不相同（服务产品带形态与交付条件，合同带正文与绑定，规则包带父子聚合……）。对象页要显示的就是目录上那一行，不是另一种形状。

## 候选与反方

- **管理台只靠列表，对象页从列表里找行。** 否决：页大小是固定上限，翻不到的版本页就打不开；深链与分享指不到「这一版」是本票的出处。
- **每本册各开一个单条读口（`/commercial-service-products/{id}/{version}` 等）。** 否决：一册一条路径、一册一个处理器，而它们做的是同一件事（按身份读一行）；类别已是领域封闭集，一条路径带 `kind` 就把集合表达出来了，与载体读回同形。
- **按身份读回另写语句、另写行转写。** 否决：对象页显示的是「目录上这一版」，两套转写在某个格上迟早不同（列表加了收尾回显四格、读回没加），管理台会在两页显示两样；投影同一条语句，列表上那一行与读回的那一行逐字段相同是结构保证的，不靠测试追。
- **范围进键（`/{kind}/{scope}/{objectId}/{version}`）。** 否决：范围不是主键的一部分，同一（类别、对象、版本）在一个租户内只有一行；把它放进路径等于让调用方多提供一格它不该需要知道的东西，提供错了还要再裁答什么。
- **接受判断时点策略与接受前财务控制声明也进本口。** 否决：两者没有自己的版本——挂在接单规则包版本 / 客户合同版本下，身份是拥有者的；它们本来就不在 `CommercialObjectKind` 里，给它们一个版本地址就是替一份声明造一个它没有的身份。
- **单一读口 + 三个目录读口各加 `Find…`、Postgres 投影列表语句（采纳）。**

## Decision

**一、一个按身份读回的读口。**（机制）

- `GET /commercial-version-readbacks/{kind}/{objectId}/{version}`，挂操作者查阅读面（`MASTER_DATA_AND_OPERATIONS_READ`，与各目录列表口同面）。`kind` 是领域的商业对象类别闭集（`CommercialObjectKindNamed`：`SERVICE_PRODUCT`、`CUSTOMER_CONTRACT`、`SUPPLIER_AGREEMENT`、`ACCEPTANCE_RULE_PACKAGE`、`PRE_ACCEPTANCE_FINANCIAL_CONTROL_POLICY`、`PRICE_RULE`、`SETTLEMENT_POLICY`、`CREDIT_POLICY`、`AUTHORIZATION_RULE`、`CUSTOMER_SERVICE_RULE`）；答复里的 `kind` 回显领域原词，不是目录列表页签用的册名（`PRICE_RULE`，不是 `PRICE_POLICY`）。
- 身份是（租户、类别、对象、版本）；租户来自 Intake 出的作用域，范围不是键。
- 在册答 200 `{ outcome: "COMMERCIAL_VERSION_READ", kind, version: <与该类别目录行同形的一份> }`。不在册答 200 `COMMERCIAL_VERSION_NOT_FOUND`（ADR-0022：状态码只说答案有没有形成）；别的租户的版本与不在册同形，不泄露存在性。类别或标识不合法答 400。
- Intake 先于路径（ADR-0055）：未配置要拒在一切请求内容之前，路径也是请求内容。
- 接受判断时点策略与接受前财务控制声明不是本口的类别（它们没有自己的版本，见候选与反方）。

**二、三个目录读口各加按身份读回的 `Find…`，Postgres 实现是列表的投影。**（机制）

- `ServiceProductCatalogueRead.FindServiceProduct`；`CommercialRelationCatalogueRead.FindCustomerContract` / `FindSupplierAgreement`；`CommercialPolicyCatalogueRead` 对有自己版本的每本册一个：`FindAcceptanceRulePackage`、`FindPricePolicy`、`FindSettlementPolicy`、`FindAuthorizationRule`、`FindCreditPolicy`、`FindCustomerServiceRule`、`FindPreAcceptanceFinancialControlPolicy`。签名（租户、对象、版本）答（行、在不在、错）。
- Postgres 实现与各自的 `List…` 共用同一条语句、同一段扫描，只多两格键过滤（`object_id`、`version_label`），不另写第二套行转写；列表的排序、页大小校验与错误处置不变。
- HTTP 处理器按类别分派到对应 `Find…`，`version` 由该类别目录端点在用的 `…BodyOf` 折出——对象页与目录页显示的是同一份转写。

**三、不在本记录。** 对象页的形态与地址（子票 02，产品策略）；按身份读回正文册的领域对象（内容读口 `Load…` 已有，那是判断的输入，不是查阅）；版本历史的单独读口（对象页从目录取）。

## Consequences

- `ports`：三个目录读口各加 `Find…`。
- `adapters/postgres`：受影响的 `List…` 各折成一个私有装载函数加 `List…` / `Find…` 两个入口；`versionKey` 把键过滤接到语句上。
- `adapters/http`：新端点 `NewQueryCommercialVersionEndpoint`；三个读口接口同步加 `Find…`；结算政策的行转写抽成 `settlementPolicyBodyOf`（此前内联在列表口里）。
- `cmd/parcel-api`：端点表一行（操作者查阅读面，带探针），装配交入同一只商业目录适配器的三个读口。
- 管理台：对象页按本口取「这一版」（子票 02）。
- 无迁移。

## 越权风险点

1. **按主键读一行走的仍是列表语句的 ORDER BY + LIMIT 1**（PC owner）。键是主键，至多一行，排序对结果无影响；规划器对主键等值过滤会走索引。若日后某册的列表语句长出代价高的聚合（规则包的子查询那一类），按身份读回也付同一份代价——那时再裁是否拆语句，本记录不预设。
2. **`kind` 用领域原词，目录列表用册名**（管理台）。同一本册在两口叫法不同（`PRICE_RULE` / `PRICE_POLICY`），管理台要各用各的；统一叫法要改列表口的对外契约，不在本记录。
3. **读口挂操作者查阅读，不挂登记写授予**（访问身份 owner）。与各目录列表口同面，不扩大可见范围：能列目录的人本就能看到这一行。
4. **不在册与跨租户同形**（PC owner）。管理台对象页对「这一版不存在」与「不是你的租户」显示同一句，这是刻意的。

## Links

- [ADR-0077](./0077-master-data-catalogue-read-follows-the-operations-read-pattern.md)：目录查阅读面与列表读口；本记录给它加按身份读回的一面
- [ADR-0314](./0314-pending-publication-drafts-are-readable-back-by-the-operator-and-approval-and-publication-are-bound-to-the-content-digest-the-caller-saw.md)：待批准载体的单份读回；本记录的路径形状与不在册答法与之同形
- [ADR-0022](./0022-http-status-carries-answer-formed-not-business-verdict.md)：`COMMERCIAL_VERSION_NOT_FOUND` 答 200
- [ADR-0055](./0055-business-endpoint-intake-has-an-unconfigured-grade.md)：Intake 先于路径
- [ADR-0040](./0040-commercial-version-key-carries-tenant-id.md)：商业版本的键带租户
