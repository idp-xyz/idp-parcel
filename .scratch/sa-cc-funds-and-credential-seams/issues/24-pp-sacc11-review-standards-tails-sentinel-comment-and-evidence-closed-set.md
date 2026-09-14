# sa-cc/11 评审 Standards 两条非阻断一笔收口：`assemble.go` 哨兵头注「前三格」一句改与代码相符、证据层级 S / R / P 封闭集只在领域一处

Category: chore
Status: resolved——**完工待进 main，2026-09-14 20:5x 通道 1**（推送方自立自做：点名无人应答、除通道 1 外全部 crash；分支 `mcp1-tails4` 基远端 main `eca6dba1`，树 `%TEMP%\idp-parcel-mcp1-tails4`；两条与 [lc/41](../../label-channel-service-first-release/issues/41-lc40-review-standards-tail-channel-basis-translation-stopped-comment-count.md) 同笔；评审门：单通道无非作者可派，推送方自审，票面如实写）。此前 in-progress——2026-09-14 20:4x 通道 1 自立（评审尾巴 A 类推送方自做，先例 09-08 awf/08「全部你自己干」）；要裁的为零
Blocked by: 无（[11](11-pp-inbox-consumer-receives-evaluation-request-envelope.md) 已进 main `6876e10f` / `eca6dba1`，两条出处全在其 Comments「评审 ← 通道 1」Standards ① ②）。撞点：在途分支零、其余通道全部 crash，无人共写

## 缺口（出处逐条指到评审原话；取证于 `eca6dba1`）

1. **头注一句与代码相反**（11 评审 Standards ①；AGENTS.md「写代码注释」——注释只写代码讲不出的东西，讲错的比不讲更糟）。`cmd/parcel-dispatch/assemble.go` `evaluationRequestSubmittedUndecidedSentinels` 头注 `ErrPricingInputUnavailable` 那条末句「今天每一封 BUY 请求信封都停在前三格之一」：名单顺序里前三格是 `ErrEvaluationRequestNotVisible` / `ErrPriceCardNotConfigured` / `ErrPriceCardApplicabilityConflict`，而范围下恰有一张卡的请求停在第四格 `ErrPricingInputUnavailable`——`assemble_test.go` `TestASubmittedEvaluationRequestStopsHonestlyAtThePricingInputSeamThroughTheRouteTable` 的正例断言的正是这一格。
2. **S / R / P 封闭集写了四份**（11 评审 Standards ②，Fowler Repeated Switches，判断题）。领域 `EvidenceKind.valid` 未导出，于是 `internal/parcelpricing/adapters/settlementaccounting/evaluation_request.go` `declaredEvidence` 与 `internal/parcelpricing/application/form_evaluation_from_request.go` `FormEvaluationFromRequestCommand.accepted` 各自再写一份同样的 switch；开工重量时 `git grep` 又带出一处既有的同病：`internal/parcelpricing/adapters/http/replay_evaluation.go` 的回放载荷译码（不是 11 引入，同一封闭集、同一取舍，一并收）。封闭集多一格时四处要一起改，改漏一处编译器不报。

**不在本票**：11 评审 Spec ①（`WithInput` 顺带保住序列说明使一种组合的解释 / 语义摘要变化、范围内无用例钉）与 Spec ②（形成之前的停格在 PP CONTEXT 里没有语言）——都归 PP owner，不是零行为尾巴；`internal/parcelpricing/domain/**` 除新增一个导出方法外不动；任何生产语句的行为。

## 做法

1. 头注末句改为点名三格：范围下没卡停 `ErrPriceCardNotConfigured`、多卡停 `ErrPriceCardApplicabilityConflict`、有恰一张卡的停在本格，并指明真库装配例证的正例就是本格。只注释。
2. 领域 `EvidenceKind` 加导出方法 `Declared()`（转调既有 `valid()`，头注写明为何导出）；适配器删 `declaredEvidence`、两处调用改 `evidence.Declared()`；入口 `accepted` 的 switch 改 `command.Evidence.Declared()`；HTTP 回放门译码的 switch 改 `if !evidence.Declared()`，错误文本不变。既有用例（适配器「证据层级不给默认」、入口 `TestFormEvaluationFromRequestRejectsAnUnformedCommand`、HTTP 回放门的畸形载荷例）不改、照旧绿。

## 红线

- 零行为：条 1 只注释；条 2 是等价重构，`go test` 范围内既有用例零 diff 且全绿。
- 不动 `internal/settlementaccounting/**`、`docs/**`、`migrations/**`；不改 `EvidenceKind` 的取值与 `valid()` 本体。
- 注释中文；不写行号、不数别处的东西。

## 完成判据

