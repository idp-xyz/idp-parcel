# 商业策略页缺第六个页签：授权规则（含取消授权目录）

Category: enhancement
Status: ready-for-agent

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
