# ADR-0066: 多载运对象信封在消费侧按成员循环拆分；成员维进事实引用，不进事实类型

Status: Accepted  
Date: 2026-08-19

## Context

事实源勘察把 `transport-fulfillment` 四条（`exception-journey.recorded`、`disposition-execution.recorded`、`regulatory-acceptance.recorded`、`transport-commission.submitted`）与 `customs-compliance` 两条（`declaration-submission.formed`、`customs-case.established`）判为「需裁定」，全卡同一件事：按信封键重读回的本体带成员清单（`[]CarriedObjectReference` 或包裹级引用），而现行 VE 链一封信派生一个包裹的投影。这六条没有对象级替代品，不裁就永远到不了 VE。裁定备料（[备料报告](../../.scratch/envelope-n-object-ruling/report.md)）查明三件事。

**一、「一封信一对象」不是用例或 ADR 的硬句，是两处消费者注释确立的实现纪律，且两处理由不同。** `psinbox` 揽收消费者说的是消费门形状（入账/回滚两格表达不了成员级部分结果），`veinbox` 同名消费者说的是双登记去重（对象级 `.registered` 已覆盖同批事实）。后者对六条不适用——无对象级替代品就不存在「派生两次」；前者才是循环拆分真正要面对的形状问题。[UC-VE-002](../application/visibility-exception/UC-VE-002-BUILD-TRACKING-PROJECTION.md) 与 VE CONTEXT 锚的都是投影粒度不是派生次数：`DeriveTrackingProjection` 逐条目校验包裹一致，「一份投影装 N 个包裹」在构造器上立不住，但没有任何一层挡「一封信循环派 N 条单包裹命令」；`AT-VE-045`/`AT-VE-046` 本就预期共同来源、多包裹、分别投影、按客户隔离。

**二、循环拆分与「成员维进事实引用」是绑死的一对，拆开必炸且炸法静默。** 事实幂等键是（租户+源上下文+事实引用+来源版本），内容指纹含包裹维。若一封信循环 N 个成员而事实引用不带成员维，第一个成员落库后，第二个成员同键异指纹撞 `FactSourceConflict`，而消费翻译把 `SOURCE_CONFLICT` 列为业务终局入账——信封入账，后 N-1 个成员的投影永不派生，无错误、无重试、无日志分格。这正是自己制造 `AT-VE-040`（同一来源身份携带不同对象范围 → 冲突待确认）。

**三、六条事实的成员语义差异大到不该同一个答案。** `regulatory-acceptance` 的拒接格结构上零对象（DECLINED 不带对象、部分承接的未承接对象连清单都没有）；`transport-commission` 有「提交≠取得控制」的里程碑资格坎（TF CONTEXT：执行准备不得提前制造实际履约段）；`disposition-execution` 与 `exception-journey` 共用聚合、同键同批成员。该不该进投影是逐条的业务问题，用哪个形状拆是一次答完的机制问题。

## Decision

**一、多载运对象的已接受事实在消费侧按成员循环拆分。** 消费适配器按信封键重读提供方本体，对成员清单逐成员构造单包裹派生命令。信封形状一字不动、不塞成员快照，提供方不为 VE 增发对象级信封。

**二、成员维必须编进 `SourceFactReference`，不得编进 `SourceFactKind`。** 进引用是循环拆分的成立前提（Context 第二件事）；进类型则按对象身份分裂事实类型，重蹈「映射目录按事实引用建目录」的不可填表——目录键按（租户+映射版本+源上下文+事实类型），一行覆盖同类型事实。引用前缀照 `transport-handover/` 等五个现行前例构造，具体字面量归各接入票。

**三、消费门两格的表达边界如实承认，不为成员级部分终局扩格。** 单成员的业务终局（冲突、不接受）译入账、循环继续，与单对象信封同一口径——冲突在事实库有行（`AT-VE-040` 格），不失审计；某成员持续未决则整封回滚重投，头端阻塞（一个成员卡全信）是已知代价，换来的是整封事务原子、无部分状态。负载证据出现前不议成员级账本。

**四、逐条接入裁定。**

