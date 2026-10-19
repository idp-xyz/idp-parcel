# ADR-0220：节点控制认交接更正链只按运输履约的更正回指——同一（对象，范围）下另起的首登是另一次交接，不是前一次的更正

Status: Accepted（2026-10-19，Claude Code 会话代决：用户原话「你作为业务和系统专家自决……解决所有的问题」；票 demo-chain-gaps/95，出自 `.scratch/product-strategy-boundary/issues/05-demo-journey-criterion-evidence.md`「重走（钉 `8197fa2f`，2026-10-19）」停点一。部分停用 [ADR-0214](./0214-node-control-transfers-out-and-in-on-an-authoritative-transport-handover.md) 决定五「同链认法」一句，回答它的越权风险点「同链按（对象，范围）认」与文末补记的越权风险点「一次交接不转出它自己立起的控制，靠的仍是同链按（对象，范围）认」。越权风险点见文末，待 NO owner 复核，TF owner 会签。）
Date: 2026-10-19

## Context

- 演示动线重走（钉 `8197fa2f`，只记 `S`）：一件包裹先后两次登`已交接`，`scope` 都写 `SZX`。
  - `H2/v1`：自营干线交进深圳口岸，只声明接收节点。TF 答首登；NO 在深圳口岸立 HANDOVER_IN。
  - `H3/v1`：深圳口岸交给干线承运商，声明交出节点为深圳口岸。TF 照样答首登，不是更正。NO 却把它当 `H2/v1` 的后版：转出一半剔掉「同链转入立起的那一份」，落「无在身控制」。
  - 再下一次交接 `H4/v1`（`scope` `SIN`，不声明交出节点）把深圳口岸那一份转出。控制在错的那次交接上转出，册上看不出错。
- TF 怎样认更正：
  - 交接的身份是（租户，对象，范围，版本）。范围是「一次交接的共同范围（一车、一批、一次交出）」，不承载结论。
  - 更正只经更正口登记：新版回指被更正的那一版，保留对象与范围、只换版本。链线性，一版至多被更正一次（票 tf-face-algebra/07）。`TransportHandovers.FindSuccessor` 按回指认后版，范围汇总按回指折掉被替代的那一代。
  - 首登口不问同一（对象，范围）下是否已有别的版本：换一个版本号就是另一次首登。
- NO 怎样认更正：ADR-0214 决定五写「同链认法：同一（对象，范围）的各版」。编排里有三处依赖这条认法，都在 UC-NO-005 两半里：
  - 「同链前版已推进过」：后继版仍`已交接`时零写入；
  - 撤回控制的更正：入账「控制来源链待重算」；
  - 「一次交接不转出它自己立起的控制」：转出一半剔掉同链转入立起的那一份。
- 两边对「哪几版是一条链」各认各的。ADR-0214 的越权风险点「同链按（对象，范围）认」预先写明了这一格，票 62 补记又添了一条后果；此前没有承接票。

## Decision

**一、一版交接所在的更正链，是它与它沿 TF 更正回指上溯的各前版。**

- 同一（对象，范围）是链上各版共有的性质，不是认链的判据。同一范围下另起的首登是另一次交接，在两个方向上都按它自己判。
- ADR-0214 决定五与文末补记里凡说「同链」的，都按这条链读：同链前版已推进、撤回控制的更正、一次交接不转出它自己立起的控制。各格的判法与处置不变。

**二、前版由消费侧适配器沿回指上溯。**

- 适配器（ADR-0025）按信封的键读回那一版之后，沿它本体上的回指逐版按键读回前版，答复多带「各前版，近者在前」；首登版不带。这一版有没有后继，照旧按 TF 的找后继一问。
- 不加 TF 读口，不改 TF：回指在每一版本体上已有，逐版按键读足够。
- 上溯途中的处置照 ADR-0029 按恢复动作分格：
  - 前版读不动：与这一版读不动同一格，登本路既有的未决哨兵、重投。
  - 前版读不回、读回的与键不符、回指成环：提供方的仓储坏了，响亮失败，不入账、不登哨兵。前版在后版登记那一笔之前已经提交，看得见后版就看得见前版，读不回不是可见性滞后。
- 编排拿到答复后立链，再核一次链的形状：链上各版对象与范围相同、版本不重复。立不起来同样响亮失败。

**三、转移册认「链上推进过没有」只取链上各版。**

- 读法是（租户，对象，范围，方向）下、版本属于这条链的推进行。（对象，范围）只用来缩小读的范围，走既有的部分索引。
- 一条链上仍至多推进一次，读到两行照旧报错。同一范围下另起的首登各自推进，范围下于是可以有不止一行推进行，不再是错误。
- 不加迁移。

**四、TF 的首登口不改。**

- 不拒同一（对象，范围）下另起的首登，不新增答复格。
- 「范围每次交接各起一个、版本只给更正用」是登记方的用法约定，写进演示动线；本记录不靠它保证节点控制正确。

**五、存量不改写、不重判。**

- 本记录之前按（对象，范围）认链落下的转移行照旧在册。
- 本记录之前零写入、没留行的那一版，信封若被人工补拍，按本记录重判。
- 存量里可能认错的行，要不要入账或处置，归票 demo-chain-gaps/81 的处置面。

## 候选与反方

