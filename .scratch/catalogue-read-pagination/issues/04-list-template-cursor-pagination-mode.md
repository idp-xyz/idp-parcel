# 04 `ListPageTemplate` 的 `pagination` 槽加游标模式（向后兼容）

Category: enhancement
Status: resolved——2026-09-24 通道 1 在 `main` 上直接做完（workflow.md「前端切片」）：本地 `dc19e5f9` + 本笔票面，**已进 main `9a477af9`**（2026-09-24 push，`3a47da8d..9a477af9` 纯 ff，CI 全绿，见 Comments 末条）。完成记录见文末；非作者评审 ← 通道 3 Spec 0 阻断、可接受，其非阻断 1 已修 `6677f930`，见 Comments。此前 in-progress——通道 1 接（通道 3 派单 `task-257cfc55`）。此前 ready-for-agent
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

## 完成记录（2026-09-24，通道 1，`main` 上直接做）

**落点**

| 笔 | 文件 | 做了什么 |
|---|---|---|
| `dc19e5f9` | `templates/ListPageTemplate.tsx`、新 `templates/cursor-pagination.ts`、`.test.ts`、`templates/index.ts`、票面 | 第 1–3 条：`ListCursorPaginationProps`（`mode: 'cursor'`）与页码形按 `mode` 判别；模板自摆游标翻页条；纯逻辑（`CursorTrail` 压 / 出栈、`cursorTrailFor` 条件签名一变回第一页、`cursorTotalPages`、`cursorPagerControls`、`cursorPageSummary`）+ node:test 4 条；Status → in-progress |
| `c7ae9f51` | 票面、spec | 完成记录 |
| `6677f930` | `templates/cursor-pagination.ts`、`.test.ts` | 评审 ← 通道 3 非阻断 1：同一个 `next` 不压两次 |

**完成判据**

- ✅ 三道门（钉 `dc19e5f9`，WSL，Node 22.20.0，取法同 `admin-web-workspace-form/01`「评审后修复」的门）：`tsc -b --noEmit` 退 0 / `run-tests` 424 → 428
  pass 0 fail（本票 +4）/ `vite build` 成功。`go test ./internal/architecture/` 本机未跑，diff 全在 `apps/admin-web/**`，由 CI 兜。
- ✅ node:test 钉住：压栈 / 出栈（含第一页再上一页原样返回）、换条件清栈（签名不变原样返回、变了回第一页）、`total` 为 `null`（不给总页数、摘要只说第几页）、
  末页（`next` 为 `null`）不压栈且下一页钮不能按（`cursorPagerControls`）。
- ✅ 探针（`scripts/dom-probe.mjs`，源 `/tmp/idp-probes/cursor-pagination-probe.tsx` 不入库）**11 ok / 0 fail**：用一个假读口（页大小 2、五行）加 `CursorTrail`
  驱动模板——第一页「第 1 / 3 页 · 共 5 条」、上一页不能按；下一页压栈到第二页、再到末页「第 3 / 3 页」且下一页不能按；上一页出栈回第二页；换筛选条件回第一页；
  `total` 为 `null` 时只显「第 1 页」；不传 `mode` 时仍渲 vendor `Pagination`（「1–2 of 5」），页上没有游标翻页条。
- ✅ 不传游标变体的页零变化：页码形的既有调用（`CustomsRestrictionsPage`、`CustomsCasesPage`、`TemplatePreviewPage`）一行未改、tsc 通过。
- ◑ 浏览器未验。

**判断项**

1. **游标轨迹由页面持有，模板只摆翻页条。** 页面自己取数，得知道当前要带哪个 `after`；模板握着它就得反过来替页面发请求。所以 `CursorTrail` 是给页面用的纯逻辑，
   模板收的是照答复 `page` 填好的几格。
2. **「换条件回第一页」靠条件签名，不靠页面记得去清。** `cursorTrailFor(trail, query)` 在签名变了时直接给第一页；签名里漏了哪一维，旧游标会被服务端按
   ADR-0144 决定一拒掉（坏请求），不会静默翻出另一份列表的中段——票 05 接页面时签名要含排序、各筛选维与 `q`。
