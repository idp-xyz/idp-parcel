# ADR-0371：来源为「仓配出库」的委托由仓配履约以货主名义代办——请求方记货主客户账户、实际提交方记仓配履约，授权是 PC 新增的「仓配代办」动作配产品常量等级、由租户按货主授予；停在等待受控补充时由仓配履约续办；资料修订的采用经资料版本信封交回；接受或拒绝无论经哪条路形成都交出接受决定信封；本产品网络从海外仓发出时只用面单渠道服务

Status: Accepted（2026-11-06，Cursor 会话代 PS owner 决：用户原话「请你起一个agent 作为owner来评审吧」，子代理代各 owner 评审后给出本记录要定的几格（报告 [owner-review-2026-11-06](../../.scratch/warehouse-fulfillment/owner-review-2026-11-06.md)，钉 `0874218c9`）；用户随后在问题卡上选「现在全部落实：立小包托运「仓配代货主」请求方的 ADR，改各处文档，非作者评审后合入 main」。决定六是产品 owner 一格：用户答「我们是集货到海外，然后走的本地小包的方式，你的判断呢」，2026-11-09 在问题卡上确认读法时选「两种都有」（专线小包与海外仓），决定六是会话据此作的判断。票 [warehouse-fulfillment/04](../../.scratch/warehouse-fulfillment/issues/04-carry-out-the-owner-review-conditions.md)。回答 [ADR-0368](./0368-commodity-inventory-and-warehouse-fulfillment-join-the-product-as-a-new-context-and-node-operations-stays-non-wms.md) 越权风险点 2 与 [ADR-0370](./0370-warehouse-outbound-dispatches-by-product-network-owner-label-or-owner-pickup-and-the-remaining-warehouse-gaps-are-decided.md) 越权风险点 2、3（无条款停用），限定 ADR-0370 决定一、决定三里「本产品网络」的读法（不是停用），见「对已接受记录的作用」。越权风险点见文末，2026-11-09 由子代理代 PS、PC、结算与经营核算和产品 owner 复核（用户原话「请你起一个agent 作为owner来评审吧」），记录在文末越权风险点之后。）<!-- 索引注记 -->｜**判据**：来源为「仓配出库」的委托，提交、补充、资料修订、撤回与包裹取消由仓配履约以货主名义代办——请求方记货主客户账户、实际提交方记仓配履约、证据指名出库请求与包裹序号，提交之外各请求的来源请求键加命令前缀；授权是 PC 授权册上新增的授权动作「仓配代办」配产品常量等级 `WAREHOUSE_FULFILLMENT_AGENT`，资料修订照旧以资料修订动作配这一等级，租户按货主授予，两条互不蕴含，包裹取消另要取消授权规则覆盖仓配代办，没有授予答未配置；接受判断停进等待受控补充时同一事务交「委托待受控补充」信封，仓配出库的委托由仓配履约续办；仓配履约提交的资料修订，采用结果经资料版本已形成的信封交回；接受或拒绝无论经接受链、主动拒绝还是授权处置形成，都在同一事务交接受决定已形成的信封，它是发布意图不是续办触发，撤回不交；本产品网络从起运侧的仓提交国际小包委托，海外仓与目的国代理仓发给当地收件人的件选本产品网络时只能提交面单渠道服务产品的委托（把尾程单独对客销售），不用运营企业自己的网络服务产品

## Context

[ADR-0368](./0368-commodity-inventory-and-warehouse-fulfillment-join-the-product-as-a-new-context-and-node-operations-stays-non-wms.md) 决定四让仓配履约以出库请求所属货主客户账户的名义向 `parcel-shipment` 提交委托，[ADR-0370](./0370-warehouse-outbound-dispatches-by-product-network-owner-label-or-owner-pickup-and-the-remaining-warehouse-gaps-are-decided.md) 决定二、三又让它为预取面单提交声明测量的更正、为代理仓出库代货主提交委托。谁是请求方、凭什么授权，两份记录都留给小包托运：ADR-0368 越权风险点 2 写「来源身份、授权依据与接受判断的关系归 PS owner。」ADR-0370 越权风险点 2 写「请求方与授权依据要小包托运定」。UC-PS-002、UC-PS-005、UC-PS-006 的仓配行在 `0874218c9` 上都记着这一格未定。

小包托运现行的请求方模型接不住这条路：

