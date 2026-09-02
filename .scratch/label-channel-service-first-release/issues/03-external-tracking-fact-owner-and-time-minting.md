# 03 末端渠道轨迹由谁收编为自己的事实，外部时间戳与标识由谁认领

Category: enhancement
Status: resolved——三问已裁，见文末「裁决」；落文与实现的前置条件列在其中，归 `16`
Blocked by: 无

## 为什么先立它

[ADR-0088](../../../docs/adr/0088-label-channel-service-enters-the-first-release-service-forms.md)
Consequences 要求「末端渠道轨迹回传的事实收编（先由事实所有者收编，不直插投影）」。
[轨迹源盘点](../tracking-source-seam-inventory.md)把这句话拆成两半，结论是**后半句已经守住，
前半句整段缺席**：

- 「不直插投影」是**编译期成立**的：`domain.SourceContext` 是封闭五值，
  `MilestoneClassification` 只能由 `AcceptedSourceFact` 构造，外部数据没有绕进投影的类型路径。
- 「先由事实所有者收编」缺执行方，且**所有者是谁本身没定**：
  [渠道适配缝备忘](../../../docs/design/channel-adapter-seams-design-note.md)把「轨迹采集 /
  状态同步」指给 `transport-fulfillment`（承运/移动/交付）与 `node-operations`（节点事实），
  而面单交易在 `parcel-shipment`。**一条渠道轨迹落在哪本事实上，备忘没答，代码里也没有。**

## 要的裁决

1. **收编方归属**：末端渠道回传的轨迹译成谁的事实——TF 的交接/移动/交付、NO 的节点事实、
   还是 PS 侧随面单交易另立一类。判据要正面处理一件事：面单渠道服务**没有自营现场作业**，
   TF 与 NO 的既有事实类型都以自营作业为原型，把渠道状态硬塞进去可能是在改领域语言（spec
   「边界」节禁止）。
2. **外部时间与标识的认领**：`domain.AcceptedSourceFact` 要求 `OccurredAt` / `EffectiveAt` /
   `ReceivedAt` **三个时间齐备**，而外部轨迹源常常只给一个。
   [ADR-0023](../../../docs/adr/0023-work-fact-identity-and-time-are-minted-by-the-device.md)
   写明「设备与外部事实的身份时间不代铸」——那么缺的两个时间怎么办：拒收、还是有一条不算
   代铸的填法。**这一问不答，收编执行器就只能自己编一个规则。**
3. **事实类型词**：收编后的 `SourceFactKind` 取值由谁命名。它是里程碑映射的键之一
   （`ports.MilestoneMappingEntry` 键为源上下文 + 事实类型），命名错了映射就配不上。

## 红线

