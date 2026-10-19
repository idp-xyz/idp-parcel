# 10 建派送任务的答复不回显 conditions：票 20 判「回显让调用方自己比」够用，所凭的回显缺一格

Category: bug
Status: resolved——2026-10-19 进 main（12、13、10 同批，代码验证钉 `b09876bf`），见「进 main 记录」。此前 in-progress——2026-10-19 评审 0 阻断（会话读 diff 审，见 Comments），待进 main。此前 in-progress——2026-10-19 作者子代理完工，待评审。此前 in-progress——2026-10-19 会话 `idp-parcel-9b` 代决并认领：照「修复方向（机制）」办，无取舍（答复形状补一格，不动编排）；分支 `claude-tfa121310`（与 12、13 同一分支，先 12、13 后 10）。此前 needs-triage——2026-10-18 会话 `idp-parcel-9b` 立（操作者与集成客户端渠道逐口核代数研判；实测发现未经对抗核验，裁定待做，动手前先复现）
Blocked by: 无
父票：[operator-channel/26](../../operator-channel/issues/26-non-author-review-backlog-for-claude-code-landings.md)
出处：逐口核研判（workflow review26-face-algebra-sweep）：/transport-fulfillment-dispatch-task-registrations
地盘：
- internal/transportfulfillment/adapters/http/open_dispatch_task.go 及其测试
- cmd/parcel-api/（新增真渠道真库回归测试文件）

代决依据：用户 2026-10-16 授权会话 `idp-parcel-9b`「你作为业务和系统专家你来自决」。

## 现象
- `dispatchTaskResponse` 只有 outcome、task、kind、objects、place、windowFrom、windowTo、openedAt、continuationReference 这几格。载荷 `DispatchTaskPayload` 收 `conditions`，答复却不回显它。
- 本轮真库实测，同一项任务在两个口上的答复不一致：
  - 手工口 201 `DISPATCH_TASK_OPENED` 的答复里没有 conditions；
  - 派送发起口对这项任务答 `DISPATCH_TASK_ALREADY_FORMED` 时，回显里带 `conditions`。
- 票 20 给本口裁「现有格够用」，理由是「`DISPATCH_TASK_ALREADY_OPEN` 已交回在册任务，差异调用方自己比得出」。可当同一任务带着不同 conditions 重投时，调用方比不出差异。

## 复现（S）
- 在 /tmp/claude-1000/-home-tops-workspace-idp-parcel/39d8f2f8-cde5-4dd1-bb34-aef8ce7c660c/scratchpad/wt-decide2 下执行 `IDP_PARCEL_POSTGRES_DSN='postgres://parcel:parcel@127.0.0.1:55432/postgres?sslmode=disable' go test -overlay /tmp/claude-1000/-home-tops-workspace-idp-parcel/39d8f2f8-cde5-4dd1-bb34-aef8ce7c660c/scratchpad/decide2/delivery-dispatch-triggers/synth/overlay-all.json -count=1 -p 1 -run 'TestAdvDDTManualOccupationIsEscapedByAHandoverCorrection' -v ./cmd/parcel-api/`。
- 日志在 /tmp/claude-1000/-home-tops-workspace-idp-parcel/39d8f2f8-cde5-4dd1-bb34-aef8ce7c660c/scratchpad/decide2/delivery-dispatch-triggers/synth/run-adv-verify.log，看「X2a 手工口占用」与「X2a 首次触发」两行。
- 读码：`writeDispatchTaskOutcome` 不写 conditions。
- 「同任务换 conditions 重投」这一格未单独实跑。

## 应当
已在册的答复回显全部内容格，包括 conditions，调用方才能照票 20 的判据自己比对。201 首答与 200 已在册两种答复同形。

## 根因
`dispatchTaskResponse` 漏了 conditions 一格。

## 修复方向（机制）
`dispatchTaskResponse` 加 `conditions`，`writeDispatchTaskOutcome` 从在册任务取值，201 与 200 两种答复都带上。时刻格式随票 31。这一改动只涉及答复形状，不动编排。

