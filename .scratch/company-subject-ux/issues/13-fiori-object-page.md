# 13 对象页照 Fiori Object Page：锚点栏单页滚动、头部随滚动收起、表单响应与字号

Category: enhancement
Status: resolved——2026-11-03 进 main（记录见文末）
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

## 完成记录（2026-11-03，分支 `claude-csux-13`，末笔代码 `e52c4f518`）

**落点**（四笔，按票面四件事分）：

- `b1482e642` 模板分区版式：`templates/DetailPageTemplate.tsx` 加 `layout?: 'tabs' | 'sections'`（缺省 `'tabs'`），`'sections'` 整条支路在分签支路之前提前返回，分签与叠 Card 两条支路一字未动。新件 `SectionsWorkspace`（头区）、`WorkspaceSections`（滚动容器 + 锚点栏 + 分区）、`SectionHeading`。锚点栏复用分签的 `Tabs` / `TabsList` / `TabsTrigger` 原语（条目 = `WorkspaceTabLabel` + 同一种计数），外包一层 `sticky top-0` 吸在模板自己的滚动容器顶上；受控 `value` 不接 `onValueChange`、条目各接 `onClick`（Radix 只在值变了才报，点已高亮的那条就滚不回它顶上），`activationMode="manual"`，方向键移焦、Enter / 空格滚；`aria-controls` 指到 `<section id="section-<id>" data-section-id>`，分区标题 h3 + `aria-labelledby`。滚动时按「顶边已过栏下沿的最后一个分区」高亮（scroll 监听，每次量各分区位置）；末尾垫一块高度让最后一个分区也能到顶（UI5 ObjectPageLayout 的 spacer 做法，否则点末尾短分区停半路、高亮落在前一个）。`WorkspaceTabInput` 加 `collapsed?` / `summary?`，`resolveWorkspaceTabs` 归并（collapsed 有一个真即真、summary 取第一个），折起只挂标题行与摘要。`activeTab` / `onActiveTabChange` 在分区版式下是「当前锚点」：外部给的 id 与上次报回去的不同才算要求（滚过去、折着的展开；挂载时记住的锚点直接跳不平滑），自己报回去的锚点经调用方 state 回流不再滚。纯判读进 `templates/object-page-sections.ts`（`sectionScrollTop`、`activeSectionAt`、`bottomSpacerHeight`、`sectionExpanded`、`sectionElementId`）+ `.test.ts`。
- `8649fe7da` 头部收起：`SectionsWorkspace` 记收起态，meta 行与指标带各包一层 `CollapsibleRows`（grid 行 `1fr → 0fr` + 透明度过渡，只用 transition、不量高度；收起时 `inert`），`WorkspaceSections` 滚动时按 `headerCollapsedAfterScroll` 报上来：展开的头滚过阈值（meta 行 + 指标带此刻的高度）收起、收起的头只在回到页首恢复（两条线不同，头矮了容器长高 scrollTop 被夹回去也不抖）；非 ready 态与卸载时一律展开。只在 `'sections'` 下做。
- `3468e3626` 两页：`LegalEntityDetailPage.tsx` `layout="sections"`，资料版本 `collapsed: true` + 摘要「第 N 版 · 起生效 · 共 N 版」（`profileVersionsSummary`，缺哪格不说哪格），登记记录 `collapsed: true` + 「共 N 笔」，关联 `collapsed: false`（可折、默认展开）；登记记录的内容补了与其它分区同样的白底 `section` 外框。**页头「更新公司资料」改开本页自己挂的 `LegalEntityProfileDrawer`**（摆在 headerActions 里，关着不占 DOM）：资料版本分区折着时内容没挂，经 `formOpenRequest` 转的打开要求到不了；页头抽屉落册后按 `headerLanded` 钥匙重挂分区让版本历史重读，分区自己的抽屉落册不转这把钥匙（刚保存完的抽屉不能随分区卸掉）。`BusinessPartyDetailPage.tsx` `layout="sections"`，时间线 `collapsed: true` + 摘要「当前第 N 版」、补白底外框；概要卡 `basicTitle` 由「概要」改「登记依据」——分区标题已是「概要」，卡再叫概要是上下两行同一个词。
- `e52c4f518` 第 3 节：`legal-entity-profile-readout.css` 读出卡成容器 `legal-entity-readout-card`，`@container (width < 28rem)` 时 `.legal-entity-readout-field` 一列、`dt` 12px / 1.25rem 可折行；`LegalEntityProfileReadout.tsx` 卡加容器类、卡标题 14、extra 13，`ProfileReadoutField` / `ProfileReadoutAddresses` / `ProfileReadoutPairs` 标签 13 / 值 14 / 等宽 13、行高 6；`LegalEntityProfileContent.tsx` 收款账户表 14 / 表头与次要行 13 / 等宽 13、维护提示 13；`LegalEntityContactCards.tsx` 人名与各行 14、所属与平台名 13、账号等宽 13；`LegalEntityContactsSection.tsx`、`LegalEntityProfileSummary.tsx` 小字 12 → 13。**落点之外**：后四个文件票面第 3 节点了名的「联系人卡、收款账户表同一尺度」落在它们身上；`components/*`、`shell/*` 未碰；`templates/*` 只动了票面点名的两份加新建的判读件。

