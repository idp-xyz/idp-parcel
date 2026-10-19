# 34 复核完成口的提交版本由服务端当前版顶替：新提交版本形成后，同一复核请求重投或按旧页面提交，会把旧版本的复核签到新版本的任务上

Category: bug
Status: in-progress——2026-10-18 完工；评审 0 阻断（会话读 diff 审，见 Comments），待进 main。此前 in-progress——2026-10-18 作者子代理完工，待评审。此前 ready-for-agent——2026-10-18 暂缓：用户因 token 花费选「精简续做」，本票没做完；分支 `claude-oc34`（基 `b58b10ef`，工作树 `/home/tops/workspace/idp-parcel-claude-oc34`）上留有2 笔（复核完成版本由调用方指名的后端修法；前端复核草案构造的 WIP），未经评审，接手前先 rebase 到最新 main 并核一遍。此前 in-progress——2026-10-18 会话 `idp-parcel-9b` 立并认领（票 15 之三、之四补核发现；用户授权代决）；分支 `claude-oc34`
Blocked by: 无
父票：[15](./15-operation-decision-faces-take-operator-intake.md)（之三 `44ef6a4a`、之四 `dc439417`）
出处：票 15 Comments「评审 ← 会话 idp-parcel-9b 起的隔离子代理 · 钉 `0e3bb7df`」那一条里的「子代理顺带提到之三那三口也查不到逐口核对记录」；票 26 遗留研判中补核票 15 之三、之四（簇 C6）发现；补记随 operator-channel/39。
地盘：
- internal/parcelshipment/adapters/http/operator_decision_intake.go
- internal/parcelshipment/adapters/http/operator_decision_intake_test.go
- apps/admin-web/src/pages/shipment-request/api.ts
- apps/admin-web/src/pages/shipment-request/AcceptanceReviewPage.tsx
- apps/admin-web/src/pages/shipment-request/（为可测抽出的复核草案构造及其 node:test，新文件）
- cmd/parcel-api/assemble_review_test.go
- 票 15 补记下的回写行（只加行）

代决依据：用户 2026-10-16 授权会话 `idp-parcel-9b`「你作为业务和系统专家你来自决」；裁定经隔离子代理研判、对抗核验与合成（研判轮 review26-followups-decide）。

本票 apps/admin-web 那部分照 docs/agents/workflow.md「前端切片」的门（tsc、node scripts/run-tests.mjs、vite build）自验，但随本票分支走 Go 那条路合入。

## 现象

- `/shipment-requests/manual-review-completions` 自之四起挂在操作者真渠道上。`OperatorDecisionIntake.IntakeManualReviewCompletion` 用的提交版本取自 `target.CurrentVersion`，也就是服务端此刻的当前版本，不从请求里收；管理台的 `ManualReviewCompletionDraft` 也只送委托标识和理由。结果，`CompleteManualReviewHandler.Handle` 里「版本已换代」那道门，比的是「此刻的当前版」对「此刻的当前版」。在真渠道上，只有寻址和编排之间的竞态窗口还能走到这道门。
- 后果：委托在 V1 上等人工复核，复核已经记下。之后客户受控补充，形成 V2；旧任务转`已停止`，新任务上没有复核完成。这时如果同一个复核 POST 因超时重投，或者操作员还开着 V1 那一页就提交了，Intake 取到的是 V2，编排就在 V2 的任务上记下一份完成，复核人和证据都是 V1 那一份，还会再铸一封「复核已完成」信封。V2 的判断经 `ShipmentRequest.manualReviewState` 读成「已完成」，`Decide` 不再停在等待人工复核；规则要求复核时，就直接接受了。

## 复现（S；探针不入库，存放在会话 scratchpad，下称 `$DECIDE` = `/tmp/claude-1000/-home-tops-workspace-idp-parcel/39d8f2f8-cde5-4dd1-bb34-aef8ce7c660c/scratchpad/decide`）