## 是否潜伏
已可触发：运营决定渠道上，演示配置员持有建派送任务的授予、准入对照与对应区间。

## 完成判据
- [x] HTTP 单测：201 与 `DISPATCH_TASK_ALREADY_OPEN` 两种答复都带 `conditions`；整秒时刻格式逐字节不变。
- [x] 真渠道真库回归（`cmd/parcel-api` 新测试文件，`buildOperatorDecisionIntakes` + 生产准入桥 + `buildSegmentOperations`）：以 conditions=A 建任务；同一任务以 conditions=B 重投，答 200 `DISPATCH_TASK_ALREADY_OPEN`，回显 conditions=A，库不变。
- [x] `go build ./...`、`go vet ./...` 退出码 0，`gofmt -l .` 无输出；带 DSN 跑 `go test -p 1 -count=1 ./internal/transportfulfillment/... ./cmd/parcel-api/...`，全部 ok。
- [x] 票 15 补记下追加回写行（只加行）。
- [x] 走 Go 那条路（[并行会话](../../../docs/agents/parallel-sessions.md)）：作者自验，非作者评审（两轴分开），推送方在干净检出上带 DSN 跑全量。

## 完成记录（2026-10-19，会话 idp-parcel-9b 的实施子代理，分支 claude-tfa121310）

基线 `a2d6a4bf`（叠在票 12、13 之上）。提交：`087ceb95`（修复与用例）、`c256ac7e`（机制清点，12、13、10 合一笔）。

### 落点
- `internal/transportfulfillment/adapters/http/open_dispatch_task.go`：`dispatchTaskResponse` 加 `conditions`（`omitempty`，格序与 `deliveryDispatchTriggerResponse` 的任务那几格一致：windowTo 之后、openedAt 之前）；`writeDispatchTaskOutcome` 从 `record.Task.Conditions()` 取。201 取刚登记的那一份、200 已在册取读回的在册任务，同走一个回显块，两种答复同形。编排未动。
- 时刻：本改动不碰时刻格，仍经 `rfc3339()`；整秒值逐字节不变由 HTTP 单测的整体比对钉住。

### 各格对应用例
- HTTP 单测：`internal/transportfulfillment/adapters/http/open_dispatch_task_test.go`（新文件）的 `TestOpenDispatchTaskAnswersEchoTheRegisteredConditions`——conditions=A 建任务答 201，整个答复体逐字节比对（含 `"conditions":"SYN-CONDITION/A"` 与 `2026-09-06T09:00:00Z` 一类整秒时刻）；同一任务 conditions=B 重投答 200 `DISPATCH_TASK_ALREADY_OPEN`，答复体除 outcome 外与首答逐字节相同、回显 A。
- 真渠道真库：`cmd/parcel-api/dispatch_task_conditions_echo_test.go`（新文件）的 `TestTheDispatchTaskFaceEchoesTheRegisteredConditionsOnAReplay`——装配照判据：`registerOperationDecisionAdmission` + `buildAdmissionScope`（生产准入桥）+ `buildOperatorMinter` + `buildOperatorDecisionIntakes` + `buildSegmentOperations`，端点 `tfhttp.NewOpenDispatchTaskEndpoint(decisions.transport, operations.opener)`。A 建答 201 回显 A；B 重投答 200 `DISPATCH_TASK_ALREADY_OPEN` 回显 A；`transport_fulfillment.dispatch_task` 里该任务仍 1 行、`conditions_ref` 仍是 A。

