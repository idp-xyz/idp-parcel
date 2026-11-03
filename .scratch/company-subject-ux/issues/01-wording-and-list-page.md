# 01 用语统一、列表页细节、签名、空值与时间精度

Category: enhancement
Status: in-progress
父票：[spec](../spec.md)
Blocked by: 无
出处：spec「统一用语」表；评估「二、内容表达」「五、视觉细节」。

## 要做的

1. **用语**：按 spec「统一用语」表改本票落点里的所有界面词与说明文字。`presentation.ts` 里 `updateOrdinalLabel`、`identityRevisionLabel`、`profileRevisionLabel`、`profileResolutionOutcomeLabels`、`profileIncompleteCauseLabels`、`partyNameUnknownNote`、`identityLayerAbsentNote` 等改成表里的话；`legal-entity-profile.ts` 的 `legalEntityProfileStatusOf` 由票 04 改，你只在 spec 表里定词，不动那个文件。
2. **签名**：`templates/workspace-tabs.ts` 的 `'identity-revisions'` 签名「身份修订」→「登记记录」；`'profile-versions'` 不变。这是共享面，照 workflow.md「前端切片」第 5 条要一份 Spec 轴评审（协调方派隔离子代理）。
3. **列表页**（`GroupLegalEntitiesPage.tsx`）：
   - 「注册地与注册号」列：类型码经注册号类型目录译成 `typeName`（页面加载一次 `listRegistrationNumberTypes`，目录读不到时原样显码，不报错）；判读进 `legal-entity-identity.ts` 纯函数并钉测试。
   - 副标题、空态、使用说明弹窗全文按 spec 表改成运营的话。
   - 「待处理」筛选加 `title` 说明：「注册地或注册号缺失，或关联的往来单位查不到」。
   - 「生效时间」「登记时间」列显示到日期（见第 5 条）。
4. **空值与地址**：`LegalEntityContactsTable.tsx`、`LegalEntityProfileContent.tsx`、`LegalEntityProfileReadout.tsx` 空格用「—」；地址去掉「CN ·」前缀，国家经基础数据译名（`CountryLabel`）放在地址后括注或单独一行——你定一种，三处一致。
5. **时间精度**：`detail-primitives.tsx` 的 `Instant` 加可选 `precision?: 'day' | 'minute' | 'second'`（缺省照旧），`day` 显示 `2026-01-02`、`title` 带完整时刻与时区。法人页的生效 / 登记 / 停用都用 `day`。版本历史（`LegalEntityProfileHistory.tsx`、`RevisionHistorySection.tsx`）的登记时刻用 `minute`。`moment.ts` 若已有可复用的格式选项就用它，不另造。
6. **版本历史**（`LegalEntityProfileHistory.tsx`、`legal-entity-profile-history.ts`、`legal-entity-revisions.ts`）：「资料 r4」→「第 4 版」；「身份 r1」→「第 1 版」；「共 1 笔身份修订，按修订号从早到晚」→「共 1 笔登记记录，从早到晚」；对比表「项目」列把联系人名、部门 / 职务、平台拆成两行或两列，不再塞一格。
7. `navigation.ts` 里 `group-legal-entities` 的 subtitle / description 若有同类句子一并改。

## 不做

- 不动 `LegalEntityDetailPage.tsx`、`LegalEntityDetailPanel.tsx`、`LegalEntityHeaderParts.tsx`（票 03）、`LegalEntityRegistrationForm.tsx`、`legal-entity-form.ts`（票 02）、`LegalEntityProfileSection.tsx`、`LegalEntityProfileFields.tsx`、`legal-entity-profile.ts`（票 04）。这些文件里的词由各自的票按同一张表改。
- 不改 `TechnicalDetails` 里的行名与码。
- 不改后端、不改领域文档。

## 完成判据

