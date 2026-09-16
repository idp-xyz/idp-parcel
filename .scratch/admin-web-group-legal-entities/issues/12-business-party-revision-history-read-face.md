# 12 业务参与方修订历史读口 + 抽屉「修订历史」区（按票 03 形态）

Category: enhancement
Status: ready-for-agent
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