**判据**（浏览器核验走 vite `:5214` 接 `:19080`，`VITE_DISPLAY_TIME_ZONE=Asia/Shanghai`，脚本断言零写请求、零页面错误；截图与探针日志在本会话 scratchpad `csux13/`：`w1440-*` / `w2000-*`（改后）、`final1440-*`（末笔 tip 上重跑）、`before-*` / `final-*`（`'tabs'` 对照）、`w1440-narrow-card-26rem.png`，数值在 `*-log.txt`）：

- 三道门退 0 ✅：`e52c4f518` 上 tsc、run-tests 1178（新增 7：`object-page-sections.test.ts` 6 条 + `workspace-tabs.test.ts` 归并 1 条）、vite build；`git merge origin/main` 为 Already up to date（main 仍在 `8c3b04391`）。
- `'tabs'` 版式字节级不变 ✅：改前（main 态起的同一份 vite）与改后、以及末笔 tip 上，运单对象页 `#/shipment-request-inquiry/EBAIYXVMVZQJ76X4P5AY6AKQ3E`（`tabs={[]}`，19357 字节）与模板演示页「对象工作区」（7 签、25771 字节）的 `main` `innerHTML` 用 `cmp` 比对一字节不差；截图 `before-shipment.png` / `final-shipment.png`、`before-demo.png` / `final-demo.png`。源码上分签支路也没动过一行（`git diff 8c3b04391..e52c4f518 -- apps/admin-web/src/templates/DetailPageTemplate.tsx` 的改动只在 import、`layout` prop、提前返回那一段与文末新件）。
- 公司主体对象页一页滚动、锚点栏吸顶并随滚动高亮 ✅（1440 与 2000 数值一样）：页首 scrollTop 0 栏顶 = 滚动容器顶（182）；滚到 300 与 1726 栏顶仍 = 容器顶（138）。点「关联」→ scrollTop 1726，关联分区顶边 174 = 容器顶 138 + 栏高 36，高亮「关联」；键盘：聚焦「概要」→ ArrowRight → Enter，高亮「联系人」、分区顶边 174。
- 资料版本与登记记录折叠可展 ✅：初始两分区各只挂标题行（children 1，`aria-expanded=false`）、摘要「第 3 版 · 2026-11-01 起生效 · 共 3 版」「共 1 笔」；点锚点「资料版本」→ 展开（children 2，`LegalEntityProfileSection` 此时才挂、才取数）并滚到顶（174）；点分区标题折回。页头「更新公司资料」→ 抽屉打开（dialog 1）、锚点拨到资料版本；只开不填，Esc 关。
- 头部滚动收起 ✅：meta 行（含事实行）44px → 滚到 300 时 0px / 透明度 0，头区 99 → 55px，栏贴到头区下沿；回顶 0 恢复 44px / 99px。往来单位页 meta 21px + 指标带 127.5px 同样收起 / 恢复；时间线折起、点锚点展开（修订链此时才取）。
- 窄卡标签在上 ◑：规则生效——把读出卡限到 26rem 时字段一列（`grid-template-columns` 382px）、`dt` 12px/20px 在值上方（`dtTop` 384 → `ddTop` 406）；但**详情栏 1440 宽时卡就是 559px = 35rem，宽于 28rem，标签仍在左**（120px 列），2000 宽时 839px 同。票面第 4 节「详情栏 559px 读出标签在上」与第 3 节的 28rem 阈值不能同时成立，按第 3 节做了；要 559 也标签在上得把阈值提到 36rem 左右（对象页 888px 的卡不受影响）。详情栏不横向溢出（`overflowX` false）。
- 读出 14 / 13 / 13 ✅：计算字号 `dt` 13px/24px、`dd` 14px/24px、卡标题 14px/20px、清单右列 14 / 等宽 13、收款账户表 14 / 等宽 13（对象页与 559 详情栏同）。
- 分区标题 ◑：票面写 13px 半粗；做成 **15px 半粗**（与联系人区既有小标题同一档）+ 横贯整列的下划线、无底色。理由：第 3 节同时把卡标题提到 14，一级 13 会比二级 14 小；Fiori 自己也是 section 大于 subsection。要回 13 是 `SectionHeading` 一处改。

