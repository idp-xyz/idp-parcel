# 05 管理台首批两页改为服务端翻页、筛选与检索下推，README 通则改写

Category: enhancement
Status: resolved——2026-10-10 通道 5 收口（通道 1 派单 `task-693dea87`）：「做什么」四条在 10-09 的四笔里已落完（`3b396074`、`22e0d476`、`c9907367`、`7f8eddd7`），本次补三道门、两条判据的走查与完成记录，见文末。此前 in-progress——2026-10-09 通道 5 接（通道 1 派单 `task-2be15e03`），在共享树 `main` 上直接做（workflow.md「前端切片」）。此前 ready-for-agent
Blocked by: 02、03、04；另等 [admin-web-workspace-form/06](../../admin-web-workspace-form/issues/06-list-detail-roundtrip-keeps-page-state.md) 进 main（检索词进地址）
地盘：用 `/commercial-customer-accounts` 与 `/network-catalog` 的页面（`pages/party/api.ts`、`pages/network/api.ts` 及其列表页）、`apps/admin-web/README.md`。
出处：[ADR-0144](../../../docs/adr/0144-catalogue-reads-share-one-cursor-pagination-sort-and-filter-contract.md) 决定四、八与 Consequences。

## 做什么

1. 两页的 api 层按 ADR-0144 带上游标、排序、筛选维与 `q`，收答复的 `page`；列表页接 04 的游标模式。
2. 检索词沿票 06 已在地址上的 `?q=` 原样下推；两页的客户端筛选与「对这一页排」的排序撤掉，头注改写成现状。
3. `apps/admin-web/README.md`「列表页上列通则」里「过滤只在已取回的数据上做」一条，按 ADR-0144 Consequences 改写。
4. 答复 400 带理由散文时原样示出（沿既有 `problemNote`）。

## 不做

- 不动其余列表页——它们的读口还没迁。

## 完成判据

- 三道门；两页在演示种子上「检索 → 翻页 → 换检索回第一页」走通（dom-probe 或浏览器，做不到如实写）。
- README 通则改写后，未迁读口的页面行为不变。

## 完成记录（2026-10-10，通道 5，`main` 上直接做）

本次是收口，不是重做：「做什么」四条在 10-09 的四笔里已经落完，逐条对过 diff 与 ADR-0144 决定四、五、八和 Consequences，没有缺口要补；本次补门禁、两条判据的走查
与这段记录，代码一行未改。

**落点**

| 笔 | 文件 | 做了什么 |
|---|---|---|
| `3b396074` | `pages/catalogue-view.ts`、`.test.ts`、票面 | 第 4 条：`catalogueViewState` 的 4xx 分支把 `ApiResult` 的 `detail` 原样接在 `problemNote` 说明之后，不带 `detail` 时说明一字不差；Status → in-progress |
| `22e0d476` | 新 `pages/catalogue-query.ts`、`pages/catalogue-cursor.ts`、`pages/use-catalogue-cursor.ts` 与前两份的 node:test；`pages/catalogue-view.ts`；`pages/party/api.ts`、`PartyContractsPage.tsx` | 第 1、2 条（客户账户签）：条件编成查询串、兼作翻页轨迹的条件签名；游标翻页 reducer 与钩子，检索词停稳 300 毫秒再下推；`catalogueViewState` 加可选 `narrowed`；`listCustomerAccounts` 收条件与游标、答复带 `page`；签上检索词改走地址 `?q=` 下推，接模板游标翻页条，撤客户端筛选 |
| `c9907367` | `pages/network/api.ts`、`NetworkCatalogPage.tsx`、`ServiceAreasPage.tsx`、`presentation.ts`，两份读口 node:test | 第 1、2 条（网络目录页，连带读同一读口的服务区域页）：`listNetworkCatalog` 收 `networkCatalogConditions`（族作册子选择器，进同一组条件）与游标；两页检索词下推，接游标翻页条，撤客户端筛选；检索框提示按族写出 `q` 落在哪几格 |
| `7f8eddd7` | `apps/admin-web/README.md` | 第 3 条：「列表页上列通则」里筛选与检索那一条，按 ADR-0144 Consequences 改成已迁 / 未迁两分 |
| 随本段提的一笔 | 票面、spec | 完成记录；Status → resolved；spec 子票表 05 一行 |

