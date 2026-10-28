# 02 收寄后处置管理台页

Category: enhancement
Status: in-progress——2026-10-28 作者完工，待评审与进 main，分支 claude-demo-pid（最后一笔见 git log）
Blocked by: 无

## 现象

审计：「取消与收寄后处置」页（`cancel-parcel`）没有收寄后处置入口；后端口都在（launch-wiring/05，ADR-0341）：`GET /post-intake-disposition-requests`、`POST /post-intake-disposition-decisions`、`GET /post-intake-disposition-manual-items`（`?parcel=`）、`POST /post-intake-disposition-manual-item-results`。冻结清单记为 R2，演示「异常处理」要用。

## 做什么

只改 `apps/admin-web`：一页（或在合适的现有页加签）——待决请求列表 → 对一件包裹做处置决定（去向：继续、拦截、改送、退运、服务终止）→ 待人工处理清单（带 `supersededByLaterDecision` 标志）→ 逐项登记已处理 / 无法处理 + 说明 → 按包裹读回各项结果。写入口按授予收起（照 ADR-0315 的自查读口与现有页面写法）。`cancel-parcel` 页上客户渠道的取消签会 401（first-party-api/05，冻结），本票不动它，但页名或说明不要让人以为收寄后处置在那里。

## 完成判据

- 组件单测覆盖列表、决定、登记、读回与写入口收起；`tsc` 与全部单测通过；在 `page-registry.tsx` 的 `liveIds` 登记。

## 完成记录

起点 `5d5a4484b`。只改 `apps/admin-web` 与本票面。

- 新页 `post-intake-disposition`（导航「收寄后处置决定」，挨着 `cancel-parcel`）：待决请求列表 → 五向处置决定（改送带目的地）→ 待人工处理项清单（所有者、动作、`supersededByLaterDecision` 文案）→ 逐项登记已处理 / 无法处理 + 说明 → 按包裹读回。已在 `page-registry.tsx` 的 `pageById` 与 `liveIds` 登记。
- 呈现逻辑在 `shipment-request/post-intake-disposition.ts`，页面在 `PostIntakeDispositionPage.tsx`，四个端点镜像在 `api.ts`。
- 写入口按授予收起：`operator-grants.ts` 加 `mayMakeDecision` 与两种运营决定原词，`use-operator-grants.ts` 加 `useMayMakeDecision`（不知道就摆，ADR-0315）。
- `cancel-parcel` 页说明加一句指向新页。

代决：(1) 组件单测按本仓惯例做成纯逻辑层 node:test（仓内无 React 渲染测试设施），覆盖列表、决定、登记、读回、入口收起；(2) 登记 409 冲突的响应体经 `exchange` 折成 callerProblem，只呈现状态并重读，不展示冲突里的当前结果；(3) 未做「运营发起」（无请求键）的处置决定入口，票面只要求待决请求列表上的决定。

自验：`tsc -b` 零错，`pnpm test` 818 通过 0 失败，`vite build` 通过。未在真浏览器里点过（无后端）。
