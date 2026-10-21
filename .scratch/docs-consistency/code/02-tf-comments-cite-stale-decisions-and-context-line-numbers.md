# 02 TF 代码注释引已被回答的越权风险点、写错改约路径，并用行号与位置序号引 CONTEXT

Category: chore
Status: ready-for-agent——R3（不急）：只是注释，不改行为；读注释的作者会被误导到已回答的风险点与走不通的改约路径。2026-10-21 文档自洽 02 立（用户「你作为业务和系统专家自决」授权），分支 claude-dc02。
Blocked by: 无
出处：[audit/tf.md](../audit/tf.md) L9、L10（审计钉 `6b0074d0`；2026-10-21 按 main `67438a0f` 复核，下列各处原样仍在）。

## 现象

两件同根（注释与现行决定、与 AGENTS.md「改文档」的引用规范不符），合成一张：

1. **注释与现行决定不符**（L9）：
   - `application/trigger_dispatch_on_handover.go` 的 `DispatchOnHandoverSettled` 一带：「补拍口再发起（ADR-0216 决定三；越权风险点『要求缺失不会自愈』）」。ADR-0230 已回答 ADR-0216 越权风险点 1、7：派送待形成册、按册重问、沿替代链认前版、铸法不改。
   - `application/trigger_delivery_dispatch.go` 的 `deliveryDispatchTaskReference` 头注：「是否该按（段，对象）铸以免更正后重开任务列为越权风险点供 owner 复核」。同上，ADR-0230 已答。
   - 「改约走撤回与新订舱」：`application/commission_transport.go`（`AnswerBooking` 一带）、`ports/ports.go`、`domain/transport_commission.go`（`CarrierAcceptance.AdmitsSuccessor` 头注）。`AdmitsSuccessor` 只让候补有后继，接受之后连撤回也答 `ANSWER_CONFLICT`，「撤回」走不到。
2. **行号与位置引用**（L10）：
   - `domain/transport_commission.go`：「（CONTEXT 195）」「（CONTEXT 119/122）」「（CONTEXT 105）」，审计按 `6b0074d0` 逐一核对三处都已指错。
   - `adapters/http/form_load_assignment.go`、`close_fulfillment_segment.go`、`terminate_fulfillment_participation.go`、`record_movement_fact.go` 里的「票 03 简报第 N 行」。
   - 「CONTEXT 生命周期①②③④」式的位置序号（例如 `domain/actual_fulfillment_segment.go`、`application/register_transport_handover.go`、`application/close_fulfillment_segment.go`、`domain/segment_service_action.go`、`cmd/parcel-api/assemble_control_facts.go`），今天还对得上，但与行号同构。

## 证据

- AGENTS.md「改文档」：「引另一个文件的某句或某段 → 优先用符号名，无符号可指时用引文，一律**不用行号，也不用计数**」，并明说「**Go 注释里的跨文件引用同受此约束**」。
- ADR-0230（派送待形成册）决定一、四、六。
- `domain.CarrierAcceptance.AdmitsSuccessor` 的实际判法（接受之后无后继）。

## 三类

机制（注释）。无产品策略、无租户取值。

## 规模

S：只改注释，`go build`、`go vet` 不受影响。

## 最小的诚实修法

- 前两处改为引 ADR-0230 的相应决定，删去「列为越权风险点」的说法。
- 「改约」那几处写明今天的路径：已接受的订舱只能由运营取消订舱再新订；不改行为。若要让已接受的订舱可撤回，另立票，不在本票。
- 行号与「第 N 行」「生命周期①–④」全部换成 CONTEXT 小节名加引文，或符号名。

## 要裁的

无。已接受订舱要不要可撤回是另一件事，不在本票裁。
