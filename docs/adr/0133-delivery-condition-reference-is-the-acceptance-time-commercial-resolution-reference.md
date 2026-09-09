# ADR-0133：交付条件引用是该包裹所属委托接受时固定的商业解析回指——按它到 `party-commercial` 持有的闭包取已采用的服务产品版本与客户合同版本读交付条件，不取进段时刻当前有效的版本、不另立条件子版本；对象走到合同的路是 `parcel-shipment` 按（租户，包裹身份）答回指；合同委派不改条件引用指向谁；交付条件本体是服务产品版本声明、客户合同版本只能收紧的商业条件，`party-commercial` 今天没有这一族，没登就是没有

Status: Accepted（2026-09-09，通道 2 按通道 1 派单 task-f5521768「用户授权代裁，PC owner 口径、第 2 问连 PS owner 口径：硬句不改、拿不准的单列越权风险点」裁决。裁决能力边界：读过票 [tf/14](../../.scratch/tf-segment-lifecycle-closure/issues/14-delivery-condition-reference-seam-party-commercial.md) 全文、[ADR-0114](./0114-delivery-dispatch-is-triggered-by-entering-a-declared-delivery-segment-and-pulls-requirements-by-reference.md) 决定三 / 四、[ADR-0080](./0080-commercial-closure-resolves-the-contract-first-and-keys-settlement-by-it.md) 全文、[ADR-0116](./0116-source-data-amendment-is-an-authorized-action-and-contract-delegation-resolves-the-actual-decider.md) 全文、[ADR-0058](./0058-stage-content-owned-by-rule-objects.md) 全文、[ADR-0062](./0062-adopted-stage-owner-from-accepted-resolution.md) 全文、[ADR-0079](./0079-pre-acceptance-control-policy-view-asks-by-commercial-resolution-reference.md) 决定一 / 六 / 九、[ADR-0130](./0130-delivery-place-reference-is-a-shipment-level-composite-reference-anchored-to-a-source-data-version.md) 决定二、[PC CONTEXT](../domain/party-commercial/CONTEXT.md)「商业依据解析」「客户合同版本」「合同委派」「保价条件」「客户服务规则版本」词条与 Rules「委托被接受时固定其适用的…」「接单规则包和接受前财务控制策略的新版本只用于其声明生效范围内的新判断」两句、客户合同版本生命周期三句、[TF CONTEXT](../domain/transport-fulfillment/CONTEXT.md)「派送要求」词条与「安全投放、智能柜…只能在适用服务产品、国家地区和客户合同允许」那句硬句、[UC-TF-006](../application/transport-fulfillment/UC-TF-006-PERFORM-DELIVERY-AND-CAPTURE-POD.md) 输入契约「派送要求」那行、`internal/transportfulfillment/ports/delivery_requirement.go` 的 `DeliveryConditionSource`、`internal/transportfulfillment/domain` 的 `ServiceConditionReference` 头注、`internal/transportfulfillment/application/trigger_delivery_dispatch.go` 的结果格名、`internal/transportfulfillment/adapters/partycommercial/carrier_identity_directory.go` 头注（既有消费侧适配器形状）、`internal/parcelshipment/domain/acceptance_basis.go` 的 `CommercialResolutionID`、`internal/parcelshipment/adapters/partycommercial/adopted_stage_owner.go`（回指换闭包的既有路）。未读 PC 闭包快照的 postgres 实现与 PC 发布用例。）
Date: 2026-09-09

## Context

ADR-0114 决定三把末端派送任务七件里的**条件**定为「交付条件引用（`party-commercial`）」：TF 按 `DeliveryConditionSource.LoadDeliveryConditions(tenant, object) → (conditions, resolution)` 拉，交回的是引用、落进任务的 `ServiceConditionReference`——TF 自己对这一格的注释是「产品、合同与授权处置规则的快照」；哪些交付方式被允许、收件范围怎么算，是 UC-TF-006 步骤 5「按服务/合同规则判断有效交付」那一步拿引用去取的事，不在建任务这一拍展开。决定四把「条件引用指哪一版、对象怎么走到合同」留给票 tf/14 与 PC owner，第 2 问要 PS owner 一起。

两侧今天的形状：PC 没有任何一个叫「交付条件」的对象——服务阶段正文里有责任结果声明（ADR-0058 的终局规则族），客户合同版本有合同委派（ADR-0116），都不是「这份合同允许哪些交付方式」；PC CONTEXT 里最接近的形是「保价条件」（服务产品版本声明、客户合同版本只能收紧）与「客户服务规则版本」（产品或合同采用的服务规则）。PS 侧，委托接受决定上有 `CommercialResolutionID`——接受时固定的商业解析回指，PC 按它持有闭包快照，闭包里已采用的成员含服务产品版本、客户合同版本、接单规则包版本等（ADR-0062 决定一的路：回指 → `LoadResolution` → `AdoptedFor(...)`；ADR-0079 决定六把「回指换闭包 → 闭包取已采用的客户合同 → 合同读声明」定成 SA 那条边的生产路径；ADR-0080 让结算政策按同一闭包解出的合同键入）。TF 手里只有 `CarriedObjectReference`（包裹身份或集运单元）。