**残留 / 未验**：暗色主题未看；`prefers-reduced-motion` 那条路（直接跳不平滑）未实测；`aside` 右栏在分区版式下没有页面在用、未验；两张对象页同时挂在工作区里时 `section-<id>` 会重复（滚动与高亮只在各自滚动容器里查、不受影响，`aria-controls` 会指到先出现的那一个）；内容刚够滚一点时头收起后 scrollTop 被夹到几像素、要回到 0 才恢复（按设计）；CI `admin-web` job 在分支上未看。票面「深链 `?tab=` 照旧可达」：仓里这两张对象页今天没有 `?tab=` 深链（壳层 hash 只到 `<模块>/<对象>`，签靠 `activeTab` prop 与工作区记住的 `useTabViewState`），受控 `activeTab` 拨到哪一节就滚到哪一节已验（页头动作、记住的锚点），没有另造深链。

**要协调方裁的**：分区标题 15 还是 13；窄卡阈值 28rem（则 559 详情栏标签在左）还是提到 36rem；页头「更新公司资料」改走页面级抽屉这一变（`LegalEntityProfileSection` 的 `formOpenRequest` 口留着给详情栏用）是否接受；往来单位概要卡改名「登记依据」是否接受；共享面（`templates/*`）评审 Spec 轴由协调方派。

## 评审与进 main 记录（2026-11-03，协调方）

- 共享面 Spec 轴隔离评审钉 `ceb9cf9d3`：阻断一条——往来单位概要卡 `basicTitle="登记依据"` 把 spec 用语表退役的词重上界面，协调方改「来源单据」。非阻断四条：内容不满一屏仍垫高（改为不垫）；折起即卸载未写进注释（已写）；联系人分区内层 h3 与分区标题同档（降为 h4 14px）；锚点栏用 `role=tab` 指向非 tabpanel 的分区——可过，更对的是 `nav` + 链接，另票。
- 评审裁决：分区标题 15px 半粗（卡标题 14，一级不能小于二级）；窄卡阈值 28rem，票面第 4 节「559 标签在上」改为「559 标签在左、不溢出」。
- 作者请裁 ③（页头主动作改走页面级抽屉、往来单位概要卡改名）——抽屉接受，改名按评审改回「来源单据」。
- main `8c3b04391` 上 `git merge --no-ff origin/claude-csux-13`（tip `ceb9cf9d3`），无冲突，协调方再加一笔评审修订。三道门在 main 上退 0（tsc、run-tests 1178、vite build）；`'tabs'` 版式按作者的 innerHTML 三次对比字节级不变。
- 其它对象页（运单、清关单等）切到分区版式归后续票，逐页看过再切。
