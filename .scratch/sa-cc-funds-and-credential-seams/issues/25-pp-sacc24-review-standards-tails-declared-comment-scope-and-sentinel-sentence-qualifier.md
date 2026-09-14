# sa-cc/24 评审 Standards ① 前半 + ② + Spec ① 三条非阻断一笔收口：`Declared()` 头注「只有这里要改」收窄到校验、`assemble.go` 哨兵末句加限定并把「这一格」写成符号名

Category: chore
Status: 已进 main——2026-09-14 21:4x 通道 1 推送方：`mcp2-tails5@b0463e1a` 重放到 main `84e37713` 之上为 **`58ef74bd`**（与 19 / 20 / 21 / 22 取证四笔同批，批 tip `09833d67`；本簿记笔在其上）；`09833d67` 带 DSN 全仓 112 ok / 1 FAIL——那 1 FAIL 是 main 既有 flaky（CC postgres `TestFundsFactVersionsAccrueAsRowsThatPointBack`，与本票零关系，见 Comments 进 main 记录）；评审门推送方自审。此前 resolved——**完工待进 main，2026-09-14 21:2x 通道 2**（按通道 1 派单 task-4edf2e4d 自立自做；分支 `mcp2-tails5` 基远端 main `0bea3610`，树 `%TEMP%\idp-parcel-mcp2-tails5`；评审门：评审人自做自己评出的尾巴，纯注释，推送方自审，票面如实写）。此前 in-progress——2026-09-14 21:1x 通道 2 自立；要裁的为零
Blocked by: 无（[24](24-pp-sacc11-review-standards-tails-sentinel-comment-and-evidence-closed-set.md) 已进 main `5eb3caee` / 簿记 `0bea3610`，三条出处全在其 Comments「评审 ← 通道 2」与「处置」）。撞点：通道 3 在 `.scratch/pp-pricing-input-seams/`、通道 4 / 5 / 6 在本目录 19 / 20 / 21 / 22 的 Comments，都不碰本票两份代码文件与本票面；`spec.md` 只本通道动

## 缺口（出处逐条指到评审原话；取证于 `0bea3610`）

1. **头注把「只有这里要改」说到了整个封闭集**（24 评审 Standards ① 前半；AGENTS.md「写代码注释」——注释只写代码讲不出的东西，讲错的比不讲更糟）。`internal/parcelpricing/domain/value_objects.go` `EvidenceKind.Declared()` 头注「封闭集多一格时只有这里要改」：校验确已归领域一处，但集合增一格时 `TranslateEvaluationRequest` / `NewFormOnEvaluationRequestSubmittedAdapter` / `EvaluationReplayPayload.Command` 三句错误文本仍要跟着改——那一侧是错误文本变、属行为，24 处置已归 PP owner 另判。本票只把句子收窄到它真正守住的那半：校验。
2. **哨兵末句的三分号称穷尽「每一封」，且第三枚以指代词点名**（24 评审 Standards ② + Spec ①）。`cmd/parcel-dispatch/assemble.go` `evaluationRequestSubmittedUndecidedSentinels` 头注 `ErrPricingInputUnavailable` 条目末句「今天每一封 BUY 请求信封都停在形成之前：范围下没卡停…、多卡停…、有恰一张卡的停在这一格」：冒号后按卡数三分，漏同名单里到不了价卡解析的 `ErrEvaluationRequestNotVisible`；「这一格」两处指代 `ErrPricingInputUnavailable` 而不写符号名——24 做法 1 说的是「点名三格」，`git grep` 拿符号名搜不到这一句。

**不在本票**：24 评审 Standards ① 后半（三句错误文本字面枚举「S / R / P」——改成从领域取成员列表是错误文本变，24 红线明禁，归 PP owner 判值不值得另立票）；Standards ③（票面判断项措辞，24 处置已收窄、原文不改写）；任何生产语句；任何测试。

## 做法