- 真库：`$DECIDE/C6/c6_api_probe_test.go` · `TestC6ProbeManualReviewThroughTheOperatorIntakeAgainstARealDatabase`，命令为 `IDP_PARCEL_POSTGRES_DSN=… go test -count=1 -p 1 -overlay $DECIDE/C6/overlay_api.json ./cmd/parcel-api/ -run TestC6Probe -v`。探针经生产 Intake、真寻址和生产编排，只有复核授权请求映射是合成替身（做法同 `manualReviewOrchestrationWith`）。结果依次为：首次 201 `RECORDED`；原样重放 200 `ALREADY_COMPLETED`（回显首份）；换人换理由 200 `ALREADY_COMPLETED`（回显首份）；受控补充形成 V2；再原样重放首次那个请求，答 201 `RECORDED`，V2 任务上的复核人和证据是 V1 那一份，「复核已完成」信封变成 2 封。对照：编排直调、命令指名 V1，答 `VERSION_SUPERSEDED`。复跑日志：`$DECIDE/C6-ticket15-part3-algebra/rerun_c6_api.log`。
- 领域：`$DECIDE/C6-adv-impl/zz_c6adv_domain_probe_test.go` · `TestC6AdvStaleReviewSignedOntoV2IsAdoptedByV2Decide`。对照组（V2 上没有完成、规则要求复核）停在 `SUBMITTED` / 等待人工复核；把 V1 那份复核签到 V2 之后，`Decide` 形成接受。复跑日志：`$DECIDE/C6-ticket15-part3-algebra/rerun_domain.log`。

## 应当

- ADR-0132 决定二：「处置命令撞上换代答`版本已换代`，撞上已成立的决定答`任务已完结`并交回那一个——与 `CompleteManualReview` 的分格同形」。`版本已换代`是复核完成代数里的一格，在真渠道上必须走得到。
- ADR-0045 决定三：「新版本必须携带自己的判断任务。旧任务转`已停止`」。它否决「复用原任务、改钉新版本号」的理由是「改钉等于把两个版本的历史缝进一条时间线」；把 V1 的复核签到 V2 的任务上，正是同一件事。
- PS CONTEXT「已提交 → 已接受：当前提交版本的……全部通过，且规则要求的人工复核已经完成」；UC-PS-001 `AT-PS-037`「原结果不再用于接受并重新判断」；ADR-0086 决定四「若再次停在`等待人工复核`（构造上只剩一种可能：新提交版本换代重开了复核），按 Decision 一再暂停一次」。
- 编排自己的契约：`CompleteManualReviewCommand` 头注「版本必填是本层的门——复核完成挂在版本的判断任务上，不指名版本的完成无从判断它签给了谁」；`ManualReviewCompletionOutcome` 头注「把完成记到新任务上就是把 A 版的判断签到 B 版头上」。
- 所以：V2 形成之后，指名 V1 的复核应当答 `VERSION_SUPERSEDED` 并报出当前版本；V2 上不落完成，也不铸信封。

## 根因

之三把「复核完成与主动拒绝的提交版本取服务端当前版本（管理台草案不带）」列为判断项，留给评审和 PS owner，但一直没人裁，之四就接上了真渠道。版本是复核对象的身份，不是服务端可以代填的默认值。同仓的授权处置口（`IntakeAuthorizedDisposition` 取载荷里的版本）和管理台 `AuthorizedDispositionDraft`（头注「不指名版本就分不清签给了谁」）都由调用方指名版本。

## 裁定（机制；代 PS owner）

取「由调用方指名版本」：复核完成的线格式加必填的 `submissionVersionId`，Intake 取载荷里的版本，写法照 `IntakeAuthorizedDisposition`。版本是复核的对象，不属于 ADR-0151 决定二列出的身份格（不采信自报的是租户、提交操作者和委托来源身份），授权处置口已有先例。

不取的方案：
- 「按旧任务上同内容的完成认重放」：堵不住「按旧页面提交」这条来路，而且等于在编排里另写一套重放判据。
- 「领域要求任务停在等待人工复核」：V2 停到等待复核之后，旧请求照样签得上；还违背 `ShipmentRequest.CompleteManualReview` 的注释「它不判断复核该不该做」。

主动拒绝的版本来源不在本票范围：拒绝成立后不会再有新版本，不影响重放代数；要不要绑定操作员看过的那一版，交 PS owner。

## 做什么

