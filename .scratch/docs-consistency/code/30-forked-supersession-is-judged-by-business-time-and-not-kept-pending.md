# 30 VE 替代链分叉按业务时间裁决、裁得出就不立信号，两个后继照常派生，分叉也不随投影版本存下

Category: bug
Status: ready-for-agent——R3（不急）：答错，但今天的生产源造不出分叉——TF 交接的更正链是线性的（`RegisterTransportHandoverHandler.CorrectWithGateReading` 遇前版已有后继即答占用，「一版最多被更正一次」），其余各源未逐一核。在第一个可能分叉的源接上之前修好即可。2026-10-21 文档自洽 04 立（用户「你作为业务和系统专家自决」授权），分支 claude-dc04。
Blocked by: 无
出处：[audit/ve.md](../audit/ve.md) VE-04（归 B）；裁定见 [ADR-0245](../../../docs/adr/0245-a-forked-supersession-chain-is-never-ordered-by-business-time-and-stays-pending-in-the-projection-version.md)。取证于 main `205eb254`。

## 现象

- `application.DeriveProjectionHandler.judgeForks` 对每处分叉调 `domain.ResolveByBusinessTime`。两个后继的业务发生时间不同，`judgment.Resolved()` 就为真，然后 `continue`：不立信号，也不进 `DeriveProjectionResult.Conflicts()`。
- 两个后继都在当前有效集里（`domain.CurrentlyEffective`），照常归类、照常派生里程碑，投影上同时有两个互斥结果。
- 分叉只在当前有效集里找（`domain.SupersessionForks(effective)`）。源上下文日后更正其中一支，那一支的旧链头离开当前有效集，分叉就检测不到了，可两个链头其实仍都当前有效。
- `application.FactConflict` 只随本轮结果返回，不持久化；冲突信号规则未配置时，分叉什么都留不下。

## 证据

- 文档：ADR-0245 决定一至三；VE CONTEXT「追踪投影与里程碑」替代链分叉一条（本票同批改写）；ADR-0065 决定三、六、七；UC-VE-002 `AT-VE-043`。
- 代码：`judgeForks`、`raiseConflict`、`domain.SupersessionForks`、`domain.CurrentlyEffective` 头注「投影据此把各方都摆在场、按信息待确认表达」——它其实没有表达待确认。
- 测试：`derive_projection_test.go::TestAForkResolvableByBusinessTimeRaisesNoSignal` 断言「业务时间排得出先后的分叉不是异常」；`TestAForkedSupersessionKeepsBothSuccessorsInTheProjection` 断言双方都是派生条目。

## 三类

机制：分叉判法、投影呈现、分叉随版本留存。产品策略：分叉不自动解、走信号与案件（ADR-0245 决定四）。租户取值：冲突信号规则（`PAR-VIS-04`），不在本票改。

## 规模

M：动 `judgeForks`、投影条目形状（分叉标记）与派生过滤、投影库序列化、运营查阅读口转写；客户视图里程碑维的待确认与 [06](./06-customer-view-shows-dimensions-without-a-visible-judgement.md) 同一处。与 [demo-chain-gaps/117](../../demo-chain-gaps/issues/117-projection-entries-are-stored-without-their-supersession.md)（被替代条目进投影版本）改同一组文件，宜同批或紧接。

## 最小的诚实修法

1. `judgeForks` 不再调 `ResolveByBusinessTime`；每处分叉都按无法裁决处理，照现有 `raiseConflict` 立适用信号。
2. 分叉按链头找：在全量事实上，同一（源上下文，事实引用）下当前有效的链头多于一份即未解。
3. 前身与各后继、各链头都进投影版本，分别标为已被替代、分叉待确认，都不参与派生；那一条事实流所在的维度答信息待确认，同包裹其他事实流照常。
4. 分叉关系（前身、各后继、各链头）随投影版本落库、可按版本读回；运营追踪查阅转写出来。
5. 客户视图：投影带未解分叉时，受影响的里程碑维答待确认（UC-VE-008 `AT-VE-163`）。
6. 测试：上列两个测试改写为断言新判法；补「更正其中一支后仍待确认」一例。改测试的 Covers 照点 `AT-VE-043`。

## 要裁的

无（ADR-0245 已裁）。

## 追记（2026-10-26，票 demo-chain-gaps/135）

票 135 给信号发作期补了因果条件与事实依据（VE 迁移 0030），冲突信号的因果条件定为分叉的那一条事实流（`domain.SupersessionForkCausalCondition`：源上下文 + 事实引用），依据是判断保留下来的各方。冲突信号的**恢复边界**留给本票：分叉按链头判定落地之后，同一事实流的当前有效链头回到一份即分叉已解，据此结束这条事实流的冲突发作期并保存解除依据（CONTEXT「条件明确解除后，该发作期结束」；「防抖、宽限和恢复边界必须具有规则版本」——边界随冲突信号规则那一版）。在今天的判法下不能这么做：分叉只在当前有效集里找，更正其中一支后就找不到了、两个链头却仍当前有效，拿「找不到了」结束发作期就是虚构一次恢复。`application.DeriveProjectionHandler.raiseConflict` 的头注与 UC-VE-004「给开发的交接」都已指向本票。
