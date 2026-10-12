# 07 PC 闭包不按键上的客户账户收窄客户合同候选：同一范围两户各一份合同即两户都`适用冲突`

Category: bug
Status: needs-triage——2026-10-12 10:5x 通道 1 立（用户令独立完成），出自 [06](06-demo-contract-02-custom-first-claim-deadline-inherits-materials.md) 灌种子后的真库取证；只读取证，未动 PC 代码。
Blocked by: 无（分诊时先定「合同凭什么认它是这一户的」，那一步可能要 ADR 或用户决定）
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
