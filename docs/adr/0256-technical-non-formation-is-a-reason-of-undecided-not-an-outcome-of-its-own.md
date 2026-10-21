# ADR-0256：「技术未形成」不是一格独立结果——原始请求保全之后提交没有落成的，答未决并指名哪一步；保全本身没完成的，没有形成答案

Status: Accepted（2026-10-21，Claude Code 会话代 PS owner 决：用户 2026-10-21 原话「需要修改文档，确保自洽，正确」，常设授权「你作为业务和系统专家自决」；票 docs-consistency/06，出处审计 [audit/ps.md](../../.scratch/docs-consistency/audit/ps.md) PS-21（取证 [audit/ps-code.md](../../.scratch/docs-consistency/audit/ps-code.md) 第 6 条），兼 PS-33。无条款停用：相抵的是 UC-PS-001、UC-PS-002、UC-PS-004、UC-PS-005 的写法，代码与 [ADR-0029](./0029-retrieval-failure-algebra-is-partitioned-by-recovery-action.md) 一致。代码不改。越权风险点见文末，待 PS owner 复核。）<!-- 索引注记 -->｜**两格**：来源保全（含认证与译成命令）没有完成，本上下文没有留下这次到达，不形成任何应用结果，接 HTTP 答 5xx、调用方原样重发；原始请求已保全、判断或决定的保存、标识签发、发布意图没有落成，答该编排的未决格并以原因码指名哪一步没落成，续办是内部重试或重放同一请求。**判据**：按恢复动作分格（ADR-0029）——本方没落库与依赖答不出的恢复动作相同，分在原因码上，不分在结果格上。**仍禁**：不得答成业务拒绝、已形成结果或已采用
Date: 2026-10-21

## Context

以下所引用例原文是本记录接受前的文字，已随本记录改写（见 Consequences）。

- PS 用例对「技术未形成」说法不一：
  - UC-PS-005 元数据「未形成结束｜技术提交未完成；不得伪装成撤回、拒绝或未决」；UC-PS-004 元数据「未形成结束｜并发或技术提交未完成；不得伪装为未决或业务拒绝」——技术未形成与未决是两格。
  - UC-PS-002 结果表另有「技术未形成」一行，定义是「原始请求已保全，但版本提交或发布意图未完成」；同一份用例步骤 1 却写「无法保全时形成技术未形成」。
  - UC-PS-001「尚未决定」一行写「依赖不可用、并发冲突、处理失败或尚未形成权威判断」——处理失败算进未决。
- 代码（取证于 main `f329f5059`）按恢复动作分格，与 ADR-0029 一致：
  - 撤回编排保存失败答 `UNDECIDED`、原因 `DECISION_NOT_RECORDED`；资料修订保存失败答 `UNDECIDED`、原因 `AMENDED_REQUEST_NOT_SAVED`；终局编排保存失败答 `FINAL_UNDECIDED`、原因 `FINAL_STORE_UNAVAILABLE`；取消编排保存失败答 `CANCELLATION_UNDECIDED`、原因 `CANCELLATION_STORE_UNAVAILABLE`；并发写入抢先各处答 `STALE_SHIPMENT_REQUEST_REVISION`。
  - 来源保全失败（以及保全之前的接入失败）不形成应用结果，客户面 HTTP 适配器答 5xx `INTAKE_FAILED` 或 `NO_ANSWER_FORMED`（[ADR-0022](./0022-http-status-carries-answer-formed-not-business-verdict.md)：状态码只回答有没有形成答案）。
- 02 的完成记录把这一格列为条件性 B 类：反向裁定则相关编排各加一格 `NOT_FORMED`。

## Decision

**一、两格，按保全有没有完成分。**

