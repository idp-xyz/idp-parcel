# 11 party-commercial 其余十八册迁到 ADR-0144

Category: enhancement
Status: resolved——2026-11-06 进 main（`098767b1e`），记录见文末「进 main 记录」
Blocked by: 01
地盘：`internal/partycommercial/{ports,adapters/http,adapters/postgres}` 里下表各册的读端口、端点与读面及其测试（客户账户已迁，不在内）；`cmd/parcel-api` 里这些读口的占位；
`migrations/party_commercial` 的一条新迁移（索引）。
出处：ADR-0144 决定三至七；spec「横贯各子票的取舍」。

## 本票的册（今天的次序钉 `a1274a6b8`）

| 路由（选择器） | 读端口方法 | 今天的 `ORDER BY` |
|---|---|---|
| `/commercial-business-parties` | `PartyIdentityCatalogueRead.ListBusinessParties` | `recorded_at DESC, party_id`（折叠到最新修订后） |
| `/commercial-group-legal-entities` | `PartyIdentityCatalogueRead.ListGroupLegalEntities` | `recorded_at DESC, legal_entity_id` |
| `/commercial-party-relationships` | `PartyIdentityCatalogueRead.ListPartyRelationships` | `recorded_at DESC, relationship_id` |
| `/commercial-product-channel-mappings` | `ProductChannelMappingCatalogueRead.ListProductChannelMappings` | `recorded_at DESC, mapping_id` |
| `/commercial-registration-number-types` | `RegistrationNumberTypeCatalogueRead.ListRegistrationNumberTypes` | `recorded_at DESC, country_code, type_code` |
| `/commercial-service-products` | `ServiceProductCatalogueRead.ListServiceProducts` | `published_at DESC, object_id, version_label` |
| `/commercial-customer-contracts` | `CommercialRelationCatalogueRead.ListCustomerContracts` | `published_at DESC, object_id, version_label` |
| `/commercial-supplier-agreements` | `CommercialRelationCatalogueRead.ListSupplierAgreements` | `published_at DESC, object_id, version_label` |
| `/commercial-policies`（九种 `kind`） | `CommercialPolicyCatalogueRead` 的九个方法 | 版本壳按 `published_at` / `declared_at` / `registered_at` 倒序加 `object_id, version_label` |
| `/commercial-open-publication-drafts` | `PublicationDraftRead.ListOpenDrafts` | `submitted_at DESC, object_kind, object_id, version_label` |

## 做法与判据

见 spec「每张迁移子票（02–13）的做法与判据」。本票特有：

- `kind` 保持册子选择器原义（决定四），九种各是一册。
- 身份三册与客户账户同一份登记册、同一个生命周期导出，状态维的口径与客户账户一字不差。
- 修订链三口（法人、业务参与方、产品渠道映射的 `…/revisions`）有意不设上限，不在内。

## 完成记录（2026-11-06，分支 `cursor/crpr-11-party`，基 `d48c5b130`）

**落点**

| 笔 | 做了什么 |
|---|---|
| `4cb3aca5c` | 参与方身份三册：声明、读端口、本包取页形状 `catalogueRead`、端点与 `writePageQueryProblem`、占位、旧用例调用点、真库与端点用例；迁移 `0047` 各册索引一次写齐 |
| `2166fde0d` | 注册号类型、渠道产品映射 |
| `75a859b40` | 服务产品、客户合同、供应商协议；按身份读回与取页共用同一段 `listed`（ADR-0361） |
| `ecfe92b0e` | 商业策略目录：`kind` 登为选择器、每种各是一册，`commercialPolicyPages` 按 `kind` 选声明 |
| `b77c4f94a` | 开放中的发布载体：本构建读不回的载体照样在册、计入总数、占页里的位置（ADR-0344） |
| `9e8a54578` | 各列表端点改为先 Intake 后解码，`/commercial-policies` 的 `kind` 校验一并挪到 Intake 之后 |
| `0ee264cb4` | 可排维、筛选维从小放；JSON 数组上的 `q` 按元素；真库夹具改 `SYN-`；注释去掉迁移前后的叙述与数别处东西的计数 |
| 本笔 | 完成记录；声明头注补缺省序不是 `-registeredAt` 的理由 |

