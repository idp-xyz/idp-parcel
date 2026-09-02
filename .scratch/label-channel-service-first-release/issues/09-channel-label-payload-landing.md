# 09 渠道返回的面单载荷在领域、库与读面都没有落点

Category: enhancement
Status: in-progress——MCP-1 认领（`04`/`07` 均已 resolved，阻塞解除）
Blocked by: 04（已 resolved）、07（已 resolved）

## 缺口

面单交易聚合上与结果有关的字段只有 `ChannelParcelIdentifier`（包裹级标识）与
`ChannelResultReasonReference`（结果原因）；`LabelTransactionParcelResult` 只有
`parcel`/`accepted`/`identifier`/`reason` 四项，读面 `LabelTransactionParcelRow` 同形。

**全仓 `internal/` 下 `PDF`、`ZPL`（含小写）零命中**——这个数本身是论点，实测于 `957e768`。
`migrations/parcel_shipment/0010_label_transaction.sql` 的列与快照文档同样没有载荷或其引用。

也就是说：渠道把面单给回来了，本仓**没有地方放它**。

## 为什么被两票阻塞

- `04` 决定要留几份：只留渠道成品、还是成品加加工件两份。
- `07` 决定载荷的形态维度：一次结果可能有**多份图件**（UPS），粒度可能是**批**而非包裹
  （UniUni），格式可能是栅格也可能是指令流。落点形状要容下这些，而它们在 `07` 才定形。

先做本票会得到一个按单份、包裹粒度、单一格式写死的列，然后在 `07` 落地当天推翻。

## 做什么

定「渠道返回载荷」在三处的落点形状：聚合、库（`0010` 之后的迁移）、读面。**只定形状不定格式**。
一个要正面回答的问题：载荷**本体**入库，还是只入引用而本体去对象存储——这决定迁移与读面的
形状，且难逆转。

## 红线

- **具体格式取值属实例半边**（哪家给 PDF、哪家给 ZPL、DPI 多少），机制里不内置任何一个
  （[ADR-0088](../../../docs/adr/0088-label-channel-service-enters-the-first-release-service-forms.md) Decision 五）。
- 不可覆盖：同一次结果的载荷不得被后来的调用顶替，更正走版本或追加。

## 完成判据

三处落点各有形状并同笔落地；`gofmt -l` 空、`go build`/`go vet` 退 0、
`go test -count=1 ./...` 绿并注明含不含真库（**本票动迁移，真库必须实跑**）。

## 参照

[能力形状盘点](../capability-shape-inventory.md)第一段与第五段；票 `04`、`07`。
