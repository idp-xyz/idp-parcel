# ADR-0184：外部标识关联自立一本只追加的登记册，随来源事实同笔形成——客户包裹件号随委托接受、渠道包裹件号随面单交易包裹结果受理；节点按号码逐字核对，只拿回此刻适用关联所标识的包裹

Status: Accepted（2026-10-16，用户原话「这些需要确定的事情……你帮我做决定吧」与「开始」。Claude Code 会话据此代 PS owner 裁定下面七条，落在票 [ps-external-mark-relations/01](../../.scratch/ps-external-mark-relations/issues/01-external-mark-relations-have-no-model-in-parcel-shipment.md) 那一格。裁决能力边界：读过 PS CONTEXT「外部标识」「面单交易包裹结果」的定义、Rules 里外部标识两条与面单交易各条、`委托` 与 `包裹身份与服务` 两节生命周期，GLOSSARY「外部标识」「包裹内部标识」，CONTEXT-MAP `parcel-shipment → node-operations` 一条，UC-NO-002 的流程表、控制与身份一节和验收表；代码读过 `IsolatedSubmissionIntake` 的译装、`SubmitShipmentRequestHandler`、`ShipmentRequests.Save` 与 `FindCurrentAcceptedByParcel`、`LabelTransactionParcelResult`、NO 的 `ParcelIdentityView` 与两处 `unconfiguredParcelIdentityView`。**没读** UC-PS-001 全文与面单交易各编排的全部分支。）

决定一的键已由 [ADR-0185](./0185-external-identifier-relation-identity-includes-its-formation-basis.md) 修订：键加入形成依据。下文原样保留。

Date: 2026-10-16

## Context

PS CONTEXT 把「外部标识关系」列为本上下文所管，定义要求记录标识类型、分配方、被标识对象、适用范围和替代关系，两条不变量要求号码挂在真实标识对象上、作废与替代只改适用关系。代码里一行没有：`internal/parcelshipment` 领域层只有内部的 `DeclaredParcelID`，NO 的身份核对缝在 `cmd/parcel-api` 与 `cmd/parcel-frontline-import` 都接着显式未配置替身。于是客户送站的每一次明确接收都停在`收寄待确认`、不落册——开发主线按 ADR-0146 四项判据重定级时，PN-03 第一项（骨架完整）点名的正是这一格。

取证时量到两件比「没有模型」更具体的事：

- **客户给的包裹号在建单那一刻就丢了。** 隔离形态的提交 Intake 收客户委托参考与逐件 `customerParcelReference`，摘要吃这一组，然后另签内部标识（`SubmissionIdentityFactory` 的注释写死「客户参考号不得变成内部标识」）；`SubmitShipmentRequestCommand` 不带客户包裹号，库里也不存。客户贴在包裹上的号与内部包裹之间的关系，没有任何地方留着。
- **渠道签发的件号有落点却没有生产来源。** `LabelTransactionParcelResult` 带 `ChannelParcelIdentifier`（头注已按「外部标识必须关联其真实标识对象」挂在包裹结果上），但面单渠道写链今天没有运营端点、也没有触发面（`assemble_label_channel.go` 头注），生产与演示里一笔面单交易都没有。

## Decision

**一、外部标识关联自立一本登记册，不并入委托或面单交易聚合。** 一行一条关联，键（租户，标识类型，分配方，号码，被标识对象）。行只插不改；适用状态的变化另起一行挂在原关联上，终态一次——已作废、已被替代、已失效都不回到适用（PS CONTEXT「外部标识关联」生命周期）。不并入委托：渠道包裹件号来自面单交易，一件包裹会因换单先后挂多笔交易的号；不并入面单交易：客户包裹件号与任何交易无关。按号码查要一张跨来源的表。

**二、标识类型封闭，并决定被标识对象的种类与分配方的种类。** 词表照 PS CONTEXT 那条不变量：客户参考号 → 委托，客户包裹件号 → 包裹，渠道包裹件号（含尾程件号）→ 包裹，渠道订单号或主号 → 面单交易。类型与对象种类绑死，「号码挂错对象」在形状上就立不起来。本记录只让有来源的两类进代码：客户包裹件号、渠道包裹件号。另两类等各自的来源与消费方——客户参考号在隔离形态里今天兼作来源请求键（真渠道的推导仍未决），还没有按它查委托的消费方；渠道订单号与主号要等出向连接器把交易级号码带回来，面单交易今天没有那一格（[ps-external-mark-relations/01](../../.scratch/ps-external-mark-relations/issues/01-external-mark-relations-have-no-model-in-parcel-shipment.md) 2026-09-01 那条「两票不要各建一份」，落脚点就是本册）。

**三、关联随它的来源事实同一笔事务形成，不开人工登记口。**

- **客户包裹件号**随委托接受形成，分配方是该委托的客户账户，形成依据是接受决定。写法与 `declared_parcel_ids` 投影列同一先例（ADR-0060）：已接受委托每次经仓储保存时，与快照同一条事务把它的客户包裹件号写进本册，按键幂等。接受前不形成——接受前的声明包裹不是正式包裹（PS CONTEXT Rules 增句）。
- **渠道包裹件号**随面单交易包裹结果受理形成，分配方是该交易的渠道服务方，形成依据是那笔交易。与渠道结果同一笔事务写下。

人工登记、更正与作废口另立票：今天没有一种业务事实要求运营手工改号，先开口就等于先给「覆盖」留门。

