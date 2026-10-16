# 08 — 本页可访问性

**Parent:** [spec](../spec.md)

**What to build:** 用 axe 按 WCAG 2 AA 扫了列表、详情栏、对象页和登记视图，实测数见 spec。本票只处理其中归属本页和工作台模板的问题：

- **小字。** 9–10px 的字用得很多：KPI 标签、`/` 键帽、修订徽章、生命周期标签、引用签、事实条标签。中文 9px 基本读不清，最小提到 12px。
- **次要文字对比度不够。** 次要文字色落在浅色底上，对比度只有 4.23–4.47，略低于 4.5:1。这个颜色是上游主题令牌，全站共用，所以本票不改令牌：只把本页和工作台模板里落在小字上的次要文字换成满足 4.5:1 的令牌，或者把字号提到大字阈值以上。
- **进度条没有可访问名称**，指「在用中已生效」那一格。
- **详情栏滚动区不能用键盘聚焦**（axe 规则 scrollable-region-focusable）。

不在本票范围：标签页关闭按钮 × 没有名称、状态栏「Comfortable」对比度过低。这两项属于外壳或上游，见 spec「后续」。

**Blocked by:** None — can start immediately

**Category:** enhancement

**Status:** in-progress

- [ ] 在列表、详情栏、对象页、登记视图上跑 axe（WCAG 2 AA）：color-contrast、aria-progressbar-name、scrollable-region-focusable 三类违规里，归属本页或工作台模板的节点为零
- [ ] 本页和工作台模板里不再有小于 12px 的文字
- [ ] 工作台模板的改动有一份共享面评审
- [ ] 三道门退 0；axe 结果写进完成记录
