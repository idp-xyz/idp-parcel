# PS 组审计（目标 4：文档与代码一致）

只读审计，读的是 worktree `/home/tops/workspace/idp-parcel-audit`（detached @ `6b0074d0`）。范围：`internal/parcelshipment/**`，`cmd/parcel-api` 与 `cmd/parcel-dispatch` 里 PS 的装配；对照 PS CONTEXT、7 份 PS UC 和题面点名的 ADR。口径照 `.scratch/docs-consistency/README.md`「审计口径」。取证方式是读码，没有跑测试；凡结论来自推断的，条目里会注明。

按严重度高 → 低排。

---

## 1. 新提交版本取代旧版本时，旧版本已成立的控制项占用不释放

- **类别**：4（文档与代码一致）
- **位置**：`internal/parcelshipment/application/form_new_submission_version.go` 与 `internal/parcelshipment/domain/submission_supersession.go`。两处都没有释放调用；全 `application` 包里，`ReleasePreAcceptanceControl(` 只在 `reject_shipment_request.go`、`withdraw_shipment_request.go`、`dispose_shipment_request.go` 和 `form_acceptance_decision.go` 的 `releaseIfRejected` 里出现。
- **证据**：
  - PS CONTEXT 词条「控制项结果」：「任一项成立即表示提供方账本上形成了占用；接受确定未成立时必须按原业务关联请求释放，不论接受侧结论如何。」
  - 词条「授权处置」：「两者都按原业务关联释放本版本已成立各项的占用」。ADR-0132 决定一给出的理由是：「选`交客户补充`那一刻，这一份提交版本的接受也已确定不成立——它不会再被判，占着客户的资金或额度等一份不会到来的接受没有依据」。
  - 代码路径（读码推断）：
    - 可达性答 `INSUFFICIENT_EVIDENCE` 时，`domain.ReachabilityCheckFor` 把它记成 `CheckUndetermined` + `ResumeByCustomerSupplement`，而 `AdvanceAcceptanceJudgmentHandler` 仍交 `AcceptanceJudgmentAdvanced`。
    - 于是 `AdvanceAcceptanceChainHandler` 照常进财务控制一步，形成冻结或额度占用。
    - 接着 `Decide` 把任务停在`等待受控补充`。客户补件走 `FormNewSubmissionVersion`，旧版本被取代，但它的 `FinancialControlResult` 没有人发释放。
  - 释放请求 `ports.ControlReleaseRequest` 带 `SubmissionVersion`，按版本认领。新版本的控制不会顶掉旧版本那一笔，旧版本的占用就成了孤儿。只有「授权处置 → 交客户补充」这一条路会释放，「Decide → 等待受控补充 → 新版本」这一条不会，同一个规则只兑现了一半。
- **归类**：C 代码错
- **严重度**：高（客户的预付冻结或账期额度被永久占住，CONTEXT 明令必须释放）
- **建议修法**：立代码票。在新提交版本形成的同一笔事务里（或经续办信封），对被取代版本 `OccupationFormed()` 为真的控制结果按原关联发释放，失败时按 `ControlReleasePending` 续办，形照 `releaseIfRejected`。补一条「可达性资料不足 → 控制已占 → 补件」的回归测试。

## 2. 面单渠道服务包裹的取消不核实际承运商收寄、面单服务终局边界和既有终局

- **类别**：4
- **位置**：`internal/parcelshipment/application/cancel_parcel.go` 的 `CancelParcelHandler.Handle`。它在决定提交边界上只读一样东西：`handler.deps.Adoptions.FindResponsibilityStart`，也就是有效网络收寄的采用链。它不读终局库，也不读承运商首次有效收寄。
- **证据**：
  - PS CONTEXT Rules：「面单渠道服务包裹只允许在实际承运商收寄前且面单服务终局边界尚未成立时取消」；「网络服务发生有效网络收寄后，或面单渠道服务发生实际承运商收寄或其他约定终局边界后，包裹不得回退为“已取消”」。
  - UC-PS-006 决定矩阵「已有终局服务结果｜返回既有终局；新服务需求建立关联委托｜重开原结果或追溯取消」，以及 `AT-PS-086`「包裹已有终局后请求取消｜返回既有终局」。
  - 面单渠道服务包裹没有网络收寄。它的终局来自 `JudgeLabelServiceFinal`，要么首次收寄，要么受控关闭路径，都不经采用链。所以承运商收寄之后、或关闭路径已经形成终局之后，这条编排仍会交 `PARCEL_CANCELLED`。
  - 生产装配里取消授权停在 `AUTHORITY_UNCONFIGURED`（`cmd/parcel-api/assemble_cancellation.go` 头注），暂时挡住了这条路。租户一登记授权，它就会生效。
