# ADR-0243：关务建案、授权确认、提交执行三口与税费付款、放行门禁两只核对编排的首发答法并进 UC 原行——被替代的格移出原 AT 行，提交执行时就绪没登也答未决，读不动与写不进答未决并指名那一口

Status: Accepted（2026-10-21，Claude Code 会话代决：用户原话「需要修改文档，确保自洽，正确」与常设授权「你作为业务和系统专家自决」；票 docs-consistency/03，代 CC owner 裁定审计 cc H1、M1、M2、M3、M4 的文档处理，评审时决定二扩到 UC-CC-009 的两只核对编排。部分停用 [ADR-0227](./0227-customs-case-and-declaration-submission-enter-production-through-operator-decision-faces.md) 决定八两句（一停一扩），见「对已接受记录的作用」。越权风险点见文末，待 CC owner 复核。）<!-- 索引注记 -->｜**第三格**：提交执行时单元的就绪判断没登，首发答未决并指名 `READINESS_UNCONFIGURED`（`AT-CC-448`），与 ADR-0227 决定八的另两格同一原则，替代 UC-CC-005 `AT-CC-109` 在「不存在」一格的结果名。**读不动与写不进**：UC 的「未形成授权结果」「未形成提交处理结果」在两口首发都答未决并指名那一口（授权确认 `READINESS_UNAVAILABLE`、`AUTHORITY_STORE_UNAVAILABLE`；提交执行 `CASE_LOOKUP_UNAVAILABLE`、`READINESS_UNAVAILABLE`、`AUTHORITY_UNAVAILABLE`、`UNIT_STORE_UNAVAILABLE`、`VERSION_IDENTITY_UNAVAILABLE`、`SUBMISSION_STORE_UNAVAILABLE`），HTTP 200 带原因，与建案（ADR-0227 决定二）同一写法；UC-CC-009「未形成税费/门禁核对」同此，税费付款核对与放行门禁核对两只编排都答 `UNDECIDED` 并指名读不动或写不进的那一口，`DUTY_VERIFICATION_HANDOFF_UNAVAILABLE` 只属资金事实新版本的重派编排（整笔回滚、消费门重投），人重核与门禁核对的交接失败照答已形成并带续办引用；动作准入照 ADR-0206 决定五保留读不动，不在此列；与「未配置」「判不了」同一个结果码，靠原因分续办。**UC 写法**：同一场景只留一套结果——被首发答法替代的格从原 AT 行移出、原行只留仍成立的格，结果表补首发结果码对照，不再行旁注明「首发答法见……」
Date: 2026-10-21

## Context

- 审计 cc H1、M2（`.scratch/docs-consistency/audit/cc.md`，钉 `6b0074d0`；2026-10-21 按 main `205eb254` 复核，现象仍在）：UC-CC-005 `AT-CC-109`「执行请求到达时就绪不存在、已失效或与目标不一致 | 返回`提交前门禁已阻断`」，而 `SubmitDeclarationHandler.Handle` 在 `LoadReadiness` 答未登记时答 `DECLARATION_UNDECIDED` + `READINESS_UNCONFIGURED`（头注「就绪规则/资格目录是实例半边：未配置停在未决，不默认就绪」），只有已失效才答 `NOT_READY`。ADR-0227 决定八定了原则——「授权确认时单元的就绪判断没登、提交执行时单元的提交授权没登，首发都答未决并指名未配置……提交编排早已这样答」——但它列的替代范围只有 `AT-CC-105` 两格与 `AT-CC-110` 一格，漏了提交执行口上就绪没登这一格。
- 决定八对已列的两行的处理是「两行的正文不改，行旁注明首发答法见 `AT-CC-444` / `AT-CC-439` 与本条」。文档自洽修订的目标之一是「同一场景只有一套结果」（`.scratch/docs-consistency/README.md`「目标」2）：两行仍断言旧结果名，只靠括注提示另有答法，照 AT 写测试的人会断言错的那一个。
- 审计 cc M1、M3、M4：UC 的「未形成授权结果」（`AT-CC-103`）、「未形成案件」（`AT-CC-016`）、「未形成提交处理结果」（`AT-CC-125`）三格，在代码里都是「未决 + 读不动或写不进那一口」：`ConfirmSubmissionAuthorizationHandler` 答 `AUTHORIZATION_UNDECIDED` + `READINESS_UNAVAILABLE` / `AUTHORITY_STORE_UNAVAILABLE`；`EstablishCaseHandler` 答 `UNDECIDED` + `CASE_REQUIREMENT_UNAVAILABLE` 等（ADR-0227 决定二已定「建案未决的原因此后分格……HTTP 以 200 带原因作答」）；`SubmitDeclarationHandler` 答 `DECLARATION_UNDECIDED` + 读不动或写不进的那一册。授权确认与提交执行两口的这一写法没有 ADR 明说，ADR-0227 越权风险点 9 只是以「未决」称呼它们。

- 本记录接受前的隔离评审发现 UC-CC-009 同一场景两套结果：新加的「放行门禁核对未决」把依赖读不动写进了未决，原有「未形成税费/门禁核对」与 `AT-CC-282` 仍答未形成，而 `verify_release_gate.go` 把读不动作为 `UNDECIDED` 的原因。决定二因此扩到税费付款核对与放行门禁核对两只编排。

