# ADR-0255：`已提交`委托在决定前重组成员或改变共享约束，只有一条路——客户撤回原委托，再提交以撤回重提方向关联它的新委托；不开「待决」关联方向

Status: Accepted（2026-10-21，Claude Code 会话代 PS owner 决：用户 2026-10-21 原话「需要修改文档，确保自洽，正确」，常设授权「你作为业务和系统专家自决」；票 docs-consistency/06，出处审计 [audit/ps.md](../../.scratch/docs-consistency/audit/ps.md) PS-06（取证 [audit/ps-code.md](../../.scratch/docs-consistency/audit/ps-code.md) 第 9 条）。部分停用 [ADR-0132](./0132-authorized-disposition-decides-the-destination-of-a-restricted-request-as-its-own-wait-state-and-never-passes.md) 决定一「客户的补充若越出委托边界……」一句里「那是受控补充的一种结果」那一半，被停原句与适用场景见「对已接受记录的作用」；ADR-0132 正文不改，只在其 Status 行与 Links 加前向指针。代码不因本记录改判，撤回委托在真库上读不回的缺口另由 [docs-consistency/code/36](../../.scratch/docs-consistency/code/36-withdrawn-shipment-requests-cannot-be-read-back-from-the-store.md) 承接。越权风险点见文末，待 PS owner 复核。）<!-- 索引注记 -->｜**唯一路径**：决定前要删除、拆分、重组成员，或改变客户账户、合同责任范围、寄收件关系、目的服务范围，由客户或其当前有效授权代表先撤回原委托，再提交一份或多份以 `WITHDRAWN_RESUBMISSION` 关联原委托的新委托；撤回带来的任务停止、占用释放与声明撤除照常发生，新委托各自重新经过生产归属与接受判断。**成员去向**：由各新委托的声明成员与它们对原委托的回指表达，未进入任何新委托的成员随原委托撤回而终止。**不开方向**：关联新委托只锚在原委托的终态上，提交口对`已提交`原委托答 `LINK_INELIGIBLE`；受控补充只形成同一委托的新提交版本
Date: 2026-10-21

## Context

- PS CONTEXT Rules：「接受决定前，普通资料纠错或补充在同一委托下形成新的提交版本并重新判断；删除、拆分、重组成员，或者改变客户账户、合同责任范围、寄收件关系或目的服务范围时，必须形成一份或多份关联新委托并保留原成员去向。」它没写原委托在重组之后怎么处置。
- UC-PS-001 业务决策简报 BD-PS-007 边界示例：「客户可以在决定前明确重组，重组后的委托各自取得完整决定。」
- [ADR-0132](./0132-authorized-disposition-decides-the-destination-of-a-restricted-request-as-its-own-wait-state-and-never-passes.md) 决定一把越出委托边界的补充写成「受控补充的一种结果」。
- 代码（取证于 main `f329f5059`）：
  - `domain.RequestLinkKind` 是封闭集合：已拒绝修正（`REJECTED_CORRECTION`）、已接受重塑（`ACCEPTED_RESHAPING`）、已撤回重提（`WITHDRAWN_RESUBMISSION`），每一向都锚在原委托的一个终态上，`EstablishPriorRequestLink` 拿原委托本体互证。头注写明「刻意没有「已提交」方向」。
  - 提交口对指着`已提交`原委托的关联答 `LINK_INELIGIBLE`。
  - 新提交版本要求成员集合与当前版本等值（[ADR-0045](./0045-new-submission-version-keeps-history-and-reestablishes-the-task.md) 第 4 条），增删拆并被它拒绝。
  - 受控补充编排只形成同一委托的新提交版本，不形成关联新委托。
- 关联方向锚在终态、新版本成员等值，合起来决定前重组只剩「先撤回、再以撤回重提方向关联」一条路。这条路是代码有意留下的，但 CONTEXT、BD 简报与 ADR-0132 都没写它，读者会以为可以直接提交关联新委托。

