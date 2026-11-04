# 01 分区编辑抽屉：读出卡上的「编辑」「添加」打开只装这一组格的表单

Category: enhancement
Status: resolved——2026-11-04 进 main `e3ccd7356`（记录见文末）
父票：[spec](../spec.md)
Blocked by: 无

## 现状

公司资料只有一个入口「更新公司资料」（详情栏页头、对象页页头、「资料版本」签底部），打开整张抽屉表单，五组格全在里面。只想改一个联系人电话、加一个收款账户，也得从头看到尾再保存。「联系人」签底下一句「要改联系人，用右上方的「更新公司资料」」就是这条路的写照。

## 要做的

1. **分区表**（纯逻辑 `legal-entity-profile-sections.ts`，node:test 钉着）：按读出的四张卡分区——代表人与地址、税务与开票、收款账户、联系人；每区认领表单里的哪几组格、哪些 JSON 路径前缀、差异预览里哪些 group。逐行的两区（收款账户、联系人）有「添加」：打开抽屉时先加一空行并把焦点滚到它。
2. **入口**：`ProfileReadoutCard` 标题条右侧留动作位；`LegalEntityProfileContent` 收 `onEdit`，有写授予（`useMayWriteRegistry`）且有当前生效版时每张卡显「编辑」、逐行的卡再显「添加…」。概要签（详情栏与对象页）与「联系人」签都接上；按日期查询的读出不接（那是历史视图）。
3. **抽屉**：`LegalEntityProfileDrawer` 收 `scope`（整张 / 某一区）。某一区时：标题「更新〈区名〉」；表单只渲染这一区的组（`LegalEntityProfileFields` 收 `groups`）；不显「基于哪一版 / 带入版本」那一行；当前版打开时现问（`fresh`）。
4. **这次更新**：来源单据一格按改动自动写（`profileSectionBasisOf`：「更新联系人：新增 张三、修改 李四」），人一改就按人的，可点「用自动写的」复位；生效时间预填打开抽屉的此刻（`moment.ts` 新增 `wallTimeOf`），与版本号一起收在高级组。
5. **保存**：本区的必填缺项照旧点名并滚到格；别区的必填缺项（例如注册地址没填）列在按钮旁、禁用保存、指路「更新公司资料」整张表单。保存前差异预览、确认、提交、落册后的答复与「关闭」都复用整张表单那一套。
6. **版本清单**改经取数层现读（`useLegalEntityRead('profileRevisions', …, { fresh })`），整张表单与分区抽屉同一份；原先的页内 effect 删掉。

## 不做

- 不改写口载荷形状；不在本地判领域规则。
- 不做行级「删除」入口：移除一条仍在该区的抽屉里点「移除」。
- 按日期查询的读出不带编辑。

## 完成判据

- 三道门退 0；新增纯函数有 node:test（分区归属、新行、自动依据、墙钟此刻）。
- 浏览器在演示租户核：概要卡上有「编辑」，联系人卡有「添加联系人」；点开只见该组格；来源单据自动写且可改；别区缺项时保存被拦并指路。

## 完成记录（2026-11-04，直接在 `main` 上做，自 `a434ae686`，提交 `e3ccd7356`，CI run 41761493660 的 `admin-web typecheck, tests, build` success）

### 落点

| 文件 | 做了什么 |
|---|---|
| `pages/party/legal-entity-profile-sections.ts`（新） | 分区表、路径归属、新行、来源单据自动写法；`legal-entity-profile-sections.test.ts` 钉着 |
| `pages/moment.ts` | `wallTimeOf`：时刻折成该区墙钟原值，与 `wallTimeToRfc3339` 互为反向；`moment.test.ts` 钉着 |
| `pages/party/LegalEntityProfileReadout.tsx` | `ProfileReadoutCard` 标题条留 `actions` 位 |
| `pages/party/LegalEntityProfileContent.tsx` | 收 `onEdit`；`SectionActions`（「添加…」「编辑」） |
| `pages/party/LegalEntityProfileSummary.tsx`、`LegalEntityContactsSection.tsx` | `onEdit` 透传；联系人签有入口时不再指路整张表单 |
| `pages/party/LegalEntityProfileFields.tsx` | `groups`：只渲染这几组 |
| `pages/party/LegalEntityProfileSection.tsx` | 抽屉与表单收 `scope`；分区模式的基线、自动依据、此刻生效、别区缺项拦截、预览复述依据；版本清单改经取数层现读 |
| `pages/party/LegalEntityDetailPanel.tsx`、`LegalEntityDetailPage.tsx` | 各挂一个分区抽屉，概要签与联系人签接 `onEdit` |

### 判据

- ✅ 三道门退 0：tsc、node:test 1193/1193、vite build；CI `admin-web` job success。
- ✅ 浏览器在演示租户核（playwright 只读探针，`syn-operator-config-01`，零写请求、零页面错误）：概要三张卡各有「编辑」，收款账户卡有「添加收款账户」；联系人签有「添加联系人 · 编辑」；「更新代表人与地址」抽屉只见代表人与地址两组；「更新联系人」从「添加」打开多一空行、焦点在新行姓名；来源单据空行时「更新联系人：内容未变，重新登记一版」，填名后「更新联系人：新增 SYN 探针联系人」，手改成 `SYN-DOC-01` 后出现「用自动写的」、点回自动句；保存前预览复述来源单据与生效时间；Esc 关抽屉有「放弃并关闭」确认；对象页「添加收款账户」同样可用，高级组摘要「生效时间 2026-11-04 10:32:10 · 版本号 4（建议值）」。
- ✅ `syn-operator-viewer-01`：零「编辑 / 添加」按钮，联系人签仍指路整张表单。
- ◑ 「别区缺项拦下保存」只有 node:test（`splitMissingBySection`），演示数据五家主体注册地址都齐，浏览器里没碰到这一支。
- 评审：diff 全在 `pages/**`，按 workflow「前端切片」第 5 条自审。
- 证据 S。
