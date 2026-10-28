# 34 CC 放行门禁核对在读回的认定折不出结论时答「未受理」

Category: bug
Status: in-progress——2026-10-28 作者完工，待隔离评审与进 main，分支 claude-code0334（代码最后一笔 1d2d457b9）。此前 in-progress——2026-10-27 会话 tops-20（idp-parcel-91）接手 tops-26 断在中途的「上线前必须」一组（冻结清单「六」），分支 claude-code34。此前 ready-for-agent——R3（不急）：放行门禁核对编排 `VerifyReleaseGateHandler` 在 `cmd/` 下没有装配，今天没有生产路径；动作准入同一情形已答读不动。接线之前修。2026-10-21 文档自洽 03 第二轮评审立（协调方裁定为代码问题，用户「你作为业务和系统专家自决」授权），分支 claude-dc03。
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


## 完成记录（2026-10-28，作者子代理，分支 claude-code0334）

起点 origin/main `94ef6908b`（本票在 code/03 两笔之后）。提交见 Status 行。

### 改了什么
- `VerifyReleaseGateHandler.Handle`：`foldRefused` 改答 `UNDECIDED` + `GATE_CONDITION_VIEW_UNAVAILABLE`，不落册、不发意图；`releaseGateAssessment` 头注与处注写明归法。
- UC-CC-009 首发结果码对照：`NOT_ACCEPTED` 行去掉现状注与缺陷链接，`GATE_CONDITION_VIEW_UNAVAILABLE` 行补「认定清单折不出结论同归此格」。
- 测试 `TestFindingsThatDoNotFoldAnswerUndecidedNamingTheGateCatalogue`。

### 代决与越权风险点
1. 二选一取「答未决、复用 `GATE_CONDITION_VIEW_UNAVAILABLE`」：与动作准入同一情形已同答此原因码（AT-CC-424），门禁读口读到坏行本就归它；恢复动作同为运维去救登记，按 ADR-0029 同格。未另立原因码（ADR-0243 已列封闭码表，新码要动 ADR）；未走整笔 500。
2. 复用意味着该格对运维不区分「读口故障」与「册上认定坏」，要区分需另立码与 ADR，留待有需要时。

### 判据对用例
UC-CC-009「未受理」只指请求不能合法关联：已对上。接线：票面未要求，`VerifyReleaseGateHandler` 仍未在 `cmd/` 装配，未接。

### 自验
`go build ./...`、`go vet ./internal/customscompliance/...`、`gofmt -l` 无输出；带 PG DSN `go test -count=1 -p 1` 跑 `./internal/customscompliance/...`、`./internal/architecture/...`、`./cmd/parcel-api/...` 全绿。

### 留给协调方
- 无新发现。`.scratch/docs-consistency/issues/03-cc-docs.md` 等处对 code/34 的历史引用未动。
