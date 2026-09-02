# 06 面单交易写侧执行器全缺：聚合方法齐备而编排一环没有

Category: enhancement
Status: ready-for-agent
Blocked by: 无

## 缺口

`internal/parcelshipment/application/` 下**一个面单交易用例都没有**（实测于 `957e768`，
该目录的执行器全是委托/受理/取消/终局侧）。`internal/architecture/production_wiring_baseline.txt`
把 `EstablishLabelTransaction` 与 `EstablishPriorLabelTransactionLink` 显式登为「无生产调用方」，
并注明这是设计而非遗漏——[ADR-0084](../../../docs/adr/0084-label-transaction-is-an-independent-aggregate-with-two-level-results.md)
立聚合时就明写「写入方缺席是设计」。

[ADR-0088](../../../docs/adr/0088-label-channel-service-enters-the-first-release-service-forms.md)
把这条缝从墙后之物变为首发主链路，**「设计如此」的理由随之失效**：它 Consequences 原话是
「它立下的面单交易聚合正是本决定落地时要接上写入方的那一个」。

## 做什么

补「委托提交 → 建立交易 → 提交渠道 → 记录结果 → 追加后续动作」这条编排链，落
`internal/parcelshipment/application/`。聚合侧已齐，本票不改领域：

- 建立：`EstablishLabelTransaction`（`EstablishLabelTransactionSpec` 七类引用逐项过构造门）。
- 提交：`SubmitToChannel`；结果不确定：`MarkResultUncertain`。
- 记录结果：`RecordChannelResult`（两层结果一次写下，逐包裹结果经 `matchResultsToCoverage` 对齐覆盖范围）。
- 后续动作：`AppendFollowUpAction`（`ChannelVoidAction` / `ChannelRefundAction` / `ChannelReplacementAction`）。
- 重试/替代：`EstablishPriorLabelTransactionLink`，在**新**交易出生时建立且要求原交易已定案。

写入代数已在 `ports`：`LabelTransactionInsertOutcome` 与 `LabelTransactionSaveOutcome` 分立，
编排要把两者分别答完，不压成一格。

## 与相邻票的边界

- **不接真渠道**：提交渠道那一步的出向调用形状归 `07`，本票对它只留缝，用替身走通。
- **不定载荷落点**：渠道返回的 PDF/ZPL 往哪放归 `09`。
- 本票只做编排，`RecordChannelResult` 收到什么由替身给。

## 红线

- 不可覆盖、更正走版本链、停用走状态推进；定案 `Finalized()` 是派生谓词，**不要为它加存储字段**。
- 不填任何渠道账号、字段名、金额（实例半边）。

## 完成判据

编排链五步各有用例与测试；`internal/architecture/production_wiring_baseline.txt` 里那两条
「无生产调用方」的登记随之更正（**改它是本票的一部分，不是顺手**）；`gofmt -l` 空、
`go build`/`go vet` 退 0、`go test -count=1 ./...` 绿并注明含不含真库。

## 参照

[能力形状盘点](../capability-shape-inventory.md)第一段；ADR-0084；
`internal/parcelshipment/domain/label_transaction.go`。
