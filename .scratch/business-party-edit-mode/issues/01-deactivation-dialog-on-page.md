# 01 停用留在本页：对象页与详情栏「更多 › 停用」开本页对话框

Category: enhancement
Status: in-progress
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
