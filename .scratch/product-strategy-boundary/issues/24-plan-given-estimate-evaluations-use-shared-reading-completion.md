# 24 给定价卡的试算走共用补齐读数：parcel-pricing 应用层开一个按调用方给的价卡形成试算评价的入口

Category: enhancement
Status: ready-for-agent——2026-10-12 通道 3 随[票 06](./06-ps-acceptance-and-label-selection-judgment-methods.md)第 2 项分诊立（派单 `task-85d7350d` ← 通道 1）。不等 [ADR-0179](../../../docs/adr/0179-pre-acceptance-control-amount-is-estimated-by-settlement-from-per-parcel-sell-trial-evaluations.md)：那篇正文与 Alternatives 里的每一种落法，都要一份对已知价卡走共用补齐读数的试算
Blocked by: 无
地盘：`internal/parcelpricing/application`（新入口与其用例）。`readingCompletion` 与 `estimateInput` 已在本包，复用、不改行为。不碰 `internal/parcelshipment`、`internal/settlementaccounting`、`cmd/`。
出处：票 06「第 2 项分诊」；[ADR-0152](../../../docs/adr/0152-operator-estimate-is-a-pricing-use-case-computed-not-recorded-and-never-handed-to-settlement.md) 决定二、三、五。

## 现象（读码，钉 main `ff3b01fa`）

- PS 的 `EstimationAmountSource` 直接调纯函数 `ppdomain.EvaluatePricing`，输入里不带序列取值与目录读数。卡上绑了序列或分区目录时，纯函数落 `PENDING`（缺序列取值即 `REFERENCE_SERIES_UNRESOLVED`），评价没有合计，金额形不成。演示售价卡 `SYN-PLAN-CN-SG-01` 绑着燃油序列 `SYN-SERIES-FUEL-01`。
- 本包里走 `readingCompletion` 的是正式评价（`EvaluatePricingHandler`，入册且无条件交付）与运营试算（`FormEstimateEvaluationsHandler`，按方向、范围、时点取全部适用价卡，证据一律 `S`）。前者会把结果交给结算；后者不收「就用这一张卡」，也不收调用方声明的证据层级。

## 做什么

- 开一个入口：调用方交一张已解析的价卡版本、一组试算声明与证据层级。每件声明是试算对象引用、实重、尺寸（可缺）、分区或邮编路线、计价基准时点；结算币种可缺。入口逐件按 ADR-0152 决定五的给法造输入快照（复用 `estimateInput`，不另写一份），走 `readingCompletion`，再由纯函数形成评价，逐件交回评价或缺项。
- 结构上不入册、不交付：依赖里没有评价库写口，也没有 `EvaluationHandoff`（ADR-0152 决定二的手法）。
- 证据层级由调用方声明，不给默认。受理前估价的输入是客户申报，不是假设包裹，所以 UC-PP-001 那一格「一律 `S`」不扩用到这里。
- 评价标识由调用方给的试算对象引用确定性派生：同一声明、同一张卡得同一标识，可比对可复算。
- 选卡不在本入口，卡由调用方给。ADR-0179 决定三若接受，调用方经 `PriceCardInForceResolver` 解析；若改走价格政策的定价方案绑定，调用方按引用装载。两种都用这一个入口。

## 不做

- 不改 `FormEstimateEvaluationsHandler`、`EvaluatePricingHandler` 的行为与答案代数。
- 不接消费方。PS 的 `EstimationAmountSource` 改用本入口，或结算的估价端口调用本入口，随[票 25](./25-pre-acceptance-estimate-formed-by-settlement.md)按 ADR-0179 的结论做。
- 不定估价形态、选卡口径与计价范围来路，那是 ADR-0179 的事。

## 完成判据

- 绑燃油序列的价卡：序列在用且基准时点有期次时，评价 `COMPLETED`，合计含燃油项；序列没登记或没复核时落 `PENDING`，解释里带补齐说明，说明与正式评价同一份。
- 绑分区目录的价卡：给邮编路线时分区由目录解析；只给分区、不给路线时，按 ADR-0152 决定五逐件答缺项。
- 同一声明同一张卡调两次，得同一评价标识与同一结果；依赖里没有写口与交付口（构造函数签名或编译期断言可证）。
- `go build ./...`、`go vet ./...` 退 0；本包用例绿；新写的 `.go` 过一遍 `gofmt -w`。
