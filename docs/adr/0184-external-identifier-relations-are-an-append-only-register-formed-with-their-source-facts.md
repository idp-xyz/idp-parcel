# ADR-0184：外部标识关联自立一本只追加的登记册，随来源事实同笔形成——客户包裹件号随委托接受、渠道包裹件号随面单交易包裹结果受理；节点按号码逐字核对，只拿回此刻适用关联所标识的包裹

Status: Accepted（2026-10-16，用户原话「这些需要确定的事情……你帮我做决定吧」与「开始」。Claude Code 会话据此代 PS owner 裁定下面七条，落在票 [ps-external-mark-relations/01](../../.scratch/ps-external-mark-relations/issues/01-external-mark-relations-have-no-model-in-parcel-shipment.md) 那一格。裁决能力边界：读过 PS CONTEXT「外部标识」「面单交易包裹结果」的定义、Rules 里外部标识两条与面单交易各条、`委托` 与 `包裹身份与服务` 两节生命周期，GLOSSARY「外部标识」「包裹内部标识」，CONTEXT-MAP `parcel-shipment → node-operations` 一条，UC-NO-002 的流程表、控制与身份一节和验收表；代码读过 `IsolatedSubmissionIntake` 的译装、`SubmitShipmentRequestHandler`、`ShipmentRequests.Save` 与 `FindCurrentAcceptedByParcel`、`LabelTransactionParcelResult`、NO 的 `ParcelIdentityView` 与两处 `unconfiguredParcelIdentityView`。**没读** UC-PS-001 全文与面单交易各编排的全部分支。）

Partially suspended by: [ADR-0185](./0185-external-identifier-relation-identity-includes-its-formation-basis.md)（被停条款：决定一「键（租户，标识类型，分配方，号码，被标识对象）」——键加入形成依据；其余各条不变，本记录正文不改写）；[ADR-0195](./0195-a-submitted-declaration-of-the-same-number-turns-a-unique-hit-into-an-identity-conflict.md)（被停条款：决定五「只交回包裹，不交分配方、客户账户或委托」里的「只交回包裹」——答复另带歧义信号；「不交分配方、客户账户或委托」与其余各条不变，本记录正文不改写）；[ADR-0212](./0212-a-customer-number-hit-needs-a-verifiable-delivery-scope.md)（被停条款：决定五「NO 的三分支据此答身份冲突，不按形成时间、来源或分配方挑一件」，适用场景是这件实物有可验证交付范围时——NO 按范围相容的候选认；决定六「现场观察只带一个号码，不带范围」——收寄来源可带交付范围依据引用，结论「核对时不按范围收窄」维持；其余各条不变，本记录正文不改写）

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

- owner 复核 2026-10-19 认可（用户 2026-10-16 授权「你作为业务和系统专家你来自决」，会话 idp-parcel-9b 代裁，逐条）：1. 接受时形成——PS CONTEXT「外部标识关联只在被标识对象成为正式对象时形成」，预收寄要先改这句、再补已拒绝与已撤回两格，不取；原句「接受之前送站答待识别」的前提由票 ps-external-mark-relations/04 更正，号只在别处在册时的唯一误关联由票 ps-external-mark-relations/05 出 ADR-0195 承接（未接受委托的声明只作歧义信号、不进候选），接受时形成不变；2. 号码逐字比对——维持（票 04 裁定）。范围：去两端空白只发生在接单入口，是把客户录入转成统一输入的机制，定的是委托的声明值；号码本身的字符一个不动，登记册、核对读口 `FindParcelsByExternalIdentifier` 与 NO 适配器都不折算；大小写、前导零、校验位的折算是决定五说的租户取值，今天`未配置`；核对读口不加任何折算，要加先改 PS CONTEXT「按外部标识核对包裹身份」那一条；事实更正归票 04，现场观察一侧归 no-reception-identity/02；3. 不随新版本重报——决定四「号码标识的是成员，不是版本内容」，接受前撤回再提交足以改号，今天没有业务事实要求版本内改号；4. 同号不拒——领域不加门；复核记录「越权风险点 4 的事实更正」留的那一问答：同一草案内不重复是该入口以客户包裹参考作成员键的形状纪律，不升格为产品策略，将来不以它作成员键的接单入口，在立那条入口的票里先补 CONTEXT 依据再定；5. 两个写入方——两方写的是不同标识类型的行（委托仓储写客户包裹件号，面单交易仓储写渠道包裹件号），各随自己的来源事实同一笔事务形成，正是决定三的要求，与 ADR-0060 投影列同一先例；改单一写入方要把事务壳挪出仓储，换不来什么；6. 客户参考号不形成——决定二的理由仍成立：没有按它查委托的消费方，真渠道的推导仍未决。附记：决定六不在上列，按票 04 裁定维持；唯一命中何时算「证据充分」，归票 05 与 ADR-0195。

