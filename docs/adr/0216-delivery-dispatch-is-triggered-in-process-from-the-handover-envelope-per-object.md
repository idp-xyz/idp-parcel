# ADR-0216：派送发起随交接信封在进程内逐对象发起——TF 自消费交接登记信封，按对象此刻在场的段调既有触发执行器，任务成立时间取派发时钟；要求缺失入账不重投，补拍口保留

Status: Accepted（2026-10-19，Claude Code 会话代决：用户原话「你作为业务和系统专家自决……解决所有的问题」；票 demo-chain-gaps/60，统一代决见 `.scratch/demo-chain-gaps/handover-dispatch-plan.md`「代决」C10 与「需要的 ADR」ADR-0216。回答 [ADR-0114](./0114-delivery-dispatch-is-triggered-by-entering-a-declared-delivery-segment-and-pulls-requirements-by-reference.md) 决定二末句，参数登记册 `PAR-NET-09` 的拍频格随之退役。越权风险点见文末，待 TF owner 复核。）
Date: 2026-10-19

## Context

- ADR-0114 决定二把派送发起定为「异步一拍」的内部执行器，末句写「执行器的生产入口（谁按拍调它）随第一条派送要求缝的实施票立」。三条要求缝都已接真（票 tf-segment-lifecycle-closure/12–14），执行器却只挂在作业事实口 `/transport-fulfillment-delivery-dispatch-triggers` 后面，要设备手打一拍。`cmd/parcel-dispatch` 里没有派送的消费者。`cmd/parcel-api` 装配点头注写的是「谁按拍调这一口、拍频多大，是租户运营节拍的实例半边」。
- 参数登记册 `PAR-NET-09` 一行写「派送何时发起的内置策略同在票 09，拍频是租户取值」（psb/09 第 2 项）。
- 交接登记成立的同一笔里，交出引用式信封 `transport-fulfillment.transport-handover.registered`，键是租户、对象、交接范围、版本四维。派发进程今天只把它投给 VE 投影。
- 执行器一拍只处理一个对象、形成一项任务。任务引用由段、对象、入场依据三件确定性铸出：票 tf-face-algebra/09 把拼法改成带长度前缀的 v2，并新增答格 `DISPATCH_TASK_REFERENCE_TAKEN`。执行器先查在册再向所有者取件，同一拍原样重跑答`已在册`，不再问所有者。
- CONTEXT 原句：「触发形成任务是异步的一拍：控制事实先如实落库，任务在下一拍形成，形成失败重跑本拍，不回滚交接。」
- 先例：初始路由信封同时扇出给 VE 投影与 TF 运输准备（ADR-0201 决定七），两路各记各的 inbox 账。未决要不要重投，按 ADR-0094 的判据：看重投会不会改变结果。

## Decision

**一、事件驱动，逐对象发起：TF 消费自己交出的交接信封。**

- 派发进程把 `transport-fulfillment.transport-handover.registered` 扇出给两路：VE 投影在前，TF 派送发起在后。两路各记各的 inbox 账。
- 派送发起这一路先按信封的键读回那一版交接。裁决不是`已交接`（拒收、待确认）时入账，不再查段：控制没有移入接收方，对象也就没有进新段。
- 裁决是`已交接`时，按对象问段登记册：对象此刻在哪个段里在场。
  - 零个：入账。
  - 一个：把这个段和对象交给既有触发执行器。
  - 多于一个：库面不一致，响亮失败，不替它挑一个。
- 段取「此刻在场的段」，不取「这次交接立进的段」，理由有两条：
  - 交接记录本身不带段号。
  - 信封晚到时，对象可能已经被下一次交接带走。这时此刻在场的段才是该派送的那一段；前一段的参与已经离场，照 ADR-0114 决定二本就不触发。
- 派送段、在场、入场依据、查在册、取件这几件，都留在执行器里判，入口不重判。

**二、这一拍就是 CONTEXT 说的「下一拍」，不是租户节拍。**

- 任务成立时间取派发进程处理这封信那一刻的时钟，也就是执行器命令里的业务时间：任务是在这一刻形成的。不取交接判断时刻，那是触发事实发生的时刻；也不取写库时刻。
- ADR-0114 已定一拍一对象，所以没有拍频可选。`PAR-NET-09` 里「派送何时发起……拍频是租户取值」那一格退役。产品不出按拍批量的形态，也不出拍频参考配置。

**三、答复的处置按「原样重投会不会改变结果」分三格。**

- **入账，不重投。** 入口自己答的两格：交接不是`已交接`；对象不在任何段里。执行器答的几格：已形成、已在册、不是触发事实（任一拒绝格）、引用被占、要求缺失。
  - 要求缺失也入账：所有者答「没有」，重投也不会变成「有」。任务保持待形成（ADR-0114 决定三），补齐之后经补拍口再发起。
