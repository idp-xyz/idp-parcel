# 13 `pages/party` 收口：09 / 10 / 12 三张票评审判断项里的同形副本、重复 switch 与陈旧读面归一

Category: chore
Status: resolved
Blocked by: 无（09 / 10 / 12 皆已进 main，远端 main `29e5117b`）
地盘：`apps/admin-web/src/pages/party/`（`BusinessPartiesPage.tsx`、`GroupLegalEntitiesPage.tsx`、三份 `*RegistrationForm.tsx` 与 02 的
`LegalEntityRegistrationForm.tsx`、`party-registration-fields.tsx`、各 `*-form.ts`、`legal-entity-list.ts` / `list-order.ts`、
`business-party-list.ts` / `party-relationship-list.ts`、`detail-primitives.tsx`；`domain/status.tsx` 如需）；`components/registration/
MultiRegistrationPanel.tsx`（若抬 `chipClass`）；`ReferencePicker`（`PublicationFormFields.tsx`）若交出行。
出处：票 09 评审 ← 通道 2（N3 / N4 / S3）、票 10 评审 ← 通道 5（N1–N5 / S1 / S2）、票 12 评审 ← 通道 2（(c) N1）——全部「无阻断、判断项」，
各票面处置写「归收口票」。清单第一版在票 10 的推送方处置里。

## 为什么

第二轮五张票（08–12）各自在 `pages/party` 下落了新件，评审逐票都判无阻断，却把同一类判断项各记了一遍：三张票之间、以及与票 01 / 02 之间，
同形的壳、同形的格、同词的句、同一封闭集的两处 switch 并存。单张票不碰他人文件是对的（parallel-sessions 地盘纪律），代价就是这些副本要在
所有票落地之后一次收——现在到了。不收，下一个改「修订号建议」或「停用色调」的人要改四处，而且会漏一处。

## 要做的（九条，每条独立成笔，可分人；序号只为对照出处，不表顺序）

1. 三份表单组件的壳（draft / revisionEdited / state / patch / submit / send / known）抬成 `useRegistrationForm` hook，02 的
   `LegalEntityRegistrationForm` 作第四个消费者（10 N1）。
2. 停用表单「种类 → {load, optionsOf, readFace, 投影}」合成一张表，收掉 `deactivationTargetsOf` 的 `switch(kind)` 与
   `IdentityDeactivationForm` 里按 `draft.kind` 的三分支（10 N2）。
3. `revisionOf` 抬独立模块；02 的 `legal-entity-form.ts` 切过来，并统一「不裁空白」判据（10 N3 + 作者跟进）。
4. `chipClass` 抬共享件，`MultiRegistrationPanel` 与登记签选册 chip 同用一份（10 N4 + 作者跟进）。
5. `ReferencePicker` 交出行（或收一份已取回的答案），免停用表单一挂即读两册、关系表单每次 `partiesVersion` 变两只 Picker 各重读
   （10 N5 + 作者跟进）。
6. `useRegisterList` 重取时保留上一份答案，免修订建议与标识提示闪回 1 /「列表里没有」；`GroupLegalEntitiesPage` 同改（10 S1）。
7. 停用 `LEGAL_ENTITY` / `CUSTOMER_ACCOUNT` 后按 kind 重读对应册，免同册再操作时建议与候选按旧册面算（10 S2）。
8. 09 的三条：两页各一份 `statusBadge` 归一处（`detail-primitives` 或 `domain/status`）；「参与方册查无此身份」同词两处归一；
   `legal-entity-list.ts` 私有比较器切 `list-order.ts`；`business-party-list.ts` 与 `party-relationship-list.ts` 的筛选码类型统一形状
   （09 N3 / N4 / S3）。
9. 两页历史区（`LegalEntityRevisionHistory` / `BusinessPartyRevisionHistory`）的依赖键与 `useRegisterList` 重取的交互取齐：12 (c) 用
   `key` 带修订号，评审判在今天的重取做法下冗余无害；第 6 条改了重取做法后再判两页要不要 key，同一做法两页取齐（12 (c) N1 + 作者判断项）。
   顺手判两页各持一份结构相同的历史区组件要不要抽成收 fetch / 标识 / 判读为 props 的共用区（12 (c) 评审 Smell 基线，不可行动判断——
   这里只判，判「抽」就做进本条，判「不抽」写明理由）。

## 不做

- 不改任何行为：每一笔既有 `run-tests` 用例零改动仍绿——那是每条的完成判据，不是顺带。
- 不改服务端、不动 Go。
- 不动票 04 / 05（needs-info）的范围；不改两签结构。

