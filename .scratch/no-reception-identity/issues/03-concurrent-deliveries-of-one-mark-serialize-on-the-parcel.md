# 03 一码两物并发送站：两笔事务互相看不到对方未提交的关联，两件可能都关联成立

Category: bug
Status: in-progress——2026-10-19 评审 0 阻断（会话读 diff 审，见 Comments），待进 main。此前 in-progress——2026-10-19 作者子代理完工，待评审。此前 in-progress——2026-10-19 会话 `idp-parcel-9b` 认领（no-reception-identity/01 已进 main），分支 `claude-no4503`（与 operator-channel/45 同一分支，先 45 后 03）。此前 ready-for-agent——2026-10-19 会话 `idp-parcel-9b` 立并代决（票 no-reception-identity/01 评审遗留）
Blocked by: no-reception-identity/01 进 main
父票：[01](./01-one-barcode-on-two-units-forms-an-identity-conflict.md)
出处：ADR-0196「越权风险点」中「并发未串行」那一条；票 01 实施子代理回报与评审记录。
地盘：
- internal/nodeoperations/adapters/postgres/reception.go 及其测试
- internal/nodeoperations/application/receive_delivered_unit.go（只在端口语义要改时）
- cmd/parcel-api 下新起一份真库并发用例文件

代决依据：用户 2026-10-16 授权会话 `idp-parcel-9b`「你作为业务和系统专家你来自决」。

## 现象（读码，未实测）

票 01 让收寄编排在身份核对唯一指向一件包裹时，先经 `ReceptionStore.FindOtherUnitIntakes` 问这件包裹是否已关联到另一件作业实物。两件贴同一个号的实物在两笔并发事务里送站时，READ COMMITTED 下各自的读都看不到对方尚未提交的关联行，两件都答`节点收寄已形成`、都关联到同一件包裹、都不记身份冲突——正是票 01 要堵的那一格，只是挪进了竞态窗口。

## 裁定（2026-10-19，会话 idp-parcel-9b 代决；机制，不涉租户取值）

按（租户、包裹关联）在收寄事务里取事务级咨询锁，把同一件包裹的「读别的实物关联 → 落收寄」串行起来：

- 锁在 PG 适配器 `FindOtherUnitIntakes` 里、查询之前取（`pg_advisory_xact_lock`，键由租户与包裹关联派生；派生方式与本仓已有的咨询锁键口径对齐，先 grep 看有没有现成的）。锁随事务结束释放。READ COMMITTED 下取锁之后的那条查询拿新快照，看得到先到者已提交的关联。
- 端口头注写明：这个读要在收寄所在的事务里调，读完到落册之间同一包裹串行。替身不必模拟锁。
- 不取：唯一索引（同一包裹允许多行关联——同一实物多次收寄是合法的，没有可建唯一约束的列组合）；`SERIALIZABLE`（整条收寄编排换隔离级别，牵连 PS 读口与意图入队）。

越权风险点：代 NO owner 定了串行粒度是（租户、包裹）。号码唯一指向的包裹之外（零候选、多候选）不取锁，那两格不读别的实物关联。

## 完成判据

- [x] cmd 真库并发用例：两件实物贴同一个号，两条连接各开事务，A 读完别的实物关联后、落册前挂起，B 送站；B 在锁上等，A 提交后 B 答待识别、记身份冲突，A 那件被追加一版冲突判断。修前同一用例红（两件都 `INTAKE_FORMED`）。等锁的观察照 tf-face-algebra/05 的做法：池外独立连接看 `pg_stat_activity` 的等锁，带超时。
- [x] 适配器用例：锁键对不同包裹、不同租户互不阻塞。
- [x] `go build ./...`、`go vet ./...` 退 0，`gofmt -l .` 无输出；带 DSN 跑 `internal/nodeoperations/...`、`cmd/parcel-api`、`cmd/parcel-frontline-import`、`internal/architecture` 全绿；机制清点重生成。
- [x] ADR-0196 文末追加复核记录：越权风险点「并发未串行」由本票收住（决定原文不动）。
- [ ] 走 Go 那条路（[并行会话](../../../docs/agents/parallel-sessions.md)）：分支、作者自验、评审、推送方在干净检出上带 DSN 跑全量。作者半边（分支、作者自验）已做，评审与推送方全量未到。

