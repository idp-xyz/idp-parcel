# ADR-0295：取消先后冲突由运营决定解开——认定取消依据不成立（那份取消不是有效取消，被它挡住的来源重新进入原编排）或认定边界事实有误交来源方更正（取消保持有效，这一代边界事实不采用），只追加留痕，授权是「运营决定」族新立的一种

Status: Accepted（2026-10-26，Claude Code 会话代 PS owner 裁定：协调方据用户「你作为业务和系统专家自决」授权，把票 [docs-consistency/code/76](../../.scratch/docs-consistency/code/76-a-cancellation-order-conflict-has-no-resolution-decision-face.md) 定级 R2 并派作者分诊；作者裁定决定的去向集合、每个去向驱动的重判、授权形状，以及「取消依据不成立之后取消在客户侧与委托汇总里怎么读」。后一问选**不立新的取消状态**：取消记录保留作历史，由「有效取消」这一读法表达，理由见「候选与反方」；客户侧已收到的「包裹已取消」与下游已释放的东西怎么改口，属收寄后服务处置（票 code/37），本记录只交出口。裁决能力边界：读过票 code/76 全文、[ADR-0254](./0254-cancellation-and-boundary-fact-order-is-judged-by-their-business-times-and-a-late-earlier-boundary-fact-is-a-fact-conflict.md) 全文、冲突册与三条被挡住的消费链（收寄采用、面单渠道终局判断、终局采用）的代码、既有的释放拒收处理面（[ADR-0293](./0293-a-refused-control-release-is-handled-by-releasing-at-settlement-or-marking-handled-and-the-handling-resumes-the-acceptance-chain.md)）；没有读过客户端 API 契约与下游追踪投影对取消事实的消费细节。部分停用（接上）ADR-0254 决定三两句，被停原句与适用场景见「对已接受记录的作用」；正文不改，只在其 Status 行与 Links 加前向指针。越权风险点见文末，待 PS owner 复核，知会 TF owner、VE owner。）<!-- 索引注记 -->｜**去向封闭两个**：认定取消依据不成立（边界事实成立，取消提出时包裹其实已越过取消边界）；认定边界事实有误、交来源方更正（取消保持有效）。**效果**：前者让那份取消不是有效取消——有效取消读排除它，取消、收寄采用、终局采用、面单终局判断、委托完成派生与节点作业的服务结果读都读有效取消，一处排除各处一致；取消记录原样保留；依据它的终局不采用行同笔作废；同笔交一封续办信封，把被这份取消挡住的来源重新交回原来的编排。后者不改取消也不改任何来源事实，只让这一代边界事实在收寄采用处不采用、在面单终局判断处答取消在先，不停在冲突上。**留痕与授权**：决定只追加，主键同冲突，一份冲突至多一个决定，并发只一笔成立；「运营决定」族新立 `CANCELLATION_ORDER_CONFLICT_RESOLUTION`，不与授权处置、释放拒收处理共用，谁持有是租户取值，未登记答未配置；按租户查阅未解开的冲突。**续办**：回放原料随冲突入账与不采用入账同笔落下，续办按当前已接受委托取目标；面单渠道服务按 TF 首次有效收寄链尾现读｜**部分停用**（2026-10-26）：ADR-0254 决定三「解开它的决定由有权主体形成，形状本记录不定。」与「今天没有这个决定面。冲突停着、交人，冲突原因按包裹查得出。决定面、入账的恢复动作与面单渠道那一半的判法由 code/35 承接。」两句，已由本记录接上——决定面的形状（谁来解、去向、各去向的效果）由本记录定。**适用场景**：只限形状与决定面这两句；决定一、二与决定三其余各句（系统不自动解开、去向的两例）不变，入账与面单渠道一半的判法仍是 code/35 已落地的样子。**回答**：ADR-0254 越权风险点 2（冲突没有解开的决定面）由本记录回答
Date: 2026-10-26

## Context