- **归类**：C 代码错
- **严重度**：高（会形成 CONTEXT 明禁的「承运商收寄后取消 / 终局后取消」业务结果）
- **建议修法**：立代码票。决定提交边界上分两步：
  - 先读当前有效终局（`FinalOutcomeStore.FindCurrentFinal`），有终局就交既有终局。
  - 再按服务形态核边界：网络服务读采用链；面单渠道服务读承运商首次有效收寄，以及合同声明的其他终局边界。

## 3. 客户面命令端点把 `!found` 答成 `500 NO_ANSWER_FORMED`（取消口答 `200 REQUEST_NOT_ACCEPTED`），与 ADR-0022 的 `404` 相反

- **类别**：4
- **位置**：
  - `internal/parcelshipment/adapters/http/withdraw_shipment_request.go` 头注：「本端点把它与其余没形成答案的失败一并回 5xx `NO_ANSWER_FORMED`——不细分」。
  - `amend_customer_source_data.go`、`form_new_submission_version.go` 的同款头注，以及 `application/complete_manual_review.go`、`dispose_shipment_request.go` 的「接 HTTP 时与其余未形成答案一并 5xx」。
  - `application/cancel_parcel.go` 在 `!found` 时交 `CancellationNotAccepted`，即 `200` + `REQUEST_NOT_ACCEPTED`。
- **证据**：
  - ADR-0022 Decision：「**`!found` 一律映射为对应上下文的`统一不可见结果`，用 `404`。** …它是 4xx 而非 5xx：指名一份看不见的对象是调用方的错，让离线客户端无限重试它只会把队列堵死。」
  - ADR-0022 Consequences：「接 HTTP 时由适配器映射为 `404`，不得原样 500。」
  - ADR-0022 的 Status 行只部分停用了 `429` 和装配那两句，`!found` 这一条仍然现行。
  - 代码注释把 `404` 读成了「把统一不可见结果拆开」，读反了。统一不可见要的是「不存在」与「他人的」同答，`404` 本身就是那个同答。
  - 顺带一点：开发主线说的是「七处 `!found` 分支」，ADR-0022 说的是「六处」，两边数的不是同一个东西。按 AGENTS.md「改文档」，这类计数引用本身就该删掉。
- **归类**：C 代码错
- **严重度**：中（这几口今天在生产装配下仍是 `UnconfiguredIntake{}`，首方 Intake 逐口换上之后，离线客户端会对 5xx 无限重投）
- **建议修法**：立代码票。客户面撤回、资料修订、受控补充、逐包裹取消这几口的 `!found` 统一答 `404` 加一个上下文统一不可见码，改掉那几处头注；取消口不再把「查无」与「字段缺」并进同一个 `REQUEST_NOT_ACCEPTED`。操作者族答 `403 OPERATOR_NOT_GRANTED`，若是 ADR-0100 有意的探针同答，就在 ADR-0022 的 Status 行补注它的适用范围。

## 4. UC-PS-001「结果语义契约」缺代码实际交出的四格，其中 `已提交` 正是 `AT-PS-101` 要求的答复

- **类别**：4（兼 2）
- **位置**：`docs/application/parcel-shipment/UC-PS-001-SUBMIT-SHIPMENT-REQUEST.md`「结果语义契约」表，列的是 已接受 / 已拒绝 / 已有结果 / 尚未决定 / 接入冲突 / 输入未受理 / 非本产品生产归属 / 生产归属未决。
- **证据**：
  - `application.SubmitOutcome` 的取值是 `SUBMITTED`、`EXISTING_RESULT`、`INGRESS_CONFLICT`、`INPUT_NOT_ACCEPTED`、`OTHER_PRODUCTION_AUTHORITY`、`OWNERSHIP_UNRESOLVED`、`ADMISSION_PAUSED`、`PRIOR_REQUEST_NOT_FOUND`、`LINK_INELIGIBLE`。
  - 提交口从不交已接受、已拒绝或尚未决定：接受链按 ADR-0081 由信封驱动，决定二写的是「HTTP 答复停在`已提交 / 尚未决定`」，决策简报步骤 7 写的是「返回生产归属结果，或者返回`已提交 / 尚未决定`」。
  - 同一份 UC 的 `AT-PS-101` 却写「答复`已提交`」。表里没有`已提交`、`准入暂停`、`查无原委托`、`关联不适用`四行，后三格在代码里是 `blockedOutcome` 与 `resolvePriorClaim` 有意分开的。
- **归类**：A 文档错
- **严重度**：中（照表实现首方契约或管理台，会漏掉四个 `outcome`）
- **建议修法**：结果表增`已提交`一行（含`尚未决定`由接受链异步形成、查询入口），以及`准入暂停`（与生产归属未决分格，理由同 `OutcomeAdmissionPaused` 头注）、`查无原委托`、`关联不适用`三行。`已接受 / 已拒绝 / 尚未决定`三行注明由「委托已提交」信封驱动的接受链形成，不在提交答复里。

## 5. UC-PS-001 末段仍写「禁止任何端口的 PostgreSQL 适配器、Repository、事务与 Outbox」，与 ADR-0026 和现行代码相反

