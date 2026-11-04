# 01 服务产品：行详情进右栏、新增走登记视图、收尾用对话框（含抽出共享件）

Category: enhancement
Status: in-progress
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
