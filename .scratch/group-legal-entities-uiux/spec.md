# 集团与法人页 UI/UX 评估与跟进

Category: enhancement
Status: in-progress

出处：2026-10-16，用户在会话里要求「作为 UI/UX 专家评估集团与法人的 UI/UX」，并按 `/to-tickets` 拆了票。拆分、主色范围、集团显示和父 spec 这几项都由用户在 Step 4 认可。

## 评估方法

- 取证时的提交是 `858280a2`。`apps/admin-web` 从这里到 `f33d849d` 都没有改动。
- 演示环境用的是演示租户 `SYN-TENANT-01`。起法见 `scripts/demo-seeds/README.md` 的「操作者渠道」一节，以及 `apps/admin-web/README.md` 的「与后端联调」一节。
- 用配置员账号 `syn-operator-config-01` 登录，在真浏览器（Playwright Chromium）里走了这几个视图：列表、详情栏三签、对象页两签、登记视图。
- 宽度试了 1440、1280、1024 三档，主题试了浅色和深色。
- 用 axe-core 4.10 按 WCAG 2 AA 扫了一遍。
- 截图只存在会话的临时目录里，没有入库。各张票都写了复现步骤，需要时照着重拍。

**这次没覆盖到的：**
- 演示租户只有一个责任法人，所以多行时的密度，以及待补、已登记未生效、已停用这几种行都没看到。
- 没有提交任何表单，因为会写进共享的演示库。
- 没用查阅员账号看只读体验。

## 做得好的（保持）

- 页面状态如实。接入那一格分三态：未配置、需要登录、没有授予。「筛选后没有匹配」和「册为空」分开呈现。计数只在拿到业务答案后才显示。
- 工作台操作效率高：点表头排序，点 KPI 就筛选，单击一行开详情栏；`/` 跳到检索，Esc 收栏，↑↓ 换行；引用签点一下复制；详情栏打开时次要列会自动收起。
- 时区标注明确，深色主题完整，1024 宽下没有横向滚动。

## 发现与去向

| 发现 | 去向 |
|---|---|
| 身份修订和资料修订都写 `rN`，同一屏同时出现「r1」和「生效修订 r2」 | [01](issues/01-identity-and-profile-revisions-named-apart.md) |
| 表单标签后面挂着载荷路径 | [02](issues/02-form-labels-drop-payload-paths.md) |
| 法人两张表单：说明框里有端点，几乎每格都重复「由服务端判」，首笔登记也显示「身份更正依据」，字段分组把身份格拆散了 | [03](issues/03-legal-entity-forms-regrouped-and-rewritten.md) |
| 登记视图不在地址里，刷新会丢掉草稿，后退也回不到表单 | [04](issues/04-registration-view-is-addressable.md) |
| 主动作是灰底灰字，看起来像禁用；筛选胶囊反而是实心蓝 | [05](issues/05-primary-action-style-on-this-page.md)（本页）、[12](issues/12-primary-action-style-on-remaining-pages.md)（其余页） |
| 详情栏里同一事实重复两三次；生命周期条带编号，让停用看起来像下一步 | [06](issues/06-detail-panel-header-slimmed.md) |
| 打开过法人资料签之后，再点「登记资料修订」，表单不会滚进视野 | [07](issues/07-profile-form-scrolls-into-view.md) |
| 9–10px 的小字、次要文字对比度略低于 4.5:1、进度条没有名称 | [08](issues/08-page-accessibility.md) |
| 对象页和详情栏是两套结构 | [09](issues/09-object-page-matches-detail-panel.md) |
| 1024 宽下事实条、徽章、引用签断行，KPI 悄悄消失 | [10](issues/10-narrow-width-layout.md) |
| 页面叫「集团与法人」，却看不到集团 | [11](issues/11-session-tenant-read-face.md) |

axe 在 `858280a2` 上的实测（只计 WCAG 2 AA 违规节点数）：
- 列表：color-contrast 17、button-name 1、aria-progressbar-name 1、scrollable-region-focusable 1。
- 详情栏：color-contrast 38，其余三项同列表。
- 登记视图：color-contrast 4、button-name 2。

## 不立票及理由

- **注册号类型照原码显示**（如 `USCC`，不显示「统一社会信用代码」）：这是有意为之，见 `legal-entity-identity.ts` 文件头「照答复原样」那段。目录会修订，拿今天的目录去译历史修订上的码可能译错。
- **两条修订历史排序方向相反**：身份修订按升序排，是 `revision-timeline.ts` 文件头「序照读口交回的修订号升序」的取舍，从上往下读就是登记 → 更正 → 停用这条链。资料修订按从新到旧排。01 只让两条历史分开命名，方向不改。
- **KPI、状态胶囊和右端计数数的是同一件事**：这是票 [admin-web-group-legal-entities/15](../admin-web-group-legal-entities/issues/15-legal-entities-follow-purchase-order-workbench.md) 按用户令「完全参考采购订单」做的形态，不动。06 也只在这套形态里面去重。
- **列表时间每格都带秒和 UTC+8**：时间呈现是各册页共用的取舍，不在本页单独改。

## 后续（不在本批，外壳或上游）

- 标签页关闭按钮 × 没有可访问名称（axe button-name）。
- 底部状态栏「Comfortable」字样的对比度只有 1.05:1。

## 子票

01–10、12 只改 `apps/admin-web`，走 [workflow.md「前端切片」](../../docs/agents/workflow.md#前端切片一人在-main-上直接做)，一个人在 `main` 上直接做。11 要碰 Go，走[并行会话](../../docs/agents/parallel-sessions.md)。

| # | 票 | 阻塞于 | 状态 |
|---|---|---|---|
| 01 | [身份修订与资料修订分开命名](issues/01-identity-and-profile-revisions-named-apart.md) | — | resolved |
| 02 | [表单标签不再挂载荷路径](issues/02-form-labels-drop-payload-paths.md) | — | resolved |
| 03 | [法人两张表单重排、说明改写](issues/03-legal-entity-forms-regrouped-and-rewritten.md) | 02 | resolved |
| 04 | [登记视图有自己的地址](issues/04-registration-view-is-addressable.md) | — | in-progress |
| 05 | [主动作用主色（本页）](issues/05-primary-action-style-on-this-page.md) | — | ready-for-agent |
| 06 | [详情栏头部瘦身](issues/06-detail-panel-header-slimmed.md) | — | ready-for-agent |
| 07 | [资料表单展开后滚进视野](issues/07-profile-form-scrolls-into-view.md) | — | ready-for-agent |
| 08 | [本页可访问性](issues/08-page-accessibility.md) | — | ready-for-agent |
| 09 | [对象页与详情栏同一套结构](issues/09-object-page-matches-detail-panel.md) | 01、06 | ready-for-agent |
| 10 | [1024 宽下不折坏](issues/10-narrow-width-layout.md) | 06、08 | ready-for-agent |
| 11 | [本会话租户读口：顶栏作用域与本页页头接上](issues/11-session-tenant-read-face.md) | — | ready-for-agent |
| 12 | [其余页面主动作改用主色](issues/12-primary-action-style-on-remaining-pages.md) | 05 | ready-for-agent |
