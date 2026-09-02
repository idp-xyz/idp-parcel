# 16 外部轨迹的收编执行器：译成所有者自己的事实

Category: enhancement
Status: draft
Blocked by: 03, 15

## 缺口

[ADR-0088](../../../docs/adr/0088-label-channel-service-enters-the-first-release-service-forms.md)
要求「末端渠道轨迹回传的事实收编（先由事实所有者收编，不直插投影）」。
[轨迹源盘点](../tracking-source-seam-inventory.md)第二段查明：**「不直插投影」已经守住，
「先由事实所有者收编」整段缺席**。

`transport-fulfillment` 与 `node-operations` 两侧今天的事实**全部来自内部登记**——TF 的用例是
`commission_transport`、`prepare_transport_opportunity`、`register_transport_handover`、
`register_offsite_pickup`、`register_effective_delivery`、`start_alternate_journey`、
`accept_regulatory_disposition`，**没有一个接受「外部系统报来的状态」**。

## 做什么

按 `03` 裁定的所有者，补收编执行器：把 `15` 交来的原始素材译成该所有者自己的事实，再由
既有的 `derive_on_*.go` 译装器进 VE 的 `AcceptedSourceFact`。

**下游一行不用改**：里程碑映射、投影、客户可见三段形状齐备（盘点第三、四、五段各自判为
无缺口），它们在等的就是这个输入。

## 必须守住的几格

- **三时间**：`AcceptedSourceFact` 要求 `OccurredAt`/`EffectiveAt`/`ReceivedAt` 齐备，而外部源
  常常只给一个。缺的怎么办**照 `03` 的裁定**，不自定。
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
