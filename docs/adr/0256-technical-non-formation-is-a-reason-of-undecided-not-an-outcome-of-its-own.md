# ADR-0256：「技术未形成」不是一格独立结果——原始请求保全之后提交没有落成的，有未决格的编排答未决并指名哪一步，重放须接着办；保全本身没完成的，没有形成答案

Status: Accepted（2026-10-21，Claude Code 会话代 PS owner 决：用户 2026-10-21 原话「需要修改文档，确保自洽，正确」，常设授权「你作为业务和系统专家自决」；票 docs-consistency/06，出处审计 [audit/ps.md](../../.scratch/docs-consistency/audit/ps.md) PS-21（取证 [audit/ps-code.md](../../.scratch/docs-consistency/audit/ps-code.md) 第 6 条），兼 PS-33。无条款停用：相抵的是 UC-PS-001、UC-PS-002、UC-PS-004、UC-PS-005 的写法，结果分格上代码与 [ADR-0029](./0029-retrieval-failure-algebra-is-partitioned-by-recovery-action.md) 一致；重放接着办这一条，提交、撤回、资料修订三只编排在保全已落下而结果没落成的那几格上今天做不到，代码随票 [docs-consistency/code/39](../../.scratch/docs-consistency/code/39-a-preserved-submission-whose-build-failed-is-never-built-on-replay.md)（提交）与 [docs-consistency/code/44](../../.scratch/docs-consistency/code/44-withdrawal-and-amendment-replays-short-circuit-after-an-undecided-round.md)（撤回、资料修订）。评审轮修订 2026-10-21：提交编排不在第 2 格内；续办改为重放须接着办。越权风险点见文末，待 PS owner 复核。）<!-- 索引注记 -->｜**两格**：来源保全（含认证与译成命令）没有完成，本上下文没有留下这次到达，不形成任何应用结果，接 HTTP 答 5xx、调用方原样重发；原始请求已保全、判断或决定的保存、标识签发、发布意图没有落成，有未决格的编排答未决格并以原因码指名哪一步没落成；提交编排没有未决格，保全之后出错照样答 5xx。**续办**：重放同一请求必须接着办，不得因来源已保全就短路成已有结果——今天受控补充编排做到；提交、撤回、资料修订三只在保全已落下而结果没落成的那几格上会短路（资料修订是版本落库之前停下的各格）。**判据**：按恢复动作分格（ADR-0029）——本方没落库与依赖答不出的恢复动作相同，分在原因码上，不分在结果格上。**仍禁**：不得答成业务拒绝、已形成结果或已采用｜**缺口已补**（2026-10-21）：越权风险点 3 所说的提交编排缺口（重发得到一个不指名委托的已有结果）已由票 code/39 补齐；提交编排的重放格局见 [ADR-0266](./0266-a-submission-replay-answers-what-the-first-round-answered-and-only-unresolved-ownership-is-judged-again.md)。｜**缺口已补**（2026-10-26）：撤回与资料修订编排的短路已由票 code/44 补齐——撤回的重放按委托此刻的状态判（已越过提交边界答已有决定并带之，仍为`已提交`接着办），资料修订的重放在委托的资料版本里按请求身份找版本（找到答已有结果并带版本，找不到接着办）。正文不变
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
  - 提交编排 `SubmitShipmentRequestHandler` 的结果 `SubmitOutcome` 没有未决格：按 [ADR-0081](./0081-acceptance-judgment-is-envelope-driven.md)，提交只答生产归属与`已提交`。来源保全单独成一笔事务，其后 `DecideProductionOwnership`、签发标识、`Insert` 出错都上抛，HTTP 答 5xx `NO_ANSWER_FORMED`。
  - 重放：来源已保全而结果没落成时，提交、撤回、资料修订三只编排都走已有结果那条路——提交答不指名委托的 `EXISTING_RESULT`；撤回在业务未决、读不回与 `STALE_SHIPMENT_REQUEST_REVISION` 之后（撤回的装配事务在编排交回业务答案时照样提交保全）对仍为`已提交`的委托答 `DECISION_ALREADY_FORMED`、不带决定；资料修订的来源保全自成一笔事务（`preservationBoundary`），版本落库之前停下的各格——授权、阶段、矩阵等业务未决，读不回，`AMENDED_REQUEST_NOT_SAVED` 与 `STALE_SHIPMENT_REQUEST_REVISION`——重放都答 `ALREADY_HANDLED`、不带版本。资料修订停在 `SOURCE_DATA_VERSION_NOT_HANDED_OFF` 之后的重放不在其中：版本已落库，`existing` 读回版本并重发交接意图，是接着办了的。受控补充编排 `FormNewSubmissionVersionHandler` 重放时接着办。
- 02 的完成记录把这一格列为条件性 B 类：反向裁定则相关编排各加一格 `NOT_FORMED`。

## Decision

**一、两格，按保全有没有完成分。**