- **类别**：4
- **位置**：`UC-PS-001-SUBMIT-SHIPMENT-REQUEST.md`「开发拆分建议」末段：「仍然禁止：任何端口的 PostgreSQL 适配器、Repository 实现、事务与 Outbox（属 Bento 持久化闸门），以及把未确认的真实合同、规则、角色、时限或阈值写成取值」。同段「并以确定性内存替身验证」。代码侧同病：`internal/parcelshipment/application/submit_shipment_request.go` 包头注「它不含任何持久化、事务或事件机制，那些仍阻断在 Bento 闸门之后（ADR-0017）」。
- **证据**：
  - ADR-0026 Decision：「**即刻解锁的范围**：针对精确 SemVer 候选编写 PostgreSQL Repository、迁移、事务与 Outbox 实现及其合同测试」。
  - `internal/parcelshipment/adapters/postgres/` 已有全套仓储与 Outbox 交接，`cmd/parcel-api/assemble_submission.go` 生产装配真库。
  - 决策简报「下一步」也写了按 ADR-0026 建 PostgreSQL 16 Repository。
  - 同一句闸门口径还在 AGENTS.md「当前默认切片」（「Bento 持久化闸门仍阻断」），那归横切组。
- **归类**：A 文档错
- **严重度**：中（默认主刀用例告诉作者持久化是红线，与仓内现实相反）
- **建议修法**：末段改为引 ADR-0017、ADR-0026 的现行口径，只保留「不把未确认实例取值写成默认」那一半，删掉「仍然禁止…PostgreSQL…Outbox」与「内存替身」。包头注同笔改成「持久化与事件机制在 adapters，不进本包」。

## 6. 「技术未形成」是不是`未决`：UC-PS-004 / 005 明禁，代码统一交 `UNDECIDED`，UC-PS-001 又把「处理失败」算进`尚未决定`

- **类别**：4（兼 2）
- **位置**：
  - UC-PS-005 元数据「未形成结束｜技术提交未完成；不得伪装成撤回、拒绝或未决」。
  - UC-PS-004 元数据「未形成结束｜并发或技术提交未完成；不得伪装为未决或业务拒绝」。
- **证据**：
  - `WithdrawShipmentRequestHandler` 保存失败时走 `handler.undecided(ctx, command, DecisionNotRecorded, …)`，`outcome` 为 `UNDECIDED`。
  - `FormParcelFinalHandler.commit` 保存失败时交 `ParcelFinalUndecided` + `FinalStoreUnavailable`，`outcome` 为 `FINAL_UNDECIDED`。
  - `AmendCustomerSourceDataHandler` 的 `AmendedRequestNotSaved` 同样落 `UNDECIDED`。
  - 代码是按恢复动作分格：保存失败与依赖不可用都是内部重试，用原因码区分，这与 ADR-0029「按恢复动作分格」一致。
  - UC-PS-001「尚未决定」一行写的是「依赖不可用、并发冲突、处理失败或尚未形成权威判断」，把处理失败算进未决，与 UC-PS-004 / 005 自相矛盾。
- **归类**：B 要裁（三份 UC 口径不一，代码只跟其中一边）
- **严重度**：中
- **建议修法**：立一份小 ADR 代决：「技术未形成」作为`未决`的一个原因（恢复动作同内部续办），而不是独立结果格。然后改 UC-PS-004 / 005 的元数据行与 UC-PS-002 的「技术未形成」行，越权风险点写「客户端只凭 `outcome` 分不出业务未决与本方落库失败，要读 `pendingReason`」。若反向裁，则立代码票，为三个编排加独立的 `NOT_FORMED` 格。

## 7. UC-PS-002 的结果分格与代码 `AmendmentOutcome` 对不上；「待确认业务决策」仍挂着已由 ADR 裁定的两项

- **类别**：4（兼 2）
- **位置**：`UC-PS-002-AMEND-CUSTOMER-SOURCE-DATA.md` 的「结果语义契约」八行、「应用流程」步骤 4「授权不足形成业务拒绝；依赖无法确定形成待复核」、「资料范围与阶段边界」的「未登记时只能形成未决或业务拒绝」、「待确认业务决策」表 `BD-PS-009`、`BD-PS-010` 两行的「当前建议」。
- **证据**：`application.AmendmentOutcome` 的取值是 `RECORDED`、`AWAITING_REVIEW`、`NOT_AUTHORIZED`、`DISALLOWED`、`ALREADY_HANDLED`、`SOURCE_CONFLICT`、`UNDECIDED`，与 UC 的对应关系如下：
  - 授权服务答不出：代码交 `UNDECIDED`（`SourceDataAmendmentAuthorityUnavailable`，内部重试），步骤 4 写的是「待复核」。
  - 允许声明缺格：代码交 `AWAITING_REVIEW`。这照的是 ADR-0120 决定三：「缺格 + `closed = false` → 「未声明」…客户看到的是「等复核」」。UC 写的是「只能形成未决或业务拒绝」。
  - `输入未受理`：代码没有这一格。委托查不到、委托未接受时上抛错误，经 HTTP 落 `500 NO_ANSWER_FORMED`，见第 3 条。
  - `业务拒绝`：代码分成 `NOT_AUTHORIZED` 与 `DISALLOWED` 两格。
  - `已记录并采用`与`已记录但待下游判断`：代码合成 `RECORDED`，由 `adoptionOutcome` 区分；采用判断今天只有 `ADOPTED / AWAITING_REVIEW`，见第 8 条。
  - `BD-PS-009` 已由 ADR-0116 裁定（修订是授权动作，合同委派解析实际决定方），`BD-PS-010` 已由 ADR-0120 裁定（允许声明是规则包版本上的第三族），UC 仍列为「待确认 / 当前建议」。
