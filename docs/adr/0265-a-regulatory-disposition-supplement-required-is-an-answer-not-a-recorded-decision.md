# ADR-0265：监管运输处置承接的「待补充」是一格答复、不是一种决定——缺移动授权时不落承接记录、不交回执，续办是协作事项本身

Status: Accepted（2026-10-21，Claude Code 会话代 TF owner 决：用户 2026-10-21 原话「需要修改文档，确保自洽，正确」，常设授权「你作为业务和系统专家自决」；票 docs-consistency/08，出处审计 [audit/tf.md](../../.scratch/docs-consistency/audit/tf.md) L8（归 B）。不停用任何已接受记录的条款：改的是 TF CONTEXT「监管运输处置承接决定」词条、「监管运输处置承接与履约」里以「监管运输处置按载运对象和运输动作逐项承接并允许部分结果」起头的那一条与生命周期「监管运输处置承接」里把待补充列为决定的那一句，以及 UC-TF-001 的结果契约。承接编排今天没有生产入口，接线另立票 docs-consistency/code/43。越权风险点见文末，待 TF owner 复核，第 1、3、4 点同时待 CC owner 会签。）<!-- 索引注记 -->｜**决定三种**：监管运输处置承接决定封闭为已承接、部分承接、已拒绝（`ACCEPTED`、`PARTIALLY_ACCEPTED`、`DECLINED`），部分承接里未承接的范围随决定记原因。**待补充是答复**：承接或部分承接缺当前有效移动授权时（扣留或处置决定本身不是移动授权），编排答 `SUPPLEMENT_REQUIRED`，不落承接记录、不交回执；续办是协作事项本身，补齐之后协作事项再交一次，照首交判。今天走得到这一格的只有缺移动授权一种，答复不带原因；接线后（票 code/43）核对对象、动作与起止责任，待补充带一个原因，封闭为缺移动授权、缺对象、缺动作、缺起止责任。纯节点动作（`NODE_ONLY_DISPOSITION`）同为答复。越权风险点含关务一侧要自己记住哪件事项在等补、本上下文看不到在等补的事项、一件事项部分缺授权只能整件待补或记作部分承接的原因、UC-CC-008 要关务自记这一格答复

Date: 2026-10-21

## Context

- 取证于 main `3619f5c0e`（下面引的 TF CONTEXT 与 UC 原句随票 docs-consistency/08 改写）。TF CONTEXT「监管运输处置承接决定」词条：「决定可以是已承接、部分承接、待补充或拒绝」；生命周期「监管运输处置待承接 → 已承接、部分承接、待补充或拒绝」；UC-TF-001 结果契约把「监管运输协作待补充」列为一种结果，必须保留的一栏写「已知边界、缺口和续办入口」。
- 代码：`domain.DispositionAcceptanceKind` 封闭三格 `ACCEPTED`、`PARTIALLY_ACCEPTED`、`DECLINED`；待补充只是编排 `application.AcceptRegulatoryDispositionHandler` 的一格答复 `SUPPLEMENT_REQUIRED`，出自领域的 `ErrMovementAuthorityMissing`（承接或部分承接缺移动授权），不落承接记录、不交回执。领域头注说 UC-TF-001 结果契约是已承接、部分承接、已拒绝三格，并注「待补充与未受理是编排层答复，不是决定」，与 UC 原文不符。
- 承接编排在 `cmd/` 下没有接线，生产上还没有人能走到这一格。待补充要不要落册、要不要本上下文留续办入口，没有记录裁过。

## Decision

**一、承接决定封闭三种：已承接、部分承接、已拒绝。**（机制）

逐协作事项一份决定。部分承接两半都要：已承接的对象，加未承接范围的原因；已拒绝带拒因、不带对象。

**二、待补充是一格答复，不落承接记录。**（机制）

- 决定要承接（全部或部分）却缺当前有效移动授权——扣留或处置决定本身不是移动授权——时，编排答 `SUPPLEMENT_REQUIRED`：不落承接记录，不交承接回执。今天答复不带原因，因为走得到这一格的只有缺移动授权一种；决定四的核对接上之后，答复带一个原因，指名缺哪一项。
- 续办引用就是协作事项本身。缺的移动授权、对象范围、起止责任都归关务协作事项给（UC-CC-008）；补齐之后由协作事项再交一次，本上下文照首交判，同一事项此前没有决定，不是重放也不是冲突。
- 纯节点动作（协作事项不含移动）答 `NODE_ONLY_DISPOSITION`，同为答复、不建任何运输对象，续办在 UC-NO-001。

