# 22 发布表单的私有副本抬到 party 共享层：`Field` / `useLoaded` / `VocabularySelect` / 整数解析各只留一份，节根问题一律渲染

Category: chore
Status: ready-for-agent——2026-09-09 通道 1 代裁立票（用户授权自决）：12 / 13 / 14 / 15 / 17 五票的非作者评审都点了同一条 Duplicated Code 判断题，作者们都按伞票纪律「不跨文件借私有件」各留一份、记「另立票」——这就是那张票。**Blocked by [21](./21-customer-service-rule-register-read-face.md) 与 [18](./18-customer-service-rule-form.md)**：三票都动 `CommercialPoliciesPage.tsx` 与各表单文件，本票是全页重构，等 18 的第十张表单落了再一次抬齐，不和它相撞
Blocked by: 21、18

## 缺口（评审取证，锚 `3d90130c`）

`apps/admin-web/src/pages/party/` 下九张发布表单组件各有一份私有 `Field`（Credit / SupplierAgreement / CustomerContract / AuthorizationRule /
SettlementPolicy / PricePolicy / AcceptanceRulePackage / PreAcceptanceFinancialControlPolicy 各一），若干份 `useLoaded` / `ReferencePicker` /
`VocabularySelect` / `vocabularyPlaceholder`；`credit-policy-form.ts` 与 `pre-acceptance-financial-control-policy-form.ts` 的 `integerText` /
`integerOf` / `integerProblem` 逐字同；`price-policy-form.ts` 从 `credit-policy-form.ts` 借 `normalizeMoment`、组件从 `supplier-agreement-form.ts`
借 `planReferenceOf`（评审 14 点名的兄弟文件互借）。伞票 07 当时的纪律是对的（并行写九张表单时不跨文件借私有件），代价是现在有九份同形副本，
改一处显示规则要改九处。

另一条随票收：票 17 评审非阻断 (1)——`authorization-rule-form.ts` `authorizationRuleFieldPaths` 认领了 `'authorizationRule'` 节根，但组件只渲染
`authorizationRule.cancellationAuthority` 与行内两格的 `Problems`；服务端若把问题点到节根会被认领又不显示（今天 http 不点那一格，是潜在吞问题）。
同形隐患在其它表单也可能有：凡认领了的路径都要有一处渲染。

## 完成判据

1. `party/` 下新建共享层（一个目录或一组文件，命名由作者定，写进完成记录）：`Field`、`Problems`（若各表单已有同形）、`RowFrame`、`useLoaded`、
   `ReferencePicker` / `VocabularySelect` / `vocabularyPlaceholder`、整数解析三函数、`normalizeMoment`、`planReferenceOf` 各**只有一份**；九张表单
   全部改为从共享层导入，私有副本删尽（`grep -n 'function Field' apps/admin-web/src/pages/party` 只剩共享层那一处，写进完成记录当证据）。
2. **行为零变化**：每张表单既有的 node:test 一条不改、全绿；组件层若无测试，作者用 tsc + 手工对照每张表单的字段与问题渲染写一句「逐张开过」。
   显示文案一字不改（伞票 07 硬句里 CONTEXT 原词的那几处更不能动）。
3. 票 17 非阻断 (1)：共享 `Field` / 节容器渲染**每个被认领路径**（含节根）的 `Problems`；`authorization-rule-form` 与其它凡认领节根的表单都由此
   补齐。加一条 node:test：对每张表单，`claimedPaths ⊆ renderedPaths`（怎么把「渲染了哪些路径」暴露成可测的纯函数由作者定，写理由）。
4. tsc 退 0、run-tests 全绿；共享文件（`CommercialPoliciesPage.tsx`）若要动只动 import。

## 边界

不动服务端、不动 `publication-draft-api.ts` 的线格式、不动 `PublicationDraftFlow` 的五步语义；不给任何下拉加内置枚举（词表仍只来自读口）；
不改任何表单的载荷输出（各 `*-form.test.ts` 钉着）。

## 验证强度要求（作者）

admin-web tsc + run-tests；Go 零改动。非作者评审重点：文案是否一字未变、载荷测试是否一条未改、节根 Problems 是否每张表单都渲染。
