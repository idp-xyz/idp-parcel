# 91 交进节点却对不上作业实物的交接没有节点对账

Category: enhancement
Status: needs-triage——2026-10-19 票 [62](./62-node-control-is-established-by-handover-in.md) 的作者子代理立（用户「你作为业务和系统专家自决……解决所有的问题」授权）。出自 [ADR-0214](../../../docs/adr/0214-node-control-transfers-out-and-in-on-an-authoritative-transport-handover.md) 决定七与文末「补记：票 62 落决定七时的补充裁定」越权风险点「对不上作业实物的转入没有节点对账」；方案「后继」一节「HANDOVER_IN 对不上作业实物时的节点对账」那一条（[handover-dispatch-plan](../handover-dispatch-plan.md)）。
Blocked by: 无

## 现象

- `已交接`且声明了接收节点、交接对象却对不上一件作业实物时，节点控制转移册在转入方向入账一行（`NO_ASSOCIATED_UNIT`、`SEVERAL_ASSOCIATED_UNITS`、`CONSOLIDATION_UNIT_PENDING_RULE`），不造待识别实物。最常见的来路是场外揽收直进干线、从没在任何节点收寄过的包裹被交进某个节点。
- 那一行不带声明的接收节点：转移册的列形里节点属于控制那一组，控制不在场时节点也不在场。运营要知道「哪个节点收进了一件本上下文对不上的包裹」，得拿交接键回交接登记册（`registry=transport-handover`）取 `receivingNode`。
- 节点上没有任何东西提示现场：这件包裹在节点的作业（扫描、分拣、集拆）照常形成事实，却没有控制；此后以该节点交出时，转出一侧入账「无关联作业实物」或「无在身控制」。

## 要裁的

- **对账读面**：按节点列出「交进来却对不上作业实物」的交接，要不要在转移册上为转入行另存声明的接收节点（列形要改），还是读面联查交接册。
- **对上之后补转入**：现场随后为这件包裹建了作业实物（例如节点到站观察成了待识别、再经重核关联到这件包裹）时，要不要反查转移册上同包裹的这类入账行、就地补一次转入；补的时刻取交接的业务时间还是关联成立的时刻。
- **与票 [81](./81-control-source-chain-recompute-has-no-disposition-face.md) 的分界**：81 裁入账各格的人工处置面；本票只管「交进节点、对不上实物」这一类的发现与对账，处置口若归 81 就只做读面。
- 三类：读面与补转入的判法是机制；对账周期与谁看是租户取值。

## 规模

M
