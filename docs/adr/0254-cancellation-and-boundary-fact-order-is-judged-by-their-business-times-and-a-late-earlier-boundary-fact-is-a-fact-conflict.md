# ADR-0254：包裹取消与取消边界事实的先后按两者的业务时间比——边界事实不晚于取消、却在取消提交之后才到，是事实冲突：停在未决，不采用也不抹取消

Status: Accepted（2026-10-21，Claude Code 会话代 PS owner 决：用户 2026-10-21 原话「需要修改文档，确保自洽，正确」，常设授权「你作为业务和系统专家自决」；票 docs-consistency/06，出处审计 [audit/ps.md](../../.scratch/docs-consistency/audit/ps.md) PS-03（取证 [audit/ps-uc.md](../../.scratch/docs-consistency/audit/ps-uc.md) PS-UC-01）。无条款停用：此前没有记录裁过这一格，相抵的是 PS CONTEXT 一句的两种读法与 UC-PS-003、UC-PS-006 各一套结果。代码随票 [docs-consistency/code/35](../../.scratch/docs-consistency/code/35-cancellation-and-boundary-fact-order-conflict-has-no-label-half-and-no-resolution-face.md)，提交边界那一半在 [docs-consistency/code/21](../../.scratch/docs-consistency/code/21-label-channel-parcel-cancellation-ignores-carrier-pickup-and-finals.md)。越权风险点见文末，待 PS owner 复核，第 3 点知会 TF owner。）<!-- 索引注记 -->｜**比什么**：取消决定所带的业务时间（取消请求经渠道声明的业务发生时间）对边界事实的实际发生时间——网络服务是被采用的收寄来源，面单渠道服务是实际承运商首次有效收寄；决定提交时刻、消息到达顺序都不参与。**三格**：取消到提交边界时边界事实已在册 → 不形成取消；取消已提交、后到的边界事实发生晚于取消 → 取消在先，边界事实不采用为责任起点或非取消终局；后到的边界事实发生不晚于取消 → 事实冲突，收寄采用或面单终局判断停在未决，已提交的取消不被自动改写。**解开**：冲突只由一份明确的业务决定解开，决定面今天没有，冲突停在未决交人
Date: 2026-10-21

## Context

- PS CONTEXT Rules：「形成取消决定前必须在同一决定提交边界重新核验当前有效收寄及终局事实；并发收寄已经先行成立时不得提交取消，消息晚到不能改变这一裁决」。「先行成立」可以读成业务时间在先，也可以读成提交时已采用。
- 两种读法在一种情形上给出相反结果：收寄在业务时间上早于取消，取消先越过提交边界，收寄消息后到。
  - UC-PS-003 `AT-PS-045`「收寄事实先发生，取消消息后到 | 按业务时间保留责任起点；取消不能追溯抹去收寄」，读来是收寄胜。照它实现，同一件包裹会同时带着取消终局与责任起点。
  - UC-PS-006 决定矩阵「收寄事实存在但有效性或业务顺序待确认 | 取消/处置未决」与 `AT-PS-081`「取消与处置均保持未决」，读来是未决。
- 代码（取证于 main `f329f5059`）：
  - 收寄采用编排 `AdoptNetworkIntakeHandler` 读到已成立的取消时，`cancellation.RequestedAt()` 早于 `source.OccurredAt()` 就不采用（原因 `CANCELLED_BEFORE_INTAKE/<取消标识>`），否则答 `ELIGIBILITY_UNDECIDED`、原因 `CANCELLATION_ORDER_CONFLICT`。测试 `TestTheCancellationBoundaryJudgesByBusinessTime` 钉住这两格。注释写的是「取消决定」，比的却是取消请求的业务时间。
  - 取消决定 `ParcelCancellation` 只记一个时间 `RequestedAt`，头注称它是「取消的业务时间」；取消编排 `CancelParcelHandler` 的接入面说这个时间「由渠道契约声明」。
  - 面单渠道服务一侧，`JudgeLabelServiceFinal` 只要读到有效取消就答 `CANCELLATION_STANDS`，不比承运商首次有效收寄的发生时间。UC-PS-004 `AT-PS-095`「已有有效取消结果，随后收到首次有效收寄 → 取消在先，不形成非取消终局」照此写。