## 复核记录（2026-10-16，接受当日；决定原文不动）

1. **决定一的键**：票 03 的两轴评审（隔离子代理）与非作者会话 `idp-parcel-9b` 对票 02 的评审各自独立揭出——作废后同号再标识同一对象、或换单沿用原号时，同键的新关联被吞。已由 [ADR-0185](./0185-external-identifier-relation-identity-includes-its-formation-basis.md) 部分停用，键加入形成依据。
2. **越权风险点 4 的事实更正**（出处同上，非作者会话对票 02 的评审）：风险点 4 写「同一委托内两件包裹给同一个号不拒收」，那只对领域成立。今天唯一的提交入口 `IsolatedSubmissionIntake` 以客户包裹参考作载荷摘要里的成员键，`CanonicalizeSubmissionPayload` 要求成员不重复，所以同一草案两件同号在规范化那一步就答 4xx，到不了领域。这是 Intake 草案形状的既有纪律，不是本记录加的门；本记录「领域不加门」的取舍，对将来不以客户包裹参考作成员键的接单入口仍成立。要不要把这条既有纪律当成产品策略写明，随风险点 4 一并交 owner 复核。
3. **越权风险点 2 的事实更正**（2026-10-16；出处：非作者会话 `idp-parcel-9b` 对票 02 的评审，Spec 与 Standards 两轴都提「去两端空白进了登记册的键」。同一会话据用户原话「你作为业务和系统专家你来自决」代 PS owner 裁定：维持，不改生产代码；仍请 PS owner 过目）：风险点 2 说「现场扫描与客户录入常差在大小写、前导零或两端空白」，要去两端空白也「不进登记册的键」。它漏看了一个既有的入口行为：客户录入一侧的两端空白，在进册之前已经去掉了。管理台 `buildDraft` 送出前逐格去一次，`IsolatedSubmissionIntake` 的 `customerDeclaredReferences` 对委托参考与逐件客户包裹参考再去一次，`customerParcelNumbers` 取的就是去过的值。所以经今天唯一的提交入口进册的客户包裹件号，不带两端空白。这一行随 `2b4186d2` 封存入库（那笔自称「未代为评审」），此后改这份文件的各笔都留着它，不是本记录加的。这里更正前提，风险点 2 原文不动；这条入口纪律自本条起以本条为准：
   - **它定的是声明值。** 去两端空白是 UC-PS-001「参与者与上下文协作」派给接入适配器的「转换为统一输入」中的一步。载荷摘要的成员键、委托快照里的配对与册里的号码同出这一个值，委托没有第二个声明值。Intake 去两端空白的是这类标识格，地址等内容条目的值原样进摘要。
   - **边界。** 入口只去单元格两端的空白，号码本身的字符一个不动：大小写、前导零、校验位、号码内部的空格都不改。改号码字符的规则，就是 PS CONTEXT「按外部标识核对包裹身份」那一条说的条码规则，属租户取值，今天`未配置`，在入口同样不许做。
   - **比对一侧照旧逐字。** 决定五与 PS CONTEXT 那一条管的是核对，不受本条影响。领域 `NewExternalIdentifierNumber`、册的写入、核对读口 `FindParcelsByExternalIdentifier` 与 NO 的 `ParcelIdentityView` 都不折算：两端带空白交来的号码原样落册；扫到的号多一个空格、带回车换行或大小写不同，核对答零个候选，NO 走待识别，不会关联错件（`TestIsolatedSubmissionIntakeTrimsOnlyTheEndsOfCustomerReferences`、`TestTheRegisterKeepsNumbersVerbatimAcrossWhitespace`、`TestTheSameNumberFromTwoCustomersAnswersBothParcelsVerbatimWithinTheTenant`）。入口去空白也不制造新的错配：去空白后撞号的，同一草案在规范化那一步答 4xx（本节「越权风险点 4 的事实更正」）；跨委托的，与本来就同号没有区别，落到本节「决定六与 UC-NO-002「权限、隔离与审计」」那两格。
   - **不在本条。** 现场观察一侧归 NO：受控批量口的收寄模板（`decodeTemplate`）逐格去两端空白，在线收寄口（`receptionCommand`）把 `mark` 原样交进命令与收寄内容摘要。扫描附带的回车换行由谁去、两口要不要一致，交 NO owner（票 no-reception-identity/02）。风险点 2 留给 owner 的那一问——核对读口做「去两端空白」之类的折算，算不算产品策略——不由本条关闭，由越权风险点节的 owner 复核行答（核对读口不加折算）；要做，先改 PS CONTEXT 那一句（CONTEXT 高于本记录）。
   - **留给 PS owner**：本册按号码跨来源查，写册的入口口径不一时，不去两端空白的那个入口进来的号，正常扫描就配不上。渠道包裹件号今天原样取自渠道结果，只是面单渠道写链还没有触发面。要不要把「进册号码不带两端空白」定为所有写入方的约定（交不出就拒收，不在册里折算），须在那条写链有触发面、真渠道或首方 API 的接单入口上线之前有答案。