- **归类**：A 文档错（代码跟随已接受的 ADR-0116 / 0120）
- **严重度**：中
- **建议修法**：
  - 结果表按代码七格重写，每格注明 UC 原名：`NOT_AUTHORIZED` 与 `DISALLOWED` 同属业务拒绝；`AWAITING_REVIEW` 含「允许声明未声明」。
  - 步骤 4 改成「授权服务答不出为未决（内部续办），授权规则未登记为未决（运营登记）」。
  - 阶段节引 ADR-0120 决定三的两种读法。
  - `BD-PS-009 / 010` 两行改为「已决：见 ADR-0116 / ADR-0120」。`输入未受理`一格随第 3 条一起定。

## 8. CONTEXT 里「当前客户资料版本采用判断」的取值在三处写法不一，代码另用了第四个名字

- **类别**：4（兼 1）
- **位置**：`docs/domain/parcel-shipment/CONTEXT.md` 三处：
  - 词条「当前客户资料版本采用判断」：「它可以表示已采用、待补充、冲突或未决」（四值）。
  - 生命周期「客户原始资料版本」：「当前判断可以是已采用、待下游判断、待补充、冲突或未决」（五值）。
  - Rules「收件地点引用按（租户，包裹身份）答」那一条：「`已采用`答带该版本锚的引用，`待复核`不给引用」，用了前两处都没有的`待复核`。
- **证据**：
  - `domain.SourceDataAdoptionOutcome` 只有 `ADOPTED`、`AWAITING_REVIEW` 两值。
  - 头注写「CONTEXT 声明的封闭集合有五项…`待复核`是 UC-PS-002 对 CONTEXT`待补充`那一格的点名叫法」。
  - `domain.DeliveryPlaceOutcome` 的 `UNDETERMINED` 对的就是`待复核`。这个对应关系只记在代码注释里，CONTEXT 自己没有写。
- **归类**：A 文档错
- **严重度**：中（作者照词条实现会少`待下游判断`一格，照收件地点规则又会多找一个不存在的`待复核`）
- **建议修法**：词条与生命周期统一成同一组值，并写明`待复核`是哪一格的名字：要么把`待补充`改名`待复核`，要么在词条里写「重叠范围上的`待补充`称`待复核`」。收件地点那条随之引词条。代码注释同步删掉「点名叫法」这层翻译。

## 9. `已提交`委托的成员重组无路可走：CONTEXT 和 BD-PS-007 要求形成关联新委托，代码刻意不开这个方向

- **类别**：4
- **位置**：
  - PS CONTEXT Rules：「接受决定前…删除、拆分、重组成员，或者改变客户账户、合同责任范围、寄收件关系或目的服务范围时，必须形成一份或多份关联新委托并保留原成员去向」。
  - 生命周期「接受判断任务」：「补充形成同一委托的新提交版本或按成员边界形成关联新委托」。
  - UC-PS-001-BUSINESS-DECISION-BRIEF「BD-PS-007」：「需要删除、拆分或重新组合成员时，必须按委托共享约束形成一份或多份关联新委托，并保留原提交、成员去向、重组原因和关联」。
- **证据**：
  - `internal/parcelshipment/domain/request_link.go` 头注：「刻意没有「已提交」方向：待决委托的普通纠错走同一委托的新提交版本（BD-PS-005），给它开关联方向等于给绕过版本机制留门」。
  - `EstablishPriorRequestLink` 对`已提交`原委托交 `ErrPriorStateIncompatibleWithLink`，提交口据此答 `LINK_INELIGIBLE`。
  - 新提交版本要求成员集合等值（ADR-0045 第 4 条）。
  - 两条门一合，`已提交`委托要重组成员，只剩「先撤回、再以 `WITHDRAWN_RESUBMISSION` 重提」一条路，而 CONTEXT 没有写这条路，也没写重组后原委托怎么处置（停判、撤回，还是另一种终态）。
