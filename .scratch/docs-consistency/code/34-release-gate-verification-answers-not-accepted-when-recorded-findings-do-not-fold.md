# 34 CC 放行门禁核对在读回的认定折不出结论时答「未受理」

Category: bug
Status: in-progress——2026-10-27 会话 tops-20（idp-parcel-91）接手 tops-26 断在中途的「上线前必须」一组（冻结清单「六」），分支 claude-code34。此前 ready-for-agent——R3（不急）：放行门禁核对编排 `VerifyReleaseGateHandler` 在 `cmd/` 下没有装配，今天没有生产路径；动作准入同一情形已答读不动。接线之前修。2026-10-21 文档自洽 03 第二轮评审立（协调方裁定为代码问题，用户「你作为业务和系统专家自决」授权），分支 claude-dc03。
Blocked by: 无
出处：文档自洽 03 第二轮隔离评审（UC-CC-009 首发结果码对照补 `NOT_ACCEPTED` 两路时发现）。

## 现象

`internal/customscompliance/application/verify_release_gate.go` 的 `VerifyReleaseGateHandler.Handle`：`assessReleaseGate` 交回 `foldRefused`（`domain.FoldGateConclusion` 拒收读回的认定清单——册上的认定不合法）时，编排答 `NOT_ACCEPTED`。

- UC-CC-009「未受理」的定义是「请求主体、租户、法人、客户、案件、税费、付款事实或权限不能合法关联」，续办是改请求；而这一格的请求本身没错，是在册数据坏了，改请求换不来结果。
- 共用同一段 `assessReleaseGate` 的动作准入编排（`judge_action_admission.go`）对同一情形答读不动（`AdmissionGateConditionsUnavailable`）。同一事实两种答法。
- `releaseGateAssessment` 头注「两条编排各按自己的答复格归它，门禁读法不替它们选格」承认了两边分格，但门禁核对选的那一格与 UC 不符。

## 三类

机制。无产品策略、无租户取值。

## 规模

S：改一格答复，补用例。

## 最小的诚实修法

`foldRefused` 在门禁核对编排里答 `UNDECIDED` 并指名一个读不动 / 在册数据坏了的原因（可复用 `GATE_CONDITION_VIEW_UNAVAILABLE`，或另立一格说「认定读回折不出结论」），与动作准入的读不动对齐；或者照 ADR-0226 决定四「在册事实坏了是不变量已破」的先例整笔报错（500）。二选一由实施者按 ADR-0029（按恢复动作分格）判，写进用例。修后改 UC-CC-009 首发结果码对照里 `NOT_ACCEPTED` 那一行的现状注。