1. `Declared()` 头注「封闭集多一格时只有这里要改」→「封闭集多一格时校验只有这里要改」。只注释。
2. `assemble.go` 末句改为「今天每一封 BUY 请求信封都停在形成之前；到得了价卡解析的那些，范围下没卡停 ErrPriceCardNotConfigured、多卡停 ErrPriceCardApplicabilityConflict、有恰一张卡的停在 ErrPricingInputUnavailable——真库装配例证的正例断的正是 ErrPricingInputUnavailable」：全称与三分之间用分号断开、三分只管「到得了价卡解析的那些」，两处「这一格」都写成符号名。切片字面零 diff。

## 红线

- 零行为：`git diff` 除 .md 外只许注释行；`TranslateEvaluationRequest` / `NewFormOnEvaluationRequestSubmittedAdapter` / `EvaluationReplayPayload.Command` 三句错误文本一字不动；不动任何测试。
- 注释中文；不写行号、不数别处（AGENTS.md「改文档」计数条，Go 注释同受约束）。
- `assemble.go` 是共享接线文件：动前广播「占」、推完广播「释」。

## 完成判据

1. `git grep -n '封闭集多一格时只有这里要改' -- internal/parcelpricing/` 零命中；`Declared()` 头注仍在、句子已收窄。
2. `git grep -n '到得了价卡解析' -- cmd/parcel-dispatch/assemble.go` 命中一处；该条目末句含 `ErrPricingInputUnavailable` 符号名；`git grep -n '这一格' -- cmd/parcel-dispatch/assemble.go` 只剩与本句无关的既有处（完成记录列出）。
3. `gofmt -l ./cmd ./internal` 空；`go build ./...` / `go vet ./cmd/parcel-dispatch/ ./internal/parcelpricing/...` 退 0；不带 DSN `go test -count=1 ./internal/parcelpricing/domain/ ./cmd/parcel-dispatch/ ./internal/architecture/...` ok；带 DSN 全仓由推送方在 tip 上跑一次。
4. 完成记录同笔；清点预报零差（不增删代码文件；新增只有本票面 .md，`tools/mechanism-inventory` 不数 `.scratch/`）。

## 地盘

`internal/parcelpricing/domain/value_objects.go`（一处头注）、`cmd/parcel-dispatch/assemble.go`（一处头注）、本票面、sa-cc spec 一行。

## 参照

[24](24-pp-sacc11-review-standards-tails-sentinel-comment-and-evidence-closed-set.md) Comments「评审 ← 通道 2」Standards ① ② / Spec ①、「处置」；[lc/41](../../label-channel-service-first-release/issues/41-lc40-review-standards-tail-channel-basis-translation-stopped-comment-count.md)、[lc/40](../../label-channel-service-first-release/issues/40-lc35-review-standards-tails-test-header-counts-and-transaction-shell-sentence.md)（同款 A 类尾巴先例）；AGENTS.md「写代码注释」「改文档」。

## Comments

- **2026-09-14 21:1x–21:2x · 通道 2（评审人自做）· 完工**。分支 `mcp2-tails5` 基远端 main `0bea3610`，立票 + 两处 + 完成记录同一笔（本笔）。
  - **条 1**：`value_objects.go` `Declared()` 头注一行，「只有这里要改」前加「校验」；其余一字不动（+1 −1）。
  - **条 2**：`assemble.go` 条目末句改为做法 2 所写（+3 −2，含折行）；冒号改分号、加「到得了价卡解析的那些」、两处「这一格」写成 `ErrPricingInputUnavailable`。
  - **验（本机，钉本笔，不带 DSN）**：`git diff --stat` 两份 .go 共 +4 −3、每一改行以 `//` 开头；`gofmt -l ./cmd ./internal` 空；`go build ./...` 0；`go vet ./cmd/parcel-dispatch/ ./internal/parcelpricing/...` 0；`go test -count=1` PP domain + `cmd/parcel-dispatch`（无库时真库用例自跳）+ architecture 三包 ok。带 DSN 全仓由推送方在 tip 上跑一次。
  - **判据逐项**：1 ✓（`封闭集多一格时只有这里要改` 于 PP 零命中；`Declared 报出` 头注仍在）；2 ✓（`到得了价卡解析` 恰一处；末句含符号名两次；`这一格` 剩余命中全是与本句无关的既有处——`migrate` 只读那段、TF 重派生那格、SA 缺引用那格、装配方说出来那句、终局链与采用链那两句）；3 ✓；4 ✓ 本条（`git diff --stat` 两份 .go 全注释行，新增只本票面 .md）。
  - **红线**：`git diff -- cmd internal` 每一改行都以 `//` 开头；三句错误文本零 diff；测试零 diff；`assemble.go` 动前 21:1x 广播占、推完释。
  - **判断项（归评审 / 推送方）**：① 「到得了价卡解析的那些」而不是「到得了价卡解析的」——后者单独成分读起来像修饰「范围」，加「那些」让主语落在信封上。② 全称与三分之间用分号不用冒号——冒号会把后面读成前面的展开（正是 24 评审 ② 点的病），分号让「都停在形成之前」自成一句。③ 末尾「正例断的正是 ErrPricingInputUnavailable」而不是重复「停在」——它说的是 `assemble_test.go` 那条用例**断言**的格，不是又一次停格描述。
  - **评审门**：三条都是本通道 21:0x 评审自己评出来的零行为注释尾巴，纯注释由推送方自审（24 处置条的派法），票面如实写；评审人做自己评出的尾巴不违「非作者评审」（作者 = 5eb3caee 的通道 1，本笔作者 = 通道 2，各不评自己写的那半）。
