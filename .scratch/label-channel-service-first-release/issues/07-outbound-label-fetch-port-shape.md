# 07 朝外的取面单端口不存在，四处形态差异要先进类型

Category: enhancement
Status: draft
Blocked by: 02

## 缺口

`internal/parcelshipment/ports/ports.go` 里与面单交易有关的端口只有两个，**都朝内**：
`LabelTransactionRepository`（存取聚合，其文档注释自称「本口今天没有生产写入方」）与
`LabelTransactionViews`（读面）。[能力形状盘点](../capability-shape-inventory.md)第二段按
`^type \w*(Gateway|Client|Channel|Carrier|Courier|Provider)\w* interface` 扫 `internal/`，
命中没有一个是「向末端渠道发起取面单请求」；`adapters/` 下的目录也全是内部上下文与传输/持久化，
**无渠道方向**。

## 做什么

只定端口形状，**不接任何真渠道**。盘点从公开开发文档量出四处形态差异，它们指向类型而不是取值，
端口要能容下而不被撑破：

1. **取面单与下单是否同一次调用**——FedEx/UPS/USPS 同次回单号与面单；UniUni 分三次
   （`create` 落 DRAFT → `purchase` 才出 `trackingId` → 另一次调用才取面单）。
2. **一次结果是否只有一份图件**——UPS 明确不是（`GraphicImage` 之外另有 `HTMLImage`、
   签名图件、`pdf417`）。
3. **面单粒度是否恒为包裹**——UniUni 的 `labelType=batching` 粒度是**批**不是包裹。
4. **成败信号在哪一层**——UniUni HTTP 一律 200，成败在 body 的 `code` 上，
   **HTTP 状态码不能用来判结果不确定**。

第 4 点与 `02` 定的出向结果代数直接相接：**本票消费 `02` 的结论，不自己另定一套**。

## 红线

- 不写任何渠道账号、字段名、报价、DPI（`PAR-INT-02`/`PAR-SET-03`，实例半边）。
- 不为某一家的形态把端口写窄——上列四处差异是端口形状的验收题，不是可选项。
- 真渠道适配器按[渠道适配缝备忘](../../../docs/design/channel-adapter-seams-design-note.md)
  「一类数据一张票」逐家另立，不在本票。

## 完成判据

端口形状定义在 `internal/parcelshipment/ports/`，四处差异各有类型上的落点；
`gofmt -l` 空、`go build`/`go vet` 退 0、`go test -count=1 ./...` 绿并注明含不含真库。

## 参照

[能力形状盘点](../capability-shape-inventory.md)第二段（含六家渠道公开形态表）；票 `02`。