- **重投。** 这一路登自己的未决哨兵，名单只列本路的未决：
  - 交接还读不回；
  - 交接登记册或段登记册读不动；
  - 执行器的未决，缝没接线那三格除外：某条要求缝读不动、段登记册读不动、任务登记册读不动。
- **响亮失败。** 不入账，也不登哨兵：
  - 本上下文自己交出的键过不了自己的构造门；
  - 执行器不受理；
  - 执行器答出封闭集合以外的格；
  - 装配缺一条要求缝（`*_SOURCE_NOT_WIRED`）；
  - 对象同时在不止一个段里在场。
  - 这些原样重投一万次都一样。入账会把缺陷吞掉，登哨兵会让它看上去像在等依赖。
- 三格怎么分由应用层判：编排结果的 `Disposition`。理由是认得执行器每一格的是本上下文，消费门不该再认一遍。消费门只把三格译成 inbox 的三种动作。
- 处理方跑在 inbox 那一笔事务里：任务那一行与 inbox 的「已处理」同笔提交。重投时消费门先认出已处理就停下，执行器不会再问所有者。

**四、作业事实口 `/transport-fulfillment-delivery-dispatch-triggers` 保留，作补拍用。**

- 补拍与进程内这一拍调的是同一个执行器，读的是同一组登记册与所有者，所以两边的答复互相认：对同一个段、对象和入场依据补拍，答`已在册`，并交回同一项任务。
- 补拍口的渠道、Intake 与答复形状都不变。

**五、演示即采用，演示租户不另登任何设置。**

- 新加坡尾程场外设备登记末端交接（`segmentServiceAction=FINAL_DELIVERY`）之后，派发进程跑一拍就形成任务。
- 再手打补拍口，答 `DISPATCH_TASK_ALREADY_FORMED`。

## 候选与反方

- **按拍批量：定时扫派送段里在场、还没派的对象。** 否决。有拍就得有人选拍频，而拍频是运营节拍；一拍一对象已经定了，信封本来就是逐对象到的。扫描还要一张「在场且未派」的读面，那是同一判据的第二份定义。
- **在交接登记编排的同一事务里触发。** 否决，ADR-0114 的 Alternatives 已经否过：同一事务要读三个外部上下文。
- **要求缺失也登哨兵重投。** 否决。重投改变不了所有者的「没有」，只会烧完失败预算落 `ABANDONED`，把一个业务答案伪装成发布失败（ADR-0094 判据）。
- **段取交接命令里声明的段。** 否决。交接记录不存段号，要取就得给交接加一维；而且信封晚到时，已离场的那一段会被拿去触发。
- **任务成立时间取交接判断时刻。** 否决。任务不是那一刻成立的；离线补登的交接还会让任务的成立时间早于系统知道它的时刻。
- **拒收与待确认也交给执行器。** 否决。它们不是触发事实。对象若仍在一个先前的派送段里，而那一拍当时答了要求缺失，一次拒收就会把它重新发起——触发只该来自进段这件事。同一情形还能从「待确认更正为`已交接`」这条路进来，本记录没有堵，见越权风险点 2。

## Consequences

- TF：
  - `application/trigger_dispatch_on_handover.go`：编排、结果代数与 `Disposition`。
  - `adapters/inbox/transport_handover_registered_consumer.go`：消费门。inbox 名是 `transport-fulfillment/trigger-delivery-dispatch-from-transport-handover`，与 VE 那一路分家。
  - 不加迁移，不加信封，执行器不改。
- 派发进程：
  - 路由表里交接那一行改为扇出。
  - 装配另立 `assemble_delivery_dispatch_on_handover.go`。三条要求缝照 `cmd/parcel-api` 的 `buildDeliveryDispatchTrigger` 另接一份，不跨 main 包复用。
  - 哨兵名单只登本路的未决哨兵。
- 文档：
  - UC-TF-006 的触发行与 `AT-TF-191`–`200`；
  - TF CONTEXT「揽收、派送与交付」那一句；
  - 参数登记册 `PAR-NET-09` 一行退掉拍频格；
  - 演示动线脚本的补记。
- 过期注释：凡说派送发起的拍频归调用方或租户、谁按拍调待定，或说作业事实口是执行器的生产入口、触发面不是进程内循环的注释，自本记录起都已过期。所在文件落本记录时在别的票的地盘里，逐处清点与改法另立票 [demo-chain-gaps/71](../../.scratch/demo-chain-gaps/issues/71-stale-cadence-comments-point-at-adr-0216.md)。

## 越权风险点

