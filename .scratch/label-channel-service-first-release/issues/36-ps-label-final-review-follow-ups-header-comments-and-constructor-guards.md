# 面单渠道链四份非作者评审的同族非阻断项一笔收口：跨文件计数头注、讲错的头注、构造期拒 nil、error 前缀、合成 / 解析收成一对

Category: chore
Status: resolved——**完工待评审进 main，2026-09-11 16:2x**（分支 `mcp2-lc36` 基 `807de571`，代码 tip `b5a7fb81`、清点 `614cda04`；九条逐笔对应见 Comments「完工」）；此前 in-progress——2026-09-11 16:0x 通道 2 按通道 1 派单 task-f20edb3f 认领，分支 `mcp2-lc36` 基远端 main `807de571`（树 `D:/tops/idp-parcel-mcp2-lc36`）；此前 ready-for-agent——2026-09-11 14:2x 通道 1 推送方立票并直接转 ready（要裁的为零：每一条都是评审已判「非阻断、可改」且作者未回改的项，改法评审原话已给）；取证锚 main `0b027ab8`
Blocked by: 无（lc/27 / 30 / 32 / 34 全部已进 main）。撞点：本票只动下列点名文件的注释与构造器 / 错误文本，与第五波五单（CC / SA / VE）零重叠

## 缺口（出处逐条指到评审原话；取证于 `0b027ab8`）

四张票的非作者评审各留了几条 Standards 非阻断，处置都是「随票记、作者可另立票」，没有人立。tasks.md 12:0x 节把它们收成一句「PS 一张小票收下今天四份评审点的同族头注 / 构造器项」，本票就是那张。逐条：

1. **lc/34 Standards ①**——`cmd/parcel-api/main.go` `run` 里 `buildLabelChannelOrchestration` 那块的头注末句「六个实例半边缝仍全部显式未配置」：数的是另一文件 `assemble_label_channel.go` `labelChannelSeams` 的字段（AGENTS.md「计数与行号同构」）。改法：去数字（「实例半边缝仍全部显式未配置」）。`assemble_label_channel.go` 自己头注里的「六个 / 三个」数的是同文件的结构体字段，不在本票。
2. **lc/27 Standards ①**——`cmd/parcel-dispatch/assemble.go` `labelFinalJudgmentCore` 头注「三路触发」「判断编排的六口」：数的是 ADR-0134 与 `JudgeLabelServiceFinalDeps` 的东西。改法：去数字（「各路触发共用的处理方核」「判断编排的各口」）。同族：`internal/parcelshipment/adapters/labelfinal/judge_parcel.go` 包头注「三路触发」——ADR-0134 决定的路数一变它就错，一并去数。
3. **lc/32 Standards ①**——`internal/parcelshipment/application/operate_label_transaction.go` `NewLabelTransactionHandler` 只拒 `Registers` / `Finals` 两口，头注理由「漏装要到第一次建立才 panic……没有它们的门等于没有门」不成立（nil 接口在 `closedParcels` 里会响亮 panic，不会静默放行）。评审给的两条改法**二选一**：全部必填口构造期拒 nil（含 `Judgments`，弃 lc/26 的运行期拒——先例 `NewFormContinuedAttemptDecisionHandler`），或头注改写成真实理由（装配点 fail-fast，与 lc/34 同一纪律）。本票取**前者**（与 sa-cc/14 给 `NewDutyPaymentReconciliationHandler` 的形一致：表驱动逐口点名、`fmt.Errorf("%w: %s", …)`）；若签名因此变成 `(*H, error)`，调用点与夹具随之接错误（`cmd/parcel-api/assemble_label_channel.go`、`application` 测试夹具）。
4. **lc/32 Standards ③**——同文件 `closedParcels` 两处 `fmt.Errorf("establish label transaction: 读包裹 %s 当前有效终局：%w")` 在英文 error 前缀惯例里掺中文。改法：与同文件其余 error 文本统一（照该文件既有多数写法，不另立第三种）。
5. **lc/30 Standards ①**——`internal/parcelshipment/adapters/partycommercial/continued_attempt_decision_authorization.go` `NewContinuedAttemptDecisionAuthorizationAdapter` 不在构造期拒 nil `adjudicate`，推到调用期才报。改法：构造期拒（`requests` 为 nil 仍是有意的「显式未配置」，不拒——头注写明两口为何一拒一不拒）。
6. **lc/30 Standards ②**——`internal/parcelshipment/ports/continued_attempt_decision.go` 查询头注说 Requester「只作请求事实随查询进提供方」，而 `formContinuedAttemptDecisionRequest` 并不把 `query.Requester` 交给 PC——注释声称了适配器没做的事。改法：头注改成代码真做的事（Requester 在哪一层用、不进 PC）。
7. **lc/30 Standards ③**——同适配器 `AuthorizeContinuedAttemptDecision` 处注释按符号名指到 PC `permits`（`grant.level == request.level`），但票 30 裁决 ③ 说 `AuthorityRole` 取请求等级「为暂行」而注释未标。改法：注释加「暂行」并指到票 30 裁决 ③。
8. **lc/30 Standards ⑤**——裁决 ②「合成 / 解析收在一对函数」：解析在 `closureResponsibilitySourceKindOf`，合成内联在 `FormControlledClosure` 里，不是一对。改法：把合成抽成与解析同名对偶的一个函数（同包、同文件相邻），行为不变；用例钉「合成后解析得回原种类」。**不做**评审 ④（把种类抬进 `domain/**` 成类型）——那是形状改动，归 lc/35 或 PS owner 另票。
9. **lc/30 头注「导出给 27」改口**——`internal/parcelshipment/adapters/postgres/continued_attempt_decision_handoff.go` 头注「导出是给 27 的消费者与路由表引同一个串」：lc/27 按 inbox 惯例自写了 `eventing.EventType` 常量，只有消费者**测试**import 它作对照（票 27「取舍两处」①）。改法：头注改成事实（「导出只供消费者测试对照两串相等；消费者自写字符串，不 import 本适配器」）。

