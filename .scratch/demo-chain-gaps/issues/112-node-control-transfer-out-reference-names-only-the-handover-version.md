# 112 节点控制的转出引用只带交接版本：同一包裹两次交接同用一个版本号时，转移册上的引用指不回是哪一次

Category: enhancement
Status: ready-for-human——2026-10-27 冻结到首个租户上线之后（[上线线与冻结清单](../../launch-line-freeze-2026-10-27.md)，分级 R3）：不挡上线，代理不认领；解冻看首个租户的真实使用或用户点名。此前 needs-triage——2026-10-19 票 [111](./111-actual-fulfillment-formation-matches-participation-ends-by-version-only.md) 的作者子代理立（用户「你作为业务和系统专家自决……解决所有的问题」授权）。出自 [ADR-0221](../../../docs/adr/0221-a-handover-participation-basis-is-minted-from-the-handover-key.md) 决定六与越权风险点 3。
Blocked by: 无（地盘 `internal/nodeoperations/**` 此刻有别的会话在途，领票前先对齐）
地盘：NO `adapters/transportfulfillment/control_transfer.go`（`FindAuthoritativeHandover` 包装转出引用那一步）、`domain/physical_control.go`（`TransferOutReference`）；若换拼法，另及 TF `domain/transport_handover.go`（`TransferOutBasis`）。

## 现象（读码，钉票 111 的分支）

- NO 的转出引用取 TF 的 `TransferOutBasis`，拼成 `TRANSPORT-HANDOVER/<版本>`，记在节点控制转移册 `transfer_reference` 一列与收寄行读回的控制上（`PhysicalControl.Release`）。
- 交接的键是（租户，对象，范围，版本），版本只在（对象，范围）之内唯一。演示动线每次交接都写 `v1`、范围各异时，转移册上几行的转出引用都是 `TRANSPORT-HANDOVER/v1`，查阅时指不回是哪一次交接（psb/05「重走（钉 `76686b7b`，2026-10-19）」文末「另记一处观察」）。
- 票 111 起，TF 参与上的交接依据改由交接键铸（`TRANSPORT-HANDOVER-KEY/<长度>:<对象>/<长度>:<范围>/<长度>:<版本>`，`TransportHandover.ParticipationBasis`）；NO 的转出引用没跟着换，同一次交接在两本册上是两串。
- 节点控制的正确性不受影响：读控制的折叠与「一份控制至多转出一次」按（作业实物，控制依据）认，转入依据 NO 自己按交接键铸（ADR-0214「转入立起的控制依据」）。缺的只是查阅读得出。

## 三类

- 机制：两条路——
  - NO 的转出引用改取 `ParticipationBasis`（或 NO 自己按交接键铸一串，与转入依据同形）；存量转移行照读、不改写；
  - 维持现状，查阅面按交接键对，转移册另加交接范围一列。
- 产品策略：无。
- 租户取值：无。

## 要裁的

- 走哪一条；换拼法时 NO 测试与 HTTP 查阅面的回显随之变，存量行两种拼法并存要不要在查阅答复里标出。
- ADR-0214 越权风险点「转入依据由本上下文铸、不复用 TF 的转出引用」要不要一并收口。

## 规模

S
