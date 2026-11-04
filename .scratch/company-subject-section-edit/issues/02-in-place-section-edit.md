# 02 分区编辑改为就地：卡体变表单、卡底保存，抽屉退回只装整张表单

Category: enhancement
Status: resolved——2026-11-04 进 main `ab5727f69`（记录见文末）
父票：[spec](../spec.md)
Blocked by: 01

## 出处

用户 2026-11-04 问「现在我们的编辑都是右边浮窗的方式，你觉得专业吗，能参考 SAP Fiori 的做法吗」。会话判断：浮窗本身是正规模式，问题在用法——工作台已是「列表 + 右栏详情」，抽屉再从右边盖上来是三层右侧面板；整张表单塞进浮窗；改的时候看不见原值。参考 Fiori 对象页的显示态 / 编辑态就地切换与底部动作条，建议先把分区编辑改成就地，用公司主体看效果；用户答「按你的建议开始吧」。

## 要做的

1. **就地编辑器**（`LegalEntityProfileSectionEditor.tsx`）：点卡上「编辑」「添加…」，这张卡的卡体从读出变成只装这一组格的表单，卡底一行「取消 / 保存」；保存前差异预览、自动来源单据、此刻生效、别区缺项拦截、落册路径全部沿用票 01 的机制，只换容器。落册后回到读出并报一次 toast；写口答了别的（不予登记、冲突）留在卡里原样示出。
2. **一次只编一区**：编辑中其余卡的「编辑」「添加」收起；编辑状态由详情栏 / 对象页持有，按公司主体钉住，换了行就不算。
3. **不再模态，所以要守住离开**：取消、表单里的站内链接、列表页换行与关栏，有没提交的输入都先问「放弃修改？」；详情栏概要签改为保活（切签不卸编辑器）。
4. **抽屉退回只装整张表单**：票 01 加进 `LegalEntityProfileDrawer` / `ProfileRegistrationForm` 的 `scope` 分支撤掉，一处机制只留一份；版本清单经取数层现读保留。保存前预览、滚到格、目录译名三件抽成 `legal-entity-profile-form-parts.tsx`，抽屉与就地编辑器共用。

## 不做

- 整张表单的对象页编辑态（spec 建议第 2 条）另立票：要动 `templates/DetailPageTemplate.tsx` 共享面。
- 草稿不做。

## 完成判据

- 三道门退 0。
- 浏览器在演示租户核：点「编辑」卡体就地变表单、其余卡按钮收起；改一格来源单据自动写；保存前预览在卡内；取消有改动时问「放弃修改？」；编辑中换行问「放弃修改？」；概要签切到联系人签再切回，编辑中的内容还在；查看者账号无按钮。

## 完成记录（2026-11-04，直接在 `main` 上做，自 `5108cfd3b`，提交 `ab5727f69`，CI run 41764018050 的 `admin-web typecheck, tests, build` success）

### 落点

| 文件 | 做了什么 |
|---|---|
| `pages/party/LegalEntityProfileSectionEditor.tsx`（新） | 就地表单：现问当前版与版本清单，自动来源单据、此刻生效、别区缺项拦截、保存前预览；落册后 toast 回读出；取消与站内链接有改动先问 |
| `pages/party/legal-entity-profile-form-parts.tsx`（新） | `ChangePreview` / `scrollToField` / `useTypeNameOf` 从抽屉文件抽出，两种表单共用 |
| `pages/party/LegalEntityProfileSection.tsx` | 撤掉 `scope` 分支，抽屉只装整张表单；版本清单经取数层现读保留 |
| `pages/party/LegalEntityProfileContent.tsx`、`LegalEntityProfileSummary.tsx`、`LegalEntityContactsSection.tsx` | 收 `editing` / `onEditEnd`；在编的那张卡渲染编辑器，其余卡按钮收起 |
| `pages/party/LegalEntityDetailPanel.tsx`、`LegalEntityDetailPage.tsx` | 持有编辑态并钉住公司，换行即丢；概要签 forceMount 保活 |
| `pages/party/GroupLegalEntitiesPage.tsx` | 换行、关栏时登记簿上有没提交的资料表单先问「放弃修改？」 |
| `pages/party/legal-entity-profile-sections.ts` | `ProfileSectionEditing` / `profileSectionEditingOf` 取代 `ProfileEditScope` |

### 判据

- ✅ 三道门退 0：tsc、node:test 1193/1193、vite build；CI `admin-web` job success。
- ✅ 浏览器在演示租户核（playwright 只读探针，`syn-operator-config-01`，零写请求、零页面错误）：点「编辑代表人与地址」表单就地出现在栏内（`role=form`），其余「编辑 / 添加」按钮 4 → 0；只见代表人与地址两组；改姓名后来源单据自动写「更新代表人与地址：修改 代表人 / 负责人」；保存前预览在卡内，复述来源单据与生效时间；切到联系人签再切回，输入的值还在；取消弹「放弃修改？」，「继续编辑」后表单仍在；编辑中点第二行弹「放弃修改？」，「放弃」后选中第二行、表单消失；回到第一行联系人签「添加联系人」就地多一空行、焦点在新行姓名；对象页「添加收款账户」同样可用。
- ✅ `syn-operator-viewer-01`：零「编辑 / 添加」按钮。
- ✅ 探针途中发现并修掉一处：换行放弃后编辑态没清，回到那一行时旧编辑态复活、按钮收起——现在换了行即丢（`useEffect` 按 `row.legalEntityId` 清）。
- ◑ 落册后的 toast 与回到读出只有代码路径，探针不真提交。
- 评审：diff 全在 `pages/**`，按 workflow「前端切片」第 5 条自审。
- 证据 S。
