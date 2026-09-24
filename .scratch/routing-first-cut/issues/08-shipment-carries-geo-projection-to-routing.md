# 08 小包托运请求随路由判断携带地理解析投影（ADR-0075 的 PS 半边）

Category: enhancement
Status: ready-for-agent
Blocked by: 02、07
父票：[psb/04](../../product-strategy-boundary/issues/04-routing-product-strategy-first-cut.md)「接路由证据取数侧」那一步（端到端的前提）
地盘：parcel-shipment 发往 network-routing 的可达性请求与接受交接载荷；network-routing 侧消费方适配器的翻译。
出处：[ADR-0075](../../../docs/adr/0075-customer-address-is-carried-with-the-routing-request.md) 决定一至三；02 的 ADR（投影字段形状）。

## 做什么

1. PS 从委托当前提交版本的客户地址取地理维（02 定的字段），随可达性请求与接受交接一并交出；身份维不过界。
2. NR 侧消费方适配器把它译进 07 定的投影入参。当次解析依据按 ADR-0075 决定三留痕：判断对象引用、所用服务区域版本、所携投影的版本化内容摘要；地址本体不落 NR。

## 不做

- 不在 NR 建读 PS 地址的端口（ADR-0075 决定一）。

## 完成判据

- [ ] 跨上下文用例：同一提交版本两次请求携带的投影摘要一致；地址改版后摘要随之变。
- [ ] NR 的判断记录里能对证当次所用的投影摘要。
