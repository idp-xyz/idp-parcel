# ADR-0238：派送出发前的交付门禁问询——作业事实渠道上按对象与拟交付时点只读一问，交回与随后同参有效交付同一套读数；运输履约不落记录，关务照记判断

Status: Accepted（2026-10-21，Claude Code 会话代决：用户原话「你作为业务和系统专家自决……解决所有的问题」，2026-10-20 又答「要我裁定的部分，你作为业务和系统专家，请你自决吧」；票 demo-chain-gaps/94，代决复核报告 `.scratch/owner-review-20261020/report.md` 的 R14 把它列为上线前必修。回答 [ADR-0213](./0213-a-transport-handover-declares-its-releasing-receiving-and-onward-nodes.md) 文末票 66 补记越权风险点 20「读数挡不住交付」，照 [ADR-0219](./0219-a-device-previews-the-departure-gate-before-departing-and-reads-what-the-departure-will-record.md) 同形；回给设备的逐对象那一格照 [ADR-0237](./0237-gate-readings-show-the-executor-only-the-objects-that-did-not-pass.md)。不停用任何已接受记录的条款，只在 ADR-0213 的 Status 行与 Links、ADR-0206 的 Links 加前向指针。越权风险点见文末，待 TF owner 复核，第 4 点同时待 CC owner。）<!-- 索引注记 -->｜**已接受**（2026-10-21，用户原话「你作为业务和系统专家自决」，Claude Code 会话代 TF owner 裁定；票 demo-chain-gaps/94）。**问询口**：`POST /transport-fulfillment/delivery-gate-previews`，载荷只收对象与拟交付时点 `deliversAt`（封闭解码，时点缺席答未受理、不补服务端时钟），答复一律 200（`DELIVERY_GATE_PREVIEWED` 带 `gateReading`，或 `INPUT_NOT_ACCEPTED`），`gateReading` 与有效交付口同一线格式，逐对象那一格照 ADR-0237 只列没得出放行的那一件。按件问。**同参同读数**：问询与有效交付登记受理后收成同一组（租户、对象、时点）走同一条读数路径（at 取法、停点、问关务只在 ADR-0213 决定五与票 66 补记），对象的受理判据同一处；问询不读派送尝试，派送尝试读面那一格只属登记；同格只在两问之间段册、交接册与关务三源不变、且拟交付时点就是随后那件对象结果的发生时刻时成立。**不落记录**：运输履约不写册、不入队、不结束参与、不记谁问过，问询读数不交给随后的交付；关务照 ADR-0206 决定六记判断，同参交付命中同一判断。**授予**：与有效交付口同一只作业事实 Intake，不新增能力种类。**不挡交付**：派不派、交不交归一线客户端与现场；挂在设备派送出发之前，不挂在派送任务形成那一拍。回答 ADR-0213 票 66 补记越权风险点 20，无条款停用。越权风险点含读到放行不等于交付那一刻仍放行、用写授予开读口、客户端处置未定、关务判断册随问询增长（票 172）、按件问、任一场外设备可零成本探知任一件的关务状态与 at 且不留痕（票 171）、拟交付时点是预计、段按此刻取、CC 文档请求时机一句未补（票 165），待 TF owner 复核
Date: 2026-10-21

## Context

- 票 66 让有效交付登记时问交付门禁、照记读数（ADR-0213 决定五与票 66 补记）。读数是交付之后的记录：读到阻断时包裹已经在收件方手里。TF CONTEXT「揽收、派送与交付」写的是「挡下违规交付是派送出发之前那一问的事」，ADR-0213 票 66 补记越权风险点 20 把那一问另立了票 94。
- 派送这一侧今天没有那一问：派送任务形成（ADR-0114、0216）、派送尝试登记都不问关务，尾程设备也没有读口。出发那一侧由 ADR-0219 补上了出发前问询：作业事实渠道上的只读一问，交回与随后同参出发同一套读数，运输履约不落记录，关务照记判断，授予挂在移动事实登记上。
- 交付读数的读法（票 66 补记）：对象取此刻在场的那一段，at 取对象进这一段时那次`已交接`声明的交出节点，时点取交付的业务时间（派送尝试里那件对象结果的发生时刻）；取不到 at 时停在本侧，各停点具名。读数取共用内核，一份恰一件对象。写事务之前先读派送尝试（不知道交付的业务时间就停在「派送尝试读面读不动」）。
- 关务的动作准入判断（ADR-0206 决定六）在自己的事务里记判断，标识按内容派生、动作时点按微秒入标识：同一请求在同样三源上重放命中同一行。
- 代决复核 R14（2026-10-20）：不做则交付侧关务门禁只能事后记账，受限或被扣的包裹照样交到收件人手里；照 ADR-0219 同形，列为上线前必修。R11 / ADR-0237：回给设备的逐对象那一格只列没得出放行的对象，交付读数同此。

