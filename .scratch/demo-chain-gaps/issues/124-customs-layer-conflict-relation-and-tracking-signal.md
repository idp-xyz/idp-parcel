# 124 关务同层冲突说不出与谁冲突：CC 不给冲突关系，VE 立不起「关务进展待确认」信号

Category: enhancement
Status: needs-triage——2026-10-21 Claude Code 会话立（用户「你作为业务和系统专家自决……解决所有的问题」授权）。出自 [12](./12-successive-same-layer-customs-results-are-judged-conflicts.md) 的作者裁定（ADR-0226 决定六）。
Blocked by: 无（[12](./12-successive-same-layer-customs-results-are-judged-conflicts.md) 进 main 之后开工，冲突的判据以它为准）
出处：[ADR-0197](../../../docs/adr/0197-customs-external-results-enter-tracking-projection-per-layer.md) 越权风险点 5 前半；[ADR-0226](../../../docs/adr/0226-successive-same-layer-customs-results-are-successors-unless-they-contradict.md) 决定六与越权风险点 8。

## 现象

CC 判出同层冲突后，只在新到那份接收记录上打一个布尔标记，说不出它与哪几份在册事实矛盾。VE 消费外部结果时，对冲突那份入账不派生（ADR-0197 决定五）。结果是 VE 看不见关务进展待确认：UC-VE-001 交接表里的 `CC-RESULT`（「已接受、分层且可更正的关务外部结果」）要形成异常信号时，没有冲突关系可读。运营追踪查阅也看不出某个成员的关务进展存在冲突。

## 证据（读码）

`internal/customscompliance/domain/external_result.go` 的 `CheckLayerConsistency` 只交出 `ErrLayerConflict`，不交对方；`internal/customscompliance/ports/ports.go` 的 `ExternalResultRecord.LayerConflict` 是布尔；`migrations/customs_compliance/0001_external_result.sql` 只有 `layer_conflict` 一列。VE 的 `internal/visibilityexception/adapters/customscompliance/derive_on_external_result.go` 遇 `record.LayerConflict` 直接返回。

## 三类

机制（冲突关系的形状与 VE 信号的形成）。VE 信号规则的取值归租户（是否对关务冲突形成信号、给谁），沿 VE 冲突信号规则那一册的做法登记，未登记答未配置。规模 M。

## 最小的诚实修法

两半：

1. CC：同层一致性核对交出新到那份与之矛盾的在册事实（来源标识集合）；接收记录连同冲突关系落库（迁移另取号）；UC-CC-006 结果契约「外部结果冲突」要的「全部来源事实……冲突前当前有效判断」按它可查。
2. VE：外部结果适配器对冲突那份不派生投影条目（ADR-0197 决定五不变），改为按冲突关系形成「关务进展待确认」一类的异常信号，逐成员展开同 ADR-0197 决定三。规则未登记答未配置，不默认形成。

## 要裁的

- 冲突关系只记在后到那份上，还是双向可查（先到那份也能答「与谁冲突」）。
- 存量：ADR-0226 施行前已记冲突的记录没有对方。回填按新判据重算对方，还是标「对方不明」。
- VE 信号的事实类型词与 UC-VE-001 的信号种类怎么对上；冲突裁决之后信号怎么收。

## 既有票

none
