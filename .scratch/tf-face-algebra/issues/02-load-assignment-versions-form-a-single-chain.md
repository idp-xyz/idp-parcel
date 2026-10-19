# 02 装载分配登记口能在同一分配下另起并列首版：版本链成了森林，抢占变化入口要用的版本号，撤回之后还能起活跃新根

Category: bug
Status: needs-triage——2026-10-18 会话 `idp-parcel-9b` 立（操作者与集成客户端渠道逐口核代数研判；实测发现未经对抗核验，裁定待做，动手前先复现）
Blocked by: 无
父票：[operator-channel/26](../../operator-channel/issues/26-non-author-review-backlog-for-claude-code-landings.md)
出处：逐口核研判（workflow review26-face-algebra-sweep）：/transport-fulfillment-load-assignment-registrations
地盘：
- internal/transportfulfillment/application/form_load_assignment.go 及其测试
- internal/transportfulfillment/ports/load_assignment.go
- internal/transportfulfillment/adapters/postgres/load_assignment_registry.go 及其测试
- internal/transportfulfillment/adapters/http/form_load_assignment.go 及其测试
- migrations/transport_fulfillment/（新迁移，序号取落地时的下一个）
- cmd/parcel-api/（新增真库回归测试文件）

代决依据：用户 2026-10-16 授权会话 `idp-parcel-9b`「你作为业务和系统专家你来自决」。

## 现象
- **同一分配可起两个首版。** 先投 v1（班次 S1，分配时刻 08:00），再投 v2（换成班次 S2，分配时刻早一天多，成员也不同），两次都答 201 `LOAD_ASSIGNMENT_FORMED`。在册两版的 `corrects_version` 都是 NULL，班次与分配时刻各不相同。
- **抢占领域「变化」要用的版本号。** 领域「变化」要形成的版本（`corrects=v1`）如果恰好也用 v2，存进生产登记册会答「版本已在册」：这个版本号已被本口占掉。
- **撤回之后还能起活跃新根。** 用生产登记册存入领域 `Withdraw` 形成的撤回版（模拟撤回入口接上之后的情形），本口再投 v3，答 201：形成一个 `withdrawn=false`、`corrects_version` 为 NULL、分配时刻早于 v1 的活跃首版。
- 同版本异内容答 200 `LOAD_ASSIGNMENT_VERSION_EXISTS` 并回显原版。这一半照票 20 判据已够用，不在本票范围。

## 复现（S）
命令：在 /tmp/claude-1000/-home-tops-workspace-idp-parcel/39d8f2f8-cde5-4dd1-bb34-aef8ce7c660c/scratchpad/wt-decide2 下执行 `IDP_PARCEL_POSTGRES_DSN='postgres://parcel:parcel@127.0.0.1:55432/postgres?sslmode=disable' go test -overlay <overlay> -count=1 -p 1 -run '<用例>' -v ./cmd/parcel-api/`。端点用生产构造 `tfhttp.NewFormLoadAssignmentEndpoint(decisions.transport, buildSegmentOperations(db).assign)`，准入走生产准入桥。

探针（各自同目录的 overlay.json）：
- /tmp/claude-1000/-home-tops-workspace-idp-parcel/39d8f2f8-cde5-4dd1-bb34-aef8ce7c660c/scratchpad/decide2/transport-fulfillment-load-assignment-registrations/adv-authority/adv_authority_probe_test.go 的 `TestAdvLoadAssignmentRegistryTakesALinkedSuccessorAndTheFaceForksTheChain`（日志 run-adv.log）。
- /tmp/claude-1000/-home-tops-workspace-idp-parcel/39d8f2f8-cde5-4dd1-bb34-aef8ce7c660c/scratchpad/decide2/transport-fulfillment-load-assignment-registrations/adv-authority2/adv2_authority_probe_test.go 的 `TestAdv2FaceFormsASecondRootForAnExistingAssignment`、`TestAdv2FaceFormsAnActiveRootOnAWithdrawnAssignment`（日志 run-adv2.log）。
- /tmp/claude-1000/-home-tops-workspace-idp-parcel/39d8f2f8-cde5-4dd1-bb34-aef8ce7c660c/scratchpad/decide2/transport-fulfillment-load-assignment-registrations/adv-impl/adv_impl_probe_test.go 的 `TestAdvImplTheFaceFormsAParallelRootAfterAWithdrawnChain`。

