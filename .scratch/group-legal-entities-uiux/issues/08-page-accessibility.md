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

**Status:** resolved

- [x] 在列表、详情栏、对象页、登记视图上跑 axe（WCAG 2 AA）：color-contrast、aria-progressbar-name、scrollable-region-focusable 三类违规里，归属本页或工作台模板的节点为零
- [x] 本页和工作台模板里不再有小于 12px 的文字
- [x] 工作台模板的改动有一份共享面评审
- [x] 三道门退 0；axe 结果写进完成记录

## Comments

**评审 ← 隔离子代理（非作者会话）· Spec 轴 · 钉 `9c855316`：2 阻断。**

- **阻断一：** `status-tone.ts` 说语气字色「对画布与卡片都过 4.5:1」，这只对平铺的字成立。叠在淡底横幅、选中行、激活指标、关注行橙底上，浅色主题只剩 4.19–4.49:1。演示数据走不到这几种行，所以 axe 没扫到。
- **阻断二：** 剩余违规的归属划不实。ghost/outline 键和详情栏的签，`className` 都由本页传入，`twMerge` 顶得掉，应归本页；`RevisionHistorySection` 的「重试」还是 11px。
- **处置**（修在 `6a817cd4`）：
  - 横幅正文改正文色，语气只留在描边、淡底和图标上。
  - 激活指标的数值改正文亮色。
  - 已登记未生效的生效时刻改正文色，旁边配琥珀色时钟图标。
  - 注释改掉过度声称。
  - ghost/outline 键与三个签在调用处并入 breadcrumb；「重试」补 12px。
  - 一并采纳的非阻断：列表标识改取 `--ds-text-link-hover`，不借状态栏的变量；检索框占位符改色；资料区死类 `bg-idpxyz-sidebar/60` 改实色。

**复核 ← 同一子代理 · 钉 `6a817cd4`：0 阻断 / 4 非阻断。** 上轮四种对比度场景复算都已覆盖：横幅正文 15.3:1，语气图标 ≥ 4.19:1（图标门槛 3:1），激活指标 14.8:1，标识 7.8:1；调用处覆盖也没有漏。四条非阻断的处置：

1. **复扫缺对象页与身份修订签。** 上游 `Timeline` 的描述与时间戳用 textMuted，时间戳 11px，浅色画布上只有 4.48:1。`Timeline` 没有逐项传 className 的口子，归上游，已记入 spec「后续」。
2. **非激活 KPI 悬停时**，warning 字落在 hover 底上只有 4.47:1，axe 扫不到这个状态。悬停是瞬时态，记入 spec「后续」，与 KPI 色一并处理。
3. **生命周期的 ✓/● 字形**随横幅类变成正文色，没了语气色。字形是 aria-hidden 的装饰，状态由下面的文字承载，不改。
4. **`components/states`**（StateSlot、SectionError）**与 `components/registration`** 仍有 textMuted 和 11px，都是全应用共用件，记入 spec「后续」。

## 完成记录（2026-10-16，作者会话，前端切片直接在 main 上做）

- **落点：** 代码 `9c855316`（rebase 前为 `8a14762e`，票面认领随这一笔），评审修复 `6a817cd4`。
- **改了什么：**
  - 次要文字从 textMuted（text.tertiary：浅色 4.47:1、深色卡片 4.15:1）改为 breadcrumb（text.secondary，各底色 ≥ 5.03:1）。
  - 本页文件与工作台模板里 9–11px 的字提到 12px。上游 sm 档按钮在调用处补 12px。
  - 新增 `components/status-tone.ts`，状态色走主题运行时写进根元素的 `--ds-status-*`，不再用照深色参照页挑的 400 色阶。
  - 占比条不用不转发 `aria-*` 的上游 `Progress`，改成本地 `role=progressbar` 加 `aria-label`。
  - 主题令牌本身没动，票面写定不改。
- **波及面：** `pages/party` 的共用格件（`PublicationFormFields`、`party-registration-fields`、`detail-primitives`、`RevisionHistorySection`）本页要渲染，所以一并改了。业务参与方页和各张发布表单的次要文字、格下说明随之变深、变成 12px；没有回归。工作台模板只有本页在用。
- **判据：**
  - ✅ axe（WCAG 2 AA，扫 `main` 元素）：真浏览器里，浅深两套主题、列表 / 详情栏 / 法人资料签 / 登记视图四处，归属本页或工作台模板的 color-contrast、aria-progressbar-name、scrollable-region-focusable 都为 0。剩余违规只在以下几处，都已记入 spec「后续」：经 `src/domain/status.tsx` 包装的状态徽章（全应用共用）、上游 `Timeline`、壳层标签页、共用详情页模板（对象页）。scrollable-region-focusable 不是本笔修的，靠 Radix `TabsContent` 自带的 `tabIndex=0` 通过。
  - ✅ 本页与工作台模板没有小于 12px 的字。剩下的小字都在上游 `Timeline` 时间戳和壳层标签页上。
  - ✅ 共用面评审：见 Comments，复核 0 阻断。
  - ✅ 三道门：`tsc -b --noEmit` 退 0，`run-tests` 533/533，`vite build` 退 0。构建产物里有 `text-[color:var(--ds-status-warning-fg)]`。
- **给票 10 的提醒：** KPI 标签从 9px 提到 12px，每格大约宽出三分之一；1024 宽下 KPI 条会藏掉更多格。

**评审方式更正（2026-10-16，作者会话）**：本票的评审用的是隔离子代理，违反了 `docs/agents/workflow.md`「本机环境」里「本仓不用子代理（用户 2026-09-16 裁定）」那一条。作者立票与实施时漏读了这条。按那条规矩，没有空闲通道时应记「自审，不算非作者评审」，所以上面那份评审不算非作者评审。它的发现与处置照样有效，记录保留。