- [ADR-0254](./0254-cancellation-and-boundary-fact-order-is-judged-by-their-business-times-and-a-late-earlier-boundary-fact-is-a-fact-conflict.md) 决定三说冲突「只由一份明确的业务决定解开」，却有意没定决定的形状，也没有给出决定面。票 code/35 落地了判法与入账：冲突记进 `parcel_shipment.cancellation_order_conflict`，收寄采用答 `ELIGIBILITY_UNDECIDED`/`CANCELLATION_ORDER_CONFLICT`、面单渠道终局判断答 `LABEL_SERVICE_NOT_FINAL`/`CANCELLATION_ORDER_CONFLICT`，消费门入账不回滚重投。
- 此后冲突包裹卡死：网络服务一侧没有责任起点，也不触发收寄后的路由复核，实物仍在节点手里；面单渠道服务一侧没有终局，客户手里是`包裹已取消`，承运商却已收走。网络服务履约结果（有效交付）在取消在册时到达，终局采用答 `SOURCE_NOT_ADOPTED`、依据 `PARCEL_CANCELLATION/<取消>`，那条不采用记录按（包裹，结果种类，来源版本）永久保全——冲突解开后同一版本重投只会读回它，包裹会一直没有终局（票 code/98 评审 N1）。
- 取消授权在生产未配置，取消形成不了，冲突还到不了；租户一登记取消授权即生效，所以决定面要在那之前就位，但不阻塞首发。
- CONTEXT 的取消边界一条写明边界越过之后包裹不得回退为`已取消`，取消记录又是只追加的已提交决定。冲突的两种真相各自要求不同的落地：取消提出时包裹其实已越过边界（边界事实对），或边界事实本身有误（边界事实错）。

## Decision

**一、冲突由一份明确的决定解开；决定只追加，不改冲突行，也不改写已提交的取消。**

- 决定挂在册上一份冲突上，主键就是冲突的主键（租户，包裹，边界种类，边界那一代，取消）。一份冲突至多一个决定；两人同时决定，主键相撞，只有一笔成立，败者读回先到的那一份、不交第二封续办信封。决定记决定人（认证出的操作者，格式 `发行方#sub`）、去向、依据（必填）与决定时刻。「未解开」不是冲突行上的状态列，是没有对应的决定行。
- 运营按租户查阅未解开的冲突与全部计数（`GET /cancellation-order-conflicts`，操作者主数据与运营查阅读面）；决定走 `POST /cancellation-order-conflict-decisions`。查阅逐字段给出决定命令要的几段与两边的业务时间，让决定人看得到谁在先、差多少。
- 册上没有这份冲突、冲突属于别的租户、被指名的几段对不上，同一个答案（统一不可见）。

**二、授权：「运营决定」族新立 `CANCELLATION_ORDER_CONFLICT_RESOLUTION`，租户取值，未登记答未配置。**

- 它与授权处置（`AUTHORIZED_DISPOSITION`）、释放拒收处理（`CONTROL_RELEASE_REFUSAL_HANDLING`）分开授予：认错一份冲突会让一件已取消的包裹长出责任起点，或让一件已在网络手里的包裹挂着取消终局，职责与前两者不同。谁持有这个授予是租户取值，在操作者册里登记；与「谁能提取消」的取消授权目录（PAR-COM-14）是两回事。
- 租户未登记任何授予时没有人能解开冲突，冲突停在未决、交人跟进——不默认任何角色可解开。租户与决定人只取自认证结果，不从请求体收。

**三、去向封闭两个，各自的落地不同。**

