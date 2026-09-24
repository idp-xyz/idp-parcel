# 04 `ListPageTemplate` 的 `pagination` 槽加游标模式（向后兼容）

Category: enhancement
Status: in-progress——2026-09-24 通道 1 接（通道 3 派单 `task-257cfc55`），在 `main` 上直接做（workflow.md「前端切片」）。此前 ready-for-agent
Blocked by: 无
地盘：`apps/admin-web/src/templates/ListPageTemplate.tsx`、新增的纯逻辑 `.ts` 与其 node:test、`templates/index.ts`（只追加）。
出处：[ADR-0144](../../../docs/adr/0144-catalogue-reads-share-one-cursor-pagination-sort-and-filter-contract.md) 决定八。

## 做什么

1. `ListPaginationProps` 加游标变体（与今天的页码形并存，按变体判别）：上一页 / 下一页、「共 N 条」、当前页 / 总页数；`total` 为 `null` 时不显总数与
   总页数。
2. 纯逻辑抬进 `.ts`：走过的游标栈（下一页压栈、上一页出栈）、换筛选或检索时清栈回第一页、由 `page.size` 与 `total` 算总页数。
3. 不传游标变体的页零变化。

## 不做

- 不改任何页面、不接任何读口（归 05）。

## 完成判据

- 三道门 + node:test 钉住：压栈 / 出栈、换条件清栈、`total` 为 `null`、末页（`next` 为 `null`）时下一页禁用。
- 组件层用 `scripts/dom-probe.mjs` 实测一次（探针源不入库，结论写完成记录）；浏览器做不到如实写「未验」。
- 共享面（`templates/*`）按 workflow 第 5 步要一份 Spec 轴评审。
