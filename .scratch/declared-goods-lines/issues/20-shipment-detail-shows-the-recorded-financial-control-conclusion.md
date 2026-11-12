# 20 委托详情交出受理判断记下的财务控制结论

Category: enhancement
Status: ready-for-agent
Blocked by: 无

依据：[ADR-0379](../../../docs/adr/0379-the-pre-acceptance-estimate-trail-records-product-weight-and-charge-lines-and-is-read-on-the-shipment-detail.md) 决定三；PS CONTEXT（小包托运不拥有金额、余额、冻结）。

## 要做的

- `/shipment-request-views` 的委托详情（`requestDetailBody`）加一格 `financialControl`：最近一次受理判断记下的接受前财务控制采用结果。形状与审核队列 `recordedJudgments.financialControl`（`reviewFinancialControlBody`）相同：outcome、resultId、basis、asOf 各格、jointPassCondition、occupationFormed、items（kind、order、conclusion、basis，以及受限项上的失败处置与责任）。
- 没判过即不带这一格。
- 列表行不带。
- 不带任何金额。
- 读的是审核队列已在用的那份受理判断记录，不另立读法。字段名钉死，管理台票 21 照它写。

## 完成判据

结论 `HELD`、`CREDIT_EXPOSED`、`RESTRICTED`（可用余额不足）、`NOT_APPLICABLE`（合同约定无控制，没有各项与条件）与没判过，在委托详情上各对；与审核队列同一形状（共用一个编码函数）；全量 `go test`（带 PG）通过。

## 不做的

金额（归结算，票 19）。
