# 13 对象页照 Fiori Object Page：锚点栏单页滚动、头部随滚动收起、表单响应与字号

Category: enhancement
Status: in-progress
父票：[spec](../spec.md)
Blocked by: 无
出处：用户 2026-11-03 问能否参考 SAP Fiori；协调方列了三条（锚点栏 + 单页滚动、Dynamic Page 头部收起、表单响应规则 + 读出字号），用户裁「三条都做」。只搬版式与行为，不搬 Fiori 的皮（颜色、圆角、字体 token 照旧用本仓的）。

## 要做的

### 1. 模板加「分区」版式（`templates/DetailPageTemplate.tsx`，共享面）

- 新增 prop `layout?: 'tabs' | 'sections'`，缺省 `'tabs'`（现状，别的对象页一字不变）。`'sections'` 时：
  - 头区照旧（标题行、meta 行）；签不再是切换器，而是**锚点栏**：一行与现在签同形的条目（图标 + 词 + 计数，复用 `WorkspaceTabLabel` 与计数），`position: sticky; top: 0` 吸在模板自己的滚动容器顶部；点一条平滑滚到对应分区（滚动偏移扣掉锚点栏高度）；滚动时按视口里最靠上的分区高亮当前条（IntersectionObserver 或 scroll 监听，纯判读进 `.ts` 钉测试）。
  - 分区按 `workspaceTabOrder` 顺序全部渲染在一页上，每个分区：`<section id="section-<id>">` + 分区标题行（词 + 计数，13px 半粗，与卡标题条区分开——它是一级，卡是二级）+ 内容。调用方传进来的 `tabs[]` 内容原样成为分区内容；`sections`（模板自身的区块，如「需要处理」）仍归概要分区之前。
  - **可折叠分区**：`WorkspaceTabInput` 加可选 `collapsed?: boolean`——为真时分区默认折起，只显标题行与一句摘要（调用方给 `summary?: ReactNode`），点标题或锚点栏条目展开；展开后内容才挂载（重区块不白取数）。
  - `activeTab` / `onActiveTabChange` 在分区版式下的含义改为「当前锚点」：外部给的 id 滚到那一节；深链 `?tab=` 照旧可达。
  - 键盘：锚点栏条目是 `role="tablist"` 里的按钮或链接，Enter 滚过去；分区标题是 h2/h3 让读屏能跳。
- 头部随滚动收起（Dynamic Page）：滚动容器滚过一个阈值（事实行高度）后给头区加一个收起态——meta 行与事实行隐藏、标题行与主动作留着、锚点栏贴上去；回到顶部恢复。只在 `'sections'` 版式下做；动画只用 transition，不要布局抖动（用 `max-height`/`opacity` 或先量高度）。
- 不改 `'tabs'` 版式的任何字节；`templates/README.md` 或文件头写清两种版式各用于什么（对象页默认仍是 tabs，分区版式给主数据类对象）。

### 2. 公司主体与往来单位对象页切到分区版式（`pages/party/LegalEntityDetailPage.tsx`、`BusinessPartyDetailPage.tsx`）

- `layout="sections"`；分区：概要（当前资料读出 + 技术详情）、联系人、资料版本（**默认折叠**，摘要一句「第 N 版 · 2026-11-01 起生效 · 共 N 版」，展开后才挂 `LegalEntityProfileSection`）、登记记录（默认折叠，摘要「共 N 笔」）、关联（展开）。往来单位：概要、时间线（折叠）。
- 事实行照旧在头区（随滚动收起）。
- 侧边详情栏（`LegalEntityDetailPanel`、`BusinessPartyDetailPanel`）**仍用签**，不动。

### 3. 表单响应规则与读出字号（`legal-entity-profile-readout.css`、`LegalEntityProfileReadout.tsx`）

- 标签位置随卡宽：卡容器 < 28rem 时 `.legal-entity-readout-field` 改成标签在上、值在下（`grid-template-columns: 1fr`，标签 12px 弱色），≥ 28rem 标签在左照旧（容器查询，不用视口）。
- 读出基准字号提一档：值 14px、标签 13px、等宽 13px、卡标题 14px；行距随之（值 `leading-6`）；联系人卡、收款账户表同一尺度。详情栏（窄）与对象页同一份件，所以一起变——核一眼 559px 的详情栏不会溢出。

### 4. 核验

- 三道门；纯判读（当前锚点、滚动偏移、折叠态）钉 node:test。
- 浏览器 1440 与 2000：公司主体对象页 SYN-LE-CN-SH-01（锚点栏吸顶、点「收款账户」所在的概要 / 点「关联」滚过去、资料版本折叠展开、滚动后头部收起、回顶恢复）、往来单位对象页；运单对象页截一张证 `'tabs'` 版式一字未变；详情栏 559px 读出标签在上。
- 共享面评审：Spec 轴，协调方派。

## 不做

- 不切运单、清关单等其它对象页（另票逐页看过再切）；不改 Fiori 皮；不动 `components/*`、`shell/*`；不改侧边详情栏的签。

## 完成判据

- 三道门退 0；`'tabs'` 版式页面字节级不变（运单对象页截图对照）；公司主体对象页一页滚动、锚点栏吸顶并随滚动高亮、资料版本与登记记录折叠可展、头部滚动收起；窄卡标签在上；读出 14 / 13 / 13。
