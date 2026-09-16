# 12 业务参与方修订历史读口 + 抽屉「修订历史」区（按票 03 形态）

Category: enhancement
Status: resolved
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
4. **装配**：`assembleBusinessEndpoints` 一行 + `isolatedReadAdmittedPatterns` 一行（注释逐条对 ADR-0078 Decision 二 / 表内既有三条口径，同 03）；
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
4. 装配 ✅ `assembleBusinessEndpoints` 一行；`businessEndpointProbes` 一格；`isolatedReadAdmittedPatterns` 一格（注释逐条对 ADR-0078
   Decision 二 / 表内既有三条口径——消费本上下文自己的存储读面、零持久化、作用域来自运营侧授权结果，经真路由期待 500 而非 400，
   钉住 chi 填 `{partyId}`；归因原写 ADR-0091 决定一，评审 S1 指出那三条是隔离准入的另一组判据，此处按评审改）；`unwiredCommercialCatalogue` 占位方法。清点随笔重生成（f435e8d9）。
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

### 评审 ← 通道 2 · (a)(b) · 钉 `0f4ff6c8` · 基线 `f6010739`（= merge-base）· 23:09

（`task-244a5040`；隔离树 `%TEMP%\idp-review-12` 干净、七笔 19 件 +942/−62 与派单一致，评完已拆；参照 03 Go 笔 `53238188` 形状一致；只读、未跑全仓、未占 55432。实跑无 DSN：端点五用例 PASS、`go vet` 两包 0、真红核过——handler 恒答空数组时仅 `TranscribesRevisionsInOrder` FAIL；`cmd/parcel-api` 无 DSN 六条探针 PASS。pg 两条、反向依赖全量、admin-web 三道门以作者自报与通道 4 广播为准。）

- **Standards（阻断 0 / 非阻断 0）**：① 与 03 同形核实——405(+Allow) → Intake → 路径参数在 Intake 之后取、空答 400 → 读失败 500 → 200 顶层回显 `partyId` + outcome + revisions；不在册 200 + `[]` 非 null；`isolatedReadAdmittedPatterns` 新行注释与 03 行及表内其余行同一口径。② SQL 两维 + `ORDER BY revision ASC` 无 LIMIT；`make([]…, 0)` 非 nil；pgtest 先落 r2 后落 r1 钉 `rows[0].Revision==1`，同租户另一参与方不混入，跨租户零行。③ 行带 PartyName 不带 Status 的理由在行类型注释（Status 是装载时点导出，给旧笔算「此刻状态」会混历史事实与此刻判断），http body / `api.ts` 同句回指 ports。④ TS 泛化：`identityRevisionTimeline<Row extends IdentityRevisionRecord>(revisions, format, content)` / `identityRevisionHistoryNote(count, subject)`；`legal-entity-revisions.ts` 薄适配导出名不动，`legal-entity-revisions.test.ts` 零 diff；回调交「各册多出的一格」优于两份副本——共用体是整段描述拼接与停用 / 色调 / 时刻格式化，变异点只一串内容与一个主语。⑤ 注释全中文、引符号名、无行号；「三条裁决 / 三条取舍 / 三条同裁决」皆同句内枚举非跨文件计数，「三条判据」为表内既有口径，不计为发现。⑥ `unwiredCommercialCatalogue.ListBusinessPartyRevisions` 交 `nil, errOrchestrationNotWired`，与法人册占位同形，非静默替身。Smell 基线（不可行动判断）：Go 三件与 03 结构重复——票面裁「照 03 形状搬一册」，第三册开修订历史时再考虑抽泛型读端点。
- **Spec（阻断 0 / 非阻断 1）**：**S1（票面措辞）** 要做的 4 与完成记录写「注释逐条对 ADR-0091 决定一的三条判据」——核 ADR-0091 原文，决定一的三条是「注入值全部带 SYN- 前缀 / 不采信自报身份 / 生产装配无通往它的代码路径」；代码注释列的「消费本上下文自己的存储读面 / 零持久化 / 作用域来自运营侧授权结果」是 `isolatedReadAdmittedPatterns` 表自表头（Covers ADR-0078 Decision 一、二）以来运营查阅行的放行论证。代码注释未引 ADR-0091、与表内惯例一致，不需改；票面归因在 (c) 那笔顺手改为「ADR-0078 Decision 二 / 表内既有三条判据」。无发现：要做的 1–4 ✓；5 ◑ 如票面（`api.ts` 三件 + `revision-timeline.ts` + `business-party-revisions.ts` + test 三条；抽屉接线待 10，`BusinessPartiesPage.tsx` 不在 diffstat）；不做三条 ✓（法人册读口三件不在 diffstat，`query_party_identities.go` / `ports.go` 只加嵌入一行 + 注释；关系册未动）；完成判据端点四条 + 三门一条逐条对得上；清点笔数字（生产 127→130 / 测试 141→143 / 端点 25→26 / 端口 412→413）与新增文件对得上。
- **一行**：Standards 0 / 0（一项不可行动判断）；Spec 0 / 1（票面归因）。无阻断，可推。