依赖 01 的两笔 `7b1166838`、`9726b2f35` 都在基里。

**各册声明**（声明在 `ports/catalogue_pages.go`，落列与 `q` 在各 postgres 读面的 `…Read` 上）

| 册 | 缺省序 | 可排维 | 筛选维 | `q` 覆盖 | 索引（`0047`） |
|---|---|---|---|---|---|
| 业务参与方 | `-registeredAt` | registeredAt、effectiveFrom、partyId | status（时点导出，与客户账户同一条 CASE） | 参与方号、名称、终身注册号（逐号）、外部编码（逐项「系统 编码」） | 新 `business_party_registration_by_recorded_at` |
| 集团法人 | `-registeredAt` | registeredAt、effectiveFrom、legalEntityId | status、registrationCountry（开放码） | 法人号、所钉参与方的标识与名称、注册国家、终身注册号（逐号） | 新 `legal_entity_registration_by_recorded_at` |
| 参与方关系 | `-registeredAt` | registeredAt、effectiveStartsAt、relationshipId | status（登记事实）、role | 关系号、双方的标识与名称 | 换 `party_relationship_registration_by_tenant` 补标识段 |
| 注册号类型 | `-registeredAt` | registeredAt | status、layer、countryCode（开放码） | 国家 / 地区、类型代码、类型名称 | 换 `registration_number_type_registration_by_tenant` 补标识段 |
| 渠道产品映射 | `-registeredAt` | registeredAt | — | 映射号、服务产品与版本、依据、渠道引用（逐个） | 换 `product_channel_mapping_registration_by_tenant` 补标识段 |
| 服务产品 | `-publishedAt` | publishedAt、objectId | status（折进退役之后的位置） | 对象、版本、范围、形态 | 新 `commercial_version_by_kind_published`（版本壳各册共用，类别为等值前缀） |
| 客户合同 | `-publishedAt` | publishedAt、objectId | status（折进退役之后的位置） | 对象、版本、范围、规则包、客户账户 | `commercial_version_by_kind_published` |
| 供应商协议 | `-publishedAt` | publishedAt、objectId | status（折进退役之后的位置） | 对象、版本、范围、供应商、法人、价格表、协议范围 | `commercial_version_by_kind_published` |
| 授权规则（`kind`） | `-publishedAt` | publishedAt、objectId | status（折进退役之后的位置） | 对象、版本、范围 | `commercial_version_by_kind_published` |
| 客户服务规则（`kind`） | `-publishedAt` | publishedAt、objectId | status（折进退役之后的位置） | 对象、版本、范围、服务产品、客户合同、责任方、规则范围 | `commercial_version_by_kind_published` |
| 接受前财务控制策略（`kind`） | `-publishedAt` | publishedAt、objectId | status（折进退役之后的位置） | 对象、版本、范围 | `commercial_version_by_kind_published` |
| 接单规则包（`kind`） | `-declaredAt` | declaredAt、objectId | — | 对象、版本、服务产品、合同、法人、范围 | 新 `acceptance_rule_package_by_declared` |
| 接受前财务控制声明（`kind`） | `-declaredAt` | declaredAt、contractObjectId | requirement | 合同对象、合同版本、不适用依据 | 新 `pre_acceptance_control_declaration_by_declared` |
| 价格政策（`kind`） | `-registeredAt` | registeredAt、objectId | direction | 对象、版本、方案引用、政策范围 | 新 `commercial_price_policy_by_registered` |
| 结算政策（`kind`） | `-registeredAt` | registeredAt、objectId | method | 对象、版本、法人、对方、合同、费用范围 | 新 `commercial_settlement_policy_by_registered` |
| 信用政策（`kind`） | `-registeredAt` | registeredAt、objectId | — | 对象、版本、法人、权限等级、费用类型 | 新 `credit_policy_by_registered` |
| 时点锚声明（`kind`） | `-declaredAt` | declaredAt、rulePackageObjectId | judgmentType | 规则包对象、规则包版本、语义引用、策略版本 | 新 `as_of_policy_declaration_by_declared` |
| 开放中的发布载体 | `-submittedAt` | submittedAt | status（待批准 / 已批准） | 对象、版本、范围、录入者、批准者 | 新部分索引 `publication_draft_open_by_submitted` |

