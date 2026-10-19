# 31 TF 命令答复的时刻经登记册精度带小数秒回显：关段与承运商判断只回显到秒，冲突与重放分不出

Category: enhancement
Status: in-progress——2026-10-19 会话 `idp-parcel-9b` 认领（分支 `claude-oc31`；30 已进 main，Blocked by 解除）。此前 ready-for-agent——2026-10-18 会话 `idp-parcel-9b` 立（票 26 序 3 评审遗留；用户授权代决）
Blocked by: operator-channel/30 进 main（要用它的 AtRegistryPrecision）
父票：[26](./26-non-author-review-backlog-for-claude-code-landings.md)。被评的是 [20](./20-tf-closure-dispatch-task-and-pickup-judgment-canonicalization.md)，代码 `857453d8`。
出处：票 20 Comments「评审 ← 会话 idp-parcel-9b 起的隔离子代理 · 钉 `857453d8`」，Spec 非阻断 (2)。取证钉 main `8d22729f`。证据层级 `S`。
地盘：
- internal/transportfulfillment/adapters/http/catalogue_intake.go
- internal/transportfulfillment/adapters/http/close_fulfillment_segment.go
- internal/transportfulfillment/adapters/http/judge_carrier_first_effective_pickup.go
- internal/transportfulfillment/adapters/http/open_dispatch_task.go
- internal/transportfulfillment/adapters/http/trigger_delivery_dispatch.go
- internal/transportfulfillment/adapters/http/judge_effective_time.go
- internal/transportfulfillment/adapters/http/record_movement_fact.go
- internal/transportfulfillment/adapters/http/register_external_carrier_credential.go
- internal/transportfulfillment/adapters/http/register_carrier_master_document.go
- internal/transportfulfillment/adapters/http/form_load_assignment.go
- internal/transportfulfillment/adapters/http/register_effective_time_rule.go
- internal/transportfulfillment/adapters/http/（相应 *_test.go）

代决依据：用户 2026-10-16 授权会话 `idp-parcel-9b`「你作为业务和系统专家你来自决」；裁定经隔离子代理研判、对抗核验与合成（研判轮 review26-followups-decide）。

## 现象

`writeSegmentClosureOutcome` 写的是「`body.ClosedAt = closedAt.UTC().Format(time.RFC3339)`」。`carrierPickupBodyOf` 的 `JudgedAt`、`RecordedAt`、`OccurredAt` 同样按 `time.RFC3339` 输出，都只到秒；链查询口 `/transport-fulfillment-carrier-first-effective-pickups` 也经过它。与此同时，入口按 `time.Parse(time.RFC3339, …)` 收小数秒，承运商判断的比对精确到微秒。

HTTP 探针在 main 上实测：

- 关段：先以 `…13:00:00.250Z` 关段，再以 `.250Z` 与 `.750Z` 各关一次。两次都答 200 `SEGMENT_ALREADY_CLOSED`，`closedAt` 都回显 `2026-09-05T13:00:00Z`。
- 收寄判断：以 `.250Z` 首判，答 201 `PICKUP_FORMED`；同串重放答 200 `ALREADY_RECORDED`；以 `.750Z` 再判答 200 `CONTENT_CONFLICT`。三次 `pickup.occurredAt` 都回显 `2026-09-10T09:00:00Z`。

冲突与重放的回显一模一样。票 20 裁定要这份回显，原话是「差异调用方自己比得出」；只到秒的回显让这一点落空。

本包的写法本身也不一致：命令答复族（地盘所列各文件）用 `time.RFC3339`，TF 目录查阅经 `catalogue_intake.go` 的 `rfc3339` 用 `time.RFC3339Nano`。在全仓 `adapters/http` 的非测试文件里，还按 `time.RFC3339` 回显的只剩 TF 命令答复族与 parcel-shipment 的 `complete_manual_review.go`，其余都用 `RFC3339Nano`。

## 裁定（2026-10-16，会话 `idp-parcel-9b` 代决；机制）

取 `RFC3339Nano`（秒以下按需输出），输出前先经 `AtRegistryPrecision` 折到登记册精度。不取固定 6 位小数，也不维持现状。

