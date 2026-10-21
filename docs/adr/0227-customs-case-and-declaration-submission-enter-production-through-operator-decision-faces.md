# ADR-0227：关务立案与申报提交经操作者渠道的运营决定口进入生产——建案、提交授权确认、申报提交执行各一种决定，首次尝试记「已登记待发送」，生产进程没有自动提交路径

Status: Accepted（2026-10-21，Claude Code 会话代决：用户原话「你作为业务和系统专家自决……解决所有的问题」；票 product-strategy-boundary/18，代 CC owner 分诊（裁定见票面「分诊裁定」）并实施。不停用任何已接受记录的条款。越权风险点见文末，待 CC owner 复核；决定一的三种决定种类与授予粒度待 PC owner 在角色模型（票 product-strategy-boundary/07）里会签。）<!-- 索引注记 -->｜**入口**：`POST /customs-cases`、`/customs-submission-authorizations`、`/customs-declaration-submissions` 三口挂操作者渠道「运营决定」能力面，各一种决定 `CUSTOMS_CASE_ESTABLISHMENT`、`CUSTOMS_SUBMISSION_AUTHORIZATION`、`CUSTOMS_DECLARATION_SUBMISSION`（access_identity 0018），批准与实际提交分开授予，谁持哪一种是租户取值；合成租户与真实租户同一条命令面，不加隔离放行。**建案**：包裹由请求指名，客户归属与来源资料引用由服务端按 parcel-shipment 当前已接受委托取，未接受答 `PARCEL_NOT_ACCEPTED`；要不要建仍只由建案要求规则册答，未登记答未决并指名未配置。**授权**：确认人取信封操作者，单元就绪须当前有效，授权依据写成「所凭决定种类@确认人」随提交版本快照；一单元一份，已有不顶替。**提交**：就绪与授权同时有效才固定提交版本；首次尝试记「已登记待发送」、无发送时间（customs_compliance 0025），不收调用方给的尝试结果与发送时间；越过发送边界归票 10。**没有自动提交路径**：接受扇出不建案不提交，建案、授权与提交编排在 `cmd/` 下只由运营决定口的装配文件构造。内置立案触发归票 35，授权的拒绝、撤销、再次申请与目标绑定归票 36。首发对就绪、授权「未登记」答未决并指名未配置，替代 AT-CC-105、110 在那一格的结果名。越权风险点含运维登记授权一路仍被提交执行认、授权只绑单元、组成不核案件关联、提交执行人不留痕、授权依据不带授予号、待发送只形成一半（无授权使用、无发送意图）与连接器的分工、建案四维与角色照请求收、授权使用未建模故更正补充在票 36 前不得开生产口、确认后授予撤销不连带授权失效｜**部分停用**（2026-10-21）：决定八「两行的正文不改，行旁注明首发答法见 `AT-CC-444` / `AT-CC-439` 与本条。」已由 [ADR-0243](./0243-customs-first-release-answers-replace-the-uc-cells-they-answer-and-unavailable-dependencies-answer-undecided.md) 停用——被首发答法替代的格从原 AT 行移出，原行只留仍成立的格；同一决定「这替代 UC-CC-004 `AT-CC-105`「请求未受理（授权前提不成立）」在「不存在」「已经失效」两格、UC-CC-005 `AT-CC-110`「提交前门禁已阻断」在「不存在」一格上的结果名（110 的已撤销照旧是门禁阻断那一类，答 `NOT_AUTHORIZED`）」一句已由同一记录扩成也替代 UC-CC-005 `AT-CC-109`「提交前门禁已阻断」在「不存在」一格上的结果名（提交执行时就绪没登答未决并指名未配置，`AT-CC-448`）。**适用场景**：只限 UC 文档的写法与替代范围；决定八的答法与其余各条不变
Date: 2026-10-21

## Context

