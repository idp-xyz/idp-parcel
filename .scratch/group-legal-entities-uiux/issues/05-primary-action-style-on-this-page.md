# 05 — 主动作用主色（本页）

**Parent:** [spec](../spec.md)

**What to build:** 现在页面上的主动作看起来像是禁用的，而筛选胶囊的激活态反倒是实心蓝，视觉层级颠倒了。根因有两层：

- 上游 `@idpxyz/ui-primitives` 的 `Button` 不传 `variant` 时用的是 `secondary`，即灰底配次要文字色。本仓没有一处用过它的 `default` 变体。
- 即便改用 `default`，它的 `bg-idpxyz-accent/20` 在本仓 Tailwind 配置下也不会生成：主题色是裸 CSS 变量，透明度变体出不来。票 [admin-web-group-legal-entities/15](../../admin-web-group-legal-entities/issues/15-legal-entities-follow-purchase-order-workbench.md) 的「白线」处置对构建产物实测过同一类问题。

本票在 admin-web 内提供一个主动作样式，属于共享面，并用在本页这几个主动作上：
- 页头「登记责任法人」
- 详情栏「登记资料修订」
- 登记表单的「提交登记」
- 资料表单的「提交资料修订」

次要动作不改，比如返回列表、移除、添加一行。其余页面另立一票处理（12）。

本票不改上游包。如果结论是上游应该补一个 primary 变体，就把它写进完成记录，作为给 idp-ui 的建议。

**Blocked by:** None — can start immediately

**Category:** enhancement

**Status:** resolved

- [x] 主动作样式在浅色和深色两套主题下，文字与背景的对比度都 ≥ 4.5:1，悬停、聚焦、禁用三种状态能区分开
- [x] 上面列出的本页主动作都改用这个样式，次要动作不变
- [x] 有一份共享面评审（Spec 轴）
- [x] 三道门退 0；两套主题各截一张图，在完成记录里说明

## Comments

**评审 ← 隔离子代理（非作者会话）· Spec 轴 · 钉 `030b2606` · 2026-10-16：** 0 阻断，非阻断四条。

- **已核实的几项：**
  - 用 tailwind-merge 2.6.1 按 cva 的拼接顺序合并（sm 和 default 两档），`/20`、`text-idpxyz-accent`、`/30`、`ring-1`、`/35`、`ring-offset-0` 全部被顶掉，`text-[12px]` / `text-[11px]` 都保留。
  - Tailwind 3.4.19 产出了全部九个类。
  - 白字对比 4.51:1，悬停 5.88:1；聚焦环对各底色都 ≥ 3:1。
  - 只改了票上的四处，次要动作没动。
- **处置：**
  - **非阻断 1**（登记视图确认框的「离开」：`tone="warning"` 在上游映射为 default 变体，同样只有强调色字、没有底）：不在本票四处之内。已记到票 [12](12-primary-action-style-on-remaining-pages.md)，并写进下面的上游建议。
  - **非阻断 2**（注释里聚焦环与画布色的里外关系写反了）：已改，`docs(admin-web)` 那一笔。
  - **非阻断 3**（补一条测试，钉住 twMerge 顶掉 default 变体）：**不采纳**。评审说「`publication-form-rendered-paths.test.ts` 已在测试里导入过 ui-primitives，有先例」，复核下来不成立：那份测试没有导入任何 `@idpxyz/*`，全仓也没有一份测试导入它们。`@idpxyz/*` 是带 `.css` 导入的 ESM 源码，在 node:test 的 CJS 链上加载不了（见 `section-error.test.ts` 文件头）。在测试里抄一份上游类名，又接不住上游升级，正好丢了这条测试的本意。覆盖是否生效，靠真浏览器计算样式与评审的 tailwind-merge 实跑证明。
  - **非阻断 4**（收口时补齐验收）：见下面的完成记录。

## 完成记录（2026-10-16，作者会话，前端切片直接在 main 上做）

- **落点：** 代码与票面认领同在 `030b2606`；评审后修正注释另在一笔。
- **改了什么：**
  - 新增 `components/primary-action.ts`，导出 `primaryActionClass`，写法同 `registration/chip.ts`：只是一串类名。内容为强调色实底、白字、`color-mix` 压暗 85% 的悬停色、实色聚焦环加画布色间隔。
  - 本页四处改为 `variant="default"` 加上这串类：页头「登记责任法人」、详情栏「登记资料修订」、登记表单「提交登记」、资料表单「提交资料修订」。
  - node:test 守住两件事：实底、白字、聚焦环都在场；主题色类不带 `/NN`。
- **判据：**
  - ✅ 对比度与状态：在真浏览器里实测计算样式，浅色和深色主题下底色都是 `rgb(0, 122, 204)`、字色 `rgb(255, 255, 255)`，白字对比 4.51:1。悬停底色 `color(srgb 0 0.4067 0.68)`，约 5.9:1。键盘聚焦时 `box-shadow` 有一圈画布色加一圈强调色环。禁用态（修订号填 `abc` 时）`disabled` 且透明度 0.4。
  - ✅ 次要动作不变：返回列表、移除、添加一行、更多菜单、刷新都还是原来的变体。
  - ✅ 共享面评审：见 Comments，0 阻断。
  - ✅ 三道门：`tsc -b --noEmit` 退 0，`run-tests` 530/530，`vite build` 退 0。构建产物里有 `hover\:bg-\[color-mix…]` 和 `ring-offset-idpxyz-bg`。
  - ✅ 截图：浅色是列表页加详情栏，两处主动作都是实底蓝；深色是登记视图，「提交登记」是实底蓝。截图存放在作者会话的临时目录，没有入库，按上面的步骤可以重拍。
- **给 idp-ui 的上游建议（不在本仓改）：**
  - `ui-primitives` 的 `Button` 补一个实底 primary 变体；或者 `default` 变体与基类的聚焦环别用透明度变体。
  - 也可以让消费方主题色改成支持 `<alpha-value>` 的写法，比如变量存 RGB 通道。
  - 现状下 default 变体、`ConfirmDialog` 的确认按钮（`confirmVariant: 'default'`）和全部按钮的聚焦环，在本仓都读成「没有底、没有环」。
- **对比度的前提：** 强调色换掉时要重算。白字对 `#007acc` 只比 4.5 多 0.01，更浅的强调色会掉到线下。比如 ui-tokens 给 parcel 登记 productAccent 时，就要重算。