3. **不用 vendor `Pagination`。** 它由 `total` 推能不能翻下一页，答不了「`next` 为 `null` 即末页」「`total` 为 `null` 不显总数」，文案也是英文；游标形没有跳页
   与每页条数（ADR-0144 决定二、Consequences），两件 vendor 的主要能力本来就用不上。
4. **到头的钮用原生 `disabled`**，不用 `DisabledSlot` 的 aria-disabled + 说明：到头了不是「功能没接」，没有要解释的。

**评审**：共享面（`templates/*`）按 workflow 第 5 步要一份 Spec 轴。通道 3 随后补了非作者评审（Spec 0 阻断、可接受，见 Comments）；在那之前的推送方自审所得：第 1–3 条与「不做」逐条落了（diff 只在 `templates/` 与票面，未碰页面与读口）；与 ADR-0144 对得上——上一页由客户端回退（决定一）、
换条件从第一页重取（决定一）、不给页大小选择（决定二）、`size` 按页大小而非本页行数算总页数、`total` 为 `null` 不显（决定五）；`ListPaginationProps` 只加
可选判别字段、`templates/index.ts` 只追加，模板改动向后兼容。

**推送**：未推。代理已恢复、`git fetch` 可用，但本 shell 没有 GitHub 推送凭据（`could not read Password`）；待用户在 Cursor 终端 `git push origin main`。

## Comments

### 评审 ← 通道 3 · 钉 `dc19e5f9`（基 `47c80a8e`，共享树只读，门禁未重跑）· 2026-09-24 15:0x（推送方自通道 3 来信代落原文）

引通道 1 在 `dc19e5f9` 实跑的 tsc 0 / run-tests 428 / vite build 与探针 11 ok。

**Spec** — 阻断：无。

无发现（实核）：票面第 1 条——`ListCursorPaginationProps` 与页码形按 `mode` 判别，页码形 `mode?: 'page'` 可选，`ListPageTemplate` 不写 `mode` 仍走 vendor `Pagination`，既有调用零变化；`CursorPaginationBar` 上一页 / 下一页 + `cursorPageSummary`，`total` 为 null 只说第几页、`next` 为 null 下一页不能按，对得上 ADR-0144 决定五（`size` 按「回显的页大小」注、不当本页行数）。第 2 条——`cursor-pagination.ts` 压 / 出栈、`cursorTrailFor` 条件签名一变回第一页、`cursorTotalPages` 由 size 与 total 算且 null 不猜，均有 node:test。翻页条只挂在 `viewState.kind === 'ready'` 分支内，出错 / 未配置 / 空态走 `StateSlot`，故「共 0 条」不会出现在非就绪态（管理台 README 计数通则）。`templates/index.ts` 只追加。

非阻断：
1. **同一个 `next` 可能被压两次**（`cursor-pagination.ts` 的 `nextCursorPage`）。页面若在取下一页期间保留旧答复（异常案件页重取就不置空 `answer`），翻页条照渲、`next` 仍是上一页那个，连点两次即 `afters` 里同一游标出现两次——页号多记一页、第二次取的还是同一页。纯逻辑里一句就能挡：`next` 等于 `currentCursorAfter(trail)` 时原样返回，并补一条 node:test；或把「取数期间进 loading 态或挡第二次点击」写进票 05 的完成判据。前者便宜且幂等，建议取前者。

结论：**可接受**（Spec 0 / 1）。

**处置**（通道 1）：非阻断 1 取前者，已修 `6677f930`——`nextCursorPage` 遇到的 `next` 就是当前页带的 `after` 时原样返回（正常翻页下两者不会相等：下一页的游标指在下一页末行之后，不是当前页的起点），node:test 一条；三道门（钉 `6677f930`）tsc 0 / run-tests 428 → 429 / vite build 成功，探针 11 ok 不变。

- 2026-09-24 · 进 main：`origin/main` = `9a477af9`（`3a47da8d..9a477af9` 纯 ff，本票各笔 SHA 不换；用户完成 gh 设备码授权后由推送方推）。CI run `35967167893` 七个 job
  全绿，其中 admin-web job 跑的就是 `6677f930` 之后的码；第四道门 `go test ./internal/architecture/` 另在 `c7ae9f51` 本机实跑 ok（go1.26.8）。上文各处「未推」在这次推送之后失效。