**完成判据**（钉 `0ee264cb4`；本笔只动注释与票面）

- ✅ 真库用例（`SYN-` 夹具，带 DSN）：
  - 身份三册（`party_identity_catalogue_page_test.go`）：翻页途中登记新行，后面各页不重出、不漏行，total 由 5 到 6；空租户答空页、total 为 0（三册各跑一遍）。各册的筛选与 `q` 表里每一问的 total 都等于本页行数、随条件变；状态词表每一格都有实例；`q` 不命中快照里的键名与 JSON 标点，相邻两个号、两个外部编码拼不成一次命中，`%` 按字面。
  - 注册号类型、渠道产品映射（各自的 `*_catalogue_page_test.go`）：翻页插入、空册、筛选或 `q` 改 total 各过一遍；注册号类型另证同一事务登记的几行登记时刻逐位相同、按行标识与排序同向收尾，渠道产品映射另证渠道引用按元素命中。
  - 版本壳各册（`version_shell_catalogue_page_test.go`）、策略正文各册（`policy_content_catalogue_page_test.go`）：表驱动，每册翻页插入、空册、筛选或 `q` 改 total 各过一遍；版本壳的 status 按折进退役之后的位置筛。
  - 开放中的发布载体（`publication_draft_catalogue_page_test.go`）：翻页插入、空册、筛选或 `q` 改 total 各过一遍，另证读不回的载体照样受筛、占页里的位置、计入总数，游标从它接得下去。
- ✅ 端点用例（`adapters/http/catalogue_pages_test.go`，表驱动，每册一行）：合规一问带 page、size 取 Intake 页大小（替身 Intake 取 37，写死页大小过不了）；未知键、集外排序维（含本票裁掉的维）、词表外筛选值、坏游标各答 400 带 detail，读口不被调用；渠道未配置时合规与不合规的一问都答同一份 403、不进读口；换条件拿旧游标答 400，条件不变则把位置交给读口。`TestCommercialPolicyCursorDoesNotCrossKinds` 证一种 `kind` 的游标翻不了另一种；`query_commercial_catalogue_test.go` 证 `kind` 缺席、为空、集外在配置时答 400，未配置时与合法 `kind` 同一份 403。
- ✅ 迁移无 CR、无 BOM。
- 门（作者自验，WSL go1.26.8）：`gofmt -l` 空；全仓 `go build`、`go vet` 退 0。带 DSN `-p 1 -count=1`，反向依赖（`go list -test -deps` 反查本票改过的 `ports`、`adapters/http`、`adapters/postgres`）全过、无 SKIP：`internal/partycommercial/adapters/postgres` 446 PASS，`internal/partycommercial/adapters/http` 306 PASS，`internal/partycommercial/application` 280 PASS，`cmd/parcel-api` 654 PASS，`cmd/parcel-commercial` 166 PASS，`cmd/parcel-dispatch` 506 PASS，`internal/transportfulfillment/application` 1326 PASS，其余反向依赖包同样全过；`internal/architecture/...` 174 PASS，`migrations`、`internal/platform/migrate` 过。计数取证：`grep -c -- '--- PASS'` 与 `grep -c -- '--- SKIP'` 打在各包 `-v` 输出上。

**判断项**

