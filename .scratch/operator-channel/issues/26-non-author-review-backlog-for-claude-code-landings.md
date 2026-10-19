# 26 补非作者评审：票 15 后两笔、20、22–25 与 ps-external-mark-relations/02 进 main 时只有作者自审

Category: task
Status: resolved——2026-10-16 会话 `idp-parcel-9b` 补评完毕：表中每行各有一条非作者评审（隔离子代理）写进对应票面 Comments；两条阻断各立修复票——[27](./27-carrier-pickup-voiding-replay-without-kind-is-not-accepted.md)（序 3，已由票 20 的代码作者会话认领）、[28](./28-controlled-closure-and-reopening-have-no-replay-algebra.md)（序 1，needs-triage）；见文末「评审结果」。此前 in-progress——2026-10-16 会话 `idp-parcel-9b` 认领（非作者会话；用户「需要」）：用户同日改裁「本仓允许子代理」（`docs/agents/workflow.md`「本机环境」那条，`ae29b3c2`），本会话起隔离子代理逐行评。此前 ready-for-agent——2026-10-16 Claude Code 会话立（用户「你帮我做决定吧」授权代决：维持 2026-09-16「本仓不用子代理」裁定，评审欠账集中到本票，由非作者通道领）
Blocked by: 无
领票条件：评审者不得是作者会话。下表全部代码出自同一个 Claude Code 会话（2026-10-16，会话名 `idp-parcel-c8`，分支 `claude-oc15` / `claude-oc20` / `claude-oc21` / `claude-oc25`）。照 [parallel-sessions](../../../docs/agents/parallel-sessions.md)「合入前独立评审」办：隔离检出、`/code-review <基线>`、两轴、只读、不跑全仓，结论写进各自票面 Comments「评审 ← 通道 N · 钉 `<SHA>`」。

## 为什么维持裁定，不放开子代理

> 本节已被用户 2026-10-16 改裁取代（「本仓允许子代理」，见 `docs/agents/workflow.md`「本机环境」）。下面四条保留作立票时的理由，不再是现行依据；「评审出阻断就在新票上修，不回滚已进 main 的提交」那一句仍照办。

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

## 评审结果（2026-10-16，会话 `idp-parcel-9b`）

做法：每行在钉代码 SHA 的隔离只读 detached 检出上起隔离子代理评（非作者；代码出自会话 `idp-parcel-c8`）。序 0 两轴各起一个子代理；其余每行一个子代理，按 `/code-review` 两轴串行，先写完 Spec 再做 Standards，两轴不互相改判。子代理报的阻断，由本会话在 main `4dd937eb` 上复核属实后才立票；各条也核过在 `4dd937eb` 上有没有被后来的提交修掉。逐条写在各票 Comments「评审 ← 会话 idp-parcel-9b 起的隔离子代理 · 钉 …」。

| 序 | 票 | 钉 | Spec 阻断 / 非阻断 | Standards 阻断 / 非阻断 | 去向 |
|---|---|---|---|---|---|
| 0 | ps-external-mark-relations/02 | `cc52e9b6` | 0 / 4 | 0 / 4 | 关联键吞掉新关联、「同号不拒」的事实更正两条，其后随同目录票 03 进 main（`58c4c496` 与 ADR-0185；`83ca983b` 给 ADR-0184 追加复核记录）。去空白进册键、决定六与 UC-NO-002 的范围冲突、已接受 ADR 后果节原地改写，待 PS owner 裁 |
| 1 | 15 之六 | `0e3bb7df` | **1** / 3 | 0 / 3 | 修复票 [28](./28-controlled-closure-and-reopening-have-no-replay-algebra.md)：needs-triage，先裁「按什么认出同一次请求的重放」；潜伏，授权映射接上之前必须修。货主重授权证据落不落册，待 PS owner 裁 |
| 2 | 25 | `0214a62a` | 0 / 2 | 0 / 3 | 待用户裁：本票裁定「读不动…不落册」与 UC-NO-002「收寄判断待确认｜物理事实已保存」相抵；同事务读 PS 遇 SQL 错误时答 500 而不是`收寄待确认`，修法随这一裁定定 |
| 3 | 20 | `857453d8` | **1** / 2 | 0 / 4 | 修复票 [27](./27-carrier-pickup-voiding-replay-without-kind-is-not-accepted.md)：已由票 20 的代码作者会话认领（分支 `claude-oc27`）。子代理建议另立：给 TF 定一处在册时刻精度规则 |
| 4 | 15 之五 | `60244fa6` | 0 / 1 | 0 / 3 | 随票记 |
| 5 | 22 | `7ec601ab` | 0 / 3 | 0 / 5 | 随票记。子代理建议另立：TF 更正载荷缺时刻答 400 |
| 6 | 23 | `9c551dc4` | 0 / 1 | 0 / 4 | 随票记（Standards 有一条已由 `0214a62a` 修掉） |
| 7 | 24 | `e8673ac1` | 0 / 3 | 0 / 1 | 随票记 |

