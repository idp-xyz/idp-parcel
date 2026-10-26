# 92 SA 外部更正把事实金额改到已核销合计以下：照采，不标超核、不重算

Category: bug
Status: ready-for-agent——R2（上线后尽快）：外部更正照采是对的（外部系统拥有资金事实），可采下来之后既不说出这条事实已经超核，也不重算它映射到的客户应收作用域；读面上的累计未分配为负是唯一的信号，入账余额停在更正前的金额上，直到同作用域下一次映射、核销或撤销才跟上。看得见但要人去找，不答错可用余额以外的数。2026-10-26 票 [code/12](./12-funds-mapping-has-no-compensation-or-recovery-targets.md) 第二轮评审 N3 立（协调方裁定本批不改代码、立票），分支 claude-code12。
Blocked by: 无
出处：code/12、code/46 第二轮隔离评审 N3；取证于分支 claude-code12。

## 现象

- UC-SA-005 `AT-SA-114`「外部事实金额被更正 → 保留原版本，按新有效版本重算差额和核销」；SA CONTEXT「入账余额在形成它的那次 `UC-SA-005` 事务里……重算并推进」。
- 代码：`application.MapExternalFundsHandler.CorrectFact` 从链头形成新版本、落版本行、交采用信封，不读这条事实上未撤销核销的合计，也不重算它映射到的作用域。
- 更正后的金额低于未撤销核销合计时：
  - 已采用资金事实册读面的 `UnappliedMinor`（事实金额 − 未撤销核销合计）为负，`parcel-settlement-register` 核销类答复把它说成「累计超核 X」——这两处是今天仅有的信号，都不截成 0。
  - 对同一事实再交核销一律答 `APPLICATION_IMBALANCE`，但没有一格告诉操作员该先撤哪一笔。
  - `CustomerReceiptPostings.CustomerDirectionNets` 的 P 取链头金额，可更正这一步不推进，`operational_balance` 停在更正前的数。
- 核销与更正在事实身份行上排队（`SettlementApplications.LockFundsFactApplications` 头注），这只保证核销读到更正后的金额，不保住「已核销合计不超过事实金额」。

## 三类

机制。无产品策略；无租户取值。

## 规模

S–M。

## 最小的诚实修法

- `CorrectFact` 先对事实身份行 `FOR UPDATE`，与核销同一把锁、同排在资金侧锁序全序首位（`ports.FundsFactApplicationLock` 头注）；插版本行时外键那把 KEY SHARE 来在读之后，晚了。
- 取锁后求未撤销核销合计，然后照采。
- 新金额低于合计时，答复带出超核金额；HTTP 照 ADR-0022 放在响应体，不改状态码。
- 同一事务里重算该事实映射到的对账单与账户作用域（走 `recomputeScope`，锁序取全序的子序列）。
- 不自动撤销任何核销：撤哪一笔是人的判断。

## 要裁的

- 超核那一格叫什么：是采用编排的一个结果格，还是采用结果上的一个字段（采用本身成立）。
- 要不要一本待处理册，让超核的事实列出来等人撤销，而不是只靠读面上的负数。
