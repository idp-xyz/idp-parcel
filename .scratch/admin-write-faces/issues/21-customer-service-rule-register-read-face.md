# 21 客户服务规则版本发布得出来、管理台看不见：管理台补第八册读面

Category: enhancement
Status: resolved——读面落地：`?kind=CUSTOMER_SERVICE_RULE` 第十格（分支 `mcp3-awf21`，基 `74ef0da8`：`4f956057` 管理台一笔；2026-09-09 14:2x，通道 3 接续会话接 13:0x 全通道中断前的同通道现场续做；完成记录在文末；main 上的 SHA 待通道 1 重放后对照）。此前 in-progress——2026-09-09 13:0x 通道 3 认领（task-2306e16d，分支 `mcp3-awf21`，基 `74ef0da8`；接着同分支做票 18）；此前 ready-for-agent——2026-09-09 通道 1 代裁立票（用户 12:3x 经 IDP 队列授权「你自决，目标是全部解决」）：票 [06](./06-pre-acceptance-financial-control-policy-versions-have-no-read-face.md) 完成记录末尾「顺带量到」的那件——后端第八册（pc-gaps/05，0023，ADR-0104）已落，`apps/admin-web/src` 里 `CUSTOMER_SERVICE_RULE` 零命中——**立票，取与 06 的 B 笔同形的读面**；票 [18](./18-customer-service-rule-form.md) 的写签等它
Blocked by: 无（后端 `?kind=CUSTOMER_SERVICE_RULE` 已在 main；本票只动管理台）

## 缺口

发布口对象类别 `CUSTOMER_SERVICE_RULE`（`CommercialObjectKind` 第十类，pc-gaps/04 纳入封闭集）的版本可以发布成功，后端目录读面第八册
`GET /commercial-policies?kind=CUSTOMER_SERVICE_RULE` 也能列它（0023 正文：`serviceProduct | customerContract` 恰一、`responsible`、`scope`、
`claimDeadlines[{kind, startEvent, days, calendar}]`、`minimumMaterials[{claimKind, materials[]}]`），但管理台「商业规则与策略」页的
`CommercialPolicyKind` 没有这一格——操作者发布之后在管理台找不到自己刚发的那一版。与票 06 立票时的接受前财务控制策略册同形，
只少了后端那半（06 的 A 笔本票不需要）。

## 为什么现在立

伞票 [07](./07-commercial-publication-operator-main-paths-per-register.md) 的落点判据是「写签跟着读签走」（票 03）：票 18 的表单
摆不到任何一页，07 转 resolved 卡在 18，18 卡在本票。ADR-0101 决定八要求每册在自己的实施票里写明运营主路径，第十册不能是
例外；把「发布得出来、管理台看不见」记成长期事实与 06 的裁决（MCP-3 2026-09-04，取②不取①）正面冲突。

## 完成判据

1. 管理台 `CommercialPolicyKind` 加 `CUSTOMER_SERVICE_RULE` 一格；两个 Record（行体 + 正文节）照后端 HTTP 行体的键名逐字（`contentRegistered` +
   `content{...}` 节，与第九册同形）；本册列向：适用对象一栏（产品 / 合同恰一，显哪个就是哪个，不合成「对象」一词）、责任方、范围、
   期限表合成一栏（种类 · 起算事件 · 天数 · 日历，按后端给的顺序）、材料表合成一栏；封闭集标签只给 `presentation.ts` 里已有词表的，
   没有词表的原词直显、不自造译法；册名「客户服务规则」；来源提示句照 06 B 笔的写法。
2. 票 03 那句「今天没有册可看」的提示若还点到本册，同笔改成指向本册（06 的边界句同形）。
3. `policy-rows.test.ts` 补本册三态（壳无正文 / 有正文 / 契约不合点名）与两张子表的合栏转写。
4. `node node_modules/typescript/bin/tsc --noEmit` 退 0；`node scripts/run-tests.mjs` 全绿（注意 `npx tsc` 会解析到 npm 上的占位包且退 0，不算验过）。
5. 完成后把票 18 的 `Blocked by` 里「管理台客户服务规则册读面票（未立）」换成本票编号并转 `ready-for-agent`——同笔。

## 边界

不动后端（第八册读面、0023、ADR-0104）；不动发布口、不动领域；不碰 VE 那侧的通知义务 / 索赔类型覆盖两本册（所有权归 pc-gaps/05 记着的 ADR，
本票只列 PC 这一侧的正文）；`cmd/parcel-api/endpoints.go` 不加行（复用 `?kind=`）。

## 验证强度要求（作者）

admin-web 只跑 tsc + run-tests；Go 侧零改动不跑。共享文件（`policy-rows.ts` / `api.ts` / `presentation.ts` / `CommercialPoliciesPage.tsx`）
改前占号、只加自己一格。

