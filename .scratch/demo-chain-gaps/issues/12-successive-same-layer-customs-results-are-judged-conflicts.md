# 12 同层先后到达的关务结果一律判冲突：先部分放行、后全部放行时，全部放行被吞

Category: bug
Status: needs-triage——2026-10-19 Claude Code 会话立（用户「你作为业务和系统专家自决……解决所有的问题」授权）。出自 [11](./11-external-result-received-has-no-subscriber.md) 的隔离评审。
Blocked by: 无
出处：[ADR-0197](../../../docs/adr/0197-customs-external-results-enter-tracking-projection-per-layer.md) 越权风险点 5。

## 现象

CC 收外部结果时，同层、同提交版本、同范围下只要原始语义不同就判同层冲突。按 ADR-0197，冲突的那份不进 VE 投影。由此：

- 先部分放行、后全部放行（剩余范围后来也放了）：后到的全部放行被判冲突，成员永远到不了 `CUSTOMS_CLEARED`。
- 同一层先后几次不同的监管过程决定（例如先查验、后补资料）：第二次起都被吞掉。

## 证据（读码）

`internal/customscompliance/domain/external_result.go` 的 `CheckLayerConsistency` 逐一比对同层既有事实；`internal/customscompliance/application/receive_external_result.go` 遇 `domain.ErrLayerConflict` 把新到那份记为冲突。VE 的 `internal/visibilityexception/adapters/customscompliance/derive_on_external_result.go` 对冲突记录入账、不派生。

## 三类

机制（同层后续事件与矛盾怎样分）。规模 M。

## 最小的诚实修法

CC 区分「同层后续事件」与「同层矛盾」：例如放行层按范围单调推进（部分 → 全部）算后续，不算冲突；监管过程决定按发生时间成序列。真正的矛盾（同一时刻同范围两种结论）照判冲突。VE 侧的冲突信号（UC-VE-001 `CC-RESULT`）要 CC 先给出冲突关系，可并在本票或另拆。

## 要裁的

放行层的单调推进怎样判（范围包含关系、放行种类的偏序）；监管过程决定是否天然是序列。归 CC owner。CC CONTEXT「与同层现有事实冲突时，不得据此猜测提交、补造缺失层次或按最后到达直接改变当前判断」那句的边界要一起说清。

## 既有票

none
