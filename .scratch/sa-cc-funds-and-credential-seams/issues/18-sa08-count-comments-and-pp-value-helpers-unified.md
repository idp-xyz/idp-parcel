# sa-cc/17 非作者评审 Standards 非阻断两条收口：SA 08 那批「三件引用 / 三件来源引用」跨文件计数换点名、PP「字符串构造器喂进去、失败即 Fatal」同形助手合到 `pptest.Value`

Category: chore
Status: resolved——完工待非作者评审进 main，2026-09-14 10:2x（分支 `mcp4-tails2` 基远端 main `6bbf2bf0`；条 1 `e7809d23`、条 2 本笔，完成记录随条 2 同提交，见 Comments「完工」；`saTestValue` 判留；清点预报零差）；此前 in-progress——2026-09-14 10:1x 通道 4 按通道 1 派单 task-c0000fd2 自立自做（SA 一条 + PP 一条合一票，先例 sa-cc/17），分支 `mcp4-tails2` 基远端 main `6bbf2bf0`（树 `D:/tops/idp-parcel-mcp4-tails2`），与 [ve-disc/06](../../ve-disclosure-policy-view/issues/06-eta-test-name-count-and-catalog-registration-nil-text.md) 同分支；要裁的为零
Blocked by: 无（[17](17-sa-nil-dependency-text-sa01-count-comments-and-pp-synthetic-evaluation-fixture.md) 已进 main `6bbf2bf0`，两条出处在其 Comments「评审 ← 通道 1」）。撞点：推送方同期改 sa-cc/11 / 12 / 13 票面与本 spec.md 的状态行，与本票新增的 spec 行不同行，重放时推送方解

## 缺口（取证于 `6bbf2bf0`，开工先重量；中文模式一律走 UTF-8 `-f` 文件，不在命令行直写）

**一、sa-cc/08 那批注释里的跨文件计数**（17 评审 ← 通道 1 Standards ②；17 条 2 只收 01 新增的、把 08 那批记成判断项交评审，评审判归 owner 另笔）。

- 「三件引用 / 三件来源引用」数的是 `EvaluationRequest` 的来源引用（发生项 / 费用项目 / 供应商协议）或 `FormSupplierExpectedCostCommand` 的字段，写在别的文件里就是数别处的东西（AGENTS.md「计数与行号同构」）；每处都能直接点名，去数词零损失。
- **重量**：模式文件 `三件引用|五种结果|三件来源`，`git grep -n -f` 于 `6bbf2bf0` 在 `internal/settlementaccounting/` 实得：`adapters/postgres/evaluation_request_handoff.go`（`evaluationRequestPayload` 头注「三件来源引用」——派单六处之外，同族）、`adapters/postgres/evaluation_request_handoff_test.go`（文件头「三件来源引用」+ 一句 Fatalf「三件引用」）、`adapters/postgres/evaluation_request_test.go`（`Covers:` 一句 + 一句 Fatalf）、`application/request_buy_evaluation.go`（`RequestBuyEvaluationCommand` 头注）、`application/request_buy_evaluation_test.go`（一句 Fatalf）、`ports/evaluation_request.go`（`EvaluationRequestView` 头注 + `EvaluationRequestIntent` 头注）、`domain/evaluation_request.go`（一句）、`domain/evaluation_request_test.go`（文件头 + 一句 Fatalf——派单六处之外）。「五种结果」在 SA 已零命中（17 条 2 收完）。
- **不动**：`domain/evaluation_request.go` 那处数的是同文件自己的字段（派单裁法，同 ve-disc/04 评审判断项 2）。**判断**：`domain/evaluation_request_test.go` 两处数的是同包另一文件（`evaluation_request.go`）的字段，按「别处」的字面算别处，一并收；`request_buy_evaluation.go` 头注数的是同文件 `RequestBuyEvaluationCommand` 自己的三个字段（`Occurrence` / `FeeItem` / `Agreement`），按同一裁法本可不动，但派单点名了它、且点名三件比数词更清楚，收；`evaluation_request_handoff.go` 生产头注是派单六处之外扫出的同族，同笔收。票面 / 裁决文不动。

