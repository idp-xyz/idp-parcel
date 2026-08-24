# ADR-0075: 客户地址随判断请求在请求期携带过界，network-routing 不建读取端口

Status: Accepted  
Date: 2026-08-24

> 裁决授权：用户 2026-08-24 授权本会话（MCP-1）代为拍板（用户指示：作为技术与业务专家代表
> 其直接决策）。裁决依据为 [nr-route-evidence-views 票 02](../../.scratch/nr-route-evidence-views/issues/02-ps-address-provision-path-is-an-unmade-boundary-decision.md)
> 全文、CONTEXT-MAP 该边与 network-routing CONTEXT 硬句在 `88215a3` 上的原文复验，以及
> `LoadNetworkEvidence` 键上 `asOf` 的请求期取值先例。裁的是提供路径的边界形状，不裁
> `PAR-NET-14` 规则内容，不动参数登记册任何一行。

## Context

`NetworkEvidenceView` 要按目的地解析服务区域，输入里有客户地址。三条已有权威句合起来留下
一条缝：

- [CONTEXT-MAP](../domain/CONTEXT-MAP.md) 的 `parcel-shipment → network-routing` 边把客户
  地址与「为本次可达性判断解析的明确 `asOf` 语义和值」同列为小包托运**提供**的请求输入；
- [network-routing CONTEXT](../domain/network-routing/CONTEXT.md) 硬句：「客户地址继续由
  `parcel-shipment` 保存。`network-routing` 只保存服务区域、节点覆盖版本和当次解析依据，
  不把客户地址建立成物流节点」；「服务区域不取得客户地址所有权」；
- `internal/networkrouting/ports/ports.go` 没有任何读 PS 地址的端口。

地址由 PS 提供是定了的；**没定的是提供路径**——以什么形态过界、谁调谁、解析依据引用什么。
[票 02](../../.scratch/nr-route-evidence-views/issues/02-ps-address-provision-path-is-an-unmade-boundary-decision.md)
把这三问从 NR 目录机制票（NR-CATALOG-MECH，ADR-0068）里划出，要求先裁再动。

## Decision

**一、地址随判断请求在请求期携带，`network-routing` 不建立读取 `parcel-shipment` 地址的端口。**

发起方（PS 侧编排）把地址随证据请求传入，与 `asOf` 同款：`LoadNetworkEvidence` 的
`ReachabilityJudgmentKey` 已确立「由发起方按接单规则包在请求期解析并传入」的先例，地址在
CONTEXT-MAP 该边的句子里与 `asOf` 同列，走同一条路。

判据是**同版性**，不是接口省事：可达性判断针对「委托当前提交版本」，地址属那个版本的内容。
随请求携带使判断所用地址与被判断的提交版本天然同笔；NR 另行回读则在请求与回读之间存在版本
漂移窗口——判断对象与判断输入可能异版，而「关键依据在接受提交前失效时由小包托运请求重判」
那条纪律预设了判断输入钉在请求上。[ADR-0025](./0025-cross-context-adapters-live-on-the-consumer-side.md)
的消费方侧适配器先例适用于「消费方拥有的判断要读上游已接受事实」；地址不是这一格——NR 明文
不保存、不拥有地址，给它开一个读地址的端口等于让它临时持有明文不拥有的数据面。

**二、过界内容取地理解析投影，不取全地址。**

携带的是地址中服务区域解析所需的地理内容（国家、行政区域、邮编范围等，随服务区域定义的
词汇走）；与地理解析无关的个人身份内容（收件人姓名、联系方式）不过界。投影的具体字段形状
属机制侧实现，随取数侧实现票定，本记录只钉「地理维过界、身份维不过界」这条线。

**三、当次解析依据以引用与摘要留痕，地址本体不落 `network-routing`。**

NR 按 CONTEXT 必须保存「当次解析依据」。其构成裁为三件：**判断对象引用**（租户、委托当前
提交版本、包裹身份——判断键既有维）＋**所用服务区域版本**＋**所携地理投影的版本化内容摘要**
（规范化形状按 [ADR-0014](./0014-versioned-canonicalization-shape-for-content-digest.md)
带版本号，随实现票定形）。摘要使依据可对证——事后拿 PS 侧该提交版本的地址重新规范化即可
比对当时解析用的是什么——而不复制所有权，也不在 NR 留下第二份地址。

## Consequences

- [票 02](../../.scratch/nr-route-evidence-views/issues/02-ps-address-provision-path-is-an-unmade-boundary-decision.md)
  随本记录 resolved；CONTEXT-MAP 的 `parcel-shipment → network-routing` 边补一句提供路径
  （随本记录同笔），network-routing CONTEXT 硬句不动——本决定与它字面一致。
- `NetworkEvidenceView` 与 `InitialRouteEvidenceView` 的取数侧仍阻于 `PAR-NET-14` 规则正文
  （[nr 票 01](../../.scratch/nr-route-evidence-views/issues/01-cut-the-mechanism-half-of-par-net-14-from-its-rule-values.md)
  的收敛结论不变）；届时实现按本记录收地址入参，`internal/networkrouting/ports/ports.go`
  不新增读 PS 端口。
- 地理投影的字段形状与摘要规范化版本是实现票的活，本记录不预拟；实现时若发现服务区域定义
  需要的地理维超出 PS 地址既有内容，那是 `PAR-NET-*` 实例半边的输入问题，按登记册流程走，
  不回改本决定。

## Alternatives considered

- **NR 长一个读 PS 地址的端口（消费方侧适配器先例）。** 否决：同版性窗口（判断对象与判断
  输入可能异版）；NR 临时持有明文不拥有的数据面；解析依据与判断键失去结构绑定——回读的
  地址与请求的判断之间没有「同一笔」保证，留痕只能靠约定。
- **全地址过界。** 否决：解析只需要地理维；把个人身份内容送进一个明文不拥有地址的上下文，
  扩大暴露面还让依据摘要把无关内容钉进去，事后对证反而更脆。
- **地址快照落 NR 作依据。** 否决：正面撞 CONTEXT 硬句「只保存服务区域、节点覆盖版本和当次
  解析依据」——快照就是第二份地址存储，作废与更正都要跟着 PS 走，恰是边界要防的形状。

## Links

- [network-routing CONTEXT](../domain/network-routing/CONTEXT.md)：地址所有权硬句与「当次解析依据」义务的出处
- [CONTEXT-MAP](../domain/CONTEXT-MAP.md)：`parcel-shipment → network-routing` 边（本记录为其「提供路径」半句的定义处）
- [nr-route-evidence-views 票 02](../../.scratch/nr-route-evidence-views/issues/02-ps-address-provision-path-is-an-unmade-boundary-decision.md)（本记录裁的三问的出处）与[票 01](../../.scratch/nr-route-evidence-views/issues/01-cut-the-mechanism-half-of-par-net-14-from-its-rule-values.md)（机制半边与 `PAR-NET-14` 阻断的分界）
- [ADR-0014](./0014-versioned-canonicalization-shape-for-content-digest.md)：内容摘要的版本化规范化形状
- [ADR-0025](./0025-cross-context-adapters-live-on-the-consumer-side.md)：消费方侧适配器先例的适用边界（本记录说明它为何不适用于地址）
- [ADR-0068](./0068-versioned-network-catalog-structure-precedes-rule-content.md)：NR 目录机制四件（本缝当时显式划出不混入）