## Decision

**一、决定前的重组只有一条路：先撤回，再关联重提。**

- 委托仍为`已提交`时，要删除、拆分、重组成员，或改变客户账户、合同责任范围、寄收件关系、目的服务范围，由客户或其当前有效授权代表先按 UC-PS-005 撤回整份原委托。
- 撤回成立时照常发生的事一件不少：接受判断任务停止；已成立控制项的占用按原业务关联释放；委托声明过的客户包裹件号在同一笔状态变化里交出声明撤除。
- 随后提交一份或多份新委托，每一份都以 `WITHDRAWN_RESUBMISSION` 方向关联原委托。新委托各自重新经过生产归属与接受判断，按 CONTEXT 委托共享约束各自成立。
- 撤回与重提是两个用例、两次提交，不要求同一笔事务。

**二、原成员去向由新委托的成员与回指表达。**

- 进入某份新委托的原成员，去向就是那份新委托；新委托的关联指回原委托，读关联的人可以对出每一件到了哪里。
- 没有进入任何新委托的原成员，随原委托撤回而终止服务请求。
- 本上下文不另记逐成员的去向表。关联只存在于新委托上，原委托的版本、决定与历史一概不动。

**三、不开「待决」关联方向。**

- 关联新委托只锚在原委托的终态上。原委托还是`已提交`时允许关联，就会有两份同时待判断的请求声明同一批成员，各自走到接受。
- 要停下原委托的判断，就是撤回；撤回已带齐任务停止、占用释放与声明撤除。另开一个「待决重组」方向，还得为它再造一遍同样的停止与补偿，等于第二条撤回路。
- 提交口对指着`已提交`原委托的关联照旧答 `LINK_INELIGIBLE`。客户该走的是：普通资料纠错走同一委托的新提交版本；重组先撤回原委托。

**四、受控补充只形成同一委托的新提交版本。**

- 任务停在`等待受控补充`时，客户补来的若越出委托边界（重组成员、换目的范围、换合同责任范围），它不是一次受控补充，受控补充编排也不为它形成关联新委托。
- 客户照第一条撤回原委托、提交关联新委托。授权处置选`交客户补充`之后同样如此。

## 候选与反方

- **新增「待决重组」方向，在同一笔里停止原任务并释放占用。** 否决：理由见决定三。它与撤回做的是同一件事，却多一套结果代数与补偿入口；客户在等待受控补充时还多了一条绕开版本机制的路。
- **允许新提交版本改成员集合，把重组也做成同一委托的新版本。** 否决：CONTEXT 把成员重组与共享约束变化判给关联新委托，ADR-0045 第 4 条据此拒绝它；一份委托的成员基线会在版本之间漂移，判断历史不再能按版本对齐。
- **由运营侧代客户在后台把原委托拆成几份。** 否决：撤回与提交都是客户意思表示，运营代录须是客户当前有效授权代表（UC-PS-005 的授权口径），不另设后门。

## 对已接受记录的作用

- **部分停用** ADR-0132 决定一「客户的补充若越出委托边界（重组成员、换目的范围、换合同责任范围），按 CONTEXT 既有规则「必须形成一份或多份关联新委托并保留原成员去向」——那是受控补充的一种结果，不是第三个去向。」一句里「那是受控补充的一种结果」那一半：越出委托边界的需求不由受控补充形成，客户撤回原委托后提交关联新委托（决定四）。**适用场景**：委托仍为`已提交`，含任务停在`等待受控补充`、授权处置选了`交客户补充`之后。同句「不是第三个去向」与 ADR-0132 其余各条不变。
- [ADR-0045](./0045-new-submission-version-keeps-history-and-reestablishes-the-task.md) 第 4 条「增删拆并是「成员重组」，CONTEXT 判给关联新委托」：不停用，本记录写出了关联新委托之前那一步。

## Consequences

