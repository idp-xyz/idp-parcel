# 05 管理台首批两页改为服务端翻页、筛选与检索下推，README 通则改写

Category: enhancement
Status: ready-for-agent
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