1. `git grep -n '前三格' -- cmd/parcel-dispatch/` 零命中；`evaluationRequestSubmittedUndecidedSentinels` 头注含三枚哨兵的符号名。
2. `git grep -nw declaredEvidence -- internal/parcelpricing/` 零命中（`-w`：两条既有用例名 `…UndeclaredEvidenceKind` 是别的词）；`git grep -n 'EvidenceSynthetic, .*EvidenceReplay, .*EvidenceProduction' -- internal/parcelpricing/` 只剩 `domain/value_objects.go` `valid()` 一处（`_test.go` 不计）。
3. `gofmt -l` 空、`go build ./...` / `go vet` 退 0；`go test -count=1 ./internal/parcelpricing/domain/ ./internal/parcelpricing/application/ ./internal/parcelpricing/adapters/settlementaccounting/ ./internal/parcelpricing/adapters/http/ ./internal/architecture/...` ok；推送方在 tip 上带 DSN 全仓一次。
4. 完成记录同笔；清点预报零差（不增删文件）。

## 地盘

`cmd/parcel-dispatch/assemble.go`（一处头注）、`internal/parcelpricing/domain/value_objects.go`（一个导出方法）、`internal/parcelpricing/application/form_evaluation_from_request.go`（`accepted` 末段）、`internal/parcelpricing/adapters/settlementaccounting/evaluation_request.go` + `form_on_evaluation_request_submitted.go`（各一处调用）、`internal/parcelpricing/adapters/http/replay_evaluation.go`（译码一处）、本票面、sa-cc spec 一行。

## 参照

[11](11-pp-inbox-consumer-receives-evaluation-request-envelope.md) Comments「评审 ← 通道 1」Standards ① ②、「进 main 记录」；[lc/40](../../label-channel-service-first-release/issues/40-lc35-review-standards-tails-test-header-counts-and-transaction-shell-sentence.md)（同款 A 类尾巴合票先例）；AGENTS.md「写代码注释」「改文档」。

## Comments

- **2026-09-14 20:4x–20:5x · 通道 1（推送方自做）· 完工**。分支 `mcp1-tails4` 基远端 main `eca6dba1`，与 lc/41 同笔（本笔）。
  - **条 1**：`assemble.go` 头注末句改为「今天每一封 BUY 请求信封都停在形成之前：范围下没卡停 ErrPriceCardNotConfigured、多卡停 ErrPriceCardApplicabilityConflict、有恰一张卡的停在这一格——真库装配例证的正例就是这一格」；切片字面零 diff。
  - **条 2**：`domain/value_objects.go` 加 `func (kind EvidenceKind) Declared() bool { return kind.valid() }` 带头注；`adapters/settlementaccounting` 删 `declaredEvidence`（−11），`TranslateEvaluationRequest` 与 `NewFormOnEvaluationRequestSubmittedAdapter` 改 `evidence.Declared()`；`application` `accepted` 末段 switch 改 `return command.Evidence.Declared()`（−7 +1）；`adapters/http/replay_evaluation.go` 译码 switch 改 `if !evidence.Declared()`（−4 +2，错误文本原样）。
  - **验（本机，钉本笔）**：`gofmt -l ./cmd ./internal` 空；`go build ./...` 0；`go vet` cmd/parcel-dispatch + PP 全部 + PS application 0；不带 DSN `go test -count=1` PP domain / application / adapters/settlementaccounting / adapters/http + PS application + architecture 六包 ok。带 DSN 全仓由推送方在 tip 上跑一次（同一会话，见「进 main 记录」）。
  - **判据逐项**：1 ✓（`git grep '前三格' -- cmd/parcel-dispatch/` 零；头注含三枚符号名）；2 ✓（`git grep -nw declaredEvidence` 零命中；三常量并列 switch 全 PP 只剩 `valid()`）；3 ✓ 见上；4 ✓ 本条。
  - **红线**：diff 七文件生产语句只有等价重构；`internal/settlementaccounting/**` / `docs/**` / `migrations/**` 零 diff；注释中文、无行号、不数别处。
  - **判断项（归评审 / PP owner）**：① 导出名取 `Declared` 不取 `Valid`——本仓领域值对象的 `valid()` 一律未导出，导出一个 `Valid` 会让读者以为封闭集校验从此归调用方；`Declared` 说的是「装配方声明的这一格在不在封闭集里」，与入口 / 适配器头注「证据层级由装配方声明、不给默认」同一用词。② `valid()` 保留、`Declared()` 转调它——领域内部构造门（`NewEvaluationRequest` 等）继续用未导出的那一个，边界只多一个名字不多一份逻辑。
  - **评审门**：本会话是全仓唯一存活通道（点名 20:33 截止 20:38 无人应答，用户 20:4x 报「现在都 crash」），没有非作者可派、隔离子代理今天两次鉴权错；按 parallel-sessions「纯 .md 簿记、清点重生成……推送方自审」一条的精神把零行为注释 + 等价重构归入自审，票面如实写「自审」，不装样子。