- 现象（读码钉 `8bf7a231`，只记 `S`）：`customscompliance/application` 的 `NewEstablishCaseHandler`、`NewSubmitDeclarationHandler` 在 `cmd/` 零引用；接受决定的扇出只到 VE 与初始路由；生产进程里没有任何端点或消费门会建案或提交，外部结果口因此答「归属不上」（票 product-strategy-boundary/05 格 9、格 11）。
- 首发范围（PILOT-SCOPE）：系统完成资料校验并形成申报就绪结果；每次首次申报、更正、补充或撤销重报都必须由当前有效授权下的业务角色明确确认后才能提交，不启用生产自动申报；直接接口提交或由报关服务商代为提交都必须保留申报版本、确认人、授权依据、提交尝试和逐步外部回执；验证申报就绪与提交授权分离，自动提交条件只做设计或模拟验证。
- 关务 CONTEXT：提交授权请求、批准、拒绝、撤销、申报渠道账号使用和实际提交权限必须分别表达，拥有其中一项不能自动取得其他权限；提交授权与就绪判断分别形成和失效；每个逻辑申报目标首次实际对外发送前都必须形成不可覆盖的提交版本。UC-CC-004 的首发分支是「当前就绪、目标明确、实际决定方在决定时拥有适用批准权限 → 逐次形成已授权」；UC-CC-005 的结果契约有一格「提交已登记待发送」：提交版本、授权使用、待发起尝试身份与持久化发送意图都已形成、外部发送尚未开始。UC-CC-001 的启动条件：只有提交版本而尚未接受的范围不得建立正式关务案件。
- 已有的机制：建案编排（四维身份键、建案要求规则册判要不要建、同键同包裹集重放、异包裹集范围冲突）、提交编排（单元落册、就绪与授权两轨把门、内容指纹幂等、版本与首次尝试同笔落、提交版本形成意图由 VE 投影消费）。就绪判断册与提交授权册今天只有运维 CLI（`parcel-customs-register`）一条写入路，那一路的依据就是「能运行本 CLI」这道运维边界，不是业务角色确认。提交尝试的结果三值（已回执、失败、待确认）都在发送边界之后，`sent_at` 不许空；申报发送连接器不存在（票 product-strategy-boundary/10「申报发送通道」一项）。
- 操作者渠道的「运营决定」能力面已落地（ADR-0151，operator-channel/15）。最近的先例是 ADR-0204 与 ADR-0212：新决定种类追加进 `accessidentity` 与迁移、端点行挂 `decisionFace`、Intake 经 `operatorDecisions` 共享渠道、准入范围照 ADR-0149 决定四判。合成租户按真实租户对待，隔离形态逐面退场（ADR-0150）。
- 相邻：routing-first-cut/12 已给路由候选的关务适用性判断口（候选级、即时作答、不落史库，三格：可用 / 不可用 / 状态未知）。

## Decision

**一、三口三种运营决定。** 操作者渠道「运营决定」能力面加三种决定：`CUSTOMS_CASE_ESTABLISHMENT`（建案）、`CUSTOMS_SUBMISSION_AUTHORIZATION`（提交授权确认，即 CONTEXT 的「批准」）、`CUSTOMS_DECLARATION_SUBMISSION`（申报提交执行，即「实际提交权限」）。三口 `POST /customs-cases`、`POST /customs-submission-authorizations`、`POST /customs-declaration-submissions` 各按自己那一种认证，铸信封前读生产权威区间（能力 `OPERATION_DECISION`、事实类型取决定种类）。持其中一种不因此能用另两口；同一人可以持两种或三种，职责分离不由本记录强制——有哪些种类是产品策略，谁持哪一种是租户取值（参数登记册 `PAR-CUS-03`），没有授予答 `OPERATOR_NOT_GRANTED`。迁移 `access_identity/0018` 重建决定种类那条 CHECK。

**二、建案口按已接受包裹寻址。** 请求给监管范围四维（辖区、方向、程序、法定义务）、包裹身份与初始角色快照；包裹的客户归属与来源资料引用由服务端按信封租户在 parcel-shipment 的当前已接受委托里查（新端口 `AcceptedParcelSourceView`，消费侧适配器 `customscompliance/adapters/parcelshipment` 译 PS 的按包裹反查口：客户归属取委托的客户账户，来源引用取「委托/当前提交版本」）。请求体不收这两格。任一件不在当前已接受委托里答 `PARCEL_NOT_ACCEPTED` 并逐件列出，不建占位案件；源上下文答出两份已接受委托是它自己的确定性坏状态：端口交出 `ErrAcceptedParcelAmbiguous`，编排整笔报错（HTTP 500 `NO_ANSWER_FORMED`），不答可重试的未决，也不在本上下文挑一份。之后整份交给既有建案编排：要不要建仍只由建案要求规则册答，未登记答未决并指名 `CASE_REQUIREMENT_UNCONFIGURED`；幂等与范围冲突照旧。建案未决的原因此后分格（规则未登记、规则读不动、案件册、标识签发、源上下文读不动），HTTP 以 200 带原因作答，与外部结果接收口同一写法。