- 三道门退 0；新增 / 改动的纯函数有 node:test。
- 浏览器在演示租户核：列表「注册地与注册号」显「统一社会信用代码 91000000000000001X」（SYN-LE-01）与目录译名；日期列只显日期、hover 有完整时刻；使用说明全文是运营的话；联系人表空格为「—」。
- `templates/workspace-tabs.ts` 的改动有评审记录。

## 落点之外要动的文件（动前先记）

- `src/pages/party/revision-timeline.ts`（两册共用的修订时间线判读）：描述里的「依据」→「来源单据」、计数句去掉「按修订号」——这两个词在 spec 表里退役，句子却在共用件里拼，法人册的薄适配改不到；往来单位页随之同词。配套改 `business-party-revisions.test.ts` 的三处字面钉。
- `src/pages/party/legal-entity-profile.test.ts`：`profileRevisionLabel` 改成「第 N 版」后，资料修订时间线标题的字面钉跟着换一行；`legal-entity-profile.ts` 本身不动（票 04）。
- `src/pages/moment.ts` + `moment.test.ts`：`formatInstant` 加 `precision` 选项（`day` / `minute` / `second`，缺省 `second`）。票面第 5 条说「若已有可复用的格式选项就用它」——没有；墙钟与时区知识只在这一处，`Instant` 不在呈现件里另算一遍。

## 完成记录（2026-11-02，分支 `claude-csux-01`，自 main `ae9a9a763`）

### 落点

| 文件 | 做了什么 | 提交 |
|---|---|---|
| `presentation.ts` | `updateOrdinalLabel` / `identityRevisionLabel` / `profileRevisionLabel` 都短写「第 N 版」，另备长写 `identityRecordLabel`「登记记录 · 第 N 版」/ `profileVersionLabel`「资料版本 · 第 N 版」给签名盖不住轴名的位置；`partyNameUnknownNote`「往来单位里查不到这家单位」、`identityLayerAbsentNote`「这一版登记时还没有这两格」、解析五格与两种原因按表照抄 | `68be91530` |
| `legal-entity-revisions.ts`、`revision-timeline.ts`（落点外，见上节） | 时间线内容段：往来单位档案 / 注册地（经基础数据译名）/ 注册号 / 更正来源单据；「依据」→「来源单据」；计数句「共 N 笔登记记录，从早到晚。」；登记时刻到分钟、生效与停用到日期由时间线自己定（format 收 `FormatInstantOptions`，页面照旧传 `formatInstant` 即生效）；`legalEntityRevisionTimeline` 多收可选 `typeNameOf` | `68be91530`、`ccdfdd2ab` |
| `templates/workspace-tabs.ts` | `identity-revisions` 签「身份修订」→「登记记录」；`profile-versions` 不变 | **`55b0bfa13`（共享面，待 Spec 轴评审）** |
| `pages/moment.ts`（落点外）、`detail-primitives.tsx` | `formatInstant` 加 `precision`（day / minute / second，缺省照旧；日期按呈现时区取墙钟再截）；`Instant` 收 `precision`，收短时 `title` 给完整时刻带偏移 | `416c26493` |
| `legal-entity-identity.ts` | `registrationNumberTypeNamesOf` 把目录折成按国家与码的查法；`identityLayerCellsOf` / `lifetimeNumbersText` 可选收它，译不出原样显码 | `fe1196123` |
| `GroupLegalEntitiesPage.tsx`、`navigation.ts` | 页面读一次 `listRegistrationNumberTypes` 折成查法交给「注册地与注册号」列（读不到按码显、不挡列表）；「待处理」胶囊 `title`；生效 / 登记 / 停用 `precision="day"`；副标题、新增页副题、空态、使用说明全文、navigation summary 按表 | `90d8150ef` |
| `LegalEntityProfileReadout.tsx`、`LegalEntityProfileContent.tsx`、`LegalEntityContactsTable.tsx` | 空格「—」（`absentValueMark`）；`ProfileReadoutAddress` 定地址版式——行在前、国家经 `CountryLabel` 另起一行，三处同一份；联系人表两行格有哪行显哪行、都没有才一个「—」 | `62f76b628` |
| `LegalEntityProfileHistory.tsx`、`legal-entity-profile-history.ts` | 版本处处「第 N 版」；登记时刻 minute、生效 day；「登记依据」→「来源单据」；对比表「项目」列拆成「姓名 · 项」+「部门 / 职务」两行；对比与快照的地址行在前、国家译名另起一行（`profileAddressText`，页面订阅名表让读回后重算）；空格「—」 | `9c51e68eb` |
| `RevisionHistorySection.tsx` | 未改：它只摆 register 交来的条目，精度与词都在 `revision-timeline.ts` 定好 | — |
| `legal-entity-list.ts` | 未改：筛选、计数与排序不涉及用语；搜索仍按类型码匹配 | — |