1. **认定取消依据不成立**（`CANCELLATION_NOT_UPHELD`）。边界事实对：边界事实成立，取消提出时包裹其实已越过取消边界。
   - 那份取消不是**有效取消**。取消记录原样保留作历史；有效取消读（`ParcelCancellationView`）排除它，所以取消编排的边界核验、收寄采用、终局采用、面单渠道终局判断、委托完成派生与节点作业的服务结果读都不把它读成取消——一处排除，各处一致，不必在每个读方各写一遍。同包裹后一份有效取消补上；「先合法形成者赢」在有效取消之间比。
   - 包裹因此是越过取消边界、未取消的包裹：之后的取消请求走收寄后服务处置（取消编排答 `DISPOSITION_PENDING`），委托汇总不把它算作已取消。不立新的取消状态，不改 CONTEXT 的生命周期。
   - 依据这份取消的终局不采用行（`PARCEL_CANCELLATION/<取消>`）同笔作废；其余依据的不采用行与已形成的终局一概不碰。
   - 同笔交一封续办信封（决定四）。
2. **认定边界事实有误，交来源方更正**（`BOUNDARY_FACT_REFERRED`）。边界事实错：本上下文不改任何来源事实。
   - 取消保持有效，不作废任何记录，没有续办信封。
   - 这一代边界事实不停在冲突上：收寄采用不采用它（`SOURCE_NOT_ADOPTED`，依据 `BOUNDARY_FACT_REFERRED/<取消>`），面单渠道终局判断答取消在先（`CANCELLATION_STANDS`）。来源方更正出新的一代按 [ADR-0254](./0254-cancellation-and-boundary-fact-order-is-judged-by-their-business-times-and-a-late-earlier-boundary-fact-is-a-fact-conflict.md) 决定二重判，又冲突则另起一行、另待一份决定。
   - 一份冲突的决定不可改：来源方回复「事实无误」时，不是改这份决定，而是等它以新的一代再到、再判出冲突、再决定。

**四、认定取消不成立之后，被这份取消挡住的来源重新进入原来的编排（续办）。**

- 决定同笔交一封 `parcel-shipment.parcel-cancellation.not-upheld` 信封，信封 ID 由冲突的身份认领，一份冲突至多一封；分区取冲突的全部维（一份冲突一条队），不与同包裹的取消、采用、终局信封排队——续办读的是库里已落定的状态，不依赖队里在前的信封，同队会让一封因下游未决而停在队头的同包裹信封把续办堵在后面。
- 被挡住的来源怎么重新交回：**回放原料**随挡住它的记录同笔落下——收寄采用判出冲突时带来源规格（对象、地点、控制依据、版本、发生时间、自报更正的那一版），终局采用因取消不采用时带责任结果规格。目标委托不存当时的，取回放那一刻的当前已接受委托。面单渠道服务的边界事实是 TF 判过的首次有效收寄，本上下文不存副本，续办按 TF 首次有效收寄链尾现读：已形成才交面单终局判断，没有或待确认不判。
- 续办进来先取包裹取消边界锁，再逐一把原料交回收寄采用、终局采用、面单终局判断；每条链的结果经它自己消费门已定的译表入账或回滚——未决（目标委托还没落到已接受、规则未配置）回滚重投，不在这里另立一套口径。重驱幂等：已形成的答已有结果。
- 「客户侧那份`包裹已取消`怎么改口、下游已释放的东西怎么补回」属收寄后服务处置（票 code/37），本记录只交出口：续办信封类型稳定、可订阅。

**五、锁序：决定与续办都只取包裹取消边界锁 P。**

- 决定编排的次序：P → 读册 → 决定留痕 → 作废不采用行 → 续办信封，整笔在一个事务里，返回错误即整笔回滚。续办进来同样先取 P，各条链进来再取是重入。P 之后不取别的锁，不更新委托行，不取接受链的作用域锁（S）与委托行锁（R），所以与取消、采用、终局判断各路之间、与持 S、R 的各路之间都不成环，与 `ports.CancellationBoundaryLock` 与 `ports.PreAcceptanceControlScopeLock` 头注的全序一致。
- 决定要取 P 的理由：它改的是「这件包裹有没有有效取消」，取消、采用、终局判断正是在 P 里读它；不取则一条链在锁里读到旧状态、在决定提交之后写下依据旧取消的不采用行，作废就漏了它。

