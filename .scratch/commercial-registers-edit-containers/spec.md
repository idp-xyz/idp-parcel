# 商业各册的编辑容器统一：列表 + 右栏详情、新增走登记视图、收尾用对话框

Category: enhancement
Status: in-progress
出处：公司主体与往来单位两张对象页改成 Fiori 式之后（`.scratch/company-subject-section-edit`、`.scratch/business-party-edit-mode`），用户 2026-11-04 问「一次做掉，还是按业务顺序一个一个来」；会话建议「规格一次定，页面按业务顺序逐页落」，用户此前裁定「以后都是 Yes」。

## 现状（取证于 main `103dd5ed2`，子代理只读调研）

服务产品、客户与合同、供应商协议、商业规则四页同一骨架：列表工作台 + 「新增 / 发布」表单摆成列表旁边的一个签（`KeepAliveTabs` 保活，票 party-commercial-uiux/09），行点开右侧抽屉看详情与版本历史（`VersionDetailDrawer`，票 21），「收尾…」再开一层抽屉（`VersionClosureDrawer`，票 16），「发新版本」切到表单签并预填（票 14）。渠道产品目录页也用详情抽屉；审批与发布页用待发布抽屉（`PendingDraftDrawer`）。四页只有列表读口，没有按版本读单条的口；版本身份是（范围、对象、版本）三元组。

与公司主体 / 往来单位的差别，正是用户点名的「右边浮窗」问题：详情是浮窗、收尾是浮窗叠浮窗、新增藏在签里。

## 决定（一次定，各页照抄）

1. **行详情 = 工作台右栏**（`WorkbenchPageTemplate` 的 `renderDetail`，与公司主体、往来单位同一形态）。内容照抄 `VersionDetailDrawer` 的两签（详情列全这一行的列、版本历史）与头部动作（发新版本、收尾…），只换容器：抽屉 → 右栏面板 `VersionDetailPanel`。不发新请求，仍用页面已取回的列表（票 21 的口径）。方向键换行、保活、缓存随工作台来。
2. **新增 / 发新版本 = 登记视图**（照往来单位 `?register=…` 的做法）：从「列表旁边的签」改成由地址决定的整页视图，页头「返回列表」+ 标题，表单住在里面；有没提交的输入时离开先问。商业规则页已有 `?tab=publish&register=<册>` 的地址，照旧；其余三页的新增表单签改成同样由地址决定的视图（键名沿用各页已有的签名）。视图内的换册保活（票 09 的保活落在视图里，不再落在页签上）。「发新版本」预填与替换前确认（票 14 的 `useReplaceDraftGuard`）不变，只是落点从签变成视图。
3. **收尾 = 对话框**（`VersionClosureDialog`）：从右栏动作打开，打开时现问册面、认旧版与后继、确认框复述、答复原样示出——票 16 的逻辑一字不动，只换外壳（照公司主体的停用对话框）。
4. **审批与发布 = 列表 + 右栏**：待发布的那一份在右栏读回、批准与发布在右栏底部的动作行做，`PendingDraftDrawer` 退役。
5. **不做「每版一个对象页」**：没有按版本读单条的口，身份又是三元组，地址编码与读口都要先定；记为后续（要后端，归 `party-commercial-uiux-backend`）。

机制：容器与地址判读。产品策略：详情在右栏、新增在整页视图、收尾在对话框。租户取值：无。

## 落的顺序（业务依赖）

| 票 | 页 | 状态 |
|---|---|---|
| [01](./issues/01-service-products.md) | 服务产品（含抽出共享件 `VersionDetailPanel`、`VersionClosureDialog`、登记视图壳） | resolved（2026-11-04 进 main） |
| [02](./issues/02-party-contracts.md) | 客户与合同（两张表单：签订合同、新增客户账户） | resolved（2026-11-04 进 main） |
| [03](./issues/03-supplier-agreements.md) | 供应商协议 | in-progress |
| [04](./issues/04-commercial-policies.md) | 商业规则（九本册、已有地址） | draft |
| [05](./issues/05-channel-product-catalog.md) | 渠道产品目录（只换详情容器） | draft |
| [06](./issues/06-pending-publications.md) | 审批与发布（待发布抽屉退役） | draft |

每票一笔进 main、一次只读探针；共享件在票 01 抽出，后面的票只接线。碰 `templates/*` 才要非作者评审；本规格预计只碰 `pages/**`。