## Decision

**一、作业事实渠道开一口只读问询：`POST /transport-fulfillment/delivery-gate-previews`。**

- 输入两格：对象 `object`，拟交付时点 `deliversAt`。对象读法与有效交付登记的同名格逐字相同；拟交付时点对的是随后那次派送尝试里这件对象结果的发生时刻——交付登记按它问关务（票 66 补记「时点」）。
- 载荷只收这两格，按封闭形状解。尝试、方式、接收方、POD 都不决定读数；派送出发之前也还没有尝试。收进来就是在暗示它们算数。
- 拟交付时点缺席答未受理，不拿服务端时钟补（ADR-0023）：补了就与随后那次交付两样。
- 答复一律 200。问到了答 `DELIVERY_GATE_PREVIEWED` 并带 `gateReading`；受理不了答 `INPUT_NOT_ACCEPTED`，不带读数。读数阻断也是 200。不答 201：本上下文没有新落任何东西（ADR-0022）。
- `gateReading` 与有效交付口是同一个线格式：整格、at、停点、逐对象那一格；逐对象那一格照 ADR-0237 只列没得出放行的那一件，关务判断标识与依据原文不回给设备（ADR-0205 决定一）。
- 按件问。交付读数一份恰一件对象，同参同读数也是按件成立；一线设备在派送出发前逐件问它这一车要交的那几件。

**二、同参同读数：问询与有效交付登记走同一条读数路径。**

- 两口各自受理后，把输入收成同一组三样：租户、对象、时点，交同一个读数编排；装配上也只有一处构造。同格靠的是只有一条路，不是两边各算一遍再保证相同。
- 两口对对象的受理一致：同一处判据。at 怎么取、各停点、读不动的依据种类只在 ADR-0213 决定五与票 66 补记定义，本决定不复述。
- 问询不读派送尝试：时点由设备给。所以「派送尝试读面读不动」那一格只属登记，问询不会停在那里。
- 「同参」指对象相同、拟交付时点就是随后那件对象结果的发生时刻。读数还取决于两问之间的段册、交接册与关务三源：它们不变时同格；变了，交付读到的就是变了之后的那一份（越权风险点 1、8）。

**三、运输履约不落记录；关务照 ADR-0206 决定六记判断。**

- 运输履约这一侧不写任何册、不入队任何信封、不结束参与，也不记谁问过。
- 问询的读数不交给随后的交付。交付照 ADR-0213 决定五在它自己那一刻重问、读数与交付同一笔落册：读到放行不等于交到收件方手里那一刻仍放行。
- 关务那一侧：问询也是对明确对象与拟执行动作的请求，照决定六在自己的事务里记判断；随后同参的交付在同样三源上命中同一行，答`已有判断`。这补的是动作准入判断的请求时机，不改判断本身，与 ADR-0219 决定三同一理由。
- 问询不包事务，理由同 ADR-0219 决定三末条。

**四、持有效交付登记授予即可问，不新增能力种类。**

- 问询与有效交付口挂同一只作业事实 Intake：问的人就是随后登交付的那台设备，能登交付就能问派送出发前那一问。
- 没有授予答 403 `OPERATOR_NOT_GRANTED`；设备不在用答 403 `DEVICE_NOT_IN_USE`；作业范围不是场外答 400。各格与有效交付口逐字相同。

**五、问询不挡交付；挂在设备派送出发之前。**

- 运输履约不据问询读数拒收随后的交付，ADR-0213 决定五不变。派不派这一件、到了门口交不交，是一线客户端与现场的事（越权风险点 3）。
- 问询挂在设备派送出发之前，不挂在派送任务形成那一拍。任务形成是异步的一拍（TF CONTEXT「揽收、派送与交付」），离派送出发可能隔很久，那之后落下的限制它看不见；读数挂在任务上又会被读成派送许可，还得在任务上另立一份存储。设备出发前问，问的就是它此刻要带走的那几件。

三类：决定一、二、三是机制；决定四（授予挂在有效交付登记上）与决定五（不挡交付、挂在设备出发前）是产品策略（ADR-0146）；本记录没有租户取值，谁持作业事实登记授予仍由租户登。