4. **决定六与 UC-NO-002「权限、隔离与审计」**（2026-10-16；出处同上，两轴都提，本记录接受时没读那一节。同一会话据同一授权代 PS owner 维持决定六，并代 NO owner 同步 UC-NO-002 那一节首条与「输入语义契约」里「包裹身份依据」一行；两处仍请各自 owner 过目）：
   - **读法。** 那一节首条「收寄和身份核对必须在明确集团租户、客户账户、节点和作业责任范围内执行」，要与同节「跨客户批量作业只允许最小必要信息」连读；它又是 UC-PS-003、UC-NR-001、UC-NR-003 同名节的同一句式。两相对照，它说的是授权与隔离。候选怎么取，CONTEXT-MAP `parcel-shipment → node-operations` 与 PS CONTEXT「按外部标识核对包裹身份」那一条已经定了：在租户内按号码取，不按分配方挑一件；权威次序上二者高于用例。同一份用例的「参与者与协作」写交付方「提供可验证的交付范围和身份线索」，可见用例设想过按交付范围识别，只是那份范围今天没有模型：作业事实信封交给收寄编排的只有租户与节点，载荷的 `deliveredBy` 只验非空，受控批量口同样只有 `deliveredBy`。按这一格的自报收窄，自报一错，身份冲突就变成唯一关联；按册上的 `scope` 过滤，还会把渠道包裹件号排出去。所以今天不收窄，决定六维持。租户隔离始终成立：读口按 `tenant_id` 过滤，租户取自认证结果或运维参数，他租户同号答零个候选。
   - **不收窄的代价有两格，都记下**（`TestTheWiredReceptionAnswersAConflictWhenTwoAcceptedRequestsShareANumber`、`TestTheWiredReceptionAssociatesTheOnlyRegisteredHolderOfANumber`）。
     - 其一，两份已接受委托撞号：答身份冲突、两个候选，包裹停在待识别，直到「身份后来被确认」（`AT-NO-026`）有执行器。
     - 其二，号只在一份已接受委托里在册：别的实物贴着同号送站，会唯一命中那一件，形成`节点收寄已形成`并建立控制。跨客户账户会这样（另一方的委托还没接受，或它根本没报这个号）；同一客户账户在两份委托里重复用号也会这样。提交入口只在同一草案内查重，所以后一种今天的演示租户就走得到。PS 采用这份收寄时，按关联到的包裹反查目标委托（`AdoptOnNodeIntakeAdapter.HandleFormedNodeIntake` 经 `FindCurrentAcceptedByParcel`），客户账户取自那份委托。所以 UC-PS-003「适用资格判断」里「来源所属租户、客户账户、责任法人和包裹与接受基线一致」那一项，对节点收寄来源拦不住它。这一格的修法另立票 ps-external-mark-relations/05，须在生产形态第一条接单入口上线之前裁定并修好。
   - **风险点 1 的更正。** 「接受之前送站答待识别」只在别处没有同号在册时成立；有，就是上面「其二」那一格。
   - **留给 PS owner 与 NO owner**（随票 05 裁）：PS CONTEXT「外部标识关联」说，客户账户给的号在该客户账户内算数。没有可验证的交付范围时，这类号的唯一命中，算不算 UC-NO-002 应用流程「唯一且证据充分时关联」里的「证据充分」？NO 端口 `ParcelIdentityView` 的头注写的是「恰一个→关联」，仓里没有记录论证过。
   - **日后。** 要让交付范围参与候选，不论收窄还是消歧，前提依次是：NO 给可验证的交付范围建模；先改 PS CONTEXT「按外部标识核对包裹身份」那一条与 CONTEXT-MAP 同句；再另立记录停用本条决定。怎么用、范围外的唯一命中怎么答，留给那篇记录定，本条不预设。哪个授权交付方可以替哪个客户账户送货，属租户取值。
