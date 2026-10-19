# 01 TF 构造门按登记册存得下的值收：零时刻之后不足一微秒的业务时间写得进、读不回，引用里的 NUL 答 500

Category: bug
Status: needs-triage——2026-10-18 会话 `idp-parcel-9b` 立（操作者与集成客户端渠道逐口核代数研判；实测发现未经对抗核验，裁定待做，动手前先复现）
Blocked by: 无
父票：[operator-channel/26](../../operator-channel/issues/26-non-author-review-backlog-for-claude-code-landings.md)
出处：逐口核研判（workflow review26-face-algebra-sweep）：/transport-fulfillment-load-assignment-registrations, /transport-fulfillment/movement-facts, /transport-fulfillment/delivery-attempts
地盘：
- internal/transportfulfillment/domain/load_assignment_movement.go
- internal/transportfulfillment/domain/offsite_pickup.go（`newRequiredValue`）
- internal/transportfulfillment/domain/（上两文件的测试，宜新增测试文件）
- internal/transportfulfillment/adapters/postgres/（新增真库回归测试文件）
- cmd/parcel-api/（新增真渠道回归测试文件）

代决依据：用户 2026-10-16 授权会话 `idp-parcel-9b`「你作为业务和系统专家你来自决」。

## 现象

### 一、零时刻之后不足一微秒：写得进、读不回
- 装载分配：`assignedAt` 取 `0001-01-01T00:00:00.000000001Z`、`…00.000000999Z` 或 `0001-01-01T14:00:00.0000005+14:00` 时：
  - 首投答 201 `LOAD_ASSIGNMENT_FORMED`，版本行与成员行照落，库面 `assigned_at` 存成 `0001-01-01T00:00:00Z`。
  - 此后原样重放与同身份异内容都答 200 `LOAD_ASSIGNMENT_UNDECIDED`，续办引用每次相同。
  - 生产 `LoadAssignments.FindByKey` 报「find load assignment: transport fulfillment: invalid load assignment」。
  - 恰为零点答 `INPUT_NOT_ACCEPTED`；零点后恰一微秒、零点前一纳秒都照常。
- 移动事实：`occurredAt` 取 `0001-01-01T00:00:00.0000001Z` 时，首投答 201 `MOVEMENT_FACT_RECORDED`，库面 `occurred_at=0001-01-01T00:00:00Z`；原样重放答 200 `MOVEMENT_FACT_UNDECIDED`；`MovementFacts.FindByKey` 报「invalid movement fact」。
- 两册都只插不改，也没有撤回入口，所以这个身份此后永远答`未决`，产品内修不回来。

### 二、引用里带 NUL：答 500
- 装载分配：版本带 NUL、新身份的成员带 NUL，都答 500 `NO_ANSWER_FORMED`。
- 移动事实：地点带 NUL 答 500。直调生产编排得到的是 `MOVEMENT_FACT_UNDECIDED` 带续办，但事务已进中止态，提交时得「commit unexpectedly resulted in rollback」。
- 派送尝试：证据引用带 NUL，两次都答 500，库零行。
- 每次重试都是同一个答复。它属于「改输入」那一格，不是「等恢复」那一格。

## 复现（S）
命令：在只读检出 /tmp/claude-1000/-home-tops-workspace-idp-parcel/39d8f2f8-cde5-4dd1-bb34-aef8ce7c660c/scratchpad/wt-decide2 里执行 `IDP_PARCEL_POSTGRES_DSN='postgres://parcel:parcel@127.0.0.1:55432/postgres?sslmode=disable' go test -overlay <overlay> -count=1 -p 1 -run '<用例>' -v ./cmd/parcel-api/`。pgtest 每例自建物理库；探针不提交。

