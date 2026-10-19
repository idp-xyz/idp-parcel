# ADR-0197：关务外部结果按层进全程追踪投影——事实类型词逐层分开、放行再按全部/部分/附条件分开，沿结果范围指名的申报单元逐成员展开，同层冲突的那份不进投影；参考映射只把全部放行映成清关

Status: Accepted（2026-10-19。用户授权「你作为业务和系统专家自决，解决所有的问题」；Claude Code 会话代 VE、CC owner 裁定，落在票 [demo-chain-gaps/11](../../.scratch/demo-chain-gaps/issues/11-external-result-received-has-no-subscriber.md)。越权风险点待 VE、CC owner 复核。）

Date: 2026-10-19

## Context

`customs-compliance` 接收外部监管结果（UC-CC-006）后经 Outbox 交出 `customs-compliance.external-result.received`，载荷只带接收幂等键（租户，来源标识）。派发进程的路由表没有这个类型，每封都撞 `dispatch.no_subscriber`，直到失败预算耗尽落 ABANDONED。结果本身已在 CC 入库，但放行事实到不了 `visibility-exception`。

演示租户的里程碑映射种子登了一行（`CUSTOMS_COMPLIANCE`，`external-result`）→ `CUSTOMS_CLEARED`。没有任何适配器产出这个类型词；若照它造一个消费者，所有外部结果都会落成同一个类型词，监管接收、业务受理、税费核定一到就被这一行映成「已清关」——正是 CC CONTEXT「外部结果与规则时效」禁的那件事：「不能使用一个“清关成功”状态覆盖各层事实」。

几处权威已经给了答案的骨架：

- CONTEXT-MAP CC→VE 边是「关务事实」；UC-CC-006 交接表写明任一已接受关务事实以「来源、层次、范围、版本、有效性和更正关系明确的稳定引用」交给 UC-VE-001。
- VE CONTEXT「追踪投影与里程碑」：「已接受事实必须携带源上下文拥有的事实类型；标准里程碑映射按源上下文与事实类型版本化登记」，无法可靠映射时保持未归类。外部结果的层次就是 CC 拥有的那个分类。
- 按语义分支分开类型词有先例：TF 权威交接把已交接、拒收、待确认各译成自己的类型词，理由是共用一个词会让目录一行把它们归进同一里程碑。
- CC CONTEXT「放行结果」区分全部、部分和附条件；部分放行里未被放行的范围继续受门禁约束。
- ADR-0066 已裁多成员事实在消费侧逐成员拆分、成员维进事实引用。

难逆转的是类型词：它们是租户登记映射的键，登了之后改名要迁每个租户的映射。

## Decision

**一、`visibility-exception` 的追踪投影消费外部结果已收信封；CC 的放行门禁核对不订阅它。** 放行门禁核对按 CC CONTEXT 必须绑定拟执行动作与监管边界，由那次动作的核对请求驱动（与 ADR-0137 决定二「来源变化不触发后台重算」同一条纪律）；需要外部结果的执行方在动作时向 CC 问，不靠订阅这封信。只投 VE、不 FanOut。

**二、事实类型词按层分开，放行层再按种类分开。** 一封外部结果只派生它自己那一层，类型词取下表，表外即仓储不变量已破：

| CC 结果层 | 放行种类 | 事实类型词 |
|---|---|---|
| 监管接收 | — | `external-result-regulatory-receipt` |
| 业务受理 | — | `external-result-business-acceptance` |
| 监管过程决定 | — | `external-result-process-decision` |
| 监管核定税费 | — | `external-result-assessed-duty` |
| 放行结果 | 全部 | `external-result-release-full` |
| 放行结果 | 部分 | `external-result-release-partial` |
| 放行结果 | 附条件 | `external-result-release-conditional` |
| 监管处置决定 | — | `external-result-disposition-decision` |

不设任何跨层的类型词，也没有「清关成功」这个词。哪一行映成哪个标准里程碑归版本化映射目录；没登记即未归类，未归类条目不是可见里程碑。非放行层同样进投影：它们是已接受关务事实，运营追踪查阅要看得见关务进展的各层，租户要为某一层登里程碑时只登映射、不改代码。

**三、沿结果范围→申报单元→单元组成逐成员展开。** 结果范围指名申报单元（CC 接收编排沿同一条链取辖区），组成取单元本体。事实引用 `external-result/<成员>/<来源标识>`：成员维进引用（ADR-0066 决定二），来源标识是一次接收的身份。事实版本取结果关联的提交版本；接收记录只建立不改写，来源更正在 CC 是另一次接收、另一个来源标识，所以不登前身。

**四、三个时间。** 发生时间与有效时间同取 CC 保存的「业务发生或适用时间」——那是 CC 按已登记解释规则对这一层作出的时间判断，也是它选解释规则的评估时点，投影不替所有者另判有效时间（ADR-0102 决定六把关务分层接受外部结果的既有路径留在原处）。接收时间取 CC 落记录的时刻，不用信封时间。

**五、不派生与拒收的格。**

- 记了同层冲突的那一份：入账不派生。CC 双方事实都留存、当前判断不因后到的一方改变；投影收下它，映射一行就能让后到的一方改掉包裹当前的关务进展。冲突标记今天指不出与谁冲突，投影立不起冲突关系。
- 归属不上的留存：CC 不为它入队；信封指到它即仓储不变量已破，响亮失败。
- 放行层缺放行三件、非放行层带放行三件、放行范围与结果范围不符、范围单元答非所问或组成为空：仓储不变量已破，响亮失败。