## Decision

**一、提交执行时就绪没登，答未决并指名未配置。** ADR-0227 决定八的原则——就绪与授权两本册的内容是租户取值，没登是租户还没配，续办是去登记，与「门禁明确不成立」的续办不同——同样适用于提交执行口上单元的就绪判断。UC-CC-005 新立 `AT-CC-448`：提交执行时单元没有就绪判断 → 未决并指名就绪未配置，不落单元、版本或尝试。它替代 `AT-CC-109` 在「不存在」一格的结果名；已失效照旧是门禁阻断那一类，答 `NOT_READY`。

**二、读不动与写不进答未决并指名那一口。** UC-CC-004「未形成授权结果」与 UC-CC-005「未形成提交处理结果」在两口首发以`未决`作答，原因指名读不动或写不进的那一口（码见注记），HTTP 200 带原因——与建案口（ADR-0227 决定二）、外部结果接收口同一写法。「未决」由此在三口都有两类原因：某本册的内容租户还没登（`*_UNCONFIGURED`，续办是去登记），与某个依赖此刻读不动或写不进（`*_UNAVAILABLE`，续办是运维去救、请求方原样重试）。结果码相同、原因分格，是 ADR-0029「按恢复动作分格」在答复体里的落法：恢复动作写在原因上，不再另立一个结果名。编排交回 error（没有形成任何答案）仍是 500。

同一写法适用于 UC-CC-009 的税费付款核对与放行门禁核对两只编排：「未形成税费/门禁核对」首发答 `UNDECIDED`，原因指名读不动或写不进的那一口（门禁核对 `GATE_CONDITION_VIEW_UNAVAILABLE`、`DUTY_PAYMENT_GATE_RULE_VIEW_UNAVAILABLE`、`DUTY_VERIFICATION_VIEW_UNAVAILABLE`、`GATE_STORE_UNAVAILABLE`；付款核对 `COLLABORATION_STORE_UNAVAILABLE`、`FUNDS_FACT_REGISTER_UNAVAILABLE`、`PAYER_REQUIREMENT_VIEW_UNAVAILABLE`、`DUTY_VERIFICATION_STORE_UNAVAILABLE`）；`DUTY_VERIFICATION_HANDOFF_UNAVAILABLE` 只由资金事实新版本的重派编排给出——新核对版本已形成、交结算意图时 Outbox 不可用，整笔回滚、消费门重投。人重核（`VerifyPayment`）与门禁核对的交接失败不算未形成：照答已形成（`DUTY_VERIFICATION_FORMED` / `EXISTING_DUTY_VERIFICATION`、`RECORDED` / `EXISTING_RESULT`），带续办引用重发同一份。同一结果码下的其余原因是「放行门禁核对未决」或付款核对未决——目录或规则没登、没有付款核对、三态待确认或冲突、等税费结果或来源补付款人。

本记录的写法不延伸到动作准入：准入按 [ADR-0206](./0206-customs-action-admission-judgement-folds-three-sources-and-judges-cross-zone-by-node-customs-zones.md) 决定五保留「读不动」这一独立答复（判断不形成、不折进任何一格）。准入与门禁核对共用 `assessReleaseGate` 这段读法，编排不同、答复各归各格。

**三、UC 同一场景只留一套结果。** 被首发答法替代的格从原 AT 行移出，原行只留仍成立的格，替代那一格由 4xx 号 AT 独自承担；原行不再行旁注明「首发答法见……」。UC-CC-001、UC-CC-004、UC-CC-005、UC-CC-009 的结果表各补一张首发结果码对照：结果码、对应哪一格结果、何时答；首发未实现的格写明未实现，不写成已有结果码。

## 对已接受记录的作用

部分停用 [ADR-0227](./0227-customs-case-and-declaration-submission-enter-production-through-operator-decision-faces.md) 决定八两句。其一停用：「两行的正文不改，行旁注明首发答法见 `AT-CC-444` / `AT-CC-439` 与本条。」——两行改为只留仍成立的格。其二扩成：同一决定里的「这替代 UC-CC-004 `AT-CC-105`「请求未受理（授权前提不成立）」在「不存在」「已经失效」两格、UC-CC-005 `AT-CC-110`「提交前门禁已阻断」在「不存在」一格上的结果名（110 的已撤销照旧是门禁阻断那一类，答 `NOT_AUTHORIZED`）」一句扩成也替代 UC-CC-005 `AT-CC-109`「提交前门禁已阻断」在「不存在」一格上的结果名（决定一）。**适用场景**：只限 UC 文档的写法与替代范围；决定八的答法（未登记答未决并指名未配置、已失效答业务负向）与 ADR-0227 其余各条不变。

## 候选与反方

**甲 · 照决定八旧做法，在 `AT-CC-109` 行旁再加一条「首发答法见……」。** 反方：同一场景两套结果，照行写测试的人断言门禁阻断，与生产答复相反；审计 H1 那一格就是这样漏的——括注靠人记得补，补漏了没有任何信号。否决。