1. **决胜键改同向**（spec「行标识取登记表主键去掉租户那几段」，与排序同向作决胜键）：本票各册分页之前都是首列倒序、标识正序（本票册表「今天的 `ORDER BY`」），迁移后标识随排序倒序，只在首列完全相同的行之间次序不同。同一事务登记的几行登记时刻逐位相同，这种情形真实存在，注册号类型的用例钉着。
2. **行标识偏离「主键去掉租户」**：身份三册、渠道产品映射与注册号类型的主键还带修订号（`0015`、`0016`、`0033`），读面先按主键折叠到最新修订、每个对象一行，行标识不带修订号；版本壳各册与策略正文各册的主键带 `object_kind`，版本壳在 `listed` 里按类别等值取，正文表的 `object_kind` 由 CHECK 钉成一格，行标识不带类别。开放中的载体取类别、对象、版本，即主键去租户。各册都用统一结果类型，没有自带 `Next` / `Total` 的端口。
3. **从小放裁掉的维**（spec「可排维、筛选维从小放」）：注册号类型、渠道产品映射与开放中的载体只留缺省维；版本壳与策略正文各册不开 effectiveStartsAt（`ecfe92b0e` 曾开，`0ee264cb4` 删）；载体不设 `kind` 筛选维。留下的维各有依据，写在声明上：身份三册照管理台表头排序；版本壳与策略页按对象归组展示，开对象号；注册号类型的 layer、countryCode 供法人表单挑候选。名称列不作可排维：名称在法人册与关系册是左连接来的、可缺（spec「可排维与行标识的列必须非空」）。
4. **关系册缺省序**：服务端缺省 `-registeredAt`（分页之前的首列），管理台关系页在客户端按生效起排；管理台接分页归 14，届时带 `sort=-effectiveStartsAt` 或改用服务端缺省。
5. **先 Intake 后解码带来的行为变化**：未配置渠道上，`/commercial-policies` 缺 `kind`、坏 `kind` 由答 400 改为同一份 403（ADR-0055 决定三）；配置了的渠道上 `kind=` 为空仍答 400，与分页之前相同。词表口 `/commercial-publication-vocabularies` 不是本票的册，次序未动，仍先校验 `kind` 再 Intake；它的头注原称「分支选择不泄露任何东西」，不实（未配置时集外 kind 答 400、合法 kind 答 403），已改成如实的说法，挪到 Intake 之后归 17 号票。
6. **自报参数从「被忽略」变「答 400」**：本票各口分页之前除 `kind` 外不读查询参数，带什么都照答；迁移后集外键答 400（ADR-0144 决定四）。隔离读实例（ADR-0078）的 Intake 照旧无视自报，但带 `?tenantId=`、`?tenant=` 的手工核对式（如 admin-web-page-wiring-frontier/07 记的取证命令）现在答 400，复核时去掉那个键即可。
7. **total**：与本页是两条语句，`ReadExecutor` 不保证同一快照，两句之间新登的一笔会让 total 与本页差一行，下一次取页即对上（`catalogueRead.page` 头注）。本票各册按参与方、合同、政策、载体计量，不是 spec「`total` 一律给精确值」点名的按包裹、交易、事件计量的那类大册，total 照给精确值。spec「已带计数与 `truncated` 的待办队列保留这两格」不适用：本票各口分页之前都不带计数与 `truncated`。
8. **状态维不走索引**：版本壳的 status 按折进退役之后的位置筛（`versionStatusCode` 与目录行同一个折法），身份三册与注册号类型的 status 是时点导出，都不是库上的列，`0047` 的索引不服务这一维。DRAFT 不在版本壳的词表里（发布登记册不收草稿），筛它答 400 而不是空页。
9. **按修订折叠的册**（身份三册、渠道产品映射、注册号类型）：外层排序穿不过折叠，`0047` 给它们补的索引暂无读者，与 `0035` 客户账户那条同一处境（`0047` 头注）。开放中的载体把状态条件写成字面量，与 `0047` 部分索引的谓词同一组。
10. **管理台与脚本带的参数**：`apps/admin-web` 只给 `/commercial-policies` 带 `kind`，登为选择器（各册 `Selectors: kind`，取值进游标摘要）；其余各口管理台不带参数，`scripts/` 不调这些路由。答复只多一格 `page`，管理台不改照旧能用，但渠道产品映射页、法人表单的注册号类型候选等只拿到第一页，接分页归 14。
11. **不设筛选维的册**：渠道产品映射没有状态代数（行上不导出状态），收窄只靠 `q`；信用政策的权限等级与费用类型、接单规则包的各格都是开放引用，不设筛选维。
12. **修订会把行挪过游标**：按修订折叠的几册按最新一笔修订的登记时刻排，开放中的载体在 `SubmitDraft` 遇冲突时改写提交时刻。翻页途中被修订的对象挪到已翻过的那头，这一轮翻不到，下一轮从头取即见。这正是 ADR-0144 Context 里「当前修订被新修订取代」的情形，与客户账户目录同形。

