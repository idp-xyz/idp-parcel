# 08 读口随行答类别与关系段数、按类别筛、分面计数

Category: enhancement
Status: resolved——2026-11-12 进 main（没有做评审，用户当日裁定）
父票：[spec](../spec.md)
Blocked by: 07（已解除：[ADR-0378](../../../docs/adr/0378-business-party-reads-answer-categories-read-from-the-existence-of-rows-and-catalogue-reads-may-carry-facet-counts.md) 2026-11-12 接受）

范围以接受后的 ADR 为准；下面按草案写，接受时有出入的以 ADR 为准并回改本票。后端票，走 `docs/agents/workflow.md`「进 main 的步骤」。

## 要做的

- **类别的判定落在领域一处。** party-commercial 领域包里写出「一个业务参与方此刻属于哪几类」的判定，输入是它名下各关系的角色与状态、名下客户账户的生命周期、钉着它的责任法人的生命周期与判定时点。读面的 SQL 是它的镜像，对照测试用同一批夹具钉住两边——做法同 `status` 对 `domain.IdentityLifecycle.StatusAt`。
- **`/commercial-business-parties` 的每一行**多答：`categories`（多值，封闭词表，不为空）与关系段数（作为持有方、作为相对方各两格：在用的、其中待确认的）。行上的事实是装载时点的，与同一行的 `status` 同一口径。
- **`categories` 成为筛选维**（维内为或），进游标摘要；集外的值照 ADR-0144 拒。
- **分面计数**：共用件（`internal/platform/cataloguepage`）支持各册在声明里点名哪几维给分面；答复多一格 `facets`。条件取本次的 `q` 与其余各维，不含该维自己；多值维一行可以记进多格。本票点名：业务参与方册的 `categories` 与 `status`，参与方关系册的 `status`。
- **参与方关系册加两维按参与方筛**：`holderId`、`counterpartyId`（参数名取答复体的字段名）。依据是按一家单位看它名下的关系：右栏「关联」签今天从合作关系册不带条件的那一页里挑（`relationshipsOfParty`），多页时挑不全；「在『合作关系』里看全部」那一跳用的是 `q` 的包含匹配，会把标识互为前缀的别家也带出来。
- **索引**：被探的各册各补一条按租户与所指参与方标识的索引（合作关系的持有方与相对方、客户账户的所属客户、责任法人钉的参与方），有没有现成的先查。
- **种子**：演示租户的量级不变；另备一份只在测试里用的多页夹具（合作关系多于一页）。

## 完成判据

- 对照测试：领域判定与读面 SQL 对同一批夹具答出同样的类别。夹具至少覆盖——只凭在用客户关系是客户；只凭未停用客户账户是客户；两者都有；只剩已到期或已撤销的关系不是；候选关系算；被未停用的责任法人钉着是我方公司，被已停用的钉着不是；我方公司同时持有角色；一类都不属于。
- 真库测试：按类别筛出的行与总数一致；游标翻过多页不重不漏；`categories` 与别的维、与 `q` 组合；分面各格之和与逐格去筛得到的总数逐一对上；拿旧条件的游标换了类别再翻，照 ADR-0144 拒。
- 没点名分面的册答复里没有 `facets`，既有读口的测试一条不改就过。
- `scripts/test-full.sh` 退 0；`scripts/demo-seeds/seed.sh` 灌一次性库退 0。
- 管理台本票不动：多答的格与 `facets` 是加法，现有页面不读它们也照常工作（票 09 才接）。

## 完成记录（2026-11-12，直接推 `main`，代码在 `96569538b`，随后一笔 `c7d999af1` 是重生成的机制清点）

范围照 [ADR-0378](../../../docs/adr/0378-business-party-reads-answer-categories-read-from-the-existence-of-rows-and-catalogue-reads-may-carry-facet-counts.md)。没有做非作者评审，也没有走隔离评审那一步：用户 2026-11-12 裁定这一串票不评审、作者自测通过即推。

- 落点：
  - 领域 `internal/partycommercial/domain/party_category.go`：`PartyCategoriesAt`、`PartyCategoryWords`、`RoleCategory` 与两个词 `OUR_COMPANY`、`NO_ROLE`；`RelationshipStatus.Live`。
  - 共用件 `internal/platform/cataloguepage`：`FilterDimension.Facet`、`Columns.MultiValued`、`Binding.FacetPlans` 与 `FacetPlan`、`Result.Facets`。条件的拼装抽成一处，取页与分面共用。
  - 声明 `ports/catalogue_pages.go`：业务参与方册加 `categories` 维，`status` 与 `categories` 给分面；参与方关系册 `status` 给分面，加 `holderId`、`counterpartyId` 两维。行 `ports.BusinessPartyRow` 多 `Categories` 与 `RelationshipCounts`。
  - 读面 `adapters/postgres/party_identity_catalogue.go`（业务参与方的前半句）与 `catalogue_pages.go`（取页之后数分面）；答复体 `adapters/http/query_party_identities.go`。
