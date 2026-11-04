# 06 审批与发布：待发布那一份进右栏，批准与发布在右栏动作行

Category: enhancement
Status: resolved——2026-11-04 进 main `56d07fbb2`（记录见文末）
父票：[spec](../spec.md)
Blocked by: 01

## 要做的

照 spec 的决定 1–4，用票 01 抽出的 `VersionDetailPanel`、`VersionClosureDialog`、`RegisterView` 接线；本页特有的东西（表单、选择器、地址键）保持。

## 完成判据

- 三道门退 0。
- 浏览器核：右栏详情、登记视图进出与离开守卫、收尾对话框、查看者无写入口。

## 完成记录（2026-11-04，直接在 `main` 上做，自 `3d44ed40b`，提交 `56d07fbb2`，CI run 41804401560 全绿）

- 落点：`PendingPublicationsPage.tsx` 改在 `WorkbenchPageTemplate` 上（右栏开时只留编号、版本、状态；状态胶囊筛在筛选区）；`PendingDraftPanel.tsx` 从 `PendingDraftDrawer` 换容器而来，读回、批准、发布与发布确认框的逻辑一字不动，动作移到栏底动作行，没有写授予的人看到一句说明；按载体身份给 key，换行整个重挂。发布后那一行离开列表时钉在表尾（空态也按摆出来的行数判），栏不跟着关，人看完答复再走。`VersionDetailDrawer`、`PendingDraftDrawer` 删除，`VersionClosureDrawer` 只剩本体改名 `VersionClosureBody`。
- 票面写的 `VersionDetailPanel`、`VersionClosureDialog`、`RegisterView` 本页用不上：这页没有版本历史、收尾与登记，照 spec 决定四只做「列表 + 右栏」。
- ✅ 三道门退 0：tsc、node:test、vite build；CI run 41804401560 全部 job success。
- ✅ 浏览器核（playwright 只读探针，零写请求、零页面错误）。演示租户没有在途改动，页面只能核到空态；右栏用 Playwright 在浏览器侧拦截两个只读口（列表与读回）灌两份合成载体核：两行与三枚胶囊筛，点开右栏显事实、提交的内容与技术详情、栏底「批准」；方向键换到已批准那一行，栏底变「发布」，点开出「发布这一版？」确认框后「先不发布」；点刷新让列表答空，选中那行钉住、栏仍开，关栏后表空。查看者右栏无按钮、显「没有批准与发布的权限」。批准与发布的真实写路径未在浏览器里走。
- 评审：diff 全在 `pages/**`，按 workflow「前端切片」第 5 条自审。证据 S。
