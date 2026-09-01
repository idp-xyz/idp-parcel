# 01 参与方身份生命周期：集团法人与货主客户账户的登记机制

Category: feature
Status: resolved——补格裁定已实施（MCP-5，2026-09-01）：参与方身份本体册的读法、真库读
适配器、第三端点与页面身份签均已落地，身份生命周期三格各有实例可显，完成判据全达

## 问题

管理台「集团与法人」「业务参与方」两页无表无写入方。`CommercialObjectKind` 注释明写货主客户
账户与责任法人刻意不在商业对象之中——它们是参与方身份、走自己的生命周期；而那个生命周期
今天没有代码。`internal/partycommercial/domain/party_relationship.go` 有参与方关系模型但无持久化。

## 做什么

1. 领域模型：参与方身份生命周期（登记→生效→停用），三层结构照 ADR-0003（运营集团租户—
   责任法人—货主客户账户），参与方关系沿既有 `party_relationship` 模型，不另起第二套。
2. 登记册表 + 写入方：版本化不可覆盖（ADR-0031 登记面代数先例），租户维打头。
3. 受控 CLI 登记入口：照治理/VE 登记先例——登记是操作者动作，不占装配号。
4. 读面：租户维目录读，`SYN-` 隔离读准入（ADR-0078）。
5. 页面：交付「集团与法人」「业务参与方」两页组件与数据装配；注册条目报 05 接入，
   不自改 `page-registry.tsx`。

## 边界

- 不建风控与合规审查；不与商业对象生命周期混（各走各的生命周期）。
- 真实法人、真实客户一律不进；演示走 `SYN-` 种子。
- 身份生命周期形状若出难逆转取舍（停用语义、跨租户唯一性、与租户本体的边界）→ 走 ADR。

## 完成判据

登记 CLI 灌 `SYN-` 种子后两页非空册；停用后目录如实显示状态；全仓绿（报绿注明含不含真库）。

## 核验发现与补格裁定（2026-08-28，MCP-4 取证 / MCP-1 裁）

MCP-4 实证：停用登记本身如实（RETIRED-01 rev2 携停用两件，CLI 重放答 DEACTIVATED），但
**目录读面无处显示它**——法人读口只上列 `legal_entity_registration` 最新修订，关系读口只上列
关系；RETIRED-01 既非法人也不在任何关系里，DEACTIVATED 格在管理台无实例可见，REGISTERED
格同病（FUTURE-01 未来生效同样不上列）。seed.sh 自注「让法人页与参与方页的身份状态三格都有
真实例可显」与实测不符。

裁定**补参与方身份本体上列面**，不改种子绕行：`business-parties` 页的所有权语言明说承载
「角色中立的业务参与方身份」与「参与方关系」两件（`navigation.ts` 的 moduleInfo），身份本体
不可见等于本票标题那个生命周期不可观察；拿种子把停用对象换成法人能让一格有实例，但那是用
数据绕读面缺口——接错看着像接对的形状。范围：`ports` 加身份本体列表读法、真库读适配器、
http 第三端点、`BusinessPartiesPage` 加身份册区；装配行与页登照旧占号 MCP-1。实施派 MCP-5。

## 补格实施（2026-09-01，MCP-5）

裁定范围逐件落地，未改种子——绕行那条路按裁定不走。

- `ports.BusinessPartyRow` + `PartyIdentityCatalogueRead.ListBusinessParties`：与法人行、
  关系行并列的第三种行形状。它没有 `HasPartyName` 那一格——名称就在本册行上，法人与关系
  两册才需要左连接过来，那两处的「查无此人」是写入门失败的悬空引用，本册没有那一格可缺。
- `adapters/postgres` 的 `ListBusinessParties`：`DISTINCT ON (party_id)` 取最新修订，status
  的 `CASE` 与法人册逐字相同（停用判断在先，其次生效时点）——两册用的是同一个
  `domain.IdentityLifecycle`，判据不该有第二种写法。
- `adapters/http` 的 `NewQueryBusinessPartiesEndpoint`（`GET /commercial-business-parties`）：
  与关系那一口分立而不是折进去，理由就是裁定那一句——折进去，不在任何关系里的参与方永远
  不上列，而被停用的那种恰恰如此。
- `cmd/parcel-api`：端点表加一行（复用既有的 `partyIdentities` 读口参数与
  `commercialCatalogueIntake`，`main.go` 因此不用改）、`businessEndpointProbes` 与
  `isolatedReadAdmittedPatterns` 各加一行、`unwiredCommercialCatalogue` 补占位方法。
- `BusinessPartiesPage` 改为两签（参与方身份 / 参与方关系），照
  `CustomsPortsPathsPage` 的分签先例。**分签而不是并表**：身份状态按时点导出、关系状态是
  登记进来的事实，两套代数并进一张表，同一个「已生效」会在两种含义间相互冒充。

**测试钉的是裁定本身**：`TestBusinessPartyCatalogueShowsAllThreeLifecycleCells` 对真库造三笔
——未来生效（`REGISTERED`）、已过生效时点（`EFFECTIVE`）、带停用两件（`DEACTIVATED`）——
断言三格各有实例可显，外加跨租户零行与 limit 非正拒；传输层另有两条证行体逐字段转写与
「空册是答案、读不回才是 5xx」。

**验证**：全仓 `gofmt -l` 无输出、`go build`、`go vet` 绿；`go test -count=1 ./...` 绿且
**含真库**；`apps/admin-web` 的 `tsc --noEmit` 无输出、`pnpm build` 绿。
