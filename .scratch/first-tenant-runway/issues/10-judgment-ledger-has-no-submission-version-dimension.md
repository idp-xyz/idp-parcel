# 判断账没有提交版本维：受控补充后续办重判可能被旧判断压住，链再次停在`等待受控补充`

Category: bug
Status: draft——通道 6 于 ftr/09（ADR-0106）实施中量到，只写事实与两条可能形状，不裁；等 owner 裁归属与形状后转 ready-for-agent
Blocked by: 无（裁决前不动判断账）

## 量到的事实（锚 `mcp6-ftr09` 分支已验 tip `90260aae`，读的都是 main 上早已有的代码）

1. **判断账的键没有版本。** `migrations/parcel_shipment/0005_acceptance_judgment_task.sql` 里 `acceptance_reachability_judgment` 的主键是（`tenant_id`, `shipment_request_id`, `parcel_id`, `as_of_at`）；同文件头注明写「没有 submission_version 列……多版本之后旧版本的判断会被读进新版本的决定，这是 ADR-0045 明写的已知后续项（读口要长出版本维度……随编排切片一起收）」。
2. **写口撞键即静默重放。** `AcceptanceJudgments.RecordReachabilityJudgment` 用 `ON CONFLICT DO NOTHING`——同成员同时点的第二份判断不落库也不报错（设计如此：重放折成幂等）。
3. **读口按时点取最新，不看版本。** `loadReachabilityJudgments` 是 `DISTINCT ON (parcel_id) … ORDER BY parcel_id, as_of_at DESC`；`LoadRecordedJudgments` 只收（租户，委托），`RecordedJudgmentReader` 端口签名里没有提交版本。
4. **时点由规则包声明的策略形成，不由本进程定。** `AdvanceAcceptanceJudgmentHandler.Handle` 每一拍都问权威，时点来自 `FormJudgmentAsOf`（`JudgmentAsOfQuery` 带 `SubmissionVersion`，但策略要不要用它由 party-commercial 那侧的声明决定）。
5. **ADR-0106 之后这一格第一次有了生产路径。** 受控补充形成新版本 → 「新提交版本已形成」信封 → `SubmissionVersionFormedConsumer` 拿新版本再驱同一条链。若策略对两版给出**同一时点**：新一拍的`可达`判断撞键被吞，`FormAcceptanceDecisionHandler` 读回的仍是旧的`证据不足`，`Decide` 再次写下 `waitingOn = CUSTOMER_SUPPLEMENT`，按 ADR-0106 Decision 一入账——**不烧失败预算，但客户的补充推不动它**，且队列上它看起来与「还没补」一模一样。
6. **本仓的闭环用例是怎么过的。** `cmd/parcel-dispatch/customer_supplement_resume_loop_test.go` 的商业依据替身对新版本把时点推后一小时（文件头写明理由），新判断因此成为新行。这是替身策略，不是机制保证。

## 两条可能的形状（不裁）

- **甲、判断账长出版本维。** 主键与读口都带提交版本：记判断时带上本拍的 `SubmissionVersion`，读口按（租户，委托，**当前版本**）取行，旧版本的判断留在库里只作历史。代价：迁移改主键、`RecordedJudgmentReader` / `AcceptanceJudgmentRecorder` 端口加参、`FormAcceptanceDecisionHandler` 与两条腿的调用点全跟；它正是 0005 头注预告的那条后续项，也与 CONTEXT「新版本成为唯一待判断版本，旧版本及其判断历史继续保留」逐字对得上。
- **乙、把「时点随版本变」立成规则包侧的约束。** 在 party-commercial 的时点策略上要求（或至少在登记时校验）同一委托的不同提交版本形成不同时点，判断账不动。代价：把一条 parcel-shipment 的正确性前提交给另一个上下文的实例参数守，登记侧要多一道校验，而任何一份声明「以首次提交时刻为时点」的策略都会立刻踩回事实 5。

哪一条、以及甲的读口该按「当前版本」还是「当前及之前所有版本里最新」取，归 parcel-shipment owner 裁；裁时对照 ADR-0045 Consequences 与 ADR-0106 Context 第 3 条。

## 红线（裁决前）

- 不在消费门里判「新版本是否已到」或改 `ON CONFLICT` 语义——那是 ADR-0106 Alternatives 否决过的地方。
- 不给任何一格默认时点。

## 参照

ADR-0045、ADR-0106、迁移 0005 头注、`internal/parcelshipment/adapters/postgres/acceptance_judgments.go`、`internal/parcelshipment/application/{advance_acceptance_judgment.go, form_acceptance_decision.go}`、票 [09](./09-does-waiting-for-a-customer-supplement-actually-self-heal.md) 完成记录「发现」段。

## Comments

- 2026-09-04 · 通道 6：立票（draft）。起因是 ftr/09 闭环用例要靠替身把时点推后才能走到接受，回头量出判断账没有版本维。**只写票面，未动代码。**