## 完成判据

- 每笔：`node node_modules/typescript/bin/tsc -b --noEmit` 0 / `node scripts/run-tests.mjs` 既有用例零改动仍绿 / `pnpm exec vite build` 0。
- 第 1 / 3 / 8 条各带至少一条测试钉共用件的规则（不是实现镜像；接手别人在途件时先写自己的 red 再读，见 parallel-sessions）。
- 完成后 `pages/party` 下 `suggestedRevision` / `revisionOf` / `statusBadge` / `chipClass` 各只剩一份定义——用 `git grep` 自己数，票面写钉的 SHA。

## Comments

### 完成记录（通道 3 → 4 → 1 三任 · 2026-09-16 23:5x–09-17 00:30 + 09-20 10:5x–11:2x · 分支 `mcp3-adminweb13` 基 main `8608aa33` · tip `d1926646`）

**谁做的**：第 6 / 3 / 8 / 1 条与第 5 条前半是通道 3 会话（09-17 00:01–00:17）；第 5 + 2 条补齐与第 7 条是通道 4 会话接手同一棵树
（00:29–00:30，见 `92ca5332` 提交信）；第 4 条那一笔的内容是通道 3 / 4 留在工作树的未提交现场，通道 1 于 09-20 逐字核对后原样收入
（`95b5d967`，只加提交不改内容）；第 9 条与本记录由通道 1 作（`d1926646`）。用户 09-20 令「全面完成，并 replay」，通道 2–6 心跳都停在
09-16 23:4x 之后，作者树无人在写，通道 1 接手不算抢地盘。

**要做的逐条**

1. ✅ 壳抬成 `useRegistrationForm`（`party-registration-fields.tsx`），各表单只交本册的 `RegistrationFormSpec`（kind / empty / suggestion /
   localProblems / payloadOf / landedOutcome / onLanded）并摆格；「建议值何时顶进草稿」（`effectiveDraftOf`）与「答什么才算落地」
   （`registrationLanded`）是 `registration-form.ts` 的纯函数，`registration-form.test.ts` 各钉一条。02 的 `LegalEntityRegistrationForm` 作第四个
   消费者，顺带切到共用格（`RevisionField` / `WallTimeField` / `businessPartyPickerOptions`），显示文案一字未改。`71c6d932`。
2. ✅ 停用表单「种类 → {readFace, load, optionsOf, targetsOf}」一张 `Record<IdentityKind, KindRegister>`，三分支 JSX 收成一支；纯模块侧
   `identityTargetOf` 表 + `isIdentityKind` 守门收掉 `deactivationTargetsOf` 的 switch；新用例「分派表与种类词表同键，集外不认」。`92ca5332`。
3. ✅ `revisionOf` 抬成 `registration-form.ts`，四份 `*-form.ts` 同引；`registration-form.test.ts` 钉「只编正整数」「允许首尾空白」。
   **「统一不裁空白判据」那半不做**：`legal-entity-form.ts` 的 `legalEntityPayloadOf` / `suggestedRevision` 今天裁空白，是票 02 验收过、
   `legal-entity-form.test.ts` 钉着的载荷行为，改它是行为变化，与本票「既有用例零改动」相抵——要改另立票，先改 02 的判据再改码。`b6e708b8`。
4. ✅ `chipClass` → `components/registration/chip.ts` 一份，从桶导出；`MultiRegistrationPanel` / `BusinessPartiesPage` 登记签 / `CommercialPoliciesPage`
   三处改导入。其它模块页（network / visibility / operations / customs / channel-selection 共十一处）的私有副本不在本票地盘，不动。`95b5d967`。
5. ✅ `ReferencePicker` 拆成自读的（`load`，读一次）与收答案的 `ReferencePickerFor`（`answer`），格与措辞共用 `ReferencePickerFaceProps`；关系表单两只
   Picker 改收页面持有的 `parties`（`partiesVersion` 撤下）；停用表单参与方册取页面答案、法人 / 客户账户册在选中那一种类时才读一次。`92ca5332`。
6. ✅ `useRegisterList` → `register-list.ts` 两页共用；重取做法：上一份是业务答案就留着，不是（未配置 / 出错）才回加载中；状态转移是纯函数
   `registerListReducer`，`register-list.test.ts` 三条。`b3579c45`。
7. ✅ 停用口 `onLanded(sent)` 按送出种类分派：BUSINESS_PARTY → 页面重取；LEGAL_ENTITY / CUSTOMER_ACCOUNT → 本表单那一册 reloadKey +1 重读；
   JSON 镜像路交 null 分不出种类 → 两边都重取。`ef007b22`。
