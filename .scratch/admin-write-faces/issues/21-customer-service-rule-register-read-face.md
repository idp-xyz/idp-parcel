# 21 客户服务规则版本发布得出来、管理台看不见：管理台补第八册读面

Category: enhancement
Status: ready-for-agent——2026-09-09 通道 1 代裁立票（用户 12:3x 经 IDP 队列授权「你自决，目标是全部解决」）：票 [06](./06-pre-acceptance-financial-control-policy-versions-have-no-read-face.md) 完成记录末尾「顺带量到」的那件——后端第八册（pc-gaps/05，0023，ADR-0104）已落，`apps/admin-web/src` 里 `CUSTOMER_SERVICE_RULE` 零命中——**立票，取与 06 的 B 笔同形的读面**；票 [18](./18-customer-service-rule-form.md) 的写签等它
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