用三个场景试过候选：

- **接受后合同发了新版本再派送。** 委托在合同 K/v1 下接受，闭包 C1 固定了 K/v1 与产品 P/v3；随后 K/v2 发布并生效于新接受；包裹凭`已交接`进派送段。条件引用指 K/v1 还是 K/v2？PC CONTEXT 客户合同版本生命周期「合同版本到期或被后续版本替代，不改变已经接受委托所保存的合同依据」、Rules「接单规则包和接受前财务控制策略的新版本只用于其声明生效范围内的新判断，不覆盖既有委托采用的规则」——取 K/v2（候选 b）直接撞这两句；且同一委托的两个包裹若在 K/v2 生效前后分别进段，会拿到不同版本，而它们共享同一份接受基线。所以引用钉在接受时固定的那一版。「钉在哪」有两种写法：裸合同版本串，或接受时的闭包回指——见决定一。
- **合同委派让实际决定方 ≠ 相对方。** K/v1 把「资料修订」委派给持某等级的运营角色。派送到门口时，条件引用该指 K/v1 本身还是「委派后的决定版本」？委派答的是「谁能替谁作出这一决定」（PC CONTEXT「合同委派」词条），它不改这份合同允许哪些交付方式、不改收件范围、不改合同责任——条件是合同版本说的话，委派是合同版本说的另一句话。一线作业端拿引用去 PC 取内容，取到的是 K/v1（连同 P/v3）声明的交付条件，与谁被委派无关。门口若出现需要客户决定的动作（比如允许邻居代收但合同没许），那是一个今天不存在的授权动作，走 ADR-0116 的动作 × 委派那条路，不在条件引用里。
- **合同没登交付条件。** 闭包 C1 采用了 P/v3 与 K/v1，两者都没有交付条件声明。PC 只能答「没有」；TF 按 ADR-0114 决定三把任务留在待形成（`REQUIREMENT_MISSING`），不拿「本人签收」顶替——UC-TF-006 输入契约那行的使用约束「不以通用签名规则替代合同」、ADR-0058 被否替代「未配置时由消费方代拟『有效交付即终局』」，两句都在说同一件事。今天 PC 连这一族的声明都没有，所以今天每一份都是「没有」——那是真话，不是缺陷。

## Decision

**一、交付条件引用是该包裹所属委托接受时固定的商业解析回指（PS 接受决定上的 `CommercialResolutionID`，PC 按它持有闭包），不是裸的合同版本串，不是进段时刻当前有效的版本，也不另立「交付条件子版本」。** 取接受时那一版（候选 a 的方向）的理由在第一个场景；写成回指而不是合同版本串，理由有三：其一，交付条件不只在合同上——TF CONTEXT 硬句「只能在适用服务产品、国家地区和客户合同允许」，产品那一半从合同版本串走不到，从闭包 `AdoptedFor(SERVICE_PRODUCT)` 走得到；其二，TF 自己给 `ServiceConditionReference` 的注释是「产品、合同与授权处置规则的快照」，接受时的闭包就是那份快照，一个回指对得上三样；其三，「对象/版本」两段式串只许 PC 一处拼（ADR-0080 决定七），拆串有损（ADR-0062 Context），TF 持一个不透明回指就不碰这条纪律，形同 ADR-0027「消费方只回指标识，提供方按标识持有」。不取候选 (c)：交付条件今天没有对象，为它另立子版本是在没有正文的地方先造一层版本身份；日后它挂到哪个对象（决定四），回指照样走得到，引用不必换形。

