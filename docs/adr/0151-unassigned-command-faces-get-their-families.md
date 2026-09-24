# ADR-0151：未归族的命令口定族——委托侧五个运营决定口与 TF 四个管理台写面归操作者渠道新增的「运营决定」能力面，主链事实的更正口随被更正事实的族，发布词表读口随商业发布那一族

Status: Accepted（2026-09-24，用户经 IDP 队列答通道 4 的立票提议「同意你的决定，开干，开干前，全面审查，确保确实如此」；通道 4 起草，2026-09-25 通道 2 按「全面审查」逐条对代码与文档复核后改定，复核记录见 [operator-channel/15](../../.scratch/operator-channel/issues/15-operation-decision-faces-take-operator-intake.md) Comments。承 [operator-channel/08](../../.scratch/operator-channel/issues/08-isolated-release-of-main-chain-command-faces.md) 逐口归类表里「未归」「同族未列」「族归属待定」三行，以及未入那张表的发布词表读口。裁决能力边界：读过 [ADR-0100](./0100-operator-identity-is-a-product-owned-access-channel-family.md)、[ADR-0149](./0149-business-command-faces-split-into-frontline-operator-and-integration-client-families.md)、[ADR-0150](./0150-synthetic-tenant-is-treated-as-a-real-tenant-and-isolated-form-retires-per-face.md) 全文，[ADR-0055](./0055-business-endpoint-intake-has-an-unconfigured-grade.md) 决定五，[ADR-0132](./0132-authorized-disposition-decides-the-destination-of-a-restricted-request-as-its-own-wait-state-and-never-passes.md) 决定一；PS CONTEXT 受控关闭的定义，PC CONTEXT 合同委派与授权动作两段；委托侧五口的 HTTP 适配器契约、应用命令（`FormControlledClosureCommand`、`FormReopeningCommand`、`CompleteManualReviewCommand`、`RejectShipmentRequestCommand`、`DisposeShipmentRequestCommand`）与 `cmd/parcel-api` 复核装配的注释；TF 四个管理台写面的 Intake 契约与票 tf-segment-lifecycle-closure/07 的定性表；票 operator-channel 03 / 05 / 06 / 08 / 10 / 13 / 14 与 product-strategy-boundary/07。没读：ADR-0126、相关 `UC-*` 与 TF CONTEXT 全文。拿不准的列在文末「越权风险点」。）；**部分停用** ADR-0149 决定一列举里「派送任务」「段关闭」两项对管理台两口的适用，见决定六
Date: 2026-09-24

## Context

**端点表里挂字面量 `UnconfiguredIntake{}` 的写行，大都有了去处。** 登记册配置写面与目录查阅面归操作者渠道（ADR-0100 决定四，实施票 operator-channel/04、05）；商业发布批准链与计价回放归 06；一线作业事实归操作者渠道的「作业事实登记」能力面，外部结果与资金事实归集成客户端族（ADR-0149，实施票 10、11）；客户侧的撤回、取消、补充、资料修订、索赔与客户追踪视图归首方客户渠道（ADR-0139，Proposed，接受与否归用户）。operator-channel/08 的逐口归类表把其余的照实记成了三行没有去处：

- 未归：委托侧运营决定五口——`/shipment-requests/manual-review-completions`、`/shipment-requests/rejections`、`/shipment-requests/authorized-dispositions`、`/shipment-requests/continued-attempt-closures`、`/shipment-requests/continued-attempt-reopenings`。
- 族归属待定：TF 管理台写面里的装载分配与终止参与——`/transport-fulfillment-load-assignment-registrations`、`/transport-fulfillment-participation-terminations`。
- 同族未列：主链事实的更正口——`/transport-fulfillment/handover-corrections`、`/transport-fulfillment/offsite-pickup-corrections`、`/transport-fulfillment/delivery-proof-corrections`、`/settlement-external-funds-fact-correction-registrations`。

另有 `/commercial-publication-vocabularies` 不在那张表里：它是发布表单的词表读口，不要租户、不读任何存储读面。

