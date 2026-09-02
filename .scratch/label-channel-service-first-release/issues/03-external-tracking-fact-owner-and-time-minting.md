# 03 末端渠道轨迹由谁收编为自己的事实，外部时间戳与标识由谁认领

Category: enhancement
Status: ready-for-human
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
