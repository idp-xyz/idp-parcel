# 13 装载分配：链头是撤回版时，「分配已形成」的答复看不出分配已撤回

Category: bug
Status: resolved——2026-10-19 进 main（12、13、10 同批，代码验证钉 `b09876bf`），见「进 main 记录」。此前 in-progress——2026-10-19 评审 0 阻断（会话读 diff 审，见 Comments），待进 main。此前 in-progress——2026-10-19 作者子代理完工，待评审。此前 in-progress——2026-10-19 会话 `idp-parcel-9b` 认领，分支 `claude-tfa121310`（与 12、10 同一分支）。此前 ready-for-agent——2026-10-19 会话 `idp-parcel-9b` 立（tf-face-algebra/02 实施中发现，票外未改）
Blocked by: 无
父票：[operator-channel/26](../../operator-channel/issues/26-non-author-review-backlog-for-claude-code-landings.md)
出处：tf-face-algebra/02 实施子代理的「本票外的发现」，经其非作者评审确认。
地盘：
- internal/transportfulfillment/adapters/http/form_load_assignment.go 及其测试
- internal/transportfulfillment/application/form_load_assignment.go（只在需要从结果取撤回标记时）

## 现象

tf-face-algebra/02 之后，分配已有版本时答 `LOAD_ASSIGNMENT_ALREADY_FORMED` 并回显链头。链头若是撤回版，答复里没有撤回标记，调用方会把一个已撤回的分配读成仍在生效。今天撤回入口没接，所以潜伏。

## 应当

回显链头时带上它是否已撤回（与撤回时刻），调用方据此决定另起分配。

## 完成判据

- [x] HTTP 用例：链头为撤回版时答复带撤回标记；非撤回链头答复形状不变。
- [x] Go 门禁照常。

## 完成记录（2026-10-19，会话 idp-parcel-9b 的实施子代理，分支 claude-tfa121310）

基线 `a2d6a4bf`（叠在票 12 的 `299682ef` 之上）。提交：`65cbe3a3`（修复与用例）、`c256ac7e`（机制清点，12、13、10 合一笔）。

### 落点
- `internal/transportfulfillment/adapters/http/form_load_assignment.go`：`loadAssignmentResponse` 加 `withdrawn`、`withdrawnAt` 两键（都 `omitempty`）；`writeLoadAssignmentOutcome` 在有记录时从 `record.Assignment.Withdrawn()` 取，时刻经本包 `rfc3339()` 按登记册精度写，没有另写一套。
- application 未改：结果交回的就是 `ports.LoadAssignmentRecord`，撤回标记从领域值上取得到。
- 取舍：非撤回时**整键省略**，不给 false 或空串。理由写在 `loadAssignmentResponse` 头注——本包可空格一律以缺席表「未发生」（`optionalInstant` 头注、`transportHandoverBody` 的 corrects 两键）；另用 false 占位，调用方就得为同一种「没有」认两种写法。结果是非撤回链头的答复逐字节不变，正合判据「形状不变」。
- 两键挂在「有记录」那一支而不只挂在`分配已形成`：`版本已在册`回显的若恰是撤回版（同一版本号原样重投），也同样带——同一份回显不按 outcome 分两种写法。这一路未单钉。

### 各格对应用例
- `internal/transportfulfillment/adapters/http/form_load_assignment_test.go`（新文件）：
  - `TestFormLoadAssignmentAlreadyFormedShowsAWithdrawnChainHead`：链头为撤回版时答 200 `LOAD_ASSIGNMENT_ALREADY_FORMED`，`version` 是撤回版，`withdrawn` 为 true，`withdrawnAt` 为 `2026-09-05T13:00:00.123456Z`（夹具撤回时刻带纳秒尾巴，证走的是登记册精度）。
  - `TestFormLoadAssignmentAlreadyFormedKeepsItsShapeForALiveChainHead`：首版、变化版各作链头一个子测，答复键集恰为 `assignedAt,assignment,members,outcome,schedule,version`。
- 既有 `segment_operations_test.go` 的 `TestFormLoadAssignmentRegistersTheVersion`、`TestFormLoadAssignmentAnswersAlreadyFormedWithTheChainHead` 未动，照绿。

