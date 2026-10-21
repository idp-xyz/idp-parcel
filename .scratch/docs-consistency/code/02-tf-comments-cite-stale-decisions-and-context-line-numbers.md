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

## 追加（2026-10-21，文档自洽 08）

[ADR-0264](../../../docs/adr/0264-capacity-assurance-is-required-on-load-assignments-only-and-neither-a-confirmed-booking-nor-a-departure-reads-capacity.md) 裁定确认订舱不挂容量保障（部分停用 ADR-0203 决定一里确认订舱那一半），于是又有一处注释过时，并进本票一起改：

- `application/commission_transport.go` 的 `SubmitBooking` 头注「确认订舱与容量的关系另归一张票」一句：改为确认订舱不读池、不要预占引用，容量保障只挂在装载分配上（ADR-0264 决定一、二）。

同一条注释里「委托在册且未取消」「订舱不占容量」等其余说法不变。

文档自洽 08 改写了 TF CONTEXT 的几句原句，下面这些 Go 注释还逐字引着改前的句子（取证于 `claude-dc08` 基 main `3619f5c0e`），一并改成引改后的小节名与原句，或符号名：

- `application/commission_transport.go` 的 `SubmitBooking` 体内，引 CONTEXT 生命周期「尚未消耗的运输委托或订舱范围可以取消」：今天的生命周期写「尚未开始的运输委托可以整份取消」。
- 同文件 `CancelCommission` 头注，引同一句与「上游运输意图取消只对尚未消耗的下游范围发出取消或释放请求」：后一句已改写为委托取消整份、不向下游传（TF CONTEXT「运输委托、订舱、容量与装载」），头注里「下游那一步本编排没做」的说法改为那就是现行规则（ADR-0204 决定三）。
- `domain/transport_commission.go` 的 `FormCarrierAcceptance` 头注，引「承运接受可以部分或附条件成立」：今天 Language「承运接受」写「承运接受可以只接受一部分」。
- 同文件 `CarrierAcceptanceOutcome` 头注，引生命周期「订舱申请 → 待确认、候补、部分确认、已确认、拒绝或失效：每次结果保留数量和来源，后续替代不覆盖原申请」：今天生命周期按五值写，含撤回。
- 同文件 `CarrierAcceptance.AdmitsSuccessor` 头注，引「后续替代不覆盖原申请」：今天写「后继不覆盖候补，也不覆盖原申请」。
- `domain/transport_charge_occurrence.go` 的 `ErrCancelledAfterTransportStart` 注释与 `ChargeOccurrenceReason` 头注，引「尚未执行服务的取消费」：今天 TF CONTEXT「运输收费发生项与成本来源」写原因封闭四种、「尚未随运输开始而消耗的订舱的取消费」。
- `domain/transport_schedule_capacity.go` 的 `CapacityPool` 头注「CONTEXT 说的多维运力范围」：今天 Language「容量池」就是一池一维、一班的多维运力是一组池，这半句改为直接引它。