1. `IntakeManualReviewCompletion`：载荷改用复核完成自己的类型，加 `submissionVersionId`。纯载荷校验（版本、理由）排在 `locate` 之前：缺版本或格式坏答 400，委托查不到时也先答 400，写法照 `IntakeAuthorizedDisposition`。命令取载荷里的版本。
2. 同一笔改正随之变成假话的注释：`OperatorDecisionIntake` 头注「复核完成与主动拒绝只有委托标识与理由」；`IntakeManualReviewCompletion` 头注「提交版本取服务端当前版本——管理台这一口的草案不带版本」；`IntakeActiveRejection` 头注「提交版本取服务端当前版本，理由同复核完成」，改成独立给出自己的理由，不再指向复核完成。
3. 管理台（`apps/admin-web/src/pages/shipment-request/`）：
   - `ManualReviewCompletionDraft` 加 `submissionVersionId`；`AcceptanceReviewPage` 的复核完成从队列行带上它（`AcceptanceReviewQueueEntry` 已经带有 `submissionVersionId`）。
   - 同一笔改正草案注释「**故意只有这两个字段。**」，以及草案头注「动作端点当前一律挂「未配置即拒」Intake,不读请求体」（自之四起已不成立）。
   - `ManualReviewCompletionOutcome` 联合类型照 Go 侧 `ManualReviewCompletionOutcome.String()` 的取值改：现在有服务端从不返回的 `'TASK_CONCLUDED'`，又缺 `TASK_ALREADY_CLOSED`、`REVISION_CONFLICT`、`NOT_AUTHORIZED`、`AUTHORITY_RULES_NOT_CONFIGURED`。票 wiring-baseline-remainder/04 当时记下了这个不一致，一直没改，这次一并收掉。
4. 回归用例，见完成判据。

## 不做

- 主动拒绝的版本来源；拒绝口的答复格（归 operator-channel/36）。
- 之三判断项里证据格的取法，以及自由文本理由送进结构化原因格（交 PS owner）。
- 复核授权请求映射（psb/07）。

## 是否潜伏

潜伏，有两把锁。一是生产装配上复核授权请求映射为 nil（`buildManualReviewOrchestration` 答 500 `NO_ANSWER_FORMED`），形成不了完成；二是 `/shipment-requests/supplements` 挂的是 `UnconfiguredIntake{}`，生产上形成不了 V2。两把锁都打开才会发作。这里取保守的排序：在两件事里先发生的那件之前修好。

## 完成判据

- [ ] 改写 `TestManualReviewCompletionIsAddressedServerSideAndSignedByTheAuthenticatedOperator`。它现在断言命令版本等于替身目标的当前版 `SYN-VERSION-02`，钉住的恰恰是缺陷。改为：载荷送 `SYN-VERSION-01`，替身目标当前版为 `SYN-VERSION-02`，断言命令取的是载荷那一份；缺版本、版本格式坏各答 400，委托查不到时也一样。
- [ ] 真库装配回归（`cmd/parcel-api` 的复核装配测试），必须经过操作者 Intake（`OperatorDecisionIntake`，真寻址）：V1 记下复核 → 受控补充形成 V2 → 原样重投指名 V1 的请求，答 200 `VERSION_SUPERSEDED`，`currentVersion` 为 V2；「复核已完成」信封仍是 1 封；V2 任务上没有完成。编排直调、指名 V1 的用例在今天的代码上就答 `VERSION_SUPERSEDED`，没有辨别力，不算数。
- [ ] 管理台 node:test：证明复核完成草案带上了队列行的 `submissionVersionId`（必要时把草案构造抽成可测的纯函数）；管理台三道门退出码为 0。
- [ ] 在票 15 补记的复核完成那一行下追加回写（✅ 只核对，附本票进 main 的 SHA），只加行。
- [ ] 走 Go 那条路（并行会话）：开分支；作者自验（gofmt / vet / build，改动包带 DSN 跑 `go test -p 1 -count=1`）；非作者评审；推送方带 DSN 跑全量。Go 和管理台在同一个分支、同一次进 main，因为版本格改成必填后，旧管理台会被答 400。不走「前端切片」：那一节的适用条件是「这张票的 diff 全部落在 `apps/admin-web/**` 与它自己的票面里」。

## 完成记录（2026-10-18，会话 idp-parcel-9b 的实施子代理，分支 claude-oc34）

**落点**（原基线 `b58b10ef`，已 rebase 到 `origin/main` `200281b2`，无冲突；未推送）：

| 笔 | 做了什么 |
|---|---|
| `87bcc45c` 后端 | `IntakeManualReviewCompletion` 改收 `manualReviewCompletionPayload`，必填 `submissionVersionId`；版本与理由两道纯载荷校验排在 `locate` 之前；命令取载荷里的版本。`OperatorDecisionIntake`、`IntakeManualReviewCompletion`、`IntakeActiveRejection` 三处头注改正，拒绝口给出自己的理由。Intake 单测与真库装配回归随这一笔。 |
| `f7754809` 前端 WIP | 原样保留：`manualReviewCompletionDraftOf` 半成品与它的 node:test（此时为红）。 |
| `43b2e118` 前端 | `ManualReviewCompletionDraft` 加 `submissionVersionId`；`manualReviewCompletionDraftOf` 取队列行的版本；`AcceptanceReviewPage` 的复核完成从队列行经它发出。`ManualReviewCompletionOutcome` 照 Go 侧 `String()` 改。草案节头注和复核草案的「故意只有这两个字段」两处注释改正。 |