**二、三份同形的「字符串构造器喂进去、失败即 Fatal」助手**（17 评审 Standards ③ + 17 作者判断项 ④）。

- `internal/parcelpricing/pptest/evaluation.go` `Value[T]`（17 条 3 导出）、PP `adapters/postgres/evaluation_test.go` `evaluationValue[T]`（被同包十一份 `_test.go` 用，`git grep -c 'evaluationValue('` 于 `6bbf2bf0` 逐文件从 2 到 27 不等）、`cmd/parcel-dispatch/assemble_test.go` `saTestValue[T]`（十七处调用，喂的是 `sadomain.*` 与 `ccdomain.*` 的构造器，一处 PP 类型都没有）。
- **判断**：`evaluationValue` 改为直调 `pptest.Value`（同包、同上下文，一行体）——调用点一处不动，`t.Helper()` 链保留；`saTestValue` **留着**：它喂的全是 SA / CC 类型，`pptest` 是 PP 的测试夹具包，让组合根测试为了 SA / CC 值去 import PP 的夹具，会把 `pptest` 当成跨上下文通用工具用，包名说的不是这件事；合一的收益（一处体）已由 PP 那对拿到，`saTestValue` 与 `pptest.Value` 是两个上下文各自的一行助手，不是第三份 PP 夹具。派单明说「只合 PP 那两份、`saTestValue` 留着并写理由，也算答案」。

## 做法

1. 计数换点名：「三件引用」/「三件来源引用」→「发生项 / 费用项目 / 供应商协议引用」或「来源引用」（上下文里已点过名的地方只去数词）；只改注释与测试文本。一笔。
2. `evaluationValue` 函数体换成 `return pptest.Value(t, construct, raw)`（保留 `t.Helper()`），import `pptest`；`saTestValue` 不动。一笔，完成记录随之同提交。

## 红线

- 零行为：条 1 只注释与 `_test.go` 文本；条 2 只 `_test.go`。
- 不动 `domain/evaluation_request.go` 那一句、任何 `.scratch/` 待裁票面、生产 `.go` 语句。
- 不写行号、不数别处的东西。

## 完成判据

1. `git grep -n -f <UTF-8 模式文件：三件引用|三件来源>` 于 `internal/settlementaccounting/` 只剩 `domain/evaluation_request.go` 一处；`internal/settlementaccounting/` diff 只有注释行与 `_test.go` 行。
2. `evaluationValue` 体内只剩一行转调 `pptest.Value`，`git grep -c 'evaluationValue('` 各文件数不变；`saTestValue` 零 diff；`pptest` 仍只被 `_test.go` import。
3. `gofmt -l` 空、`go build ./...` / `go vet ./...` 退 0；`go test -count=1 ./internal/settlementaccounting/... ./internal/parcelpricing/... ./cmd/parcel-dispatch/... ./internal/architecture/...` ok（不带 DSN——注释 / 测试助手，推送方进 main 前跑全量）。
4. 完成记录随条 2 那一笔同提交（Status → resolved、逐笔 SHA、判据逐项、判断项）；清点预报零差（不增删文件）。

## 地盘

`internal/settlementaccounting/adapters/postgres/evaluation_request_handoff{,_test}.go`、`adapters/postgres/evaluation_request_test.go`、`application/request_buy_evaluation{,_test}.go`、`ports/evaluation_request.go`、`domain/evaluation_request_test.go`（都只注释 / 测试文本）、`internal/parcelpricing/adapters/postgres/evaluation_test.go`（`evaluationValue` 一处）、本票面、sa-cc spec 一行。

## 要裁的

零。

## 参照

[17](17-sa-nil-dependency-text-sa01-count-comments-and-pp-synthetic-evaluation-fixture.md) Comments「评审 ← 通道 1」Standards ② ③、作者判断项 ④；`internal/parcelpricing/pptest/evaluation.go`（`Value` 头注）；AGENTS.md「写代码注释」。

## Comments

