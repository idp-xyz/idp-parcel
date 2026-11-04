# 02 管理台：「操作者与授予」页

Category: enhancement
Status: resolved——2026-11-04 进 main（本笔提交，紧随票 01）
父票：[spec](../spec.md)
Blocked by: 01（契约已定，可并行开发；联调等 01 落地）

## 要做的

1. 导航「设置」组加 `operator-access`「操作者与授予」（扫描设备之后）；`moduleInfoById` 一条。
2. `pages/operator-access/`：`api.ts`（四个口）、`presentation.ts`（能力面中文、授予状态判读：生效中 / 未到 / 已到期 / 已撤销，带 node:test）、`OperatorAccessPage.tsx`（`WorkbenchPageTemplate` + 右栏 `OperatorDetailPanel`：绑定事实、授予表、「授予…」就地表单、每条「撤销…」对话框）、`?tab=register` 登记视图「新增操作者」（`RegisterView`，发行方预填自读口）。
3. 授予自查加一格：`heldGrantsOf` 认 `OPERATOR_ACCESS_ADMINISTRATION`，`useOperatorAccessAdministration()`；没有的人导航项照摆、页内不摆写入口与右栏动作（与 ADR-0315 同款），读口答 403 时按状态区显示。
4. 本页读口答未授予时，状态区显示本人发行方与主体标识（从 ID token 的 `iss` / `sub` 解出，只显示不上送）；管理台不另设整站拒绝页。

## 完成判据

- 三道门退 0。
- 浏览器核（演示租户，配置操作者持管理授予）：列出两个操作者；右栏授予表与状态；新增一个合成操作者、给它授予查阅读、撤销之；撤销自己的管理授予答防锁死；查看者无入口。

## 完成记录（2026-11-04，直接在 `main` 上做）

- 落点：`pages/operator-access/`（`api.ts` 四个口，写口答到后整域作废；`presentation.ts` 能力面中文、授予状态判读、检索串，带 node:test；`OperatorAccessPage.tsx` 工作台 + 右栏 `OperatorDetailPanel`（绑定事实、授予列表与状态胶囊、「授予…」就地表单、每条「撤销…」对话框）+ `?tab=register` 登记视图「新增操作者」）；`operator-grants.ts` / `use-operator-grants.ts` 多认 `OPERATOR_ACCESS_ADMINISTRATION`（`useOperatorAccessGrant`）；导航「设置」组加「操作者与授予」，`page-registry` 登记。读口答未授予时状态区显本人发行方与主体标识（从 ID token 声明解出，只显示）。
- ✅ 三道门退 0：tsc、node:test 1203/1203、vite build；`go test ./internal/architecture/` 的管理台路径门禁过。
- ✅ 浏览器核（演示租户，land 构建旁路实例 19081 + vite 5175，演示库已施加迁移 0040、配置操作者经 CLI 登了管理授予）：列出操作者；「新增操作者」视图发行方预填，登一个合成操作者答已登记、回目录后列表多一行（写口整域作废，不手动重取）；右栏「授予…」给它登主数据与运营查阅读答已登记、列表里立刻显生效中；「撤销…」对话框撤之答已登记、状态变已撤销；换成运营决定时决定种类下拉来自读口词表；撤销自己的操作者接入管理授予答「不能撤销你自己此刻生效的操作者接入管理授予」；重放同一操作者答已在册、同标识改起点答内容冲突。查看者进本页：读口答未授予，状态区显出本人主体标识，页头无「新增操作者」。
- 评审：与票 01 同一份非作者评审（APPROVE），前端两处建议已照改（发行方只读；撤销框关闭时重置在途标记；落册后不再手动重取，靠整域作废）。证据 S。
- 演示库里留下的探针操作者（`SYN-PROBE-SUBJECT-01/02`）与其授予已用 SQL 删掉。