**不在本票**：lc/32 Standards ②（三处「读终局 → 读册 → 空册」同形抽 helper，归 PS owner 另票）、lc/30 Standards ④（种类抬成领域类型）、lc/34 Standards ②（适配器头注反向指组合根，评审判不要求改）、lc/27 Standards ②（每路各装一只核，判断题不改）；lc/27 一条龙验收、lc/30 (c)(d) / 偏离 ④、lc/32 并发窄格——都是行为或形状题，各归 owner。

## 做法

按上面 1–9 逐条改；1 / 2 / 6 / 7 / 9 是注释，4 是错误文本，3 / 5 / 8 是构造器与函数形状但**零行为改动**（拒 nil 只在装配错误时可见；抽函数等价）。每条一笔或合并成两三笔都行，提交信里按条号点名。3 若改签名，调用点与夹具同笔接。

## 红线

- 不改任何判断语义：`Judge` / `closedParcels` / `JudgeLabelServiceFinalHandler` / `FormContinuedAttemptDecisionHandler` 的分支与结果代数一字不动。
- 不动 `domain/**`（评审 ④ 不做）。
- 注释改口只换成代码真做的事与真实理由，不添新的计数、不引行号。

## 完成判据

1. `git grep -n -E '六个实例半边缝' -- cmd/parcel-api/main.go` 零；`git grep -n -E '三路|六口' -- cmd/parcel-dispatch/assemble.go internal/parcelshipment/adapters/labelfinal/judge_parcel.go` 零。
2. `NewLabelTransactionHandler` 每口各缺一 → 具名错误（用例照 `TestTheReconciliationHandlerNamesWhichDependencyIsMissing` 的形），口齐不拒；`NewContinuedAttemptDecisionAuthorizationAdapter` `adjudicate` 为 nil → 构造期错，`requests` 为 nil → 仍构造成功且答显式未配置（既有用例不变）。
3. 合成 / 解析对偶函数各一个、相邻，用例钉往返。
4. 三处头注（6 / 7 / 9）改后与代码逐句对得上（评审读代码核）。
5. `gofmt -l` 空、`go build ./...` / `go vet ./...` 退 0；`go test -count=1` PS `application` + PS `adapters/partycommercial` + PS `adapters/postgres`（带 DSN，占 55432 前后广播）+ `cmd/parcel-api` + `cmd/parcel-dispatch`（带 DSN）+ `./internal/architecture/...`；清点 tip 重生成核零差（不加文件不加端口，应零差）。

