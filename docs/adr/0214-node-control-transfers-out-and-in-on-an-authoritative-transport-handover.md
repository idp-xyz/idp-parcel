# ADR-0214：节点控制按权威运输交接转出与转入——交接对象按正式包裹身份经节点自己的当前关联对上作业实物，转移记在只追加的节点控制转移册上、读控制时折叠，`已交接`转出此刻唯一在身的控制，撤回控制的更正入账待重算（UC-NO-005）

Status: Accepted（2026-10-19，Claude Code 会话代决：用户原话「你作为业务和系统专家自决……解决所有的问题」；票 demo-chain-gaps/61，统一代决见 `.scratch/demo-chain-gaps/handover-dispatch-plan.md`「代决」C2–C7 与「需要的 ADR」ADR-0214。决定七（转入）由票 demo-chain-gaps/62 实现。越权风险点见文末，待 NO owner 复核，TF owner 会签。）
Date: 2026-10-19

## Context

- NO CONTEXT「节点控制」一节早已写下转出：「`transport-fulfillment` 针对明确对象形成权威运输交接的“已交接”结果；已拒收或待确认不转出控制」，以及逐对象纪律：「部分交接时，各对象分别结束或保留节点控制」。CONTEXT-MAP 的 `node-operations ↔ transport-fulfillment` 一条同调。
- 用例层没有承接：UC-NO-001、UC-NO-002、UC-NO-003 都把权威交接排除在外。UC-NO-004 已被身份冲突处置占用。
- 代码层的缺口：
  - 交接登记信封 `transport-fulfillment.transport-handover.registered` 只扇出给 VE 投影与 TF 派送发起，NO 收不到。
  - 消费侧适配器 `ControlTransferAdapter.TransferOut` 没有生产调用点，对身份靠交接对象与作业实物标识的字符串相等。交接对象是正式包裹身份或集运单元，控制挂在作业实物上，三者今天靠同名碰运气。
  - 节点控制只有收寄一个生产来源，存在收寄行的 `control` 列里；收寄册只追加，没有转出写口。
- 后果：控制从不转出，收寄编排认「当前收寄」时一直认那条旧收寄；包裹日后回到原节点，会被并进一条陈旧收寄（ADR-0199 决定四）。
- TF CONTEXT 的约束：「若证据已经表明实物占有发生变化……不能继续把已经没有实际保管能力的一方记为当前实物控制方」；同时禁止「直接把当前控制回退给早期交出方」。
- 节点维：交接记录今天没有节点。交出、接收、去向节点的声明由 ADR-0213（票 demo-chain-gaps/59 起草）补；转出本身不需要节点——节点随在身的控制一起来。

## Decision

**一、新立 UC-NO-005「按权威运输交接推进节点控制」，转出与转入同一用例；NO 经派发进程消费交接登记信封。**

- UC-NO-002、UC-NO-003 不动。
- 派发进程把交接登记信封再扇出一路给 NO，inbox 名 `node-operations/transfer-control-on-transport-handover`，与另两路分家。
- 消费侧适配器（ADR-0025）按信封的四维键（租户、对象、范围、版本）读回那一版，再问它有没有被同链后一版更正。信封里别的格不读。
- 交接的业务时间折到微秒再用：转移册落在 `timestamptz` 上，转出前与控制成立时刻比较的、与读回时比较的必须是同一个时刻。

**二、交接对象怎样对上作业实物。**

- 先问对象是不是本租户的集运单元。是：入账「待单元控制规则」，不推进。包裹号与集运单元号同串时按集运单元判。
- 否则按正式包裹身份，读本上下文收寄册上此刻关联到它的作业实物。只读各条判断链的当前有效版本——被重核、冲突或处置接续过的那一版上的关联不再算，与 ADR-0210 方向依据「读链头」同一读法。
  - 恰一件：推进。
  - 零件、多件：入账，不推进。
- 不按字符串比对象与作业实物。

**三、控制转移记在只追加的「节点控制转移册」上，读控制时折叠。**