- **TF 首登口遇到同一（对象，范围）已有版本时拒绝，答一个指名结果、要求走更正口。** 否决。
  - 范围在 TF 语言里是一次交接的共同范围，不是更正链的身份；拿它当链的身份，等于替 TF 改语言。
  - 首登口多一种拒绝，就会有真实交接登不进来：设备离线补登、两台设备把节点代码当范围填都会撞上。作业事实照记，不能为下游的读法拒收。
  - 提供方的语言本来就按回指认更正（找后继、范围汇总）。错在消费方的读法，改读法的一方是 NO。
- **NO 在转移行上存链首版本，认链按链首。** 否决。
  - 要迁移，还要回填：存量行的链首得回 TF 读。
  - 链首与回指是同一件事实的两份写法，日后会漂。
- **TF 开一个「按链列出各版」的读口。** 否决。多一个提供方读口；更正链短，回指在每一版上已有。
- **NO 按（对象，范围）加业务时间或声明的节点推断是不是同一次交接。** 否决。那是推导，不是声明（ADR-0096），与 ADR-0213 不从参与方或范围解析节点同一个理由。

## Consequences

- NO：
  - `domain/handover_correction_chain.go`：更正链值，立链时核形状。`AuthoritativeHandoverReference.SameChain` 退役；`PendingControlSourceRecompute` 改收更正链，只认链上的前版。
  - `ports/physical_control.go`：交接来源答复加各前版；`FindAdvancedInHandoverChain` 改收更正链。
  - `adapters/transportfulfillment/control_transfer.go`：沿回指上溯。
  - `adapters/postgres/physical_control_transition.go`：认链按链上各版取。
  - `application`：`Advance` 立链，两半共用。
- 不加迁移，不加信封，不改 TF，派发路由、inbox 名与哨兵名单不变。
- 每封信多读 TF 的次数等于这一版的前版数，首登版不多读。
- 文档：
  - UC-NO-005：输入契约、流程第 2 步、一致性一条、`AT-NO-126`–`128`；
  - NO CONTEXT「节点控制」一句、CONTEXT-MAP `node-operations ↔ transport-fulfillment` 一个分句；
  - 演示动线的补记；
  - ADR-0214 的 Status 行与 Links 加前向指针，README 那一行写部分停用。

## 越权风险点（待 NO owner 复核，TF owner 会签）

1. **TF 不拒同一（对象，范围）下另起的首登**（TF）。节点控制不再受影响；但 TF 的范围汇总按回指折，不按对象去重，同一对象在同一范围下两次首登会在那个范围的汇总里算两次。靠的是「范围每次交接各起一个」这条用法约定，没有门禁守。TF owner 若要守，在首登口加门禁或让汇总按对象取一个结果，节点控制一侧不用跟着改；后继票 demo-chain-gaps/99。
2. **存量不重判**（NO）。本记录之前按（对象，范围）认错链的行留在册上：
   - 转出一侧落了「无在身控制」，或被当成撤回控制的更正而入账「控制来源链待重算」；
   - 那份控制随后被下一次交接转出，转出行上看不出是错的那一次。
   - 零写入的那一版不留行，册上找不到。
   - 处置归票 demo-chain-gaps/81。演示库与开发库按一次性库用，生产没有存量。
3. **补拍旧信封答复会变**（NO）。本记录之前零写入的版本，若它其实是同一范围下另起的首登，补拍时会按自己的链推进或入账，不再零写入。这是要修的行为本身；人工补拍的人要知道结果可能与当时不同。
4. **每封信多读 TF**（NO、TF）。链长没有上限，读次随之线性增加；成环响亮失败。更正频繁的租户，派发这一路的延迟随更正次数增长。
5. **前版读不回按缺陷处置**（NO、TF）。依赖 TF「前版先于后版提交、原交接不删」。TF 日后若归档或清理旧版，这一路会响亮失败，要先改这一格。
6. **链上各版同对象与范围是立链的前提**（NO、TF）。TF 的更正今天保留对象与范围。TF 日后若允许更正换范围，NO 立链会拒，信封响亮失败；要连同 ADR-0214 决定三的主键一起重看。

## Links

- [ADR-0214](./0214-node-control-transfers-out-and-in-on-an-authoritative-transport-handover.md)：决定五「同链认法」一句由本记录停用；越权风险点「同链按（对象，范围）认」与文末补记的越权风险点「一次交接不转出它自己立起的控制，靠的仍是同链按（对象，范围）认」由本记录回答
- [ADR-0213](./0213-a-transport-handover-declares-its-releasing-receiving-and-onward-nodes.md)：节点只声明、不从范围推导；更正承继前版的节点声明
- [ADR-0025](./0025-cross-context-adapters-live-on-the-consumer-side.md)：消费侧适配器
- [ADR-0029](./0029-retrieval-failure-algebra-is-partitioned-by-recovery-action.md)：读不动与仓储坏了分格
- [ADR-0096](./0096-a-fulfillment-segment-identity-is-declared-not-derived.md)：声明，不推导
- [UC-NO-005](../application/node-operations/UC-NO-005-ADVANCE-NODE-CONTROL-ON-AN-AUTHORITATIVE-TRANSPORT-HANDOVER.md)
- [TF CONTEXT](../domain/transport-fulfillment/CONTEXT.md)：「权威运输交接结果」词条与生命周期「权威运输交接」
- 票 `.scratch/demo-chain-gaps/issues/95-node-operations-reads-a-new-handover-under-the-same-scope-as-a-correction.md`、`81-control-source-chain-recompute-has-no-disposition-face.md`、`99-handover-scope-summary-counts-one-object-twice-under-the-same-scope.md`；票 `.scratch/tf-face-algebra/issues/07-handover-correction-chain-stays-linear.md`
