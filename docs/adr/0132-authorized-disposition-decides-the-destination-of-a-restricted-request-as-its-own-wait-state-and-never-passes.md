# ADR-0132：`parcel-shipment` 的「授权处置」是授权角色对按共同通过条件不成立的委托**决定去向**——去向封闭为`拒绝`与`交客户补充`，放行不在集内；它是接受判断任务上独立的等待态`等待授权处置`（不复用`等待人工复核`），由处置命令自身续办、不新增信封；受限项的失败处置与责任引用在形成控制判断那一步经本上下文自己的商业缝读回并保存为采用引用，拒绝决定的依据经采用结果回指，补偿续办不用它

Status: Accepted（2026-09-09，用户 22:2x 经 IDP 队列通道 1 授权代裁，通道 6 以 `parcel-shipment` owner 口径裁票 [sa-preacceptance-policy-view/04](../../.scratch/sa-preacceptance-policy-view/issues/04-authorized-disposition-flow-and-failure-disposition-read.md)「要裁的」四问；只裁不码。裁决能力边界：读过该票全文、票 [03](../../.scratch/sa-preacceptance-policy-view/issues/03-parcel-shipment-expresses-per-item-control-results.md)「裁决」节、[ADR-0115](./0115-pre-acceptance-financial-control-policy-content-is-a-row-per-control-and-no-control-stays-with-the-contract.md) / [ADR-0122](./0122-pre-acceptance-control-executes-the-policy-content-items-and-parcel-shipment-folds-by-the-joint-pass-condition.md) / [ADR-0125](./0125-parcel-shipment-financial-control-result-carries-per-item-results-and-folds-in-the-domain.md) 全文、[ADR-0086](./0086-manual-review-wait-is-a-committed-pause-resumed-by-completion-envelope.md) / [ADR-0106](./0106-customer-supplement-wait-is-a-committed-pause-resumed-by-the-new-submission-version-envelope.md) 全文、`parcel-shipment` `CONTEXT.md` 全文、`party-commercial` `CONTEXT.md`「接受前财务控制策略」词条、UC-PS-001 校验组「接受前财务控制」行与步 8、`AT-PS-034` / `AT-PS-035`、PC `pre_acceptance_financial_control_policy.go` 的 `ControlFailureDisposition` 与 `ControlResponsibilityReference`、PS `judgment_translation.go` 的 `FinancialControlCheckFor`、`acceptance_task.go` 全文（`ResumePath`、`CompleteManualReview`、`AwaitOperatorRegistration`）、`complete_manual_review.go` 与 `reject_shipment_request.go` 的命令 / 结果 / 依赖形状、`ports.go` 里各 `*Authorizer` 端口分立的理由、`judgment_continuation.go` 的未决原因集、PS→SA 适配器的控制请求身份取法；**未读** `acceptance_decision.go` 的 `Decide` 全文、`form_acceptance_decision.go` 的暂停路径全文、`FormNewSubmissionVersionHandler`、HTTP 读面与 admin-web。因此本记录只裁四问；实施形状写在票 04「做法」，由实施者以代码为准。不动 `settlement-accounting`、不改 `party-commercial` 正文形状、不改 ADR-0115 / 0122 / 0125 正文）
Date: 2026-09-09

## Context

策略正文那一侧已经把话说到了门口：ADR-0115 让每项控制成行并带失败处置（`ControlFailureDisposition`，封闭两值 `REJECT` / `AUTHORIZED_DISPOSITION`）与失败或补偿责任引用（`ControlResponsibilityReference`，开放引用），并明写它们「只答委托去向，不拥有拒绝决定」；ADR-0122 决定四让它们不随 `settlement-accounting` 的答复走；ADR-0125 决定五裁定 `parcel-shipment` 要用时经自己的商业缝读、只读绑定到本次委托费用范围的那几行、只在有项受限时读——但没建这条读路，理由是「处置的消费方是 UC-PS-001 说的『进入授权处置』那条流程，本上下文今天没有」。它把这一格记为过渡状态：`RESTRICTED` 一律译确定性不通过 → 拒绝，等于替每份合同按 `REJECT` 处置；owner 2026-09-09 复核认可为票 04 落地前的过渡。

本上下文这一侧今天是这样的（取证锚 `91df9aaa`，只作当时取证）：

