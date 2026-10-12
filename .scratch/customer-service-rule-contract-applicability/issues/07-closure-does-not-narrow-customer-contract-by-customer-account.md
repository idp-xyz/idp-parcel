# 07 PC 闭包不按键上的客户账户收窄客户合同候选：同一范围两户各一份合同即两户都`适用冲突`

Category: bug
Status: in-progress——2026-10-12 11:3x 通道 1 认领（用户令「全部独立完成」），分支 `mcp1-csr07`，基 `9ba5d623`。此前：ready-for-agent——2026-10-12 11:2x 用户经 IDP 队列对「分诊」节推荐答「同意」，决定落为 [ADR-0177](../../../docs/adr/0177-customer-contract-names-its-customer-account-and-the-closure-narrows-by-it.md)（Accepted）；实施按其 Consequences 的落地位，走并行会话那条路。此前：needs-info——2026-10-12 11:1x 通道 1 分诊（见文末「分诊」）：是代码没兑现 PC `CONTEXT` 既有不变量的缺口，不是新产品问题；但兑现它要给客户合同的规范化文档加键、动 PCC-1，属难逆转取舍，须先有一篇 ADR。此前：needs-triage——2026-10-12 10:5x 通道 1 立（用户令独立完成），出自 [06](06-demo-contract-02-custom-first-claim-deadline-inherits-materials.md) 灌种子后的真库取证；只读取证，未动 PC 代码。
Blocked by: 无（用户决定已落 ADR-0177）
父票：[spec](../spec.md)
地盘：`internal/partycommercial/domain` 的闭包解析（客户合同那一格的候选选法）；碰 Go，走并行会话那条路。

## 现象（钉 main `8c41be83`）

把 06 那份种子（分支 `mcp1-csr06` 的 `2b76eab4`：第二客户 `SYN-PARTY-SHIPPER-02` / `SYN-ACCOUNT-02`、同范围 `SYN-SCOPE-01` 的第二份客户合同 `SYN-CONTRACT-02` 及其预付结算政策、账户 02 的解析键）用 `seed.sh` 灌进一只干净库，零报错。随后按两户各自登记的解析键行经 PS `CommercialResolutionKeys.FormResolutionKey` 成键、交 PC 真 `ResolveCommercialBasisHandler` 对同库发布登记册解闭包，两户都答 `APPLICABILITY_CONFLICT`，冲突依据含 `CUSTOMER_CONTRACT`。

对照：同一探针对 main 未改的种子（只有 `SYN-CONTRACT-01`）解账户 01，冲突依据只有 `SERVICE_PRODUCT`，没有 `CUSTOMER_CONTRACT`。（`SERVICE_PRODUCT` 那一格是探针成键时没带委托声明的服务产品，真提交会带，与本票无关。）

所以多出的那一格冲突是第二份合同带进来的：**范围里有两份生效的客户合同，闭包对任何一户都选不出唯一的一份**。探针是一次性的、未提交；两只库已删。

## 为什么是缺口

`ResolutionKey` 带 `CustomerAccountID`，注释说「少任何一维，都会让一个客户的解析回答另一个客户」；但客户合同那一格的候选选法今天不看这一维（在 `internal/partycommercial/domain` 里检索 `CustomerAccountID`，除键的定义、最小身份核与身份串之外没有一处读它）。一租户一范围只有一户时看不出来；第二户一进来，第一户就回退。

这也挡住了 ADR-0176 要的那件事本身：合同版客户服务规则按「壳恰指名闭包解出的客户合同」选，闭包连合同都解不出唯一，合同层无从谈起。

## 待裁

合同凭什么认它属于键上那一户。种子里现成的线索有三条，各有代价，本票不代裁：

