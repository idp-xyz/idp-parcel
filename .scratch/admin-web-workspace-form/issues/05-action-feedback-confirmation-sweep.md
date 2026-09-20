# 05 写动作反馈与高风险确认一致性：先审计全部写面调用点成对照表，再补 `useToast` / `ConfirmDialog` / 进行中态

Category: enhancement
Status: in-progress
认领：通道 5 · 2026-09-20 20:1x · 分支 `mcp5-wsform05` · 基 `origin/main` `11e6111a` · worktree `D:\tops\idp-parcel-mcp5-wsform05`
Blocked by: 无
地盘：审计段只读全仓 `apps/admin-web/src/pages/**`；改动段落 `apps/admin-web/src/components/`（新 `components/action-feedback.ts` 纯逻辑 + 可能的 `components/ActionButton.tsx`）与
各页写动作调用点（只改「按下 → 请求 → 结果」那几行，不改页面结构）。**不碰** `templates/*`、`shell/*`、`Layout.tsx`、`pages/party/**`（它已用 `useToast`，作为对照基线不动）。
出处：spec「缺口」表第五档；蓝图 21.1「Every action must have feedback：Success / Failure / In progress / Queued / Partial success」、21.2「High-risk actions need confirmation」；
参照 `idp-ui@6751fb2` `apps/myshop-web/src/shell/RightSidebar.tsx` 快捷操作 → `addToast`、`components/ActionModal.tsx`（确认 + 表单 + 反馈一体的动作弹层）。
**参照物里的反馈全是假的**（按钮直接 `addToast('操作成功')`，没有请求），只搬「每个动作三态可见」这条规则，不搬实现。

## 为什么

本仓写面不少（`postMasterData` 的调用点：登记、发布、停用、决定、评估……），反馈各自为政：`pages/party` 用 `useToast`，别处有的行内显结果代数、有的什么都不显、
「进行中」多半没有禁按钮。蓝图 21.1 说的是**用户视角的完备性**：按下去之后必须知道成了没、还在等、还是坏了；21.2 说不可逆的要拦一下。本票不改任何语义，只把这两条铺平。

## 要做的

1. **审计段（先做，单独一笔，只读）**：列全 `apps/admin-web/src/pages/**` 里所有发 `POST` / 非幂等请求的调用点（从 `postMasterData` 与各 `api.ts` 的写函数反查，
   用 `rg` 不凭记忆），每处四格：**成功反馈**（toast / 行内 / 无）、**失败反馈**（toast / 行内 / 无 / 只 console）、**进行中**（按钮禁用 + 文案 / 无）、**是否高风险**
   （停用 / 撤销 / 覆盖 / 删除 / 不可逆发布 → 是）与**有没有二次确认**。表落进本票面「审计对照表」小节，带取证 SHA。`pages/party` 也列，作对照基线。
2. **`components/action-feedback.ts` 纯逻辑**：`feedbackFor(result: ApiResult<unknown>, verb: string)` 把结果代数（`outcome` / `problem` / `transport`）映到统一三态文案
   （成功：「已<动词>」；失败：`problem.detail` 或传输错原文，不改写；进行中：「<动词>中…」），node:test 钉三态 + 边界（`problem` 无 detail、`transport` 空 message）。
   文案里的动词取调用点已有的按钮文字，不自造。
3. **改动段**：对照表里「成功或失败无反馈」的调用点接 `useToast`（`ToastProvider` 已在 `App.tsx`）；「进行中无禁用」的加 pending 态（按钮 `disabled` + 文案）；
   高风险且无确认的接 `ConfirmDialog`（`@idpxyz/ui-primitives` 已有，第一轮 02 在用），确认文案写明**删的 / 停的是什么、影响谁**，不写「确定吗」。
   每页一笔或按上下文分组成笔，提交信写「对照表第 N 行」。
4. **不统一成弹层**：myshop-web 的 `ActionModal` 把确认 + 表单 + 反馈揉成一件；本仓的表单（登记 / 发布）已各有页面位，只补反馈与确认，不换壳。

## 不做

- 不加「排队 / 部分成功」两态——本仓没有异步排队或批量部分成功的端点；对照表里若有就如实标「无此态」。
- 不改结果代数（`ApiResult`）、不改任何 `api.ts` 的请求形状、不改 `problem` 的分类。
- 不碰 `pages/party/**`（已对齐，是基线）；不碰只读页。

## 完成判据

- 审计对照表在票面，行数 = `rg` 实测数（写「实测于 `<sha>`」），每行四格填满；无反馈 / 无确认的行在改动段结束时全部标「已补 `<sha>`」或「不补 + 理由」。
- `action-feedback.test.ts` 三态 + 边界绿；四道门绿；改过的页 `vite build` 产物含新确认文案字面量。
- 浏览器验收做不到如实写「未验」；一次性 esbuild 束断言至少一处：失败结果渲染出 `problem.detail` 原文。

## Comments