- `FinancialControlCheckFor` 对 `RESTRICTED` 一律译`未通过`，续办路径填的 `ResumeByInternalRetry` 从不会被用上（注释自己写着「本组没有`无法判定`那一支」）。
- `ResumePath` 的取值与 CONTEXT 接受判断任务的等待态一一对应：`等待受控补充`、`等待内部续办`、`等待人工复核`、`等待运营登记`；ADR-0029 / ADR-0094 立的判据是**按恢复动作分格**——每一格回答「它的恢复动作是什么」，处置由恢复动作唯一决定。
- `等待人工复核`的前置是「全部所需权威结果已经到齐通过、但适用规则要求的人工复核尚未完成」；完成动作是 `CompleteManualReview`，记下授权引用、复核方、证据，CONTEXT 明写「复核完成本身不形成决定，决定仍由判断任务按适用规则形成」——完成之后接受链由「复核已完成」信封再驱一拍，`Decide` 据全部通过的校验形成**接受**。
- 主动拒绝（`RejectShipmentRequestHandler`）是授权角色在自己的命令事务里直接形成拒绝决定：授权先于一切写动作、结构化原因与证据必填、实际决定方留痕；它与自动接受竞争同一个决定提交边界，不发续办信封（ADR-0086 决定三的保留条款）。
- `ports.go` 把 `ActiveRejectionAuthorizer`、`ManualReviewAuthorizer`、`WithdrawalAuthorizer` 分成不同端口，理由写在端口注释里：「获准复核不等于获准直接拒掉这单业务（PC CONTEXT 把两者定为互不蕴含的两个动作），合成一个端口会让一份只授复核权的规则被读成拒绝权。」
- CONTEXT 里已经有这条流程的名字而没有它的形：「否则只能保持未决或**交由授权角色处置**」；硬句「人工处理不得绕过硬规则或把缺少的权威结果改成通过」一字不动。

票 04 要裁的是四件：「授权处置」是什么、它是不是一种新的等待态、处置怎么读（票 03 已裁只落地）、责任引用落在哪。

## Decision

**一、「授权处置」是授权角色对一份当前提交版本**决定去向**的业务决定，去向封闭两值：`拒绝`与`交客户补充`；「放行」不在集内。** 它只在接受前财务控制按共同通过条件不成立、且**全部**受限控制项在策略正文里登记的失败处置都是 `AUTHORIZED_DISPOSITION` 时发生。两个去向各自是什么：

- `拒绝`——形成授权角色拒绝决定，即 CONTEXT 生命周期「已提交 → 已拒绝」的第二种（「授权角色依据结构化原因和证据决定不承担该服务请求」）。留痕与主动拒绝同一份要求：实际决定方、授权依据、结构化原因、证据、决定时间；原因目录仍属 `PAR-COM-14` 实例半边，本上下文只记引用。
- `交客户补充`——本版本不再判，任务转入`等待受控补充`，续办由「新提交版本已形成」信封驱动（ADR-0106 已建的形状）。客户的补充若越出委托边界（重组成员、换目的范围、换合同责任范围），按 CONTEXT 既有规则「必须形成一份或多份关联新委托并保留原成员去向」——那是受控补充的一种结果，不是第三个去向。

不在集内的，逐条写明理由。`放行`：硬句；且已记录的`业务限制`是 `settlement-accounting` 交回的权威结果，处置从不改写它。`补资金后重判`：它的恢复动作是「资金到位后重跑控制」，而今天没有任何东西承载这条触发——`settlement-accounting` 的资金事实到本上下文没有信封，已记录的控制判断也只在「依据失效时必须重新判断」那条规则下重判；ADR-0094 决定四写死「只把回滚改成提交而不给触发是被禁止的」，一格没有触发就不许开，所以它是**后继**而不是否决——SA→PS 的资金事实缝接通那天，再按恢复动作判它该不该成为一格。`换控制策略 / 换合同`：那是 `party-commercial` 侧的商业变更，它让本次判断的商业依据失效，走 CONTEXT「依据失效时必须重新判断」那条路，不是本上下文的处置动作。

任一受限项登记为 `REJECT` 时**没有**授权处置，接受条件确定不成立、照今天形成拒绝：一行 `REJECT` 已经替合同定了去向，另一项的处置救不回一份「全部通过」之下的委托——这是硬句的保守读法。`settlement-accounting` 今天停在首处`业务限制`（ADR-0122 决定二），所以至多一项受限、这一格今天走不到；但本上下文不把那条假设写进构造期，与 ADR-0125 决定二对多项受限的态度一致。

两个去向都对**本版本**已成立的项按原业务关联请求释放（`OccupationFormed`，ADR-0125 决定四）：选`拒绝`即确定未成立；选`交客户补充`那一刻，这一份提交版本的接受也已确定不成立——它不会再被判，占着客户的资金或额度等一份不会到来的接受没有依据。释放失败按既有补偿续办（`ControlReleasePending`，内部重试），不因去向不同而不同。