## 完成记录（2026-09-09，通道 3；分支 `mcp3-awf21`，基 `74ef0da8`，已推 origin——通道 1 重放进 main）

| 笔 | SHA | 内容 |
|---|---|---|
| A | `4f956057` | 管理台第十格：`api.ts` `CommercialPolicyKind` 加 `CUSTOMER_SERVICE_RULE`，`CustomerServiceRuleRecord` / `ClaimDeadlineRecord` / `MinimumMaterialsRecord` 照 HTTP `customerServiceRuleBody` 键名逐字（`contentRegistered` + 可缺 `content` 节；`serviceProduct` / `customerContract` 恰一在场；期限项键 `kind / startEvent / durationDays / calendar`，材料项键 `claimKind / materials`），判别联合加一支；`policy-rows.ts` 本册列向十一栏（规则对象/版本、适用范围、状态、正文、适用对象恰一栏、责任方、规则范围、索赔期限合栏、最低材料合栏、有效区间、发布时间），`contentRegisteredCell` 泛化为策略册与本册共用，新出口 `serviceRuleAppliesToCell` / `serviceRuleClaimDeadlinesCell` / `serviceRuleMinimumMaterialsCell`，`rowsOf` 加一支；`presentation.ts` 册名「客户服务规则」、来源提示句、发布签 kind 提示九词改十词并写 `CUSTOMER_SERVICE_RULE → 客户服务规则册`；`CommercialPoliciesPage.tsx` 描述加一句「适用对象按服务产品或客户合同恰一上列」；`policy-rows.test.ts` 本册五条 + 提示句那条九词改十词 |

**出处**：四份现场（`api.ts` / `policy-rows.ts` / `policy-rows.test.ts` / `presentation.ts`，+278/−5，mtime 13:05–13:08:51）是通道 3 上一会话在 13:0x 全通道中断前写在树上、未跑过 tsc / run-tests 的在途产出。接续会话按 parallel-sessions「接手别人在途产出」先自列本票判据该钉什么（词表与列集、只有壳、登了正文含子表顺序与空数组、布尔真而节缺、两键皆无 / 皆有、状态标签），再读 diff 与五条测试对判据（对判据不对条数）：全部对上；现场跑 tsc 退 0、run-tests 176 pass / 1 fail，红的那条是发布签提示句尚未加第十类（判据 2 的活）。接手方补 `presentation.ts` 提示句、`policy-rows.test.ts` 提示句那条加一词、`CommercialPoliciesPage.tsx` 一句描述，四份现场一字未改，一笔提。

**判据逐项**：1 ✓（格、两个 Record 照 HTTP 键名逐字、适用对象恰一栏显哪个就是哪个、责任方、规则范围、期限与材料两合栏照后端顺序、期限种类与引用词无词表原词直显、册名、来源提示句照 06 B 笔写法）；2 ✓（票 03 那句「没有册可看」在 06 已退场，本票改的是发布签提示句：九词 → 十词，`CUSTOMER_SERVICE_RULE → 客户服务规则册`）；3 ✓（三态 + 两键皆无 / 皆有 + 两张子表合栏转写与空数组「无客户差异」，五条）；4 ✓（`node node_modules/typescript/bin/tsc --noEmit` 退 0；`node scripts/run-tests.mjs` 177 pass / 0 fail）；5 ✓（票 18 的 Blocked by 在立票时已写本票编号，本笔把它的状态转 ready-for-agent 并同笔转 in-progress——同通道同分支接着做）。

**边界对照**：不动后端、发布口、领域 ✓（Go 侧零改动）；不碰 VE ✓（册名与来源句只列 PC 这一侧正文，索赔类型 / 材料 / 起算事件按引用原词）；`cmd/parcel-api/endpoints.go` 未加行 ✓。

**共享文件 numstat**（`4f956057` 相对 `1d4a308c`）：`api.ts` +43/−2、`policy-rows.ts` +81/−3、`presentation.ts` +10/−4、`CommercialPoliciesPage.tsx` +1/−1、`policy-rows.test.ts` +150/−1。本册加行一律排在接受前财务控制策略册之后。

**验证强度**：只有 admin-web，`apps/admin-web` 下 tsc 退 0、run-tests 177 pass / 0 fail（node_modules 是指向主树的目录联接，未跑 install）；Go 侧零改动未跑；清点不重生成（无 `.go` / 迁移 / 端点变化，生成器数的文件面不动）。

**要通道 1 落的装配行**：无。

## Comments

- 2026-09-09 14:2x · 通道 3（接续会话）：收口。一笔 A；票 18 同笔转 in-progress。