5. **后果节在接受后被原地改写的补记**（2026-10-16；出处同上，两轴都列为非阻断。同一会话据同一授权补记，决定原文不动）：本记录在 `5714a137` 以 Accepted 立下。同日，票 02 的代码提交 `cc52e9b6` 在同一分支、进 main 之前，把 Consequences 以「PS 迁移」起头那一条的开头原地改了。接受时原句是「PS 迁移一份（关联表、适用变化表、按号码查的索引）」；现行文字是「PS 迁移两份：关联表与按号码查的索引随客户包裹件号进来，适用变化表随渠道来源的作废与替代进来（客户包裹件号没有作废或替代的来源事实）」。同一条其余各句、Decision 与越权风险点都没动。理由见票 02 完成记录「评审中改的」：票 02 没有任何来源事实会产生作废或替代，适用变化表挪到票 03，与渠道来源同笔进来。这是实施拆分的如实更正，但改的是已接受记录的正文。照 AGENTS.md「改文档」里「不改写已接受 ADR 历史」，它本该像本节「越权风险点 4 的事实更正」那样追加在这里。现行文字与落地相符（迁移 `0024`、`0025`；`0025` 同笔另含 ADR-0185 的改键，见该记录），所以保留、不回滚：回滚是对正文的又一次改写，还会让后果节与迁移对不上。接受时的原句，以本条引文与 `5714a137` 为准。

## 复核记录（2026-10-19，决定原文不动）

1. **决定五一句由 ADR-0195 部分停用，「其二」里对方已提交的那一格由它收住**（出处：票 [ps-external-mark-relations/05](../../.scratch/ps-external-mark-relations/issues/05-a-unique-hit-in-someone-elses-number-space-is-not-enough.md) 裁定。会话 `idp-parcel-9b` 据用户原话「你作为业务和系统专家你来自决」代 PS owner 与 NO owner 裁定，决定原文不动）：
   - **决定五「只交回包裹」。** 身份核对的答复另带歧义信号：号码此刻是否同时被本租户一份`已提交`委托声明为客户包裹件号。同句「不交分配方、客户账户或委托」照旧，信号只是一个是否值。
   - **决定六维持。** 候选与信号都不按范围收窄。
   - **上一节「决定六与 UC-NO-002「权限、隔离与审计」」的代价「其二」。** 对方的委托已提交、还没接受的那一格，不论跨客户账户还是同一客户账户重复用号，NO 都答身份冲突、不再唯一关联（`TestTheWiredReceptionAnswersAConflictWhenAnUnacceptedRequestAlsoDeclaresTheNumber`，由 `TestTheWiredReceptionAssociatesTheOnlyRegisteredHolderOfANumber` 翻转而来）。对方从未报过这个号、或对方委托已被拒绝或撤回的那几种情形，仍按原文，唯一命中照旧关联（`TestTheWiredReceptionAssociatesOnceTheOtherDeclarationIsRejectedOrWithdrawn`）。
   - **上一节「风险点 1 的更正」再更正。** 「接受之前送站答待识别」：号码只在别处一份已接受委托里在册、而送站这件所属的委托已提交并报了这个号时，答待识别并记身份冲突，不再唯一关联到别处那一件。