1. **TF HTTP 包只留一个时刻格式化助手。** `rfc3339` 改为 `domain.AtRegistryPrecision(at).Format(time.RFC3339Nano)`，`optionalInstant` 照旧经它。这样回显的就是登记册上那一份：首答的记录取自内存（命令时刻与 `Clock.Now()` 都带纳秒），重放的记录取自读回（只到微秒），两者回显同一个串。如果只换成 `RFC3339Nano` 而不先折，同一份记录在首答和重放里会差一截亚微秒尾巴。
2. **命令答复族全部改走这个助手。** 必改的是 `writeSegmentClosureOutcome` 的 `closedAt`，以及 `carrierPickupBodyOf` 的 `occurredAt`、`judgedAt`、`recordedAt`，同一个 body 里不混两种精度。地盘所列其余文件里的 `Format(time.RFC3339)` 在同一张票里一并换掉。这是对票 20 裁定「RFC 3339 UTC，与本包其余时间格同形」的实施细化：下裁定时本包有两套写法，只有改整族，「同形」才有唯一自洽的读法。
3. **各答复形状头注里「时间一律 RFC 3339 UTC」不改。** `RFC3339Nano` 仍是 RFC 3339 UTC。
4. **不取固定 6 位，也不维持。** 固定 6 位在仓里没有先例，而且会在 HTTP 层多出第二处精度定义；先经 `AtRegistryPrecision` 再按 `RFC3339Nano` 输出，结果已精确等于登记册上的值。维持现状，等于让最后一环继续丢掉比对实际用到的那部分精度。
5. **影响面。**
   - 整秒值逐字节不变：小数为零时，`RFC3339Nano` 连小数点一起省去。
   - 亚秒值会多出小数位，末尾的零按需省去（如 `.25Z`）。
   - 管理台有效时间判断页经 `describeJudgmentAnswer` → `basisLine` 读答复里的 `effectiveAt`；`formatInstant`（即 `new Date(value)`）照常解析，显示仍到秒，前端不用改。
   - 集成客户端读答复里的时刻时，须按时刻解析，不要按字串比较。

## 做什么

1. 照裁定 1 改 `rfc3339`。它此后不只服务查阅接入，可以挪到本包一个中性文件里，文件名由实施方定。
2. 照裁定 2 逐个文件替换。
3. 用例见完成判据。如果新增了生产文件，在干净检出上重生成 `docs/product/MECHANISM-INVENTORY.md`，随这一笔提交。

## 不做

- 入口解析不动。
- parcel-shipment 的 `complete_manual_review.go`：同属秒级回显的例外，这里只列出，归 PS owner 处理。
- `cataloguepage.FormatInstant`：它管的是游标的写法，不管答复。

## 完成判据

- [ ] HTTP：以 `.250Z` 关段，再以 `.750Z` 关段，回显 `…13:00:00.25Z`；以同串 `.250Z` 再关，回显相同。
- [ ] HTTP：收寄判断以 `.250Z` 首判，再以 `.750Z` 判，答 `CONTENT_CONFLICT`，`pickup.occurredAt` 回显 `.25Z`，与 `ALREADY_RECORDED` 的回显同为登记册上的值。
- [ ] HTTP：带纳秒的命令，首答（201）与原样重放（200）回显同一个串。收寄判断口与凭证登记口各一例。
- [ ] 现有整秒用例照过（整秒回显逐字节不变）。
- [ ] 在 `internal/transportfulfillment/adapters/http` 的非测试文件里 `grep -rnF 'Format(time.RFC3339)'`，无命中。
- [ ] `go build ./...`、`go vet ./...` 退出码为 0，`gofmt -l .` 无输出；带 DSN 跑 `go test -p 1 -count=1 ./internal/transportfulfillment/... ./cmd/parcel-api/... ./internal/architecture/...`，全部 ok。
- [ ] 走 Go 那条路（[并行会话](../../../docs/agents/parallel-sessions.md)）：从当时的 `origin/main` 起分支、作者自验、非作者评审（空闲通道，或 `/code-review` 的隔离子代理，两轴分开）、推送方在干净检出上带 DSN 跑全量。
