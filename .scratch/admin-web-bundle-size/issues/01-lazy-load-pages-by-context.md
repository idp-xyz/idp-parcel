# 01 页面按域懒加载，首屏块压回告警线内

Category: enhancement
Status: ready-for-agent
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

## Comments
