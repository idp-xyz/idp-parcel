# 44 PS 撤回与资料修订：保全之后没落成的那一次，重放短路成「已有结果」，这次请求从此不会形成

Category: bug
Status: needs-triage——建议 R2（客户面撤回口、资料修订口接上首方 Intake 之前必修）：答错且静默。两只编排都先保全来源、再判断与保存；保全落下之后、撤回或版本没落成（保存失败、并发抢先，或业务未决），客户照 [ADR-0256](../../../docs/adr/0256-technical-non-formation-is-a-reason-of-undecided-not-an-outcome-of-its-own.md) 重放同一请求，编排认出来源已保全就走已有结果那条路：撤回对仍为`已提交`的委托答 `DECISION_ALREADY_FORMED` 却不带任何决定；资料修订答 `ALREADY_HANDLED` 却不带版本。这次请求从此不会形成。两口今天在生产装配下仍是 `UnconfiguredIntake`，接口接上即生效。2026-10-21 文档自洽 06 评审轮立（用户「你作为业务和系统专家自决」授权），分支 claude-dc06。分级由协调方定稿。
Blocked by: 无（撤回在真库上读不回的那一半归 [code/36](./36-withdrawn-shipment-requests-cannot-be-read-back-from-the-store.md)，宜同批）
出处：文档自洽 06 隔离评审阻断 B2；ADR-0256 决定一第 2 格「重放须接着办」；UC-PS-002、UC-PS-005「给开发的交接」。提交编排的同类问题归 [code/39](./39-a-preserved-submission-whose-build-failed-is-never-built-on-replay.md)，不在本票。

## 现象

来源保全一经落下，重放就被当成同一请求已经办过，不再判断、不再保存。可上一次在保全之后停下了，什么都没形成。

## 证据

- 撤回：
  - `cmd/parcel-api/assemble_withdrawal.go` 的 `transactionalWithdrawal` 头注：「编排交回业务答案（含未决与已有决定）时事务提交——被拦下的撤回也已保全来源」。所以撤回停在业务未决与读不回那几格（`SHIPMENT_REQUEST_UNAVAILABLE`、`WITHDRAWAL_AUTHORITY_UNAVAILABLE`、`WITHDRAWAL_AUTHORITY_RULES_NOT_CONFIGURED`、`DECISION_IDENTITY_UNAVAILABLE`）与 `STALE_SHIPMENT_REQUEST_REVISION`（保存答版本冲突，事务照常提交）时，保全照样提交。`DECISION_NOT_RECORDED` 走不到这里：保存的 SQL 出错后事务已中止，`Commit` 失败，整笔回滚、答 5xx，保全一并回滚，重放从头办。
  - `internal/parcelshipment/application/withdraw_shipment_request.go` 的 `resolvePreserved` 对同内容重放只追加观察、读委托、走 `existing`；`existing` 不论委托此刻是否仍为`已提交`都答 `WithdrawalDecisionAlreadyFormed`。委托仍为`已提交`时这一答复不带任何决定，违反 ADR-0256 决定三（不得答成已形成结果）。
- 资料修订：
  - 来源保全与版本落库是两层边界（`AmendmentHandler` 头注：「来源保全一层、版本落库一层」）。`Save` 失败答 `AMENDED_REQUEST_NOT_SAVED` 时保全已提交。
  - `internal/parcelshipment/application/amend_customer_source_data.go` 的 `resolvePreserved` 走 `existing`：在委托的资料版本里找不到这次请求身份的版本，就答 `AmendmentAlreadyHandled`、不带版本。
  - 同文件保存失败那一支的注释「重放这次修订时来源保全会认出同一个身份，走 existing」把这条短路当成了续办——这正是误判。来源保全自成一笔事务（`preservationBoundary`），所以授权、阶段、矩阵等业务未决与读不回之后重放，同样走这条短路；允许声明未声明答等复核之后的重放同样答 `ALREADY_HANDLED`、不带版本。
  - 停在 `SOURCE_DATA_VERSION_NOT_HANDED_OFF` 的那一格不在其中：版本已落库，`existing` 读回版本并重发交接意图，是接着办了的。
- 对照：受控补充编排 `FormNewSubmissionVersionHandler` 重放时接着办。

## 三类

机制。无产品策略、无租户取值。

## 规模

M：两只编排的重放路径分出「保全过、已形成」与「保全过、没形成」两格，后一格按首次请求接着办；补用例（每一种未决之后重放都能形成，且只形成一次）；改那段注释。

## 最小的诚实修法

1. 撤回：重放时按委托状态判——已撤回、已接受或已拒绝，答已有决定并带上那份决定；仍为`已提交`，就按首次请求继续走授权与决定提交。不按请求身份找撤回：`domain.Withdrawal` 不带请求身份。
2. 资料修订：重放时在委托的资料版本里找这次请求身份的版本，找到照旧 `ALREADY_HANDLED` 带版本；找不到就按首次请求继续走授权、阶段、矩阵与保存。
3. 改 `amend_customer_source_data.go` 保存失败那一支的注释：重放要靠第 2 条接着办，不是靠来源保全认出身份。
4. 用例：撤回在授权未决、`STALE_SHIPMENT_REQUEST_REVISION` 之后重放形成撤回（`DECISION_NOT_RECORDED` 整笔回滚，不在本票）；资料修订在 `AMENDED_REQUEST_NOT_SAVED`、授权未决之后重放形成版本；各自重复重放只形成一次。

## 要裁的

无（ADR-0256 决定一第 2 格已定「重放须接着办」）。