- 装载分配，零点与 NUL：
  - /tmp/claude-1000/-home-tops-workspace-idp-parcel/39d8f2f8-cde5-4dd1-bb34-aef8ce7c660c/scratchpad/decide2/transport-fulfillment-load-assignment-registrations/probe_load_assignment_test.go 的 `TestProbeLoadAssignmentWrittenButUnreadable`、`TestProbeLoadAssignmentUnreadableCauseIsTheZeroInstant`、`TestProbeLoadAssignmentDatabaseErrorInsideTheTransaction`（同目录 overlay.json）。
  - /tmp/claude-1000/-home-tops-workspace-idp-parcel/39d8f2f8-cde5-4dd1-bb34-aef8ce7c660c/scratchpad/decide2/transport-fulfillment-load-assignment-registrations/adv-impl/adv_impl_probe_test.go 的 `TestAdvImplZeroInstantWindowThroughTheEndpoint`、`TestAdvImplDatabaseErrorBranches`、`TestAdvImplTruncationAgreesWithThePersistedPrecision`（同目录 overlay.json；日志 run-adv-impl.log）。
- 移动事实零点：/tmp/claude-1000/-home-tops-workspace-idp-parcel/39d8f2f8-cde5-4dd1-bb34-aef8ce7c660c/scratchpad/decide2/movement-facts/synth/zz_probe_zero_instant_test.go 的 `TestProbeDecide2MovementFactZeroInstant`（overlay 为 /tmp/claude-1000/-home-tops-workspace-idp-parcel/39d8f2f8-cde5-4dd1-bb34-aef8ce7c660c/scratchpad/decide2/movement-facts/synth/overlay-zero.json；日志 /tmp/claude-1000/-home-tops-workspace-idp-parcel/39d8f2f8-cde5-4dd1-bb34-aef8ce7c660c/scratchpad/decide2/movement-facts/synth/run-zero.log）。
- 移动事实 NUL：/tmp/claude-1000/-home-tops-workspace-idp-parcel/39d8f2f8-cde5-4dd1-bb34-aef8ce7c660c/scratchpad/decide2/movement-facts/probe_movement_fact_test.go 的 `TestProbeMovementFactNulMechanism`（同目录 overlay.json）。
- 派送尝试 NUL：/tmp/claude-1000/-home-tops-workspace-idp-parcel/39d8f2f8-cde5-4dd1-bb34-aef8ce7c660c/scratchpad/decide2/delivery-attempts/adv/probe_da_adv_test.go 的 `TestProbeDAAdvEdges`（同目录 overlay-da-adv.json；日志 run-adv3.log）。

## 应当
- 写得进就读得回。构造门按登记册存得下的值来判：零时刻之后不足一微秒的时刻，在登记册上就是零点，应答`未受理`（`INPUT_NOT_ACCEPTED`），不落库。
- 库收不下的取值，在构造门就拒：引用含 NUL 时答`未受理`（200），不落成 500。
- 依据与票 30 裁定对严格先后门的定性相同：ADR-0031「`error` 留给『这次写入没能完成』」；永远存不进去的输入，应答「改输入」。

## 根因
- 判零按纳秒，存值按微秒：
  - `FormLoadAssignment` 以「`spec.AssignedAt.IsZero()`」守门，`RecordMovementFact` 以「`spec.OccurredAt.IsZero()`」守门，都按纳秒判。
  - 库面 `timestamptz` 写入时截到微秒，存成了零点。
  - `RehydrateLoadAssignment`、`RehydrateMovementFact` 复用同一道构造门（头注「复用它而不是抄一遍」），所以读回即被拒。
  - 对抗核验实测：Go 的 `Truncate(time.Microsecond)` 与 pgx 落库后读回的值逐例相同。
- NUL 不拒：`newRequiredValue` 只拒空白（「`strings.TrimSpace(value) == ""`」），不拒 NUL。PG `text` 收不下 NUL，写入报 SQL 错，事务进中止态；编排折出的`未决`在提交时被回滚吞掉，端点按「编排交回 error」写 500。
- 票 30 的裁定收了「相等类」与「严格先后类」，没有收「判零」这一类。

## 修复方向（机制）
1. 判零按登记册精度：改成 `AtRegistryPrecision(t).IsZero()`，只改判定，不改存值。
   - 装载分配的 `ReviseMembers` / `Withdraw`、移动事实的 `TransportMovementFact.Correct`，时刻门都照同一判据改。
   - 在 `AtRegistryPrecision` 头注那条规则里补一句「时刻非零门同属此类」。这是对那条规则的扩展，不另立第二处定义。