- 「来源请求身份」词条写它「只用于来源重放与冲突分类，不授予查询或操作权限」，所以来源「仓配出库」本身不能兼作授权。
- 「资料补充/更正请求」词条在 `0874218c9` 上只认客户或授权运营角色，仓配履约两样都不是。
- 首方渠道的授权是直客 1:1、册上绑定即授权（[ADR-0310](./0310-first-party-customer-command-five-faces-launch-with-direct-customer-one-to-one-and-command-prefixed-request-keys.md) 决定二），资料修订以产品常量等级 `CUSTOMER_SELF_SERVICE` 向 PC 问授权（[ADR-0311](./0311-a-first-party-customers-source-data-amendment-asks-party-commercial-at-the-product-level-customer-self-service-and-the-tenant-controls-it-by-granting-that-level.md)）。这两份都限定首方渠道，内部上下文提交时没有渠道身份可绑。

2026-11-06 的 owner 评审（子代理代各 owner，报告见 Status 行）在这一格上又发现几件同根的事：

- **等待受控补充没有续办方。** 仓配履约 CONTEXT 写「都没有的格照实空着」，客户可补充缺口会是常见路径；授权处置选交客户补充也落到这一格。首方补充要带提交回执里的 `submissionVersionId`（ADR-0310 决定五），回执却交给了仓配；等待态不是决定，小包托运也不交接受决定信封，包裹打好包停在仓里，没有人被告知。
- **资料修订的采用结果怎样到仓配没写。** 小包托运已有资料版本已形成的信封（`parcel-shipment.source-data-version.formed`），载荷带来源请求身份、资料组、采用结果与采用的版本，取证于 `0874218c9` 时还没有消费方。
- **两条拒绝路径不交接受决定信封。** 取证于 `0874218c9`：交这封信封的只有 `FormAcceptanceDecisionHandler` 的 `handOffDecision`，`RejectShipmentRequestDeps` 与 `DisposeShipmentRequestDeps` 都没有交接端口；撤回没有任何信封（`handOffDecision` 头注「没有决定可交时不发意图（例如已撤回的委托）」）。仓配履约 CONTEXT 与 CONTEXT-MAP 在 `0874218c9` 上已把这两种拒绝也经这封信封交出写成目标，评审认可，但指出这是小包托运自己的出向规则，应写在小包托运 CONTEXT。
- **海外仓发给当地收件人的件不是国际小包。** 小包托运「委托」词条是「客户要求运营企业为一个或多个包裹提供同一项约定国际小包服务的业务请求。」ADR-0370 的 Context 以「海外仓的一件代发」引出自备面单与自提，代理仓也多在目的国；从那里经本产品网络发给当地收件人，按这条定义不是委托。评审把它列为产品 owner 二选一：写明本产品网络只对国际小包成立，或另立 ADR 放宽客户群。用户答自己的业务是「集货到海外，然后走的本地小包的方式」；问清读法时列了专线小包（包裹在国内就有收件人）与海外仓（整批备货、下单后在当地发）两种，用户答「两种都有」。

## Decision

**一、来源为「仓配出库」的委托，五种请求由仓配履约以货主名义代办。**

- 提交、补充（停在等待受控补充时形成新提交版本）、资料修订（资料补充/更正请求）、撤回与包裹取消：请求方记出库请求所属的货主客户账户，实际提交方记 `warehouse-fulfillment`，请求证据指名出库请求与包裹序号；代理仓出库另指名代理仓报来的装箱结果。
- 提交的来源请求键是（出库请求，包裹序号）；其余各请求的来源请求键照 ADR-0310 决定一加命令前缀，不与提交键相撞。
- 只对来源为「仓配出库」的委托成立；别的来源的委托，仓配履约不是请求方。
- 委托提交版本要保留的提交主体就记这两格：审计上看得出这是仓配代办，不是货主本人经客户渠道交来，也不是运营角色的动作。

**二、授权：PC 授权册新增授权动作「仓配代办」，配产品常量等级 `WAREHOUSE_FULFILLMENT_AGENT`，由租户按货主授予。**

- PC 的授予是「动作 × 等级」，提交、补充、撤回与取消在授权册上原本没有动作可问，所以新增一个动作。「仓配代办」与 PC 既有各授权动作并列，同样按责任法人、等级、适用范围和有效期间版本化，与资料修订互不蕴含。
- 等级形照 ADR-0311 的 `CUSTOMER_SELF_SERVICE`：产品常量，只在 `internal/parcelshipment/adapters/partycommercial` 一处定义（与 `CustomerSelfServiceAuthorityLevel` 同包），含义是「仓配履约以货主名义代办来源为仓配出库的委托」这一种请求方身份。法人与范围照 ADR-0311 决定二，读该货主客户账户的解析键登记行。
- 各请求问哪条授予：
  - 提交、补充与撤回问「仓配代办 × `WAREHOUSE_FULFILLMENT_AGENT`」。补充另核对基准版本，过期答 `SUPPLEMENT_BASIS_STALE`（ADR-0310 决定五）。撤回时，仓配履约在这条授予下就是货主的当前有效授权代表。
  - 资料修订问「资料修订 × `WAREHOUSE_FULFILLMENT_AGENT`」（动作出自 [ADR-0116](./0116-source-data-amendment-is-an-authorized-action-and-contract-delegation-resolves-the-actual-decider.md)，问法同 ADR-0311）；允许改什么照旧由所采用接单规则包的资料修订允许声明说。
  - 包裹取消问「仓配代办」那条授予，另要租户的取消授权规则（ADR-0310 决定三）里有一条管仓配代办，没有答`未配置`。