1. **要求缺失不会自愈**（TF owner）。入账之后，所有者补齐了也没有任何东西回头再发起；补拍要人记得去打。待形成的对象没有查阅面，只在那一次答复里出现一次。另立票 [demo-chain-gaps/70](../../.scratch/demo-chain-gaps/issues/70-delivery-dispatch-left-unformed-has-no-trace.md)。
2. **段取此刻在场的段，不核链尾的入场依据是不是这一版交接**（TF owner）。有两种情形，会让这一拍替一个这次交接并没有带对象进去的段发起：
   - 信封晚到：对象已被下一次交接带进后一段，若后一段也是派送段，就替后一段形成任务；后一段自己的信封随后答`已在册`。
   - 待确认更正为`已交接`：更正只替代凭前版入场的参与（ADR-0112），前版待确认没有参与，于是更正既不结束对象在先前段里的参与，也不让它进新段。对象若仍在先前的派送段里，而那一段的那一拍当时答了要求缺失，这封更正版信封就会让执行器替那一段形成任务——可这次交接说的正是对象已交出那一段。这与「候选与反方」否掉拒收、待确认交执行器的理由是同一种情形。
   - owner 若要求严格只认「这次交接进的那一段」，最小的改法是：执行器命令带一个可选的期望入场依据，由本编排传入这一版交接，链尾的入场依据不是它就答`不是触发事实`。改动只在执行器那一道门，不另立第二份规则。另一条路是让交接记录带段号。
3. **拒收与待确认不交执行器**（TF owner）。入口自己挡下。若 owner 认为这两格也该交执行器、由它答`不是触发事实`，改编排一处即可。
4. **任务成立时间取派发时钟**（TF owner）。重投那一拍形成的任务，成立时间是形成那一刻，不是首投那一刻。
5. **拍频格退役**（TF owner、产品）。若某些租户要按班次批量派送（一车一单），那属于「派送任务合并规则」（psb/09 第 8 项），不是拍频，不回到本格。
6. **缝没接线按缺陷处置**（TF owner）。ADR-0114 决定三把它答成未决；在派发这一路，重投不会让缝接上，所以落发布失败、不登哨兵。
7. **交接更正会自动另立一项任务**（TF owner）。更正后仍是`已交接`时，参与按新入场依据被替代（ADR-0112），更正版的信封进来，执行器按新键形成第二项任务，前一项不动。这是 ADR-0114 与票 tf-face-algebra/09「不做（观察）」里已记的那一条；进程内发起让它自动发生，原先要人手打。owner 若要求更正不另立任务，要改执行器的铸法，按段和对象铸；本记录不动执行器。

## Links

- [ADR-0114](./0114-delivery-dispatch-is-triggered-by-entering-a-declared-delivery-segment-and-pulls-requirements-by-reference.md)：决定二末句「执行器的生产入口……随第一条派送要求缝的实施票立」由本记录回答；决定三的「要求缺失」与「未接线」答法
- [ADR-0201](./0201-schedule-templates-materialize-concrete-schedules-by-date-and-preparation-answers-not-configured-on-an-empty-register.md)：决定七，同一封信扇出给 VE 与 TF、各记各的 inbox 账的先例
- [ADR-0094](./0094-undecided-retry-is-decided-by-resume-path-with-a-fourth-grade-for-operator-registration.md)：重投与否按恢复动作分
- [ADR-0029](./0029-retrieval-failure-algebra-is-partitioned-by-recovery-action.md)：确定性错误不进未决名单
- [ADR-0049](./0049-publish-channel-is-in-process-delivery-until-load-evidence.md)：接得住才登记
- [ADR-0112](./0112-source-correction-rederives-participation-as-a-superseding-version-on-the-same-segment.md)：更正在同段上形成替代参与
- [ADR-0146](./0146-product-strategy-is-a-third-class-between-mechanism-and-tenant-values.md)：内置策略归产品，拍频不再是租户取值
- [TF CONTEXT](../domain/transport-fulfillment/CONTEXT.md)：「揽收、派送与交付」一节
- [UC-TF-006](../application/transport-fulfillment/UC-TF-006-PERFORM-DELIVERY-AND-CAPTURE-POD.md)：触发行与 `AT-TF-191`–`200`
- 参数登记册 `PAR-NET-09`：[../product/PILOT-PARAMETER-REGISTER.md](../product/PILOT-PARAMETER-REGISTER.md)
- 票 `.scratch/demo-chain-gaps/issues/60-delivery-dispatch-is-triggered-in-process-from-the-handover-envelope.md`、`06-delivery-dispatch-has-no-in-process-trigger-and-no-recipient-leg.md`；票 `.scratch/tf-face-algebra/issues/09-dispatch-trigger-task-reference-is-a-faithful-identity.md`