**四、客户包裹件号挂在委托上，首次提交时固定。** 提交命令带（声明包裹，客户包裹件号）配对，每件至多一个号，指着成员集合外的包裹即`输入未受理`；号码进快照，重建时带回。新提交版本不重报：新版本的成员集合必须与当前版本相等（`supersessionMembers`），号码标识的是成员，不是版本内容。接受前想改号走撤回再提交。

**五、身份核对：PS 端口按（租户，号码）答此刻适用、被标识对象为包裹的关联所指的包裹，去重后交回。** 号码逐字比对，不做大小写、空白或校验位折算——UC-NO-002 明禁写默认条码规则，折算属租户取值。命中多件即交回多件，NO 的三分支据此答身份冲突，不按形成时间、来源或分配方挑一件。只交回包裹，不交分配方、客户账户或委托（CONTEXT-MAP 增句）：节点要的是「这是哪一件」，客户是谁不是节点岗位该看到的。NO 侧由 `internal/nodeoperations/adapters/parcelshipment` 的防腐适配器把包裹译成 `ParcelAssociationReference`（与 PS 消费节点收寄的适配器同一约定：关联引用的串就是声明包裹标识），两处生产装配换掉 `unconfiguredParcelIdentityView`。

**六、适用范围按分配方编号空间表达，首切与分配方同值；核对不按范围收窄。** 客户账户给的号在该客户账户内算数，渠道签发的号在该渠道服务方内算数。范围仍单列一格，不由分配方推出：委托内箱号、渠道产品内号段这类更窄的范围日后是加列，不是改键。核对时不按范围收窄——现场观察只带一个号码，不带范围；两个客户账户恰好用了同一个号，就是两个候选。

**七、渠道来源的作废与替代随面单交易的后续动作形成。** 渠道作废覆盖了某件包裹（逐件或整笔），该交易给这件包裹形成的关联转已作废。换单形成的新交易（前序关系为替代）对同一包裹受理出新号时，原交易给这件包裹形成的关联转已被替代，并回指新关联。渠道退款不改适用状态——退款不等于号作废。已失效没有执行器：有效期规则今天没有登记处，本记录不造。

## Consequences

- PS 迁移两份：关联表与按号码查的索引随客户包裹件号进来，适用变化表随渠道来源的作废与替代进来（客户包裹件号没有作废或替代的来源事实）；领域一组值类型与关联、客户包裹件号配对；端口一个写入面、一个核对读口；委托仓储与面单交易仓储各多写一张表；NO 一个防腐适配器；两处装配换掉未配置替身，`cmd/parcel-frontline-import` 开工前那句「身份核对缝未配置」的提示随之退场。`MECHANISM-INVENTORY.md` 的迁移份数、端口与消费缝计数都会变。
- 生产效果：委托接受之后，客户贴着自己包裹号送站，节点收寄能唯一识别、形成收寄与控制，票 operator-channel/25 的服务结果标记随之生效。接受之前送站答待识别；「身份后来被确认」的补关联（`AT-NO-026`）不在本记录。
- 演示：隔离形态的提交 Intake 已经收逐件 `customerParcelReference`，只需把配对带进命令；演示租户的委托走到接受之后，同一号码在节点即可识别。证据只记 `S`。
- 不在本记录：包裹身份谱系（拆分、合并后的新包裹继承还是新起号码）——谱系今天领域里没有模型；号码作废、更正的人工口；外部标识的查阅读面；客户参考号与渠道交易级号码两类的形成。

## 越权风险点

本记录由用户授权代决而成，下列各点是代决时拿不准或触及他方归属的，逐条单列供 PS owner 复核；不认可的走 supersede，不改写本记录。

1. **接受时形成而不是提交时形成。** 代价是客户提交后、接受前就送站的包裹答待识别。如果 owner 要「预收寄」——提交即可识别——改为提交时形成，核对时再按委托状态过滤，并补回已拒绝、已撤回那两格不进候选的规则。
2. **号码逐字比对。** 现场扫描与客户录入常差在大小写、前导零或两端空白。本记录一种折算都不内置；若 owner 认为「去两端空白」之类属产品策略，加在核对读口一处，不进登记册的键。
3. **客户包裹件号不随新提交版本重报。** 接受前改号只能撤回再提交。若 owner 要允许新版本改号，旧号的关联在接受时就不该形成，改法落在第三条的投影上。
4. **同一委托内两件包裹给同一个号不拒收。** 节点必然答身份冲突。要不要在提交时就拒，是一条没有 CONTEXT 依据的新门，本记录不加。
5. **委托仓储在保存已接受委托时写本册。** 登记册因此有两个写入方（委托仓储、面单交易仓储），同写一张表。若 owner 要单一写入方，改为编排显式调用本册写口，并把提交与接受的事务壳从仓储装饰器挪到编排外。
6. **客户参考号这次不形成关联。** 理由在第二条。若 owner 要现在就按客户参考号查委托，加一类、在同一投影里多写一行即可。

## Links

- PS CONTEXT「外部标识」「外部标识关联」与 Lifecycles「外部标识关联」：[../domain/parcel-shipment/CONTEXT.md](../domain/parcel-shipment/CONTEXT.md)
- [ADR-0060](./0060-parcel-lookup-uses-current-snapshot-projection.md)（委托当前投影列）、[ADR-0084](./0084-label-transaction-is-an-independent-aggregate-with-two-level-results.md)（面单交易两层结果）、[ADR-0113](./0113-carrier-master-document-is-an-independent-register-keyed-by-declared-reference-and-version.md)（外部号码立册、版本链只插不改的先例）
- 票：[ps-external-mark-relations/01](../../.scratch/ps-external-mark-relations/issues/01-external-mark-relations-have-no-model-in-parcel-shipment.md)