- **2026-09-14 21:4x · 进 main 记录 · 通道 1 推送方**：隔离检出 `%TEMP%\idp-replay-tails5` @ `84e37713`（main 自 `0bea3610` 起前进两笔，只动 tasks.md 与新目录 `.scratch/pp-pricing-input-seams/`，与本票四文件零重叠），`cherry-pick b0463e1a` 零冲突 → **`58ef74bd`**，`git diff b0463e1a 58ef74bd -- cmd internal .scratch/sa-cc-funds-and-credential-seams` 空；同一重放树再叠 19 + 22 / 20 / 21 三笔取证（`cbdcb360→f248a0b9` / `5fab18e0→af75066e` / `a42bdbfb→09833d67`），批 tip `09833d67`。**推送方自审**：`git diff 0bea3610 b0463e1a -- cmd internal` 逐行读，两份 .go 每一改行以 `//` 开头、三句错误文本零 diff、切片字面零 diff；票面判据 1–4 与判断项 ①–③ 逐条对过，接受。**验证（推送方全量一次）**：`gofmt -l` 空、`go build ./...` / `go vet` 0；21:3x 先 `check_messages(waitMs 0)` 排队列再广播占 55432，`09833d67` 带 DSN `go test -p 1 -count=1 ./...` **112 ok / 1 FAIL / 16 无测试 / 0 cached**（129 s）。**那 1 FAIL 与本批零关系、是 main 既有 flaky**：`internal/customscompliance/adapters/postgres` `TestFundsFactVersionsAccrueAsRowsThatPointBack`「v1 走样」——该包及 `migrations/customs_compliance/` 在 `84e37713` 与 `09833d67` 之间 `git diff --stat` 为空；成因是用例以 `map[string]ports.ExternalFundsFactRegistration{"v1","v2"}` 迭代登记两版，Go 小 map 迭代起点随机、每版各自一笔事务（`register` → `WithinTransaction`）故 `received_at` 不同，`ListFundsFactVersions ORDER BY received_at ASC` 在 v2 先登时如实把 v2 列在前，用例却断言 `versions[0]` 是 v1；同 tip 单跑该用例 `-count=40` 得 **36 PASS / 4 FAIL**（钉 `09833d67`），全仓那一次撞上的就是这 1/8 左右的那格。用例自 sa-cc/13 `e833355d` 进 main 起即如此，今天此前的历次全仓绿是运气。**处置**：按 parallel-sessions「有已知 flaky 该做的是把它修掉或点名」——点名于此、立票修（sa-cc/26，派通道 3），本批不因它重跑一遍「凑绿」。释号广播。清点不重生成（不增删代码文件）。共享 main `merge --ff-only 09833d67` → 簿记一笔在其上 → `ls-remote` 核 `84e37713` 未动 → `push <sha>:main`。`mcp2-tails5` → `merged/`、远端删；作者树与重放树比内容后拆。
