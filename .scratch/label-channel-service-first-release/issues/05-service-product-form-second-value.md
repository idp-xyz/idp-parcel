# 05 `ServiceProductForm` 第二取值缺席，六处同步扩展

Category: enhancement
Status: ready-for-agent
Blocked by: 无

## 缺口

`internal/partycommercial/domain/service_product.go` 的 `ServiceProductForm` 只有
`NetworkServiceForm` 一个有效取值，`valid()` 直接写成 `form == NetworkServiceForm`。
它的注释写着「独立面单渠道服务是长期产品形态，`PAR-COM-12` 明确它对首发不适用，所以这里
有意不列出它」——而 `PAR-COM-12` 已随 [ADR-0088](../../../docs/adr/0088-label-channel-service-enters-the-first-release-service-forms.md)
改为纳入，**注释的依据句已经过期**。

这不是「加一个枚举值」。[能力形状盘点](../capability-shape-inventory.md)第四段列出同步
扩展的一组（少改一处就是一半说谎）：

- `domain/service_product.go`：`ServiceProductForm.valid()`、`String()`、`NewServiceProduct` 的门。
- `migrations/party_commercial/0008_service_product_form.sql`：`service_product_form_closed` CHECK，
  其注释自己写明「扩展先改领域封闭集，再改这一条」。
- `adapters/postgres/service_product_form.go` 的 `serviceProductFormFrom` 分派。
- `adapters/postgres/commercial_resolution.go` 的 `rehydrateServiceProduct` 分派。
- `internal/networkrouting/adapters/partycommercial/form.go` 的 `translateForm`——**波及面里最实
  的一处**：面单渠道服务恰恰是「不要求网络可达性判断」那一格（依据引用 `LABEL_ONLY_CHANNEL_SERVICE`
  已在 NR 侧测试里出现），而它今天会把该形态判成 `ErrUntranslatableAnswer`。
- `application/register_product_channel.go` 的 `RegisterServiceProductFormCommand.Form` 写路。

## 会变红的守卫（这是设计，不是意外）

- `domain/service_product_test.go` 的 `TestNoServiceProductCanTakeAnIndependentWaybillChannelForm`
  扫完整个 `uint8` 值域；`TestServiceProductFormIsAFacetNotASeparateCatalog` 断言
  `ServiceProductForm(2).String() == ""`。两条随本票一同改，**并说明为什么原断言不再成立**。
- `adapters/postgres/service_product_form_test.go` 文件头登记了两条「今天够不着因此没有用例」
  的防御分支，并写明「第二种形态落地那一天两条同时变得够得着，届时补用例」——**本票就是那一天**。

## 红线

- 第二取值本身**不是实例参数**：`PAR-COM-12` 登记的是已确认范围决策，不是待提供取值。
- `translateForm` 的落点若需要「面单渠道服务对应哪种网络资格」的口径裁决，**先在票面问清再改**，
  不在实现里顺手定。

## 完成判据

上列六处同笔改齐，两条守卫用例随之更新并各自说明理由；`gofmt -l` 空、`go build`/`go vet` 退 0、
`go test -count=1 ./...` 绿并注明含不含真库（本票动 SQL CHECK，**真库必须实跑**）。

## 参照

[能力形状盘点](../capability-shape-inventory.md)第四段；ADR-0088 Decision 二；参数登记册 `PAR-COM-12`。