- 取消编排在提交边界上只重读网络收寄的采用链；面单渠道服务的边界核验缺口已由 code/21 立票，它的「要裁的」一节把晚到这一格交给本记录。

## Decision

**一、比的是两个业务时间，不是提交顺序。**

- 取消一侧：取消决定所带的业务时间，即取消请求经渠道声明的业务发生时间（取消决定上的 `RequestedAt`）。取消在业务上从这一刻起成立，客户也是在这一刻说的「不要了」。
- 边界事实一侧：网络服务是被采用的节点收寄或场外揽收来源的实际发生时间；面单渠道服务是实际承运商首次有效收寄的实际发生时间。
- 决定提交时刻、消息到达时刻、数据库写入时刻都不参与比较。它们说的是本系统何时知道，不是包裹何时越过边界。

**二、逐包裹三格。**

1. **取消到达提交边界时，边界事实已经在册**（网络服务的采用链上已有责任起点；面单渠道服务已有首次有效收寄或当前有效终局）：不形成取消。网络服务转收寄后处置判断（取消编排答 `DISPOSITION_PENDING`），面单渠道服务答既有终局或不可取消。这一格不比时间：提交边界上看得见的边界事实，无论发生在请求之前还是之后，都使取消不成立（CONTEXT「并发收寄已经先行成立时不得提交取消」）。
2. **取消已经提交，此后到达的边界事实发生晚于取消的业务时间**：取消在先。网络服务的收寄来源不采用为责任起点（`SOURCE_NOT_ADOPTED`，原因 `CANCELLED_BEFORE_INTAKE/<取消标识>`），物理事实保留并交明确后续处置；面单渠道服务不形成非取消终局（`CANCELLATION_STANDS`）。
3. **取消已经提交，此后到达的边界事实发生不晚于取消的业务时间**：事实冲突。取消形成时包裹其实已经越过取消边界，只是本系统还不知道。不按到达顺序选边：
   - 网络服务的收寄采用停在未决（`ELIGIBILITY_UNDECIDED`，原因 `CANCELLATION_ORDER_CONFLICT`），不形成责任起点与正式承诺；
   - 面单渠道服务的终局判断同样停在未决，不形成非取消终局，也不按取消在先作答；
   - 已经提交的取消不被自动改写、撤销或覆盖，物理事实照旧保留。
- 两个时间相等按第 3 格办。相等说明两者在业务上同时发生，哪一边在先都没有依据。

**三、冲突只由一份明确的业务决定解开。**

- 系统不自动解开：自动认取消，等于让一件已交到网络或承运商手里的包裹挂着取消终局；自动认收寄，等于静默撤掉一个已提交的不可覆盖决定。
- 解开它的决定由有权主体形成，形状本记录不定。可能的去向例如：认定取消依据不成立、把包裹转入收寄后服务处置；或由来源方更正边界事实的发生时间，再按第二条重判。
- 今天没有这个决定面。冲突停在未决、交人，未决原因按包裹查得出。决定面与面单渠道那一半的判法由 code/35 承接。

## 候选与反方

- **收寄胜：按业务时间保留责任起点，取消视为无效**（`AT-PS-045` 的字面读法）。否决：取消已越过不可覆盖的决定边界，静默作废它违反「已经合法提交的首个决定获胜」；包裹同时带取消终局与责任起点的状态在今天的模型里就出现了。
- **取消胜：已提交的取消一律优先，后到的边界事实一律不采用**（面单渠道今天的做法）。否决：包裹已在网络或承运商手里，客户被告知已取消，履约却在继续，责任与费用无人认领；CONTEXT 明写边界越过之后「包裹不得回退为“已取消”」。
- **比取消决定的提交时刻，不比取消请求的业务时间。** 否决：提交时刻是本系统的写入时刻，CONTEXT 不许它单独裁决业务先后；客户在边界前提出、本系统晚了几分钟才提交的取消，会被判成冲突。
- **网络服务照此办，面单渠道服务另议。** 否决：CONTEXT 的取消边界一句对两种服务形态同写；两边各一套先后判法，面单渠道包裹会在同一情形下得出与网络包裹相反的结论。

## 对已接受记录的作用