**二、对象走到合同的路归 `parcel-shipment`：按（租户，包裹身份）答商业解析回指；委托、接受基线、接受决定是 PS 内部走到答案的路，不是键。** 这一句与 [ADR-0130](./0130-delivery-place-reference-is-a-shipment-level-composite-reference-anchored-to-a-source-data-version.md) 决定二同一句：「PS 对外一律按（租户，包裹身份）答，不要求消费方持有委托、接受基线或提交版本；委托与接受基线是 PS 内部走到答案的路，不是键。」PS 内部经接受基线成员关系（含身份谱系回到来源包裹）走到委托，取其接受决定上的 `CommercialResolutionID`。答法封闭三格：委托`已接受`且有回指 → 回指；对象不属任何已接受委托的成员集合（含集运单元、不可见对象）→ 按统一不可见结果答「没有」；委托在册而接受决定缺回指 → error（接受流的装配缺陷，判据同 ADR-0062 决定三「闭包在场却未采用规则包 → error」）。PC 那一头按（租户，回指）答，不认识 TF 的词也不认识 PS 的包裹：闭包在场且采用了客户合同版本与服务产品版本、其上有交付条件声明 → 交付条件引用（就是这个回指）；闭包在场但两者都没有交付条件声明 → 「没有交付条件」；闭包不在场 → error（`LoadResolution` `found=false` 对一份已接受委托的回指是提供方缺数据，不是「没登条件」）；闭包在场却未采用客户合同版本 → error（同 ADR-0062 的判据）。**PS 的「没有」与 PC 的「没有」是两格**，各由各的所有者答，TF 适配器里各一行，不并成一句「没有条件」——恢复动作不同（ADR-0029）：前者是对象不在册、没人能登；后者是商业责任方去登一份交付条件。TF 的端口今天只有一格 `RequirementMissing`，两格在 TF 结果上要不要分开是 TF 的事（越权风险点 1）。

**三、合同委派不改交付条件引用指向谁。** 条件引用指接受时固定的闭包，从它读到的是那一版客户合同（连同产品版本）声明的交付条件；委派只回答某一授权动作「谁能替谁决定」，交付条件不是授权动作。一线作业端拿引用去 PC 取内容时取到的是合同版本本身的规则，不因委派而换成任何一方的另一套规则。若交付现场日后长出需要客户决定的动作，那是 `AuthorizedAction` 加一格并走委派（ADR-0116 决定一 / 二的路），与本记录无关。

**四、交付条件本体是服务产品版本声明、客户合同版本只能在其内收紧的商业条件——允许的交付方式集合、收件范围规则引用、合同责任与证据规则引用；`party-commercial` 今天没有这一族。** 形取「保价条件」先例（服务产品版本给产品级条件、合同版本只收紧不放宽），不取 ADR-0058 那一族（阶段内容声明归接单规则包版本）：收寄资格、终局规则、取消授权、修订允许四件说的是接受流程的规则，交付条件说的是客户买到的服务长什么样——与保价、服务规则同族，不与接单规则同族。取值（哪些方式、什么证据）是 `PAR-NET-09` / `PAR-COM-05` / `PAR-COM-06` 实例半边，本记录一个都不拟；声明族的表、读口、发布通道与批文归 PC 地盘的实施票（[pc-gaps/11](../../.scratch/party-commercial-context-gaps/issues/11-delivery-condition-declaration-family-and-resolution-keyed-read-face.md)）。没登就是没有：两层都缺席答「没有交付条件」，不默认「本人签收」也不默认「任何方式」。

## Consequences

- PS `ports` 另立一个按（租户，包裹身份）答商业解析回指的窄读口（[ps-port-remainder/07](../../.scratch/ps-port-remainder/issues/07-commercial-resolution-reference-by-parcel-read-face.md)），与 [ps-port-remainder/06](../../.scratch/ps-port-remainder/issues/06-delivery-place-reference-read-face.md) 共用包裹 → 委托那条内部路，一口一问、不合并；不拓宽既有写口。
- PC 立「交付条件」声明族与按（租户，回指）答「交付条件引用 / 没有 / error」的读口（pc-gaps/11）。读口只答有没有、不交内容；内容读口（一线作业端拿引用取允许集与证据规则）是第二个消费方，届时另立。
- TF 适配器 `internal/transportfulfillment/adapters/partycommercial/` 里与承运主体目录并列的第二只（票 tf/14 做法节），消费两个提供方、两份翻译各自在 TF 侧，不让 PC 去读 PS（票面既有红线）。
- PC CONTEXT Language 加「交付条件」一条、Rules 加一句（随本记录同笔）。TF CONTEXT「派送要求」词条已写「交付方式、收件范围与合同责任的条件引用归 `party-commercial`」，不改。
- 不在本记录内：交付条件声明的字段形状与批文（pc-gaps/11）；`RequirementResolution` 要不要分「对象无合同」与「合同无条件」两格（TF owner）；有效交付判断那一步怎么读内容（UC-TF-006 步骤 5 的实施票）；交付现场的客户决定动作与委派。

## Alternatives considered

