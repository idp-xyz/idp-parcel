# sa-cc/11 评审 Standards 两条非阻断一笔收口：`assemble.go` 哨兵头注「前三格」一句改与代码相符、证据层级 S / R / P 封闭集只在领域一处

Category: chore
Status: 已进 main——2026-09-14 21:0x 通道 1 推送方：`mcp1-tails4@5eb3caee` 基 `eca6dba1` = 当时 main tip，共享 main **直接快进**（SHA 不换：代码 `5eb3caee`，本簿记笔在其上）；`5eb3caee` 带 DSN 全仓 113 ok / 0 FAIL；清点零差；评审门单通道自审，见 Comments 末条。此前 resolved——**完工待进 main，2026-09-14 20:5x 通道 1**（推送方自立自做：点名无人应答、除通道 1 外全部 crash；分支 `mcp1-tails4` 基远端 main `eca6dba1`，树 `%TEMP%\idp-parcel-mcp1-tails4`；两条与 [lc/41](../../label-channel-service-first-release/issues/41-lc40-review-standards-tail-channel-basis-translation-stopped-comment-count.md) 同笔；评审门：单通道无非作者可派，推送方自审，票面如实写）。此前 in-progress——2026-09-14 20:4x 通道 1 自立（评审尾巴 A 类推送方自做，先例 09-08 awf/08「全部你自己干」）；要裁的为零
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
- **2026-09-14 21:0x · 进 main 记录 · 通道 1 推送方**：`mcp1-tails4@5eb3caee` 基 `eca6dba1`，期间 main 未动（`ls-remote` 推前核），共享 main `git merge --ff-only` 直接快进——**没有重放、SHA 不换**（09-08 awf/08 同一形态）；本簿记笔在其上，SHA 见广播。**验证（推送方全量一次）**：`gofmt -l` 空、`go build ./...` / `go vet` 0；20:5x 广播占 55432，`5eb3caee` 带 DSN 全仓 `go test -p 1 -count=1 ./...` **113 ok / 0 FAIL / 16 无测试 / 0 cached**（129 s）；释号广播；不增删文件、清点不重生成（判据 4 预报零差成立：`tools/mechanism-inventory` 只数文件与端口，本笔两者都没动）。**评审门**：自审（上条）。**后继**：无新立；11 评审 Spec ① ② 仍归 PP owner。`mcp1-tails4` → `merged/mcp1-tails4`、远端删；树 `%TEMP%\idp-parcel-mcp1-tails4` 拆。
- **2026-09-14 21:0x · 评审 ← 通道 2 · 钉 `5eb3caee` · 基线 `eca6dba1`**（补评审：进 main 时单通道自审，20:5x 五通道恢复后推送方派 task-293ce358，隔离检出 `%TEMP%\idp-review-tails4`；评审者 21:0x 交，派后约 15 分；全文照录，与 lc/41 共此一份）：
  **Standards · 阻断 0 / 非阻断 3**。阻断：无。非阻断：① `internal/parcelpricing/domain/value_objects.go` `EvidenceKind.Declared()` 头注「封闭集多一格时只有这里要改」过宽——`adapters/settlementaccounting/evaluation_request.go` `TranslateEvaluationRequest`、同包 `NewFormOnEvaluationRequestSubmittedAdapter`、`adapters/http/replay_evaluation.go` `EvaluationReplayPayload.Command` 三句错误文本及后者字段头注仍字面枚举「S / R / P」，集合增一格即无声变旧，正是该句自称消掉的那类（AGENTS.md「写代码注释」：讲错的比不讲糟）。零行为改法：句子收窄为「校验只有这里要改」；错误文本改取自领域属行为改动，票 24 红线「错误文本不变」明禁，另立票。② `cmd/parcel-dispatch/assemble.go` `evaluationRequestSubmittedUndecidedSentinels` 新句「今天每一封 BUY 请求信封都停在形成之前：」全称之后按卡数三分，漏同名单的 `ErrEvaluationRequestNotVisible`（到不了价卡解析那一格）。三格与 `form_on_evaluation_request_submitted.go` `Handle` 的 switch 逐格对上，正例与 `assemble_test.go` `TestASubmittedEvaluationRequestStopsHonestlyAtThePricingInputSeamThroughTheRouteTable` 断言的 `ErrPricingInputUnavailable` 一致（有卡→输入不可得、没卡→未配置、缺席→还看不见）；只是枚举不穷尽。可不改；改则加限定「到得了价卡解析的」。③ 票 24 Comments 判断项 ①「本仓领域值对象的 `valid()` 一律未导出」——PP domain 内成立（导出 `Valid()` 零），全仓不成立：`customscompliance/domain/funds_fact_payer.go` `FundsPayer.Valid`、`networkrouting/domain` 五处、`settlementaccounting/domain/pre_acceptance_control.go` `ControlAsOf.Valid` 共七处导出。取名 `Declared` 我接受，依据是语义半边（装配方声明的那一格在不在封闭集）而非「一律」；票面那半句由推送方落处置时收窄。
  无发现（查过）：(a) 领域外 `EvidenceKind(...)` 转换全仓仅 `replay_evaluation.go` 一处、既有；导出谓词让调用方不再需要知道成员，泄露减不增；`valid()` 本体零 diff。(b) 三处逐格等价：`EvidenceKind` 底型 string，零值 `""` 在旧 switch 与 `valid()` 同落 default→false；`accepted` 仍为末段、前序校验顺序不变；`replay_evaluation.go` 错误文本逐字同、仍带 `payload.Evidence`；两处 `!declaredEvidence(evidence)`→`!evidence.Declared()` 同义、文本同。(d) `ChannelSelectionBasisTranslatorDeps` 全仓唯一定义，`Accounts` / `Agreements` / `Resolutions` 与 `channel_selection_basis.go` 结构体及同包测试「授权源没装 / 协议源没装 / 接受时解析源没装」一致；字段是 `cmd/parcel-api/assemble_label_channel.go` 实际设的那层，接口名换了字段名多半不换，判可；application 头注点名 adapters 类型不成 import、只成 grep 锚。注释全中文、无行号、无跨文件计数；「证据层级」为 docs/README、ADR-0124 既用术语。冒烟基线：`Declared()` 转调 `valid()` 形似 Middle Man，被 PP domain 未导出 `valid()` 惯例压掉；Repeated Switches 由本笔消三留一。
  **Spec · 阻断 0 / 非阻断 1**。阻断：无。非阻断：① sa-cc/24 判据 1「头注含三枚哨兵的符号名」按整段头注满足（`ErrPricingInputUnavailable` 在条目头），但做法 1「末句改为点名三格」的末句本身只写两枚符号，第三枚以「这一格」指代。句在该条目内无歧义，判满足；记一笔供推送方决定是否把第三枚也写成符号。
  无发现（逐条）：24 判据 1 `git grep '前三格' -- cmd/parcel-dispatch/` 零命中 ✓；判据 2 `git grep -nw declaredEvidence -- internal/parcelpricing/` 零、三常量并列 switch 全 PP 仅 `domain/value_objects.go` `valid()` 一处 ✓；判据 3 / 41 判据 2 见末行全绿 ✓；判据 4 / 41 判据 3 两票 Comments 与代码同笔 `5eb3caee`、11 文件全 M 零增删 ✓；41 判据 1 `三个实例半边源` 于 PS application 零命中、`ChannelSelectionBasisTranslatorDeps` 在 `establish_selected_label_transaction.go` 恰一处（头注）✓。红线：24「条 1 只注释」✓（assemble.go +2 −1 纯注释）、「条 2 等价重构」✓（见 Standards (b)）、「不改 `EvidenceKind` 取值与 `valid()` 本体」✓、「domain 除一个导出方法外不动」✓（+6）；41「diff 只许注释行」✓（PS 文件 +3 −2 全注释）、「不动 PS domain / adapters/partycommercial / 任何测试」✓；地盘 `internal/settlementaccounting/**` / `docs/**` / `migrations/**` 零 diff ✓（`git diff --stat` 空）。范围蔓延：`replay_evaluation.go` 在 24 缺口 2「一并收」与地盘内，非蔓延；两 spec 各一行为票面所要；无票面未要之物。
  跑了 / 没跑：在 `%TEMP%\idp-review-tails4`（HEAD `5eb3caee`、树干净）不带 DSN：`gofmt -l ./cmd ./internal` 空；`go build ./...` 0；`go vet ./cmd/parcel-dispatch/ ./internal/parcelpricing/... ./internal/parcelshipment/application/` 0；`go test -count=1` PP domain / application / adapters/settlementaccounting / adapters/http + PS application + architecture 六包 ok。没跑：全仓、带 DSN、`cmd/parcel-dispatch` 真库用例（靠推送方 113 ok 那一跑）；未占 55432；未改任何文件、未拆检出。
