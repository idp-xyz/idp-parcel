# 22 发布表单的私有副本抬到 party 共享层：`Field` / `useLoaded` / `VocabularySelect` / 整数解析各只留一份，节根问题一律渲染

Category: chore
Status: resolved——2026-09-09 20:5x 通道 4 完工（接续单 task-5c5a179a，通道 2 派；分支 `mcp4-awf22` tip `186ac61b`，待非作者评审与推送方重放）。此前 in-progress——2026-09-09 18:3x 通道 4 认领（接续单 task-7a3944f8，通道 3 派；分支 `mcp4-awf22` 基 main `d5a35960`，隔离树 `D:/tops/idp-parcel-mcp4-awf22`，`node_modules` 走 junction 借共享树；21 / 18 已进 main，Blocked by 已无未落项）。此前 ready-for-agent——2026-09-09 通道 1 代裁立票（用户授权自决）：12 / 13 / 14 / 15 / 17 五票的非作者评审都点了同一条 Duplicated Code 判断题，作者们都按伞票纪律「不跨文件借私有件」各留一份、记「另立票」——这就是那张票。**Blocked by [21](./21-customer-service-rule-register-read-face.md) 与 [18](./18-customer-service-rule-form.md)**：三票都动 `CommercialPoliciesPage.tsx` 与各表单文件，本票是全页重构，等 18 的第十张表单落了再一次抬齐，不和它相撞
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

## 完成记录（通道 4，2026-09-09；分支 `mcp4-awf22` 基 `d5a35960`，纯 admin-web，Go 零改动）

分层 SHA（全部已推 origin 同 SHA；每笔 tsc --noEmit 0，run-tests 前六笔 188 / 188、末笔 195 / 195）：

- `21c40771` 票面转 in-progress。
- `a2f0cc2a` 共享层落地：纯逻辑 `publication-form-shared.ts`（`integerText` / `integerOf` / `integerProblem`、`normalizeMoment`、`planReferenceOf`、`unrenderedClaimedPaths`）与组件件 `PublicationFormFields.tsx`（`Field` / `Problems` / `PathProblems` / `RowFrame` / `useLoaded` / `ReferencePicker` / `VocabularySelect` / `vocabularyPlaceholder`，与 `fieldLabel` / `selectClass` / `momentPlaceholder` 三个样式常量）；五个 `*-form.ts` 改从共享层导入，`price-policy-form.ts` 不再从 `credit-policy-form.ts` 借 `normalizeMoment`。
- `e32cb604` CustomerServiceRule / PreAcceptanceFinancialControlPolicy / SupplierAgreement 三张组件接共享层。
- `713b37a0` CreditPolicy / PricePolicy / SettlementPolicy 三张接共享层；`ReferencePicker` 为价卡目录那格加 `manualPlaceholder` / `optionsNote` / `unknownNote` 三个整句留口——都是措辞不是形状。
- `3435569e` CustomerContract / AuthorizationRule 两张接共享层，行框改用 `RowFrame`，`useObjectCandidates` / `usePartyOptions` 改走 `useLoaded`。
- `79210fa3` AcceptanceRulePackage / ServiceProduct 两张接共享层——至此十张全接。
- `186ac61b` 判据 3：补 `'authorizationRule'` 节根与客户合同隐藏格的代显，十张各加 `*RenderedPaths` 声明，新增 `publication-form-rendered-paths.test.ts`。

**判据 1 证据**——`git grep -n 'function Field' -- apps/admin-web/src/pages/party` 在 `186ac61b` 上的全部输出：

```
apps/admin-web/src/pages/party/PublicationFormFields.tsx:56:export function Field({
```

同一条 grep 换成 `Problems` / `RowFrame` / `useLoaded` / `ReferencePicker` / `VocabularySelect` / `vocabularyPlaceholder`，也各只剩 `PublicationFormFields.tsx` 一处。共享层的命名与位置：不另开目录，`party/` 下两个平级文件——`publication-form-shared.ts` 只放与哪一册无关的纯函数（可进 node:test），`PublicationFormFields.tsx` 放组件件；某册自己的路径、草稿、载荷仍留在它自己的 `*-form.ts`。ServiceProduct 此前 grep 不到 `function Field` 不是本就没有：它把「标签 + Input + 问题行」写成内联箭头 `field()`，另留一份 `FieldProblems` 与 `fieldLabel`——同形副本换个名字，`79210fa3` 一并接了。

**留在本册、有意不抬的**（不是同形副本，换成共享件会改显示文案，判据 2 不许）：CustomerContract 的 `ObjectPicker`（按对象标识去重、没有「手填不在读面上」那一项、说明句不同）；AuthorizationRule 的 `PartyPicker` 与 AcceptanceRulePackage 的 `CodeSelect` / `CodeChecklist`（占位三句与选项格式与 `VocabularySelect` 不同）；SettlementPolicy 的 `ContractPicker`（对象 + 版本两格一起落，是一格选单对两条路径，形状本就不同）；PricePolicy 的 `ConditionalField`（隐时显值与「清空」，是条件格不是普通格）；CustomerServiceRule 与 PreAcceptance 两张的行外框（行标题「第 n 行」+「删这一行」，与 `RowFrame` 的「删」不同句）；ServiceProduct 引用表那种没有标签的两格一行（与 `RowFrame` 的 `pt-5` 对不齐）。它们各自的数据取用已改走 `useLoaded`，只留下呈现那一层。