- 一行是一版交接在一个方向上的处理结果：转出（OUT）或转入（IN），推进了（ADVANCED）或入账不推进（NOT_ADVANCED，带原因）。
- 主键是（租户，交接对象，交接范围，交接版本，方向）：同一版在同一方向上只处理一次。
- 收寄行的 `control` 列不改写。控制是否已转出，读时把转移册上的转出按（租户，作业实物，控制依据）折进去。按键读回收寄、认当前收寄与查阅目录都经同一个折叠读。
- 一份控制至多转出一次，由部分唯一索引守。
- 读在身控制之前取的实物锁，与收寄认当前收寄是同一把：转出与同一件实物的收寄、与同一件实物上的另一次转出串在一起。
- 查阅口 `/node-operations-records?registry=physical-control` 照行上列，推进的与没推进的同册。

**四、怎样判转出。**

- `已交接`即转出该实物此刻唯一在身的控制。收寄成立的与交接转入成立的都算。转出依据是交接给出的引用（`TRANSPORT-HANDOVER/<版本>`），时刻取交接的业务时间。
- 入账、不推进的几格：
  - 没有在身控制；
  - 不止一份在身控制；
  - 交接早于那一份控制成立——它说的是更早的一次占有。
- 拒收与待确认：零写入。
- 声明的交出节点（ADR-0213）与在身控制的节点不符时照转、记信息格。这一格随票 demo-chain-gaps/62 落，本记录不建列。

**五、交接更正。**

- 后继版仍是`已交接`，同链前版已转出过：零写入。
- 撤回控制的更正（同链前版转出过，这一版是拒收或待确认）：入账「控制来源链待重算」，带着先前那次转出。控制不自动回退。
- 被同链后一版更正了的那一版晚到：零写入，由后一版自己的信封决定。
- 同链认法：同一（对象，范围）的各版。

**六、对信封的处置，判据同 ADR-0216 决定三：原样重投会不会改变结果。**

- 入账不重投：
  - 转出了、入账不推进的各格；
  - 已在册；
  - 零写入的三格。
- 重投：交接登记册读不动，或还读不回这一版。这一路登自己的未决哨兵。
- 响亮失败，不入账、不登哨兵：
  - 信封键过不了本上下文的构造门；
  - 提供方记录与键不符；
  - 编排答出封闭集合以外的格。

**七、转入（HANDOVER_IN），由票 demo-chain-gaps/62 实现。**

- 只在`已交接`且声明了接收节点时，在该节点立 HANDOVER_IN 控制，落同一本转移册（方向 IN）。读在身控制已认这种行。
- 对不上作业实物就入账，不造待识别实物。待识别实物只能由现场观察建立。

## 候选与反方

- **在收寄行上改写 `control` 列的转出两格。** 否决。收寄册是只追加的判断链（ADR-0199），改写一行就是改写一次已经作出的判断；冲突版、重核版、处置版各抄着同一份控制，改一行漏一行就会读出两种答案。
- **另立控制表，收寄与转入都写进去。** 否决。收寄的控制起点今天随收寄判断同一笔落下，挪出去要动收寄编排与它的全部替身，而那是另一条在途会话的地盘。读时折叠只多一个函数，收寄编排一字不改。
- **转移册只记推进了的转出，不推进的不留行。** 否决。对不上实物、控制来源链待重算都要人看得见；不留行，它们只在日志里出现一次。
- **继续按字符串比交接对象与作业实物。** 否决。正式包裹身份属 PS，作业实物属 NO；同名只在合成夹具里成立。
- **不止一份在身控制时一并转出，或挑最新的一份转出。** 否决。说不出交接结束的是哪一份；一并转出会把漏登的那次交接一起抹掉，挑一份则另一份永远在身。
- **撤回控制的更正自动把控制交还交出方。** 否决。TF CONTEXT 明文禁止。
- **集运单元的交接逐成员转出。** 暂不。单元控制与成员「随单元受控」的规则还没立，拿单元的交接替成员逐件转出，就是替那条规则拍了板。

## Consequences