- PS CONTEXT：Rules「接受决定前……必须形成一份或多份关联新委托并保留原成员去向」一条补上撤回在先与成员去向的表达；委托生命周期「成员重组或共享约束变化建立关联新委托」、接受判断任务生命周期「按成员边界形成关联新委托」两处随之改。
- UC-PS-001：接受条件段与「成员基线」行改写；业务决策简报 BD-PS-005、BD-PS-007 的决定与边界示例改写。UC-PS-005 补写「为重组而撤回」这一用途。
- 代码：判法不改。`OutcomeLinkIneligible` 头注「客户该走的是资料修订（待决）或等决定」改为指向新提交版本与撤回重提（随 [docs-consistency/code/38](../../.scratch/docs-consistency/code/38-parcel-shipment-comments-follow-docs-consistency-06.md)）。
- 已撤回委托今天在真库上读不回（重建门不开`已撤回`，撤回决定也不进快照），所以撤回重提关联与重复撤回在真库上走不通；这一缺口由 code/36 补齐，本记录的路径不变。

## 越权风险点

1. **客户多一步撤回**（PS owner）。撤回之后、重提之前，生产归属与准入可能已变：准入暂停时新委托进不来，原委托却已撤回。缓解：撤回与重提都留关联，客户回执应提示先确认能重提再撤回；准入暂停的回执另有一格（`ADMISSION_PAUSED`）。
2. **成员去向不逐件登记**（PS owner）。新委托没带客户包裹件号或客户参考时，原成员与新成员之间只能靠人对。缓解：首方契约的客户包裹参考可选但建议必带，属接入产品策略。
3. **撤回释放后新委托重新占用**（PS、SA owner）。原占用先释放、新占用后形成，中间若客户额度被他用，新委托的控制可能不通过。这是两次独立判断的正常后果，不另设预留。
4. **运营角色重组须有撤回授权**（PS owner）。BD-PS-007 写「客户或授权运营角色可以在接受决定前显式纠正成员」；照本记录，运营角色要重组，须是客户当前有效授权代表、持有撤回授权（`PAR-COM-14` 的租户取值）。

## Links

- [ADR-0132](./0132-authorized-disposition-decides-the-destination-of-a-restricted-request-as-its-own-wait-state-and-never-passes.md)：部分停用决定一「客户的补充若越出委托边界……」一句里「那是受控补充的一种结果」那一半
- [ADR-0045](./0045-new-submission-version-keeps-history-and-reestablishes-the-task.md)：新提交版本成员集合等值
- [ADR-0106](./0106-customer-supplement-wait-is-a-committed-pause-resumed-by-the-new-submission-version-envelope.md)：受控补充由新提交版本信封续办
- [ADR-0211](./0211-an-ambiguity-only-identity-conflict-releases-by-recheck-once-the-declaration-is-gone.md)：撤回时交出声明撤除
- [ADR-0061](./0061-accepted-shipment-request-rehydration-by-snapshot-expressiveness.md)、[ADR-0188](./0188-rejected-shipment-request-rehydration-by-snapshot-expressiveness.md)：重建门开到哪几格
- [PS CONTEXT](../domain/parcel-shipment/CONTEXT.md)；[UC-PS-001](../application/parcel-shipment/UC-PS-001-SUBMIT-SHIPMENT-REQUEST.md)、[业务决策简报](../application/parcel-shipment/UC-PS-001-BUSINESS-DECISION-BRIEF.md)、[UC-PS-005](../application/parcel-shipment/UC-PS-005-WITHDRAW-SUBMITTED-SHIPMENT-REQUEST.md)
- 票 `.scratch/docs-consistency/issues/06-ps-docs.md`；代码票 `.scratch/docs-consistency/code/36-withdrawn-shipment-requests-cannot-be-read-back-from-the-store.md`、`.scratch/docs-consistency/code/38-parcel-shipment-comments-follow-docs-consistency-06.md`
