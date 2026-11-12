# ADR-0378：业务参与方读口随行答「类别」——持有的角色与是否我方公司由读面按本上下文登记册上「有没有那样一行」导出，类别可作筛选维；目录读口的答复可按各册点名的维带分面计数

Status: Accepted（2026-11-12，Cursor 会话代 party-commercial owner 决，用户授权：草案 2026-11-10 由同一条会话线为票 [business-party-redesign/07](../../.scratch/business-party-redesign/issues/07-adr-party-categories-on-the-read.md) 起草；2026-11-12 会话把「这篇改读契约的 ADR 在不在此前的授权之内」单独问了用户一次，用户选「接受草案、不评审，直接做完 07→08→09」，同日并说「不需要评审和验收了，你直接测试通过就行了」。**没有经过非作者评审**：票 07 原定的三处评审重点各留在文末「越权风险点」，待 party-commercial owner 复核。扩充 [ADR-0144](./0144-catalogue-reads-share-one-cursor-pagination-sort-and-filter-contract.md) 决定四与决定五，延伸适用 [ADR-0372](./0372-a-catalogue-status-filter-is-put-out-only-for-a-status-read-directly-from-the-registers-own-rows.md) 决定一的判据一次，两篇正文不动。接受时定稿的几处见「接受时定稿」一节。起草能力边界：读过 ADR-0144 的 Context、候选与反方、Decision、Consequences 与越权风险点；ADR-0372 全文；[ADR-0367](./0367-a-business-party-may-carry-identification-and-the-customer-role-is-read-from-a-live-relationship-or-a-live-customer-account.md) 的标题、`Status` 行与决定四的标题句；party-commercial `CONTEXT.md` 里检索到的几句（责任法人与参与方资料词条、客户角色一句、被责任法人钉着的业务参与方诸句）；`internal/partycommercial/ports/catalogue_pages.go`；管理台 `business-party-list.ts`、`complete-register.ts` 与 `BusinessPartiesPage.tsx` 的取数与计数几段。接受时补读：`internal/platform/cataloguepage` 的非测试源文件，业务参与方、法人、参与方关系各册的读面与 HTTP 答复体，迁移 `0015_party_identity_registry.sql` 与 `0047_catalogue_read_pagination_indexes.sql`。仍没读：客户账户册读面全文、ADR-0077、ADR-0367 正文、`CONTEXT.md` 全文）<!-- 索引注记 -->｜**判据**：业务参与方的类别——持有的各个角色、是否我方公司、都不是即无角色——每一项都是本上下文登记册上「有没有那样一行」的一次存在性判断，由读面按装载时点导出、随行答出，并放成业务参与方册的筛选维；目录读口的答复可按各册点名的筛选维带分面计数，条件取本次的检索词与其余各维、不含该维自己
Date: 2026-11-12

## Context

**规则在哪。** party-commercial `CONTEXT.md` 写着：「一个业务参与方持有客户角色，当且仅当它持有一段在用（候选或已生效）的客户关系，或是一个未停用货主客户账户的所属客户参与方。」其余角色由参与方关系表达；「我方公司」在 `CONTEXT.md` 里指被责任法人钉着的业务参与方。ADR-0367 决定四把客户角色定为「由在用客户关系或在用客户账户判出」，它的索引注记同时记着：后端没有任何业务逻辑读客户关系。

**实现在哪。** 这条规则在本记录之前唯一的实现在管理台（取证于 `8d6f5ffba`）：`apps/admin-web/src/pages/party/business-party-list.ts` 的 `indexPartyRoles` 与 `ourCompanyPartyIds`，输入是合作关系、客户账户、公司主体各册不带条件的第一页。在 `internal/` 的 Go 源码里检索「客户角色」没有结果。

**多页时坏在哪。** 一页的大小由接入面定（ADR-0144 决定二）。往来单位一多：

- 关系或账户排在第一页之后的单位，被推成一个角色都没有；
- 按类型的数给不出来，类型只能在已取回的那一页上筛（ADR-0372 决定三的形态）；
- 小包网络的往来单位册很不对称——货主客户成百上千到上万家，供应商与承运代理几十家——在一页里筛「供应商」，多半答出一个假的「没有」。这正是 ADR-0144 的 Context 要消灭的情形，只是它当时说的是状态与检索，类别不在册上、没被带到。

**两份现行记录管着这件事。** ADR-0144 的答复形状只有 `page`，没有分面计数。ADR-0372 把状态筛选维收窄到「由本册登记行直接读出」的状态，并把「领域规则在多行之上求出的结论」「按读取时点现算的读数」「管理台的呈现词」排除在外；类别不是状态，字面上不归它管，但它的理由（规则不出第二处定义、翻页之间不能漂、词表要是领域的）对类别同样成立，所以本记录逐条对着它写。

## 候选与反方

