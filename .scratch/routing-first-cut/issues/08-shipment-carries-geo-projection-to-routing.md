# 08 小包托运请求随路由判断携带地理解析投影（ADR-0075 的 PS 半边）

Category: enhancement
Status: resolved——2026-10-01 进 main（`1fd88812→629217a1`，ADR-0075 / ADR-0148，无新 ADR）。Blocked by 02、07 已在 main
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

- [x] 跨上下文用例：同一提交版本两次请求携带的投影摘要一致；地址改版后摘要随之变。
- [x] NR 的判断记录里能对证当次所用的投影摘要。

## Comments

**评审 ← 通道 1 · 钉 `1fd88812` · 2026-10-01**

- **阻断**：无。不立新 ADR。迁移 `0014`，判断记录只加投影摘要与服务区域版本，不存地址本体。邮编原样、不去空白。架构门禁 PASS。点名用例在 `parcelshipment/adapters/networkrouting` 与 `networkrouting/adapters/parcelshipment` PASS。
- `ports.go` 与 network-routing `CONTEXT.md` 与 rfc/05 自动合并，两边的句子都在。
- **结论：可重放**。

**进 main 记录（2026-10-01，通道 1）**

重放到 `4f4ff71e` 之上，零冲突：`1fd88812→629217a1`，清点 `27c33efe`。全量 `go test -p 1 -count=1 ./...`：135 ok / 0 FAIL。分支作封存出处。