- **(b) 进段那一拍当前有效的合同版本。** 否决：撞客户合同版本生命周期「不改变已经接受委托所保存的合同依据」；同一委托的包裹在不同拍拿到不同版。
- **(a) 写成裸合同版本串（`对象/版本`）。** 否决：产品那一半走不到；拆串有损、两段式只许 PC 一处拼；TF 持一份 PC 内部拼法。回指是同一个决定的无损写法。
- **(c) 另立交付条件子版本。** 否决：今天没有正文，先造版本身份是在空处立门；日后正文挂到产品 / 合同版本上，回指本就走得到。
- **条件引用指「委派后的决定版本」。** 否决：委派不改合同说的话，只改谁能替谁决定；把它揉进条件引用会让同一份合同的交付条件按谁在场而变。
- **交付条件挂接单规则包版本（ADR-0058 那一族）。** 否决：那一族是接受流程的规则；交付条件是卖出去的服务形态，与保价条件、客户服务规则同族。记为越权风险点 2 供 owner 复核。
- **建任务时不问 PC，只把回指交给 TF，内容缺席留到有效交付那一步再发现。** 否决：UC-TF-006「派送任务已形成」固定的四件里有「规则」，没有规则的任务是让一线跑一趟注定形不成有效交付的活；ADR-0114 决定三「所有者答没有时任务待形成」在这一拍就该成立。
- **PS 与 PC 的两个「没有」并成一格。** 否决：恢复动作不同（ADR-0029）。
- **闭包不在场答「没有」。** 否决：对一份已接受委托的回指答「没登条件」是拿一次数据缺失冒充一句商业责任方说过的话（ADR-0080 决定四同一判据）。

## 越权风险点

1. **两个「没有」在 TF 结果上怎么落。** TF 端口只有 `RequirementMissing` 一格；适配器两行都译成它，则 `REQUIREMENT_MISSING / DELIVERY_CONDITION` 分不出「对象无合同」与「合同无条件」。要不要在 TF 侧加子原因或第二格是 TF owner 的事；本记录只要求两个提供方各答各的、适配器各译各的。
2. **交付条件归服务产品版本 + 合同收紧，而不归接单规则包版本。** 我按「卖出去的服务形态 vs 接受流程的规则」分族；若 PC owner 认为交付方式允许集该与终局规则同住接单规则包版本（因为有效交付是终局结果的输入之一），改的是 pc-gaps/11 的拥有对象一格与本记录决定四，读口按回指走的路不变。
3. **闭包不在场答 error。** 我按「已接受委托必有闭包」读；生产上闭包至今形不成（ADR-0080 Consequences），机制接上前这一格会对每个对象都 error 而不是「没有」。若 owner 认为在闭包机制接通前该答「未配置」（ADR-0055 那一格），改适配器一行；本记录不改。
4. **PC 读口只答有没有、不交内容。** 一线作业端读内容的口没有立；UC-TF-006 步骤 5 的实施票会需要它。若 owner 认为形成任务那一拍就该把允许集快照进任务（避免第二次读），那与 ADR-0114 决定三「拉引用不拉本体」相悖，我没取。
5. **谱系包裹与集运单元同答「没有」。** 与 ADR-0130 越权风险点 2 同形：完整性问题藏进业务答案。

## Links

- [PC CONTEXT](../domain/party-commercial/CONTEXT.md)：「交付条件」词条与 Rules 那句（本记录的落地处）；「客户合同版本」「合同委派」「保价条件」「商业依据解析」
- [TF CONTEXT](../domain/transport-fulfillment/CONTEXT.md)：「派送要求」词条、交付方式那句硬句
- [ADR-0114](./0114-delivery-dispatch-is-triggered-by-entering-a-declared-delivery-segment-and-pulls-requirements-by-reference.md)：决定三 / 四（本记录回答的那一格）
- [ADR-0130](./0130-delivery-place-reference-is-a-shipment-level-composite-reference-anchored-to-a-source-data-version.md)：决定二（按包裹身份答、委托是内部的路——本记录第二问引用的原句）
- [ADR-0080](./0080-commercial-closure-resolves-the-contract-first-and-keys-settlement-by-it.md)：合同维是结论不是输入；两段式串一处拼；「前提未解析」不冒充「无适用依据」
- [ADR-0062](./0062-adopted-stage-owner-from-accepted-resolution.md)、[ADR-0079](./0079-pre-acceptance-control-policy-view-asks-by-commercial-resolution-reference.md)：回指换闭包再取已采用成员的既有路
- [ADR-0116](./0116-source-data-amendment-is-an-authorized-action-and-contract-delegation-resolves-the-actual-decider.md)：合同委派答什么、不答什么
- [ADR-0058](./0058-stage-content-owned-by-rule-objects.md)：阶段内容声明那一族（本记录说明交付条件为何不归它）
- [ADR-0029](./0029-retrieval-failure-algebra-is-partitioned-by-recovery-action.md)：两个「没有」分格的判据
- 票 [tf/14](../../.scratch/tf-segment-lifecycle-closure/issues/14-delivery-condition-reference-seam-party-commercial.md)（三问出处）、[pc-gaps/11](../../.scratch/party-commercial-context-gaps/issues/11-delivery-condition-declaration-family-and-resolution-keyed-read-face.md)、[ps-port-remainder/07](../../.scratch/ps-port-remainder/issues/07-commercial-resolution-reference-by-parcel-read-face.md)