**完成判据**

- ✅ 三道门（钉 `b57ff794`，即四笔之后此刻的 main 头；WSL，`~/.local/node` 的 Node v22.23.3）：`tsc -b --noEmit` 退 0 / `run-tests` 484 pass 0 fail、退 0 /
  `vite build` 退 0（只有 vendor 块超 500 kB 的既有告警，见 README「已知跟进」）。跑完 `apps/admin-web` 下 `git status` 干净。
- ✅ 两页在演示种子上「检索 → 翻页 → 换检索回第一页」走通：`scripts/dom-probe.mjs` 打一次性真栈，**42 ok / 0 fail**（探针源 `/tmp/ch5-demo/probe.tsx`，不入库）。证据层级 `S`。
  - **栈**：`git archive b57ff794` 的快照原样构建 `parcel-api`，Go 一行未改，页大小仍是 `cmd/parcel-api` 的 `isolatedReadLimit`（200）；另起一只 `postgres:16.14`
    （`127.0.0.1:55439`，不碰共享门禁库 55432），`scripts/demo-seeds/seed.sh` 退 0；进程只设 `IDP_PARCEL_ISOLATED_READ_TENANT=SYN-TENANT-01`，只听回环。页面源取自
    同一快照，探针把 `fetch` 转发到这个栈。用完已拆。
  - **补数**：原样种子里客户账户只有 `SYN-ACCOUNT-01` 一个、节点族 5 行，页大小 200 下只有一页，「翻页」那一步在原样种子上无从发生。于是往这只一次性库补了 205 个
    合成账户 `SYN-ACCOUNT-PAGE-001`…`205`（各配一个合成参与方 `SYN-PARTY-PAGE-*`，经 `parcel-commercial register-parties`，410 项 REGISTERED）与 205 个合成节点
    `SYN-NODE-PAGE-001`…`205`（经 `parcel-network-register`，205 行 REGISTERED）。
  - **浏览器没走**：`AuthGate` 包在应用最外层，未登录只渲登录页，要 gk.idp.xyz 的真登录，本会话做不到；探针直接挂页面组件，绕开的只是登录门。
  - **客户账户签**：首屏「第 1 / 2 页 · 共 206 条」、200 行。逐字敲 `PAGE`（每字隔 60 毫秒）只发出停稳后的一问 `?q=PAGE`，地址随之带上 `?q=PAGE`，「第 1 / 2 页 ·
    共 205 条」。下一页带同一检索词与答复给的游标，「第 2 / 2 页 · 共 205 条」5 行，与第一页不相交、两页合起来恰是 205 个不同账户，末页下一页不能按；上一页回第一页，
    再翻回第二页。在第二页上换检索词 `SYN-ACCOUNT-01`，新一问不带旧游标，「第 1 / 1 页 · 共 1 条」；改回 `PAGE` 落第一页，不复活先前翻到的第二页；零命中说「当前
    检索条件下没有匹配的货主客户账户」，不报册为空。
  - **网络目录页**（节点族）：首屏「第 1 / 2 页 · 共 210 条」；检索 `PAGE` 发 `family=node&q=PAGE`，「第 1 / 2 页 · 共 205 条」；下一页「第 2 / 2 页」5 行，不相交、
    合起来 205；在第二页上换检索词 `SHA` 回第一页，`SYN-NODE-SHA-HUB` 两个版本；在第二页上换族「网络连接」发 `family=connection&q=PAGE`、不带旧游标，零命中说
    「当前检索条件下没有匹配的网络连接版本」；清空检索后连接族三行。
  - **服务区域页**（同一读口，`c9907367` 一并迁的）：首屏「第 1 / 1 页 · 共 2 条」；检索 `SG` 下推为 `family=service-area&q=SG`，只剩 `SYN-AREA-SG`。
  - **第 4 条顺带实测**：服务端对 101 字的检索词答 400 `MALFORMED_REQUEST`，`detail` 是「检索词 q 超过 100 个字符」；页面示「调用方式问题（HTTP 400）」，`problemNote`
    的说明之后原样接上这句。
