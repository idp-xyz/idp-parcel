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
- `translateForm` 的落点需要一次口径裁决，按本条要求**已先在票面结清**，见下节；实现照它落，
  不在代码里另定。

## 口径裁决：取值名与 `translateForm` 的落点

2026-09-02 由 MCP-4 经 owner 授权裁定。三问，都不许在实现里顺手定：

### 一、第二取值叫什么——取 `LabelChannelServiceForm` / `"LABEL_CHANNEL_SERVICE"`

**用文档里的原词，不自造译法。** [CONTEXT-MAP](../../../docs/domain/CONTEXT-MAP.md) 把两种形态
并列写作「网络服务与**面单渠道服务**形态」，[ADR-0088](../../../docs/adr/0088-label-channel-service-enters-the-first-release-service-forms.md)
的标题用的也是`面单渠道服务`，[PC CONTEXT](../../../docs/domain/party-commercial/CONTEXT.md)
的小节标题同样是`面单渠道服务`。既有取值 `NetworkServiceForm` / `"NETWORK_SERVICE"` 对应
`网络服务`，第二格照同一映射法得 `LabelChannelServiceForm` / `"LABEL_CHANNEL_SERVICE"`。

**「独立」二字不进取值名。** 它在文档里出现的位置是「独立面单渠道服务属于长期产品模型」——
那是在说它是一种可独立销售的产品模型，不是形态名的一部分；CONTEXT-MAP 与 ADR-0088 列举形态
时都不带它。既有测试名 `TestNoServiceProductCanTakeAnIndependentWaybillChannelForm` 里的
`IndependentWaybillChannel` 是测试自己的措辞，不是领域词，不作命名依据。

### 二、面单渠道服务对应哪种网络资格——`NetworkJudgmentNotRequired`

[NR CONTEXT](../../../docs/domain/network-routing/CONTEXT.md) 写明「仅提供面单渠道服务时，
**不得虚构运营企业不控制的端到端网络路由**」；[PC CONTEXT](../../../docs/domain/party-commercial/CONTEXT.md)
写明该服务「不虚构运营企业收寄，其责任来源于面单交易绑定的账号和合同」。两句都指向同一件事：
这个问题不该问，而不是问过了答案是否定的——正是 `NetworkJudgmentRequirement` 注释区分的那一格。

### 三、`不要求`必须携带的依据从哪来——**用它读到的那个形态本身，适配器不另铸**

`NewNetworkEligibility` 要求 `NetworkJudgmentNotRequired` 必携 `EligibilityBasisReference`，
而该类型的注释写明它「指名……所依据的**商业事实**」。因此依据不能由 NR 侧适配器凭空写一个
字面量——那就是消费方替商业侧铸了一条商业事实。

**取 `product.Form().String()` 作为依据**，即 `"LABEL_CHANNEL_SERVICE"`。它就是适配器刚刚
从闭包里读到的那条商业事实本身，复核时答得出「为什么不要求判断网络可达性——因为已采用产品
的服务形态是面单渠道服务」，满足该类型「结果要能按依据维度复核」的要求，且适配器一个字都
没有发明。

**不要用 `"LABEL_ONLY_CHANNEL_SERVICE"`。** 它今天只出现在若干测试的夹具里
（`network_eligibility_test.go`、`assess_parcel_reachability_test.go` 等），是测试自选的字面量，
**不是任何生产常量或文档词**。把测试夹具提拔成生产依据，等于让一个当初随手写的字符串变成
此后所有留痕的依据维度。既有测试保持原样即可——它们验的是构造门，不是这条翻译。

## 完成判据

上列六处同笔改齐，两条守卫用例随之更新并各自说明理由；`gofmt -l` 空、`go build`/`go vet` 退 0、
`go test -count=1 ./...` 绿并注明含不含真库（本票动 SQL CHECK，**真库必须实跑**）。

## 参照

[能力形状盘点](../capability-shape-inventory.md)第四段；ADR-0088 Decision 二；参数登记册 `PAR-COM-12`。
