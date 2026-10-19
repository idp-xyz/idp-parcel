# ADR-0205：出发门禁是拉取的读数——与移动事实同笔落册，不拒收；移动事实带声明的段

Status: Accepted（2026-10-19，Claude Code 会话代决：用户原话「你作为业务和系统专家自决……解决所有的问题」；票 demo-chain-gaps/32，统一代决见 `.scratch/demo-chain-gaps/tf-chain-plan.md`「05 移动事实与出发门禁」D05-1、D05-2、D05-6 至 D05-10 与「需要的 ADR」ADR-0205。越权风险点见文末，待各条括注的 owner 复核。）
Date: 2026-10-19

## Context

- 出发门禁由设备自报。`gateRequired` 与 `gateClearance` 随命令进来；`gateRequired` 为真而没带 `gateClearance` 时，登记编排答 `DEPARTURE_GATE_BLOCKED`（200）且不落库。这一格出自票 tf-unwired-seven/06，不是哪一份 ADR 定的。
- 一线设备的离线队列 2xx 即出队（ADR-0022）。答 200 而不落库，等于把一次实际发生的出发从记录里删掉，而车已经开走了。
- 设备报 `gateRequired=false` 就没有门：设备自己能把门关掉。放行引用是自由串，没人核。
- 移动事实只挂班次，不知道车上是哪几件，也就无从问哪件受门禁。UC-TF-005 要求移动事实带对象、段、资源。
- ADR-0206 立了关务的动作准入判断：请求驱动，消费方只交意图，适用哪些受门禁动作、三源怎么折都由关务答；判断在关务自己的事务里记下，在调用方事务里调用即拒。它在 `cmd/parcel-api` 装好了，还没有生产消费者。
- 先例：ADR-0186，跨上下文读不动不挡已发生的物理事实落册，记成「读不动」；ADR-0210 那一族的节点方向依据在写事务之前判好、与事实同一笔落册。
- CONTEXT 禁止「用计划覆盖事实」；ADR-0096 规定段身份是声明出来的。

## Decision

**一、出发一律落册，门禁是同一笔保存的读数，不是拒收的理由。**

- 读数是封闭五格：`放行` / `阻断` / `不受门禁` / `未决` / `读不动`。阻断带原因与限制来源，未决带缺什么，读不动指名哪一口。
- 退役「`DEPARTURE_GATE_BLOCKED`、不落库」那一格。读数阻断，出发照样答 201 `MOVEMENT_FACT_RECORDED`。
- 挡下违规出发是动作发生之前那一问的事（设备出发前读口，见越权风险点 1），不是登记一条已发生事实的事。

**二、读数由本上下文在写事务之前同步拉取，逐对象问关务，只翻译。**

- 消费侧端口 `DepartureGateView`（ADR-0025）在写事务之外问关务的动作准入判断（ADR-0137 决定二：出发登记本身就是请求）。读不动不挡落册：读数记读不动，出发照记。
- 意图是「出发：自 A 节点到 B 节点」，起讫取该班次在本上下文班次册里当前版的方向（ADR-0201 的连接代码加起讫节点，按 NR 拼写）。不取设备报的地点：地点不一定是网络节点，而且起讫若由设备自报，设备就能报一个同区去向把跨区那道门关掉。
- 时点取出发的业务发生时间，不取登记时刻：离线补登的出发按它出发那一刻判。
- 逐对象问，每件一格：关务的答复译成本上下文的五格，依据原文（适用动作、依据种类、来源）与关务判断标识一并保存。只带回与关务整次答复同格的依据。
- 关务的`未配置`落`未决`：对一条已经发生的出发，缺配置与缺案卷同样是「此刻判不出」；依据种类仍说得出缺的是配置。关务自己答读不动，那一件记读不动、依据指名关务那一口；问不出答复（调用失败、答复译不回来）也记读不动，依据种类为 `CUSTOMS_ADMISSION_UNAVAILABLE`。一件读不动不停下，其余照问。
- 本上下文这一侧找不齐问关务要的东西就停下、不问关务，停点只属一格：
  - 未决：段未声明、段不在册、出发那一刻段里没有在控对象、班次不在册（说不出自哪到哪）。
  - 读不动：段册或班次册读不动。
- 整条出发的读数按压制次序取最重：阻断 > 读不动 > 未决 > 放行 > 不受门禁。读不动压过未决：有一件根本没问到，比「问到了、关务说还缺东西」说得更少，不能被后者盖住。其余次序与关务答复的压制次序同一（ADR-0206 决定五）。整格另存一列，重建时核它与停点、逐对象那几格推出的那一格一致。