- ✅ README 通则改写后，未迁读口的页面行为不变。核法三层：
  1. **改动面**（实测于 `b57ff794`）：`3b396074~1..7f8eddd7` 之间碰 `apps/admin-web` 的只有这四笔；未迁读口各页的源文件一个没动，`PartyContractsPage.tsx` 里同页的
     「客户合同」表也没动（diff 只在账户表与导入）。未迁页共用到的改动只有 `pages/catalogue-view.ts` 那两处，两处都只在已迁那一侧生效：`narrowed` 只有已迁三页在传；
     `detail` 只在答复带它时才接上，而带 `detail` 的目录读口 4xx 只出自已迁两口——商业上下文的 `writeProblemWithDetail` 只有客户账户读口与登记写口在调，其余上下文的
     错误体里没有这一格。`listCustomerAccounts`、`listNetworkCatalog` 两个读口函数也只有已迁三页在调。
  2. **node:test**：`catalogue-view.test.ts` 的「调用方问题带理由散文时原样接在问题码说明之后，不带时说明不变」与「带检索或筛选条件答出零行仍是 ready，不带条件的零行才是
     空态」钉住不传 `narrowed`、不带 `detail` 时与先前同，在上面那次 484 pass 里。
  3. **真栈**：同一探针切到同页「客户合同」签（`/commercial-customer-contracts`，未迁）：只发裸路径、不带任何查询参数，没有游标翻页条；检索 `NO-SUCH-CONTRACT` 在已取回
     的行上筛空、不发新请求，清空后 `SYN-CONTRACT-01` 一行回来，仍不发新请求。

  覆盖面：前两层覆盖 `b57ff794` 上经 `catalogueViewState` 的全部未迁页；真栈只走了「客户合同」这一页。

**判断项**

1. **「对这一页排」没有可撤的。** 改前三页都只在已取回的行上做包含匹配，没有页内排序（四笔 diff 里没有删任何排序）；三页都不带 `sort`，取读口的缺省序。
2. **网络目录页用缺省序 `code`**：同一对象的各版本按版本升序挨着（实测 `SYN-NODE-SHA-HUB@1` 在 `@2` 之前），不是「最新版在前」。Comments 里 09-24 第一条说可以给
   `sort=-code` 换成最新版在前，代价是对象按代码倒序；本票没换，页面头注写的是「次序是读口各族的缺省序,本页不另排」。
3. **「共 N 条」只陈述、不断言。** 三页都没有拿累计行数与总数相等作判断或提示，Comments 里 09-24 第二条的要求成立。
4. **检索框提示与读口检索列逐格对过**：网络目录各族的提示对 `networkrouting` postgres 读面各族的 `keyword` 列，客户账户签对 `customerAccountKeywordColumns`（账户、
   客户参与方、名称），没有多写一格。

**缺陷**：地盘内未发现，没有另提修复笔；地盘外也没有要记进 Comments 的。

**未验**

- 真浏览器里的动线（理由见判据一）。
- `go test ./internal/architecture/` 本机未跑：diff 全在 `apps/admin-web/**` 与票面，由 CI 兜。

**评审**：四笔只碰 `pages/**` 与 README，不碰 `templates/*`、`shell/*`、`components/*`，按 workflow「前端切片」第 5 步不要 Spec 轴评审，判据自查如上。

## Comments

- 2026-09-24 · 接页面前先知道（票 03 评审 ← 通道 3 非阻断 2，推送方代记）：网络目录迁到 ADR-0144 后，版本表没有登记时间一格，缺省序是 `code`（决胜键同向），
  同一对象的最新版不再排在前面。页面若要「最新版在前」，可给 `sort=-code`（决胜键同向即版本降序），代价是对象按代码倒序。
- 2026-09-24 · 翻完不等于「共 N 条」（票 02 评审 ← 通道 3 非阻断 2，推送方代记）：客户账户目录按最新修订的登记时刻排（票 02 判断项 1），还没翻到的账户在翻页
  期间新增修订会跳到游标之前；总数与本页又是两条语句、不在同一快照（票 02 判断项 4）。两者叠加，翻完所有页累计的行数可以不等于「共 N 条」——页面别拿两者相等作断言或提示。