- **2026-09-14 21:1x · 处置 · 通道 1 推送方**（代码已在 main，非阻断不回滚、随票记）：**Standards ① 前半 + ② + Spec ①** 三条都是零行为注释改法，合成一张 A 类尾巴票（本目录新号，与 lc/40 / sa-cc/23 / 24 同款）：`Declared()` 头注「封闭集多一格时只有这里要改」收窄为「校验只有这里要改」；`assemble.go` 末句加限定「到得了价卡解析的」、第三枚写成 `ErrPricingInputUnavailable` 符号名——派通道 2 自立自做。**Standards ① 后半**（三句错误文本字面枚举「S / R / P」、集合增一格即旧）：改成从领域取成员列表是行为改动（错误文本变），本票红线明禁，**归 PP owner** 判值不值得另立票，不进尾巴票。**Standards ③**：判断项 ① 那半句「本仓领域值对象的 `valid()` 一律未导出」**收窄**为「PP domain 内 `valid()` 未导出（导出 `Valid()` 零）；全仓另有导出 `Valid` 七处（评审于 `5eb3caee` 量：CC `FundsPayer.Valid`、NR domain 五处、SA `ControlAsOf.Valid`）」，取名 `Declared` 的依据只剩语义半边——原判断项文字不改写（历史），以本条为准。**评审门补齐**：非作者评审缺席那一笔就此兑清；进 main 记录「评审门：自审」一句自本条起读作「自审 + 21:0x 补评审 ← 通道 2 两轴 0 阻断」。
