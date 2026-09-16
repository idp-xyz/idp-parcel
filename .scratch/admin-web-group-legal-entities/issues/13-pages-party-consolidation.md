# 13 `pages/party` 收口：09 / 10 / 12 三张票评审判断项里的同形副本、重复 switch 与陈旧读面归一

Category: chore
Status: ready-for-agent
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