## 候选与反方

- **派送任务形成那一拍也问一次、读数挂在任务上。** 否决：见决定五第二条。
- **一次问一批（一个派送任务或一车）。** 不取作本次形状：交付读数按件、同参同读数按件；批量问要另一种答复形状。日后按任务批量问是加法，逐件读数不变（越权风险点 5）。
- **为问询新立一种能力。** 否决：理由同 ADR-0219 候选第一条，会出现能登交付却问不了的设备。
- **运输履约落一本问询册。** 否决：理由同 ADR-0219；「当时凭什么这么答」由关务判断册答得出。
- **问询读数交给随后的交付，交付不再重问。** 否决：两问之间三源可能变，等于让设备自报门禁读数。
- **问询也带派送尝试、从尝试里取时点。** 否决：派送出发之前还没有尝试；那就成了交付之后才问得出的一问。
- **拟交付时点缺席取服务端此刻。** 否决：ADR-0023。
- **载荷接受整条交付载荷、多余键忽略。** 否决：封闭解码是本渠道的门（ADR-0219 候选同条）。
- **交付登记在读数阻断时拒收。** 否决：交付已经发生，ADR-0213 决定五不重开。

## Consequences

- 运输履约：
  - `application/read_delivery_gate.go`：读数路径收成一条 `readIntent`，交付登记与问询共用；问询的受理收成 `deliveryGateIntentOf`，对象判据与交付登记的键共用 `deliveredObjectFrom`（在 `register_effective_delivery.go`）。
  - 新 `application/preview_delivery_gate.go`：问询编排与结果代数（`DELIVERY_GATE_PREVIEWED` / `INPUT_NOT_ACCEPTED`）。
  - 新 `adapters/http/preview_delivery_gate.go`：Intake 形、载荷、端点与未配置答法；作业事实那一种答法在 `work_fact_intake.go`。
  - 不加端口、迁移与信封。
- 装配：新 `cmd/parcel-api/assemble_delivery_gate_preview.go`（读数编排取 `buildDeliveryGateReader`，与交付登记同一处构造）与 `endpoints_delivery_gate_preview.go`（ADR-0222 的特性文件），同名测试文件登未接线占位与作业事实探测载荷。
- 关务：代码不改。GLOSSARY「动作准入判断」所有者一条、CONTEXT-MAP `transport-fulfillment → customs-compliance` 各补一句；CC CONTEXT「动作准入判断」词条与 UC-CC-013 触发一格的请求时机一句没补（本票 CC 地盘只读），另立票 [demo-chain-gaps/165](../../.scratch/demo-chain-gaps/issues/165-customs-docs-name-the-delivery-gate-preview-as-a-request-timing.md)。ADR-0206 的 Links 加前向指针，正文不动。
- 文档：TF CONTEXT「揽收、派送与交付」加一条；GLOSSARY 加「交付前门禁问询」词条；UC-TF-006 规则一句与 `AT-TF-322`–`325`；ADR-0213 Status 与 Links 加指针；演示脚本补记。
- 演示（证据 `S`）：新加坡枢纽交给尾程之后、派送出发之前，尾程设备逐件问；演示包裹答 200、读数 `NOT_GUARDED`、`at` 为 `SYN-NODE-SIN-HUB`、没有逐对象那一格；随后同参交付答 201，`gateReading` 与问询逐字节相同。

## 越权风险点