**三、授权确认口是 UC-CC-004 首发人工确认那一格。** 请求只指名申报单元；确认人取信封里认证出的操作者（「发行方#sub」），所凭授予取本口的决定种类，授权依据写成「`CUSTOMS_SUBMISSION_AUTHORIZATION`@确认人」，授予时刻取服务端时钟。单元当前就绪须有效：就绪没登答未决并指名 `READINESS_UNCONFIGURED`，就绪已失效答 `NOT_READY`，都不形成授权。授权落进既有提交授权册（一单元一行）；册上已有这个单元的授权时不另形成、不顶替——同一确认的重放与另一位有权角色的第二次确认都答 `AUTHORIZATION_EXISTING` 并交回原确认人，原授权已失效答 `AUTHORIZATION_NO_LONGER_EFFECTIVE`。授权确认不形成提交版本或尝试，也不入队（今天没有消费者）。提交版本固定时把授权依据一并快照，申报版本因此留得住确认人与授权依据。

**四、提交执行口与「已登记待发送」。** 请求给单元、所属案件、程序、组成、资料与角色快照引用、发送目标；编排照既有提交链走：案件反查核存在、就绪与授权两轨各自把门（授权没登答未决并指名 `AUTHORITY_UNCONFIGURED`）、单元落册、固定不可覆盖的提交版本、形成首次提交尝试、交提交版本形成意图。提交执行口不发送任何东西：首次尝试一律记为新的一格「已登记待发送」（`REGISTERED_PENDING_SEND`），没有发送时间；请求体不收尝试结果与发送时间——让请求方说「已回执」「何时发出」就是替一次没发生的发送作证，沿用「待确认」也不行，它说的是「可能已经发出去了」。迁移 `customs_compliance/0025` 放开结果集合、让 `sent_at` 可空并与结果配对，受控重发（序号大于 1）本身是一次发送，不会停在这一格；领域与读回同一道门。越过发送边界（「尝试已发起」）的那一步归发送通道（票 product-strategy-boundary/10「申报发送通道」一项）。这一格今天只形成了 UC-CC-005「提交已登记待发送」的一半：尝试身份与目标登记了，授权使用关系没有建模，提交版本形成信封只有 VE 投影消费、不是发送意图——今天的「已登记待发送」不含发送意图（越权风险点 7、10）。

**五、生产进程没有自动提交路径。** 建案、授权确认与提交执行三只编排在 `cmd/` 下只由 parcel-api 运营决定三口的装配文件构造；派发进程不订阅任何会触发它们的信封，接受决定的扇出照旧只到 VE 与初始路由。两件事由用例守住：真库上一份委托被接受、接受信封走过生产派发图之后关务各册与两类关务意图仍为零；扫 `cmd/` 全部生产源码，上列构造函数只出现在那一个装配文件里。

**六、入口形态。** 三口直接挂操作者渠道，合成租户 `SYN-TENANT-01` 与真实租户走同一条命令面，不加隔离放行（ADR-0150 决定三：写开关换不了运营决定口）。票面「隔离形态下照 operator-channel/08 放行」那一半不采用：ADR-0149 的前线操作者族已落地，再开一条只收 `SYN-` 的隔离写口就是 ADR-0150 要退场的那种形态。

**七、分工与不裁的。**

