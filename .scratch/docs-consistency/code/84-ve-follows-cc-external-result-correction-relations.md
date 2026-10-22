# 84 VE 跟随 CC 外部结果更正关系

Category: enhancement
Status: ready-for-agent——R2（上线后尽快）：CC 已形成层内更正关系（[code/04](./04-customs-external-results-have-no-in-layer-correction-relation.md)），当前判断按更正取；VE 运营投影仍把生效的更正当作一条独立条目派生，被更正的原条目照旧当前有效。客户视图今天不列里程碑明细，演示映射下改前改后客户面输出相同，所以不列 R1；接真实关务程序、或客户视图开始按条目披露之前升 R1。2026-10-21 协调方据 code/04 隔离评审立（作者子代理代笔；用户「你作为业务和系统专家自决……解决所有的问题」授权）。
Blocked by: [code/04](./04-customs-external-results-have-no-in-layer-correction-relation.md)（进 main 之后开工）
出处：code/04 完成记录的代决「VE 不跟随更正关系」与越权风险点「CC 当前判断与 VE 运营投影分叉」；code/04 隔离评审 N1。

## 现象

CC 收到来源明确指名所更正那份的同层结果时，记下更正关系（`ExternalResultRecord.CorrectionInForce`、`Result.Corrects()`），放行读口按时点取：更正发生之后以更正后的结果为准。VE 的外部结果适配器按 [ADR-0197](../../../docs/adr/0197-customs-external-results-enter-tracking-projection-per-layer.md) 决定三的取法，事实引用是 `external-result/<成员>/<来源标识>`、版本取提交版本、不登前身。更正有它自己的来源标识，于是落在另一个事实引用下；VE 的替代关系只在同一事实引用的版本之间成立（VE CONTEXT「来源事实替代关系」），登不出前身。

结果：放行收窄更正之后，CC 放行读口只交出部分放行，VE 运营投影里原全部放行那条仍当前有效、仍按映射归入 `CUSTOMS_CLEARED`，另多一条部分放行条目；核定税费、处置决定的更正同样两条并存。新出的是 CC 当前判断与 VE 运营投影分叉。

## 证据

- 现状钉：`internal/visibilityexception/adapters/customscompliance/derive_on_external_result_correction_test.go::TestAnInForceCorrectionIsDerivedAsAnIndependentEntryWithoutAPredecessor`——生效更正作为独立条目派生、不带 Supersedes，原条目仍归入清关。本票落地时翻这一条。
- ADR-0197 决定三：「事实版本取结果关联的提交版本；接收记录只建立不改写，来源更正在 CC 是另一次接收、另一个来源标识，所以不登前身。」
- ADR-0226 决定三：「VE 一侧不必改。投影把每一份未记冲突的接收各派生成本层条目（ADR-0197 决定三）」。
- VE CONTEXT：「源上下文随更正给出替代关系 → 登记该关系，被指名的前身条目转为已被替代并继续保留」。

## 三类

机制：替代关系怎样从 CC 更正关系译成 VE 的事实引用与版本。产品策略：无。租户取值：无（里程碑映射照旧是租户取值，不随本票变）。

## 规模

M：一篇新 ADR 加适配器改动。新 ADR 部分停用 ADR-0197 决定三中事实引用、事实版本与前身的取法，视裁法而定；「要裁的」第 4 件按推荐裁时还触及 ADR-0226 决定三「投影把每一份未记冲突的接收各派生成本层条目」。代码只改 VE 消费 CC 外部结果的适配器与它的用例，CC 一侧已给出需要的全部字段。

## 做法

1. 起草新 ADR，裁下「要裁的」各件，并照 [adr/README](../../../docs/adr/README.md) 顶部写法在被部分停用的 ADR 上挂前向指针。ADR 号向编号分配方取，不自取。起草与接受由协调方代决（用户已授权自决），越权风险点照常列出。
2. ADR 接受后改适配器，把 `TestAnInForceCorrectionIsDerivedAsAnIndependentEntryWithoutAPredecessor` 翻成新口径，并补真库派发用例。

## 要裁的（新 ADR）

1. **事实引用落链根**：生效的更正改落在它所更正那条链的链根的事实引用下（`external-result/<成员>/<链根来源标识>`），还是另立一种引用并在 VE 侧放开跨引用替代（后者要改 VE CONTEXT，推荐前者）。
2. **链根怎么取**：适配器沿 `Corrects()` 逐份回读到第一份不带生效更正的记录；还是由 CC 在记录上另存链根，省掉回读。回读的终止与坏链（读不回、成环）归不变量已破还是续办。
3. **事实版本**：链根照旧取提交版本（存量不动）；链上各份怎样取一个在同一引用下唯一、稳定的版本（例如提交版本加来源标识），前身指名哪个版本。
4. **哪些行登替代**：只有 `CorrectionInForce` 的更正登前身；范围不等（交集为空）、记了冲突、指向已被更正过的那份（链尾冲突）这几种更正各怎样派生——照独立条目、不派生，还是另立冲突关系。
5. **时点**：替代在 VE 上从更正的业务发生时间起生效，还是派生即替代；被替代条目在更正之前的投影版本里怎样读回（与 CC 放行读口按时点取对齐）。
6. **客户视图换代**：原条目转为已被替代时，客户全程追踪视图是否形成新版本、标明更正关系（VE CONTEXT「来源更正……必须重新派生客户视图并形成追加更正或明确替代关系」），已发布的清关怎样以追加更正表达。
7. **存量**：本票之前已按独立条目派生的更正怎样处理——重投重派生、写迁移回填前身，还是首发前随演示库重灌（ADR-0226 决定五同一取舍）。
8. **过渡期**：本票落地前接收的更正，投影上原条目与更正并存；是否要一个运营可见的提示，还是只在完成记录里记风险。
9. **与冲突关系的合并**：票 [demo-chain-gaps/124](../../demo-chain-gaps/issues/124-customs-layer-conflict-relation-and-tracking-signal.md) 要给 VE 立「关务进展待确认」信号；分叉的更正（链尾冲突那份）与同层冲突是否走同一条冲突关系与信号。
