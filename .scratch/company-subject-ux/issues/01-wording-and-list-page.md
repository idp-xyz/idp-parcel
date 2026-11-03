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