序 2 的去向（2026-10-18）：用户授权会话 `idp-parcel-9b` 代决，修复票 [29](./29-node-reception-records-physical-facts-when-cross-context-reads-fail.md) 落地，裁定成文为 [ADR-0186](../../../docs/adr/0186-node-reception-records-physical-facts-when-the-service-result-read-fails.md)，取代票 25 裁定里「读不动答`收寄待确认`、带续办引用、不落册」那一句；同事务读 PS 的 SQL 级错误改经 savepoint 隔离。

序 1「货主重授权证据落不落册，待 PS owner 裁」的去向（2026-10-19，研判轮 review26-followups-decide 簇 C5）：用户授权会话 `idp-parcel-9b` 代决，拆两票。C5-Q2（请求方种类段与责任来源词表收进 application）由票 [37](./37-shipper-requester-and-closure-source-vocabulary-live-in-application.md) 落地；C5-Q1 由票 [38](./38-controlled-closure-and-reopening-record-request-and-operation-evidence.md) 落地（分支 `claude-oc38`，待评审），裁定成文为 [ADR-0187](../../../docs/adr/0187-controlled-closure-and-reopening-record-request-and-operation-evidence-on-the-decision.md)：请求证据与操作证据落在决定上，部分停用 ADR-0183 决定五首句与 ADR-0151 决定二受控关闭与重开一项半句。

序 0 的去向（2026-10-19）：待裁的三格已于 2026-10-16 由会话 `idp-parcel-9b` 代裁，见 [ADR-0184](../../../docs/adr/0184-external-identifier-relations-are-an-append-only-register-formed-with-their-source-facts.md) 复核记录「越权风险点 2 的事实更正」「决定六与 UC-NO-002「权限、隔离与审计」」「后果节在接受后被原地改写的补记」三条与票 [ps-external-mark-relations/04](../../ps-external-mark-relations/issues/04-ticket-02-review-follow-ups-records-and-pinned-behavior.md)；另立 ps-external-mark-relations/05、no-reception-identity/01 与 02。

序 4、序 5 的去向（2026-10-19）：两行非阻断里 TF `adapters/http` 那半边（更正载荷缺时刻答 400；作业事实与运营决定 Intake 的逐口用例、头注与 `decisionLines` 计数）由票 [32](./32-tf-correction-payloads-pass-absent-instants-to-orchestration.md) 落（分支 `claude-oc32`，待评审），`cmd/parcel-api` 那半边归票 [48](./48-cmd-parcel-api-tests-and-comments-say-what-the-code-does.md)。

完成判据核对：表中每一行都有一条非作者评审写进对应票面（序 1、序 4 都在票 15），三格齐；有阻断的序 1、序 3 各立修复票（28、27）。非阻断照 parallel-sessions 随票记，不另立票；要不要另立、何时清，归各票的 owner。

评审用的隔离检出（钉各行代码 SHA 的那几份，加上复核用的 main `4dd937eb` 那一份）都已 `git worktree remove` 拆掉。
