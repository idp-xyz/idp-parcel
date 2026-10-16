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

**Status:** in-progress

- [ ] 主动作样式在浅色和深色两套主题下，文字与背景的对比度都 ≥ 4.5:1，悬停、聚焦、禁用三种状态能区分开
- [ ] 上面列出的本页主动作都改用这个样式，次要动作不变
- [ ] 有一份共享面评审（Spec 轴）
- [ ] 三道门退 0；两套主题各截一张图，在完成记录里说明
