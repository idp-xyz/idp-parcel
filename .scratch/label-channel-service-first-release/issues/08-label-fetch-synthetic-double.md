# 08 取面单合成替身：把结果不确定、部分成功、批粒度三条走通

Category: enhancement
Status: in-progress——MCP-1 认领（`07` 已 resolved，阻塞解除）
Blocked by: 07（已 resolved）

## 为什么要一个替身

`07` 定出端口形状之后，没有任何东西证明那个形状**装得下**它声称要装的三种情形。真渠道
接不了（账号与授权属 `PAR-INT-02`，实例半边，租户尚未到位），所以证据只能来自合成替身。

盘点第二段点名的三条正是最容易在类型上被压平的：

1. **结果不确定**——`LabelTransaction` 有 `MarkResultUncertain` 与
   `LabelTransactionResultUncertain` 一格，替身要能造出「对端答了但答案不确定」，
   证明它没有被折成「失败」。
2. **部分成功**——聚合有 `LabelTransactionPartiallySucceeded`，逐包裹结果经
   `matchResultsToCoverage` 对齐覆盖范围；替身要造出一笔交易里有的包裹成、有的不成。
3. **批粒度面单**——UniUni 的批面单粒度不是包裹。替身要证明端口能表达「这份图件对应的
   不是某一个包裹」，而不是被迫拆成 N 份假的包裹级图件。

## 红线

- 替身**只记 `S`**：隔离合成证据不得记成任何更高等级（[AGENTS 红线](../../../AGENTS.md#红线)）。
- 不造任何「开发用」的真实渠道凭证或账号。
- 替身不进生产装配——它是取证件，不是「先用着的实现」。

## 完成判据

三条各有测试证据；`gofmt -l` 空、`go build`/`go vet` 退 0、`go test -count=1 ./...` 绿并注明
含不含真库。若某一条**装不下**，如实记「端口形状不足」并回 `07`，不改测试去迁就形状。

## 参照

[能力形状盘点](../capability-shape-inventory.md)第一段与第二段；
`internal/parcelshipment/domain/label_transaction.go` 的状态封闭集与 `RecordChannelResultSpec`。