**判据 2 证据**——载荷测试一条未改：`git diff --stat d5a35960 -- '*.test.ts'` 只有新增那一条（`publication-form-rendered-paths.test.ts | 163 +`）。文案：把十张组件在 `d5a35960` 与 `186ac61b` 两版去掉注释后的中文文本段做多重集比对（`186ac61b` 那侧并上 `PublicationFormFields.tsx`），406 段对 404 段，仅差四段，全是同一句话被参数化拆开——`正在读价卡目录…` 成了 `正在读${readFace}…` + `readFace="价卡目录"`，`（手填，不在读面上）` 成了 `（{unknownNote}）` + 默认值——渲染出的字一样；SettlementPolicy 方式格那句里「枚举、」后的半角空格是此前 JSX 折行的渲染结果，作为整句传入时照原样保留。**逐张开过**：十张组件在 Node 里 `renderToStaticMarkup` 各渲染一遍（`@idpxyz/ui-primitives` 顶层 import 了 `.css`，以只含 Button / Input / Card 四件的原生元素替身顶替），零抛错、标题在、每格标签旁的路径在；未在浏览器里点开——本机 `pnpm build` 那道门本来就不通（见 workflow.md 本机环境），tsc 是仅剩的门禁。

**三处呈现上的有意变化**（不是文案，评审可以不同意）：(a) 标签旁一律显 JSON 路径——CustomerContract / AuthorizationRule / AcceptanceRulePackage / ServiceProduct 四张此前不显，共享层头注定的是「抬齐后一律显」，为的是让操作者把公共半边列出的「未认领路径」与眼前的格对上；(b) 上述四张此前私有 `Field` 一律用 `div` 包，现在只有控件是一组按钮或一排勾选框的格才 `as="div"`（CustomerContract「要求」、AcceptanceRulePackage「缺格怎么读」「适用校验组」「允许来源」），其余回到 `label`——点标题会聚焦控件；(c) SupplierAgreement / CustomerServiceRule / PreAcceptanceFinancialControlPolicy / SettlementPolicy 四张此前私有 `Field` 把格下的问题行渲染成一串 `span`，现在与其余六张一样是 `ul/li`，字一样、颜色字号类一样（CustomerServiceRule / PreAcceptance 行本身与壳上引用那几处带前缀的 `span` 仍在本册，没动）。

**判据 3**——逐张对照认领表（`*FieldPaths`）与 JSX，只有两张有洞：AuthorizationRule 认领了正文根 `'authorizationRule'` 却只显目录根与行（票 17 评审点名那一条），在目录节标题下补一处 `Problems`；CustomerContract 的「不适用依据」与约定行的「指名策略 / 不适用依据」是按选项显隐的格，隐着时认领仍在而没处显，改为由同组显着的那格以 `alsoPaths` 代显、显着时各显各的不重复——载荷只送被选的那格，服务端今天点不到隐格，但认领表说它在、就得有处显。其余八张（含 PricePolicy 的条件格——`ConditionalField` 隐时也显自己那条）逐条核过都有处显。**「渲染了哪些路径」怎么暴露、为什么这样**：各 `*-form.ts` 里与认领表相邻加一份纯声明 `*RenderedPaths`（常量表的四张是常量、随行数长的六张是函数），按 JSX 逐处抄、注释写明每条由哪件显；测试 `publication-form-rendered-paths.test.ts` 用共享层的 `unrenderedClaimedPaths` 对每张表单在空草稿与带行 / 带选项的草稿上各比一次（CustomerContract 三种要求 × 三种行模式，CustomerServiceRule 两种适用对象 × 两张表 × 多行材料）。不从组件渲染结果取证，是因为本包测试只编 `*.test.ts` 与其引到的 `.ts`（`tsconfig.test.json`），`.tsx` 会拖进 react 与 ui-primitives 在 `node --test` 下跑不起来；而认领表本来就是「表单渲染了哪几条路径」的一份声明，第二份从 JSX 一侧抄出来放它旁边，两份对不上即有一条路径两边说法不一。这条测试守的是两份声明一致，JSX 与声明一致仍靠改 JSX 的人同步改声明（各声明注释都写了这一句）与非作者评审对照——红测过一次：把 `'authorizationRule'` 从声明里拿掉，那一条 not ok、其余 194 ok。

**判据 4**：tsc --noEmit 0；run-tests 195 / 195（188 既有一条未改 + 新增 7）；`CommercialPoliciesPage.tsx` 零改动（十张组件的导出名与 props 都没变，页面不需要动 import）。服务端、`publication-draft-api.ts` 线格式、`PublicationDraftFlow` 五步语义零改动；没有任何下拉加内置枚举。
