# 商业策略页缺第六个页签：授权规则（含取消授权目录）

Category: enhancement
Status: resolved

自[票 04 的导航裁决](./04-registered-but-unreadable-rows-need-a-nav-ruling.md)。本票的起因不是「取消授权目录没页可去」，是**它的拥有对象没页可去**。

## 事实（实读代码与演示库，锚 `dc611a3`）

1. 种子发布批里有**三条** `AUTHORIZATION_RULE`（`SYN-AUTH-PRICE-DIR-01`、`SYN-AUTH-COST-DIR-01`、`SYN-AUTH-CANCEL-01`），全部 `PUBLISHED_EFFECTIVE`，库里有行。
2. `/commercial-policies` 的 `kind` 封闭集只有五格：`ACCEPTANCE_RULE_PACKAGE`、`PRE_ACCEPTANCE_CONTROL`、`PRICE_POLICY`、`SETTLEMENT_POLICY`、`AS_OF_POLICY`。**授权规则这一类商业对象在管理台上没有任何落脚点**——不是少一个页签那么简单，是整类对象看不见。
3. 取消授权目录 = `cancellation_authority_content`（壳）+ `cancellation_authority_declaration`（按请求方封闭二值 `CUSTOMER` / `OPERATIONS` 进主键，各带一个 `rule_reference`）。0013 抬头写死它挂**授权规则**（`object_kind=9`），不是接单规则包。
4. 计价那两条授权规则被价卡的方向授权引着（`SYN-AUTH-PRICE-DIR-01` / `SYN-AUTH-COST-DIR-01`），今天只能从价卡那边间接看到它们的标识，看不到授权规则本身。

## 要做什么

- `ports.CommercialPolicyCatalogueRead` 按封闭集**扩一个方法**（该接口的文件注释已经写明「正文表落库时按封闭集扩方法」，本票正是那种情况），不开按 `object_kind` 传参的通用口。
- `adapters/postgres` 加 `ListAuthorizationRules`：从 `commercial_version` 取 `object_kind=9` 的版本壳，左连接取消授权目录父子两表——**一条语句**，父子不分两次取。
- `adapters/http`：`kind` 封闭集加第六格，响应体判别子加一支。**路径不变**——这一格加的是页签，不是页（判据同票 01：那个 `kind` 分的是这一页里的页签）。
- `apps/admin-web` 的 `CommercialPoliciesPage` 加页签 + `presentation.ts` 加词表。
- **不碰任何共享接线文件**：端点已在放行面里、页面已 live。

## 两条形状约束

1. **上列对象是授权规则版本壳，取消授权目录是它的一族正文。** 三条授权规则里只有 `SYN-AUTH-CANCEL-01` 有取消授权目录，另两条没有——那不是缺数据，是那两条授权规则本来就不声明取消授权。壳在不在要显式布尔，判据同票 01 的 `contentRegistered`。
2. **请求方封闭二值**（`CUSTOMER` / `OPERATIONS`）各自带自己的规则引用，两方不能合成一栏——「客户可取消」与「运营可取消」是两条独立授权。

## 与票 07 的关系

两票都碰 `internal/partycommercial/**` 与 `apps/admin-web/src/pages/party/**`，**串行**。次序无所谓，但同一时刻只能有一张在做。

## 完成标准

- `/commercial-policies?kind=AUTHORIZATION_RULE` 答 `200` + 三条；`SYN-AUTH-CANCEL-01` 那条带取消授权目录，另两条如实答「未声明」。
- 真库测试钉住：只取 `object_kind=9` 不串别类、壳缺席与零声明可分辨、租户隔离、`limit` 非正即拒。
- 全仓 `go test -count=1 ./...` 绿（含真库）；页面层 DOM 取证照票 01 脚本。

## 收口（2026-08-26 · MCP-2）

`2131d05` 后端（端口第六个方法、真库装载口、传输层第六格、四条真库用例），`04de101` 页面层，`2230745` 之后的取证工具另记。

**一处与票面不符，如实记下。** 票面「要做什么」末条写「不碰任何共享接线文件」——不成立。扩 `CommercialPolicyCatalogueRead` 会让 `cmd/parcel-api/unwired_orchestration.go` 的 `unwiredCommercialCatalogue` 编译不过，必须补一个方法。改动是在该类型方法表末尾追加一段，不动邻行，但它确实在 `cmd/parcel-api` 下。**扩读口就一定会碰到这个文件**，下一张同形的票照此预期，别再写「一个都不碰」。

**三态的中间那格票面只说了一半。** 票面形状约束 1 把三态写成「壳在不在」，而领域（`CancellationAuthorityContent` 的注释）比这多一层：壳在而某请求方**没有行**，是这份目录说出的真话——该请求方不许取消——不是配置缺件；壳在而零行才是缺件。因此页面按请求方逐格作答（未声明 / 允许:引用 / 不许取消），而不是把目录里有的几行列出来完事：后者会把「不许取消」显示成空白，读的人会去补一份已经写好的目录，而那份目录正是拒绝的依据。

**取证**

1. 端点：`GET /commercial-policies?kind=AUTHORIZATION_RULE`（隔离读实例 `:19081`）答 `200` 三条。`SYN-AUTH-CANCEL-01` 带 `cancellationAuthorityDeclared:true` 与两方声明，`SYN-AUTH-PRICE-DIR-01`、`SYN-AUTH-COST-DIR-01` 皆 `false` + 空数组。
2. 真库四条：三态可分辨、不串别类对象（三个对象共用同一标识与版本号，只有 `object_kind=9` 那份在列）、租户隔离、`limit`。
3. `go test -count=1 ./...` 全绿。首轮曾红在 `customscompliance` 的一条真库用例上，报 `Only one usage of each socket address`——Windows 临时端口耗尽，与本笔无关；单跑该包与再跑全仓均绿。
4. 页面层：授权规则页签十项该在的全 HIT，四个未配置码全 miss；顺带回核默认页签与另两页仍绿。**「不许取消」这一态 DOM 里核不到**——种子那份目录两方齐全，库里没有「已声明而某方缺行」的行，该态由真库用例 `TestAuthorizationRuleCatalogueSeparatesUndeclaredFromPartyAbsent` 钉住。

**测试起初没网住租户**

`TestAuthorizationRuleCatalogueIsTenantIsolated` 最初只断言 `HasCancellationAuthority` 为假。把装载口 SQL 里 `declaration.tenant_id = version.tenant_id` 删掉实测，四条用例**全绿**——壳的在场走 `LEFT JOIN`（那处租户条件还在），请求方声明走相关子查询（那处被删了），于是邻租户的声明照样跟过来而布尔仍是假。补上对数组的断言后同一处改动即红。两处各带自己的租户条件，断言就必须两条都有。