## 完成记录（2026-10-19，会话 idp-parcel-9b 的作者子代理，分支 claude-no4503，叠在 operator-channel/45 之后）

**落点**

- PG 适配器 `internal/nodeoperations/adapters/postgres/reception.go` 的 `Receptions.FindOtherUnitIntakes`：查询之前取 `pg_advisory_xact_lock(hashtext('node_operations.reception.parcel_association'), hashtext(租户 || chr(31) || 包裹关联))`。派生口径对齐 `internal/accessidentity/adapters/postgres` 的 `lockMappingKey`：类键取 hashtext 表意名，键取各维以 chr(31) 拼接后的 hashtext。执行器由 `ReadExecutor` 改为 `RequireExecutor`，无事务即拒。
- 端口头注 `internal/nodeoperations/ports/ports.go` 的 `ReceptionStore.FindOtherUnitIntakes`：这个读要在收寄落册所在的事务里调，从读到事务结束同一租户同一包裹串行；单线程替身不必模拟。
- 编排 `receive_delivered_unit.go` 未动：端口语义只多了「在事务里调」，两条接线（`transactionalReception`、前线导入逐行一笔）本就如此。
- 真库并发用例借票 45 拆出的 `buildReceptionDeps` / `receptionWithin`：A 只把收寄库换成读完别的实物关联后挂住的那一层，B 用 `buildReceptionOrchestration` 原样装出的生产编排。

**作者自定的取舍**

1. **读口改 `RequireExecutor`。** 裁定只说「在收寄事务里取锁」。事务外取 `pg_advisory_xact_lock` 不报错，锁在那条语句结束就放掉，读完到落册之间不再串行，是静默失效。改成无事务即拒，契约由适配器自己守；PBC-08 门禁要的同包负向证据由 `TestTheParcelLockedReadRefusesToRunOutsideATransaction` 补上。既有用例 `TestOtherUnitIntakesAreFoundByTheirParcelAssociation` 的三次读随之各包一笔事务，断言不变。
2. **等锁观察只认咨询锁**（`wait_event_type = 'Lock' AND wait_event = 'advisory'`），比 tf-face-algebra/05 的「任意锁等待」窄一格，免得别的锁等待冒充包裹锁。用例还断言 B 确实等过（`waited`）：只看结果的话，一次没走到竞争路径的运行也会碰巧绿。
3. **适配器用例「互不阻塞」用探针事务的 `SET LOCAL lock_timeout = '300ms'` 量**，同租户同包裹作正对照（必须等到 55P03）。没有正对照，修前锁根本不存在时「互不阻塞」也恒真。
4. 新的并发用例点 `AT-NO-019`，注「并发半边」；`.scratch/at-coverage-inventory.md` 的 `AT-NO-019` 一行照扫描口径补上这个用例。

**各格对应用例**

| 格 | 用例 |
|---|---|
| 一码两物并发：A 挂起时 B 等在包裹锁上（池外连接看 `pg_stat_activity`，带超时），A 提交后 B 答待识别、记身份冲突、候选是那件包裹；A 答 INTAKE_FORMED、原判断原样；两件的身份一格都含身份冲突（A 那件被追加了冲突版） | `cmd/parcel-api/reception_one_mark_concurrent_test.go::TestConcurrentDeliveriesOfOneMarkSerializeOnTheParcel` |
| 锁键对不同包裹、不同租户互不阻塞；同租户同包裹等锁 | `internal/nodeoperations/adapters/postgres/reception_test.go::TestTheParcelLockSerializesOnlyTheSameTenantAndParcel` |
| 读口无事务即拒 | `internal/nodeoperations/adapters/postgres/reception_test.go::TestTheParcelLockedReadRefusesToRunOutsideATransaction` |
| 票 01 的串行一码两物与读口语义照旧 | `TestTheWiredReceptionTurnsOneMarkOnTwoUnitsIntoAnIdentityConflict`、`TestOtherUnitIntakesAreFoundByTheirParcelAssociation` 照旧绿 |