2. **号码进册前要存得下：带 NUL 的不收，UTF-8 字节数以 512 为界**（出处：票 [ps-external-mark-relations/06](../../.scratch/ps-external-mark-relations/issues/06-numbers-enter-the-register-only-when-storable.md) 裁定与裁定补。会话 `idp-parcel-9b` 据用户原话「你作为业务和系统专家你来自决」代 PS owner 裁定，决定原文不动）：
   - **为什么要这道门。** 本册按号码建 btree 索引，索引行有长度上限。号码原样进册，越过上限的在写入那一刻整笔回滚，入口答 500，重发同一个号永远一样。号码带 NUL 时 Postgres 的 text 存不下，同样答 500。决定一说行只插不改，决定五说号码逐字保存，两条都没说号码要存得下。
   - **门在哪里。** 只在领域 `NewExternalIdentifierNumber` 一处：拒 NUL，拒 UTF-8 字节数超过 512 的号，拒因是 `ErrExternalIdentifierNumberNotStorable`。512 是「存得下、索引得了」的机制界，不是业务口径：远低于索引行上限，远高于现行条码与运单号的长度。租户真实号码多长，不在这里配。决定五的逐字照旧，其余字符一个不拒、不折算，回车换行、Tab、GS 分隔符照收。
   - **写入方怎么答。** 首方提交口（`CustomerSubmissionIntake`）的客户包裹件号越界，答 400 `MALFORMED_REQUEST`，`error.detail` 点名载荷里那一格并说明拒因、不带号码本身（ADR-0140 Decision 三给 `detail` 划的边界），委托与来源保全都不落。面单渠道结果里受理那件的渠道包裹件号越界，`RecordChannelResult` 照该路坏结果的答法拒收（`ErrInvalidLabelTransaction`，编排答`未受理`），交易停在原状态。这道门只设在新号进来的那一步：已落库的面单结果重建时不过它，委托快照里的号经 `RehydrateExternalIdentifierNumber` 还原、只拒全空白，门立起来之前存下的号照样读得回。
   - **核对一侧怎么答。** 入册门只管写，读的一侧不拿它挡已在册的号。读口 `FindParcelsByExternalIdentifier` 收核对用的号码类型 `ExternalIdentifierLookupNumber`，构造时只拒全空白与带 NUL 的号，长度不设界。读口按号比较、不构造索引行，长号不报错。所以门立起来之前进册的超长号，现场扫到照样唯一关联；册里没有的照常答零个候选。带 NUL 的号 Postgres 的 text 存不下，确定不在册，NO 的 `ParcelIdentityView` 在构造那一步就答零个候选、无歧义信号、不报错，不去问 PS。拿去问反倒在库面报错，收寄会读成身份读不动，落未核对的待识别、响重核门铃，而重核拿着同一个号永远续不上。核对用的号与入册的号分成两个类型，前者交不进任何写入方，绕不过入册门。
   - 用例：`TestAnExternalIdentifierNumberMustBeStorable`、`TestAChannelResultWhoseNumberCannotEnterTheRegisterIsNotAccepted`、`TestTheWiredSubmissionRefusesACustomerParcelReferenceTheRegisterCannotStore`、`TestTheWiredReceptionAnswersAMarkTheRegisterCannotHoldAsCheckedPending`、`TestASnapshotStoredBeforeTheNumberGateStillReadsBack`、`TestTheWiredReceptionAssociatesAnOverlongNumberRegisteredBeforeTheGate`、`TestANulMarkIsAnsweredWithoutAskingTheRealRegister`。
   - **越权风险点。** 代 PS owner 定了 512 字节这一界。owner 复核时看：有没有渠道会把更长的内容当号码交来，例如整段二维码内容。要放宽，只改领域那一处的界，新界仍须低于索引行上限。要让超过索引行上限的号进册，得先换索引形状，例如按号码摘要建索引，那要另立记录。
   - **门前已存下的超长号。** 门落地之前，可能已有超过 512 字节的客户包裹件号进了委托快照（提交口那时只拒全空白）。这样的委托读得回，接受时号照常进册，现场扫到同一个号照样唯一关联，见上两条。真实租户尚未上线；演示种子不灌客户包裹件号，演示库里经管理台录入的号有没有超界，本票没有去查。

