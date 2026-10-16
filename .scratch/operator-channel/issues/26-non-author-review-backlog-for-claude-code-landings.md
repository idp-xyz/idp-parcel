# 26 补非作者评审：票 15 后两笔、20、22–25 与 ps-external-mark-relations/02 进 main 时只有作者自审

Category: task
Status: ready-for-agent——2026-10-16 Claude Code 会话立（用户「你帮我做决定吧」授权代决：维持 2026-09-16「本仓不用子代理」裁定，评审欠账集中到本票，由非作者通道领）
Blocked by: 无
领票条件：评审者不得是作者会话。下表全部代码出自同一个 Claude Code 会话（2026-10-16，会话名 `idp-parcel-c8`，分支 `claude-oc15` / `claude-oc20` / `claude-oc21` / `claude-oc25`）。照 [parallel-sessions](../../../docs/agents/parallel-sessions.md)「合入前独立评审」办：隔离检出、`/code-review <基线>`、两轴、只读、不跑全仓，结论写进各自票面 Comments「评审 ← 通道 N · 钉 `<SHA>`」。

## 为什么维持裁定，不放开子代理

- 2026-09-16 裁定写明「裁定不随宿主变」。换到子代理能用的宿主，正是那句话预见过的情形，用户当时选了不随之翻案。
- 并行会话可能跑在不同宿主上。一条随宿主变的规矩，会让两边各守一套。
- 欠账不靠子代理也有出口：parallel-sessions 本来就规定派空闲的非作者通道评。今天缺的是一个空闲通道，不是规矩。
- 下表各笔进 main 时没有空闲的非作者通道，按 workflow.md 那条裁定退到推送方自审，并已在各票面写明「不算非作者评审」。评审出阻断就在新票上修，不回滚已进 main 的提交。

## 评审清单（按业务风险排序，先评前面的）

| 序 | 票 | 代码（main 上的 SHA） | 基线 | 重点看 |
|---|---|---|---|---|
| 0 | [ps-external-mark-relations/02](../../ps-external-mark-relations/issues/02-register-and-customer-parcel-numbers-feed-node-identity.md) · [ADR-0184](../../../docs/adr/0184-external-identifier-relations-are-an-append-only-register-formed-with-their-source-facts.md) | `cc52e9b6`（建模 `5714a137`） | `98934a17` | 代决的建模取舍七条，ADR 末尾列了六条越权风险点（接受时形成、逐字比对、号码不随版本重报、同号不拒、委托仓储写册、客户参考号不形成）；生产上节点收寄从此真的会形成收寄与控制，是这一批里影响面最大的一笔 |
| 1 | [15](./15-operation-decision-faces-take-operator-intake.md) 受控关闭与重开 · [ADR-0183](../../../docs/adr/0183-operator-channel-addresses-controlled-closure-and-reopening-by-parcel-and-derives-the-shipper-from-the-accepted-request.md) | `0e3bb7df` | `60244fa6` | 代决的业务授权取舍：载荷只按包裹寻址，请求方与货主账户取包裹所属已接受委托的客户账户，操作者不进命令。对照 ADR-0183 与 PS CONTEXT 受控关闭、重开各句，看有没有越过客户授权边界 |
| 2 | [25](./25-node-reception-carries-parcel-shipment-service-result-markers.md) 收寄带服务结果标记 | `0214a62a` | `7a8f56e5` | 收寄事务里的跨上下文读；读不动答`收寄待确认`而不是空标记；封闭词表只认前缀；拒收、仅扫描、待识别、身份冲突不问读口 |
| 3 | [20](./20-tf-closure-dispatch-task-and-pickup-judgment-canonicalization.md) 承运商首次有效收寄判断 | `857453d8` | `e558c66e` | TFC-1 规范化（业务时间截到微秒，因 PG 存微秒）；同依据不同内容答 `CONTENT_CONFLICT`；关段已关闭回显在册关闭时刻 |
| 4 | [15](./15-operation-decision-faces-take-operator-intake.md) 余下四口 | `60244fa6` | `858280a2` | 关段、建派送任务、有效时间判断、承运商首次有效收寄判断换操作者 Intake；同笔撤隔离写放行，看有没有哪个口因此失去入口 |
| 5 | [22](./22-tf-isolated-command-intake-retires.md) | `7ec601ab` | `f33d849d` | 隔离命令 Intake 退场后，原用例是否逐格迁到作业事实 Intake |
| 6 | [23](./23-node-operations-isolated-command-intake-retires.md) | `9c551dc4` | `7ec601ab` | 同上，NO 一侧；门里节点作业那一格删得干净 |
| 7 | [24](./24-demo-configurator-holds-the-shipment-side-decision-grants.md) | `e8673ac1` | `9c551dc4` | 演示种子：五种委托侧运营决定的授予与依据引用，只用 `SYN-` 值 |

票 15 的之一至之四不是本会话写的，不在本表。

## 完成判据

- 表中每一行都有一条非作者评审写进对应票面，三格齐（阻断 / 非阻断 / 无发现）。
- 有阻断的，各立修复票并在本票列出票号。
