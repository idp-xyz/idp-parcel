# 12 业务参与方修订历史读口 + 抽屉「修订历史」区（按票 03 形态）

Category: enhancement
Status: in-progress
Blocked by: 09（抽屉归 09 建，前端半边落在它上面；Go 半边不依赖 09，同一张票内先后做——派票时若 09 未落，先做 Go 半边）
地盘：`internal/partycommercial/ports/`（`BusinessPartyRevisionHistoryRead`，紧邻 `LegalEntityRevisionHistoryRead`）、
`internal/partycommercial/adapters/postgres/`（`OperationsCatalogue.ListBusinessPartyRevisions`）、`internal/partycommercial/adapters/http/`
（`GET /commercial-business-parties/{partyId}/revisions`）、`cmd/parcel-api/`（装配表一行 + `isolatedReadAdmittedPatterns` 一行）、
`apps/admin-web/src/pages/party/`（抽屉历史区 + `api.ts` 一函数 + 时间线判读）。
出处：通道 1 评估「业务参与方」页（钉 main `dbe989cc`，2026-09-16 20:2x），spec「第二轮」表第四档。

## 为什么

与票 03 一字不差的理由换一册：`party_commercial.business_party_registration` 存全部修订，`GET /commercial-business-parties` 只答每个
参与方的最新一笔，`r2` 显出来了但点不进去——名称从哪份换到哪份、停用是哪一笔、依据换过几次，页面上看不见。03 已经给法人册开了这条路
（读口 / 适配器 / 端点 / 装配 / 隔离读放行面 / 前端时间线），本票把同一形状搬到参与方册。

## 要做的

1. **读口**：`ports` 加 `BusinessPartyRevisionHistoryRead`，输入授权查询作用域（租户维）+ 参与方标识，输出该参与方全部修订按修订号升序，
   每行含修订、名称、依据、生效时点、停用时点 / 依据、登记时间。不做 diff（03 第 1 条）。嵌进 `PartyIdentityCatalogueReader` 与 03 同位。
2. **适配器**：`OperationsCatalogue` 同一只上补 `ListBusinessPartyRevisions`，`WHERE tenant_id=$1 AND party_id=$2 ORDER BY revision`；
   真库用例走 `pgtest`：多笔按序（先落 r2 后 r1，钉按修订号不按落库时刻）、不在册非 nil 空切片、跨租户不可见。
3. **端点**：`GET /commercial-business-parties/{partyId}/revisions`，Intake 同 `commercialCatalogueIntake`；未配置 403；不在册 **200 + 空数组**
   （ADR-0022，同 03 裁）；顶层回显 `partyId`；线上名 `BUSINESS_PARTY_REVISIONS_LISTED`。405 / 400 / 500 三门同 03。
4. **装配**：`assembleBusinessEndpoints` 一行 + `isolatedReadAdmittedPatterns` 一行（注释逐条对 ADR-0091 决定一的三条判据，同 03）；
   机制清点随笔重生成（新文件必有差）。
5. **前端**：09 建的身份抽屉「修订历史」区改取真数据，`Timeline` 每笔显修订号、名称、依据、生效时点、登记时间，停用笔标出；墙前显未配置
   （「没问到」与「没有历史」两句分开，03 的 `revisionHistoryNote`）。时间线判读若与 `legal-entity-revisions.ts` 字段同形，**泛化一份共用**
   （名称是本册多出的一格，法人册没有），别抄第二份。

## 不做

- 不做修订 diff、不做回滚。
- 不动法人册的读口（03 已落）。
- 不动关系册（关系修订历史是另一册、另一票）。

## 完成判据

- Go：`go build ./...`、`go vet ./...`；动过的包及反向依赖 `go test -count=1`，`cmd/parcel-api` 带 DSN；`-v` 探针新 pg 用例 PASS 非 SKIP。
- 端点用例四条：未配置 403、在册多笔按序、不在册 200 空数组、跨租户不可见。
- 前端三道门绿；演示形态：`SYN-PARTY-RETIRED-01` 抽屉显 2 笔、第 2 笔标`已停用`。浏览器验收做不到就如实写「未验」。
- 清点随笔重生成，porcelain 空。

## Comments

### 完成记录（通道 4 · 2026-09-16 22:4x–23:0x · 分支 `mcp4-adminweb12` 基 main `f6010739`）

**(a)(b) 齐；(c) 抽屉接线待票 10 进 main。** 票 10 与本票共享 `BusinessPartiesPage.tsx`，派单裁本票在 10 进 main 之前不碰该文件；
Go 半边与 TS 纯逻辑不依赖 10，先落。10 进 main 后 `git fetch` + rebase，把抽屉「修订历史」区改取真数据另作一笔、再报一次。

**要做的逐条**

1. 读口 ✅ `ports.BusinessPartyRevisionHistoryRead` + `BusinessPartyRevisionRow`（`business_party_revision_history.go`），嵌进
   `PartyIdentityCatalogueRead` 与 `LegalEntityRevisionHistoryRead` 同位。行带 PartyName 不带 Status，理由在行类型注释。不做 diff。
