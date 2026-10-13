# 27 受理前控制只认委托当前提交版本：已换代的版本不读申报、不形成冻结

Category: bug
Status: ready-for-agent——2026-10-13 19:3x 通道 1 立（用户 19:2x 经队列点头「按你的建议开始吧」），出自[票 25](./25-pre-acceptance-estimate-formed-by-settlement.md)两段非作者评审：第一段 Spec ③（`task-c7cfb347`）与第二段 Standards 1（`task-386c6296`），原文摘在票 25 的 Comments。
Blocked by: 无
地盘：`internal/parcelshipment/domain`（`SubmissionVersionDeclarationFor` 与其用例；`FormNewSubmissionVersion` 只读不改）、`internal/parcelshipment/adapters/postgres` 的提交版本申报读口（`submission_version_declaration_view.go` 与其用例）、`internal/parcelshipment/application`（守卫若落在编排一层，只动受理判断里发起财务控制的那一段与其用例）、`cmd/parcel-dispatch` 的纵向用例（只新增）。**不碰** `internal/parcelshipment/adapters/settlementaccounting/`——那是[票 28](./28-psb25-review-tails-estimate-gaps-unit-folding-and-trace-as-of.md)的地盘，真要动先报通道 1。
出处：[ADR-0179](../../../docs/adr/0179-pre-acceptance-control-amount-is-estimated-by-settlement-from-per-parcel-sell-trial-evaluations.md) 决定二「对委托当前提交版本的每一件声明包裹」、决定四「按委托当前提交版本读」；ADR-0045（新版本成为唯一待判断版本）；ADR-0094 决定一、二（`未形成`与`未配置`的恢复动作）。

## 现象（读码，钉 main `6e4d9a1d`）

- PS 领域 `ShipmentRequest.SubmissionVersionDeclarationFor` 的头注写「当前版本与此前各版都认，取哪一版由调用方指名」，PS 读口 `submission_version_declaration_view.go` 原样调它。
- PS `adapters/settlementaccounting` 的 `applyCommandFor` 把请求里的提交版本原样交给 SA，SA 施加编排据此要试算、形成估价、冻结。
- 续办经 `ListWaitingOnOperatorRegistration` 取 `current_submission_version_id`，这一路守住了当前版本。提交信封重投按信封里的版本走，而 `FormNewSubmissionVersion` 在`已提交`、未形成决定时允许换代（头注：新版本成为唯一待判断版本，旧任务转`已停止`）——这一路没守：旧版本的信封再投一次，会为一个不再待判断的版本读申报、占资金。已删的旧执行器 `estimation_amount.go` 遇版本换代即停。
- 今天采用册是空的，估价走不到读申报那一步，所以还没出错；[票 26](./26-demo-tenant-adopts-the-pre-acceptance-estimate-form.md)给演示租户登了采用行之后就走得到。

## 做什么

1. 先写 red：同一委托在`已提交`时换代，旧版本的提交信封再投一次，断言不为旧版本形成冻结或信用暴露、也不读它的申报。用例落在能复现这条路的最低一层（编排层或纵向真库，开工时核）。
2. 守住它。两处可落，选一处并写进判断项：
   - 读口一侧：`SubmissionVersionDeclarationFor`（或读口适配器）对非当前版本答不可见。SA 现有原因 `EstimateSubmissionNotVisible` 接得住，PS 映射里它落`未形成`。
   - 编排一侧：受理判断发起财务控制之前核信封版本是不是委托当前版本，不是就照旧任务`已停止`的现状收口，不发控制请求。

   取舍要答清两件：旧版本信封重投落哪一格、烧不烧失败预算（ADR-0094 决定一、二）；`SubmissionVersionDeclarationFor` 头注那句「此前各版都认」改不改、改成什么。
3. 当前版本那一路行为不变：既有用例全绿，经当前版本续办的纵向用例不动。

## 不做

- 不改 SA 施加编排、SA 原因集与 PS `notFormedOutcomeFor`（票 28 的地盘）。
- 不改 ADR-0179；读法与它对不上时停下报通道 1。
- 不碰 `scripts/demo-seeds`（票 26）。

## 完成判据

- 第 1 条的 red 修前红、修后绿；去掉守卫的变异（`go test -overlay`）转红。
- 选定的那一处写成判断项。改了 PS 领域语义（头注那句）或定了旧版本信封落哪一格的，标「越权风险点 · 待 owner 复核（PS）」。
- 架构门禁、机制清点、受影响包组绿，口径照 [parallel-sessions.md](../../../docs/agents/parallel-sessions.md)「全量只跑一次」。证据层级只记 `S`。