### 判据

- 三道门退 0（`git merge origin/main` 后，main 仍是 `ae9a9a763`）：`tsc -b --noEmit` 0、`run-tests` 1133 / 1133、`vite build` 0 ✅
- 新增 / 改动的纯函数有 node:test：`legal-entity-identity.test.ts`（+2）、`moment.test.ts`（+1）、`legal-entity-profile-history.test.ts`（+1）、`legal-entity-revisions.test.ts`（改 4、新 2）、`workspace-tabs.test.ts`（改字面） ✅
- 浏览器（演示租户，自起 vite :5201 + `VITE_DISPLAY_TIME_ZONE=Asia/Shanghai`，playwright 只读、零写请求、零 pageerror；截图在作者 scratchpad `csux01/`）：
  - 列表「注册地与注册号」：SYN-LE-01 显「统一社会信用代码 91000000000000001X」，另四家显目录名「模拟注册号（仅演示，非真实证照格式） SYN-CN-REG-0102」等 ✅ `01-list.png`
  - 日期列只显 `2026-01-02`，`title` 为 `2026-01-02 08:00:00 UTC+8` ✅
  - 使用说明全文是运营的话 ✅ `02-help.png`
  - 对象页签「概要 | 联系人 4 | 资料版本 | 登记记录」✅ `04-object-summary.png`；概要与快照的地址行在前、「中国（CN）」另起一行 ✅
  - 资料版本：「第 4 版 · 当前生效」、登记时刻 `2026-10-30 18:20`、「来源单据」、项目列两行、空的上一版显「—」 ✅ `06-object-profile-versions.png`
  - 登记记录：「共 1 笔登记记录，从早到晚。」「登记于 2026-10-28 19:59」「生效自 2026-01-02 · 往来单位档案 … · 注册地 中国（CN） · 注册号 …」 ✅ `07-object-identity-records.png`
  - 联系人表空格为「—」：演示数据每格都有值，线上没出现过空格，只按代码路径（`StackedValues` / `absentValueMark`）◑ `05-object-contacts.png`
- `templates/workspace-tabs.ts` 评审记录：待协调方派隔离子代理（`55b0bfa13`）◑

### 未验 / 留给别的票

- 详情栏（`LegalEntityDetailPanel`）与对象页头仍显「身份修订」签、「更新法人资料」、「注册号 USCC …」、「资料状态：开票资料可用」、「对应的往来单位」——票 03 / 04 的落点。用词源在本票已备好：`identityRecordLabel` / `profileVersionLabel`、`legalEntityRevisionTimeline` 的第三参 `typeNameOf`、`legal-entity-identity.ts` 的查法。
- `contactPurposeLabels.GENERAL`「通用联系」→「日常联系」在 `legal-entity-profile.ts`（票 04）。
- `identityKindLabels`（停用表单种类词「责任法人」等）未改：`identity-deactivation-form.test.ts` 钉着它，停用对话框归票 03。
- 资料读出与快照的税务登记号仍显类型码（`SYN-CN-TAX SYN-CN-TAX-0001`）：译名要把目录查法穿过票 03 / 04 的组件，本票未接。
- 列表搜索仍按类型码匹配，不按译名。
