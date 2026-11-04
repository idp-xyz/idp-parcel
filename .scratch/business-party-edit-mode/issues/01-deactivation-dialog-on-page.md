# 01 停用留在本页：对象页与详情栏「更多 › 停用」开本页对话框

Category: enhancement
Status: resolved——2026-11-04 进 main `e6402095b`（记录见文末）
父票：[spec](../spec.md)
Blocked by: 无

## 要做的

1. `BusinessPartyDeactivationDialog.tsx`（新）：照 `LegalEntityDeactivationDialog` 的做法，表单仍是 `IdentityDeactivationForm`，种类与对象锁死为这家往来单位；参与方册的答案由页面传进来（候选与版本号建议用）；标题带单位名，确认框里叫「往来单位」。
2. `BusinessPartyMoreMenu` 自己挂对话框：收 `parties` 与 `onChanged`，落册后关对话框并让页面重取参与方册；不再往列表页的停用深链送。
3. 详情栏与对象页接上：详情栏传列表页持有的参与方答案与 retry；对象页传 `listAnswer` 与 `retry`。
4. 列表页登记视图里的停用表单与别处来的停用深链不动。

## 完成判据

- 三道门退 0。
- 浏览器在演示租户核：对象页与详情栏「更多 › 停用这家单位…」都在本页弹对话框，种类与对象两格不摆、标题带单位名；取消关掉不留痕；查看者账号菜单里无停用项。

## 完成记录（2026-11-04，直接在 `main` 上做，自 `caf7444bc`，提交 `e6402095b`，CI run 41776841881 的 `admin-web typecheck, tests, build` success）

- 落点：`BusinessPartyDeactivationDialog.tsx`（新）；`BusinessPartyHeaderParts.tsx` 的 `BusinessPartyMoreMenu` 收 `parties` / `onChanged` 并自挂对话框；`BusinessPartyDetailPanel.tsx`、`BusinessPartiesPage.tsx`、`BusinessPartyDetailPage.tsx` 接线；列表页里只剩定义的 `openDeactivation` 与 `deactivationLinkHash` 引用删掉。
- ✅ 三道门退 0：tsc、node:test 1194/1194、vite build。
- ✅ 浏览器在演示租户核（playwright 只读探针，零写请求、零页面错误）：详情栏「更多 › 停用这家单位…」在本页弹「停用往来单位 IDP供应链（深圳）有限公司（演示）」，地址仍是列表 `#/business-parties`，表单只有版本号、来源单据、停用时间三格，没有「停用什么 / 停用哪一个」；「关闭」后对话框不留；对象页同样；Esc 可关。查看者账号菜单只有「在新标签打开对象页 | 复制编码」。
- 评审：diff 全在 `pages/**`，按 workflow「前端切片」第 5 条自审。证据 S。