8. ✅ `statusBadge` → `detail-primitives.tsx` 一份（词表 + 码）；「参与方册查无此身份」六处 → `presentation.ts` 的 `partyNameUnknownNote` 一句 +
   `UnknownPartyName`；`legal-entity-list.ts` 私有比较器切 `list-order.ts`；身份族四张词表 `as const satisfies`、键派生码类型，`list-order.ts` 加
   `CodeFilter<Code>` / `codeFilterOptions`，三份列表模块筛选类型同一形状；`list-order.test.ts` 两条。`6e4d09dc`。
9. ✅ 两页历史区抽成 `RevisionHistorySection`（`pages/party/RevisionHistorySection.tsx`），两册各交一份模块级 `RevisionHistoryRegister`
   （subject / endpoint / load / echoedSubjectId / timelineOf / noteOf）；判读仍在 `*-revisions.ts`。**判断写在下节。** `d1926646`。

**完成判据逐条**

- 三道门 ✅ 每笔提交信各带当笔数字；tip `d1926646`：`tsc -b --noEmit` 0 / `run-tests` **289**（main `8608aa33` 279 + 10：register-list 3、
  registration-form 4、list-order 2、identity-deactivation-form 1）/ `vite build` 0。既有用例零改动：`git diff 8608aa33..d1926646 -- '*.test.ts'`
  四文件 129 行全为新增。
- 第 1 / 3 / 8 条各带钉共用件规则的测试 ✅ `registration-form.test.ts`（第 1 条 `effectiveDraftOf` / `registrationLanded`，第 3 条 `revisionOf` 两条）、
  `list-order.test.ts`（第 8 条）。
- 各只剩一份定义 ✅（钉 `d1926646`，`git grep -n -E "(const|function) (chipClass|statusBadge|revisionOf|suggestedRevision)\b" -- apps/admin-web/src/pages/party
  apps/admin-web/src/components/registration`）：`chipClass` 在 `components/registration/chip.ts`、`pages/party` 下零；`statusBadge` 在 `detail-primitives.tsx`；
  `revisionOf` 在 `registration-form.ts`；`suggestedRevision` 在 `legal-entity-form.ts`。
- 浏览器验收**未验**（本机无浏览器验收路径，与 10 / 12 (c) 同口径）；行为不变由既有用例零改动仍绿钉着。

**判断项**

- 第 9 条·要不要 revision 依赖：**要。** 第 6 条之后 `useRegisterList` 重取不清上一份答案，抽屉在列表重取期间开着不卸载；登记签给同一对象登了下一笔
  （或停用）后行的 `revision` 变、`subjectId` 不变，只按 `subjectId` 记依赖会继续显上一条链——12 (c) 评审 N1 说的「今天冗余」在第 6 条落地那一刻
  失效，参与方页的 `key` 从保险变成必需，而法人页没有。现在两页同一做法：共用件的 effect 依赖 `[load, subjectId, revision, reloadKey]`，
  调用方不必记得给 `key`；`key` 撤下。
- 第 9 条·要不要抽共用区：**抽。** 两份组件六十余行同形，只在读口、回显标识字段、主语两字、读口路径与两个判读函数上不同，正好收成一份
  `RevisionHistoryRegister`；而上一条要改的「何时重取」若不抽就得改两处——本票要收的正是这一类。抽的代价是六个 props，比两份副本便宜。
  页面文案逐字保住（读口路径与主语由 register 交进来）。
- 第 3 条那半不做，理由见要做的 3。
- `suggested*Revision` 四份同形（`suggestedRevision` / `suggestedBusinessPartyRevision` / `suggestedDeactivationRevision` /
  `suggestedPartyRelationshipRevision`，只差取键的字段、法人那份多一道 trim）：完成判据点名的 `suggestedRevision` 字面只剩一份，但同形族
  不在九条里、本记录只记不动；要折成一份 `suggestedNextRevision(rows, id, keyOf)` 归后续票，与第 3 条那半一起。
- 第 4 条其它模块的十一处 `chipClass` 副本：抬到 `components/` 后它们能直接改导入，不在本票地盘，归后续票。

**笔** `9848d60a`（认领）→ `b3579c45`（6）→ `b6e708b8`（3）→ `6e4d09dc`（8）→ `71c6d932`（1）→ `92ca5332`（5 + 2）→ `ef007b22`（7）→
`95b5d967`（4）→ `d1926646`（9）→ 本票面笔。分支全推 origin。