- 无条款停用。此前没有记录裁过取消与边界事实的先后；[ADR-0117](./0117-same-source-correction-forms-a-superseding-adoption-version-chained-to-the-current-responsibility-start.md) 管同来源更正，[ADR-0135](./0135-carrier-first-effective-pickup-is-a-judged-control-fact-with-its-own-registry-and-enters-the-segment.md) 管承运商首次有效收寄的判定，都不涉及取消。

## Consequences

- PS CONTEXT「形成取消决定前……消息晚到不能改变这一裁决」一句改写为本记录的三格，网络服务与面单渠道服务同用。
- UC-PS-003：`AT-PS-045` 改写为第 1 格（收寄已采用后取消到达）；新立 `AT-PS-107` 写第 3 格；「一致性、幂等与并发」里「取消与收寄并发」一条随之改。
- UC-PS-006：决定矩阵「收寄事实存在但有效性或业务顺序待确认」一行、`AT-PS-080`、`AT-PS-081` 按第 2、3 格改写。
- UC-PS-004：`AT-PS-095` 限到第 2 格；新立 `AT-PS-108` 写面单渠道的第 3 格，现状缺口指 code/35。
- 代码随票 code/35：面单渠道终局判断读到有效取消时比承运商首次有效收寄的发生时间，不晚于取消的业务时间即停在未决；冲突的解开决定面；收寄采用编排那段注释的「取消决定」改为「取消的业务时间」。提交边界上读面单渠道边界事实那一半归 code/21。

## 越权风险点

1. **比的是渠道声明的取消业务时间**（PS owner）。渠道把时间报早了，本应冲突的会判成取消在先。缓解：取消请求的业务时间与接收时间分开保存，审计看得出两者相差多少；渠道契约属接入实例半边。
2. **冲突没有解开的决定面**（PS owner）。在 code/35 落地前，冲突包裹没有责任起点，也不触发收寄后的路由复核，实物照旧在节点手里。缓解：未决原因 `CANCELLATION_ORDER_CONFLICT` 按包裹查得出，运维可人工跟进；首发前只有演示租户。
3. **面单渠道半边改了 `AT-PS-095` 的适用面**（PS owner，知会 TF owner）。今天的代码对承运商收寄早于取消的情形答 `CANCELLATION_STANDS`；本记录改判为未决。TF 侧首次有效收寄的判定与发生时间不变，只是 PS 多比一次。
4. **两个时间相等按冲突**（PS owner）。若 owner 认为同一时刻应按取消在先，改的是比较符一处，三格的结构不变。

## Links

- [PS CONTEXT](../domain/parcel-shipment/CONTEXT.md)：Rules 取消边界与「形成取消决定前」一条
- [UC-PS-003](../application/parcel-shipment/UC-PS-003-ESTABLISH-NETWORK-INTAKE-AND-FORMAL-COMMITMENT.md)（`AT-PS-044`、`AT-PS-045`、`AT-PS-107`）、[UC-PS-006](../application/parcel-shipment/UC-PS-006-CANCEL-PARCEL-OR-COORDINATE-POST-INTAKE-DISPOSITION.md)（决定矩阵、`AT-PS-079`–`AT-PS-081`）、[UC-PS-004](../application/parcel-shipment/UC-PS-004-FORM-PARCEL-FINAL-SERVICE-OUTCOME.md)（`AT-PS-095`、`AT-PS-108`）
- [ADR-0029](./0029-retrieval-failure-algebra-is-partitioned-by-recovery-action.md)：冲突停在未决、按恢复动作分格
- [ADR-0117](./0117-same-source-correction-forms-a-superseding-adoption-version-chained-to-the-current-responsibility-start.md)、[ADR-0135](./0135-carrier-first-effective-pickup-is-a-judged-control-fact-with-its-own-registry-and-enters-the-segment.md)：相邻而不相涉
- 票 `.scratch/docs-consistency/issues/06-ps-docs.md`；代码票 `.scratch/docs-consistency/code/35-cancellation-and-boundary-fact-order-conflict-has-no-label-half-and-no-resolution-face.md`、`.scratch/docs-consistency/code/21-label-channel-parcel-cancellation-ignores-carrier-pickup-and-finals.md`