- **归类**：B 要裁（CONTEXT 要关联新委托，却没定原委托的去向；代码据 BD-PS-005 关掉了方向）
- **严重度**：中
- **建议修法**：立 ADR 代决两件事：
  - `已提交`期间成员重组时原委托的去向，例如以「重组」为因停止判断任务并释放占用，或明定「先撤回后关联」为唯一路径。
  - 是否给 `RequestLinkKind` 增一个「待决重组」方向。

  据裁决改 CONTEXT 生命周期，以及代码与 `request_link.go` 头注。越权风险点写「客户在等待受控补充时多了一条绕开版本机制的路」。

## 10. 外部标识关联的`已失效`不形成：ADR-0184 和代码说「有效期规则没有登记处」，而 ADR-0119 早已立了这一格并接进面单终局判断

- **类别**：4（兼 3）
- **位置**：
  - PS CONTEXT 生命周期「外部标识关联」：「适用 → 已失效：渠道确认或接受时固定的有效期规则届满」。
  - `internal/parcelshipment/domain/external_identifier.go` 中 `ExternalIdentifierChangeKind` 头注：「已失效不在其中：有效期规则今天没有登记处，没有执行器会形成它（ADR-0184 决定七）」。
  - ADR-0184 决定七：「已失效没有执行器：有效期规则今天没有登记处，本记录不造。」
- **证据**：
  - ADR-0119（Accepted，2026-09-07，早于 ADR-0184）的标题就是「面单成功结果的有效期是终局规则声明上的一格…随 `FINAL_RULE` 通道同一份正文登记」。
  - 代码里有 `ports.LabelValidityRuleView`，由 `adapters/partycommercial/label_validity_rule.go` 的 `DeclaredLabelValidityRule` 实现，并经 `JudgeLabelServiceFinalHandler` 喂给 `LabelServiceFinalInput.LapsedTransactions`。
  - 结果是同一件「面单成功结果已不可逆失效」：终局判断认它（不再阻止终局），渠道包裹件号关联却仍是`适用`，身份核对仍把它当候选。
- **归类**：B 要裁（两份已接受 ADR 对同一事实的陈述相抵，CONTEXT 写的是第三种现状，三者都没有停用关系）
- **严重度**：中
- **建议修法**：立 ADR 代决「`LabelValidityRuleView` 判出失效时，是否随之形成渠道包裹件号关联的`已失效`」。若形成，就部分停用 ADR-0184 决定七那一句，并立代码票加 `ExternalIdentifierLapsed`。若不形成，就改 CONTEXT 生命周期那一行，写明今天只有作废与替代两种终态。无论哪种结论，ADR-0184 决定七的事实陈述都要在 Status 行注明已被 ADR-0119 推翻。

## 11. UC-PS-004 `AT-PS-063`「POD 错误致有效交付失效」：代码的重派生只能产出另一个终局，表达不了来源失效

- **类别**：4
- **位置**：UC-PS-004 验收「`AT-PS-063`｜原有效交付因 POD 错误被失效｜保留原终局历史，形成新判断版本并重新派生当前结果」；结果表「当前终局重派生｜来源更正/失效导致当前判断变化」。PS CONTEXT 生命周期「面单继续尝试」也承认「源事实更正或事件有效性变化使终局失效」。
- **证据**：
  - `domain.ResponsibilityOutcomeSpec` 没有有效性格（种类、包裹、决定、执行、版本、时间，六项）。
  - `FormParcelFinalHandler.Handle` 遇「同包裹已有终局、同种来源」一律走 `rederive`，写下 `Finalized: true`。
  - `ParcelFinalOutcome.Rederive` 沿用原 `kind` 与 `ruleVersion`，不重新调用 `FinalRuleView.JudgeFinalOutcome`。
  - 头注声称它就是 `AT-PS-063`（「依据来源更正或有效性变化形成新的当前判断版本（AT-PS-063）」），但「失效后当前不再有终局」这一格在形状上立不起来；更正后的来源也不会被重新按规则判。
- **归类**：C 代码错
- **严重度**：中（POD 被判错时，包裹仍挂着一份终局，委托完成摘要与下游消费都跟着错）
- **建议修法**：立代码票。责任结果引用加有效性格（或「失效」种类），重派生时重新判终局规则，判不成立就形成「终局已失效」的新版本、摘要回退到部分完成，原版本保留。同步修正 `Rederive` 头注对 `AT-PS-063` 的声称。

## 12. ADR-0140 已接受，但首方 Intake 接上后仍没有 `/v1`、问题体仍是自造形，也没有立实施票

