# 03 整张「更新公司资料」改为对象页编辑态：页内就地、页脚动作条，抽屉删除

Category: enhancement
Status: resolved——2026-11-04 进 main `cd0463366`（记录见文末）
父票：[spec](../spec.md)
Blocked by: 02

## 出处

spec「不变的决定」2026-11-04 用户裁定的第二步：整张表单也照 Fiori 对象页的显示态 / 编辑态落；用户答「需要」。

## 要做的

1. **编辑态由地址决定**（`group-legal-entities-address.ts`）：`#/group-legal-entities/<id>?edit=profile` 即对象页的整版资料编辑态，与登记视图 `?register=1` 同一做法——刷新、后退都按地址走；进出只改这一个键。node:test 钉着。
2. **页脚动作条**（`templates/DetailPageTemplate.tsx`，共享面）：加 `footer` 槽位，钉在内容滚动区之下、不随内容滚；不传不渲染，渲染逐字节同。
3. **对象页编辑态**（`LegalEntityDetailPage.tsx`）：概要分区换成整张表单（`ProfileEditMode`，原抽屉体去掉 Dialog 外壳），「保存 / 取消」与答复经 portal 钉进页脚；页头「更新公司资料」与「更多」收起；联系人分区显一句「联系人在上方表单里编辑」；分区就地编辑的按钮收起。落册后报 toast、回到显示态、资料版本分区重挂。
4. **离开守卫**：「返回列表」、表单里的站内链接，有没提交的输入先问「放弃修改？」。
5. **入口统一**：详情栏页头与「资料版本」签底部的「更新公司资料」都跳到对象页编辑态；`LegalEntityProfileDrawer` 删除，表单只有一处。

## 不做

- 联系人组仍在概要分区的表单里，不拆到联系人分区（要把一份表单状态拆到两个分区，另议）。
- 草稿不做。

## 完成判据

- 三道门退 0；地址判读有 node:test。
- 共享面评审：`templates/DetailPageTemplate.tsx` 有非作者评审（隔离子代理），结论记在本票。
- 浏览器在演示租户核：页头「更新公司资料」后概要分区变成整张表单、页脚出现「取消 / 保存」、页头按钮收起；地址带 `?edit=profile`；改一格点保存出现预览；取消有改动先问；详情栏的「更新公司资料」跳到对象页编辑态；查看者账号无入口。

## 评审记录

非作者评审：隔离子代理，钉 `e798cea87` 之上的工作树。首轮 **BLOCK**：Must「返回列表」守卫确认后只退出编辑态、没真的返回；Should 进出编辑态都压历史（后退会回到编辑态）、`detailHash` 与新加的 `objectHash` 重复、两处跨文件序数；Nit 页脚按钮尺寸不一、查看者落到编辑地址不清键、分区编辑态进编辑态时不清。共享面 `templates/DetailPageTemplate.tsx` 判合格（不传逐字节同、两种布局页脚不随内容滚、portal 时序无悬挂）。修订后复评 **APPROVE**，仅余一处过期注释，已随提交改掉。

不做的一条：查看者落到 `?edit=profile` 不清键——`useMayWriteRegistry` 在授予加载期返回 false，清键会在刷新时把有授予的操作者也踢出编辑态。

## 完成记录（2026-11-04，直接在 `main` 上做，自 `e798cea87`，提交 `cd0463366`，CI run 41773143180 的 `admin-web typecheck, tests, build` success）

### 落点

| 文件 | 做了什么 |
|---|---|
| `templates/DetailPageTemplate.tsx`（共享面） | `footer` 槽位：两种布局都钉在滚动区之下，只在 ready 态渲染；不传不渲染 |
| `pages/party/group-legal-entities-address.ts` | `objectHash` / `profileEditRequested` / `profileEditHash`；测试钉着 |
| `pages/party/LegalEntityProfileSection.tsx` | 抽屉删除；`ProfileEditMode` 取代抽屉体，动作条经 portal 进页脚；预览出现时滚进视野并留出吸顶锚点栏；`landed` 计数去掉 |
| `pages/party/LegalEntityDetailPage.tsx` | 编辑态（概要换表单、页头收起、联系人指路、分区按钮收起）、页脚槽位、落册 toast 与分区重挂、按去向分的离开守卫 |
| `pages/party/GroupLegalEntitiesPage.tsx` | 进出编辑态照登记视图压历史；退出后还要去别处时替换再走并同步 hash 状态；`detailHash` 并入 `objectHash` |
| `pages/party/LegalEntityDetailPanel.tsx` | 页头与「资料版本」签底部的「更新公司资料」都跳到对象页编辑态 |

### 判据

- ✅ 三道门退 0：tsc、node:test 1194/1194、vite build；CI `admin-web` job success。
- ✅ 共享面评审：见上「评审记录」，复评 APPROVE。
- ✅ 浏览器在演示租户核（playwright 只读探针，`syn-operator-config-01`，零写请求、零页面错误）：详情栏「更新公司资料」后地址变为 `…/SYN-LE-CN-SZ-01?edit=profile`，对象页概要分区变成整张表单（五组 + 这次更新 + 高级），「保存」在页脚、不在分区内，页头「更新公司资料」消失，联系人分区显指路句；改姓名并填来源单据与生效时间后点保存出现预览且滚进视野；「返回列表」弹「离开这一页？」，「继续编辑」留下、「离开」后列表显示且对象页表单已卸；再进编辑态改一格后「取消」弹「放弃修改？」，「放弃」后留在对象页、地址不带编辑键、页头按钮与读出卡回来；「资料版本」签底部按钮进编辑态，未改动时取消不问；深链 `?edit=profile` 刷新仍在编辑态。
- ✅ `syn-operator-viewer-01`：深链 `?edit=profile` 不出表单，页头无按钮。
- ◑ 落册后的 toast、回到显示态与资料版本分区重挂只有代码路径，探针不真提交。
- 证据 S。