### 先红后绿
- 红（票 12 之后、本票改代码之前 + 新用例）：`/tmp/claude-1000/-home-tops-workspace-idp-parcel/39d8f2f8-cde5-4dd1-bb34-aef8ce7c660c/scratchpad/tfa13-red.log`。原话：「链头是撤回版，答复却没有撤回标记：{"outcome":"LOAD_ASSIGNMENT_ALREADY_FORMED","assignment":"SYN-LA-0001","schedule":"SYN-SCHEDULE-1","members":["SYN-PARCEL-1","SYN-PARCEL-2"],"version":"SYN-LAV-2","assignedAt":"2026-09-05T12:00:00Z"}」。形状用例改代码前就绿：它守的是「不变」，是护栏不是红用例。
- 绿：`/tmp/claude-1000/-home-tops-workspace-idp-parcel/39d8f2f8-cde5-4dd1-bb34-aef8ce7c660c/scratchpad/tfa13-green.log`。

### 门
- 钉 `087ceb95`：改动的 `.go` 跑 `gofmt -l` 无输出；`go build ./...`、`go vet ./...` 退 0（`/tmp/claude-1000/-home-tops-workspace-idp-parcel/39d8f2f8-cde5-4dd1-bb34-aef8ce7c660c/scratchpad/tfa-gates-build.log`）。
- 带 DSN `go test -p 1 -count=1 ./internal/transportfulfillment/... ./cmd/parcel-api/... ./internal/architecture/...` 全 ok（`/tmp/claude-1000/-home-tops-workspace-idp-parcel/39d8f2f8-cde5-4dd1-bb34-aef8ce7c660c/scratchpad/tfa-gates-test.log`）。未跑全仓全量（属推送方）。
- 机制清点重生成：只有测试文件计数变（`c256ac7e`），生产文件不变。

### 越出地盘
无。新测试文件属「及其测试」；它复用 `segment_operations_test.go` 里的登记册替身与 intake 替身，没改那个文件。

证据层级：`S`（`SYN-` 值，替身登记册）。

### 未做
- `cmd/parcel-api/load_assignment_single_chain_test.go` 的 `TestTheLoadAssignmentFaceAnswersAlreadyFormedAfterAWithdrawnChain` 在真渠道真库上已走到撤回版链头，但没给它加撤回两键的断言——那个文件不在本票地盘，需要时另起。
- 非作者评审、推送方干净检出全量：属会话与推送方。

## Comments

### 评审 ← 会话 idp-parcel-9b 读 diff 审（不算非作者评审；2026-10-19，钉 `ea540e94`）

两轴合审，0 阻断。

- **Spec**：`withdrawn` / `withdrawnAt` 成对，撤回版在场、非撤回版整键缺席；非撤回链头答复逐字节不变（守形用例修前即绿，作守护）；时刻走本包 `rfc3339()`，夹具带纳秒尾巴证了登记册精度。编排不动。
- **Standards**：头注给了缺席而不答 false 的理由与本包先例。
- **非阻断（不另立票）**：真渠道 `TestTheLoadAssignmentFaceAnswersAlreadyFormedAfterAWithdrawnChain` 没断言两新键，该文件在本票地盘外；撤回入口（tf-unwired-seven/05）接上时随那张票补真渠道断言即可。`VERSION_EXISTS` 回显撤回版也会带两键，无专门用例，形状由同一处写出。

### 进 main 记录（2026-10-19，会话 idp-parcel-9b）

- 分支 `claude-tfa121310`（12、13、10 同批）rebase 到远端 main 之上快进推送；rebase 只撞机制清点，在新树上重生成解决（cmd 测试文件 148）。验证钉之后远端只前进了一笔 `apps/admin-web/**`（operator-channel/44）与纯票面，再 rebase 一次，`git diff b09876bf HEAD -- . ':!.scratch' ':!apps/admin-web'` 为空，未重跑。
- 推送方全量一次：钉 `b09876bf` 的隔离 detached 检出，带 DSN `go test -p 1 -count=1 -v ./...` 退 0——有测试的包 143 个 ok、0 FAIL；`--- SKIP` 1（`TestHelperTemplateOwnerProcess`，照设计跳过）。同一检出上 `gofmt -l .` 无输出，`go build ./...`、`go vet ./...` 退 0，机制清点重生成零差。