- `exception-journey.recorded`：**接**。
- `customs-case.established`：**接**。事实引用须含案件维——同一包裹可关联多个彼此独立的案件，缺案件维同包裹的两条案件事实互撞。
- `declaration-submission.formed`：**接**。前置核证已毕（[核证报告](../../.scratch/declaration-envelope-version-dedup/report.md)）：信封 ID 无版本维的吞版本坑机制坐实，但今天无触发路径，引信在「原案内更正」编排落地那天——信封 ID 修复挂那张票，不阻塞本条接入；接入票按载荷已带的版本维立 inbox 账。
- `disposition-execution.recorded`：**缓**。与 exception-journey 共用聚合、同键同批成员，两条都接同批成员各派生一次；其语义是「处置已执行」，消费本意是 CC 侧处置执行核对，等那条消费方向明确再裁。
- `transport-commission.submitted`：**缓**。「提交≠取得控制」在源语义上有硬对照，映成任何里程碑都不是 VE 单方能定的映射语义；等映射目录的语义有主再接。
- `regulatory-acceptance.recorded`：**不接，退回 TF**。拒接格零对象、循环体零次，「监管处置被拒」在这份聚合上没有对象可锚；若业务要它可见，那是 TF 侧对象级登记的票（`offsite-pickup.registered` 模式），不是 VE 消费侧拆得出来的。

**五、两处「一封信一对象」注释改口径。** 该纪律约束的是「存在对象级替代品」的选择场合——有 `.registered` 就不认 `.formed`；对无对象级替代品的多成员事实，按本记录在消费侧循环拆分。`psinbox` 的消费门形状理由与 `veinbox` 的双登记去重理由各自保留，但都不得再被读成对循环拆分的禁止。

## Consequences

- 首批接入票：exception-journey 与 customs-case 的 inbox+adapter（消费侧 A 类，互不占调度器接线）；declaration-submission 随后。新消费适配器的测试对 [ADR-0065](./0065-projection-versions-are-append-only-and-supersession-is-source-given.md) 之后的 `ProjectionStore` 接口写。
- 调度器登记（B 类）逐张排，一次只一张占 `assemble.go`。
- CC 修信封 ID 票挂「原案内更正」编排落地之后，先立案不动工。
- N 成员单事务的性能形状（N 次重归类、N 个投影版本、N 份意图）无上限约束，属结构推断——与 [ADR-0049](./0049-publish-channel-is-in-process-delivery-until-load-evidence.md) 的消息中间件同一等法，负载证据出现时再议。
- `disposition-execution` 与 `transport-commission` 保持未登记（`no_subscriber` 如实可见，ADR-0049 口径），不为了「先接上」登记接不住语义的消费者。

## Alternatives considered

- **形状 B：提供方拆，TF/CC 增发对象级信封（照抄 `offsite-pickup.registered` 模式）。** 否决其作为本轮总形状：六条各立新事件类型、新出账口，工作量与评审面数倍于消费侧循环；CC 侧要 owner 先承认「成员关联」是独立可发布事实，语义发明风险大。否决的是「六条全走提供方拆」，不是这条路本身——regulatory-acceptance 的「被拒可见」若立票，走的正是它。
- **形状 C：拒接，六类不进 VE 投影。** 否决。六条无对象级替代品，拒接等于异常旅程、申报提交、案件建立这些核心可见性素材永远到不了 VE；UC-VE-002 边界句没有把多成员事实排除在「范围明确」之外，VE CONTEXT 的消费面也明文列着运输委托。
- **成员维进 `SourceFactKind`（每对象一个类型）。** 否决。分格依据是语义分支，从不是对象身份；按对象分裂类型等于按引用建目录，重蹈已裁定的不可填表。
- **为成员级部分终局扩消费门格。** 否决，不预先建模。今天没有负载或运营证据要求成员级账本，冲突已在事实库可审计；预扩格是给还不存在的问题定形状。

## Links

- [UC-VE-002](../application/visibility-exception/UC-VE-002-BUILD-TRACKING-PROJECTION.md)：`AT-VE-040`（同来源身份异对象范围即冲突）、`AT-VE-045`/`AT-VE-046`（共同来源分别投影、跨客户隔离）
- [ADR-0025](./0025-cross-context-adapters-live-on-the-consumer-side.md)：跨上下文适配器在消费方侧——循环拆分正是消费侧翻译的一部分
- [ADR-0043](./0043-publish-intent-claimed-by-result-identity.md)：发布意图由结果标识认领——每成员一份投影意图，重放重发同一份
- [ADR-0049](./0049-publish-channel-is-in-process-delivery-until-load-evidence.md)：接不住不登记；未登记消费者如实 `no_subscriber`
- [ADR-0065](./0065-projection-versions-are-append-only-and-supersession-is-source-given.md)：投影版本只增不改写——新消费者的存储面
- [全程追踪与异常上下文](../domain/visibility-exception/CONTEXT.md)：「集运、装载或同批关系不合并包裹身份和客户轨迹」
- 裁定备料：[.scratch/envelope-n-object-ruling/report.md](../../.scratch/envelope-n-object-ruling/report.md)；信封 ID 核证：[.scratch/declaration-envelope-version-dedup/report.md](../../.scratch/declaration-envelope-version-dedup/report.md)
