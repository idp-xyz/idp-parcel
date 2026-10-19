# ADR-0213：权威运输交接声明交出、接收、去向节点——节点控制转移、路由复核与交出 / 交付门禁的位置都取这份声明

Status: Accepted（2026-10-19，Claude Code 会话代决：用户原话「你作为业务和系统专家自决……解决所有的问题」；票 [demo-chain-gaps/59](../../.scratch/demo-chain-gaps/issues/59-handover-declares-releasing-receiving-and-onward-nodes.md) 起草并落决定一至三，决定四由票 [65](../../.scratch/demo-chain-gaps/issues/65-handover-records-an-outbound-release-gate-reading.md) 落，决定五由票 [66](../../.scratch/demo-chain-gaps/issues/66-effective-delivery-records-a-final-delivery-gate-reading.md) 落；统一代决见 `.scratch/demo-chain-gaps/handover-dispatch-plan.md`「代决」C1、C13、C14。回答 [ADR-0206](./0206-customs-action-admission-judgement-folds-three-sources-and-judges-cross-zone-by-node-customs-zones.md) Consequences「挂法等 peer」一格与其越权风险点 9。部分停用 [ADR-0014](./0014-versioned-canonicalization-shape-for-content-digest.md) 决定里的一句，适用场景只限 TF 交接首登上的节点补报，见 Consequences。越权风险点见文末，待 TF owner 复核，NO、NR、CC owner 会签。）
Date: 2026-10-19

## Context

- 交接记录没有节点维。`TransportHandoverSpec` 的交出方、接收方是不透明的参与方引用，交接范围也不透明。下游有四件事都要知道节点：
  - NO 按交接立 HANDOVER_IN，要知道转入哪个节点；
  - NR 交接触发的路由复核要实际位置；
  - ADR-0206 的交出意图要 at（交出节点）与 onward（后续去向）；
  - 交付意图要 at。
- 参与方不是节点。一个承运方在多个节点上交接，一个节点上也有多个参与方，所以从参与方解析不出节点。ADR-0096 已经为段身份裁过同一类问题：声明，不推导。从参与方或范围里解析节点，同样是推导。
- ADR-0206 Consequences 把外包干线交接与有效交付的门禁挂法留给「等 peer」。它的越权风险点 9 写的是「交接登记怎样取去向、挂哪种意图，等 peer 收工后由交接那一票定」。
- 交接首登按 TFC-1 形存内容摘要（ADR-0014），演示库与开发库里已有存量行，更早的还有无版本摘要。存量摘要不回写。

## Decision

**一、交接登记加三个可选声明。**

- 交出节点 `releasingNode`：对象在哪个节点交出，即哪个节点的控制在这里结束。
- 接收节点 `receivingNode`：对象交进哪个节点，即哪个节点在这里取得控制。
- 去向节点 `onwardNode`：交出之后对象往哪个节点去。

三格都存 network-routing 的节点代码，本上下文不透明保存。不核 NR 节点目录，不核登记设备的作业范围，不从交出方、接收方或交接范围解析，也不与它们对账。三格与裁决无关：拒收与待确认同样发生在某个节点上，同样可以声明。

只声明一个「交接节点」不够。节点交给运输方时控制从节点出去，运输方交给节点时控制进节点，节点之间直接交接时一出一进，这三种情形要分得开。

**二、缺席是一种答案。** 没声明的那一格就是「没有声明」：本上下文不补，它也不是待补的欠账。取位置的各方按自己的规则作答，见决定四、五与决定六所列。三格都不声明的交接，与节点进文法之前同形、同答。

**三、节点声明是交接内容。**

- **进内容摘要。** 至少声明一格时，首登摘要写新形状 `TFC-2`：三格都写，没声明的写 null，因为哪一格缺席本身是内容。一格都不声明时仍写 `TFC-1`，字节与此前相同，存量行照原内容重放仍答已有版本。这一条是「旧行按旧版比」。按内容分版而不是整口换号的先例是 PC 的 `PCC-2`（ADR-0209）。
- **同键换节点是冲突。** 同一交接版本换一个节点、撤掉一个节点，或撤掉全部节点，都答 `SOURCE_CONFLICT`，在册那一版不动。
- **旧形行上补报节点也是冲突。** `TFC-1` 或更早的无版本形存下的行，同键再交时若声明了节点，同样答冲突。旧形里没有节点一格，命令在旧形之下没有写法；按旧形重算会把声明静默丢掉，而 PC 不肯把 `PCC-2` 的条件按 `PCC-1` 静默丢掉，是同一个理由。这一句停用 ADR-0014 决定里的一句，范围见 Consequences。
- **更正承继前版。** 更正改的是判断，不是交接发生在哪：更正命令不带节点，新版本照前版的三格。更正载荷带节点键答坏报文（400），不静默丢掉。
- **回显与查阅。**
  - 登记答复回显在册那一版的三格：重放答首登时的声明，更正答承继来的声明，没声明的键不出现。
  - 查阅口 `registry=transport-handover` 逐格列出三格。
  - 交接登记信封不变，只带键，消费方按键读回。所以 ADR-0216 的派送发起不受影响。