1. **没有形成答案。** 来源保全没有完成——包括保全之前的认证、把请求译成命令——本上下文没有留下这次请求到达的事实。不形成任何应用结果；接 HTTP 答 5xx（`INTAKE_FAILED`、`NO_ANSWER_FORMED`），调用方原样重发同一请求。
2. **未决，原因是本方的提交没有落成。** 原始请求已经保全，之后判断或决定的保存、标识签发、发布意图有一步没有落成。答该编排的未决格（例如撤回的 `UNDECIDED`、终局的 `FINAL_UNDECIDED`、取消的 `CANCELLATION_UNDECIDED`），原因码指名是哪一步（例如 `DECISION_NOT_RECORDED`、`AMENDED_REQUEST_NOT_SAVED`、`FINAL_STORE_UNAVAILABLE`、`CANCELLATION_STORE_UNAVAILABLE`、`STALE_SHIPMENT_REQUEST_REVISION`），带续办引用。续办是内部重试，或重放同一请求——来源保全会认出它，走已有结果那条路。

**二、未决按原因分，不按结果格分。**

- 「依赖答不出」与「本方没落库」的恢复动作相同：本进程重试，或客户重放。按 ADR-0029，恢复动作相同就是同一格，差别落在原因码上。
- 运维与客户端要区分两者时读原因码，不读结果格。

**三、仍然禁止的。**

- 不得答成业务拒绝；不得答成已形成结果（撤回成立、资料已记录并采用、终局已形成）；不得让下游按一份没落库的结果办事。
- 这些禁令原先写在「技术未形成」那一格里，自此写在未决格的禁止行为里。

**四、用例里「技术未形成」不再作结果名。**

- 用例的元数据、结果表与步骤改写为上面两格之一：保全没完成写「没有形成答案」，保全之后没落成写「未决（本方提交未完成）」。

## 候选与反方

- **相关编排各加一格 `NOT_FORMED`。** 否决：它的恢复动作与依赖类未决相同，单列一格只是让消费门与客户端多认一个同义值；ADR-0029 否决的正是按失败原因分格。
- **把保全失败也算进未决。** 否决：保全没完成时本上下文连这次请求到达过都不知道，没有续办引用可给；答成未决等于承诺了一件不存在的续办。

## 对已接受记录的作用

- 无条款停用。本记录是 ADR-0029 在 PS 用例结果语义上的落法；[ADR-0022](./0022-http-status-carries-answer-formed-not-business-verdict.md) 的「没有形成答案」照旧由 5xx 承载。

## Consequences

- UC-PS-001「尚未决定」一行注明它包含本方提交未完成；UC-PS-002 结果表去掉「技术未形成」行、步骤 1 改为「不形成任何结果」、元数据改两格；UC-PS-004、UC-PS-005 元数据「未形成结束」一行改为「没有形成答案」，未决结束一行补上本方提交未完成。
- PS CONTEXT「客户原始资料版本」生命周期里「技术未形成」一词改为同一组说法。
- 代码不改。

## 越权风险点

1. **客户端只凭结果格分不出业务未决与本方落库失败**（PS owner）。要分就得读原因码；首方契约的 `outcome` 封闭词表因此不必多一格，但文档须写明未决的原因码集合会增长、未识别的原因按内部续办处理。
2. **未决统计里混着本方故障**（PS owner）。运维指标要按原因码拆，否则一次库故障看上去像一批业务未决。

## Links

- [ADR-0029](./0029-retrieval-failure-algebra-is-partitioned-by-recovery-action.md)：按恢复动作分格
- [ADR-0022](./0022-http-status-carries-answer-formed-not-business-verdict.md)：状态码只回答有没有形成答案
- [ADR-0031](./0031-owned-repository-write-outcome-is-a-closed-algebra-not-an-error.md)：保存结果是封闭代数，`决定没落库` 与 `换代冲突` 分格
- [UC-PS-001](../application/parcel-shipment/UC-PS-001-SUBMIT-SHIPMENT-REQUEST.md)、[UC-PS-002](../application/parcel-shipment/UC-PS-002-AMEND-CUSTOMER-SOURCE-DATA.md)、[UC-PS-004](../application/parcel-shipment/UC-PS-004-FORM-PARCEL-FINAL-SERVICE-OUTCOME.md)、[UC-PS-005](../application/parcel-shipment/UC-PS-005-WITHDRAW-SUBMITTED-SHIPMENT-REQUEST.md)
- 票 `.scratch/docs-consistency/issues/06-ps-docs.md`
