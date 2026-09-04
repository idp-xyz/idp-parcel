# 21 有效时间显式判断的在线面：所有者今天没有地方就一条事实说「从何时起有效」

Category: enhancement
Status: in-progress——MCP-5（2026-09-04，基线 `a17bfac`，隔离 worktree 分支 `mcp5-lc19-21`；随票 `19` 同分支）
Blocked by: 无（`16` 已 resolved）

## 缺口

ADR-0102 决定三给有效时间两种来源，第一种是「所有者就这一条显式给出」。票 `16` 落了它的应用入口
`JudgeEffectiveTimeHandler.Judge`（指名事实 + 有效时间 → 新版本回指前版 → 交 VE），**但没有在线面**：
没有端点、没有管理台写面，也没有一个列出「待判断事实」的读面让人知道该判哪几条。与票 `17` 同一条理由
先有事实再谈面——事实现在有了。

## 做什么

1. 读面：按（租户，轨迹源）列出有效时间待判断的事实（`effective_basis = 'PENDING'` 且未被回指的当前版），
   带源事件、状态词、发生时间、接收时间，供判断人看。落 TF `adapters/http` 与管理台页，形状照既有的
   TF 读面（`query_transport_fulfillment_records.go`）。
2. 写面：一条事实一次判断，调 `JudgeEffectiveTimeHandler.Judge`；结果代数四格（已判断／已按同值判过／未受理／
   未决）各有 HTTP 落点。操作者身份走 ADR-0100 的管理台接入面。
3. 载荷形状按 ADR-0101 由本票裁（逐条判断，不是批量导入）。

## 红线

- 面上不解释状态词，不建议一个「推荐的有效时间」——那是替所有者判断。
- 已判断过的版本可以再判（形成新版本），但面上要把「这一条已经判过、按哪个依据判的」摆出来。
- 不动 `16` 的领域与应用层；端点表在 `cmd/parcel-api` 加行时与占号的会话互报。

## 完成判据

读面与写面各有实现与测试，能在管理台上把一条待判断事实判成已判断并看到它进 VE 投影；
`gofmt -l` 空、`go build`/`go vet` 退 0、`go test -count=1 ./...` 绿。

## 参照

ADR-0102 决定三；票 `16` 完成记录「刻意留下的三格」第 3 条；
`internal/transportfulfillment/application/judge_external_tracking_effective_time.go`；ADR-0100、ADR-0101。