**三、设备自报字段失去权威。**

- `gateRequired` 不再决定是否设门，`false` 照样问关务。命令与载荷里的字段暂留不读：一线入口仍按原形透传它，删它要随那一侧一起改。
- `gateClearance` 只是设备出示的证据，原样保存；挂在移动或到达上仍是输入不合法。

**四、移动事实带登记方声明的段与资源，不加外键；对象取段里出发那一刻在控的参与。**

- 段与资源是登记方声明的引用（ADR-0096 同一纪律），可缺、不加外键（迁移 TF 0010 同理）：事实可以先于段登记到达，那时照实记下事实。段不在册照记，读数答未决。
- 对象取段里每个对象链尾那一版参与（未被替代、未失效），且出发时点落在它的控制起止之内。按出发时点不按此刻：迟到的出发到达时，那几件可能已经在下一站交接离场。
- 不经装载分配推对象：分配是意图，拿它覆盖段里的实际控制就是「用计划覆盖事实」。
- 不核班次在册（班次只用来取方向）：事实可以早于班次登记，班次不在册时读数答未决、事实照记。

**五、移动事实信封与它的消费者在同一个变更上线；阻断读数派生异常信号。**

- 本决定不入队。`transport-fulfillment.movement-fact.recorded` 与 VE 消费者、路由行、里程碑映射参考配置同一变更上线（ADR-0049：接得住才登记；票 demo-chain-gaps/33）。
- 阻断读数随信封交 VE，由 VE 派生异常信号；本上下文不立异常案件。

**六、读数是判断，不是内容。**

- 内容摘要（TFC-1，按本体现算、不存）收登记方报的那几格：班次、种类、地点、声明的段与资源、设备出示的凭据。读数不进：同一条出发重投时关务可能答得不同，不能因此把重放判成冲突。
- 同身份同内容再到答 `已有版本`，回显首次登记时问来的读数，不另记一份；换段、换资源是内容冲突。
- 迁移 0026 之前落册的出发没问过关务，回填为未决、停点「登记于门禁读数之前」。新登记的出发不会停在这一格，构造门拒它。

## 候选与反方

- **维持拒收，阻断时答 200 不落库。** 否决：离线队列据此出队，一次实际发生的出发就从记录里消失，违规出发也因此查无可查。
- **拒收改答 4xx 让设备重投。** 否决：重投一万次车也回不来，队列只会堵住后面的事实。
- **关务把放行推给运输履约，运输履约按推来的结果设门。** 否决：ADR-0137 决定二规定门禁判断由请求驱动，出发登记本身就是请求；推的模型要运输履约维护一份关务事实的副本，一决策两处定义。
- **运输履约自己折限制、门禁与放行三源。** 否决：同 ADR-0206 的反方，决策两处定义。
- **在写事务里问关务。** 否决：关务判断随出发那一笔回滚，「当时凭什么这么答」查不回来；跨上下文读在写事务里出一次 SQL 级错误，整笔进中止态，读不动反而挡住了落册。
- **对象经装载分配推。** 否决：用计划覆盖事实。实际装载、短装、多装只能由事实说。
- **起讫取设备报的地点与去向。** 否决：地点不一定是网络节点；去向自报让设备能关掉跨区的门。
- **关务`未配置`在读数里另成第六格。** 否决：读数是对一条已发生事实的记录，续办动作都是「去补，然后由下一道读口用上」，与未决同；依据种类已经说清缺的是配置。
- **一件读不动就整条停下、不问其余。** 否决：其余几件里可能有阻断，停下就丢了那个信号。

## Consequences

- 领域：新文件 `domain/departure_gate_reading.go`（五格、停点、逐对象那一格、`FormDepartureGateReading` 与重建门、段的 `ObjectsUnderControlAt`）；`TransportMovementFact` 带段、资源与读数，`ErrDepartureGateBlocked` 退役，读数放错地方是 `ErrDepartureGateReadingMisplaced`；内容摘要加段与资源。
- 应用：`DepartureGateReader`（写事务之前读段、班次，逐对象问关务）；`RecordMovementFactHandler.Record` 多一个读数参数，`MovementFactGateBlocked` 退役。
- 端口与适配器：新文件 `ports/departure_gate.go`；新包 `adapters/customscompliance`（TF→CC 消费侧，只翻译）；postgres 读写新列；迁移 TF `0026`。
- 入口：`/transport-fulfillment/movement-facts` 载荷加可缺的 `segment`、`resource`，响应加 `gateReading`；`gateRequired` 照收不读。
- 装配：`buildMovementFactOrchestration` 接段册、班次册与关务准入判断那一层自带事务的壳，main 不动。
- 演示（证据 `S`）：按种子登节点区域与周期安排、显式登班次、交接立段之后，自营 SHA→SZX 出发答 201，读数不受门禁。
- 后继：信封与 VE 投影（票 33）；外包干线交接与有效交付的门禁、删设备字段、设备出发前读口等 peer（方案「等 peer 的」）。

