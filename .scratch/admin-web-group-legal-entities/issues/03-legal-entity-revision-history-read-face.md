# 03 责任法人修订历史读口 + 详情抽屉「修订历史」区

Category: enhancement
Status: resolved
Blocked by: 无（前端那半要等本票 Go 侧落地，同一张票内先后做）
地盘：`internal/partycommercial/ports/`（读口接口）、`internal/partycommercial/adapters/postgres/`（读适配器）、
`internal/partycommercial/adapters/http/`（端点）、`cmd/parcel-api/`（装配表一行 + 隔离读面 Intake 一格）、
`apps/admin-web/src/pages/party/`（抽屉历史区 + `api.ts` 一函数）

## 为什么

身份登记按修订版本化、只增不覆盖（party-commercial CONTEXT「身份生命周期」；`legal_entity_registration` 表存全部修订），
这是本模型对企业客户的卖点——可它在页面上看不见：`GET /commercial-group-legal-entities` 答的是每个法人的**最新**修订，
`r1` 显出来了但点不进去，操作者看不到 r1→r2 改了什么、依据从哪份换到哪份、停用是哪一笔。

## 要做的

1. **读口**：`ports` 加 `LegalEntityRevisionHistory`（或按本包既有读口命名规矩），输入法人标识 + 授权查询作用域（租户维，
   同 `CommercialCatalogueIntake` 的形状），输出该法人全部修订按修订号升序，每行含修订、依据、生效自、停用时点/依据、
   登记时间。**不做 diff**——两笔修订之间改了什么由前端并排显，读口只交事实。
2. **适配器**：`pcpostgres` 在 `NewOperationsCatalogue` 同一只上补方法（同表同作用域纪律，判据同 PS 的复核队列读口），
   或另立——按行形状归谁裁；真库用例走 `pgtest`。
3. **端点**：`GET /commercial-group-legal-entities/{legalEntityId}/revisions`，Intake 同 `commercialCatalogue`（隔离读面
   由 `buildIsolatedReadIntakes` 的 `commercialCatalogue` 一格顺带覆盖，不另加开关——入格按 ADR-0091 决定一的三条判据，
   ADR-0078 决定四那句「只此一维」已由它停用；`isolatedReadAdmittedPatterns` 加行处的注释逐条对那三条）。
   未配置答 403 `ACCESS_CHANNEL_NOT_CONFIGURED`；法人不在册答 200 + 空数组还是 404，按 ADR-0022 裁：**能力在、册在、
   只是没有这一个身份 → 200 空数组**，判据同 `writePartyRegistryAnswer` 对`未找到`的处置。
4. **装配**：`assembleBusinessEndpoints` 加一行；机制清点重生成随笔。
5. **前端**：详情抽屉「修订历史」区改为取真数据，纵向时间线（`Timeline` 原语），每笔显修订号、依据、生效自、登记时间；
   停用那笔标出。读口墙前照旧显未配置。

## 完成判据

- Go：`go build ./...`、`go vet ./...`、动过的包及反向依赖 `go test -count=1`（`cmd/parcel-api` 带 DSN）。
- 端点用例：未配置 403、在册法人多笔按序、不在册 200 空数组、跨租户不可见。
- 前端三道门禁绿；抽屉在演示形态下对 `SYN-LE-01` 显 1 笔。

## 完成记录（2026-09-16，分支 `mcp5-legal-entity-revisions`，基 `71c7d5a3`）

作者：`028c0d8a`（Go 半边：读口、pcpostgres 适配器、端点、装配表一行、隔离读放行面一行）为通道 5 所作；前端半边（抽屉「修订历史」区、`api.ts` 读口客户端、`legal-entity-revisions.ts` + test）为通道 5 所作、已 stage 未提交时会话断，由通道 1 按 parallel-sessions「未提交现场」原样封存成 `284c49d3`（`chore(salvage)`，一字不改）。本节由推送方按 `git diff 71c7d5a3 284c49d3` 代写，作者会话已无。

对完成判据：

