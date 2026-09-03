# 16 外部轨迹的收编执行器：译成所有者自己的事实

Category: enhancement
Status: in-progress——MCP-1 于 2026-09-03 19:2x 占号（MCP-5 已明确让位）；开工前置三件已落 `2bb9300`
Blocked by: 03, 15（均已 resolved：03 裁决在票面，15 落 `7904003`）

## 缺口

[ADR-0088](../../../docs/adr/0088-label-channel-service-enters-the-first-release-service-forms.md)
要求「末端渠道轨迹回传的事实收编（先由事实所有者收编，不直插投影）」。
[轨迹源盘点](../tracking-source-seam-inventory.md)第二段查明：**「不直插投影」已经守住，
「先由事实所有者收编」整段缺席**。

`transport-fulfillment` 与 `node-operations` 两侧今天的事实**全部来自内部登记**——TF 的用例是
`commission_transport`、`prepare_transport_opportunity`、`register_transport_handover`、
`register_offsite_pickup`、`register_effective_delivery`、`start_alternate_journey`、
`accept_regulatory_disposition`，**没有一个接受「外部系统报来的状态」**。

## 开工前置（`03` 裁决要求，先落文再实现）

`03` 裁定收编方是 TF，且**新立一类「外部承运轨迹事实」**而不复用既有自营作业事实类型。
这是领域语言的扩充，按 [AGENTS 改文档](../../../AGENTS.md#改文档)**本票开工前必须先完成**：

1. 增补 `docs/domain/transport-fulfillment/CONTEXT.md`：新事实是什么、与既有自营作业事实的
   边界、它的事实类型词。
2. 一篇新 ADR 记「外部事实的三时间按归属分铸」这条对 ADR-0023 的适用解释——此后**每一个**
   外部源都继承它，不止面单渠道。
3. 必要时同步 [GLOSSARY](../../../docs/domain/GLOSSARY.md) 与 CONTEXT-MAP。

**这三件没做完，本票不得动 `internal/`。** 落文在评审中被否则 `03` 与本票一并重开。

三件已落 `2bb9300`：[ADR-0102](../../../docs/adr/0102-external-fact-three-times-are-minted-by-ownership.md)、
TF `CONTEXT.md` 两词＋规则节＋生命周期节＋Boundaries 一行、GLOSSARY 两条、CONTEXT-MAP TF 拥有清单一项。
事实类型词定为 `external-carrier-tracking`（定义在 CONTEXT.md，不在本票）。

## 做什么

按 `03` 裁定的所有者（TF），补收编执行器：把 `15` 交来的原始素材译成 TF 新立的那一类事实，
再由既有的 `derive_on_*.go` 译装器进 VE 的 `AcceptedSourceFact`。

**下游一行不用改**：里程碑映射、投影、客户可见三段形状齐备（盘点第三、四、五段各自判为
无缺口），它们在等的就是这个输入。

## 必须守住的几格

- **三时间**：`AcceptedSourceFact` 要求 `OccurredAt`/`EffectiveAt`/`ReceivedAt` 齐备，而外部源
  常常只给一个。`03` 已裁定分三种归属，本票照办不自定：`ReceivedAt` 由本仓铸（那本来就是
  本仓的事实）；**`OccurredAt` 必须由源给，缺则不收编**，如实留痕为「源未给发生时间」，
  **不得拿 `ReceivedAt` 顶替**——顶替之后迟到轨迹与实时轨迹在类型上就分不开了；
  `EffectiveAt` 由 TF 作为一次**显式判断**铸，**不得默默等于 `OccurredAt`**，
  否则「所有者判断过」与「没人判断过」长成同一张脸。
- **取代关系只登记不裁决**：`Supersedes` 由源上下文随更正给出；本收编执行器若要表达「这条
  轨迹更正了上一条」，得由所有者作出判断，**不得从到达先后或业务时间推断**。
- **`SourceFactKind` 的命名照 `03` 的裁定**——它是里程碑映射的键之一，命名错了映射配不上。

## 红线

- 不新造领域语言；要改 TF/NO 的事实定义，先回 `CONTEXT.md`。
- 不填状态码表、承运商编码、时区口径（实例半边）。
- 不绕过所有者直接写 VE 的事实库。

## 完成判据

收编执行器有实现与测试，能把一条外部轨迹走到投影上；三时间与取代关系的处理各有用例；
`gofmt -l` 空、`go build`/`go vet` 退 0、`go test -count=1 ./...` 绿并注明含不含真库。

## 参照

[轨迹源盘点](../tracking-source-seam-inventory.md)第二段；票 `03`、`15`；
`internal/visibilityexception/domain/tracking_projection.go`。
