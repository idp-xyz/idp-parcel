# 25 估价因申报不全没形成时等客户补充

Category: enhancement
Status: needs-triage
Blocked by: 无

依据：[ADR-0380](../../../docs/adr/0380-an-estimate-the-price-card-side-cannot-form-waits-for-operator-registration-and-the-attempt-records-why.md) 决定一与越权风险点「申报不全照旧内部重试、用完放弃」。

## 现状

结算答 `ESTIMATE_DECLARATION_INCOMPLETE`（申报缺了计价要的某一格，例如价卡绑了始发维度目录而没报寄件邮编）时，小包托运照旧内部重试，整笔回滚、重投用完放弃，委托悬着、看不出缺什么。

## 要定的

- 怎样接进受理判断的客户补充那一路：续办方是客户，缺项说清哪一件缺哪一格（同「必要资料」组的缺项，ADR-0376）。
- 计价的「申报缺哪一格」怎样经结算带回来，今天只有一个原因码。

## 不做的

价卡一侧的未形成（票 23）。