- 两条授予互不蕴含，哪一条没授予，对应的请求就答`未配置`：提交不建委托，其余请求不受理。产品不给默认授予。实际决定方是货主客户账户本身，合同委派不参与。
- 接受判断不因来源与代办而变（ADR-0368 决定四照旧）。

**三、停在等待受控补充时，续办由代办的仓配履约以货主名义做。**

- 接受判断停进等待受控补充时（客户可补充缺口，或授权处置选交客户补充），小包托运在同一事务交出「委托待受控补充」信封：指名委托、停住的提交版本与缺口原因的引用，不带资料内容。它是交给下游的发布意图，不是续办触发：续办照旧由新提交版本已形成的信封驱动（[ADR-0106](./0106-customer-supplement-wait-is-a-committed-pause-resumed-by-the-new-submission-version-envelope.md)）。
- 来源为「仓配出库」的委托，仓配履约据此向货主要补正，以货主名义补充（决定一、二），补充基准取信封里的提交版本。货主手里没有提交回执，不直接补。

**四、仓配履约提交的资料修订，采用结果经资料版本已形成的信封交回。**

- 仓配履约认来源为「仓配出库」的资料版本已形成信封，据此判断它提交的更正采用了没有。
- 采用结果停在待补充时，续办由代办的仓配履约做：小包托运「当前资料采用判断」说待补充的续办方是客户，这一来源上客户由仓配代办。停在待复核或未决时等；冲突即更正不成。仓配等多久、到时怎么办归仓配履约 CONTEXT。

**五、接受或拒绝决定无论经哪条路形成，都在形成它的同一事务里交出接受决定信封。**

- 接受链形成的接受与拒绝、运营主动拒绝、授权处置选拒绝，都交接受决定已形成的信封；信封标识取决定标识，接受链再跑一轮时补交的是同一封，下游按标识认领。
- 这封信封是交给下游的发布意图，本上下文不消费它，它不是续办触发：[ADR-0086](./0086-manual-review-wait-is-a-committed-pause-resumed-by-completion-envelope.md) 决定三不许主动拒绝再发的续办信封，[ADR-0132](./0132-authorized-disposition-decides-the-destination-of-a-restricted-request-as-its-own-wait-state-and-never-passes.md) 决定二说的「不新增信封类型」，都不是它。
- 撤回不形成决定，不交这封信封；仓配履约自己发起的撤回以小包托运对那次请求的答复为准。
- 现有消费方对拒绝都是入账收工（取证于 `0874218c9`），不需改。

**六、本产品网络按仓的位置分两种：起运侧提交国际小包委托，海外仓的本地件只经面单渠道服务。**

- 仓在起运侧（如国内的自营仓或代理仓）时，发运方式选本产品网络，提交的是国际小包委托：委托随网络到目的国，尾程交本地小包派送。这正是开发主线「小包专线与货代」一层「头程自营或半自营、尾程交末端渠道」的常规形态。
- 海外仓与目的国代理仓发给当地收件人的件，货已经在目的国。发运方式选本产品网络时，只能提交面单渠道服务产品的委托：运营企业代货主向本地承运商取得面单，仓配履约打印粘贴，交给面单所指的承运商即出库确认。这就是开发主线同一行说的「把这段单独对客销售」，客户群不变。
- 小包托运「委托」词条说的「同一项约定国际小包服务」，含这样单独销售的尾程：货已在目的国、委托只取得本地承运商面单的，仍属本产品的国际小包服务。
- 这类件不用运营企业自己的网络服务产品。运营企业在目的国自己组织本地派送，是开发主线说的放宽到本地小包网络，要另立 ADR。
- 海外仓与目的国代理仓照旧可以选自备面单或自提。

## 对已接受记录的作用