## 越权风险点

1. **违规出发被记录而不是被挡下**（TF owner）。挡下要靠设备在动作之前问；那个读口在一线入口那一侧（`work_fact_intake*.go`），等 peer 收工后另立票。在那之前，阻断只能事后由 VE 派生异常信号发现。
2. **读不动也照记**（TF owner、运维）。关务长时间不可用时，那段时间的出发全是读不动，没有自动补问；补问要另立机制（例如对读不动的出发按原时点重问并追加一版读数）。
3. **外包段的门禁不在这里**（TF owner）。外包干线的跨境门禁落在把货交给承运人的那次交接上（ADR-0206 后果节），本决定只修自营移动。
4. **起讫取班次当前版的方向**（TF owner）。出发之后班次登了下一版且换了方向（ADR-0201 越权风险点「出发之后仍可成下一版」），读数取的是登记那一刻的当前版。班次不在册时只能答未决，演示与生产都要求出发引用的班次先登。
5. **对象只取段里的参与，不展开集运单元**（TF owner、NO owner）。段里在控的是袋或集运单元时，关务按包裹引用解析不到（ADR-0206 越权风险点「对象只认包裹」）。展开成员要读 NO 的集运成员关系，等那一侧的端口稳定。
6. **出发前的段登记是读数的前提**（TF owner、一线客户端）。入口已收 `segment`，但一线客户端不报它时，出发一律停在「段未声明」答未决；客户端补报段号属一线那一侧，等 peer。
7. **关务的未配置并进未决**（CC owner、TF owner）。消费方从整格分不出「去补配置」与「等立案」，要看依据种类。若 owner 要求读数单列未配置，五格加一格、压制次序随关务那边改。
8. **更正沿用原读数**（TF owner）。移动事实的更正今天没有生产入口；领域的更正版带着原登记那一刻的读数。更正入口落地时，要不要按更正后的时点重问，由那一票定。
9. **整格的压制次序是本上下文自定的**（TF owner、VE owner）。读不动压过未决是本决定的取舍；VE 若只看整格派生信号，会把「一件读不动、一件未决」当成读不动处理。

## Links

- [TF CONTEXT](../domain/transport-fulfillment/CONTEXT.md)：「出发门禁读数」词条；Rules「实际移动与出发门禁读数」一节
- [UC-TF-005](../application/transport-fulfillment/UC-TF-005-ESTABLISH-HANDOVER-AND-ACTUAL-FULFILLMENT.md)：移动事实与门禁读数的输入、结果、流程与 `AT-TF-105`–`111`
- [ADR-0206](./0206-customs-action-admission-judgement-folds-three-sources-and-judges-cross-zone-by-node-customs-zones.md)：关务动作准入判断——本决定的提供方
- [ADR-0186](./0186-node-reception-records-physical-facts-when-the-service-result-read-fails.md)：跨上下文读不动不挡物理事实落册
- [ADR-0137](./0137-customs-gate-judgments-are-registered-facts-driven-by-assessment-requests-payment-gate-rule-is-registered-and-funds-facts-are-minted-only-in-settlement-accounting.md)：决定二，门禁判断请求驱动
- [ADR-0096](./0096-a-fulfillment-segment-identity-is-declared-not-derived.md)：段身份是声明的
- [ADR-0201](./0201-schedule-templates-materialize-concrete-schedules-by-date-and-preparation-answers-not-configured-on-an-empty-register.md)：班次方向是连接代码加起讫节点
- [ADR-0025](./0025-cross-context-adapters-live-on-the-consumer-side.md)：跨上下文适配器在消费方
- [ADR-0022](./0022-http-status-carries-answer-formed-not-business-verdict.md)：2xx 即出队
- [ADR-0049](./0049-publish-channel-is-in-process-delivery-until-load-evidence.md)：接得住才登记
- 票 `.scratch/demo-chain-gaps/issues/32-movement-facts-declare-segment-and-record-departure-gate-reading.md`、`05-movement-facts-are-unlinked-and-departure-gate-is-self-reported.md`