- Go：`go build ./...` / `go vet ./...` 0；带 DSN `go test -p 1 -count=1 ./...` 在重放 tip `ef7086f0` 上 115 ok / 0 FAIL / 16 无测试 / 0 cached（18:38:45→18:40:44，119 s）；`-v` 探针 `TestLegalEntityRevisionHistoryListsAllRevisionsByRevisionNumber` / `TestLegalEntityRevisionHistoryAnswersUnknownAndForeignAsEmpty` PASS 非 SKIP。
- 端点用例四条各有：未配置 403 `…RefusesWhenAccessChannelUnconfigured`；多笔按序 http `…TranscribesRevisionsInOrder` + pg `…ListsAllRevisionsByRevisionNumber`（先落 r2 后 r1，钉按修订号不按落库时刻，另一法人不混入）；不在册 200 空数组 http `…AnswersUnknownEntityAsEmptyArray` + pg（非 nil 空切片）；跨租户 http `TestLegalEntityRevisionsEndpointDoesNotSeeForeignTenant` + pg。另 405 / 400 `MALFORMED_REQUEST` / 500 `NO_ANSWER_FORMED` 三门。
- 前端三道门：`tsc -b --noEmit` 0、`run-tests` 240/240、`vite build` 0（推送方于 `ef7086f0` 实测）。
- 演示形态浏览器验收（`SYN-LE-01` 显 1 笔）：**未验**——落地会话未起前端栈；`legal-entity-revisions.test.ts` 钉了时间线的每格与停用笔标记、`revisionHistoryNote` 的「没问到」与「没有历史」两句分开。

五条逐条：① `ports.LegalEntityRevisionHistoryRead`，输入租户（授权作用域）+ 法人标识，输出全部修订按修订号升序，行含修订、依据、生效自、停用时点 / 依据（`HasDeactivation` 守）、登记时间、`partyId`；不做 diff。② `OperationsCatalogue.ListLegalEntityRevisions` 同一只上补方法，`WHERE tenant_id=$1 AND legal_entity_id=$2 ORDER BY revision`，真库用例走 `pgtest`。③ `GET /commercial-group-legal-entities/{legalEntityId}/revisions`，Intake 同 `commercialCatalogueIntake`，法人不在册 200 + 空数组（ADR-0022），顶层回显 `legalEntityId`。④ `assembleBusinessEndpoints` 一行 + `isolatedReadAdmittedPatterns` 一行（经真路由钉 `PathValue`）；机制清点由推送方在重放 tip 重生成 `ef7086f0`。⑤ 抽屉「修订历史」区 `LegalEntityRevisionHistory` 接 `listLegalEntityRevisions`，`Timeline` 每笔显修订号 / 依据 / 生效自 / 登记于，停用笔 warning 标`已停用`，墙前显未配置，重试按钮同 `catalogueViewState` 口径，换行时旧答后到按 `legalEntityId` 核对丢弃。

**进 main 记录**：重放 `028c0d8a→53238188`、`284c49d3→9a297b2c`，清点 `ef7086f0`；`push ef7086f0:main` 于 18:42，远端 main = `ef7086f0`。评审 Standards ①② 的五处注释（三处 CONTEXT 旧节名「身份生命周期」、两处计数引用）由推送方在其后一笔纯注释修正（零行为，推送方自审）。

## Comments

**评审 ← 通道 2（非作者）· 钉 `284c49d3` · 基线 `71c7d5a3` · 18:27 / 18:29（汇总 18:31）。** Standards 阻断 0 / 非阻断 4；Spec 阻断 0 / 非阻断 2。全文在任务 `task-2ad8c9f0` 的 report_task。

- 无发现（Standards）：读口只交事实无 diff 无 status，`legalEntityRevisionTimeline` 只按本笔 `deactivatedAt` 标`已停用`；租户只从 `query.Scope.Tenant()` 来、路径只有 `legalEntityId`、SQL 双条件，pg 与 http 各钉跨租户；线上名 `LEGAL_ENTITY_REVISIONS_LISTED` Go / TS 字面一致，403 / 400 / 500 用例按字面断言；`LegalEntityID` 真回显；注释全中文无行号，「0015 的 paired 约束」指向 `legal_entity_registration_deactivation_paired` 存在；spec 红线五条均守。
- 已在进 main 后一笔修：Standards ①（三处引 CONTEXT「身份生命周期」——CONTEXT 无此节，实为 Lifecycles「参与方身份（业务参与方、责任法人、货主客户账户）」，分支基 `71c7d5a3` 沿旧名）；Standards ②（`PartyIdentityCatalogueReader` 注释「四个端点共用」、`LegalEntityRevisionHistoryRead` 注释「三个方法」——AGENTS 计数条，去数字）。Spec ②（票面第 3 条「ADR-0078 决定四只此一维」已被 ADR-0091 决定一停用——本笔改票面）。
- 推送方处置：Spec ①（清点未随笔）——按现行重放流程在重放 tip 重生成单独成笔 `ef7086f0`，不回作者。
- 留票面记、未动：Standards ③（`LegalEntityRevisionHistory` 自写四段文案与 `catalogueViewState` 同形——`StateSlot.override` 不交出 title/description，紧凑渲染本就要重写文案，kind→态的判别是第二份；判断题，可接受）；Standards ④（`useEffect` cancelled 取数与 `PublicationFormFields.useLoaded` 同形——`useLoaded` 无重试键、依赖变时不清回 null，直接复用需先扩；可接受）。