**三、理由：缺的东西和下一步都在关务那一侧。**

本上下文落一条「待补充」记录，只是替关务存一份它自己的未完成事项，而这条记录上没有本上下文能推进的下一步。派送待形成留痕（[ADR-0230](./0230-delivery-dispatch-left-unformed-is-kept-on-a-pending-register-and-reasked-through-the-same-executor.md)）不同：那里缺的派送要求补齐之后，要本上下文再问一次所有者，所以要册。

**四、协作事项其余几格的核对随接线落地。**

UC-TF-001 要求核对载运对象、移动动作、起止责任与当前有效性，不足时同样答待补充。今天编排只核移动授权，协作事项本体不读；接线时随读协作事项一起补上（票 docs-consistency/code/43）。补上之后待补充带一个原因，封闭为缺移动授权、缺对象、缺动作、缺起止责任，指名缺的那一项——续办都是协作事项再交，但关务要知道去补哪一样。今天只有缺移动授权一种，不带原因不是缺口。

## 候选与反方

- **待补充落一条记录，留续办入口。** 否决，理由见决定三；另外，同一事项补齐后再交时，本上下文要先把那条待补充了结，再形成决定，多出一套了结规则而换不来任何判断力。
- **待补充并进未受理。** 否决。未受理是请求自身立不起来，续办是改请求；待补充是请求合法、缺的是关务那边的依据，续办动作不同，不并格（[ADR-0029](./0029-retrieval-failure-algebra-is-partitioned-by-recovery-action.md)）。

## Consequences

- TF CONTEXT 词条「监管运输处置承接决定」，「监管运输处置承接与履约」里以「监管运输处置按载运对象和运输动作逐项承接并允许部分结果」起头的那一条，生命周期「监管运输处置承接」里「监管运输处置待承接 →」那一句，改成本记录的口径。
- UC-TF-001 结果契约「监管运输协作待补充」一行改为答复、不落决定；`AT-TF-004` 写明答复不落决定；「给开发的交接」列出接线缺口。
- `domain.DispositionAcceptanceKind` 的头注（三格，「待补充与未受理是编排层答复，不是决定」）与改后的 UC 一致，不用改。
- 承接编排的生产入口与协作事项的核对另立票 docs-consistency/code/43。

## 越权风险点

1. **关务一侧要自己记住哪件事项在等补**（TF、CC owner）。本上下文答了待补充就不留痕；关务的协作事项若不据这一格答复记下「等移动授权」，补齐之后也没人再交一次。接线时（code/43）关务那一侧的消费者要把这一格记进自己的协作事项状态。
2. **本上下文看不到在等补的事项**（TF owner）。运营人员要知道哪些监管运输还在等授权，只能去关务那一侧查。今天承接没有生产入口，这一问还不会出现。
3. **一件事项里一部分可承接、一部分缺授权**（TF、CC owner）。一事项一决定：要么整件答待补充，等关务补齐整件再交；要么决定人把缺授权的那一部分作为部分承接里未承接范围的原因写下，已承接的那一部分先走。后者一经落下就是这件事项的决定，补齐之后不能再改成全部承接，要关务另起事项或更正事项。
4. **UC-CC-008 要关务自己记这一格答复**（TF、CC owner）。UC-CC-008 要求关务「保存其接受、拒绝、待补充或部分接受结果」；本上下文的待补充不落记录、不交回执，关务那一侧只拿得到这一格同步答复，要靠它自己记下。须 CC owner 会签。

## Links

- [ADR-0029](./0029-retrieval-failure-algebra-is-partitioned-by-recovery-action.md)：续办动作不同的结果不共格
- [ADR-0230](./0230-delivery-dispatch-left-unformed-is-kept-on-a-pending-register-and-reasked-through-the-same-executor.md)：派送待形成为什么要册——对照
- [transport-fulfillment CONTEXT](../domain/transport-fulfillment/CONTEXT.md)「监管运输处置承接决定」、[UC-TF-001](../application/transport-fulfillment/UC-TF-001-ACCEPT-AND-FULFILL-REGULATORY-TRANSPORT-DISPOSITION.md)（`AT-TF-004`）
- 票 `.scratch/docs-consistency/issues/08-tf-docs.md`；代码票 `.scratch/docs-consistency/code/43-regulatory-disposition-acceptance-and-alternate-journey-have-no-production-face.md`
