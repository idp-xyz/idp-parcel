# 10 `面单继续尝试决定`登记册未建，读面那一格派生自空历史

Category: enhancement
Status: in-progress——MCP-1 认领
Blocked by: 无

## 缺口

[ADR-0084](../../../docs/adr/0084-label-transaction-is-an-independent-aggregate-with-two-level-results.md)
决定六点名本切片不建`面单继续尝试决定`登记册，`internal/parcelshipment/domain/label_transaction.go`
的聚合注释也显式点名。后果是读面 `ports.LabelTransactionParcelRow` 的 `ContinuedAttemptOpen`
**派生自一段真实为空的决定历史**——它今天恒答同一个值，而页面上看不出这一点。

这与本仓治过多次的病同形：「尚未接线」与「已接线但登记册为空」长同一张脸。区别是这一格
连登记册都还没有，所以连「为空」都算不上。

## 做什么

建`面单继续尝试决定`登记册：决定的身份、它挂在哪一笔交易与哪一个包裹上、决定内容的封闭集
（继续尝试 / 不再尝试 / 待定，具体格数按 PS CONTEXT 与 ADR-0084 的原词，**不新造**），以及
`ContinuedAttemptOpen` 据以派生的规则。

## 红线

- **不新造领域语言**：决定的名字与格数取 CONTEXT/ADR-0084 原词，要改先回 `CONTEXT.md`
  （[AGENTS 改文档](../../../AGENTS.md#改文档)）。
- 登记不可覆盖，更正走版本链。
- 读面那一格改成据实派生之后，**如果登记册为空要说得出「没有人作过决定」**，不得折成
  「不继续」——两者续办动作相反。

## 完成判据

登记册的领域、端口、持久化与读面派生四处齐；`ContinuedAttemptOpen` 不再派生自空历史；
`gofmt -l` 空、`go build`/`go vet` 退 0、`go test -count=1 ./...` 绿并注明含不含真库
（**本票大概率动迁移，真库必须实跑**）。

## 参照

[能力形状盘点](../capability-shape-inventory.md)第一段；ADR-0084 决定六。