**四、交出门禁（C13，票 65 落）。**

- 写事务之前，对`已交接`的每个对象，按交出意图问 CC 的动作准入判断：at 取交出节点，onward 取去向节点。
- 读数随交接同一笔照记，与 ADR-0205 是同一套封闭集。读数不拒收交接。
- 没声明交出节点：记未决「交出节点未声明」，不问 CC。
- 没声明去向：照问。跨区那一道按 ADR-0206 决定二不列，读数里写明去向未声明。

这一条回答 ADR-0206 的越权风险点 9：交接登记的去向取登记方声明，挂交出意图。

**五、交付门禁（C14，票 66 落）。**

- 有效交付按交付意图问 CC，at 取对象进入该派送段的那次交接的交出节点，读数照记。
- 对象凭收寄进段，或那次交接没声明交出节点：记未决「交付节点不可得」。

at 取的是本上下文自有的事实，不在有效交付上另增一个声明。

**六、其余取位置的各方。**

- NO 的节点控制转出与 HANDOVER_IN 怎样读这三格，由 ADR-0214 定（票 61、62）。本记录不把哪一格定为控制转移的前提：没声明交出节点，不等于控制不转出。
- NR 交接触发的路由复核怎样取实际位置，由 ADR-0215 定（票 63）。

本记录只定三格的形状与语义。各方怎样读、缺席或与自家记录不符时答什么，写在各自的记录里。

## Alternatives considered

- **从交出方、接收方解析节点。** 否决：一个参与方在多个节点上交接，解析出来的节点只是猜。这正是 ADR-0096 不推导段身份的理由。
- **只声明一个「交接节点」。** 否决：控制转出与转入分不开，见决定一。
- **从登记设备的作业范围推节点。** 否决：作业范围是授予的范围，不是位置。场外设备的范围是 `OFFSITE`，推不出节点。
- **核 NR 节点目录。** 否决：TF 要多开一个读 NR 目录的口，还要处理目录的版本。ADR-0201 决定二对连接与节点同样只保存、不核：两本册子各有各的登记方与节奏。声明错了，各消费方读到时按自己的规则答，见越权风险点 1。
- **交接整口换号到 TFC-2，不声明节点也写 TFC-2。** 否决：同一内容在库里会有两种摘要，存量行的原样重放要多走一道旧形比对，换来的只是形状整齐。
- **旧形行上补报节点按旧形比、认作重放（ADR-0014 字面）。** 否决：登记方声明的节点被静默吞掉，只能从答复里少了的那一格看出来。下游的节点控制转入、路由复核与门禁都会当作没声明。
- **更正可以改节点。** 否决：更正口的内容是判断，节点是交接发生在哪。改事实走另一种面，今天没有，见越权风险点 4。

## Consequences

- 领域：`domain.HandoverNodeDeclarations`，交接判断上的 `Nodes`、`ReleasingNode`、`ReceivingNode`、`OnwardNode`。`Correct` 承继前版，`RehydrateTransportHandoverSpec` 带三格。摘要形状 `TFC-2` 由 `CanonicalizeHandoverPayload` 按内容选，交接口改用 `CompareStoredHandoverDigest` 比对。
- 编排：`RegisterTransportHandoverCommand` 加三格。空白读作没有声明；存不下的值（含 NUL）答未受理。
- 存储与查阅：TF 迁移 `0039` 给交接登记册加三列可空，空白串库面拒。登记册的各条读路都带回这三列，查阅册逐格列出。
- 线格式：首登载荷加三个可缺键，答复与交接册回显。更正载荷不收这三键。
- **部分停用 ADR-0014。**
  - 被停条款：决定里「规范化版本不同不是冲突，也不得据此判定重放失败」一句。
  - 适用场景：仅限 TF 交接首登一口，在册那一版存的是 `TFC-1` 或无版本形，而本次命令声明了节点，此时答冲突。
  - 理由的前提：旧形之下节点没有写法，按旧形重算会丢掉声明。只要本次命令没声明节点，旧形行仍按旧形比、照答重放。
  - 反方向不需要停用：在册是 `TFC-2`、本次没声明节点时，按 `TFC-2` 重算三格皆为 null，同一形状下必然不同，本来就是冲突。
  - ADR-0014 的其余各条，以及其余各口，都不变。
