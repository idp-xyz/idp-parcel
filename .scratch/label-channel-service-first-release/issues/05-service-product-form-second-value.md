# 05 `ServiceProductForm` 第二取值缺席，六处同步扩展

Category: enhancement
Status: resolved——八处同步扩展与两条守卫用例同笔落地，见文末「完成记录」
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
- `service_product_form_closed` CHECK。**⚠ 不是就地改 `0008`——见下面「迁移的改法」一节，
  本条原写「改 `migrations/party_commercial/0008_service_product_form.sql`」是错的。**
- `adapters/postgres/service_product_form.go` 的 `serviceProductFormFrom` 分派。
- `adapters/postgres/commercial_resolution.go` 的 `rehydrateServiceProduct` 分派。
- `internal/networkrouting/adapters/partycommercial/form.go` 的 `translateForm`——**波及面里最实
  的一处**：面单渠道服务恰恰是「不要求网络可达性判断」那一格（依据引用 `LABEL_ONLY_CHANNEL_SERVICE`
  已在 NR 侧测试里出现），而它今天会把该形态判成 `ErrUntranslatableAnswer`。
- `application/register_product_channel.go` 的 `RegisterServiceProductFormCommand.Form` 写路
  （实测为直通，`command.Form` 原样交 `NewServiceProduct`，**无需改动**）。

## 盘点漏掉的两处（2026-09-02 实做时查出）

立票时按能力形状盘点第四段写成「六处」，并加了一句「少改一处就是一半说谎」。**实做时全仓
重扫 `ServiceProductForm|NETWORK_SERVICE` 又查出两处名称镜像，盘点第四段没有列**——它盘的
范围是 `internal/partycommercial` 与 NR 适配器，没有扫 `cmd/` 与 `apps/`：

- **`cmd/parcel-commercial/register_products.go` 的 `serviceProductFormFromName`**：受控 CLI
  的字符串→形态镜像，第三处 `switch`。不改的话面单渠道服务产品**根本登记不进去**——
  它是今天唯一有真实实现的写入路径。
- **`apps/admin-web/src/pages/party/presentation.ts` 的 `serviceFormLabels` 与
  `service-product-form` 提示句**：词表此前列过 `LABEL_CHANNEL_SERVICE` 又撤下，注释写明撤下
  理由是「那个取值服务端产生不出来，填进快照回来的是受理门拒绝」，并留了「解封那天先扩领域
  封闭集与迁移 CHECK，再补这一格」的指引——本票就是那一天。提示句原写「服务形态今天只有
  一格」，不同笔改就会与两格词表在同一屏上各说各的，那正是当初撤下时点名要防的事。

**`internal/partycommercial/adapters/http/register_product_channel.go` 不用改**：
`ServiceProductFormRegistrationIntake` 是接口且本包不带实现（`PAR-INT-01` 待提供），
那里没有任何字符串→形态的翻译。

同步点因此是**八处**而不是六处（两条守卫用例另计）。教训与 `04` 那条同形：
**盘点的搜索范围决定了它能看见什么，而它的结论不自带这个边界。**

## 迁移的改法：另起一份，**不得就地改 `0008`**（2026-09-02 更正）

立票时把这处写成「改 `0008` 的 CHECK」，依据是 `0008` 自己那句注释「扩展先改领域封闭集，
再改这一条」。**那句话指的是改动次序，不是改动位置**，照字面做会踩本仓的迁移门禁：

`internal/platform/migrate/runner.go` 的 `verifyNoDrift` 拿每条**已施加**迁移的记录校验和与
本次构建的工件比对，不一致即以 `ErrChecksumDrift` **阻断整次运行**；校验和由
`migrations.checksumOf` 对文件原始内容算。就地改 `0008` 会让所有已经跑过它的库在下一次
迁移时全部卡死，且这个后果在本机空库上**看不出来**——空库没有历史行，比不出漂移。

仓里已有一模一样的先例，`migrations/party_commercial/0004_commercial_version_kind_range.sql`
放宽 `0001` 的 `object_kind` CHECK 时，注释写的就是这条：

> 不改 0001：已随提交落库的迁移正文按校验和守着，改写它会让下一次运行以校验和不一致暴露
> （见 `migrations.Asset`）。放宽只能是一份新的不可变迁移。

**因此本票照 `0004` 的形状办**：新增 `migrations/party_commercial/0017_service_product_form_label_channel.sql`，
`DROP CONSTRAINT` 再 `ADD CONSTRAINT` 重建 `service_product_form_closed`。`0008` 一字不改，
但要在新迁移的注释里指回它，说明这一对为什么分两份。

**不需要动 `migrations/migrations.go`**：`assetsForModule` 按目录扫 `*.sql` 并以零填充序号
排序，`party_commercial` 已在 `//go:embed` 清单里，放一份新文件即被收进计划。这一格与
新建模块目录不同——那种才要同笔改嵌入行。

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

## 完成记录

2026-09-02 由 MCP-4 落地，八处 + 两条守卫用例 **同一笔提交**——少一处这一对就走散，而走散
在 `go build` 下不报。取值取 `LabelChannelServiceForm` / `"LABEL_CHANNEL_SERVICE"`，落点与
依据来源照本票「口径裁决」一节，实现未另定。

改动分布：领域封闭集与 `valid()`/`String()`；新迁移 `0017`（**不改 `0008`**，理由见上）；
`serviceProductFormFrom`、`rehydrateServiceProduct`、`translateForm` 三处 `switch`；
CLI 的 `serviceProductFormFromName`；管理台 `serviceFormLabels` 与同屏提示句。
`register_product_channel.go` 实测直通未改，HTTP Intake 无翻译未改。

守卫用例两条按设计改，**并各自写明原断言为何不再成立**：
`TestNoServiceProductCanTakeAnIndependentWaybillChannelForm` 更名为
`TestOnlyTheTwoDocumentedServiceProductFormsConstruct`，保留全 `uint8` 值域扫描（它当初就是
为了挡住「新形态加在别的取值上」而这么写的，今天照旧成立，只是改挡第三格）；
`TestServiceProductFormIsAFacetNotASeparateCatalog` 换掉借来表达不变量的手段——原来靠
「第二格没有名字」，现在直接断言两种形态都由服务产品版本承载，并按 `AT-PC-030` 钉住两格
不同名。

真库侧补两条用例：`内容冲突`（同版本改登另一形态不得覆盖）与面单渠道形态的往返。
同时**更正了那份文件头的一句预言**：它写「第二种形态落地那一天，两条防御分支同时变得
够得着」，实际只有`内容冲突`够得着了；「形态取值不认识」仍够不着，而且它本来就不会因为
封闭集变大而够得着——它够得着的唯一条件是 CHECK 与 Go 封闭集**走散**，那不是用例造得出的
状态，正是那段代码存在的理由。

验证（`gofmt -l` 空，`go build ./...`、`go vet` 退 0）：`go test -count=1 ./...` **退 0，含真库**
——DSN 已设且同刻单跑 `TestFreezeScopesAreInvisibleToEachOther` 为 `PASS` 不是 `SKIP`；
本票动 SQL CHECK，真库实跑是完成判据的一部分。前端 `tsc --noEmit` 退 0。