### 进 main 记录 · (a)(b)（推送方 = 通道 1 · 23:14）

隔离树 `%TEMP%\idp-land12` detached 于 `44b6f41b`（= 当时远端 main），cherry-pick 七笔零冲突 `ee8481b0→5d9fdcdb` / `d0cb97d0→98fea43b` / `fa459894→cddd793a` / `90a8ee57→5694f07b` / `67e24804→00b08df5` / `f435e8d9→9cb67009` / `0f4ff6c8→44c4ebe2`（patch-id 逐笔同；19 件对作者 tip `0f4ff6c8` 零 diff）；tip `44c4ebe2` 上 `gofmt -l` 空、`go build ./...` / `go vet ./...` 0、清点在 tip 重生成 porcelain 空（作者随笔那份在新基上仍准）；admin-web 全新 `pnpm install --frozen-lockfile` 后 `tsc -b --noEmit` 0 / `run-tests` **257 pass** / `vite build` 0；带 DSN `go test -p 1 -count=1 ./...` **115 ok / 0 FAIL / 16 无测试 / 0 cached**，退出码 0（23:05:05→23:07:23）；`-v` 探针 `TestBusinessPartyRevisionHistoryListsAllRevisionsByRevisionNumber` / `…AnswersUnknownAndForeignAsEmpty` PASS、`cmd/parcel-api` `TestIsolatedLegalEntityRegistrationLandsAgainstARealDatabase` PASS 非 SKIP → 评审无阻断（上）→ `ls-remote` 核 `44b6f41b` 未动 → 23:13 `push 44c4ebe2:main` 成，**远端 main = `44c4ebe2`**；共享树 ff 同 SHA。Status 仍 in-progress——(c) 抽屉接线等 10 进 main 后由作者另作一笔、另评、另进 main；评审 S1 的票面归因由 (c) 那笔顺手改。

### 完成记录 · (c) 抽屉接线（通道 4 · 2026-09-16 23:2x · 分支 `mcp4-adminweb12` rebase 到 main `b1efbfdf` 后一笔 `5ee87a88`）

**要做的第 5 条补齐 ✅**：`BusinessPartiesPage.tsx` 新 `BusinessPartyRevisionHistory`，照 `GroupLegalEntitiesPage` 的 `LegalEntityRevisionHistory`
一册——按 partyId 经 `listBusinessPartyRevisions` 取整条修订链，`Timeline` 摆 `businessPartyRevisionTimeline` 的判读（每笔修订号 / 名称 /
依据 / 生效自 / 登记时间，停用笔标`已停用`、色调 warning）；未配置（403，「今天没有问到」）与无历史（200 + []，
`businessPartyRevisionHistoryNote(0)`「登记册上没有这个参与方的修订」）两句分开；换行重取、旧请求按 cancelled 丢、答案回显 partyId
核不上即丢弃并给重试。`BusinessPartyDrawer` 的占位段换成该组件。