## 候选与反方

- **取消失效作为取消状态机里的新一格**（`已取消` → `已失效`）。否决：取消是只追加的已提交决定，改状态等于改写它；而且「谁在什么时候认定它失效」是一份独立的业务决定，应有自己的留痕、授权与并发规则，而不是取消行上的一列。有效取消读让所有读方一处排除，状态机不动，CONTEXT 生命周期不动。
- **只记决定，不交续办**（把重驱留给来源重投）。否决：冲突入账时消费门已把信封标为已处理，没有谁会重投；有效交付的不采用记录又会原样返回——决定了也没有责任起点、没有终局。
- **续办从来源方重读，不存回放原料**。否决：节点收寄经来源编号取回、场外揽收经对象与尝试取回、有效交付经对象与尝试取回，三种引用各不相同，冲突行也不带它们；存原料是同笔落下的几个字符串，重读则要在续办里复刻三套消费方适配器。面单渠道一侧不存，因为边界事实本就是 TF 的、链尾可现读。
- **认定取消不成立后删掉依据它的冲突行与取消行**。否决：删除即失去审计，冲突册与取消记录只追加；决定行就是冲突被解开的唯一表达。
- **把两个去向合成一个「解开」**。否决：两个去向的效果相反——一个让取消失效，一个让边界事实不采用；合成一个会让决定人事后读不出自己解开的是哪一边。
- **自动解开**（例如按渠道声明时间可信度）。否决：ADR-0254 决定三已否决；自动认取消等于让一件已交到网络手里的包裹挂着取消终局，自动认收寄等于静默撤掉一个已提交的决定。
- **续办信封与同包裹的取消、终局信封同队**。否决：理由见决定四。同队的好处是续办不越过还没投出去的同包裹信封，但续办读库里已落定的状态，不需要这个次序，坏处却是实在的（本记录实现时在真链上复现过：同包裹上终局不采用意图停在下游消费的未决上，同队的续办信封被堵在它后面）。

## 对已接受记录的作用

- **部分停用（接上）** [ADR-0254](./0254-cancellation-and-boundary-fact-order-is-judged-by-their-business-times-and-a-late-earlier-boundary-fact-is-a-fact-conflict.md) 决定三两句：「解开它的决定由有权主体形成，形状本记录不定。」与「今天没有这个决定面。冲突停着、交人，冲突原因按包裹查得出。决定面、入账的恢复动作与面单渠道那一半的判法由 code/35 承接。」**适用场景**：只限决定面的形状（谁来解、去向、各去向的效果）与「没有这个决定面」这个现状；决定三其余各句（系统不自动解开、两个去向的例子）与决定一、二不变，入账的恢复动作与面单渠道一半的判法仍是 code/35 已落地的样子。
- **回答** ADR-0254 越权风险点 2（冲突没有解开的决定面）：决定面、查阅与续办就位；缓解句里「首发前只有演示租户」不是唯一的依靠。越权风险点 1、3、4、5 不变。
- **读法**（不是停用）ADR-0254 决定一「取消先提交，不悄悄撤掉已提交的决定」：认定取消不成立是一份明确的、留痕的、有授权的业务决定，不是悄悄撤；取消记录原样在，只是不被读成有效取消。CONTEXT「包裹不得回退为已取消」同理：包裹没有回退，是那份取消从未在边界之前成立。
- **不停用** [ADR-0293](./0293-a-refused-control-release-is-handled-by-releasing-at-settlement-or-marking-handled-and-the-handling-resumes-the-acceptance-chain.md)：本记录的授权与续办是同一族写法的又一个实例，不改它的任何一句。**不停用** [ADR-0094](./0094-undecided-retry-is-decided-by-resume-path-with-a-fourth-grade-for-operator-registration.md) 决定四：续办信封是「触发与格同笔落地」的又一个实例。**不停用** [ADR-0029](./0029-retrieval-failure-algebra-is-partitioned-by-recovery-action.md)：决定读不动是依赖未决（回滚重投），未决的冲突等业务决定（入账）——两格分开。

