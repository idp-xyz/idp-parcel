# 12 装载分配：在变化版上撤回会照抄前版的修订时刻，存库撞 revision_xor_withdrawal

Category: bug
Status: in-progress——2026-10-19 评审 0 阻断（会话读 diff 审，见 Comments），待进 main。此前 in-progress——2026-10-19 作者子代理完工，待评审。此前 in-progress——2026-10-19 会话 `idp-parcel-9b` 认领，分支 `claude-tfa121310`（与 13、10 同一分支）。此前 ready-for-agent——2026-10-19 会话 `idp-parcel-9b` 立（tf-face-algebra/02 实施中发现，票外未改）
Blocked by: 无
父票：[operator-channel/26](../../operator-channel/issues/26-non-author-review-backlog-for-claude-code-landings.md)
出处：tf-face-algebra/02 实施子代理的「本票外的发现」，经其非作者评审确认。
地盘：
- internal/transportfulfillment/domain/load_assignment.go 及其测试
- internal/transportfulfillment/adapters/postgres/load_assignment_registry.go 的真库用例

## 现象

领域 `LoadAssignment.Withdraw` 撤回一个变化版时，把前版的 `revisedAt` 一并照抄进撤回版；迁移 0009 的 `revision_xor_withdrawal` 要求修订与撤回二者只占其一，这一版存库会被 CHECK 拒，整笔事务作废。今天撤回入口没接（tf-unwired-seven/05），所以潜伏。

## 应当

撤回版只带撤回那一组格，不带修订时刻；链上的分配时刻照迁移 0009 头注沿用原分配时刻。

## 完成判据

- [ ] 领域用例：在变化版上撤回，撤回版不带修订时刻。
- [ ] 真库用例：同一条撤回版经 `LoadAssignments.Save` 落库成功、读回一致。
- [ ] 排在 tf-unwired-seven/05 接撤回入口之前修好。
- [ ] Go 门禁照常。

## 完成记录（2026-10-19，会话 idp-parcel-9b 的实施子代理，分支 claude-tfa121310）

基线 `a2d6a4bf`。提交：`299682ef`（修复与用例）、`c256ac7e`（机制清点，12、13、10 合一笔）。

### 落点
- `internal/transportfulfillment/domain/load_assignment_movement.go` 的 `LoadAssignment.Withdraw`：值拷贝之后清掉 `revisedAt`，撤回版只带撤回两件与前版引用；分配时刻沿用原分配时刻（迁移 0009 头注），对象范围随前版。重建门 `RehydrateLoadAssignment` 与库面 `load_assignment_revision_xor_withdrawal` 本就拒「同时是变化与撤回」，未动。
- 生产代码里今天没有 `Withdraw` / `ReviseMembers` 的调用方（撤回与变化入口仍未接），本修复落在入口接上之前，判据「排在接撤回入口之前修好」由此满足。

### 各格对应用例
- 领域：`internal/transportfulfillment/domain/load_assignment_movement_test.go` 的 `TestWithdrawingARevisedAssignmentCarriesNoRevisionTime`——首版 → 变化版 → 撤回版：撤回版 `RevisedAt()` 缺席，撤回两件与时刻对，回指变化版，分配时刻是原分配时刻，对象范围随变化版；变化版自己的修订时刻不被改。
- 真库：`internal/transportfulfillment/adapters/postgres/load_assignment_registry_test.go` 的 `TestAWithdrawalOfARevisedVersionSavesAndReadsBack`——三版各一笔事务经 `LoadAssignments.Save` 落库，`FindByKey` 读回撤回版逐格同上，`FindChainHead` 交回撤回版 v3。

### 先红后绿
- 红（基线代码 + 新用例）：`/tmp/claude-1000/-home-tops-workspace-idp-parcel/39d8f2f8-cde5-4dd1-bb34-aef8ce7c660c/scratchpad/tfa12-red.log`。领域：「撤回版带了前版的修订时刻 2026-08-12 11:00:00 +0000 UTC——修订与撤回只占其一」；真库：「事务内写入失败：save load assignment: ERROR: new row for relation "load_assignment" violates check constraint "load_assignment_revision_xor_withdrawal" (SQLSTATE 23514)」。
- 绿：`/tmp/claude-1000/-home-tops-workspace-idp-parcel/39d8f2f8-cde5-4dd1-bb34-aef8ce7c660c/scratchpad/tfa12-green.log`。

### 门
- 钉 `087ceb95`：改动的 `.go` 跑 `gofmt -l` 无输出；`go build ./...`、`go vet ./...` 退 0（`/tmp/claude-1000/-home-tops-workspace-idp-parcel/39d8f2f8-cde5-4dd1-bb34-aef8ce7c660c/scratchpad/tfa-gates-build.log`）。
- 带 DSN `go test -p 1 -count=1 ./internal/transportfulfillment/... ./cmd/parcel-api/... ./internal/architecture/...` 全 ok（`/tmp/claude-1000/-home-tops-workspace-idp-parcel/39d8f2f8-cde5-4dd1-bb34-aef8ce7c660c/scratchpad/tfa-gates-test.log`）。未跑全仓全量（属推送方）。
- 机制清点重生成：只有测试文件计数变（`c256ac7e`），生产文件不变。

### 越出地盘
无。只动了地盘里的领域文件、领域测试与真库测试。

证据层级：`S`（真库用例取 `SYN-` 值；领域用例沿用本文件既有夹具）。

### 未做
- 非作者评审、推送方干净检出全量：属会话与推送方。

## Comments

### 评审 ← 会话 idp-parcel-9b 读 diff 审（不算非作者评审；2026-10-19，钉 `ea540e94`）

两轴合审，0 阻断。

- **Spec**：撤回版在值拷贝后清掉修订时刻，分配时刻与成员照前版，回指前版；领域用例与经 `LoadAssignments.Save` 的真库往返（含 `FindChainHead` 取到撤回版）各一条，真库红在 `load_assignment_revision_xor_withdrawal`，正是票面现象。
- **Standards**：注释写为什么（重建门与库面 CHECK 都会拒），不写经过。