1. **读到放行不等于交付那一刻仍放行**（TF owner）。两问之间限制可能落下、放行可能撤回，交付照它自己那一刻重问。设备若把问询当许可，读数变了之后照样会交；那一次只能靠交付读数里的阻断事后发现（阻断信号另票 93）。
2. **用写授予开读口**（TF owner、access-identity owner）。同 ADR-0219 越权风险点 2。
3. **问询不挡交付，挡不挡归一线客户端**（TF owner、一线客户端 owner）。本仓今天没有一线客户端代码；客户端读到阻断怎么处置（不带走、带走不交、转人工、找谁放行）未定。不问就派送的设备照样登交付（ADR-0213 决定五）。
4. **问询在关务判断册上留行**（CC owner、运维）。同 ADR-0219 越权风险点 4：判断标识含拟交付时点（微秒），设备按「此刻」轮询，每问一次每件多一行；恶意设备可以让这本册无限增长，没有限流与保留期，也分不出问询那几行与动作时的判断。另立票 [demo-chain-gaps/172](../../.scratch/demo-chain-gaps/issues/172-customs-judgement-register-grows-with-every-gate-preview.md)。
5. **按件问**（TF owner、一线客户端 owner）。一车一百件就是一百问、一百行判断（判断册的增长归票 172）。按派送任务或按车批量问另议，逐件读数不变。
6. **任一件的关务状态可以零成本探知**（TF owner）。持任一 OFFSITE 作业范围授予的设备可以对本租户任一件包裹问，拿到它此刻的交付读数——是否被扣、at 是哪个节点、或停在哪一种本侧停点（不在任何段里、凭收寄进段……），运输履约这一侧不留痕（关务判断册记了对象与时点，不记谁问的）。ADR-0237 只收窄逐对象那一格，整格就是被问那一件的答案。今天派送任务上没有执行方或作业范围归属，「只问得到自己派送任务上那几件」现在收不了；两种问询一起按作业范围或派送指派限对象、并在本上下文留痕，另立票 [demo-chain-gaps/171](../../.scratch/demo-chain-gaps/issues/171-gate-previews-are-scoped-to-the-asking-devices-work-and-leave-a-trace.md)。
7. **不记谁问过**（TF owner、VE owner）。同 ADR-0219 越权风险点 6；留痕随票 171。
8. **拟交付时点是预计，段按此刻取**（TF owner）。交付登记按派送尝试里那件对象结果的实际发生时刻问，问询按设备给的拟交付时点问；两者不同（预计几乎总与实际不同）时，关务判断标识不同、各记一行，问询那一行不被交付命中，限制以生效时点判，时点之间落下的限制只在交付那一问看得见。at 所在的段两边都按「此刻」找（票 66 补记「该派送段」），不按拟交付时点：派送出发之前对象若还没进尾程段，问询答的是它此刻所在那一段的入口节点，或停在「此刻不在任何段里」；设备应在接过包裹之后再问。
9. **CC 文档的请求时机一句没补**（CC owner）。CC CONTEXT「动作准入判断」词条与 UC-CC-013 触发一格只写了出发前问询；本票 CC 地盘只读，另立票 165 补一句。GLOSSARY 与 CONTEXT-MAP 那两处是共用文档，已补。

## Links

- [ADR-0213](./0213-a-transport-handover-declares-its-releasing-receiving-and-onward-nodes.md)：决定五与文末票 66 补记（交付读数的读法、时点、停点）；补记越权风险点 20 由本记录回答
- [ADR-0219](./0219-a-device-previews-the-departure-gate-before-departing-and-reads-what-the-departure-will-record.md)：同形先例——设备出发前门禁问询
- [ADR-0237](./0237-gate-readings-show-the-executor-only-the-objects-that-did-not-pass.md)：回给设备的逐对象那一格只列没得出放行的对象
- [ADR-0206](./0206-customs-action-admission-judgement-folds-three-sources-and-judges-cross-zone-by-node-customs-zones.md)：决定六，判断按内容派生标识、在自己的事务里记
- [ADR-0149](./0149-business-command-faces-split-into-frontline-operator-and-integration-client-families.md)：决定二，作业事实渠道
- [ADR-0022](./0022-http-status-carries-answer-formed-not-business-verdict.md)、[ADR-0023](./0023-work-fact-identity-and-time-are-minted-by-the-device.md)、[ADR-0146](./0146-product-strategy-is-a-third-class-between-mechanism-and-tenant-values.md)、[ADR-0222](./0222-composition-roots-register-endpoints-and-routes-per-feature-file.md)
- [TF CONTEXT](../domain/transport-fulfillment/CONTEXT.md)：Rules「揽收、派送与交付」
- [UC-TF-006](../application/transport-fulfillment/UC-TF-006-PERFORM-DELIVERY-AND-CAPTURE-POD.md)：交付前门禁问询一条规则与 `AT-TF-322`–`325`
- [GLOSSARY](../domain/GLOSSARY.md)「交付前门禁问询」「动作准入判断」、[CONTEXT-MAP](../domain/CONTEXT-MAP.md) `transport-fulfillment → customs-compliance`
- 票 `.scratch/demo-chain-gaps/issues/94-a-courier-previews-the-final-delivery-gate-before-dispatching.md`（含评审记录）；后继票 `.scratch/demo-chain-gaps/issues/165-customs-docs-name-the-delivery-gate-preview-as-a-request-timing.md`、`171-gate-previews-are-scoped-to-the-asking-devices-work-and-leave-a-trace.md`、`172-customs-judgement-register-grows-with-every-gate-preview.md`；代决复核 `.scratch/owner-review-20261020/report.md` R14