**同一组 TF 管理台写面被拆在了两处。** 装载分配与终止参与的两个兄弟——关段（`/transport-fulfillment-segment-closures`）与手工建派送任务（`/transport-fulfillment-dispatch-task-registrations`）——被 ADR-0149 决定一列进了「作业事实登记」（列举里的「派送任务」「段关闭」），隔离形态也按那一族放行了（08）。可这四口是同一组：票 tf-segment-lifecycle-closure/07 立它们时逐口写了「为什么是写面不是回传口」——关段是「『不再接受新对象』是运营决定，由人做」，建派送任务是 UC-TF-006「授权角色建立派送任务」，装载分配是「分配本体在消耗之前、由人定」，终止是「指向本上下文之外的处置决定，依据只能由调用方给」；端点表据此给四口取读面册名前缀，与「接入方回传事实」的控制事实那组分开。作业事实登记能力面要求设备在册、事实身份与发生时间由设备签发（ADR-0149 决定二、ADR-0023），套在管理台的决定上，就是要调度员先登记一台设备才能建派送任务。

**这些口没有去处，不是在等外部。** 运营决定由运营企业自己的人在管理台作出，更正由原事实的提交方提交。按 AGENTS.md 与 ADR-0146 的三分，渠道形态归产品，「谁被授予、谁担任审批人」才是租户取值；ADR-0100 决定一也已把 `PAR-INT-01` 收回到客户生产委托接入渠道。代码注释里它们仍被写成「属 `PAR-INT-01`（实例半边）」，那是 ADR-0100 之前的口径。

**身份格从哪里来，现有契约已经写明，且各口不同。** 委托侧五口的命令都带委托的来源身份（租户、客户账户、来源、来源请求键），各口适配器契约都禁止采信自报：

- 主动拒绝与授权处置两口的适配器写明「Intake 只交出『谁在请求』」，复核装配写明「谁在复核」是接入身份、「谁有权复核」是商业授权，两问分源：复核人、决定人、处置人就是认证出来的那个人；他有没有权作这项决定，由编排去问 party-commercial，授权先于一切写动作（ADR-0132）。
- 受控关闭与重开的适配器写的是登录操作人「只作操作证据、不进任何一格」（PS CONTEXT 受控关闭定义：「登录操作人可以作为操作证据，但不能替代实际决定方和授权角色」）；决定方、授权角色与授权依据快照不在命令里，由授权答复给出。
- TF 四口的命令今天只从信封收租户，不落操作者。

**ADR-0055 决定五的两项对这些口还没解开。** 决定五说载荷规范化摘要与准入范围装配「仍然拦着真渠道 Intake……真渠道落地时它们必须先行或同批」。登记册配置写面由 ADR-0100 以「登记快照自带规范化版本与摘要」「登记一个版本不形成任何生产事实」两条豁免；作业事实与外部结果由 ADR-0149 决定四按口解开；运营决定口两边都没有落。

## 候选与反方

**甲 · 运营决定并进 ADR-0100 登记写面那一族（operator-channel/04）。** 反方：登记写面的授予是「登记册配置写」，运营决定改变的是某一份委托或某一段履约的去向，授予维度不同（按决定种类，不按登记册）；并进去会让一个能配价卡的操作者自动能拒单。否决。

**乙 · 运营决定归「作业事实登记」能力面（operator-channel/10）。** 反方：作业事实是一线人员带设备登记已发生的事实，要求设备在册；运营决定是管理台上的判断，没有设备，也不是事实来源。否决。

**丙 · 维持 ADR-0149 对关段与建派送任务的归类，只把装载分配与终止参与另归新能力面。** 反方：同一组被拆进两族，否决乙的理由一字不差地适用于关段与建派送任务；而且调度员要先登记设备才能建派送任务。否决。

**丁 · 运营决定口等首方客户渠道（ADR-0139）一起定。** 反方：它们的操作者不是货主客户，与客户渠道无关；挂在 ADR-0139 上就是把产品自己的后台重新挂回客户的渠道证据——ADR-0100 否决过的同一个范畴错误。否决。

**戊 · 操作者渠道新增「运营决定」能力面，TF 四口整组归入；身份格照现有契约逐口取；ADR-0055 决定五两项沿 ADR-0149 决定四解；更正口随被更正事实的族；词表读口随商业发布。** 即本记录。

## Decision

**一、操作者渠道新增能力面「运营决定」，覆盖委托侧五口与 TF 四个管理台写面。** 操作者册的授予多一个能力面，授予按租户 × 决定种类登记：复核完成、主动拒绝、授权处置、受控关闭、受控重开、关段、建派送任务、装载分配、终止参与。上列各口在装配点换成操作者 Intake，答复照 ADR-0100 决定四的三格（发行方未配置 / 令牌无效 / 无此决定种类的授予），外加决定三的「不在准入范围」一格。有哪些决定种类、每种决定要什么授权条件，是产品的角色模型（产品策略，票 product-strategy-boundary/07）；某个租户把哪位操作者授予哪种决定，是租户取值。