**六、未决只给等得来的格。** 接收记录与范围单元读不动或读不回、派生编排停在自己的未决上，落 `dispatch.consumer_undecided`；其余失败保持 `dispatch.publish_failed`。

**七、参考映射只把全部放行映成清关。** 演示租户的映射（产品随附的参考配置，证据 `S`）只登（`CUSTOMS_COMPLIANCE`，`external-result-release-full`）→ `CUSTOMS_CLEARED`；部分、附条件放行与其余各层留未归类。别的租户登什么是租户取值。

## 越权风险点

1. **代 VE owner 定「非放行层也进投影」。** 票面倾向只派生放行层。若 owner 只要放行进投影，改的是适配器在层次分支上入账不派生，类型词表不动。
2. **代 CC owner 定类型词表。** 事实类型归源上下文拥有；词一经租户映射引用，改名要迁映射。
3. **部分放行逐成员展开。** 范围指名单元，CC 今天说不出部分放行具体放了哪几件；成员上的条目说的是「所在单元部分放行」，不是「本件已放行」。租户若把它映成清关就是错的映射；参考映射不这样登。
4. **监管过程决定、监管处置决定只有层级粒度。** 查验与扣留在类型词上分不开，把这一层映成具体里程碑会误导；要细分得等 CC 在层内给出分类。
5. **同层冲突不进投影，VE 也就看不见「关务进展待确认」。** 冲突在 CC 可审计；VE 侧的冲突信号（UC-VE-001 的 `CC-RESULT`）需要 CC 先给出冲突关系，另立票。
6. **组成取单元而不取提交版本快照。** 申报单元今天只建立不更新，两者相同；若将来单元组成在提交后可变，要回来重判取哪一份。
7. **有效时间等于发生时间。** 依赖 CC 只保存一个「业务发生或适用时间」；CC 若把适用时间单独存出，本适配器应改取它。
8. **CC 放行门禁核对不订阅。** 若 CC owner 要在外部结果到达时重核挂起的门禁，另立票；那是给这条路由加一路 FanOut，不改本记录的投影一路。

## Consequences

- `customs-compliance.external-result.received` 有了订阅者；可归属、无冲突的每封外部结果在范围单元每个成员上各派生一版投影，并经投影派生信封进客户视图链。
- 演示动线上，全部放行到达后成员投影归入 `CUSTOMS_CLEARED`；其余层与部分、附条件放行只作为未归类条目出现在运营追踪查阅里。
- 类型词表新增一层或一种放行时，要同时改这张表、适配器的分支与参考映射；表外的值按不变量已破处理，不会悄悄落成未归类。
- VE 侧关务冲突信号与执行方动作时向 CC 问放行，留给后续票。

## Alternatives considered

- **只派生放行层，其余层入账不派生（票面倾向）。** 否决：丢掉的是已接受关务事实，运营追踪查阅看不见受理、查验、税费各层；租户要为它们登里程碑得改代码。「不制造假的可见事实」靠的是映射未登记即未归类，不必靠丢事实。
- **单一类型词 `external-result`（原种子的假设）。** 否决：目录一行就把各层都映成清关，撞 CC CONTEXT。
- **类型词只按层、不按放行种类分。** 否决：部分与附条件放行会跟着全部放行一起被映成清关。
- **组成取提交版本的成员快照。** 否决：结果范围指名的是单元；CC 接收时没有核对版本与范围是否同属一个单元，按版本展开等于让 VE 去裁两者不一致时听谁的。
- **同层冲突的那份照常派生。** 否决：按最后到达改当前进展。
- **同层冲突的那份落未决。** 否决：CC 今天没有冲突的解开路径，这封信只会烧光失败预算后 ABANDONED，与「没有订阅者」同样吵。
- **由 CC 放行门禁核对订阅这封信。** 否决：见决定一。

## Links

- [关务与贸易合规上下文](../domain/customs-compliance/CONTEXT.md)：「外部结果与规则时效」「放行结果」「放行门禁核对」
- [全程追踪与异常上下文](../domain/visibility-exception/CONTEXT.md)：「追踪投影与里程碑」
- [UC-CC-006](../application/customs-compliance/UC-CC-006-RECEIVE-AND-RECONCILE-EXTERNAL-RESULTS.md)：交接表
- [UC-VE-002](../application/visibility-exception/UC-VE-002-BUILD-TRACKING-PROJECTION.md)：`AT-VE-039`（重放）、`AT-VE-042`（未归类）
- [ADR-0066](./0066-multi-object-envelope-unrolls-per-member-on-the-consumer-side.md)：消费侧逐成员拆分、成员维进引用
- [ADR-0102](./0102-external-fact-three-times-are-minted-by-ownership.md)：外部事实三个时间的归属
- [ADR-0137](./0137-customs-gate-judgments-are-registered-facts-driven-by-assessment-requests-payment-gate-rule-is-registered-and-funds-facts-are-minted-only-in-settlement-accounting.md)：门禁判断由请求驱动
- [ADR-0049](./0049-publish-channel-is-in-process-delivery-until-load-evidence.md)：接得住才登记