**甲 · 留在页内筛，只把「没读到」标出来。** 即票 [business-party-redesign/01](../../.scratch/business-party-redesign/issues/01-type-reads-unknown-when-the-source-registers-are-incomplete.md) 的止血形态。反方：标明了仍然筛不出、数不出；规则仍只在浏览器里。它是过渡形态，不是去向。

**乙 · 管理台把这几册翻页读全再算。** 反方：页大小不由调用方定（ADR-0144 决定二），读全只能一页一页翻完这几本册，每开一次页面翻一遍；规则仍只在浏览器里，换一个调用方就要再写一份。否决。

**丙 · 写面落列**（ADR-0372 决定二·1 给第一类的出路）：关系、账户、法人各处写入时维护一张「参与方 × 类别」的投影，读口按列筛。反方：几条写路径共同维护一张投影，漏一条就错；更要紧的是客户账户与责任法人的「未停用」是生效与停用时点对时钟的推导（这几册的 `status` 同此），时间流过不是一次写入，投影恰好在这一处会陈旧——ADR-0372 对第二类说的「没有落列的时机」在这里原样成立。否决；复核若判类别属于第一类，这条要重估（越权风险点 1）。

**丁 · 读面按「有没有那样一行」导出（本记录）。** 代价：业务参与方的每一问多折叠这几本册；分面计数每点名一维多一次聚合；ADR-0144 的答复形状多一格。

## Decision

**一、类别的判定在领域一处。** 一个业务参与方在某一时点的**类别**是下面各项里成立的那些，一项都不成立时是「无角色」：

- 对每个参与方角色：它是一段在用（候选或已生效）的该角色关系的持有方；
- 客户角色另有一条：它是一个未停用货主客户账户的所属客户参与方（`CONTEXT.md` 原句，两条任一成立即可）；
- 我方公司：它被一个未停用的责任法人钉着。

判定写在 party-commercial 的领域包里，输入是这家单位作为持有方的各段关系的角色与登记的状态、名下客户账户与钉着它的责任法人的生命周期事实、判定时点。管理台自己的那一份删掉。

**二、读面是它的镜像。** 读面在 SQL 里对折叠到最新修订的这几本册，按参与方标识逐项探「有没有那样一行」，时点取装载时点——与同一行的 `status` 同一口径（那一格就是 `domain.IdentityLifecycle.StatusAt` 的 SQL 镜像）。领域判定与 SQL 用同一批夹具对照测试。答的是装载时点的类别；行上不带这几本册的事实，要别的时点的调用方自己去读它们。

**三、答复行多带两样。**

- `categories`：多值，封闭词表为参与方角色各值（以 `domain.PartyRole` 的词为准）加 `OUR_COMPANY`、`NO_ROLE` 两个词，永不为空，`NO_ROLE` 不与别的词同现，次序照词表。
- `relationshipCounts`：`asHolder` 与 `asCounterparty` 两格，各带 `live`（在用的段数）与 `candidate`（其中候选的段数）。只是行数，不带判断。

**四、`categories` 是业务参与方册的筛选维**（ADR-0144 决定四：维内为或、值按词表校验、进游标摘要）。它是多值维：一行的类别里有一个在所给的值里即匹配。它与 ADR-0372 的关系：ADR-0372 把界划在「有没有那一行」与「那几行合起来怎么判」之间。类别的每一项都是一次存在性判断——行与行之间不比较、不挑选、不汇总；被探的这几本册都在本上下文、由本上下文写入、以参与方标识指回本册；判断里用到的「在用」「未停用」读的是那一行登记的状态或它自己的生命周期时点。本记录据此判它落在 ADR-0372 决定一的同一侧，不属决定二排除的那几类。这是对 ADR-0372 判据的一次延伸适用（从「以本行为键的留痕册」到「本上下文内以本行标识为外键的登记册」）。

**五、目录读口的答复可以带分面计数。** 各册在声明里点名哪几个筛选维给分面；点名了的，答复在 `page` 之外多一格：

```json
{ "facets": { "categories": [{ "value": "CUSTOMER", "count": 1200 }], "status": [{ "value": "EFFECTIVE", "count": 1270 }] } }
```

每一维按词表次序答词表里每一个值的行数，为零的也答；条件取本次请求的 `q` 与其余各维，**不含该维自己**——点下某一格会得到几行，那一格上就是几。多值维一行可以记进多格。只有带封闭词表、且读口给精确 `total` 的维可以点名。没点名的册答复里没有这一格，不认识这一格的调用方不受影响。实现落在 ADR-0144 决定七的那个共用件里，不含任何上下文的词。

**六、本次点名的。** 业务参与方册：`categories` 与 `status` 给分面。参与方关系册：`status` 给分面；另加 `holderId`、`counterpartyId` 两个筛选维（开放标识，不设词表），供按一家单位读全它名下的关系。