## 应当
本口的写法与本上下文对「一次分配」的成文口径相左。成文口径是：
- `LoadAssignmentKey` 头注：「变化与撤回都形成新版本、原版本原样保留……所以「一次分配」在库里是一条版本链而不是一行会变的记录」；
- 迁移 0009：「assigned_at 在整条链上不变：变化与撤回沿用原分配时刻」；
- `LoadAssignment.Withdraw`：「已撤回的分配不再变化——后续要装载只能是新的分配」。

而 `FormLoadAssignmentHandler.Form` 的注释是：「换对象范围要换版本号，那是变化不是重投」。

推荐取法（待 TF owner 裁）：
- 本口只形成一次分配的**首版**。该分配已有任何版本时，对指名的新版本号答一个新格（如「分配已形成」，200，回显链头），不落库，也不另起根。
- 变化与撤回走领域 `ReviseMembers` / `Withdraw` 的入口。tf-unwired-seven/05 记着：「变化（`ReviseMembers`）与撤回（`Withdraw`）的**入口**没有接」。

备选：本口把「换版本号」当作变化。那就必须回指链头、沿用分配时刻，等于把变化入口并进本口，要改命令形状（加前版格），一并由 TF owner 裁。

## 根因
- 幂等键（租户、分配、版本）只防同一版本重复，不防同一分配另起根；`LoadAssignmentRegistry` 也没有按分配读链头的口。
- 处理器注释与端口、迁移、领域对「换版本号」的说法不一致，而且没有任何票裁过。

## 修复方向（机制，按推荐取法）
1. `ports.LoadAssignmentRegistry` 加按（租户、分配）读链头的方法，并补 PG 实现。
2. `FormLoadAssignmentHandler.Form`：按键查不到时，再查该分配有无版本；有则答新格并带链头，不 `Save`。
3. 库面兜底并发：
   - 新迁移给 `transport_fulfillment.load_assignment` 加部分唯一索引「（tenant_id, assignment_ref）WHERE corrects_version IS NULL」，写法照 0014、0017、0020。
   - 建索引前先查存量里有没有并列首版；有就如实报出，不静默删。
   - `Save` 的 `ON CONFLICT DO NOTHING` 撞上这条索引时，编排按分配读回链头再答新格；不能按本次键读回，那样会读空。
4. HTTP：`writeLoadAssignmentOutcome` 给新格映射 200 并回显链头，时刻格式随票 31。

## 是否潜伏
- 同一分配另起并列首版：已可触发。生产装配不问授权映射，演示部署里配置员已有授予、对照与区间。
- 撤回之后起活跃新根：潜伏。撤回入口未接，今天只有直接写登记册才造得出撤回版。

## 完成判据
- [ ] 编排单测：已有 v1 时投 v2，答新格并回显 v1，不调 `Save`；同版本重放仍答 `LOAD_ASSIGNMENT_VERSION_EXISTS`，`TestASecondFormOfTheSameVersionKeepsTheOriginal` 照过。
- [ ] 真库回归：
  - v1 之后投 v2，答新格，行数不变；
  - 撤回链（经生产登记册存入撤回版）之后再投新版本，答新格；
  - 两份不同版本号的首版并发到达：恰一份 201，另一份答新格并回显赢家，库面只有一个首版。
- [ ] HTTP 真渠道回归（`cmd/parcel-api` 新测试文件，生产端点构造 + 生产准入桥）：上一条三格各答其格。
- [ ] 迁移：干净库上 `migrate` 顺序跑通；存量检查语句与结果写进完成记录。
- [ ] `go build ./...`、`go vet ./...` 退出码 0，`gofmt -l .` 无输出；带 DSN 跑 `go test -p 1 -count=1 ./internal/transportfulfillment/... ./cmd/parcel-api/... ./internal/architecture/...`，全部 ok。
- [ ] 新增生产文件时，在干净检出上重生成 `docs/product/MECHANISM-INVENTORY.md`。
- [ ] 在票 15 补记下追加回写行（只加行）。
- [ ] 走 Go 那条路（[并行会话](../../../docs/agents/parallel-sessions.md)）：作者自验、非作者评审（两轴分开）、推送方在干净检出上带 DSN 跑全量。
