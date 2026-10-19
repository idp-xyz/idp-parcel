# 42 出发门禁读数不展开集运单元成员：一袋被扣包裹的境内出发读成不受门禁

Category: enhancement
Status: needs-triage——2026-10-19 Claude Code 会话立（用户「你作为业务和系统专家自决……解决所有的问题」授权）。出自 [32](./32-movement-facts-declare-segment-and-record-departure-gate-reading.md) 的隔离评审（非阻断项，评审建议进 main 前写明后果、另立票）。
Blocked by: 无（NO 那一侧的端口在另一会话 idp-parcel-9b 的地盘里变动，落地前要与那边对齐读口形状）
出处：[ADR-0205](../../../docs/adr/0205-departure-gate-is-a-pulled-reading-recorded-with-the-movement-fact-not-a-refusal.md) 越权风险点 5；[ADR-0206](../../../docs/adr/0206-customs-action-admission-judgement-folds-three-sources-and-judges-cross-zone-by-node-customs-zones.md) 越权风险点「对象只认包裹」（「展开成员由消费方做」）。

## 现象

出发门禁读数逐对象问关务，对象取段里出发那一刻在控的参与。段里在控的是袋或集运单元时，关务按包裹引用解析不到它：跨区出发答未决，境内出发答不受门禁。于是一袋里装着被扣包裹的境内出发读成不受门禁。那是一格假的干净读数，不是未决，VE 据阻断派生异常时也看不见它。

## 证据（读码）

- `internal/transportfulfillment/domain/departure_gate_reading.go` 的 `ObjectsUnderControlAt`：只取段的参与对象，不看对象是什么。
- `internal/transportfulfillment/adapters/customscompliance/departure_gate.go` 的 `ReadDepartureGate`：载运对象引用按同一串译成关务的包裹引用。
- `internal/customscompliance/application/judge_action_admission.go`：不在册的对象，跨关务区域移动答未决、其余动作答不受门禁（ADR-0206 决定四）。
- `internal/transportfulfillment/domain/offsite_pickup.go` 的 `CarriedObjectReference`：引用不分包裹与集运单元，本上下文此刻判不出「这一件是袋」。

## 三类

机制（集运单元在出发那一刻的成员怎么取、成员与单元本身怎样各记一格）。规模 M。

## 最小的诚实修法

TF 新开消费侧端口读 NO 的集运成员关系（`internal/nodeoperations/ports/catalogue_read.go` 里有成员关系册的读面，按出发时点取当时成员）。出发读数对集运单元按成员逐件问关务，单元本身那一格取成员里最重的一格，成员各自一格存进逐对象明细。成员读不动时，单元那一格记读不动、依据指名 NO 那一口。

在那之前若要先止损，可以让「本上下文判不出这一件是不是包裹」的对象落未决而不是照收关务的不受门禁；但这要先让载运对象带上种类，比展开成员的改动更宽，不建议单做。

## 要裁的

- 成员取出发那一刻的成员，还是封签那一刻的成员快照（CONTEXT「封签连续且相符的封闭集运单元……成员随单元受控」）。
- 嵌套单元（袋套袋）展开到哪一层。
- 归 TF owner，并要 NO owner 会签读口形状。

## 既有票

none