**先红后绿**

- 红（钉 `d11a94d5`，即票 45 两笔之上，加新用例、改代码前）：`/tmp/claude-1000/-home-tops-workspace-idp-parcel/39d8f2f8-cde5-4dd1-bb34-aef8ce7c660c/scratchpad/nri03-red.log`。现象原话：并发用例先记「另一方没被挡住，已走完」，再红于「两件 outcome = INTAKE_FORMED / INTAKE_FORMED, want INTAKE_FORMED / PENDING_IDENTIFICATION——两笔事务互相看不到对方的关联」；适配器用例红于「同租户同一件包裹的读 err = <nil>, want 等在锁上直到 lock_timeout（55P03）」与「无事务按包裹读应返回 ErrTransactionRequired，实得：<nil>」。
- 绿：`/tmp/claude-1000/-home-tops-workspace-idp-parcel/39d8f2f8-cde5-4dd1-bb34-aef8ce7c660c/scratchpad/nri03-green.log`，并发用例记「另一方等在包裹锁上，放行」。
- 连跑：`-count=5` 跑两条并发用例各五次全绿，见 `/tmp/claude-1000/-home-tops-workspace-idp-parcel/39d8f2f8-cde5-4dd1-bb34-aef8ce7c660c/scratchpad/nri03-count5.log`；另加 `-race -count=2` 跑两条并发用例与票 45 的回滚用例，退 0。

**门**（钉代码笔 `89bbb2aa`）：`gofmt -l .` 无输出；`go build ./...`、`go vet ./...` 退 0；带 DSN `go test -p 1 -count=1 ./internal/nodeoperations/... ./cmd/parcel-api/... ./cmd/parcel-frontline-import/... ./internal/architecture/...` 退 0，11 个包 ok。机制清点在干净检出上重生成，单独一笔提交（`cmd/` 测试 147→150，本分支三份新 cmd 测试文件，两票合计）。

**证据层级**：`S`（隔离合成夹具，`SYN-` 值）。

**ADR**：ADR-0196 文末追加「复核记录（2026-10-19，决定原文不动）」两条：越权风险点 4 由本票收住；串行粒度（租户、包裹）为代 NO owner 定，以及不取唯一索引与 `SERIALIZABLE` 的理由。决定原文与越权风险点原文未动。

**越出地盘**：`internal/nodeoperations/ports/ports.go`（端口头注，裁定点名要写）；`docs/adr/0196-…`（复核记录，判据点名）；`.scratch/at-coverage-inventory.md`（`AT-NO-019` 一行）；`docs/product/MECHANISM-INVENTORY.md`（重生成）。真库并发用例所用的装配缝 `buildReceptionDeps` / `receptionWithin` 是票 45 那一笔拆的。

**没做**

- 锁不管绕过本读口的写入（直插收寄册），与 access_identity、visibilityexception 两处咨询锁同一边界。
- 零候选、多候选两格不取锁，裁定写明，不读别的实物关联。
- 「走 Go 那条路」的评审与推送方全量未到。

## Comments

### 评审 ← 会话 idp-parcel-9b 读 diff 审（不算非作者评审；2026-10-19，钉 `32b14c73`）

两轴合审，0 阻断。

- **Spec**：锁取在 `FindOtherUnitIntakes` 查询之前，键为（租户、包裹关联），口径同 access_identity 的 `lockMappingKey`；改走 `RequireExecutor`，事务外调用报 `ErrTransactionRequired` 而不是静默失去串行（作者自定，认可——两条接线都在事务里，全仓无事务外调用方）。真库并发用例修前红在「另一方没被挡住」与两件都 `INTAKE_FORMED`，修后观察到等在包裹锁上；适配器用例以 `lock_timeout` 证不同包裹、不同租户互不阻塞，同包裹撞 55P03 作正对照。连跑五次绿。
- **Standards**：端口头注写明须在收寄事务里调；ADR-0196 文末追加复核记录，决定原文不动。
- 零候选与多候选两格不取锁，裁定越权风险点已写。