- NO：
  - 迁移 `node_operations/0017`：转移册、四本索引、折叠函数 `control_with_transfer_out`。
  - `domain/control_transition.go`、`ports/physical_control.go`、`application/transfer_control_on_handover.go`。
  - `adapters/postgres` 的转移册适配器与查阅册。收寄读口的控制一格改经折叠读。
  - `adapters/transportfulfillment` 改写：按键读回交接，去掉字符串比对；`ControlTransferAdapter` 退场。
  - `adapters/inbox` 的交接登记消费门。
  - 查阅口新册名 `physical-control`。
- 派发进程：交接那一行的扇出加第三路，装配在 `assemble_node_control_on_handover.go`，哨兵名单只登本路的未决。
- 文档：UC-NO-005；NO CONTEXT「节点控制」一节追加；CONTEXT-MAP `node-operations ↔ transport-fulfillment` 一句；开发主线 PN-04 一行的补记；演示动线的补记。
- 不加信封，不改 TF。

## 越权风险点（待 NO owner 复核，TF owner 会签）

1. **按集运单元判同串**（NO、TF）。包裹号与集运单元号同串时按集运单元入账，那件包裹的控制不转出。
2. **集运单元的交接暂不推进**（NO）。演示与真实网络里整袋交接是常态，成员的控制会一直在身，直到单元控制规则立起来。后继票 demo-chain-gaps/80。
3. **对象认法只读链头**（NO）。一码两物给先到那件追加冲突版之后，那件实物对不上、转出入账不推进。身份冲突期间交走的包裹，控制要等处置之后靠人补。
4. **不止一份在身控制时不推进**（NO）。本记录之前从无转出，同一件实物在两个节点各收寄过一次就会有两份在身；这类存量要靠人对账。
5. **交接早于控制成立入账**（NO）。离线补登、时钟偏差都可能落进这一格。
6. **同链按（对象，范围）认**（TF）。TF 若允许同一对象在同一范围下另起一条独立的交接，第二条会被当成第一条的更正：仍`已交接`时零写入。
7. **一次转出可能掩盖漏登的中间交接**（NO）。转出只看此刻在身的那一份，不核交出节点；ADR-0213 的节点声明落地后由票 62 记信息格。
8. **控制来源链待重算没有处置面**（NO、TF）。查阅口看得见，没有入口处置；对不上实物、不止一份在身控制等入账各格同。后继票 demo-chain-gaps/81。
9. **折叠在 SQL 函数里**（NO）。凡读收寄控制的语句都要经它读，直接读 `control` 列的新语句会读出未转出；本记录只改了今天的几处读口。

## Links

- [ADR-0199](./0199-node-reception-preserves-the-delivery-source-and-rechecks-pending-identities.md)：判断链、当前有效版本，决定四「同一作业实物在同一节点并入」
- [ADR-0210](./0210-identity-conflicts-are-disposed-by-an-operation-decision-and-the-identity-basis-reads-chain-heads.md)：读链头
- [ADR-0216](./0216-delivery-dispatch-is-triggered-in-process-from-the-handover-envelope-per-object.md)：同一封交接信封扇出的先例，决定三的处置判据
- [ADR-0025](./0025-cross-context-adapters-live-on-the-consumer-side.md)：消费侧适配器
- [ADR-0029](./0029-retrieval-failure-algebra-is-partitioned-by-recovery-action.md)：确定性错误不进未决名单
- [UC-NO-005](../application/node-operations/UC-NO-005-ADVANCE-NODE-CONTROL-ON-AN-AUTHORITATIVE-TRANSPORT-HANDOVER.md)
- [NO CONTEXT](../domain/node-operations/CONTEXT.md)：「节点控制」一节
- [TF CONTEXT](../domain/transport-fulfillment/CONTEXT.md)：「运输交接与实物控制」一节与生命周期「权威运输交接」
- 票 `.scratch/demo-chain-gaps/issues/61-node-control-transfers-out-on-an-authoritative-handover.md`、`62-node-control-is-established-by-handover-in.md`、`04-handover-does-not-move-node-control-or-trigger-reassessment.md`；勘察 `.scratch/no-control-transfer-out/report.md`
