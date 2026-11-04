# 02 客户与合同：行详情进右栏、签订合同与新增客户账户走登记视图、收尾用对话框

Category: enhancement
Status: resolved——2026-11-04 进 main `e778a775c`（记录见文末）
父票：[spec](../spec.md)
Blocked by: 01

## 要做的

照 spec 的决定 1–4，用票 01 抽出的 `VersionDetailPanel`、`VersionClosureDialog`、`RegisterView` 接线；本页特有的东西（表单、选择器、地址键）保持。

## 完成判据

- 三道门退 0。
- 浏览器核：右栏详情、登记视图进出与离开守卫、收尾对话框、查看者无写入口。

## 完成记录（2026-11-04，直接在 `main` 上做，自 `b2c9c5159`，提交 `e778a775c`，CI run 41793544089 全绿）

- 落点：`party-contracts-address.ts`（新，测试钉着；四种视图，离开登记视图回对应的列表）；`PartyContractsPage.tsx` 合同目录换到 `WorkbenchPageTemplate` + `VersionDetailPanel` + `VersionClosureDialog`，两张表单成登记视图，客户账户册照旧用 `ListPageTemplate`、页头加「新增客户账户」，两本列表换签只替换地址。
- ✅ 三道门退 0：tsc、node:test 1197/1197、vite build；CI 全绿。
- ✅ 浏览器在演示租户核（playwright 只读探针，零写请求、零页面错误）：签「客户合同 | 客户账户」，页头「签订合同」；点行右栏「版本 SYN-CONTRACT-01 v1」，动作「签新版本」，详情按列竖排含客户列；「签新版本」后地址 `?tab=publish-contract`、标题「签订合同」、预填提示在；改一格返回弹「离开这一页？」，离开回合同签；账户签地址 `?tab=accounts`、页头「新增客户账户」进 `?tab=register` 视图、返回回到账户签；深链落在视图。查看者：`?tab=register` 深链清成 `?tab=accounts`，无写入口。
- 评审：diff 全在 `pages/**`，自审。证据 S。