- 自己拿的主意：
  - **先聚后连，不逐行去探**。ADR 说读面「按参与方标识逐项探有没有那样一行」。写成 SQL 时是把关系、客户账户、责任法人三册各折叠到最新修订，按所指的参与方聚一遍，再左连接到业务参与方上——逐行探要对每个参与方扫一遍折叠出来的册。判的仍是同一件事。
  - **领域判定收现成的 `PartyRelationship`**，没有另立一个只装角色与状态两格的类型：架构的类型可达性棘轮不认只有测试用得到的导出类型。
  - **生产接线棘轮基线加了一行 `PartyCategoriesAt`**，理由写在那一行上面：生产里答类别的是读面的镜像语句，领域判定此刻只给对照测试当基准，等第一个要在 Go 里判类别的调用方出现再接上。没有为了出名单给它造一个生产调用点。
  - **给分面的维不许带代码换算、词表里不许有重复值、没有词表的维不许点名**，装配时就报。
  - **分面每维一句查询**，没有并成一句：现在点名的最多两维。
  - **不补索引**（ADR「接受时定稿」一节）。票面「索引」一条因此没有做。
  - **种子没有动**。票面要「另备一份只在测试里用的多页夹具」：真库测试自己灌了十二家单位、八段关系，用页大小 2 翻页；没有另做一份能灌进本地 API 的多页种子（接入面的页大小是 200，要两百多家才翻得了页）。票 09 的多页核对因此靠这里的真库测试，见票 09。
- ✅ `scripts/test-full.sh` 全量退 0（含真库；175 个包 ok），跑的是变基之前的同一份代码；变基只垫进了别的会话的文档提交，变基后重跑了架构、partycommercial、cataloguepage 三处（退 0）、`gofmt`、`go build`、`adr-index -check`，并重生成机制清点核对无差异。
- ✅ 完成判据逐条：
  - **对照测试**：`TestBusinessPartyCategoriesMirrorTheDomainJudgment`——同一批登记既写进库让读面答，又原样交给领域判定判，十二家逐家相同。票面点名的情形都在：只凭在用客户关系、只凭未停用客户账户、两者都有、只剩已撤销的关系、候选关系、被未停用 / 已停用的责任法人钉着、我方公司同时持有角色、一类都不属于；另有生效时点未到的账户、已停用的账户、只作相对方不得角色。到期与替代两种终局只在领域单测里（真库夹具里的终局是撤销）。
  - **真库测试**：按类别筛出的行与总数一致、与状态和 `q` 组合（`TestBusinessPartyCatalogueFiltersByCategory`）；页大小 2 翻三页不重不漏、旧条件的游标换了类别即拒（`TestBusinessPartyCategoryPagesTurnWithoutRepeatsOrGaps`）；分面逐格与去筛得到的总数对上，四组条件各核一遍（`TestBusinessPartyFacetsAgreeWithFilteringEachValue`）；关系册按双方标识筛与状态分面（`TestPartyRelationshipCatalogueFiltersByEitherPartyAndCountsStatuses`）。
  - **没点名分面的册答复里没有 `facets`**：共用件单测（`TestACatalogueWithoutFacetsPlansNone`）与 HTTP 测试；既有读口的测试没有改一条。
  - **`scripts/demo-seeds/seed.sh` 没有跑**：这张票没有动迁移与种子。
  - **管理台没有动**。
- ✅ 演示租户上看了一眼（预览 API 换成新二进制重起，库没有动）：往来单位 24 家，分面是客户 9、供应商 2、承运商代理 3、转售 1、渠道账号持有 1、授权交付方 1、我方公司 5、无角色 3，状态是待生效 1、已生效 22、已停用 1；合作关系 26 段，候选 3、已生效 23。客户、供应商、承运商代理三格与管理台此前自己推出来的数相同。
- 未核：
  - 性能。没有量过大册上多出来的三次折叠与两次分面聚合要多久；ADR 越权风险点里记着。
  - `main` 上的 CI：这台机器上查不到。