2. `newRequiredValue` 拒含 NUL 的值，各构造门照旧折成`未受理`；错误归哪一类由实施方定。不经 `newRequiredValue` 的自由文本格要逐处核，完成记录按符号名列出。
3. 不放的位置：
   - 不放 HTTP 入口 `parseOptionalInstant`：它多口共用，精度也不属传输层。
   - 不让 PG 适配器拒写：编排会把 error 答成`未决`，落错格。
4. 同形门清点：在 TF `domain` 非测试文件里 `grep -n 'IsZero()'`，逐处看该时刻是否落 `timestamptz`、重建是否复用构造门。本票只改地盘内的两份文件；别处同病的写进完成记录，另立票守地盘。

## 是否潜伏
已可触发。输入虽然离谱，后果却是永久的。
- 装载分配挂运营决定渠道：生产装配上不问授权映射；演示部署三道都开着，即权威串、授予 `SYN-GRANT-DEX-CONFIG-LOAD-ASSIGNMENT-01`、准入对照 `SYN-MAP-OD-LOAD-ASSIGNMENT-01` 与对应治理区间。
- 移动事实、派送尝试挂作业事实登记渠道：编排没有未配置缝，铸造也不判准入；只差租户登记作业事实授予与在用设备。这两样可用随产品发布的 `parcel-access-register` 登，属租户取值。

## 完成判据
- [ ] 领域单测，零点格：装载分配、移动事实各取「零点后 1ns」「零点后 999ns」「`+14:00` 写法零点后 500ns」，答 `ErrInvalidLoadAssignment` / `ErrInvalidMovementFact`。
  - 「零点后 1µs」「零点前 1ns」照常形成；恰为零点照旧拒。
  - 替身需要模拟读回时，用测试里独立写的 `Truncate(time.Microsecond)`。
- [ ] 领域单测，NUL：`newRequiredValue` 对含 NUL 的值报错；前后带空白的非空值，行为不变。
- [ ] 真库回归（`adapters/postgres` 新测试文件，编排加真登记册，同一事务）：
  - 上列零点格答 `INPUT_NOT_ACCEPTED`，版本行、成员行、移动事实行都是零行；
  - 「零点后 1µs」首登后 `FindByKey` 读得回，原样重放答 `…_VERSION_EXISTS`。
- [ ] HTTP 真渠道回归（`cmd/parcel-api` 新测试文件）：
  - 装载分配走生产端点构造、生产准入桥与 `buildSegmentOperations`；移动事实、派送尝试走 `WorkFactCommandIntake` 与生产编排。
  - 零点格答 200 `INPUT_NOT_ACCEPTED`。
  - 装载分配版本带 NUL、移动事实地点带 NUL、派送尝试证据带 NUL，各答 200 `未受理`而不是 500，库零行。
- [ ] 变异自检：把判零改回按纳秒、把 NUL 检查撤掉，对应用例都转红；完成记录写明对哪些调用点做过。
- [ ] 同形门清点结果写进完成记录，用符号名，不计数。
- [ ] Go 门禁：
  - `go build ./...`、`go vet ./...` 退出码 0，`gofmt -l .` 无输出；
  - 带 DSN 跑 `go test -p 1 -count=1 ./internal/transportfulfillment/... ./cmd/parcel-api/... ./internal/architecture/...` 全部 ok，`-v` 下 TF `adapters/postgres` 无 `--- SKIP`。
- [ ] 新增生产文件时，在干净检出上重生成 `docs/product/MECHANISM-INVENTORY.md`，随笔提交。
- [ ] 票 15 补记下追加回写行（只加行）。
- [ ] 走 Go 那条路（[并行会话](../../../docs/agents/parallel-sessions.md)）：
  - 从当时的 `origin/main` 起分支；
  - 作者自验；
  - 非作者评审（空闲通道，或 `/code-review` 的隔离子代理，两轴分开）；
  - 推送方在干净检出上带 DSN 跑全量。