**二、身份格只从信封与授权答复来，逐口照现有契约取，不从请求体收。**

- 租户与提交操作者只从信封来（ADR-0100 决定二「不采信自报」），请求体里带即 `400`（封闭载荷）。
- 复核完成的复核人、主动拒绝的决定人、授权处置的处置人，取信封里的提交操作者；他有没有权作这项决定，仍由编排问 party-commercial，不在 Intake 判。
- 受控关闭与重开：提交操作者只作操作证据，不进命令的任何一格；决定方、授权角色与授权依据快照由授权答复给出。请求方（如有）与重开时的货主账户同样不采信自报；它们在操作者渠道上怎么形成，归 operator-channel/15 与 PS owner 对齐，定之前 Intake 不收这两格，需要它们的决定由编排与领域如实拒，不以提交操作者或请求体自报顶替。
- 委托寻址：委托侧五口的命令都带委托的来源身份，信封里只有租户与提交操作者。Intake 按信封的租户与请求指名的委托（或包裹）在服务端查出来源身份，不从请求体收客户账户、来源或来源请求键；查不到的答复与越权探针同答（ADR-0029）。
- TF 四口：信封给租户，其余照各口既有载荷。

**三、ADR-0055 决定五两项对运营决定口照样适用，解法沿 ADR-0149 决定四。**

- 准入范围：运营决定改变的是生产对象的去向，本身就是生产事实——ADR-0100 以「登记一个版本不形成任何生产事实」豁免登记写面的理由对它不成立。铸信封前读生产权威区间，不覆盖答「不在准入范围」（`403`，operator-channel/14）。
- 载荷规范化：逐口核。领域已自带同身份重放与内容冲突代数的口只核对、不另造摘要——与 ADR-0100 豁免登记写面、operator-channel/13 对已有形状之口「只核对不重做」同一判据；没有的补一版带形状版本的规范化形状。未核完的口不开真渠道。

**四、主链事实的更正口随被更正事实的族。** 交接更正、揽收更正、POD 更正随原事实归「作业事实登记」能力面（ADR-0149，operator-channel/10）；外部资金事实更正随原事实归集成客户端族（operator-channel/11）。更正与首登的提交方是同一方，认证与授予也应是同一份；分开定族，就会出现「能登不能改」或「能改不能登」。

**五、发布词表读口随商业发布那一族。** `/commercial-publication-vocabularies` 与商业发布批准链一起换操作者 Intake（operator-channel/06）：它唯一的消费者是发布四口喂的表单，不要租户、不读存储读面，操作者渠道的认证即可。

**六、部分停用 ADR-0149 决定一列举里「派送任务」「段关闭」两项对管理台两口的适用。** 被停的是这两项把 `/transport-fulfillment-dispatch-task-registrations`（手工建派送任务）与 `/transport-fulfillment-segment-closures`（关段）归进「作业事实登记」；两口归本记录决定一。**适用场景**：只限这两口。派送发起口 `/transport-fulfillment-delivery-dispatch-triggers`（内部触发执行器的按拍入口，与手工建任务是两件事）与 ADR-0149 其余各条不变。

**七、不裁的。** 客户侧各口仍归 ADR-0139，接受与否归用户。隔离形态照 ADR-0150：关段与建派送任务今天经写开关放行，换上运营决定能力面的同一笔撤下；其余各口今天没有隔离放行，本记录不给它们加。授权那一段也不在本记录：委托侧五口越过 Intake 之后，今天在生产装配上各停在自己的授权缺口——复核完成、主动拒绝、受控关闭与重开的授权请求映射是 nil（票 product-strategy-boundary/07 第 1 项），授权处置在 party-commercial 的授权动作词表里还没有一格（ADR-0132 越权风险点 2）；各口如实答各自的未决或未配置格，不代拟坐标。

## Consequences