### 先红后绿
- 复现：票面「复现」节引的 overlay 未用，改由新写的真渠道真库用例先红。
- 红（票 13 之后、本票改代码之前 + 新用例）：`/tmp/claude-1000/-home-tops-workspace-idp-parcel/39d8f2f8-cde5-4dd1-bb34-aef8ce7c660c/scratchpad/tfa10-red.log`。HTTP 单测首答 got 为 `{"outcome":"DISPATCH_TASK_OPENED",…,"windowTo":"2026-09-06T12:00:00Z","openedAt":"2026-09-05T12:00:00Z"}`，没有 conditions；真渠道真库原话：「以 A 建任务：201 {"outcome":"DISPATCH_TASK_OPENED","task":"SYN-DISPATCH-COND-ECHO",…,"windowTo":"2026-10-19T12:00:00Z","openedAt":"2026-10-18T20:00:00Z"} , want 201 DISPATCH_TASK_OPENED 回显 conditions A」。两条都红在 201 首答（Fatal 之后重投那段没跑到）；重投那段由同一用例在修复后走通，「同任务换 conditions 重投」这一格由此实跑。
- 绿：`/tmp/claude-1000/-home-tops-workspace-idp-parcel/39d8f2f8-cde5-4dd1-bb34-aef8ce7c660c/scratchpad/tfa10-green.log`。

### 门
- 钉 `087ceb95`：改动的 `.go` 跑 `gofmt -l` 无输出（整树 `gofmt -l .` 同样无输出）；`go build ./...`、`go vet ./...` 退 0（`/tmp/claude-1000/-home-tops-workspace-idp-parcel/39d8f2f8-cde5-4dd1-bb34-aef8ce7c660c/scratchpad/tfa-gates-build.log`）。
- 带 DSN `go test -p 1 -count=1 ./internal/transportfulfillment/... ./cmd/parcel-api/... ./internal/architecture/...` 全 ok（`/tmp/claude-1000/-home-tops-workspace-idp-parcel/39d8f2f8-cde5-4dd1-bb34-aef8ce7c660c/scratchpad/tfa-gates-test.log`）。未跑全仓全量（属推送方）。
- 机制清点重生成：只有测试文件计数变（`c256ac7e`，`cmd/` 测试 +1），生产文件不变。

### 越出地盘
无。

证据层级：`S`（`SYN-` 值；真渠道走隔离合成的准入对照与区间）。

### 未做
- 「票 15 补记下追加回写行」：**留给推送方**（票 15 正在另一条分支改，合并时补）。
- 「走 Go 那条路」的非作者评审与推送方干净检出全量：属会话与推送方。

## Comments

### 评审 ← 会话 idp-parcel-9b 读 diff 审（不算非作者评审；2026-10-19，钉 `ea540e94`）

两轴合审，0 阻断。

- **Spec**：`conditions` 从在册任务取，201 与 `DISPATCH_TASK_ALREADY_OPEN` 同形；格序与派送发起口的任务那几格一致。HTTP 单测逐字节比对（含整秒时刻格式）；真渠道真库以 A 建、以 B 重投答 200 回显 A、库里一行仍是 A。编排不动。
- **Standards**：头注写为什么（少一格调用方比不出差异）。
- 票 15 回写行由推送方在进 main 时补。

### 进 main 记录（2026-10-19，会话 idp-parcel-9b）

- 分支 `claude-tfa121310`（12、13、10 同批）rebase 到远端 main 之上快进推送；rebase 只撞机制清点，在新树上重生成解决（cmd 测试文件 148）。验证钉之后远端只前进了一笔 `apps/admin-web/**`（operator-channel/44）与纯票面，再 rebase 一次，`git diff b09876bf HEAD -- . ':!.scratch' ':!apps/admin-web'` 为空，未重跑。
- 票 15 补记下的回写行由推送方随本笔追加。
- 推送方全量一次：钉 `b09876bf` 的隔离 detached 检出，带 DSN `go test -p 1 -count=1 -v ./...` 退 0——有测试的包 143 个 ok、0 FAIL；`--- SKIP` 1（`TestHelperTemplateOwnerProcess`，照设计跳过）。同一检出上 `gofmt -l .` 无输出，`go build ./...`、`go vet ./...` 退 0，机制清点重生成零差。
