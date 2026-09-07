# 判断账没有提交版本维：受控补充后续办重判可能被旧判断压住，链再次停在`等待受控补充`

Category: bug
Status: resolved——通道 2 于 2026-09-07 受用户当轮授权（「继续」，即逐票代裁并实施）裁**甲**（见「裁决」节）并实施完成：分支 `mcp2-ftr10` 代码笔 `6d325ca7` + 机制清点笔 `c0ced2ec`，基线本地 main `3b411556`；**main 上的 SHA 待 MCP-1 重放后在 Comments 对照**（重放前本票的 resolved 以分支为封存出处）。完成记录见文末。此前 draft（通道 6 于 ftr/09 实施中量到，只写事实与两条形状）
Blocked by: 无

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

## 裁决（通道 2，2026-09-07，用户当轮授权代裁；取证锚本地 main `b2a78a2e`）

**取甲，读口按当前版本。不立新 ADR。**

1. **甲不是新决定，是 ADR-0045 Consequences 明写的后续项落地。** 那一条原句：「`RecordedJudgmentReader.LoadRecordedJudgments(ctx, requestID)` 的读口没有版本参数，多版本后旧版本的判断会被读进新版本的决定。这是本记录明知的后续项：读口要长出版本维度……随编排切片一起收，不在本记录定形状。」编排切片（ADR-0106 决定三、四）已于 2026-09-04 落地，本票就是它划出来的那一半；方向已裁，本票只定形状，故不需要新 ADR，形状记在这里。
2. **乙否决。** 它把一条 parcel-shipment 的正确性前提交给 party-commercial 的实例参数守：时点语义由接单规则包声明（UC-PC-002 步骤 6，`PAR-COM-14`），一份「以首次提交时刻为判断时点」的策略是合法声明，PS 无权在登记侧拒它——而它恰好会让两版同时点。用登记校验挡合法声明，是拿机制半边去改实例半边的可选范围。
3. **读口按当前版本，不按「当前及之前所有版本里最新」。** CONTEXT 硬句是「每份已提交委托必须针对**当前提交版本**形成或续办独立的接受判断任务」；旧版本的判断是对旧内容作出的，受控补充正因内容变了才形成新版本。若读跨版本最新，则新版本尚未重判时会读到旧版的`可达`并据以接受——与事实 5 是同一个错的反方向。历史留在行里（版本列钉住它属于哪一版），不参与当前版本的决定。
4. **范围。** 可达性判断表与财务控制表各加 `submission_version` 列并纳入主键（新迁移 `parcel_shipment/0018`，`ADD COLUMN … NOT NULL` 无默认，照 VE `0003`/`0014` 先例：测试库与 CI 走到这里表是空的，有行填不出版本就让迁移失败，不代填）；`AcceptanceJudgmentRecorder.RecordReachabilityJudgment` / `RecordFinancialControlResult` 与 `RecordedJudgmentReader.LoadRecordedJudgments` 各加一个 `SubmissionVersionID` 参数；四处调用点（推进可达性、推进财务控制、形成决定、撤回与拒绝的释放路径、复核详情读面）都手里有版本，直接传。**不动**：处理尝试表（续办引用的派生已含版本，且尝试不参与决定）、采用解析表（每轮 `formAdoptedBasis` 按版本重解、后写覆盖，读回的恒是本轮那次）。`ON CONFLICT DO NOTHING` 语义不动——同版本同成员同时点仍是重放。
5. **撤回 / 拒绝的释放路径按当前版本读**：`ControlReleaseRequest` 本就携带 `SubmissionVersion`，释放的是这一版形成的那次冻结。**顺带量到一格不在本票的缺口**：前一版本若已形成 `HELD`，新版本形成时它怎么处置（新版本重控是否再冻一次、旧冻结谁释放）——今天读口按时点取最新时也只释放最新一份，同一缺口本就在；归受控补充编排或 SA 的下一票，本票不碰。
6. **验证形状。** 票面事实 6 那条闭环用例（`cmd/parcel-dispatch/customer_supplement_resume_loop_test.go`）的替身把新版本时点推后一小时才走得到接受；本票先把它改成两版**同时点**做 red（新判断被吞、决定仍读旧`证据不足`、链再次停等补充），再落版本维转 green——那就是本票的进程级验收。适配器层另钉：同成员同时点两版各成一行；按版本读只回本版；旧版的`可达`不进新版。

**能力边界**：读过票面、ADR-0045 与 ADR-0106 全文、迁移 `0005` 头注、`acceptance_judgments.go` 全文、`advance_acceptance_judgment.go` 全文、`form_acceptance_decision.go` 读判断那一段、`judgment_continuation.go` 的 `formAdoptedBasis`、`withdraw_shipment_request.go` / `reject_shipment_request.go` 的 `releaseFreeze`、`query_acceptance_review_queue.go` 的 `serveReviewCase`、`customer_supplement_resume_loop_test.go` 全文；**没读** SA 侧冻结/释放的实现（第 5 条那格因此只记不裁）。

## 红线（裁决前）

- 不在消费门里判「新版本是否已到」或改 `ON CONFLICT` 语义——那是 ADR-0106 Alternatives 否决过的地方。
- 不给任何一格默认时点。

## 参照

ADR-0045、ADR-0106、迁移 0005 头注、`internal/parcelshipment/adapters/postgres/acceptance_judgments.go`、`internal/parcelshipment/application/{advance_acceptance_judgment.go, form_acceptance_decision.go}`、票 [09](./09-does-waiting-for-a-customer-supplement-actually-self-heal.md) 完成记录「发现」段。