- 内置立案（消费接受决定或已采用路由、派生要建的案件）不在本记录，另立票 product-strategy-boundary/35，作产品策略。立案不等于申报，内置立案与「不启用生产自动申报」不冲突；不在本记录是因为接受决定不带监管程序与法定义务范围，派生要一套「已采用路由的关务段 → 四维」的判断方法，那是新的产品策略。与 routing-first-cut/12 的分工：关务适用性判断是路由判断的输入，不触发建案，也不被建案当依据复用；建案要不要建只由建案要求规则册答。
- UC-CC-004 其余各格（拒绝、撤销、待人工、再次申请）、授权按使用建模（原案内更正、补充开生产口之前必须先有）、授权目标绑定到组成 / 资料 / 渠道、授予撤销要不要连带未使用的授权失效、受理口核单元组成都在所属案件里、运维登记授权一路的去留，另立票 product-strategy-boundary/36。
- 申报发送连接器、监管结果代码与层次映射仍归票 product-strategy-boundary/10。

**八、首发对「未登记」一格的答法。** 授权确认时单元的就绪判断没登、提交执行时单元的提交授权没登，首发都答未决并指名未配置（`READINESS_UNCONFIGURED`、`AUTHORITY_UNCONFIGURED`），已失效的答业务负向（`NOT_READY`、`NOT_AUTHORIZED`）。这替代 UC-CC-004 `AT-CC-105`「请求未受理（授权前提不成立）」在「不存在」「已经失效」两格、UC-CC-005 `AT-CC-110`「提交前门禁已阻断」在「不存在」一格上的结果名（110 的已撤销照旧是门禁阻断那一类，答 `NOT_AUTHORIZED`）：就绪与授权两本册的内容是租户取值，没登是租户还没配，续办是去登记，与「请求本身不成立」「门禁明确不成立」的续办不同（ADR-0029 按恢复动作分格）；提交编排早已这样答，确认口跟它同一口径。两行的正文不改，行旁注明首发答法见 `AT-CC-444` / `AT-CC-439` 与本条。

## 候选与反方

**甲 · 一口一种决定，一次请求先形成授权再提交。** UC-CC-005 允许界面连续编排授权与提交。反方：CONTEXT 要求批准与实际提交权限分别表达，合成一口一种授予，持它的人就同时有了两种权；界面要连续编排，两口两种授予照样能连着调，同一人持两种是租户取值。否决。

**乙 · 提交授权继续只走运维 CLI，命令面只给提交执行。** 反方：CLI 一路的依据是运维边界，不是业务角色的明确确认；首发范围要求申报版本留得住确认人。否决（CLI 一路保留为既有治理工具，越权风险点 1）。

**丙 · 首次尝试沿用「待确认」，发送时间取服务端此刻。** 反方：待确认是「可能已发出、结果不明」，拿它记一份没发出去的尝试就是假装已发送；服务端此刻是编出来的发送时间（ADR-0023 同一纪律）。否决。

**丁 · 只固定提交版本，不形成尝试，尝试留给连接器。** 反方：UC-CC-005「已登记」那一格要求待发起尝试身份与发送意图在同一边界形成；提交册、读面与重放都按版本与尝试成对落，拆开要改提交链全部读写。否决。

**戊 · 建案请求体带客户归属与来源引用。** 反方：操作者自报客户归属能把另一客户的包裹挂进案件；UC-CC-001 要求只有已接受委托里的包裹才能建正式案件，服务端去查的同时就核了这一条（与 ADR-0151 决定二「委托寻址」同一纪律）。否决。

**己 · 本记录同时做内置立案。** 反方见决定七。否决。

**庚 · 照 operator-channel/08 给三口加隔离放行。** 反方见决定六。否决。

## Consequences

- 生产进程有了建案并形成提交版本的路：持三种授予的操作者经三口建案、确认授权、发起提交，提交版本与「已登记待发送」的首次尝试落库，两类意图由 VE 投影消费。cmd/parcel-api 的真库用例经生产装配与真操作者册证三口的答复格；2026-10-21 在一次性库上按 `seed.sh` 灌入，起 `parcel-api` 与 `parcel-dispatch`，演示租户一票委托经首方渠道提交、被接受后，`declare-one-parcel.sh` 用配置员的 Dex 令牌走通三口，重跑三口各答已有结果（实测，只记 `S`）。票 product-strategy-boundary/05 格 9 越过；格 11 的外部结果口此后有提交可归属，那一格的重走归票 05。
- 迁移 `access_identity/0018`（决定种类 CHECK）、`customs_compliance/0025`（尝试结果与发送时间配对）。
- 关务 CONTEXT「提交尝试」词条与「提交与外部监管结果」生命周期首条、GLOSSARY「申报提交尝试」随本记录改；UC-CC-001、UC-CC-004、UC-CC-005 验收示例加 `AT-CC-437`–`445`，`AT-CC-105`、`AT-CC-110` 两行旁注首发答法（决定八）。
- 演示种子：配置员加三种授予，准入对照与权威区间各三行；演示申报单元只登就绪（步 `460`），授权走确认口。
- 立票 product-strategy-boundary/35、36。