1. **没有形成答案。** 来源保全没有完成——包括保全之前的认证、把请求译成命令——本上下文没有留下这次请求到达的事实。不形成任何应用结果；接 HTTP 答 5xx（`INTAKE_FAILED`、`NO_ANSWER_FORMED`），调用方原样重发同一请求。
2. **未决，原因是本方的提交没有落成。** 原始请求已经保全，之后判断或决定的保存、标识签发、发布意图有一步没有落成。有未决格的编排答它的未决格（例如撤回的 `UNDECIDED`、终局的 `FINAL_UNDECIDED`、取消的 `CANCELLATION_UNDECIDED`），原因码指名是哪一步（例如 `DECISION_NOT_RECORDED`、`AMENDED_REQUEST_NOT_SAVED`、`FINAL_STORE_UNAVAILABLE`、`CANCELLATION_STORE_UNAVAILABLE`、`STALE_SHIPMENT_REQUEST_REVISION`），带续办引用。
   - **提交编排不在这一格内。** 它没有未决格（ADR-0081 下提交只答生产归属与`已提交`），保全之后出错照样没有形成答案、答 5xx；但它与第 1 格不同，这次到达已经留下了保全，所以重发必须接着办。
   - **续办是内部重试，或重放同一请求，重放须接着办**：来源保全认出这是同一请求之后，编排要按这一次实际走到的位置把没落成的那一步做完，不得因为来源已保全就短路成已有结果。今天受控补充编排做到；提交、撤回、资料修订三只在保全已落下而结果没落成的那几格上会短路成已有结果（资料修订是版本落库之前停下的各格），这次请求从此不会形成——现状缺口，由 code/39（提交）与 code/44（撤回、资料修订）补齐。

**二、未决按原因分，不按结果格分。**

- 「依赖答不出」与「本方没落库」的恢复动作相同：本进程重试，或客户重放。按 ADR-0029，恢复动作相同就是同一格，差别落在原因码上。
- 运维与客户端要区分两者时读原因码，不读结果格。

**三、仍然禁止的。**

- 不得答成业务拒绝；不得答成已形成结果（撤回成立、资料已记录并采用、终局已形成）；不得让下游按一份没落库的结果办事。
- 这些禁令原先写在「技术未形成」那一格里，自此写在未决格的禁止行为里。

**四、用例里「技术未形成」不再作结果名。**

- 用例的元数据、结果表与步骤改写为决定一的两格之一：保全没完成写「没有形成答案」，保全之后没落成写「未决（本方提交未完成）」。

## 候选与反方

- **相关编排各加一格 `NOT_FORMED`。** 否决：它的恢复动作与依赖类未决相同，单列一格只是让消费门与客户端多认一个同义值；ADR-0029 否决的正是按失败原因分格。
- **把保全失败也算进未决。** 否决：保全没完成时本上下文连这次请求到达过都不知道，没有续办引用可给；答成未决等于承诺了一件不存在的续办。

## 对已接受记录的作用

- 无条款停用。本记录是 ADR-0029 在 PS 用例结果语义上的落法；[ADR-0022](./0022-http-status-carries-answer-formed-not-business-verdict.md) 的「没有形成答案」照旧由 5xx 承载。

## Consequences

- UC-PS-001「尚未决定」一行注明它包含本方提交未完成；UC-PS-002 结果表去掉「技术未形成」行、步骤 1 改为「不形成任何结果」、元数据改两格；UC-PS-004、UC-PS-005 元数据「未形成结束」一行改为「没有形成答案」，未决结束一行补上本方提交未完成。
- PS CONTEXT「客户原始资料版本」生命周期里「技术未形成」一词改为同一组说法。
- 结果分格不改代码。重放接着办要改代码：提交编排随 code/39，撤回与资料修订编排随 code/44（资料修订编排里「重放这次修订时来源保全会认出同一个身份」那段注释的误判一并改）。

## 越权风险点

1. **客户端只凭结果格分不出业务未决与本方落库失败**（PS owner）。要分就得读原因码；首方契约的 `outcome` 封闭词表因此不必多一格，但文档须写明未决的原因码集合会增长、未识别的原因按内部续办处理。
2. **未决统计里混着本方故障**（PS owner）。运维指标要按原因码拆，否则一次库故障看上去像一批业务未决。
3. **提交编排保全之后出错答 5xx 而不是未决**（PS owner）。客户端拿不到续办引用，只能原样重发；这一格要靠决定一第 2 格「重发必须接着办」才成立，落地归 code/39 修法第 2 条；code/39 落地之前重发得到一个不指名委托的已有结果。

## Links

- [ADR-0029](./0029-retrieval-failure-algebra-is-partitioned-by-recovery-action.md)：按恢复动作分格
- [ADR-0022](./0022-http-status-carries-answer-formed-not-business-verdict.md)：状态码只回答有没有形成答案
- [ADR-0031](./0031-owned-repository-write-outcome-is-a-closed-algebra-not-an-error.md)：保存结果是封闭代数，`决定没落库` 与 `换代冲突` 分格
- [ADR-0081](./0081-acceptance-judgment-is-envelope-driven.md)：提交编排不做接受判断，提交答复没有未决格
- [UC-PS-001](../application/parcel-shipment/UC-PS-001-SUBMIT-SHIPMENT-REQUEST.md)、[UC-PS-002](../application/parcel-shipment/UC-PS-002-AMEND-CUSTOMER-SOURCE-DATA.md)、[UC-PS-004](../application/parcel-shipment/UC-PS-004-FORM-PARCEL-FINAL-SERVICE-OUTCOME.md)、[UC-PS-005](../application/parcel-shipment/UC-PS-005-WITHDRAW-SUBMITTED-SHIPMENT-REQUEST.md)
- 票 `.scratch/docs-consistency/issues/06-ps-docs.md`；代码票 `.scratch/docs-consistency/code/39-a-preserved-submission-whose-build-failed-is-never-built-on-replay.md`、`.scratch/docs-consistency/code/44-withdrawal-and-amendment-replays-short-circuit-after-an-undecided-round.md`