- **类别**：4
- **位置**：`cmd/parcel-api/endpoints_submission.go` 的 `"/shipment-requests": {method: http.MethodPost, face: customerClientFace(...)}`；`internal/parcelshipment/adapters/http/submit_shipment_request.go` 的 `problemResponse{Error: problemDetail{Code: …}}`，以 `application/json` 写出。
- **证据**：
  - ADR-0140 Decision 二：「首方渠道 Intake 在装配点接上时，客户面端点的路径模板加前缀 `/v1`」。
  - ADR-0140 Consequences：「首方族就位后，客户面提交口的路径变为 `/v1/shipment-requests`」。
  - ADR-0140 Decision 三：「4xx 与 5xx 一律 `application/problem+json`（RFC 9457）」，成员包括 `type`、`title`、`status`、`code`、`requestId`。
  - 首方 Intake 已经接上：票 `first-party-api/00` 已 resolved，提交口挂 `customerClientFace`。
  - 仓内没有 `tools/api-contract`、端点描述符或 `docs/api/generated/`；全仓 Go 代码零处 `problem+json`。
  - `.scratch/first-party-api/spec.md` 写「ADR-0140 / 0141 / 0142 各自的实施票——各 ADR 接受后按其 Consequences 另立」，ADR 接受（2026-10-19）后至今没有立。
- **归类**：C 代码错（已接受的决定按其自身触发条件早该落地）
- **严重度**：中（首方契约在首个兼容性基线发布前改形状代价为零；拖过基线再改就是破坏性变更）
- **建议修法**：按 ADR-0140 Consequences 立实施票：`/v1` 前缀按族加在装配点，`writeProblem` 收进 `httpapi` 并改成问题详情，加描述符与生成器。同时在 `first-party-api/spec.md` 子票表里登记。

## 13. ADR-0140 的 Status 行自相矛盾：修订要点说 `detail` 随「改请求才会好」的 4xx 在场，索引注记却写「不带 `detail`」

- **类别**：3（兼 4）
- **位置**：`docs/adr/0140-…problem-details.md` 的 Status 行。索引注记那一段写：「问题体换 `application/problem+json`（`type`/`title`/`status`/`code`/`requestId`，不带 `detail`——`UC-PS-001` 泄露约束）」。
- **证据**：
  - 同一 Status 行的修订要点：「`detail` 只随「改请求才会好」的 4xx 在场」。
  - Decision 三：「`detail` 只随「改请求才会好」的 4xx 在场（接受修订时是 `MALFORMED_REQUEST`）」。
  - 代码 `problemDetail.Detail` 在 `MALFORMED_REQUEST` 上带 `detail`（`writeIntakeProblem` 经 `malformedFieldError.detail()`）。
  - 这句注记会进 `docs/adr/README.md` 的索引，实施第 12 条的人最先读到的就是它。
- **归类**：A 文档错（Decision 正文与代码一致，注记是修订前的旧口径）
- **严重度**：中（照注记实施问题详情，会删掉已经落地的 `detail`）
- **建议修法**：索引注记里那一处改为「`detail` 只随改请求才会好的 4xx 在场」，然后重生成 ADR 索引。

## 14. UC-PS-004 / 005 / 006 的状态行仍是「稳定业务骨架待实现」，代码已装配真编排

- **类别**：4
- **位置**：UC-PS-004「状态：稳定业务骨架待实现；真实网络服务终局规则…」；UC-PS-005「状态：稳定业务骨架待实现；真实撤回授权…」；UC-PS-006「状态：稳定业务骨架待实现；真实取消授权…」。
- **证据**：
  - `application.FormParcelFinalHandler` 经 `cmd/parcel-dispatch` 的有效交付、承运商首次收寄路由接真。
  - `WithdrawShipmentRequestHandler` 与 `CancelParcelHandler` 由 `cmd/parcel-api/assemble_withdrawal.go` 和 `assemble_cancellation.go` 装配真库。
  - 对照之下，UC-PS-003 已写「稳定业务骨架已形成」。
- **归类**：A 文档错
- **严重度**：低
- **建议修法**：三份状态行改为「机制半边已实现（逐格见代码），实例半边待…」，并点出各自尚未实现的那几格：UC-PS-004 的身份谱系与来源失效（第 11 条）；UC-PS-006 的收寄后处置决定、面单渠道边界（第 2 条）。

## 15. Go 首个消费者决策简报的包布局与仓内现状不符

- **类别**：4
- **位置**：`docs/design/parcel-go-first-consumer-slice-decision-brief.md` 的布局块，列有 `parcel-migrate/             显式迁移命令`、`parcel-outbox/              Outbox 发布进程`、`internal/platform/` 下的 `bootstrap/`、`postgres/`、`observability/`，以及 `tests/` 下的 `bento_contract/`。
- **证据**：
  - `cmd/` 下没有 `parcel-migrate` 与 `parcel-outbox`。发布与派发是 `cmd/parcel-dispatch`；迁移计划在 `internal/platform/migrate`，由各 `cmd/parcel-*-register` 与 `parcel-api` 引用。
  - `internal/platform/` 下是 `buildinfo`、`dispatch`、`httpapi`、`migrate`、`outboxintent` 等，没有 `bootstrap`、`postgres`、`observability`。
  - 测试目录实名 `tests/bentocontract`；同一份简报的后文也写 `tests/bentocontract`。
  - AGENTS.md「开工顺序」第 5 步要求「技术切片以 Go 首个消费者决策简报与包布局为准」。