**七、过渡。** 管理台改取读口之前照 ADR-0372 决定三在页内筛并标明（票 01）；票 [business-party-redesign/09](../../.scratch/business-party-redesign/issues/09-admin-web-takes-categories-from-the-read.md) 落地后那一套撤掉。

## 接受时定稿

草案留给接受时定的，与接受时改掉的：

- **词与字段名**（草案越权风险点 6）：两个新词取 `OUR_COMPANY`、`NO_ROLE`；段数取一格 `relationshipCounts`，下分 `asHolder`、`asCounterparty`，各带 `live`、`candidate`。与答复体既有的驼峰字段名、全大写词表同一种写法。
- **分面答词表里的每一个值**，为零的也答：调用方要分得出「这一格此刻为零」与「答复里没有这一维」。
- **不补索引**。草案的 Consequences 写「这几本册各要一条按租户与所指参与方标识的索引」，接受时改为不加：读面先把这几本册按租户整册折叠到最新修订再连接，条件穿不过折叠，按所指参与方的索引没有读者——迁移 `0047_catalogue_read_pagination_indexes.sql` 头注记的同一处境。按修订折叠的册该配什么索引，量级超出租户级前提时与那几条一并定。
- **没有做非作者评审**。草案原定先评审再接受，用户改定为不评审；本记录因此没有评审记录可引，三处评审重点原样留在越权风险点。

## Consequences

- party-commercial `CONTEXT.md` 在客户角色那一句之后补类别的定义句。
- ADR-0144：决定五的答复形状多一格 `facets`，决定四多一种「由本上下文其他登记册的存在性读出」的多值筛选维。ADR-0372：判据延伸适用一次。两篇各加前向指针，正文不动。
- 业务参与方册每一问多折叠这几本册，每点名一维分面多一次聚合。
- 管理台删掉 `indexPartyRoles`、`ourCompanyPartyIds` 与随它们的计数、页内类型筛选；类型列、类型筛选与各处的数改取读口（票 09）。
- 别的上下文的读口不受影响；谁要分面计数，在自己的声明里点名即可，不必再过一篇 ADR。

## 越权风险点

待 party-commercial owner 复核；前三条是票 07 原定的评审重点，没有人评过。

1. **类别落在 ADR-0372 的哪一侧。** 本记录判「有没有那样一行」。反方可以说：被探的不是留痕册而是另外几本独立的登记册，「在用」「未停用」又各带一条判据，这已经是「几行合起来怎么判」。若复核采此说，类别属于 ADR-0372 的第一类加第二类，筛选维放不出来，去向只剩候选丙（并且要先解决时钟那一处），或者长期停在候选甲。
2. **「我方公司」「无角色」进了词表。** 它们不是参与方角色。本记录把它们与角色并在一个多值维里，是因为运营是并着看的（一家我方公司也可以对别人担任角色），而且「无角色」只能表达成「前面各项都不成立」。若 owner 认为该拆成两维（持有的角色、是否我方公司），「无角色」就要用两维的组合表达，分面给不出它的数。
3. **分面进了共用契约。** 它是加法，但各册一旦点名，调用方就会依赖那几格数；对别的上下文的读口有没有连带，没有人对着它们的读面核过。点名从小放：只为页面确实摆出来的数点名。
4. **时钟。** 客户账户与责任法人的「未停用」随时间流过而变，同一组条件翻两页之间一行可能换类别。业务参与方、法人、客户账户各册的 `status` 筛选维本来就是这样；本记录没有让它变得更糟，也没有解决它。
5. **成本。** 每一问多几次折叠，每维分面多一次聚合，以「按租户读、租户级规模」为前提（同 ADR-0144 越权风险点 3），又没有索引可倚。哪个租户先超出这个前提，分面要能单独关掉而不影响筛选。
6. **起草与接受是同一条会话线。** 没读的那几份（客户账户册读面全文、ADR-0077、ADR-0367 正文）里若有与本记录相抵的地方，没有第二双眼睛看过。

## Links

- [ADR-0144](./0144-catalogue-reads-share-one-cursor-pagination-sort-and-filter-contract.md)：答复形状与筛选维在此扩充
- [ADR-0372](./0372-a-catalogue-status-filter-is-put-out-only-for-a-status-read-directly-from-the-registers-own-rows.md)：判据延伸适用
- [ADR-0367](./0367-a-business-party-may-carry-identification-and-the-customer-role-is-read-from-a-live-relationship-or-a-live-customer-account.md)：客户角色的判法
- [spec](../../.scratch/business-party-redesign/spec.md)、票 [07](../../.scratch/business-party-redesign/issues/07-adr-party-categories-on-the-read.md)、[08](../../.scratch/business-party-redesign/issues/08-read-answers-categories-filter-and-facets.md)、[09](../../.scratch/business-party-redesign/issues/09-admin-web-takes-categories-from-the-read.md)