**二、`等待授权处置`是接受判断任务上独立的等待态，不复用`等待人工复核`。** 票 04 给的分格判据是「续办方是不是同一个角色、完成动作是不是同一个命令」，两条都答否：

- 续办方：授权处置角色，由 `party-commercial` 的授权规则说谁是（本上下文只保存所采用的授权引用，与复核、主动拒绝同一纪律）。它与复核权、拒绝权互不蕴含——`ports.go` 已经为「只授复核权的规则不得被读成拒绝权」把端口分开，处置权是第三个动作，同一条理由。
- 完成动作：处置命令**选去向**。复核的完成动作是记下「复核已完成」，之后决定由任务按规则形成、**可以是接受**；若把不通过的控制放进`等待人工复核`，复核完成那条路就会把一项`业务限制`推到接受——那正是硬句禁的。前置也相反：复核前置是「全部所需权威结果已经到齐通过」，处置前置是「按共同通过条件不成立」。

它在消费门的处置是**提交入账**，与`等待人工复核`、`等待受控补充`、`等待运营登记`同组（ADR-0094 按恢复动作分格：恢复动作是一次人的处置，本进程重试推不动）；ADR-0086 决定一的保存护栏原样扩用——编排在交回`等待授权处置`之前先把带等待态的聚合 `Save` 落库，保存失败不得交回该原因。续办触发是**处置命令自身**，不新增信封类型：选`拒绝`时决定在命令事务里形成，形照主动拒绝（ADR-0086 决定三写明主动拒绝「在自己的命令事务里直接形成决定，不需要也不得再发续办信封」）；选`交客户补充`时等待态在命令事务里转到`等待受控补充`，之后的触发就是 ADR-0106 已建的「新提交版本已形成」信封。ADR-0094 决定四「触发与格同笔落地」由此满足——触发不是一封新信封，是那道命令。

接受校验的译法随之变：`RESTRICTED` 且全部受限项为 `AUTHORIZED_DISPOSITION` → `无法判定`、续办路径「授权处置」（带最靠前受限项的原因）；`RESTRICTED` 且任一受限项为 `REJECT` → `未通过`，照今天。这里的`无法判定`说的是**接受判断**尚未确定——去向待授权角色决定——不是控制结果不确定：控制结果是确定的`业务限制`，原样保留在采用结果上。

处置记录追加在接受判断任务上、一版至多一次、不覆盖，形照 `ManualReviewCompletion`：去向 × 实际处置方 × 授权引用 × 原因引用 × 证据引用 × 处置时点，三项引用必填——少授权说不出凭什么算数、少处置方无从追责、少证据与「有人点了一下」分不开。换代、撤回、主动拒绝照旧竞争：新提交版本重建任务（ADR-0045）使等待态失效，处置命令撞上换代答`版本已换代`，撞上已成立的决定答`任务已完结`并交回那一个——与 `CompleteManualReview` 的分格同形。读面照 ADR-0086 / ADR-0094 / ADR-0106 各自的队列开一格：`task_waiting_on = AUTHORIZED_DISPOSITION AND state = SUBMITTED` 的行集恰是此刻停等处置的委托集，逐行透出受限项、其失败处置与责任引用、受限原因。

**三、读处置的时机与范围不重裁——票 03 第 2 问与 ADR-0125 决定五已定：经本上下文自己的 PC 消费缝读，只读闭包里已采用结算政策 `Applicability().ChargeScope()` 下 `ItemsFor` 的那几行，只在有项受限时读。本记录只定落地形：在形成控制判断那一步读，读到的处置与责任引用作为采用引用记在受限的控制项结果上。** 不在 `Decide` 时再读一次正文：CONTEXT 要「保存实际采用的规则、输入事实版本和结构化结果」，处置是这份判断当时按哪一版正文形成的一部分——策略换版不改已形成的判断，读面与处置角色看到的也是当时那一版。按受限项的控制种类对行；对不上（该范围下没有这一种类的行）是本上下文与 `settlement-accounting` 读到了不同版本的正文（换版竞争）或坏数据，停在`等待内部续办`，不折成任一去向、不折成 `REJECT`。