- ADR-0368 越权风险点 2「委托的新来源（决定四）：内部上下文以货主名义提交委托，是 `parcel-shipment` 一种新的、不经客户渠道的来源；来源身份、授权依据与接受判断的关系归 PS owner。」由决定一、二回答：来源身份照旧只管重放与冲突，授权依据是代办等级，接受判断不因来源变。无条款停用。
- ADR-0370 越权风险点 2 里「请求方与授权依据要小包托运定」一句由决定一、二、四回答；越权风险点 3 的仓配代货主提交由决定一回答，证据另指名代理仓报来的装箱结果。无条款停用。
- ADR-0370 决定一「**本产品网络**：出库请求带要求的服务产品，逐包裹以（出库请求，包裹序号）提交委托，来源「仓配出库」，照 ADR-0368 决定四。」与决定三「发运方式照决定一：本产品网络的，由仓配履约在代理仓报来装箱结果时代货主提交委托，声明测量取代理仓报来的数，委托接受后把面单交给代理仓；」两句按决定六读：从起运侧的仓提交国际小包委托；海外仓与目的国代理仓的本地件只能提交面单渠道服务产品的委托，不用运营企业自己的网络服务产品。这是读法限定，不是停用。
- 不停用、照旧成立：
  - ADR-0310、ADR-0311 限定首方渠道，本记录另立一种请求方，不改它们。
  - ADR-0116、ADR-0311：「仓配代办」与资料修订并列、互不蕴含；仓配履约的资料修订照旧问资料修订动作，只是等级换成代办等级。
  - ADR-0086 决定三与 ADR-0132 决定二守的是接受链只有一个推进者；决定三、五的信封都不驱动接受链。
  - ADR-0106 的续办触发照旧是新提交版本已形成的信封。

## 同笔改动的文件

- [小包托运 CONTEXT](../domain/parcel-shipment/CONTEXT.md)：「委托」「资料补充/更正请求」两个词条；规则补决定一、二、三、五。
- [参与方与商业 CONTEXT](../domain/party-commercial/CONTEXT.md)：规则补「仓配代办」这一授权动作；UC-PC-003「给开发的交接」记它未做。
- [仓配履约 CONTEXT](../domain/warehouse-fulfillment/CONTEXT.md)：决定一至六落到仓配的规则，以及 owner 评审的其余条件（见票 04）。
- [CONTEXT-MAP](../domain/CONTEXT-MAP.md)：仓配履约与小包托运之间两条关系约束与关系图的边；[GLOSSARY](../domain/GLOSSARY.md)「出库请求」「委托」「服务产品」。
- UC-PS-001、UC-PS-002、UC-PS-005、UC-PS-006「给开发的交接」：代办请求方、授权等级、两封信封与采用回传的实现差距。
- [开发主线](../product/PARCEL-NETWORK-FIRST-RELEASE-DEVELOPMENT-BASELINE.md)三层表 3PL 行补仓配形态（owner 评审 ADR-0368·1 的条件）。
- ADR-0368、ADR-0370：Status 行加本记录的来向指针，Links 各加一条；越权风险点旁记 owner 复核。ADR 索引由推送方重生成。

## Consequences

- 小包托运多一种请求方（仓配代办）和一个授权等级，多两件出向义务：各路径的接受决定信封、委托待受控补充信封。实现差距记在 UC-PS-001、UC-PS-002、UC-PS-005、UC-PS-006，本记录不动 `internal/**`。
- 仓配履约要消费四类小包托运的信封：接受决定已形成、包裹取消已记录、资料版本已形成、委托待受控补充。
- 租户不给货主授予代办等级，这个货主的本产品网络出库就答`未配置`，仓里的包裹提交不了；这是如实作答，不是故障。
- 海外仓一件代发可以用运营企业的尾程面单（面单渠道服务），也可以货主自带面单或自提；运营企业在目的国自己的本地派送网络不在产品内。

## Alternatives considered

- **仓配履约以运营角色的身份提交与修订。** 否决：运营角色是租户的人，仓配履约是系统上下文；混成一种，审计上分不出代办与运营决定，运营角色的授权也会被一个系统动作借用。
- **把来源「仓配出库」本身当授权。** 否决：小包托运「来源请求身份」明写它不授予查询或操作权限。
- **复用 `CUSTOMER_SELF_SERVICE`。** 否决：租户就分不开「货主自己改」与「仓代货主改」两种授予，给了一种就等于给了另一种。
- **等待受控补充由仓配轮询委托查阅。** 否决：每个打好包的包裹都要轮询；暂停是一次性事实，与暂停同一事务交出信封，不会漏也不会晚。
- **主动拒绝与授权处置不交信封，由仓配读委托状态。** 否决：违仓配履约「不读委托的内部状态」，也让同一种决定按形成路径分两种交法。
- **海外仓的本地件只走自备面单或自提。** 否决：用户确认海外仓也是自己的业务（「两种都有」），这样会把它挡在本产品之外；开发主线已把单独销售尾程列为这一层的常规形态。
- **海外仓的本地件也走运营企业自己的网络服务产品。** 否决（此刻）：那是在目的国自己组织本地派送，即放宽到本地小包网络，要重判依旧客户群作出的能力范围结论（开发主线「客户群一旦放宽……必须重新评估」）；海外仓业务用面单渠道服务就够。