## 地盘

`cmd/parcel-api/main.go` 一句、`cmd/parcel-dispatch/assemble.go` 两句（`labelFinalJudgmentCore` 头注）、`internal/parcelshipment/adapters/labelfinal/judge_parcel.go` 包头注、`internal/parcelshipment/application/operate_label_transaction.go`（构造器 + 两处错误文本 + 用例）、`internal/parcelshipment/adapters/partycommercial/continued_attempt_decision_authorization.go`（构造器 + 两处注释 + 用例）、`internal/parcelshipment/ports/continued_attempt_decision.go` 一处头注、`internal/parcelshipment/application/` 里 `FormControlledClosure` 所在文件（抽对偶函数）、`internal/parcelshipment/adapters/postgres/continued_attempt_decision_handoff.go` 一处头注；3 若改签名则 `cmd/parcel-api/assemble_label_channel.go` 调用点一行。**不动** `domain/**`、路由表、任何 CC / SA / VE 文件。共享接线文件 `assemble.go` / `main.go` 只改注释，动前仍占号。

## 参照

[lc/34](34-label-channel-chain-startup-assembly-fail-fast.md) Comments 评审 Standards ①；[lc/27](27-controlled-close-reopen-decision-triggers-label-final-judgment.md) Comments 评审 Standards ① 与「取舍两处」①；[lc/32](32-establish-label-transaction-checks-continued-attempt-register.md) Comments 评审 Standards ① ③；[lc/30](30-controlled-close-reopen-decision-write-face-ps-half.md) Comments 评审 Standards ① ② ③ ⑤ 与裁决 ② ③；[sa-cc/14](../../sa-cc-funds-and-credential-seams/issues/14-adopt-digest-header-and-duty-reconciliation-handler-rejects-nil.md)（构造期拒 nil 的形）；AGENTS.md「写代码注释」；tasks.md 2026-09-11 12:0x 节「候选后继」。

## Comments