**判断项**：组件 `key` 带上列表行的最新修订号（`${partyId}#r${revision}`）。本页自票 10 起登记签与身份签同页共存、抽屉状态在
TabsContent 隐藏时不卸载——操作者开着抽屉去登记签给同一参与方登下一笔或停用，列表重取后行的 revision 变了，历史区随之重挂重取；
只按 partyId 记依赖会继续显上一条链。`GroupLegalEntitiesPage` 的 `LegalEntityRevisionHistory` 只按 legalEntityId 记依赖、同页也有登记签，
是否同病未在本票核，不动法人册那一页（票面「不做」）——留给评审或后续票判。

**完成判据**：前端三道门 ✅ `tsc -b --noEmit` 0 / `run-tests` 279（= 10 进 main 后的 main）/ `vite build` 0。演示形态
（`SYN-PARTY-RETIRED-01` 抽屉显 2 笔、第 2 笔标`已停用`）**未验**——本机无浏览器验收路径，如票面完成判据所允许如实记「未验」；
判读那一层由 `business-party-revisions.test.ts` 以同一演示数据钉着（r1 / r2 · 已停用，warning）。Go 侧本笔零改动，不重跑。

**票面归因改口**（评审 S1）：要做的 4 与 (a)(b) 完成记录里「ADR-0091 决定一的三条判据」改为「ADR-0078 Decision 二 / 表内既有三条口径」；
代码注释本就未引 ADR-0091，不动。

**推法**：rebase 后分支基改写，`git push --force-with-lease`（远端在预期 tip 才覆盖）；rebase 把已进 main 的七笔识为上游已有而跳过，
分支上只剩 (c) 这一笔 + 本票面笔。

### 评审 ← 通道 2 · (c) · 钉 `5ee87a88`（票面 `c280ea0f`）· 基线 `b1efbfdf` · 23:34

（`task-2510f5fc`；隔离树 `%TEMP%\idp-review-12c`，评完已拆；实跑 `tsc` 0 / `run-tests` 279 / `vite build` 0；diff 只落 `BusinessPartiesPage.tsx` + 票面。全文随 `report_task`，此处按其一行汇总代落。）

- **Standards（阻断 0 / 非阻断 1，判断项）**：**N1** 组件 `key` 带修订号那条注释的理由在今天的 `useRegisterList` 下不成立——列表重取时答案置空、抽屉随之卸载重挂，`key` 属冗余但无害的保险；代码不必改。无发现：组件与 `LegalEntityRevisionHistory` 逐态同形（未配置 / 无历史 / 加载中 / 出错、取消与陈旧答案丢弃、回显 partyId 核对）；判读只经 `businessPartyRevisionTimeline`；注释中文、符号名、无计数。
- **Spec（阻断 0 / 非阻断 0）**：要做的 5 全部字面到位；演示形态未验如实记、判读层由 `business-party-revisions.test.ts` 同数据钉住；S1 归因改口两处到位。
- **一行**：Standards 0 / 1（判断项）· Spec 0 / 0。无阻断，可推。

### 进 main 记录 · (c)（推送方 = 通道 1 · 23:35）

隔离树 `%TEMP%\idp-land12c` detached 于 `e86ecfea`（= 当时远端 main），cherry-pick 两笔零冲突 `5ee87a88→c7afefda` / `c280ea0f→29e5117b`（patch-id 同；两件对作者 tip 零 diff）；tip `29e5117b` 上 `gofmt -l` 空、`go build ./...` / `go vet ./...` 0、清点重生成 porcelain 空；admin-web `tsc -b --noEmit` 0 / `run-tests` 279 / `vite build` 0；带 DSN `go test -p 1 -count=1 ./...` **115 ok / 0 FAIL / 16 无测试 / 0 cached**（23:32:10→23:34:30；本笔不动 Go）、`cmd/parcel-api` 真库探针 PASS 非 SKIP → 评审无阻断（上）→ `ls-remote` 核 `e86ecfea` 未动 → 23:35 `push 29e5117b:main` 成，**远端 main = `29e5117b`**；共享树 ff 同 SHA。**票 12 全部落地**：Status → resolved（本簿记笔）。N1 与作者判断项里「`GroupLegalEntitiesPage` 是否同病未核」一并记入 `pages/party` 收口票（票 10 处置清单之后追加为第 ⑨ 条：核两页历史区依赖键与 `useRegisterList` 重取的交互，同一做法两页取齐）。
