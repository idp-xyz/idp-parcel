# 14 管理台各列表页改游标翻页、筛选与检索下推

Category: enhancement
Status: ready-for-agent
Blocked by: 逐上下文跟随 02–13——某上下文的后端子票进 main 之后，才接该上下文的页面
地盘：`apps/admin-web/src/pages/**` 里消费 02–13 各册的 api 层与列表页；`apps/admin-web/README.md`「列表页上列通则」只在需要时改一句。
出处：ADR-0144 决定四、八；首批 [05](../../catalogue-read-pagination/issues/05-first-two-pages-push-down.md) 的接法（`pages/catalogue-query.ts`、
`pages/use-catalogue-cursor.ts`、模板 `pagination` 的 `mode: 'cursor'`）。

## 做什么

1. 各 api 层按 ADR-0144 带上游标、筛选维与 `q`，收答复的 `page`；列表页接游标翻页条（模板现成的 `mode: 'cursor'`，取数走 `use-catalogue-cursor`）。
2. 检索词沿地址上的 `?q=` 原样下推；已迁读口那几页撤掉客户端筛选，头注改写成现状。
3. 带计数与 `truncated` 的队列页：积压数照旧示出，「仅显示前 N 条」一类提示改由翻页条承担。
4. 答复 400 带理由散文时原样示出（沿既有 `problemNote`）。

按上下文分笔，每笔只接后端已进 main 的那几册：shipment-request（含 channel-selection）、visibility、pricing、operations（node 与 transport）、network（路线三页）、
settlement、customs、collection、party（客户账户之外）、governance、operator-access 与 work-devices。

## 不做

- 不动模板与外壳（`templates/*`、`shell/*`）；不改未迁读口的页面行为。

## 完成判据

- 每笔：三道门（tsc / run-tests / vite build）退 0；改到的 api 层有 node:test 钉住查询串的拼法与 `page` 的接收。
- 每个上下文至少一页在演示种子上「检索 → 翻页 → 换检索回第一页」走通（dom-probe 或浏览器，做不到如实写）。