## 越权风险点

1. **运维登记授权一路仍被提交执行认**（CC owner）。`parcel-customs-register authority-grant` 写进的授权依据不是业务角色确认，提交执行照样认它，生产租户若用它就绕过了确认人留痕；演示种子里单元 01、02 的授权就是这样登的（案件页靠它们显示撤销态）。本记录不停它，去留归票 36。
2. **决定种类与授予粒度**（PC owner，票 07）。三种是按 CONTEXT「分别表达」取的最小粒度；请求、拒绝、撤销要不要各成一种，职责分离（确认人不得是提交人）要不要成为产品策略，归角色模型。本记录不强制职责分离。授予之外 party-commercial 的角色、委派与渠道账号使用授权（UC-CC-004 步 4）今天不问——决定种类授予就是全部的授权门，与 ADR-0204、ADR-0212 同形。
3. **授权只绑申报单元**（CC owner）。UC-CC-004 要求授权绑案件、组成、资料、拟提交动作与指定渠道；提交授权册一单元一行，组成、资料与渠道在提交那一刻才进提交版本，确认之后换组成或资料，授权照样支持提交。票 36。
4. **受理口不核单元组成是否都在所属案件里**（CC owner）。UC-CC-002 组单元那一步没有生产入口，提交执行照既有编排把组成当请求内容收，操作者可以把案件外的包裹放进单元。票 36。
5. **提交执行人不留痕**（CC owner）。提交版本的授权依据带确认人，尝试不带发起人；UC-CC-005 审计要求的「实际执行主体」今天只在接入日志里。票 36。
6. **授权依据不带授予号**（PC owner）。操作者信封不交出授予标识，依据只写「所凭决定种类@确认人」，确认时刻在授权行上；要查是哪一笔授予，按（确认人、种类、时刻）回读操作者册。票 36。
7. **「已登记待发送」只形成了一半，与连接器的分工待裁**（CC owner，票 10）。UC-CC-005 的定义是授权使用、待发起尝试身份与持久化发送意图都已形成；今天只有尝试身份与目标，没有授权使用关系（见 10），唯一的意图 `customs-compliance.declaration-submission.formed` 只有 VE 投影消费、不是发送意图——今天的「已登记待发送」不含发送意图。连接器接上时要么自带发送意图（随提交版本同笔入队一封给它的信），要么扫待发送的尝试行；越过边界时是推进这一份尝试的状态、还是另记一笔发送事实，尝试行今天没有 UPDATE，都归票 10 先裁。外部结果口若在尝试仍待发送时收到回执（例如报关服务商线下代发），编排按版本归属照收，不判这一格。
8. **建案的四维与初始角色照请求收**（CC owner、PC owner）。UC-CC-001 要求角色资格逐项来自 party-commercial 的版本化来源、程序与义务来自适用规则；本记录只核包裹的接受与建案要求规则，四维与角色不核。内置立案（票 35）若做，要回答四维从哪来。
9. **授权确认与提交执行的未决整笔回滚**（CC owner）。装配点在编排答未决时回滚整笔，提交执行在单元落册之后才停的那几格（版本标识签不出、提交册写不进）因此不留单元行，与编排注释「单元行落了重试续上」那一句的行为不同：重试从头再来，结果相同。
10. **授权使用没有建模，更正与补充不得在票 36 之前开生产口**（CC owner）。提交授权册一单元一行，提交版本只快照授权依据、不形成「这份授权已被哪个逻辑提交用掉」的关系。原案内更正、补充的编排（`CorrectDeclarationHandler`）按同一单元读的就是首报用掉的那一份授权，确认口对新的确认也只答 `AUTHORIZATION_EXISTING`——首发范围「每次更正、补充都必须由业务角色明确确认」与 UC-CC-005 `AT-CC-137`（后续动作各自取得授权）一旦接线就会被绕过。今天更正、补充没有生产入口，不违反；票 36 把授权按使用建模之前，不得给它们开生产口。
11. **确认之后确认人的授予被撤销，授权照旧有效**（CC owner、PC owner）。授权确认在确认那一刻核确认人持有授权确认授予（UC-CC-004「决定时重新校验」），之后授予过期或撤销不使已形成的授权失效，提交照旧通过。这是取舍不是疏漏：授权是一次具体决定，不是确认人此后一直在场的状态；要不要让授予撤销连带使未使用的授权失效（UC-CC-004「新规则明确适用时才形成不再有效」），归票 36。

