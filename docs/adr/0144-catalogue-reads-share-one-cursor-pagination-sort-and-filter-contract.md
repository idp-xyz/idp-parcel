# ADR-0144：目录读口的分页、排序与筛选下推一处定义——游标分页、每册点名的封闭集排序与筛选维加统一关键词 `q`、页大小仍由 Intake 按渠道契约定、答复带 `page`；契约一次定，读口逐册迁

Status: Accepted（2026-09-24，通道 3 经用户在 IDP 队列授权自决——「你能帮我自决吗，你是业务和系统专家」——裁决；票 [admin-web-group-legal-entities/04](../../.scratch/admin-web-group-legal-entities/issues/04-catalogue-read-pagination-sort-filter-contract.md) 的产物，五问的裁决原文在票面「裁决」节，本记录把它写成契约并补齐细处，两处措辞有出入时以本记录为准。裁决能力边界：读过 [ADR-0077](./0077-master-data-catalogue-read-follows-the-operations-read-pattern.md) Decision 全节；`internal/partycommercial/adapters/http/catalogue_intake.go` 全文（`CatalogueQuery`、`CommercialCatalogueIntake`、`problemDetail` 与 `writeProblemWithDetail`）；同包 `query_party_identities.go` 的三种答复体与 outcome 常量、`query_commercial_policies.go` 对 `?kind=` 的封闭集分派、`isolated_read_intake.go` 的 `IntakeCatalogueQuery`；`cmd/parcel-api/assemble_isolated_read.go` 的 `isolatedReadLimit` 常量注与 `buildIsolatedReadIntakes` 开头；`internal/parcelpricing/adapters/postgres/operations_catalogue.go` 的 `catalogueLimit`；party-commercial postgres 适配器各 `ORDER BY` 子句（`git grep`）；`apps/admin-web/README.md`「列表页上列通则」；`apps/admin-web/src/templates/ListPageTemplate.tsx` 的 `ListPaginationProps`；`pages/party` 三份列表模型的头注。没读：其余上下文的目录处理器与 postgres 读口全文、[ADR-0071](./0071-catalogue-views-carry-tenant-in-the-method-signature.md)、ADR-0076 与 ADR-0078 全文、各登记表的索引现状、vendor `Pagination` 源码。拿不准的列在文末「越权风险点」。）
Date: 2026-09-24

## Context

**问题。** 各上下文的目录读口今天都是答满一页就停：`CatalogueQuery` 只有作用域与 `Limit` 两格，`Limit` 由 Intake 按渠道契约注入（隔离形态即 `isolatedReadLimit`），读适配器照它截断；没有翻页、排序与筛选参数。管理台因此立了通则「过滤只在已取回的数据上做……那要改端点契约」，`pages/party` 的列表模型头注也写明「这里的排序是对这一页排，不是对册排」。演示种子的量级下看不出问题；租户规模一上来（几百个货主客户账户、上千条网络版本行），在截断后的那一页上再筛，答出的「没有」可能是假的——被筛的行根本没取回来。

**已有的纪律，本记录必须接住。**

- 页大小不采信调用方自报：`CatalogueQuery` 头注「页大小由接入面按渠道契约裁决——两样都不采信调用方自报」。
- 查询参数是封闭集：`/commercial-policies?kind=` 缺席或集外按坏请求拒（`MALFORMED_REQUEST`），前端对它的文案是「查询参数不在服务端接受的封闭集合内」。
- 4xx 可带理由散文：`problemDetail.Detail` 只随 4xx 在场，前端原样示出、不据此分支。
- 键形状：租户维在方法签名上，`limit` 非正拒（ADR-0077 Decision 五；`catalogueLimit` 是它的一处实现）。
- 空目录如实答空，走 2xx 成格（ADR-0077 Decision 四）。

**各册今天的次序各不相同。** 以 party-commercial 为例，各读面的 `ORDER BY` 有按标识的、按登记号的、按修订号的，没有统一的缺省序——截断发生时留下哪些行，由各册的次序偶然决定。