**四、责任引用随受限项一起保存为采用引用，拒绝决定的依据经采用结果回指、不复制；补偿续办不用它。** `ControlResponsibilityReference` 答的是「谁承担失败或补偿责任」，不是「谁有权处置」——处置权来自授权规则（决定二）。它在本上下文的落点只有一处：受限控制项结果上、与失败处置同格（决定三）。拒绝决定的依据里带它，是**经接受前财务控制采用结果回指**——那份结果本就是拒绝的依据，再抄一份进决定就是同一事实两处定义。授权处置队列读面透出它，处置角色据以知道这次不通过的责任在谁。补偿（释放失败）续办**不用**它：释放按原业务关联请求向 `settlement-accounting` 发、续办方是系统（内部重试），责任方是谁不改变谁来重试；本上下文也不据它裁任何费用、追偿或通知——那些各归 `settlement-accounting` 与商业侧。

## Consequences

- 票 04 转 ready-for-agent 并落地：PS 域加 `等待授权处置` 一格与处置记录；`FinancialControlCheckFor` 按处置分路；受限控制项结果加失败处置与责任引用两格采用引用（PS 自有封闭集镜像 PC 词汇、不 import，集外报错不吸收，ADR-0025）；PS→PC 适配器新文件读策略正文行；处置命令与授权端口；`undecidedDisposition` 穷举加一格（提交入账）；队列读面；PS 迁移（登记册子表两列 + 等待态投影 CHECK 放宽 + 处置记录）。做法与完成判据见票面。
- **ADR-0125 的过渡状态解除**：`RESTRICTED` 不再一律拒绝；`REJECT` 行仍拒绝，`AUTHORIZED_DISPOSITION` 行进等待态。首发没有租户时接受前控制停在`未配置`（ADR-0054 / ADR-0122），走不到本记录改的任何一格。
- `ResumePath` 多一格，消费门仍只有一个回滚格（`等待内部续办`）——ADR-0106 Consequences 那句仍成立；失败预算不被处置烧穿。
- `parcel-shipment` `CONTEXT.md`：新词条「授权处置」；Rules 里「客户可补充的资料缺口、系统内部查询或重试、规则要求的人工复核、运营企业尚未登记的适用参数必须使用不同原因和续办路径」那句加上第五种缺口与续办方；接受判断任务生命周期加`等待授权处置`一格。UC-PS-001 里对应等待态数目的措辞与校验组「接受前财务控制」行「按策略拒绝或进入授权处置」的落地形，归实施票改口。
- 越权风险点（供 owner 复核，都不阻断票 04 开工）：
  1. `ResumePath` 加格与 CONTEXT / UC-PS-001 / ADR-0094 里以数目指称等待态的措辞——按 ADR-0029 判据加格是对的，字面要跟改的归各自所有者（UC 归实施票，ADR-0094 正文不改）。
  2. 处置`拒绝`由**处置授权**放行、不另问主动拒绝授权——`party-commercial` CONTEXT 把动作定为互不蕴含，「授权处置」要不要成为那边授权动作词汇里的一格、`PAR-COM-14` 实例半边怎么登记，归 PC；本记录只在 PS 开一只独立 `Authorizer` 端口。
  3. 受限控制项结果加两格采用引用，扩了 ADR-0125 决定一的形——ADR-0125 正文不改，以本记录为准；owner 若要另立一个「采用处置」对象而不扩 `ControlItemResult`，改的是决定三那一句。
  4. `交客户补充`即释放本版本已成立项的占用——把「接受确定未成立」读到了处置时刻；owner 若认为应保留占用到新版本形成再判，改的是决定一末段。
  5. `补资金后重判`不入集，按「没有触发不开格」排除；它是真实的运营需要，SA→PS 资金事实缝或「已记录判断失效重判」语义两条路任一落地时回看。
  6. 多项受限一 `REJECT` 一 `AUTHORIZED_DISPOSITION` 时 `REJECT` 优先——硬句的保守读；今天走不到，依赖 SA 停在首处限制。

## Alternatives considered