## Comments

- 2026-09-04 · 通道 6：立票（draft）。起因是 ftr/09 闭环用例要靠替身把时点推后才能走到接受，回头量出判断账没有版本维。**只写票面，未动代码。**
- 2026-09-07 · 通道 2：裁甲并领票（见「裁决」节），实施于分支 `mcp2-ftr10`；完成记录随分支交付补在文末。

## 完成记录（通道 2，2026-09-07，分支 `mcp2-ftr10`，已验 tip `c0ced2ec`）

**落点（代码笔 `6d325ca7`）**

- 迁移 `migrations/parcel_shipment/0018_acceptance_judgment_submission_version.sql`：`acceptance_reachability_judgment` 与 `acceptance_financial_control` 各加 `submission_version text NOT NULL`（无默认，照 VE `0003`/`0014`）+ 非空白 CHECK，主键重建为（租户+委托+**版本**+[成员]+时点）。头注写明为什么版本进键、为什么不代填、为什么另两表不动。
- `ports.AcceptanceJudgmentRecorder.RecordReachabilityJudgment` / `RecordFinancialControlResult` 与 `ports.RecordedJudgmentReader.LoadRecordedJudgments` 各加 `version domain.SubmissionVersionID`；两处端口注释写下「为什么读本版不读跨版本最新」。
- `pspostgres.AcceptanceJudgments`：写口带版本列，读口 `WHERE … AND submission_version = $3`（可达性仍 `DISTINCT ON (parcel_id) … ORDER BY as_of_at DESC`，财务控制仍取最新，都在本版内）。
- 五处调用点传各自手里的版本：`AdvanceAcceptanceJudgmentHandler` / `AdvanceFinancialControlJudgmentHandler` 传 `command.SubmissionVersion`；`FormAcceptanceDecisionHandler` 读 `command.SubmissionVersion`；`WithdrawShipmentRequestHandler.releaseFreeze` / `RejectShipmentRequestHandler.releaseFreeze` 读 `command.SubmissionVersion`（与 `ControlReleaseRequest` 同版）；`serveReviewCase` 读 `record.SubmissionVersionID`。`cmd/parcel-api/unwired_orchestration.go` 的桩跟签名。
- **不动**：处理尝试表、采用解析表、`ON CONFLICT DO NOTHING` 语义、迁移 `0005` 头注（已施加不改写；它那句「没有 submission_version 列」自此是历史陈述）。

**验证形状（TDD，两条 red 先后见红再转绿）**

- 进程级 red：`cmd/parcel-dispatch/customer_supplement_resume_loop_test.go` 去掉替身「新版本时点推后一小时」（`synRPerVersionAsOfCommercialBasis` 整个删除，两版都答 `synRAsOfAnchor`），加断言「可达性判断行数 = 2」——落版本维前实跑 **FAIL：`可达性判断行数 = 1, want 2`**（票面事实 5 在真库上复现）；落后 PASS 到接受。文件头那段「时点随版本推移」的解释改写为现状。
- 适配器级 red→green：`TestJudgmentsBelongToTheSubmissionVersionTheyWereFormedFor`——两版同成员**同时点**各成一行；按新版读只回新版的`可达`与本版控制；按旧版读回旧版的`证据不足`与首版控制（历史留住）；从未判过的版本读回零值；同表两行。`TestAcceptanceJudgmentShapesArePinnedInTheDatabase` 加两支：空白版本的判断与控制被 CHECK 拒。
- 应用层 seam：`TestAcceptanceJudgmentResolvesBasisThenFormsAsOfThenAssesses` 钉「判断记在命令的版本上」，`TestAllAdoptedJudgmentsPassingFormsAnAcceptance` 钉「决定读命令的版本」；HTTP `TestReviewCaseAnswersDetailWithJudgments` 钉「复核详情按详情记录的版本读」。替身各加一个收版本的槛。

**验证强度（隔离 worktree，`c0ced2ec`）**：`gofmt -l` 空；`go build ./...` / `go vet ./...` 退 0；`go test -count=1 ./...` **未设 DSN 全绿**（PG 用例跳过；期间 `TestEmbeddedMigrationAssetsCarryNoCarriageReturnOrBOM` 曾红一次——新迁移文件带 CRLF，已改 LF 无 BOM）；**DSN 指向门禁容器 55432**：`./internal/parcelshipment/... ./cmd/parcel-dispatch/... ./cmd/parcel-api/... ./migrations/...` 全绿（postgres 包 44s、dispatch 27s，真连了库），`TestJudgmentsBelongToTheSubmissionVersionTheyWereFormedFor` 与 `TestACustomerSupplementWaitIsResumedByTheNewSubmissionVersionEnvelope` 为 **PASS 非 SKIP**。未带 DSN 跑全仓（推送方在 tip 上兑底）。机制清点笔 `c0ced2ec` 在 `6d325ca7` 干净检出上重生成（迁移 parcel_shipment 17→18，合计 141→142——只作此刻取证）。

**顺带量到、不在本票**：前一版本已形成的 `HELD` 在新版本形成时怎么处置（是否随新版本重控、旧冻结谁释放）——见「裁决」第 5 条；今天与改前行为一致（都只释放当前读到的那一份），未变差，也未解决。归受控补充编排 / SA 的下一票，要不要立归 PS owner。