## Consequences

- PS CONTEXT 取消一条补冲突的解开（两个去向与有效取消），UC-PS-006 决定矩阵「事实冲突」一行、规则与验收表补 `AT-PS-166` 至 `AT-PS-173`，UC-PS-003、UC-PS-004、UC-PS-006 的「给开发的交接」里指向 code/76 的几行改成已补。
- 代码（票 code/76）：迁移 `parcel_shipment/0032`（决定表、回放原料表）与 `access_identity/0026`（决定种类放开）；有效取消读排除被认定不成立的取消；收寄采用与面单终局判断在冲突那一格读决定（新依赖 `Decisions`，nil 与决定面未接入同义）；收寄采用的冲突行、终局采用的不采用行带回放原料；决定编排、查阅与续办门；`parcel-api` 两个新端点、`parcel-dispatch` 一条新路由与未决名单。
- 新读法的一个后果：同一请求键重放取消，`FindByKey` 仍读回原 `包裹已取消` 的答复（历史答复原样）；有效与否只看有效取消读。
- 管理台页与演示种子不随本票：授予与页面是租户配置面的事，沿 [ADR-0293](./0293-a-refused-control-release-is-handled-by-releasing-at-settlement-or-marking-handled-and-the-handling-resumes-the-acceptance-chain.md) 的先例另跟。

## 越权风险点

1. **去向集合封闭为两个、各自的效果由本记录定**（PS owner）。若 owner 认为还需要第三种（例如「取消成立但物流已不可追回，转异常案件」），那是另一个去向、另一份效果，不改这两个。缓解：去向词是封闭的，库内 CHECK 与领域解析各守一遍，加一个去向是一处迁移加一格。
2. **一人下决定即生效，没有二人复核**（PS owner）。认错会让已取消的包裹长出责任起点，或让已在网络手里的包裹挂着取消终局，且「认定取消不成立」同笔作废不采用行、放行被挡住的来源，之后撤不回。缓解：授予与授权处置、释放拒收处理分开；决定人与依据必填、留痕只追加可审；租户不登记授予就没有人能解开；决定前有查阅清单给出两边的业务时间。若 owner 要二人复核，是另一份决定面的写法（先提议再确认），不改本记录的去向与效果。
3. **认定取消不成立之后，客户侧已收到的`包裹已取消`与下游已释放的东西不会自动改口、补回**（PS owner，知会 TF owner、VE owner）。本记录只让 PS 一侧不把那份取消读成有效取消、让被挡住的来源重新进入编排，并交出续办信封；已发给下游的取消释放意图、下游投影里依据这份取消形成的事实、客户通知，由收寄后服务处置（票 code/37）承接。在它落地之前，这类包裹的客户可见状态可能与 PS 的有效取消读不一致。
4. **取消记录保留，取消重放读到历史答复**（PS owner）。同一请求键再提交取消，读回原取消记录，客户面看到的仍是`包裹已取消`；有效与否只看有效取消读。缓解：决定留痕与取消记录并存、可对照；客户面重放读的是历史答复，这是只追加的代价。
5. **续办只重驱「被这份取消挡住」的来源**（PS owner）。范围是冲突那一代来源与依据这份取消的终局不采用行；`CANCELLED_BEFORE_INTAKE/<取消>` 的收寄前取消不采用记录不重判——它们的来源发生晚于取消的业务时间，而责任起点已由被解开的冲突来源占用，重判也只会落 `RESPONSIBILITY_ALREADY_STARTED`。若冲突来源本身要不到责任起点（收寄资格未证明），这些记录是否也该重判，要 owner 复核。同一份取消名下另有别的代冲突时，决定一份不会连带另一份：另一份仍留在查阅清单里，由人再决定（认定不成立同样入一封续办）。
6. **面单渠道服务的续办按 TF 首次有效收寄链尾现读，不是冲突那一代**（PS owner，知会 TF owner）。链尾换了新一代的，以新一代重判；失效或待确认不判。冲突那一代的收寄若在续办前已被更正为不表达收寄（ADR-0135 决定六），续办就没有收寄可判，面单终局由它自己的关闭路径形成。
7. **交来源方更正不保证来源方一定更正**（PS owner）。旧一代的冲突被决定后即出清单；来源方不更正，这一代也不弹出，包裹停在取消在先。缓解：决定留痕指名依据；来源方确认无误时，同内容的新一代到达会再判出冲突、再进清单。
8. **`Decisions` 为 nil 与决定面未接入同义**（PS owner）。装配漏接时冲突入账等候，不认任何决定——不响亮；`cmd/parcel-dispatch` 的收寄采用装配有真链用例钉住，面单终局判断那一处的接线没有可区分的持久效果（交来源方更正与冲突都是`不形成`终局并入账），只有应用层用例和代码审读守着。
9. **一份冲突一条队，同包裹多份冲突的续办并发靠包裹取消边界锁排队**（PS owner）。分区登记表里这一口登的是「逐信封」。