## 候选与反方

**甲 · 偏移分页（`page` / `pageSize`）。** 操作者熟，`ListPaginationProps` 现成就是页码形。反方：登记册只增不改，列表的自然序又是「新登的在前」——翻页期间一有新登记，第二页开头就会重出第一页末尾那一行；当前修订被新修订取代时，后面的行同样会前移，漏掉一行。操作者逐条复核时恰恰不能漏。`OFFSET` 在大页码上还要扫过前面所有行。同一份契约将来要服务包裹、委托这类量级大得多的列表。否决。

**乙 · 任意字段排序与筛选（或通用查询语言）。** 反方：与本仓查询参数一律封闭集的纪律相违；每开放一个字段，就多一个要索引、要校验、要在各册之间保持一致的面；通用查询语言等于把各册的存储形状当成对外契约。否决。

**丙 · 维持现状，只把 `isolatedReadLimit` 调大。** 反方：调大只是把截断点推远，截断后的假「没有」仍在；`isolatedReadLimit` 是隔离形态的注入值，真渠道上页大小归渠道契约，调它解决不了真渠道的问题。否决。

**丁 · 游标分页 + 每册点名的封闭集排序与筛选维 + 统一关键词。** 即本记录。代价：不能跳到第 N 页；总数要多一次 `COUNT`。

## Decision

**一、翻页用游标（keyset）。** 请求参数 `after=<游标>`，缺席即第一页。游标由服务端编成、对客户端不透明，内含本次的排序维、末行在该排序维上的值与末行标识，以及本次排序与筛选（含 `q`）的摘要。服务端解开游标后，按「排序维值 + 标识」取严格排在其后的行。游标解不开，或摘要与本次请求的排序 / 筛选不符 → `MALFORMED_REQUEST`，理由散文说「游标与本次的排序或筛选不符，请从第一页重取」——换了条件还拿旧游标，翻出来的是另一份列表的中段，不能静默照翻。服务端只给向后的游标；「上一页」由客户端记住走过的游标自己回退。

**二、页大小仍由 Intake 按渠道契约定，调用方不给。** 不新增 `limit` / `pageSize` 参数；隔离形态的页大小仍是 `isolatedReadLimit`，含义不变。答复回显本次使用的页大小（决定五），客户端据此算页数。

**三、排序：每册点名的封闭集。** 参数 `sort=<字段名>`，前缀 `-` 为倒序；一次只按一维排，服务端总以标识作决胜键（同值时按标识同向排），游标才稳定。字段名取答复体里的 JSON 字段名。可排的维由各册在自己的读口里列出，集外 → `MALFORMED_REQUEST`。**缺省序为 `-registeredAt`（登记时间倒序，新登的在前）**；没有登记时间一格的册，在自己的封闭集里点名缺省序，并在读口头注写明为什么。

**四、筛选：每册点名的封闭集维度 + 统一关键词 `q`。**

- 维度参数名取答复体里的 JSON 字段名（如 `status=ACTIVE`）；同一维可重复给，维内为「或」、维间为「与」；值属封闭词表的维，值也按词表校验。已被用作册子选择器的参数（`/commercial-policies` 的 `kind`）保持原义，不兼作筛选维。
- 有状态的册至少放出状态一维；其余维由各册按自己的关键标识 / 种类点名，不开放任意字段。
- `q`：去掉首尾空白，空即缺席；在该册点名的文本列（标识、名称这类）上做不分大小写的包含匹配；长度上限在共用件里定一次。管理台地址上的检索词与它同名（票 `admin-web-workspace-form/06`），读口支持之后原样下推。
- 未知参数键 → `MALFORMED_REQUEST`：多给的键若被静默忽略，前端以为筛了、服务端其实没筛，答出的列表与屏上的筛选条件就对不上。

**五、答复：原有形状外加一格 `page`。**

```json
{ "outcome": "…", "<各册既有的集合键>": [], "page": { "size": 200, "next": null, "total": 0 } }
```

