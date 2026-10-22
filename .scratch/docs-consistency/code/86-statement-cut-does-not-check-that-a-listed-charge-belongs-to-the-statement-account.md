# 86 截单不核命令列出的费用是否属于这张对账单的结算账户，迟到费用纳入也不核

Category: bug
Status: needs-triage——建议 R2（截单发布已在生产装配，碰钱：别的账户的费用可以进这张单，不丢不重但错户出账）。2026-10-21 票 [docs-consistency/code/11](./11-statement-cut-does-not-check-the-settlement-attribution-date.md) 作者会话读码时发现并立（用户「你作为业务和系统专家自决」授权），分支 claude-code11。
Blocked by: 无
上下文：SA（结算与经营核算），领域层与截单编排。
出处：code/11 实现时读码，钉在分支 claude-code11 的基 `e0d0909cf`。

## 现象（读码，只记 `S`）

- `domain.CutStatementDraft` 核费用阶段、归属日、结算币种与去重，不核费用确认时固定的结算账户（`ConfirmedChargeFacts.SettlementAccount`）是否等于草稿的 `Account`。
- `domain.IncludeLateChargeInSubsequentPeriod` 同样不核迟到费用的结算账户是否等于原对账单的账户。
- code/11 之后，截单对没有归属日的费用按**费用自己所属**结算账户的对账安排当场判（ADR-0269 决定二「这件费用所属结算账户的对账安排」）；费用属于别的账户时，它照别的账户的日界判出归属日，再按本单的本期最后一个归属日纳入，两个日界混在一张单上。
- 本组应用层夹具里，截单命令的账户与费用事实上的账户本就不同（`settlement-account-1` 对 `ACCOUNT/syn-1`），今天照样发布成功。

## 规则

- SA CONTEXT「结算账户」词条：结算账户在一个运营责任范围内归集同方向费用、余额、对账和核销；每条确认费用固定结算账户，不能事后推断。
- UC-SA-003「权限、隔离与审计」：按结算账户隔离；「一致性、幂等与并发」：同一费用被两个账户同时纳入形成归属冲突。

## 三类

机制。产品策略：无。租户取值：无。

## 规模

S：`CutStatementDraft` 与 `IncludeLateChargeInSubsequentPeriod` 各加一道账户相等的门，截单编排把它译成整笔未受理（指名了别的账户的费用是提交矛盾）；改应用层夹具，让命令的账户与费用事实上的账户一致；补用例。

## 最小的诚实修法

1. 领域门：费用 `ConfirmedFacts().SettlementAccount` 不等于草稿或原单的账户即拒。
2. 截单编排在判归属日之前先核账户，免得照别户的对账安排判出一个归属日并落库。
3. 用例：命令列出别户费用 → 未受理、不形成草稿、不落归属日；迟到纳入同理。

## 相关

- 票 code/11（按归属日纳入）、ADR-0269 决定二、ADR-0158（结算账户五格固定）。
