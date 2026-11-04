# 01 服务产品：行详情进右栏、新增走登记视图、收尾用对话框（含抽出共享件）

Category: enhancement
Status: resolved——2026-11-04 进 main `b2c9c5159`（记录见文末）
父票：[spec](../spec.md)
Blocked by: 无

## 要做的

1. `VersionDetailPanel`（新，从 `VersionDetailDrawer` 抽）：同样的 props，渲染成右栏面板（头部：标题、状态、动作、关闭；两签：详情、版本历史）。抽屉件保留给还没改的页，最后一页改完再删。
2. `VersionClosureDialog`（新，从 `VersionClosureDrawer` 抽）：`ClosureBody` 原样复用，外壳换成对话框。
3. 登记视图壳 `RegisterView`（新）：页头「返回列表」+ 标题 + 副题，有没提交的输入时离开先问；地址键沿用各页的签名。
4. 服务产品页：`versions` 签的列表改用 `renderDetail` 开右栏；`publish` / `register` 两签改成登记视图（地址 `?tab=publish` / `?tab=register`，进出照往来单位的历史收口）；「发新版本」落到登记视图并预填；「收尾…」开对话框。

## 完成判据

- 三道门退 0；地址判读有 node:test。
- 浏览器核：点行右栏出详情与版本历史；「发新版本」进登记视图且已预填，返回有改动先问；「收尾…」弹对话框并现读册面；查看者无写入口。

## 完成记录（2026-11-04，直接在 `main` 上做，自 `87e20d4ab`，提交 `b2c9c5159`，CI run 41791212112 全绿）

- 落点：`VersionDetailPanel.tsx`（新，`HistoryList` 抽出与抽屉共用）、`VersionClosureDialog.tsx`（新，`ClosureBody` 从抽屉抽出并去掉 Drawer 专用外壳）、`RegisterView.tsx`（新）、`service-products-address.ts`（新，测试钉着）；`ServiceProductsPage.tsx` 从 `ListPageTemplate` 换到 `WorkbenchPageTemplate`，行上「发新版本 / 收尾」收进右栏头部，两张表单成登记视图，查看者落到视图地址授予答出后清键；`ServiceProductPublicationForm.tsx` 两处「签」改「视图 / 里」。
- ✅ 三道门退 0：tsc、node:test 1196/1196、vite build；CI 全绿。
- ✅ 浏览器在演示租户核（playwright 只读探针，零写请求、零页面错误）：页头「登记服务形态 | 新增产品版本」；点行右栏出「版本 SYN-PROD-CN-SG-ECON v1」，动作「发新版本」，两签「详情 | 版本历史」，详情按列竖排；右栏「发新版本」后地址 `?tab=publish`、整页登记视图标题「新增产品版本」、预填提示在、列表不在；改一格「返回列表」弹「离开这一页？」，「继续填写」留下，「离开」后回到目录；「登记服务形态」进视图、无改动直接返回；深链 `?tab=publish` 落在视图。查看者：深链被清成 `#/service-products`，页头无写入口，右栏无动作。
- ◑ 演示数据每个产品只有一版，没有可收尾的行，收尾对话框只有代码路径（ClosureBody 逻辑与抽屉同一份，未动）。
- ◑ 右栏里 ↑/↓ 换行未见生效（探针点行后按 ↓ 栏没换），是工作台模板的键盘行为，与本票无关，另查。
- 评审：diff 全在 `pages/**`，自审。证据 S。