**完成判据逐格**：

- [x] Intake 单测：`TestManualReviewCompletionIsAddressedServerSideAndSignedByTheAuthenticatedOperator` 已改写，载荷送 `SYN-VERSION-01`、替身当前版 `SYN-VERSION-02`，断言命令取载荷那一份。`TestAManualReviewCompletionWithoutItsVersionAnswers400EvenWhenTheRequestIsUnknown` 覆盖缺版本、版本空白、版本非字符串、缺理由、理由空白，委托查得到和查不到两种情形下都答 `ErrMalformedRequest`。
- [x] 真库装配回归：`cmd/parcel-api` · `TestAReviewNamingASupersededVersionIsNotSignedOntoTheNewVersionThroughTheOperatorIntake`。它经生产的 `OperatorDecisionIntake` 和真寻址（`pspostgres.NewShipmentRequestViews`），走完整个 HTTP 端点。用例依次验证：V1 复核答 201 `RECORDED`；受控补充形成 V2；原样重投答 200 `VERSION_SUPERSEDED`，`currentVersion` 为 V2；信封仍是 1 封；V2 任务上没有完成。
- [x] 管理台 node:test：`manual-review-completion.test.ts` 两例，一例钉住草案带上队列行的 `submissionVersionId` 且整形只有三格，另一例钉住旧页面那一行指名它自己的版本。三道门退 0（见下）。
- [ ] 票 15 补记下回写：**未做**。票 15 里还没有「复核完成那一行」，那一行归 [39](./39-ticket-15-records-the-algebra-of-every-operator-decision-face.md)，而 39 要等本票进 main 才能动；进 main 的 SHA 现在也还没有。这一行留给 39，或留给本票进 main 的那一笔。
- [ ] Go 那条路：作者自验已做（见下）。**非作者评审**与**推送方带 DSN 跑全量**未做。

**先红后绿**：

- 后端：rebase 之后，只把 Intake 的命令版本退回 `target.CurrentVersion`（载荷照收），再跑新用例。真库回归在重投那一步答 201 `RECORDED`，复核人是 V1 那一份，`currentVersion` 是 V2，失败于「want 200 VERSION_SUPERSEDED」。Intake 单测失败于「submission version = "SYN-VERSION-02"」。整份退回 `origin/main` 的 Intake 时，三例都红，答 400 `unknown field "submissionVersionId"`。复原修法之后全部转绿。
- 前端：接手时 WIP 那两例在草案不带版本时 ✖；`43b2e118` 之后 ✔。

**门**：

- Go：`gofmt -l` 无输出；`go build ./...`、`go vet ./...` 退 0。带 DSN 跑 `go test -p 1 -count=1 ./internal/parcelshipment/... ./cmd/parcel-api ./internal/architecture/...` 退 0，21 个包 ok。
- 前端（`apps/admin-web`）：`tsc -b --noEmit` 退 0；`node scripts/run-tests.mjs` 退 0，536 例全过；`pnpm exec vite build` 退 0。
- 机制清点重生成后无差。

**证据层级**：`S`（合成 `SYN-` 数据，复核授权请求映射为合成替身）。

**评审**：待会话读 diff 审。

## Comments

- **评审 ← 会话 idp-parcel-9b 读 diff 审（不算非作者评审）· 钉 `bee6d794`（基线 `200281b2`）· 2026-10-18**：用户选「精简续做」，评审由会话本身读 diff 做；代码出自本会话起的实施子代理，不算非作者评审，如实记下。
  - **Spec**：阻断无。`IntakeManualReviewCompletion` 改收必填的 `submissionVersionId`，版本与理由校验排在委托寻址之前（坏载荷 400）；主动拒绝仍取当前版并在头注写明理由（拒绝成立后不再形成新版本，是否绑定所见版本归 PS owner）。管理台 `manualReviewCompletionDraftOf` 从队列行取版本，`ManualReviewCompletionOutcome` 照 Go 侧取值。真库回归在旧做法下重投答 201、修后答 `VERSION_SUPERSEDED`。
  - **Standards**：阻断无。非阻断 1：前端头注用「他」指操作员，已在 `bee6d794` 改中性说法。
  - **未做的格**：票 15 补记行归 operator-channel/39（等 34、36 进 main 后一并写）。