## 越权风险点

1. 决定一、二扩了小包托运的请求方模型，并给 PC 授权册加一个授权动作；动作与等级的名字归 PS 与 PC owner。包裹取消另要租户的取消授权规则覆盖仓配代办，那份规则怎样表达这一格归 PS owner。
2. 决定三新增一种小包托运出向信封；它与 ADR-0132 决定二「不新增信封类型」的分界（不驱动接受链）归 PS owner。
3. 决定五把主动拒绝与授权处置纳入出向；现有消费方对拒绝入账收工，新消费方要按决定标识认领。归 PS owner。
4. 决定六是会话按用户确认的两种业务模式作的判断，属产品 owner；把单独销售的尾程读进小包托运「委托」的国际小包服务，归 PS owner 复核。

- owner 复核 2026-11-09 认可（复审时有条件认可的各格，条件在合入前已落地，终审核过。用户 2026-11-06 原话「请你起一个agent 作为owner来评审吧」，子代理代 PS、PC、结算与经营核算和产品 owner 复核，复审 [owner-rereview-2026-11-07](../../.scratch/warehouse-fulfillment/owner-rereview-2026-11-07.md)，终审 [owner-final-2026-11-09](../../.scratch/warehouse-fulfillment/owner-final-2026-11-09.md)；逐条）：1. PS 与 PC owner 认可：「仓配代办」动作与代办等级照此命名；取消授权规则按请求方分格写，客户系统与仓配代办各一格，演示租户各登一条参考配置，真实租户上线时自登；2. PS owner 认可：新信封不驱动接受链，不违 ADR-0132 决定二；3. PS owner 认可：现有消费方对拒绝入账收工，新消费方按决定标识认领；4. 产品 owner 一格认可（子代理代看，用户可推翻），PS owner 认可：决定六是读法，不放宽客户群，GLOSSARY「委托」「服务产品」同笔写明单独销售的尾程。

## Links

- [ADR-0368](./0368-commodity-inventory-and-warehouse-fulfillment-join-the-product-as-a-new-context-and-node-operations-stays-non-wms.md)：回答其越权风险点 2
- [ADR-0370](./0370-warehouse-outbound-dispatches-by-product-network-owner-label-or-owner-pickup-and-the-remaining-warehouse-gaps-are-decided.md)：回答其越权风险点 2、3，限定决定一、决定三里「本产品网络」的读法
- [ADR-0310](./0310-first-party-customer-command-five-faces-launch-with-direct-customer-one-to-one-and-command-prefixed-request-keys.md)、[ADR-0311](./0311-a-first-party-customers-source-data-amendment-asks-party-commercial-at-the-product-level-customer-self-service-and-the-tenant-controls-it-by-granting-that-level.md)：首方渠道的请求方与授权等级，代办等级照它的形
- [ADR-0116](./0116-source-data-amendment-is-an-authorized-action-and-contract-delegation-resolves-the-actual-decider.md)：资料修订是授权动作
- [ADR-0086](./0086-manual-review-wait-is-a-committed-pause-resumed-by-completion-envelope.md)、[ADR-0132](./0132-authorized-disposition-decides-the-destination-of-a-restricted-request-as-its-own-wait-state-and-never-passes.md)、[ADR-0106](./0106-customer-supplement-wait-is-a-committed-pause-resumed-by-the-new-submission-version-envelope.md)：接受链的推进者与续办触发，决定三、五的信封不在其列
- [ADR-0266](./0266-a-submission-replay-answers-what-the-first-round-answered-and-only-unresolved-ownership-is-judged-again.md)：仓配重发提交时照它续办
- [owner 评审报告（2026-11-06）](../../.scratch/warehouse-fulfillment/owner-review-2026-11-06.md)、[票 warehouse-fulfillment/04](../../.scratch/warehouse-fulfillment/issues/04-carry-out-the-owner-review-conditions.md)
- [开发主线：目标客户群](../product/PARCEL-NETWORK-FIRST-RELEASE-DEVELOPMENT-BASELINE.md#目标客户群)