- 合同正文的 `contractDelegations[].delegator`（`CUSTOMER_ACCOUNT` 类）——今天是资料修订委派，拿它兼作归属会让一份没有委派的合同找不到主。
- 参与方登记里客户关系 `relationships[].basis` 指名合同、`holder` 是货主参与方、客户账户 `customerPartyId` 指回同一参与方——三段间接，跨两册。
- 给合同壳加一条指名客户账户的引用（`references` 新键）——改规范化文档，PCC-1 要按「加 omitempty 键不换号」的先例另核（ADR-0176 决定五的同一纪律）。

## 完成判据（分诊后细化）

- [ ] 同一范围两户各一份合同时，各户闭包解出自己那一份；只有一户时行为不变。
- [ ] 06 的种子灌库后两户闭包不在 `CUSTOMER_CONTRACT` 上冲突。

## 分诊（2026-10-12 11:1x，通道 1，钉 main `a0aae6a1`；只读）

**归类：bug——代码没兑现 [PC `CONTEXT`](../../../docs/domain/party-commercial/CONTEXT.md) 已写的不变量。** 三处原话：

- 不变量一节：「货主客户账户必须明确关联其客户参与方，**合同版本必须明确关联责任法人参与方和货主客户账户**。」
- 「客户合同」词条：「运营企业责任法人与**明确货主客户账户**在一个适用期间内接受的商务和服务条件。」
- 商业依据解析：「针对明确租户、**客户账户**、责任法人候选、服务范围和商业选择锚点，返回唯一适用商业版本集合」。

**代码事实。** 客户合同正文（`domain/customer_contract.go`）没有任何指向客户账户或责任法人的格；版本壳的 `references` 只收商业对象类别，客户账户是参与方身份、不在其列。通用选法 `CommercialRegistry.applicable` 只按租户、类别、范围、声明的服务产品、生效状态与选用区间筛，不读键上的 `CustomerAccountID`。所以「一个范围一份合同」是今天能工作的唯一形态，06 的取证就是第二份进来的那一刻。

**不是「范围即客户」的设计。** 先排除过这一读法：若范围就是客户维，ADR-0176 要的「同一范围并存产品版与合同版、合同版缺行继承同范围产品版」就只能一户一范围、每户把产品与规则包在自己范围里各登一遍——那正是 spec 「Problem Statement」要消掉的「把客户差异写成产品差异」的同族做法；`CONTEXT` 的不变量也明写合同关联客户账户，而不是关联范围。

**推荐（待 ADR）：客户合同正文显式指名货主客户账户，闭包按键上的客户账户收窄客户合同候选。**

- 落位：合同正文加一格客户账户（必填，与 `CONTEXT`「必须明确关联」同强度）；`applicable` 对客户合同那一类多一道「正文指名的账户 = 键上的账户」。结算政策、合同版客户服务规则都已跟着闭包解出的合同走，不必再动。
- 责任法人那一半不同时做：键上的「责任法人候选」是另一维、另一套选法，并进来会把本票变成两件事；ADR 里点名留作后续。
- 难逆转之处：规范化文档加键、PCC-1 摘要随之变。现在没有任何租户的生产数据，已登记的只有演示种子与测试夹具，新键可不走 omitempty、直接必填，演示种子按新摘要重灌即可；但这是对 ADR-0176 决定五同一条纪律（「若要改则按加 omitempty 键不换号的先例另核」）的一次显式取舍，必须写进 ADR，不在实现票里顺手做。
- 不选的两条：合同委派的 `delegator`——委派回答「谁能替谁决定」，一份没有委派的合同就找不到主，拿它兼作归属是一词两义；客户关系 `basis` + `holder` + 账户 `customerPartyId` 三段间接——跨参与方册与发布册两处权威，一户多账户时还要再选一次。

**决定权。** 这一笔动规范化与摘要，按 AGENTS.md「改难逆转技术或产品取舍 → 新 ADR」归 ADR；写不写、按不按推荐写，由用户定，通道 1 不代裁。定了之后本票转 ready-for-agent，实施落在 `internal/partycommercial` 的 domain + 发布批文翻译（`cmd/parcel-commercial`），走并行会话那条路。