**改到的共享文件**：`cmd/parcel-api/unwired_orchestration.go` 只改本票读口的占位方法签名（`unwiredPublicationDraftRead.ListOpenDrafts` 与 `unwiredCommercialCatalogue` 上本票各册的 `List…`），邻行未动。

**未验**

- 未跑 EXPLAIN：`0047` 各索引（尤其载体的部分索引）是否被规划器选用，没有在真库上看计划。
- 管理台未接（归 14）；全量 `scripts/test-full.sh` 由推送方在重放后的 tip 上跑。

## 评审记录

**评审 ← 子代理 · 钉 `5b91b6517`**：两轴无阻断（首次评审被接口限流打断，续上后在同一棵干净的隔离树上做完）。

| 评审意见 | 处理 |
|---|---|
| 非阻断：词表口仍先校验 `kind` 再 Intake，未配置时集外 kind 答 400、合法 kind 答 403，封闭集可被探出；头注还写着「分支选择不泄露任何东西」 | 不在本票的册；头注改成如实的说法，挪到 Intake 之后立 17 号票；判断项 5 补记 |
| 非阻断：修订会把行挪过游标，判断项没点名 | 判断项 12 |

修订由派单方在本分支上做（作者会话已收工），只动一段头注与本票面。

## 进 main 记录（2026-11-06，通道 1 推送）

**出处**：分支 `cursor/crpr-11-party`，tip `92ba89921`。**评审**：钉 `5b91b6517` 两轴无阻断。

**main 上的 SHA**：`098767b1e`（重放到当时的 `origin/main` `0874218c9` 之上：各分支逐笔 cherry-pick（带 `-x`，出处在提交说明里），推送方补的几笔见下）。

| 步骤 | 结果 |
|---|---|
| 重放 | 01/02 分支与 03–13、15、14a 各分支共 106 笔，无冲突 |
| 推送方补的几笔 | `de1fb75d8` AT-TF-333 改指新用例名；`66a689cbc` `cmd/parcel-api` 三处注释改成现状；`29af46c24` 删掉 15 留的两个别名；`6aff3149a` 管理台路线计划契约用例认 `page`；`446252dc4` 重生成机制清点；`098767b1e` 01 与 05 叠合处的用例 |
| 生成物 | `go run ./tools/adr-index -check`、`go run ./tools/at-coverage-counts -check` 在 tip 上退 0；机制清点在 `446252dc4` 重生成，tip 上再生成无差异 |
| 全量真库测试 | `scripts/test-full.sh` 对 `098767b1e` 退 0（175 个包 ok、17 个包无测试文件）。前一跑对 `446252dc4` 红一条：01 的 NUL 补件与 05 的挡回册判据用例叠合，`098767b1e` 修后重跑全绿 |
| 种子灌一次性库 | 升级路径：`0874218c9` 的代码灌库 A，本批代码对库 A 迁移退 0（施加 11 条新迁移）；本批代码灌空库 B 退 0；两库用后删 |
| 管理台三道门 | tsc 退 0、run-tests 1236 条全过、vite build 退 0 |
| 推确切 SHA | `git push origin 098767b1e:main`，`origin/main` 等于 `098767b1e` |