- **复用`等待人工复核`，处置角色用复核完成或主动拒绝收口。** 否决：复核完成之后决定由任务按规则形成且可以是接受，这条路对一项`业务限制`开着就是「把缺少的权威结果改成通过」；且复核权与处置权互不蕴含，一份只授复核权的规则会被读成处置权。
- **把处置做成第三种决定（接受 / 拒绝 / 交客户）。** 否决：`交客户补充`不是决定——版本仍待判、委托仍`已提交`；CONTEXT 一版只有一个合法决定且不可覆盖，把一个不终局的去向塞进决定边界会让「决定已形成」说不清。
- **封闭集里给「例外接受」留一格。** 否决：硬句。
- **首发就加`等待资金到位`一格。** 否决于今天：没有触发，ADR-0094 决定四禁止只开格不给触发；记为后继。
- **处置命令交出一封「处置已完成」信封驱链。** 否决：`拒绝`在命令事务里就形成决定（主动拒绝的保留条款），`交客户补充`无需推链——触发是已有的新提交版本信封；多一封信封就多一个驱动点，ADR-0086 决定三否决路 A 的理由适用。
- **`Decide` 时读正文取处置，不落采用引用。** 否决：策略换版后同一份已形成的判断答案会变；CONTEXT 要保存实际采用的规则。
- **处置权由主动拒绝授权兼任。** 否决：动作互不蕴含，`ports.go` 已用同一理由分开三只端口。
- **责任引用复制进拒绝决定。** 否决：同一事实两处定义；拒绝的依据本就是那份采用结果，经它回指。
- **本记录同笔落实施。** 否决：派工口径「只裁不码」；实施形状以代码为准写在票面。

## Links

- 票 [sa-preacceptance-policy-view/04](../../.scratch/sa-preacceptance-policy-view/issues/04-authorized-disposition-flow-and-failure-disposition-read.md)：四问——本记录的出处；「裁决」节记结论与做法
- 票 [sa-preacceptance-policy-view/03](../../.scratch/sa-preacceptance-policy-view/issues/03-parcel-shipment-expresses-per-item-control-results.md)：第 2 问裁了读处置的时机与范围——决定三回指的对象
- [ADR-0115](./0115-pre-acceptance-financial-control-policy-content-is-a-row-per-control-and-no-control-stays-with-the-contract.md)：失败处置两值与责任引用的出处，「只答委托去向，不拥有拒绝决定」——决定一、四的依据
- [ADR-0122](./0122-pre-acceptance-control-executes-the-policy-content-items-and-parcel-shipment-folds-by-the-joint-pass-condition.md)：决定二「停在首处业务限制」、决定四「失败处置不随 SA 答复走」——决定一末段与决定三的前提
- [ADR-0125](./0125-parcel-shipment-financial-control-result-carries-per-item-results-and-folds-in-the-domain.md)：逐项结果、`OccupationFormed`、决定五留给本票的读路与过渡状态——本记录解除的那一格
- [ADR-0086](./0086-manual-review-wait-is-a-committed-pause-resumed-by-completion-envelope.md)：入账暂停的保存护栏、主动拒绝不发信封的保留条款——决定二的形状来源
- [ADR-0094](./0094-undecided-retry-is-decided-by-resume-path-with-a-fourth-grade-for-operator-registration.md)：按恢复动作分格、「触发不同笔不许落地」——决定一排除`补资金后重判`与决定二加格的判据
- [ADR-0106](./0106-customer-supplement-wait-is-a-committed-pause-resumed-by-the-new-submission-version-envelope.md)：`等待受控补充`由新提交版本信封驱动——`交客户补充`去向的续办机制
- [ADR-0045](./0045-new-submission-version-keeps-history-and-reestablishes-the-task.md)：新提交版本重建任务——等待态随换代失效的依据
- [ADR-0029](./0029-retrieval-failure-algebra-is-partitioned-by-recovery-action.md)：按恢复动作分格
- [ADR-0025](./0025-cross-context-adapters-live-on-the-consumer-side.md)：消费侧封闭集镜像不 import、集外不吸收
- [parcel-shipment CONTEXT](../domain/parcel-shipment/CONTEXT.md)：「人工处理不得绕过硬规则或把缺少的权威结果改成通过」「否则只能保持未决或交由授权角色处置」「依据失效时必须重新判断」「必须形成一份或多份关联新委托并保留原成员去向」——决定一、二的全部硬依据
- [party-commercial CONTEXT](../domain/party-commercial/CONTEXT.md)：「失败处置只回答委托的去向——按策略拒绝或进入授权处置——并指名失败或补偿责任方，不拥有拒绝决定本身」
- [UC-PS-001](../application/parcel-shipment/UC-PS-001-SUBMIT-SHIPMENT-REQUEST.md)：校验组「接受前财务控制」行、`AT-PS-034` / `AT-PS-035`——本记录落地后要改口的两处
- `internal/parcelshipment/domain/judgment_translation.go`、`domain/acceptance_task.go`、`application/complete_manual_review.go`、`application/reject_shipment_request.go`、`application/judgment_continuation.go`、`ports/ports.go`：Context 取证点与实施落点
- 来源：IDP 队列通道 1 的派工与授权（2026-09-09）
