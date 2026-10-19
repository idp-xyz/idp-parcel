# 10 建派送任务的答复不回显 conditions：票 20 判「回显让调用方自己比」够用，所凭的回显缺一格

Category: bug
Status: needs-triage——2026-10-18 会话 `idp-parcel-9b` 立（操作者与集成客户端渠道逐口核代数研判；实测发现未经对抗核验，裁定待做，动手前先复现）
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
- [ ] HTTP 单测：201 与 `DISPATCH_TASK_ALREADY_OPEN` 两种答复都带 `conditions`；整秒时刻格式逐字节不变。
- [ ] 真渠道真库回归（`cmd/parcel-api` 新测试文件，`buildOperatorDecisionIntakes` + 生产准入桥 + `buildSegmentOperations`）：以 conditions=A 建任务；同一任务以 conditions=B 重投，答 200 `DISPATCH_TASK_ALREADY_OPEN`，回显 conditions=A，库不变。
- [ ] `go build ./...`、`go vet ./...` 退出码 0，`gofmt -l .` 无输出；带 DSN 跑 `go test -p 1 -count=1 ./internal/transportfulfillment/... ./cmd/parcel-api/...`，全部 ok。
- [ ] 票 15 补记下追加回写行（只加行）。
- [ ] 走 Go 那条路（[并行会话](../../../docs/agents/parallel-sessions.md)）：作者自验，非作者评审（两轴分开），推送方在干净检出上带 DSN 跑全量。