- **归类**：A 文档错
- **严重度**：低（`internal/parcelshipment/` 自身的分层与简报一致，错在进程与平台层）
- **建议修法**：布局块按现状改，并写一句「进程入口以 `cmd/` 实际目录为准」；`parcel-migrate`、`parcel-outbox` 两行改成指向 `cmd/parcel-dispatch` 与 `internal/platform/migrate`。

## 16. 首方 Intake 把 `occurredAt` 铸成服务端接收时刻，CONTEXT 定义它是「来源方声明」的时间

- **类别**：4
- **位置**：`internal/parcelshipment/adapters/http/customer_submission_intake.go` 的 `IntakeSubmission`，`OccurredAt:              now,` 与 `ReceivedAt:              now,` 同取 `intake.clock.Now()`。
- **证据**：
  - PS CONTEXT 词条「来源发生时间（`occurredAt`）」：「来源方声明请求或事件发生的时间…用于溯源、迟到和因果判断」。
  - UC-PS-001 输入表「来源信封｜客户可声明：否」。
  - ADR-0139 把「来源信封铸造」判给产品，但没有写首方族的 `occurredAt` 取什么。
  - 结果是首方提交的两类时间恒等，迟到与因果判断在首方族上失去这一维；而 CONTEXT 写的是两者分开保存，且不建立 `occurredAt <= receivedAt` 约束。
- **归类**：B 要裁（「来源方声明」与「客户不可声明」在首方族上相抵，产品契约没有定这一格）
- **严重度**：低
- **建议修法**：在 ADR-0139 的后继或首方契约实施票里定：首方族的 `occurredAt` 是载荷里客户系统声明的一格（进信封、不进摘要），还是明定「首方族不提供来源发生时间、以接收时刻记」。据此改 Intake，或在 CONTEXT 词条补一句首方族的取法。

## 17. 其余各 UC 的结果表缺代码已有的格

- **类别**：4
- **位置与证据**：
  - UC-PS-005「输入与结果语义」与目标节写「区分撤回已成立、已有决定、未决、输入未受理和技术未形成」。代码 `WithdrawalOutcome` 是 `FORMED`、`NOT_AUTHORIZED`、`DECISION_ALREADY_FORMED`、`UNDECIDED`、`SOURCE_CONFLICT`：`未获授权`（`AT-PS-075` 后半）与`来源冲突`（`AT-PS-069`）不在结果表；`输入未受理`在代码里不是结果，是 `400 MALFORMED_REQUEST`。
  - UC-PS-003「结果语义契约」六行里没有`请求未受理`，代码有 `IntakeRequestNotAccepted`（`REQUEST_NOT_ACCEPTED`，也是 `AT-PS-052` 的出口）。
  - UC-PS-004 结果表同样缺代码的 `REQUEST_NOT_ACCEPTED`（`FinalRequestNotAccepted`）。
- **归类**：A 文档错
- **严重度**：低
- **建议修法**：三份 UC 的结果表各补齐对应行，并标出 `outcome` 原名的对照（UC 不定字段名，但要列出这一格的业务含义）。

## 18. ADR-0081 决定二引了一句 UC-PS-001 里不存在的「排除项」

- **类别**：3
- **位置**：`docs/adr/0081-acceptance-judgment-is-envelope-driven.md` 决定二：「提交编排**不做**接受判断——UC-PS-001 的排除项「不执行接受条件判断」维持原样」；Alternatives 也有「UC-PS-001 排除项明写提交不执行接受条件判断」。
- **证据**：全 `docs/` 只有决策简报步骤 7 有这句（「返回生产归属结果，或者返回`已提交 / 尚未决定`及稳定查询关联；不执行接受条件判断。」）。UC-PS-001 的「本用例包含」反而列了「校验客户与商业有效性、服务适用性、必要资料和委托成员基线」。
- **归类**：A 文档错（正文不改；在 Status 行注记里更正出处）
- **严重度**：低（与第 4 条相关：UC-PS-001 至今没写「提交答复不含接受判断」）
- **建议修法**：ADR-0081 的 Status 行加一条「引用坐标更正：该句出处是决策简报步骤 7，不是 UC-PS-001」。UC-PS-001 在第 4 条修订时补写「提交编排不执行接受条件判断，接受链由信封驱动（ADR-0081）」，让那句引用有着落。

---

## 已核无问题的要点

