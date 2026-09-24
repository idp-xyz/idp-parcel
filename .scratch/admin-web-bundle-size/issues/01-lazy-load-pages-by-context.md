# 01 页面按域懒加载，首屏块压回告警线内

Category: enhancement
Status: resolved——2026-09-24 通道 1 在 `main` 上直接做（前端切片），与完成记录同一笔；Spec 轴为自审（无空闲的非作者通道，本仓不用子代理）；懒加载与错误边界的浏览器实测未验，见完成记录。此前 ready-for-agent
Blocked by: 无
地盘：`apps/admin-web/src/page-registry.tsx`、`src/main.tsx`、`src/Layout.tsx`、`vite.config.ts`（只改拆块注释）、`README.md`「已知跟进」；如抬出纯逻辑，新增 `.ts` 与其 node:test。
出处：[spec](../spec.md)「原型实测」的 B 格。

## 开工前

前端切片是一人在 `main` 上做。开票时（2026-09-24）共享树上压着 [legal-entity-profile/04](../../legal-entity-profile/issues/04-admin-web-identity-fields-and-profile-face.md) 的未提交改动（`pages/party/**`），与本票地盘不重叠，但仍等它提交后再开工。

## 做什么

1. `page-registry.tsx`：`pageById` 的值改为按域 barrel（`./pages/<域>`）的 `React.lazy`。页面登记仍只此一处；`readinessOf` 只看有无登记，行为不变。
2. `main.tsx`：`configureShipmentRequestApi`、`configureVisibilityApi` 改从各自的 `api` 模块导入，不经 barrel。经 barrel 时入口会静态依赖整个域，这两个域有一部分连同模板被钉回首屏块（spec 实测）。
3. `Layout.tsx` 渲染 `pageById[moduleId]` 那一处：包 `Suspense`，加载态只占该标签的页面区，外壳不动；再包一层错误边界接住块加载失败（发版后旧块 404），给重试或刷新，不白屏。
4. 预取：首屏渲染后浏览器空闲时预取全部域块。导航点击、命令面板、恢复标签、hash 直达走的是同一条加载路径，悬停预取只盖得住第一种；总下载量与今天相同，只是挪到首屏之后。
5. 改写 `vite.config.ts` 拆块注释：「页面本身不做路由级懒加载——重量在依赖不在页面」的前提已不成立，改成现在的取舍与理由。`README.md`「已知跟进」产物体积一条改正病因（是 `IconPicker`，见 spec），并指向票 02。

## 约束

- 导出名拼错必须仍是 tsc 错误：懒加载器按属性访问取导出（`m.GroupLegalEntitiesPage`），不用字符串名查表。
- 不动 `manualChunks` 的三块划分。各页今天只经 ui-kit 用第三方依赖，懒加载页不会把新依赖卷进 `vendor`；哪天某页独用一个重依赖再议——那时 `vendor` 兜底规则会把它提前拉进首屏。

## 不做

- 不按单页拆：按域已够，spec 实测最大的懒加载块（party）约 206 kB。
- 不碰 `@idpxyz/ui-primitives`（归 02），所以本票做完 `vendor` 仍超线、告警仍在。

## 完成判据

- 三道门退 0。
- `vite build` 输出：入口块远低于告警线，各域为独立的懒加载块；超线的只剩 `vendor`。
- 懒加载与错误边界：能用 `scripts/dom-probe.mjs` 实测就实测（打开一个域页面先见加载态再渲出页面；加载器失败时见错误边界），探针源不入库、结论写完成记录；做不到如实写「未验」。
- 工作台就绪度总览不变（`readinessOf` 各档）。
- 碰外壳（`Layout.tsx`），按 workflow「前端切片」第 5 步要一份 Spec 轴评审，无人就自审并在票面写明。

## 完成记录（2026-09-24，通道 1，`main` 上直接做）

**落点**（一笔）：`page-registry.tsx`（按域 barrel 的 `React.lazy` + `prefetchPages`）、`main.tsx`（`configure*Api` 改从 `api` 模块引）、`Layout.tsx`（页面区包 `Suspense` 与错误边界、首屏后空闲预取）、新 `shell/PageChunkBoundary.tsx`、`vite.config.ts` 拆块注释、`README.md`「已知跟进」、本票与 spec 子票表。

**完成判据**

- ✅ 三道门（钉工作树 = HEAD `9eacbd8a` + 本笔，WSL，Node 22.23.3）：`tsc -b --noEmit` 0 / `run-tests` 443 pass 0 fail / `vite build` 成功；admin-web 端点消费门 PASS。
- ✅ `vite build`：入口块 70.58 kB（gzip 23.29 kB；本笔之前在 `9eacbd8a` 为 618.87 kB），各域是独立的懒加载块，最大的 party 域 224.12 kB；超线的只剩 `vendor` 1,187.08 kB（归 02）。`index.html` 只预加载 `react-vendor` / `ui-kit` / `vendor`。
- ◑ 懒加载与错误边界：**未验**——没用 `scripts/dom-probe.mjs` 或浏览器走加载态与块加载失败；结构上 `Suspense` 与错误边界各包一张标签的页面区。
- ✅ 工作台就绪度总览不变：`pageById` 的 id 集与改前逐一相同（对照 `9eacbd8a`），`liveIds` / `demoIds` / `readinessOf` 一行未动。
- ✅ 碰外壳（`Layout.tsx`）的 Spec 轴评审：**自审**，见下。

**Spec 轴自审**——阻断：无。做什么 1–5 逐条对上；约束两条守住（导出名经属性访问取、拼错是 tsc 错误；`manualChunks` 三块划分未改，只在兜底规则旁补一句为什么要留意）；不做两条守住（按域不按页；未碰 `@idpxyz/ui-primitives`）。

**判断项**

1. 预取走 `requestIdleCallback`，旧版 Safari 没有时退回定时；预取失败吞掉不报，真打开那一页时由错误边界接住。
2. 错误边界的「重试」是整页刷新：`React.lazy` 会记住失败的那次加载，原地重渲取不回来。
3. 错误边界也接页面渲染时的运行错误，不只块加载失败——只红那一张标签比整台白屏好；文案里「若刚发过版」写成条件句。

## Comments