- 2026-09-14 10:1x · 通道 4（task-c0000fd2）：立票，Status 直接 in-progress，作者自立自做。**只写票面，未动代码。** 两条在 `6bbf2bf0` 上重量过：条 1 比派单多出 `evaluation_request_handoff.go` 生产头注与 `domain/evaluation_request_test.go` 两处同族，收；条 2 `saTestValue` 喂的全是 SA / CC 类型，判留。
- **2026-09-14 10:1x–10:2x · 通道 4（task-c0000fd2）· 完工**。分支 `mcp4-tails2` 基远端 main `6bbf2bf0`，与 ve-disc/06 同分支（推送方重放后 main 上 SHA 会换，对照由推送方在「进 main 记录」补）。立票笔 `2a61e0d0`（两票 + 本 spec 行，无代码）。
  - **条 1 `e7809d23`**：十三句、七个文件——`adapters/postgres/evaluation_request_handoff.go` `evaluationRequestPayload` 头注；`evaluation_request_handoff_test.go` 文件头 + 一句 Fatalf；`evaluation_request_test.go` `Covers:` 一句（括注补齐为发生项 / 费用项目 / 供应商协议）+ 一句 Fatalf；`application/request_buy_evaluation.go` `RequestBuyEvaluationCommand` 头注；`request_buy_evaluation_test.go` 一句 Fatalf；`ports/evaluation_request.go` `EvaluationRequestView` 与 `EvaluationRequestIntent` 头注；`domain/evaluation_request_test.go` 文件头 + 一句 Fatalf。「三件引用 / 三件来源引用」→「发生项 / 费用项目 / 供应商协议引用」或上下文已点名处只留「来源引用」。**不动** `domain/evaluation_request.go` 那一句（数同文件自己的字段，派单裁法）。
  - **条 2 本笔**：PP `adapters/postgres/evaluation_test.go` `evaluationValue` 函数体换成一行 `return pptest.Value(t, construct, raw)`（`t.Helper()` 保留，头注写为什么留本地名），同包十一份用例的调用点零改；`cmd/parcel-dispatch/assemble_test.go` `saTestValue` **留着**——十七处调用全喂 `sadomain.*` / `ccdomain.*` 构造器，一处 PP 类型都没有，让组合根为 SA / CC 值 import PP 的夹具包会把 `pptest` 当跨上下文通用工具用、包名说的不是这件事；「一处体」的收益 PP 那对已拿到，剩下的是两个上下文各一行的助手，不是第三份 PP 夹具（派单明说这也算答案）。
  - **验（本机，钉本笔）**：`gofmt -l`（暂存 blob）空；`go build ./...` / `go vet ./...` 0；不带 DSN `go test -count=1 ./internal/settlementaccounting/... ./internal/parcelpricing/... ./cmd/parcel-dispatch/... ./internal/architecture/...` 全 ok（注释 / 测试助手，PP 真库用例无 DSN 时跳过——`evaluationValue` 转调是编译期事实，`go vet` 覆盖；推送方进 main 前全量）。未跑全量。
  - **判据逐项**：1 ✓ 模式文件 `git grep -n -f` 于 `internal/settlementaccounting/` 只剩 `domain/evaluation_request.go` 一处；三份生产文件 `-U0` 非注释差 0，其余全是 `_test.go`；2 ✓ `evaluationValue` 体一行转调，`git grep -c 'evaluationValue('` 逐文件与 `6bbf2bf0` 同数，`saTestValue` 零 diff，`git grep -l 'parcelpricing/pptest'` 仍只命中三份 `_test.go`；3 ✓ 见上；4 ✓ 本条即，清点零差（不增删文件）。
  - **红线**：零行为——条 1 只注释与 `_test.go` 文本，条 2 只 `_test.go`；待裁票面、生产 `.go` 语句零 diff。
  - **判断项（归评审）**：① 条 1 把 `request_buy_evaluation.go` 头注（数同文件自己的字段）也改了——按派单裁法本可不动，派单点名了它、点名比数词清楚；与 `domain/evaluation_request.go` 那句「不动」的裁法有一格不一致，评审若要一致可退回零行为。② 条 1 收了派单六处之外的 `evaluation_request_handoff.go` 生产头注与 `domain/evaluation_request_test.go` 两处同族。③ 条 2 取「转调」而非「删本地助手、八十余处调用点改名」——diff 一处对八十余处，体只剩一份。④ `saTestValue` 留着，理由见上；若评审认为跨上下文用 `pptest.Value` 无妨，改成转调是三行 diff。
