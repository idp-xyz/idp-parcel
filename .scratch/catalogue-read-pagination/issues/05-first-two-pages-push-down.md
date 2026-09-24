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

## Comments

- 2026-09-24 · 接页面前先知道（票 03 评审 ← 通道 3 非阻断 2，推送方代记）：网络目录迁到 ADR-0144 后，版本表没有登记时间一格，缺省序是 `code`（决胜键同向），
  同一对象的最新版不再排在前面。页面若要「最新版在前」，可给 `sort=-code`（决胜键同向即版本降序），代价是对象按代码倒序。
- 2026-09-24 · 翻完不等于「共 N 条」（票 02 评审 ← 通道 3 非阻断 2，推送方代记）：客户账户目录按最新修订的登记时刻排（票 02 判断项 1），还没翻到的账户在翻页
  期间新增修订会跳到游标之前；总数与本页又是两条语句、不在同一快照（票 02 判断项 4）。两者叠加，翻完所有页累计的行数可以不等于「共 N 条」——页面别拿两者相等作断言或提示。