## Links

- 票 `.scratch/docs-consistency/code/76-a-cancellation-order-conflict-has-no-resolution-decision-face.md`；出处票 `.scratch/docs-consistency/code/35-cancellation-and-boundary-fact-order-conflict-has-no-label-half-and-no-resolution-face.md`、`.scratch/docs-consistency/code/98-a-network-delivery-final-is-formed-over-a-standing-cancellation.md`
- [ADR-0254](./0254-cancellation-and-boundary-fact-order-is-judged-by-their-business-times-and-a-late-earlier-boundary-fact-is-a-fact-conflict.md)：部分停用（接上）决定三两句；回答越权风险点 2
- [ADR-0293](./0293-a-refused-control-release-is-handled-by-releasing-at-settlement-or-marking-handled-and-the-handling-resumes-the-acceptance-chain.md)：「运营决定」族新立决定种类、处理留痕只追加、同笔交续办信封的先例
- [ADR-0284](./0284-an-authorized-role-decides-a-current-reroute-suggestion-through-an-operator-decision-face.md)：「运营决定」族新立决定种类的先例
- [ADR-0151](./0151-unassigned-command-faces-get-their-families.md)：运营决定按租户与决定种类授予
- [ADR-0094](./0094-undecided-retry-is-decided-by-resume-path-with-a-fourth-grade-for-operator-registration.md)：决定四，续办触发与格同笔落地
- [ADR-0029](./0029-retrieval-failure-algebra-is-partitioned-by-recovery-action.md)：按恢复动作分格，读不动才是未决，等业务决定的冲突入账
- [ADR-0025](./0025-cross-context-adapters-live-on-the-consumer-side.md)：新增结果格后每个消费方有落点
- [ADR-0022](./0022-http-status-carries-answer-formed-not-business-verdict.md)：已形成的业务答案答 2xx，业务判别进响应体
- [ADR-0135](./0135-carrier-first-effective-pickup-is-a-judged-control-fact-with-its-own-registry-and-enters-the-segment.md)：承运商首次有效收寄的判定与失效版本
- [小包托运上下文](../domain/parcel-shipment/CONTEXT.md)：Rules 取消一节
- [UC-PS-006](../application/parcel-shipment/UC-PS-006-CANCEL-PARCEL-OR-COORDINATE-POST-INTAKE-DISPOSITION.md)：决定矩阵「事实冲突」、规则与 `AT-PS-166` 至 `AT-PS-173`
- [UC-PS-003](../application/parcel-shipment/UC-PS-003-ESTABLISH-NETWORK-INTAKE-AND-FORMAL-COMMITMENT.md)、[UC-PS-004](../application/parcel-shipment/UC-PS-004-FORM-PARCEL-FINAL-SERVICE-OUTCOME.md)：`AT-PS-107`、`AT-PS-108` 与各自的交接行