- 不新造也不改写领域语言：要改 TF/NO 的事实定义，先回对应 `CONTEXT.md`（[AGENTS 改文档](../../../AGENTS.md#改文档)）。
- 不填任何源的状态码表、承运商编码、时区口径（实例半边）。
- **不得以「先落地再说」为由让外部数据直插投影**——那条今天由类型守着，本票不许松它。

## 完成判据

三问各有明确答复并落在可引用处；`15`、`16`、`17` 三票的阻塞边据此解除或改写。若裁到要
动 TF/NO 的领域语言或 ADR-0023 的适用范围，**先落文再谈实现**。

## 参照

[轨迹源盘点](../tracking-source-seam-inventory.md)第二段；
`internal/visibilityexception/domain/tracking_projection.go` 的 `SourceContext` 与
`AcceptedSourceFact`；[渠道适配缝备忘](../../../docs/design/channel-adapter-seams-design-note.md)
「轨迹采集 / 状态同步」行；ADR-0023。

## 裁决

裁决人：本会话，经 owner 明确授权（2026-09-02，原话「你作为业务和系统专家，你的明确的建议呢，
不要管其他的 agent 了，你自己完全独立工作」）。能力边界写在本节末。

### 一、收编方归属：`transport-fulfillment`，但**新立一类事实**，不塞进既有类型

**排除 `node-operations`**：它管的是节点事实，而末端渠道服务在本仓**没有节点**——运营企业
既不揽收也不分拣这批包裹，`NO` 的事实原型（节点收货、作业、交接）在这条链上一个都不发生。

**取 `transport-fulfillment`**：它管承运、移动、交付，末端渠道做的正是这三件事，与
[渠道适配缝备忘](../../../docs/design/channel-adapter-seams-design-note.md)把「轨迹采集 /
状态同步」指给 TF 的既有归属一致。

**但不复用 TF 既有事实类型。** 本票「要的裁决」第一问自己点名的风险成立：TF 今天的七个用例
（`commission_transport`、`register_transport_handover`、`register_offsite_pickup`、
`register_effective_delivery` 等）**全部以自营作业为原型**，它们的语义前提是运营企业自己
执行了那个动作。把「渠道报来的状态」塞进 `register_transport_handover`，等于宣称运营企业
作过一次交接——那是在改这些词的含义，违反 spec「边界」节与 [AGENTS 改文档](../../../AGENTS.md#改文档)。

因此走**扩充而非改写**：在 TF 下新立一类「外部承运轨迹事实」，与既有自营作业事实并列。
它的语义前提是「外部承运方报来、由 TF 认领为自己的承运事实」，与自营事实在类型上分得开。
这也保住了[轨迹源盘点](../tracking-source-seam-inventory.md)第二段查明的那条编译期约束：
收编后它仍是 `SourceTransportFulfillment` 这一格，`SourceContext` 封闭五值一字不改。

### 二、外部时间与标识：三个时间分三种归属，`OccurredAt` 缺则拒收

[ADR-0023](../../../docs/adr/0023-work-fact-identity-and-time-are-minted-by-the-device.md)
禁的是**代铸外部事实的身份与发生时间**。按这条切，`AcceptedSourceFact` 要的三个时间恰好
不是同一种东西：

- **`ReceivedAt` 由本仓铸。** 它是「本仓什么时候知道的」，本来就是本仓自己的事实，
  铸它不构成代铸。
- **`OccurredAt` 必须由外部源给，源不给就不收编。** 它就是 ADR-0023 所指的发生时间，
  没有任何填法不算代铸。**这一格不留例外**：收不了的如实留痕为「源未给发生时间」，
  不进事实库，也不许拿 `ReceivedAt` 顶替——顶替之后迟到轨迹与实时轨迹在类型上就分不开了，
  而 `AcceptedSourceFact` 的三时间分立注释点名要防的正是这件事。
- **`EffectiveAt` 由 TF 铸，但必须作为一次显式判断。** 它答的是「这条外部状态从何时起对本仓
  有效」，那是**收编方自己的业务判断**而不是外部事实的时间，因此不在 ADR-0023 的射程内。
  但**不得默默等于 `OccurredAt`**：默认相等会让「所有者判断过」与「没人判断过」长成同一张脸，
  那正是本仓反复治过的那一类。要么由 TF 显式给出，要么记为该源的一条已登记规则并带版本。

**标识**同理：外部事件标识由源给，本仓不代铸；本仓自己的事实标识由 TF 按既有身份工厂铸，
两者分开存，不互相顶替。

### 三、事实类型词由 TF 命名，随 CONTEXT 增补一并定

`SourceFactKind` 是里程碑映射的键之一（`ports.MilestoneMappingEntry` 键为源上下文 + 事实
类型），所以命名必须与新立的事实类型同一处定义、同一笔落地，不得由收编执行器就地发明。
具体取值词随下面的 CONTEXT 增补给出，不在本票预先写死。

## 落文与实现的前置条件（归 `16`，不归本票）

本裁决要动 TF 的领域语言，按 [AGENTS 改文档](../../../AGENTS.md#改文档)**先落文再谈实现**。
`16` 开工前必须先完成：

1. 增补 `docs/domain/transport-fulfillment/CONTEXT.md`：新立的外部承运轨迹事实是什么、
   它与既有自营作业事实的边界、它的事实类型词。
2. 一篇新 ADR 记「外部事实的三时间按归属分铸」这条对 ADR-0023 的适用解释——它难逆转，
   且此后**每一个**外部源都继承它，不止面单渠道。ADR 里要正面写清 `EffectiveAt` 为何不在
   ADR-0023 射程内，以及 `OccurredAt` 缺失即拒收这条没有例外。
3. 必要时同步 [GLOSSARY](../../../docs/domain/GLOSSARY.md) 与 CONTEXT-MAP。

**若上述落文在评审中被否**，本裁决随之作废，本票重开——裁决记在这里是为了让 `15`/`16`/`17`
现在就有一个可指的答案，不是为了绕过落文。

## 连带结论：`17` 不做

外部轨迹既然译成**新立**的一类事实，就不需要给 TF 既有的`交接`与`外场取件`补在线登记口——
那两个口原本只是「若裁定译成既有事实」这条路的前置。处置见票 `17`。

## 能力边界

本裁决依据的是 TF/NO/VE 三处 `CONTEXT.md`、ADR-0023/0088、[渠道适配缝备忘](../../../docs/design/channel-adapter-seams-design-note.md)、
两份盘点，以及 `tracking_projection.go` 与 TF `application/` 下的用例清单。**未取任何外部
轨迹源的公开开发文档**——本裁决答的是「归谁、时间怎么认领」，不需要各源接口形态；真要选
拉取还是回调、要不要按源分适配器，归 `15`，届时另行取证。**未打开 `docs/wooolink/` 或
`docs/reference/xls/` 下的客户文件**，因此裁的是结构不是对任何客户的承诺。
