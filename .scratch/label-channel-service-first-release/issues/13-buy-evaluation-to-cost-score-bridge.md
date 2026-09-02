# 13 `BUY` 评价到成本准则分值之间没有桥，也没有逐候选批量评价口

Category: enhancement
Status: draft
Blocked by: 01

## 缺口

两头都齐，中间断开：

- **`BUY` 侧评价齐备**：`internal/parcelpricing/domain/value_objects.go` 的
  `PricingDirectionBuy` 与 `PricingPurposeSupplierCost` 配对，装载按方向隔离
  （`adapters/postgres/price_card_catalog.go` 的 `LoadApplicable`），评价用例在
  `application/evaluate_pricing.go`；计费重与体积系数机制在 `domain/weight_rounding.go`、
  `domain/dimensions.go`，价表族在 `domain/rate_families.go`。
- **比较器齐备**：`SelectRouteCandidate` 按准则序作字典序比较，`CriterionScore` 越小越优。

断开的是：`CriterionScore` 的注释明写「折算发生在事实形成处」，而**「把某候选的
`PricingEvaluation` 总价折成 `COST` 准则分值」这一步今天没有实现方**；`parcelpricing` 侧
也没有「一次输入对多份 `BUY` 价卡逐份评价」的批量口。

## 做什么

两件：

1. **批量评价口**：一次输入（同一批包裹尺寸重量）对多份 `BUY` 价卡逐份评价。
   `PAR-NET-16` 要求「各候选按自己的体积系数与进位算出计费重之后再比总价」——**逐候选各
   算各的计费重**，不是算一次再套不同费率。这是本票最容易做错的一格。
2. **成本分值桥**：把逐候选的 `PricingEvaluation` 折成 `COST` 准则分值。

## 必须守住的一格

`PAR-NET-16` 硬约束：「候选评价为待判断或不可计价时该候选出局，**不得以零金额顶替**」。
`parcelpricing` 有 `EvaluationPending` 这一格（`domain/evaluation_test.go` 的
`DIMENSIONS_REQUIRED`/`RATE_NOT_FOUND`），本票要消费它——**折成零分就是把「算不出」变成
「最便宜」**，那会让不可计价的候选每次都赢。

## 红线

- 不填任何价卡、体积系数、进位分段、金额（`PAR-SET-03`，实例半边）。
- 出局判断放在哪一层由 `01` 裁定，本票照裁定落，不自定。

## 完成判据

批量评价口与分值桥各有实现与测试，含「待判断/不可计价候选出局且不以零金额顶替」的用例；
`gofmt -l` 空、`go build`/`go vet` 退 0、`go test -count=1 ./...` 绿并注明含不含真库。

## 参照

[能力形状盘点](../capability-shape-inventory.md)第三段；票 `01`；参数登记册 `PAR-NET-16`。