| 要点 | 文档 | 代码 | 结论 |
|---|---|---|---|
| 委托状态集合 | CONTEXT 生命周期：已提交 / 已接受 / 已拒绝 / 已撤回；已取消、已完成由包裹结果派生 | `ShipmentRequestState`：SUBMITTED / ACCEPTED / REJECTED / WITHDRAWN；`DeriveShipmentCompletion`、`DerivesShipmentCancelled` 派生 | 一致（草稿在客户端，不入库） |
| 接受判断任务等待态 | CONTEXT 五个等待态 + 已完成 / 已停止 | `ResumePath` 五值、`AcceptanceTaskState` RUNNING / COMPLETE / STOPPED；`JudgmentPendingReason.resumePath()` 全函数 | 一致；ADR-0094 第四格、ADR-0132 第五格都已落地 |
| 等待受控补充入账 | ADR-0106 部分停用 ADR-0094 Decision 三那一格 | `psinbox.undecided_disposition` 把 `ResumeByCustomerSupplement` 列入提交入账组 | 一致 |
| 授权处置去向 | CONTEXT「拒绝，或交客户补充」，放行不在集内；UC-PS-001 的 `REJECT` / `AUTHORIZED_DISPOSITION` | `AuthorizedDispositionChoice` REJECT / CUSTOMER_SUPPLEMENT；`ControlFailureDisposition` REJECT / AUTHORIZED_DISPOSITION；两去向都释放 | 一致 |
| 释放按「有没有成立项」 | ADR-0125 决定四 | `releaseIfRejected` 等处按 `OccupationFormed()` 发 | 一致（缺口只在第 1 条的取代路径） |
| 来源请求身份四段 + 重放 / 冲突 | CONTEXT「来源请求身份」「PayloadDigest」 | `SourceIdentity` 四段；`ClassifySourceSubmission` 只比摘要；`SubmissionPayloadSpec` 结构上没有 occurredAt / receivedAt 位置，`requestEffectiveAt` 缺失与显式都进摘要 | 一致 |
| 摘要不吃现签内部标识 | ADR-0223 决定二「接单入口每次都现签一组新号」 | 首方 Intake 的摘要用客户参考号，内部标识只进命令 | 一致 |
| 首方提交答复 `parcels` | ADR-0223 决定一、二；`AT-PS-101`–`105` | `submitResponse.Parcels`、`submittedParcelView{parcelId, customerParcelReference}`；`naming` 只在建成 / 重放 / 撞已存在三路；按首个提交版本次序 | 一致 |
| 生产归属四格 | UC-PS-001 步骤 3B、ADR-0128 | `blockedOutcome`：其他权威需交接已确认，否则未决；暂停另成一格 | 一致（UC 结果表缺暂停一格，见第 4 条） |
| 面单渠道终局判断 | CONTEXT 规则「首个面单渠道服务产品默认跨…判断终局」与生命周期四条；`AT-PS-094`–`099` | `JudgeLabelServiceFinal` 六步逐句对应；结果不确定的交易不拦首次收寄 | 一致 |
| 受控关闭 / 重开留痕 | CONTEXT 受控关闭条；ADR-0187（请求证据与请求方同在同缺、操作证据另格） | `ContinuedAttemptDecision` 字段齐全；操作者口 `operatorEvidence` 只进操作证据 | 一致 |
| 歧义信号与范围相容 | CONTEXT 两词条；ADR-0195 / 0212 | `ExternalIdentifierStore` 一条 SQL：候选不因范围收窄，`compatible` 按类型与范围判，歧义只认`已提交`委托、带范围时收到范围内账户 | 一致 |
| 声明撤除门铃 | CONTEXT「由`已提交`转为`已拒绝`或`已撤回`时…交出一次声明撤除」；ADR-0211 | `declarationsRetiredBy` 按前后两态判，接受不响；只带（租户，号码） | 一致 |
| UC-PS-003 结果格 | 结果语义契约六行 | `IntakeAdoptionOutcome` 一一对应（另多 `REQUEST_NOT_ACCEPTED`，见第 17 条） | 一致 |
| UC-PS-004 终局规则未配置即未决 | 「未配置时终局未决」「不得默认有效交付即所有产品终局」 | `FinalRuleUnconfigured` → `FINAL_UNDECIDED` | 一致 |
| 撤回竞争与已有决定 | UC-PS-005 `AT-PS-070`–`072`、`AT-PS-076` | `WithdrawByCustomer` 的 `ErrDecisionAlreadyFormed` → `existing`；`LinkWithdrawnResubmission` | 一致 |
| 代码引用的 `AT-PS-*` | 7 份 UC | 代码里出现的每个 `AT-PS-` 编号都在 UC 中存在 | 一致 |
| PS 端点装配 | ADR-0222 按特性文件自登记；ADR-0139 逐口换首方 Intake | `cmd/parcel-api/endpoints_*.go` 各一族；只有提交口换成首方族，其余客户面口仍是 `UnconfiguredIntake{}`（票 00 写明「其余客户面各口逐口换，另立票」） | 一致（逐口换属在途，不算矛盾） |

范围外的观察只记一句：AGENTS.md「当前默认切片」的「Bento 持久化闸门仍阻断」与第 5 条同病，归横切组。