2. 适配器 ✅ `OperationsCatalogue.ListBusinessPartyRevisions`，`WHERE tenant_id=$1 AND party_id=$2 ORDER BY revision`；pgtest 用例
   `TestBusinessPartyRevisionHistoryListsAllRevisionsByRevisionNumber`（先落 r2 后 r1，钉序按修订号；名称随每一笔；停用两件只在停用笔；
   同租户另一参与方不混入）与 `TestBusinessPartyRevisionHistoryAnswersUnknownAndForeignAsEmpty`（不在册非 nil 空切片；跨租户零行）。
3. 端点 ✅ `NewQueryBusinessPartyRevisionsEndpoint`，`GET /commercial-business-parties/{partyId}/revisions`，Intake 同
   `CommercialCatalogueIntake`；未配置 403、不在册 200 + []、顶层回显 partyId、线上名 `BUSINESS_PARTY_REVISIONS_LISTED`；405 / 400 / 500
   三门同 03。`PartyIdentityCatalogueReader` 嵌入 `BusinessPartyRevisionHistoryReader`。
4. 装配 ✅ `assembleBusinessEndpoints` 一行；`businessEndpointProbes` 一格；`isolatedReadAdmittedPatterns` 一格（注释逐条对 ADR-0091 决定一
   三条判据，经真路由期待 500 而非 400，钉住 chi 填 `{partyId}`）；`unwiredCommercialCatalogue` 占位方法。清点随笔重生成（f435e8d9）。
5. 前端 ◑ TS 纯逻辑已落：`api.ts` 加 `BusinessPartyRevisionRecord` / `BusinessPartyRevisionListResponseBody` / `listBusinessPartyRevisions`；
   时间线判读**泛化一份共用**——新 `revision-timeline.ts`（`identityRevisionTimeline(revisions, format, content)` /
   `identityRevisionHistoryNote(count, subject)`），`legal-entity-revisions.ts` 改成薄适配且导出名不动，新 `business-party-revisions.ts`
   薄适配 + node:test 三条。**抽屉接线未做**（等 10）。

**完成判据逐条**

- Go ✅（取证于 `f435e8d9`）`gofmt -l .` / `go build ./...` / `go vet ./...` 零输出。`go list` 反查 ports / pcpostgres / commercialhttp 的
  全部反向依赖包（`cmd/parcel-api`、`parcel-commercial`、`parcel-dispatch`、PS/PC postgres 等）+ `./internal/architecture/...` 带 DSN
  `go test -count=1 -p 1`：全 ok，`--- PASS` 1608 / SKIP 0 / FAIL 0；`-v` 探针新 pg 用例两条 PASS 非 SKIP。
- 端点用例四条 ✅ 未配置 403 / 在册多笔按序（含名称换了一次）/ 不在册 200 空数组 / 跨租户不可见，另加 405+400+500 三门一条。
- 前端三道门 ✅ `tsc -b --noEmit` 0 / `run-tests` 257（main 254 + 3）/ `vite build` 0。演示形态（`SYN-PARTY-RETIRED-01` 抽屉显 2 笔、第 2 笔标
  `已停用`）**未验**——抽屉接线待 10，浏览器验收随 (c) 那一笔。
- 清点 ✅ 随笔重生成（partycommercial 生产 +3 / 测试 +2、端点 +1、端口 +1），porcelain 空。

**判断项**

- 泛化取舍：两册修订行在修订号 / 依据 / 生效 / 停用两件 / 登记时间上同形（同一个 `IdentityLifecycle`），各册多出的一格（法人册 partyId、
  参与方册 partyName）是这一笔的**内容**，以 `content` 回调交进共用判读，放在描述里同一位置（「依据 · 生效自 · <内容> · 停用于」），两张抽屉
  版式一致。不改 03 已验收的描述次序。
- `legal-entity-revisions.ts` 保住导出名（`legalEntityRevisionTimeline` / `revisionHistoryNote`）：`GroupLegalEntitiesPage` 与既有用例零改动
  仍绿，是泛化保住行为的证据；改名属评审期重构，不在本票。
- 注释句主语随册走（`identityRevisionHistoryNote(count, subject)`）：判读能共用，句子里点名的册不能共用；node:test 钉参与方那句不含「法人」。
- 描述位置：名称放在「生效自」之后而不是最前——与法人册版式对齐优先于「名称是主内容」的直觉；03 已验收的版式不动。

**笔** `ee8481b0`（票面 in-progress）→ `d0cb97d0`（ports + pcpostgres）→ `fa459894`（端点 + 嵌入）→ `90a8ee57`（装配）→ `67e24804`（TS）→
`f435e8d9`（清点）。每片先红后绿：pg 方法缺失编译红、端点构造函数缺失编译红、装配两方向红（「没进装配」/ 404 want 500）。
