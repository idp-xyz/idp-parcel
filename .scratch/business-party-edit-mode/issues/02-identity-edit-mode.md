# 02 名称更正 = 对象页编辑态

Category: enhancement
Status: resolved——2026-11-04 进 main `c5bf8c4c5`（记录见文末）
父票：[spec](../spec.md)
Blocked by: 01

## 要做的

1. 地址：`#/business-parties/<id>?edit=identity` 即编辑态（`business-parties-address.ts`，照 group-legal-entities-address 的 profileEditHash 一套）。
2. 页头「更正名称」进编辑态：概要分区换成表单（名称、来源单据、生效时间；高级：更新序号；编码锁死显示），「保存 / 取消」经 portal 钉进模板页脚；落册后 toast、回显示态、重取参与方册。壳与草稿沿用 `useRegistrationForm` + `business-party-form.ts`，不另造载荷。
3. 离开守卫：返回列表与站内链接有改动先问。详情栏不加入口（名称更正低频），对象页一处即可。

## 完成判据

- 三道门退 0；地址判读有 node:test。
- 浏览器核：进编辑态、页脚保存、预览 / 答复、取消与返回列表的确认、深链刷新仍在编辑态、查看者无入口。

## 完成记录（2026-11-04，直接在 `main` 上做，自 `3f622278d`，提交 `c5bf8c4c5`，CI run 41777645542 success）

- 落点：`business-parties-address.ts` 加 `identityEditRequested` / `identityEditHash`（测试钉着）；`BusinessPartyIdentityEditMode.tsx`（新）：壳与载荷仍是 `useRegistrationForm` + `business-party-form.ts`，编码锁死、名称预填、生效时间预填此刻，动作行经 portal 进页脚；`BusinessPartyDetailPage.tsx` 收 `identityEditing` / `onEnterIdentityEdit` / `onExitIdentityEdit`，编辑态下概要分区换成表单、页头按钮收起、返回列表有改动先问；`BusinessPartiesPage.tsx` 照登记视图压历史并同步 hash 状态。
- ✅ 三道门退 0：tsc、node:test 1195/1195、vite build；CI `admin-web` job success。
- ✅ 浏览器在演示租户核（playwright 只读探针，零写请求、零页面错误）：页头「更正名称」后地址 `…/SYN-PARTY-CN-SZ-01?edit=identity`，页头只剩「返回列表」，编码只读、名称预填「IDP供应链（深圳）有限公司（演示）」，「保存」在页脚、全景仍可见；改名后「返回列表」弹「放弃修改？」，「继续编辑」留下；「取消」→「放弃」后地址去掉编辑键、页头按钮回来；深链 `?edit=identity` 刷新仍在编辑态；未改动时「返回列表」直接回到列表且对象页表单已卸。查看者账号：深链无表单、页头无「更正名称」。
- 评审：diff 全在 `pages/**`，按 workflow「前端切片」第 5 条自审。证据 S。