- [operator-channel/15](../../.scratch/operator-channel/issues/15-operation-decision-faces-take-operator-intake.md)：「运营决定」能力面与上列各口换操作者 Intake，Blocked by 03、14。
- [operator-channel/10](../../.scratch/operator-channel/issues/10-device-register-and-operation-fact-capability-face.md) 多更正三口、少关段与建派送任务两口；11 多外部资金事实更正口；06 多词表读口。
- 端点表与各上下文 Intake 注释里「运营侧认证属 `PAR-INT-01` 待提供」一类过期口径，改为指向各自的渠道族（ADR-0100、ADR-0149、本记录）；客户侧照旧写 `PAR-INT-01`，直到 ADR-0139 有了结论。清扫记在 [operator-channel/16](../../.scratch/operator-channel/issues/16-stale-par-int-01-comments-on-operator-faces.md)。
- ADR-0149 的 Status 行与 Links 按 [adr/README](./README.md) 的部分停用机制加前向指针；adr/README 增本记录一行，0149 一行补部分停用；operator-channel/08 票面追加一条 Comment；psb/15 子票表增 15、16 两行。

## 越权风险点

1. **「运营决定」一个能力面还是逐种决定各一个。** 本记录取一个能力面、授予按决定种类登记；若 owner 认为某种决定（如授权处置）要单独的能力面或额外的授权条件，另裁。
2. **TF 四口判为运营决定。** 依据是票 tf-segment-lifecycle-closure/07 的定性与端点表的分组；若实际作业里某一口由一线人员带设备提交（如司机在车载端关段），那一口应回到作业事实登记能力面，归 TF owner。
3. **更正的提交方不总是原提交方。** 运营人员在管理台更正一线登记的事实时手里没有设备，而作业事实登记能力面要求设备在册；是否对更正口另开授予或免设备，归 TF owner。
4. **委托寻址与请求方的形成。** Intake 按什么查委托、受控关闭与重开的请求方与货主账户在操作者渠道上怎么形成，归 operator-channel/15 与 PS / PC owner；本记录只守「身份格不从请求体收」「提交操作者不顶替实际决定方」两条。
5. **TF 决定要不要记提交操作者。** 四口的命令今天不落操作者；要不要把它作为操作证据落进 TF 的档，归 TF owner。
6. **词表读口也可归目录查阅面（05）。** 本记录跟随使用它的发布表单归 06；两者认证相同，归哪张票只影响排期。
7. **ADR-0149 决定一列举里还有几项的提交者既不是带设备的一线人员，也不是管理台上的决定。** 派送发起的调用方是调度拍，有效时间判断的时间由事实所有者给出；本记录只停用关段与建派送任务两项，这几项不裁。

## Links

- [ADR-0100：管理台运营操作者身份是产品自有的接入渠道族](./0100-operator-identity-is-a-product-owned-access-channel-family.md)：操作者渠道、三格答复与「不采信自报」的出处，本记录在其上加一个能力面
- [ADR-0149：主链业务命令面按提交者分两族](./0149-business-command-faces-split-into-frontline-operator-and-integration-client-families.md)：作业事实登记能力面与集成客户端族，决定四给出 ADR-0055 决定五两项的按口解法；本记录部分停用其决定一列举里「派送任务」「段关闭」两项对管理台两口的适用
- [ADR-0055：业务端点 Intake 增设「未配置」格](./0055-business-endpoint-intake-has-an-unconfigured-grade.md)：决定五两项未决，本记录决定三裁它们对运营决定口适用
- [ADR-0132：授权处置](./0132-authorized-disposition-decides-the-destination-of-a-restricted-request-as-its-own-wait-state-and-never-passes.md)：主动拒绝与授权处置「授权先于一切写动作、实际决定方留痕」的出处
- [ADR-0150：开发与演示中合成租户按真实租户对待](./0150-synthetic-tenant-is-treated-as-a-real-tenant-and-isolated-form-retires-per-face.md)：隔离形态的过渡规则，决定七沿用
- [ADR-0139（Proposed）](./0139-first-party-customer-api-channel-is-a-product-owned-access-channel-family.md)：客户侧各口的去处，接受与否归用户
- [ADR-0146：产品策略是机制与实例之间的第三类](./0146-product-strategy-is-a-third-class-between-mechanism-and-tenant-values.md)：角色模型归产品策略、人到角色的分派归租户取值的出处
- [operator-channel/08](../../.scratch/operator-channel/issues/08-isolated-release-of-main-chain-command-faces.md)：逐口归类表，本记录收它留下的三行
- [tf-segment-lifecycle-closure/07](../../.scratch/tf-segment-lifecycle-closure/issues/07-admin-write-faces-segment-closure-dispatch-task-load-assignment.md)：TF 四个管理台写面「为什么是写面不是回传口」的逐口定性
- [product-strategy-boundary/07](../../.scratch/product-strategy-boundary/issues/07-pc-authorization-coordinates-and-role-models.md)：授权请求坐标的推导与角色模型，决定一、七所指
