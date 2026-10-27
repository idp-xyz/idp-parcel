# 02 TF 代码注释引已被回答的越权风险点、写错改约路径，并用行号与位置序号引 CONTEXT

Category: chore
Status: resolved——2026-10-27 进 main（见文末「进 main 记录」）。此前 in-progress——完工待评审与进 main，分支 claude-cmtA
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

测试注释里同类的几处（文档自洽 08 第 2 轮评审补；Covers 点名的 AT 号不变，只改引文）：

- `internal/transportfulfillment/application/accept_regulatory_disposition_test.go` 的 `TestADeclineCarriesItsBasisAndNoObjects`，引 UC-TF-001 结果契约`已拒绝`行「拒绝范围、依据、决定方和业务时间」：今天那一行是「拒绝范围、拒因、决定方和业务时间」，并标了码 `DISPOSITION_DECIDED`（决定 `DECLINED`）。
- 同文件 `TestAPartialAcceptanceKeepsBothSides`，引`部分承接`行「已承接、未承接范围及逐项原因」：今天那一行是「已承接对象、未承接范围的原因与移动授权」，码 `DISPOSITION_DECIDED`（决定 `PARTIALLY_ACCEPTED`）。
- `internal/transportfulfillment/application/commission_transport_test.go` 的 `TestCancellationOnlyReachesUnstartedCommissions`，用行号写法「195「尚未消耗的运输委托或订舱范围可以取消」」：行号去掉，引文改为 TF CONTEXT 生命周期「运输委托、订舱与承运接受」里「尚未开始的运输委托可以整份取消」。
- `internal/transportfulfillment/domain/transport_commission_test.go` 的 `TestCancellationOnlyReachesUnstartedIntents`，同样的行号写法与旧句：照上一条改。

## 完成记录（分支 claude-cmtA）

只改注释与测试说明文字，不改任何可执行语句、标识符、字符串字面量。`git diff -U0` 里非注释行的变动为零；`gofmt -l` 无输出；`go build ./...`、`go vet` 通过；`scripts/test-full.sh ./internal/transportfulfillment/... ./internal/customscompliance/... ./internal/visibilityexception/... ./internal/architecture/...` 全绿（三张票同在分支 claude-cmtA，一次跑完，38 个包 ok，无 FAIL）。

改了 `internal/transportfulfillment`（含 `cmd/parcel-api/assemble_control_facts.go` 一处）61 个文件、约 100 处，按类：

1. **已回答的越权风险点与走不通的改约路径**
   - `trigger_dispatch_on_handover.go` 的 `DispatchOnHandoverSettled`：去掉「越权风险点『要求缺失不会自愈』」，改为执行器把那一件同笔留在派送待形成册、由运营按册重问或经补拍口再发起（ADR-0230 决定一、四、七）。
   - `trigger_delivery_dispatch.go` 的 `deliveryDispatchTaskReference` 头注：去掉「列为越权风险点供 owner 复核」，改为替代链上前版已形成的任务由执行器沿链回看认回、铸法不为此改（ADR-0230 决定二、六）。
   - 「改约走撤回与新订舱」三处（`commission_transport.go` `AnswerBooking` 体内、`ports/ports.go` `BookingAnswerRecord`、`domain/transport_commission.go` `AdmitsSuccessor`）：改为定局之后连撤回也不接，已接受的订舱要变只能由运营经取消订舱口（`CancelBooking`）取消再新订。
