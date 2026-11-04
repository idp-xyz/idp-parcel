# 03 供应商协议：行详情进右栏、签订协议走登记视图、收尾用对话框

Category: enhancement
Status: resolved——2026-11-04 进 main `5aae1ad23`（记录见文末）
父票：[spec](../spec.md)
Blocked by: 01

## 要做的

照 spec 的决定 1–4，用票 01 抽出的 `VersionDetailPanel`、`VersionClosureDialog`、`RegisterView` 接线；本页特有的东西（表单、选择器、地址键）保持。

## 完成判据

- 三道门退 0。
- 浏览器核：右栏详情、登记视图进出与离开守卫、收尾对话框、查看者无写入口。

## 完成记录（2026-11-04，直接在 `main` 上做，自 `e778a775c`，提交 `5aae1ad23`，CI run 41795151722 全绿）

- 落点：`SupplierAgreementsPage.tsx` 整页改在 `WorkbenchPageTemplate` 上（`SupplierAgreementWorkbench`，列按 id 搬成 key，右栏开时收起次要列），行详情用 `VersionDetailPanel`，收尾用 `VersionClosureDialog`；「签订协议」是 `?tab=publish` 的 `RegisterView`，进视图压历史、出视图后退或替换，表单有未提交输入时「返回列表」先问；`supplier-agreements-address.ts` 新增视图键的读写与测试。
- ✅ 三道门退 0：tsc、node:test、vite build；CI run 41795151722 全部 job success。
- ✅ 浏览器在演示租户核（playwright 只读探针，零写请求、零页面错误）：右栏详情含详情 / 版本历史两签；「签新版本」进登记视图并显示预填说明、工作台不在；改一格后「返回列表」弹「离开这一页？」，「继续填写」留在视图，「离开」回列表；页头「签订协议」进视图、未改动返回不问；深链 `?tab=publish` 直落视图。查看者深链落回列表，页头无「签订协议」，右栏无写动作。演示数据各协议只有一版，收尾对话框没有可收尾的行可核（与票 01 同）。
- 评审：diff 全在 `pages/**`，按 workflow「前端切片」第 5 条自审。证据 S。
