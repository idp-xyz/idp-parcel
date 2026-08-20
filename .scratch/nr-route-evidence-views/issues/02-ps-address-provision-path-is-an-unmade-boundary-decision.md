# `NetworkEvidenceView` 要的客户地址，PS→NR 的提供路径是一条没做过的边界决策

Category: enhancement
Status: needs-triage

从本目录 `01` 号票「二、`NetworkEvidenceView` 还缺一条真缝」划出。`01` 的机制四件
（NR-CATALOG-MECH）不含此缝，做完也不解此缝。

## 事实

- `NetworkEvidenceView` 要按目的地解析服务区域，输入里有客户地址。
- [network-routing CONTEXT](../../../docs/domain/network-routing/CONTEXT.md) 明文：「客户地址
  继续由 `parcel-shipment` 保存。`network-routing` 只保存服务区域、节点覆盖版本和当次解析依据」。
- `internal/networkrouting/ports/ports.go` 没有任何读 PS 地址的端口。
- [CONTEXT-MAP](../../../docs/domain/CONTEXT-MAP.md) 的 `parcel-shipment → network-routing`
  一条里客户地址确实由 PS 提供，但**提供路径未定**。

## 为什么是决策不是实现

要定的是边界形状，不是写一个适配器：

1. **地址以什么形态过界**——全地址、脱敏投影、还是发起方在请求期解析好随请求携带
   （`LoadNetworkEvidence` 的键由发起方按接单规则包解析，先例倾向请求期携带，但没人裁过）。
2. **谁调谁**——NR 长一个读 PS 的端口（消费方适配器先例），还是 PS 把地址随证据请求传入，
   NR 根本不需要新端口。两条路对「NR 只保存当次解析依据」这句的守法程度不同。
3. **解析依据怎么留痕**——CONTEXT 要求 NR 保存「当次解析依据」，地址若不落 NR，依据引用
   什么标识。

## 裁断输入

CONTEXT-MAP 该条、PS CONTEXT 的地址所有权句、`ServiceAreaResolutionSpec` 注释、
`create_initial_route.go` 的请求期取值先例。裁断落 CONTEXT-MAP（必要时新 ADR），
不落在实现票里。

## Comments

- 2026-08-20 MCP-1：随 `01` 号票开工（机制四件）同步划出，防止这条缝被塞进实现票里顺手裁掉。