**乙 · 「读不动」另立结果名（沿用 UC 的「未形成……结果」），不并进未决。** 反方：各编排已经这样答、HTTP 契约已经这样写、建案口已由 ADR-0227 决定二定了同一写法；为文档另立一个结果名就得改各口代码与答复契约，换来的只是把原因字段里已有的区别再说一遍。否决。

**丙 · 把替代的格留在原行、删掉 4xx 号 AT。** 反方：4xx 号 AT 是首发答法的测试钉子（`.scratch/at-coverage-inventory.md` 已登），原行还带着首发未实现的长期格（目标绑定、授权已被另一逻辑提交使用等）；合并会把已钉住的与未落地的混在一行里。否决。

## Consequences

- UC-CC-001：结果表补「包裹未接受」「建案要求未配置」两格与首发结果码对照，`AT-CC-016` 写首发答法。UC-CC-004：结果表补「不再就绪」一格与对照，`AT-CC-103`、`AT-CC-105` 去掉被 `AT-CC-444` 替代的格。UC-CC-005：新立 `AT-CC-448`，`AT-CC-109`、`AT-CC-110` 去掉被 `AT-CC-448`、`AT-CC-439` 替代的格，结果表补对照，`AT-CC-107`、`AT-CC-125`、`AT-CC-126`、`AT-CC-127` 写明首发「已登记待发送」只形成一半（ADR-0227 决定四）。
- UC-CC-009：结果表「放行门禁核对未决」只留判不了与未配置，「未形成税费/门禁核对」与 `AT-CC-282` 写首发答 `UNDECIDED` 并指名那一口，补首发结果码对照。
- AT 盘点（`.scratch/at-coverage-inventory.md`）登 `AT-CC-448`。
- 不改代码。

## 越权风险点

1. **「未决」一个结果码装两类续办**（CC owner）。调用方只看结果码会把「去登记」与「等运维」读成一件事；答复体里的原因是唯一的分法。管理台与集成方须按原因分流，这一点写进了各 UC 的首发结果码对照，没有别的护栏。
2. **持久化写不进也答未决**（CC owner）。UC 原意「未形成」强调本次没有形成业务结果；「未决 + 写不进」同样什么都没留下，各口所依的代码不同：建案、授权确认与提交执行在写不进时整笔回滚（ADR-0227 越权风险点 9）；门禁核对 `VerifyReleaseGateHandler.Handle` 唯一的写是 `Store.Save`，写不进即答未决、此前只有读；人重核 `VerifyPayment` 唯一的写是 `SaveVerification`，同此；资金事实新版本的重派与版本行、交接意图同在消费侧适配器的一笔事务里（`rederive_duty_verifications.go` 包注释「版本行、新核对版本、交接意图三者同生同灭」），答未决即整笔回滚。若将来某一口在部分落册后停下（例如单元已落、版本未落），这一写法就要重核。
3. **长期格保留在原行**（CC owner）。`AT-CC-105`、`109`、`110` 仍带首发未实现的格（就绪与授权目标不一致、授权已被另一逻辑提交使用等），它们的结果名是长期契约，首发没有代码答它们；对照表写明「首发未实现」。

## Links

- [ADR-0227](./0227-customs-case-and-declaration-submission-enter-production-through-operator-decision-faces.md)：本记录部分停用其决定八两句（一停一扩）（见「对已接受记录的作用」）；决定二（建案未决分格）、决定四（「已登记待发送」只形成一半）、越权风险点 9（未决整笔回滚）
- [ADR-0029](./0029-retrieval-failure-algebra-is-partitioned-by-recovery-action.md)：按恢复动作分格
- [ADR-0022](./0022-http-status-carries-answer-formed-not-business-verdict.md)：HTTP 状态只说答案形成没有
- [UC-CC-001](../application/customs-compliance/UC-CC-001-ESTABLISH-CUSTOMS-CASE.md)、[UC-CC-004](../application/customs-compliance/UC-CC-004-AUTHORIZE-DECLARATION-SUBMISSION.md)、[UC-CC-005](../application/customs-compliance/UC-CC-005-FREEZE-SUBMISSION-AND-INITIATE-ATTEMPT.md)、[UC-CC-009](../application/customs-compliance/UC-CC-009-RECONCILE-TAX-PAYMENT-RECOVERY-AND-RELEASE-GATE.md)
- [ADR-0137](./0137-customs-gate-judgments-are-registered-facts-driven-by-assessment-requests-payment-gate-rule-is-registered-and-funds-facts-are-minted-only-in-settlement-accounting.md)：「税费付款」那一道的规则未登记答规则未配置（决定三），门禁核对未决原因的来处
- [ADR-0206](./0206-customs-action-admission-judgement-folds-three-sources-and-judges-cross-zone-by-node-customs-zones.md)：决定五，动作准入保留「读不动」独立答复，本记录的写法不延伸到准入
- 审计 [cc.md](../../.scratch/docs-consistency/audit/cc.md) H1、M1、M2、M3、M4；票 [docs-consistency/03](../../.scratch/docs-consistency/issues/03-cc-docs.md)