## Links

- [ADR-0151](./0151-unassigned-command-faces-get-their-families.md)：运营决定能力面、决定种类授予与准入（决定一、三）；本记录追加三种决定，不改其条款
- [ADR-0149](./0149-business-command-faces-split-into-frontline-operator-and-integration-client-families.md)：前线操作者族与准入范围（决定四）
- [ADR-0150](./0150-synthetic-tenant-is-treated-as-a-real-tenant-and-isolated-form-retires-per-face.md)：合成租户按真实租户、写开关换不了运营决定口（决定三）
- [ADR-0091](./0091-isolated-form-extends-to-the-write-path-by-graded-switches.md)：隔离形态逐口放行（本记录不采用）
- [ADR-0146](./0146-product-strategy-is-a-third-class-between-mechanism-and-tenant-values.md)：机制、产品策略与租户取值三分
- [ADR-0204](./0204-a-fulfillment-segment-adopts-a-transport-commission-by-declaration-and-a-started-commission-cannot-be-cancelled.md)、[ADR-0212](./0212-a-customer-number-hit-needs-a-verifiable-delivery-scope.md)：新运营决定口的先例
- [ADR-0222](./0222-composition-roots-register-endpoints-and-routes-per-feature-file.md)：三口登在 `cmd/parcel-api/endpoints_customs_decisions.go`
- [ADR-0025](./0025-cross-context-adapters-live-on-the-consumer-side.md)：按包裹查已接受委托的适配器在消费方
- [ADR-0023](./0023-work-fact-identity-and-time-are-minted-by-the-device.md)：服务端不代铸外部事实的时间
- [ADR-0073](./0073-declaration-unit-is-a-persisted-aggregate-holding-its-case.md)：申报单元持有案件维
- [UC-CC-001](../application/customs-compliance/UC-CC-001-ESTABLISH-CUSTOMS-CASE.md)：`AT-CC-437`、`438`；[UC-CC-004](../application/customs-compliance/UC-CC-004-AUTHORIZE-DECLARATION-SUBMISSION.md)：`AT-CC-440`、`442`、`444`；[UC-CC-005](../application/customs-compliance/UC-CC-005-FREEZE-SUBMISSION-AND-INITIATE-ATTEMPT.md)：`AT-CC-439`、`441`、`443`、`445`
- 票 [product-strategy-boundary/18](../../.scratch/product-strategy-boundary/issues/18-customs-case-and-declaration-submission-entry.md)；后继票 [35](../../.scratch/product-strategy-boundary/issues/35-built-in-customs-case-trigger-from-acceptance.md)（内置立案）、[36](../../.scratch/product-strategy-boundary/issues/36-submission-authorization-business-role-face-remainder.md)（授权业务角色面其余各格）；相邻 [10](../../.scratch/product-strategy-boundary/issues/10-cc-declaration-channel-and-public-regulatory-reference-configuration.md)「申报发送通道」一项（发送通道）、[routing-first-cut/12](../../.scratch/routing-first-cut/issues/12-cc-customs-applicability-judgment-for-route-candidates.md)（关务适用性判断口）
- [ADR-0243](./0243-customs-first-release-answers-replace-the-uc-cells-they-answer-and-unavailable-dependencies-answer-undecided.md)：部分停用决定八两句——停「两行的正文不改」一句，把「这替代……的结果名」一句扩到 `AT-CC-109`「不存在」一格；授权确认、提交执行与 UC-CC-009 两只核对编排读不动、写不进答未决并指名那一口