- TF CONTEXT「权威运输交接结果」词条补一句。UC-TF-005 的输入契约补一行，验收表立 `AT-TF-216`–`222`。演示动线脚本补记。
- 演示种子不登交接。交接是作业事实，演示走查时由场外设备登。

## 越权风险点

1. **声明错了没人核**（TF owner；NO、NR、CC 会签）。节点代码不核目录，也不核设备作业范围。拼错或填错的节点会带来三种后果：
   - NO 在错误的节点立 HANDOVER_IN，或者对不上实物；
   - NR 按错误的位置复核；
   - CC 按错误的节点判关务区域。
   缓解是各消费方按自家规则作答：NO 对不上实物就入账；CC 遇到未登记区域的节点，按 ADR-0206 决定二作答。owner 若要核，可以加一道「节点在 NR 目录在册」，代价是立 TF→NR 的节点读口。
2. **节点与参与方两套指称并存**（TF owner）。交出方、接收方与三格节点可能对不上，例如声明的交出节点是 A，交出方却是节点 B 的参与方。本上下文不裁谁对。
3. **存量行补不上节点**（TF owner）。按决定三，旧形行上补报节点答冲突，部分停用 ADR-0014 一句。更正口又不收节点，所以节点进文法之前登下的交接永远没有节点声明。
4. **没有改节点的面**（TF owner）。节点声明错了，更正改不了，只能由人工处置。处置面没有立。
5. **没声明交出节点的交出不问 CC**（TF、CC owner）。读数记未决，不是阻断。漏声明交出节点的跨境交出不会被关务挡，只能靠读数派生的信号让人看见，那一步是后继。
6. **交付门禁的 at 取派送段入口那次交接的交出节点**（TF、CC owner）。派送实际发生在收件地，at 取的是派送出发的节点；凭收寄进段的对象答未决。
7. **去向没声明时跨区那一道不列**（CC owner）。登记方漏填去向，跨关务区域那一道门就不出现在读数里，这与 ADR-0206 越权风险点 12 是同一类风险。

## Links

- [ADR-0206](./0206-customs-action-admission-judgement-folds-three-sources-and-judges-cross-zone-by-node-customs-zones.md)：Consequences「挂法等 peer」与越权风险点 9，由决定四、五回答
- [ADR-0096](./0096-a-fulfillment-segment-identity-is-declared-not-derived.md)：声明，不推导
- [ADR-0014](./0014-versioned-canonicalization-shape-for-content-digest.md)：摘要形状版本；本记录部分停用其中一句，见 Consequences
- [ADR-0209](./0209-supplier-agreements-declare-charge-occurrence-conditions-and-registration-judges-by-them.md)：按内容分版（`PCC-2`），不静默丢掉新形状的内容
- [ADR-0205](./0205-departure-gate-is-a-pulled-reading-recorded-with-the-movement-fact-not-a-refusal.md)：门禁读数随事实同笔照记、不拒收
- [ADR-0201](./0201-schedule-templates-materialize-concrete-schedules-by-date-and-preparation-answers-not-configured-on-an-empty-register.md)：决定二，节点与连接只保存、不核 NR 目录
- [TF CONTEXT](../domain/transport-fulfillment/CONTEXT.md)：「权威运输交接结果」词条
- [UC-TF-005](../application/transport-fulfillment/UC-TF-005-ESTABLISH-HANDOVER-AND-ACTUAL-FULFILLMENT.md)：输入契约「交接节点声明」一行与 `AT-TF-216`–`222`
- 票 `.scratch/demo-chain-gaps/issues/59-handover-declares-releasing-receiving-and-onward-nodes.md`；统一代决 `.scratch/demo-chain-gaps/handover-dispatch-plan.md`「代决」C1、C13、C14