- 2026-09-11 14:2x · 通道 1 推送方：立票。**只写票面，未动代码。** 能力边界：九条全部抄自四份评审原话与票 27「取舍两处」，文件与符号名按评审所指；`FormControlledClosure` 所在文件名没有核，实施时 `git grep -n FormControlledClosure -- internal/parcelshipment/application` 一下即得。
- **2026-09-11 16:0x–16:2x · 通道 2（task-f20edb3f）· 完工**。分支 `mcp2-lc36` 基远端 main `807de571`，八笔：`a3e130ab` 票面 in-progress；`8a4ab2b8` **条 3 + 4**（`NewLabelTransactionHandler` 五口表驱动逐口拒 nil，`ErrNilDependency` 哨兵 + `fmt.Errorf("%w: %s")`，Judgments 进构造门、lc/26 在 `judgmentBeat` 里的运行期 nil 门撤掉；旧头注「漏装要到第一次建立才 panic」改写成真实理由；`closedParcels` 两处 + 同族 `priorLink`「读原交易」一处 error 前缀改英文——第三处是同文件同一族，不改会留下一条中英混杂；签名本就 `(*H, error)`，调用点不动；用例 `TestTheLabelTransactionHandlerNamesWhichDependencyIsMissing` 照 CC 那条的形，lc/26 的 `TestTheLastTwoBeatsRefuseToLandWithoutAJudgmentHandoff` 撤——那一格改在构造期证）；`7b396803` **条 5 + 6 + 7**（`NewContinuedAttemptDecisionAuthorizationAdapter` → `(*Adapter, error)`、拒 nil `adjudicate`、`requests` nil 不拒并在头注写明一拒一不拒；运行期 `adjudicate == nil` 门撤；调用点 `cmd/parcel-api/assemble_continued_attempt_decision.go` 接错误、测试夹具收 `t`；新用例 `TestTheAuthorizerRefusesANilAdjudicatorButAcceptsAnUnconfiguredRequestMapping`；ports 查询头注改成「Requester 不进提供方、今天无适配器读它、用处在写面」；`AuthorityRole` 处注释加「暂行」指票 30 裁决 ③）；`e0acde49` **条 8**（合成抽成 `closureResponsibilitySourceOf` 与 `closureResponsibilitySourceKindOf` 相邻对偶，空种类 / 空白主体在函数内拒成 error、`FormControlledClosure` 折成同一格 `NOT_ACCEPTED / RESPONSIBILITY_SOURCE_UNKNOWN`，结果代数逐格同前；新包内用例文件 `form_continued_attempt_decision_internal_test.go` 两条：三种类往返得回原种类 + 主体空白去掉 + 落册串形状；没名字的种类与空白主体合成不出）；`0c57084f` **条 9**（handoff 头注改成事实：导出只为测试对照，消费者自写常量、路由表引 inbox 那一个）；`35fc65d8` **条 2**（`assemble.go` `labelFinalJudgmentCore` 与 `judgeLabelFinalOnLabelTransactionConsumer` 头注「三路 / 六口」→「各路 / 各口」；`judge_parcel.go` 包头注与 `ParcelJudgmentCore` 头注去「三路」并注明哪几路由 ADR-0134 定）；`b5a7fb81` **条 1**（`main.go` 那一句去「六个」，不动空行）；`614cda04` 清点（**非零差**：PS 测试 169→170，条 8 的包内用例文件——对偶函数不导出，往返只能从包内测；生产 / 端口 / 端点零差）。**验（本机，钉 `b5a7fb81`）**：`gofmt -l` 空；`go build ./...` / `go vet ./...` 0；16:21 占号，带 DSN `go test -count=1 -p 1` PS `application` + PS `adapters/partycommercial` + PS `adapters/postgres` + `cmd/parcel-api` + `cmd/parcel-dispatch` + `./internal/architecture/...` 六包 ok，新用例 `-v` PASS；16:23 释号。未跑全量。**判据逐项**：1 ✓ 两条 `git grep` 于 `b5a7fb81` 零命中；2 ✓ 五口各缺一具名错误 + 口齐不拒（`8a4ab2b8`）、`adjudicate` nil 构造期错 + `requests` nil 构造成功且既有「映射未配置」用例不变（`7b396803`）；3 ✓ 对偶函数相邻、往返用例（`e0acde49`）；4 ✓ 条 6 / 7 / 9 头注改后与代码逐句对得上（评审读代码核）；5 ✓ 见上，清点有差已单独成笔并说明。**红线**：`Judge` / `closedParcels` / `JudgeLabelServiceFinalHandler` / `FormContinuedAttemptDecisionHandler` 分支与结果代数一字未动；`domain/**` 未动；新注释无计数、无行号。**撞点**：`assemble.go` 只改 `labelFinalJudgmentCore` 一族头注（与通道 5 路由表块不同）；`main.go` 只改那一句（与通道 6 插入点相邻不同行）；重放时逐块核。**判断项（归评审）**：① 条 4 多改了同文件 `priorLink` 那一处「读原交易」——票面只点名 `closedParcels` 两处，第三处同族同文件；② 条 3 撤掉 lc/26 的运行期 nil 门与其用例——票面写「弃 lc/26 的运行期拒」，我读成连门带用例一起撤，用例的那一格由构造期用例接住。
