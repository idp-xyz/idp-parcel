# 71 释放端口与控制端口改答封闭集，分已收口、可重试未形成、确定未受理三格

Category: bug
Status: ready-for-agent——R2（上线后尽快）：今天 PS 的释放端口只交回 `error`，可重试与确定性失败分不开。确定性失败（提供方拒收请求、作用域映射变了）被当成补偿续办或内部重试，永远收不了口：撤回、拒绝那几路留一条没人推得动的补偿引用；接受链对被取代版本的释放（code/20）则会挡住新版本，一直重试到信封死信。2026-10-21 协调方据 code/20 隔离评审立。
Blocked by: 无
出处：code/20 隔离评审（越权风险点 1、N6）；code/20 完成记录「评审记录」。

## 现象

- `ports.PreAcceptanceControlRelease.ReleasePreAcceptanceControl` 只答 `error`。`pssettlement.PreAcceptanceControlAdapter` 把 `ReleaseRequestNotAccepted` 译成 `ErrUntranslatableAnswer`、把作用域未配置译成普通错误，到了 PS 编排都只是一个 `err != nil`。
- 各释放点一律折成 `ControlReleasePending`。释放点：`FormAcceptanceDecisionHandler.releaseIfRejected`、`RejectShipmentRequestHandler`、`WithdrawShipmentRequestHandler`、`DisposeShipmentRequestHandler` 的释放、接受链的 `releaseSuperseded`。
  - 撤回、拒绝那几路：确定失败留一条补偿引用，之后无人能推。
  - 接受链：确定失败让新版本一直停在内部重试。
- 控制端口一侧（`ApplyPreAcceptanceFinancialControl`）的业务答复已是封闭集，但适配器的确定性错误（`ErrUntranslatableAnswer`）同样经 `error` 交回，`AdvanceFinancialControlJudgmentHandler` 一律折成 `FINANCIAL_CONTROL_UNAVAILABLE` 内部重试。两个端口同一个缺口。

## 证据

- `internal/parcelshipment/adapters/settlementaccounting/pre_acceptance_control.go` 的 `ReleasePreAcceptanceControl`：`ReleaseRequestNotAccepted` 那一格的注释已写明「不是一种可重试的未决」，交回的却仍是一个 `error`，调用方分不出来。
- [ADR-0081](../../../docs/adr/0081-acceptance-judgment-is-envelope-driven.md) 决定三：封闭集合外的格保持 `publish_failed` 响亮，折进未决会重投到失败预算耗尽。释放这一侧今天正是把确定格折进了未决。

## 风险（并入本票，评审 N6）

释放时现查作用域：`FormControlScope` 在释放那一刻重新解析商业依据、再查账户目录，而不是取施加时用过的那一份。施加之后若结算政策回显或账户登记变了，释放查到的是另一个作用域，SA 在那个作用域下找不到这一笔，答`无可释放`，PS 当成已收口。原占用静默成孤儿。修法候选：施加时把作用域随控制结果存下、释放按存下的那一份认领，或释放查到的作用域与施加时不同即答确定未受理。本票落地时一并定。

## 三类

机制。无产品策略、无租户取值。

## 规模

M：端口答复改形（三格），适配器逐格翻译，各释放点逐格分派；确定格响亮上抛，或落到人能处理的格，不进补偿续办。

## 最小的诚实修法

1. `ControlReleaseRequest` 的答复改成封闭集：`已收口`（已释放、无可释放）、`可重试未形成`（账本读不回或存不回、依赖答不出）、`确定未受理`（请求被拒收、作用域对不上）。
2. 适配器逐格翻译，不留兜底。
3. 控制端口同此：确定性的翻译失败不再折成 `FINANCIAL_CONTROL_UNAVAILABLE`。
4. 各释放点逐格分派：已收口不留引用；可重试照旧走 `ControlReleasePending`；确定未受理响亮上抛（照 ADR-0081 决定三）。
5. 接受链 `releaseSuperseded` 同改：确定未受理不再折成内部重试。
6. N6 那条作用域现查的风险按上一节定法。

## 要裁的

- 确定未受理在撤回、拒绝几路上是响亮上抛，还是另立一个等人处理的格：上抛会让已成立的撤回或拒绝答 5xx，落地时由 PS owner 选。接受链那一路照 ADR-0081 决定三上抛。