## Links

- PS CONTEXT「外部标识」「外部标识关联」与 Lifecycles「外部标识关联」：[../domain/parcel-shipment/CONTEXT.md](../domain/parcel-shipment/CONTEXT.md)
- [ADR-0060](./0060-parcel-lookup-uses-current-snapshot-projection.md)（委托当前投影列）、[ADR-0084](./0084-label-transaction-is-an-independent-aggregate-with-two-level-results.md)（面单交易两层结果）、[ADR-0113](./0113-carrier-master-document-is-an-independent-register-keyed-by-declared-reference-and-version.md)（外部号码立册、版本链只插不改的先例）
- 票：[ps-external-mark-relations/01](../../.scratch/ps-external-mark-relations/issues/01-external-mark-relations-have-no-model-in-parcel-shipment.md)
- [ADR-0185](./0185-external-identifier-relation-identity-includes-its-formation-basis.md)：部分停用本记录决定一的键——键加入形成依据；其余各条不变
- [ADR-0195](./0195-a-submitted-declaration-of-the-same-number-turns-a-unique-hit-into-an-identity-conflict.md)：部分停用本记录决定五「只交回包裹」一句——答复另带歧义信号；决定六维持，「其二」里对方已提交的那一格由它收住
- [ADR-0212](./0212-a-customer-number-hit-needs-a-verifiable-delivery-scope.md)：部分停用本记录决定五「NO 的三分支据此答身份冲突，不按形成时间、来源或分配方挑一件」（只限有可验证交付范围时）与决定六「现场观察只带一个号码，不带范围」；决定六的结论维持，复核记录「决定六与 UC-NO-002「权限、隔离与审计」」代价「其二」余下的各格由它收住

## 复核记录（2026-10-19，ADR-0212 部分停用决定五、决定六各一句；决定原文不动）

出处：票 [ps-external-mark-relations/07](../../.scratch/ps-external-mark-relations/issues/07-verifiable-delivery-scope-for-customer-numbers.md) 的方向裁定与 [ADR-0212](./0212-a-customer-number-hit-needs-a-verifiable-delivery-scope.md)。会话 `idp-parcel-9b` 据用户原话「你作为业务和系统专家你来自决」与 2026-10-19「需要」代 PS owner 与 NO owner 裁定方向，ADR-0212 由该会话起的作者子代理成文。停用随票 ps-external-mark-relations/11 进 main 起生效，此前 main 上的行为照原句。

- **决定五**「NO 的三分支据此答身份冲突，不按形成时间、来源或分配方挑一件」停用，适用场景：这件实物有可验证交付范围（送站预报，或绑定已全部确认的送站交接单）时，NO 只数范围相容的候选，恰一个且没有范围内歧义信号即唯一命中。同句「命中多件即交回多件」照旧，PS 仍全部交回、不挑；同条「不交分配方、客户账户或委托」照旧，范围相容只是是否值，所比的客户账户由 NO 给出。
- **决定六**「现场观察只带一个号码，不带范围」停用：收寄来源可以带交付范围依据引用。结论「核对时不按范围收窄」与「两个客户账户恰好用了同一个号，就是两个候选」维持：候选照旧在租户内全部取、全部记，范围只消歧。
- **复核记录「决定六与 UC-NO-002「权限、隔离与审计」」。** 「日后」一段所待的那篇记录就是 ADR-0212：NO 给可验证交付范围建了模，PS CONTEXT「按外部标识核对包裹身份」与 CONTEXT-MAP 同句已先改；范围外的唯一命中怎么答由它定——只经客户包裹件号的唯一命中缺范围时答待识别、不记冲突。代价「其二」余下的各格（对方从未报过这个号、对方委托已被拒绝或撤回）由此收住。「留给 PS owner 与 NO owner」那一问——没有可验证交付范围时，客户账户给的号的唯一命中算不算证据充分——答：不算。同段说授权交付关系属租户取值，ADR-0212 把它定为 `party-commercial` 参与方关系的一种。
