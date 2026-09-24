# 06 parcel-shipment：受理链与面单择优链上被归进实例半边的判断方法

Category: enhancement
Status: needs-triage——2026-09-24 通道 4 随票 02 立（登记册逐行拆分划出的产品策略，PS 一张）；逐项先核执行器有无，缺的定出内置策略或参考配置形态后转 ready-for-agent
Blocked by: 无（第 4 项里公开承运商接口的参考配置那半等 03）
地盘：`internal/parcelshipment/adapters/` 下受理链与面单择优链的消费侧适配器，`cmd/parcel-api`、`cmd/parcel-dispatch` 的对应装配点；缝对面的 party-commercial、parcel-pricing、settlement-accounting 只读，要改提供方另开票。
出处：[票 02](./02-split-parameter-register-and-retriage-deferrals.md) 登记册逐行拆分——[参数登记册](../../../docs/product/PILOT-PARAMETER-REGISTER.md) `PAR-COM-13`、`PAR-COM-14`、`PAR-COM-15`、`PAR-COM-17`、`PAR-INT-02`、`PAR-NET-16` 行内「〔ADR-0146 拆分〕」点名的部分；[ADR-0146](../../../docs/adr/0146-product-strategy-is-a-third-class-between-mechanism-and-tenant-values.md) 决定一、二与决定五第三条。已知缺口沿用[开发主线](../../../docs/product/PARCEL-NETWORK-FIRST-RELEASE-DEVELOPMENT-BASELINE.md)「按四项判据重定级」表 PN-02 行的原话，不另立口径。

## 做什么

每项落一个执行器：内置策略（租户只选形态、填数值）或可显式采用的参考配置。租户没选没填时照旧答`未配置`。

1. **逐项时点语义的折法**（`PAR-COM-14`「锚点语义」「各判断的 `asOf` 语义」）。`AsOfValueSource` 生产为 nil，受理链第二阶段必答`未配置`；其类型注释把「语义如何折成一个时刻」判为实例半边。标准语义怎样折成时刻是方法，租户的截点才是取值。已知缺口：重定级表 PN-02 行第三项「受理链逐项时点 `Values` 为 nil」。
2. **受理前控制金额的估价方法**（`PAR-COM-15`「账户和价格依据」里价格那半）。`ControlAmountSource` 生产为 nil，其注释写「金额该由估价形成（计价缝），接通前属实例半边」。已知缺口：同表 PN-02 行第三项「受理前控制金额 `Amounts` 为 nil」。
3. **面单择优链的选法与触发面**（`PAR-INT-02` 使用授权、`PAR-NET-16`「自动择优与人工确认的分界」）。缺账号使用授权选法、供应商协议选法与触发面（`cmd/parcel-api/assemble_label_channel.go` 文件头注自称触发面「是产品题」）；渠道约束、计价输入 `PricingInputSource`、BUY 价卡逐口定性。已知缺口：同表 PN-02 行第三项。
4. **面单出向连接器**（`PAR-INT-02`）。一家都没有（`unconfiguredLabelChannelGateway`）。连接器形态归产品；公开承运商面单接口按参考配置随产品发布，形态等 03。已知缺口：同表 PN-02 行第三项。
5. **待核：资料修订的并发合并与下游交接**（`PAR-COM-13`「基础版本/并发合并规则」「与制签/收寄/装袋/申报/关闭后续办的交接规则」）。`UC-PS-002` 编排已有资料版本追加与「资料版本已形成」意图；核它们是否就是这两项的执行器，缺哪半补哪半。
6. **待核：多包裹终局汇总**（`PAR-COM-17`「多包裹汇总」）。核 `UC-PS-004` 的「委托完成派生」是否即此方法。
7. **待 PS owner 定：资料组词表**（`BD-PS-010`；票 02 暂缓清单：ADR-0120 与 ADR-0130 把「资料组怎么划」「收件怎么叫」判为实例半边）。有哪些资料组不看任何租户就答得出，按分界检验是产品词表；各组内哪些字段允许何时修订仍是租户取值（`PAR-COM-13`）。

顺带（[票 01](./01-regrade-slices-under-four-criteria.md)「严格复核记录」交来，只改注释）：`acceptanceConsumer` 的函数头注与其行内 ADR-0064 注释相抵；`acceptanceFinancialControl` 里「`Amounts` 留空……见 `acceptanceChainConsumer` 的注释」指向一段不存在的注释，实际说明在 `ControlAmountSource`。

## 不做

- 不给任何租户定截点、价卡、账户或授权，不预选某租户用哪种形态。
- 重定级表同一行的机制缺口（可达性资格视图闭包标识、网络定义登记册写入方、结算账户登记册、面单择优链的接受时解析回指）不在本票。

## 完成判据

- 每项要么有执行器（带测试），要么记下已有执行器的证据；登记册对应行「〔ADR-0146 拆分〕」一句同步收短。