2. **CONTEXT 行号与位置序号**：`domain/transport_commission.go` 的「CONTEXT 195 / 119/122 / 105」、测试里的「195 / 243 / 105 / 208」，以及全部「生命周期①②③④」「末句」「首条」「末条」「规则节第三条」，换成小节名加逐字引文。「票 03 简报第 N 行」四处（`form_load_assignment.go`、`close_fulfillment_segment.go`、`terminate_fulfillment_participation.go`、`record_movement_fact.go`）改指票 03 接入判定表里的符号名那一行；票 07「第三个、第四个写面」改「写面之一」。
3. **追加段落**：`SubmitBooking` 头注改为确认订舱不读池、不要预占引用，容量保障只挂在装载分配上（ADR-0264 决定一、二）；`SubmitBooking` 体内与 `CancelCommission` 头注（「下游那一步本编排没做」改为那就是现行规则，ADR-0204 决定三）、`FormCarrierAcceptance`、`CarrierAcceptanceOutcome`、`AdmitsSuccessor`、`ErrCancelledAfterTransportStart`、`ChargeOccurrenceReason`、`CapacityPool` 的旧引文换成文档自洽 08 之后的原句；`TestADeclineCarriesItsBasisAndNoObjects`、`TestAPartialAcceptanceKeepsBothSides` 改引 UC-TF-001 结果契约现行行（带码与决定值）；`TestCancellationOnlyReachesUnstartedCommissions`、`TestCancellationOnlyReachesUnstartedIntents` 去行号、改引「尚未开始的运输委托可以整份取消」。
4. **同类残留（票未点名）**：用脚本把 TF 包里所有「CONTEXT…「…」」引文逐一对 TF CONTEXT 核逐字，不是原文的改成原文或不带引号的转述，约 20 处（例：`PC 登记之后凭同一份证据形成新版本`、`整批/整车/整袋结论…`、`POD 被更正或失效时…`、`更正使原结果失效`、`形成失败重跑本拍`、`已拒收或待确认不转出控制…`）；两处错归 CONTEXT 的引文（`ports/fulfillment_segment.go` 的 `SegmentJoinOutcome` 实为 UC-TF-002 的句子，`summarize_handover_scope_test.go` 的「这一次交接」CONTEXT 里没有）改归出处或去掉归属；`supplier_agreement_source.go` 引 PC CONTEXT 的「到期、终止或被替代」也不是原文，改为 PC CONTEXT 现句。变更叙述词里的明确叙事（「退场」「不再是拒绝」「此前」「票 06 之后」「照旧按…铸」）改成现状陈述，共 10 处。

### 已漂移或已不存在的位置

票上列的位置全部仍在，没有已被后来的票顺手改掉的。

### 代决

- 「简报第 N 行」其实指票 03 判定表的第 N 行，用符号名（`CloseFulfillmentSegment` 等）指那一行，不再写序数。
- 带 `……` 省略号的引文，只要各段分别逐字存在于 CONTEXT 就保留，没有硬改成整句。
- 约 100 处「今天」「照旧」「不再是」是状态判断（「照旧答已在册」「被替代的版本不再是当前控制」）或「现状没有消费者」的时点说明，不是变更叙述，保持原样。

### 越权风险点

- 「订舱要变」的现行路径写成「运营取消订舱再新订」；`CancelBooking` 对已开始委托下、申请不晚于开始的订舱取消不动（ADR-0204 决定三），那种情形下已接受订舱既不能改答也取消不了，本票不裁；若要让已接受订舱可撤回，仍如票面所说另立票。
- ADR 内部的位置序数（「决定二末条」「Alternatives 第二条」「决定二第三条」，约 8 处）本票没动：规范只明许「决定几」，这些是更细的位置词，但改动要逐份读 ADR 找句首引文，留给后续一次统一清。

## 进 main 记录（2026-10-27，协调方）

- 分支 `claude-cmtA` 与 code/02、26、32 同批，rebase 到 origin/main `b8b1ca1e9` 后由协调方快进，tip `08eeddd2d`；隔离评审（Sonnet）可合入，无阻断：三个提交每个改动的 `.go` 文件（61、33、24 个）去掉注释后语法树逐个比对 0 差异（程序放评审私有目录，对有代码变动的老提交试过会报差异）；新写的「」引文去重 82 条逐字核对通过；约 30 个 ADR、AT、UC 指针属实。评审四条措辞建议由协调方当场修了三条（订舱变更路径补不可取消的限定；盘点去掉变更叙述加票号；两处无票号的「票面」位置词），第四条（既有的若干对着旧版本讲的话）不在本批范围。协调方单独提交重算的 AT 计数、ADR 索引与机制清点（本批无变动）；全量 `scripts/test-full.sh` 退 0、159 个包 ok；`go build ./...`、`go vet ./...` 干净。无迁移、无新 ADR、无 AT 号变化，未碰 `apps/admin-web` 与 `internal/partycommercial`。
- 本票（TF，61 个文件、约 100 处）：票点名的各类全部改了；ADR-0216 已被 ADR-0230 回答的风险点改引 ADR-0230 决定；「改约走撤回与新订舱」三处改成"定局之后连撤回也不接，已接受订舱要变只能运营取消订舱再新订（订舱已被运输开始消耗的取消不了，ADR-0204 决定三）"，评审核实"定局之后连撤回也不接"与 `AdmitsSuccessor` 一致；CONTEXT 行号与「生命周期①②③④」去掉。
- 去向：无新票。**已接受订舱在订舱已被运输开始消耗时既不能改答也取消不了**（ADR-0204 决定三），本票没有裁，注释里已如实写限定；要让它可撤回需另立票，待用户决定。