`size` 是本次使用的页大小（不是本页实际行数）；`next` 为 `null` 即已到末页。目录读口给精确 `total`——按租户读的册在租户级规模上 `COUNT` 便宜，而「共 N 条」是操作者判断筛得对不对的依据；契约允许将来的大表读口显式给 `null`，客户端见 `null` 不显总数、不去猜。`total` 与本页出自同一组筛选条件，不另开口径。空目录仍如实答空：集合为空、`next` 为 `null`、`total` 为 `0`，走 2xx 成格（ADR-0077 Decision 四不动）。

**六、谁解码什么。** Intake 只管它今天管的两样——作用域与页大小（授权结果与渠道契约）。`after`、`sort`、筛选维与 `q` 是调用方的查询意图，不属授权，由端点处理器经共用件解码成查询对象，与作用域、页大小一起交给读端口。读端口签名从「租户 + limit」扩为带查询对象、答「本页行 + 下一游标 + 总数」；租户仍在签名上、`limit` 非正仍拒（ADR-0077 Decision 五不动）。

**七、一处定义，逐册迁移。** 游标编解码、参数校验（键集、排序维、筛选维、`q` 的规整与长度上限）与答复 `page` 的拼装落在 `internal/platform` 的一个共用件里，不含任何上下文的词；各册只声明自己的排序维、筛选维、`q` 覆盖的列与缺省序。逐册迁移，按规模先到的先改——客户账户与网络版本两册打头；每迁一册，它的登记表补一条与缺省序同形的索引（租户 + 排序维 + 标识）。未迁的册维持今天的「答满一页」，管理台对它们维持客户端筛选。

**八、管理台一次接。** `ListPageTemplate` 的 `pagination` 槽加游标模式（上一页 / 下一页、「共 N 条」、当前页 / 总页数），向后兼容——今天的页码模式不动。检索词按票 `admin-web-workspace-form/06` 已写进地址，下推时原样带上；筛选条的维度随各册迁移改为下推。

## Consequences

- 各上下文读端口改签名、各登记表补索引，都是逐册的实施票，不在本记录里做。首批两册迁完之前，管理台的列表行为不变。
- 管理台 README「列表页上列通则」里「过滤只在已取回的数据上做」一条，要改为「已迁到本契约的读口：筛选与检索下推；未迁的：只在已取回的数据上做」，由首个迁完那一册的票改。
- `ListPaginationProps` 的页码形保留给将来确需页码的场景；游标模式不提供跳页，操作者找行靠筛选与检索。
- 首方客户 API（[ADR-0139](./0139-first-party-customer-api-channel-is-a-product-owned-access-channel-family.md) / [ADR-0140](./0140-first-party-api-contract-is-generated-from-endpoint-descriptors-with-url-major-version-and-problem-details.md)，均为 Proposed）若被接受，其列表端点沿用本契约，不另起一套分页。

## 越权风险点

1. **缺省序 `-registeredAt`** 对按对象分组看版本的册（价卡、网络版本）未必顺手。本记录允许各册在封闭集里点名别的缺省序，但缺省是否该统一到登记时间，归各上下文 owner 复核。
2. **未知键即拒**比今天严：今天的读口对多给的键不校验（以 party-commercial 为例，隔离 Intake 不解析请求，`/commercial-policies` 只读 `kind`）。已知的调用方只有本仓管理台；若有脚本拼了多余参数，迁移后会开始收到 400。
3. **精确 `total`** 以「按租户读、租户级规模」为前提；哪一本册先超出这个前提，要回到本记录，把它改成给 `null`。
4. **页大小不可由调用方选**：界面不提供「每页 N 条」切换。若运营确需，得先在渠道契约层面允许，而不是在读口开 `limit` 参数。
5. **`q` 的包含匹配不走索引**；租户级规模下可接受，量级上来后要补三元组索引或改成前缀匹配，那时语义可能收窄。
6. **游标内带筛选摘要**比通行做法严：换条件必须从第一页重取。它防的是「翻到另一份列表的中段」，代价是客户端换条件时要丢掉游标——管理台本来就这么做。
