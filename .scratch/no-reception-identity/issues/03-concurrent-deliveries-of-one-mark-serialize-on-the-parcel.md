# 03 一码两物并发送站：两笔事务互相看不到对方未提交的关联，两件可能都关联成立

Category: bug
Status: ready-for-agent——2026-10-19 会话 `idp-parcel-9b` 立并代决（票 no-reception-identity/01 评审遗留）
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

- [ ] cmd 真库并发用例：两件实物贴同一个号，两条连接各开事务，A 读完别的实物关联后、落册前挂起，B 送站；B 在锁上等，A 提交后 B 答待识别、记身份冲突，A 那件被追加一版冲突判断。修前同一用例红（两件都 `INTAKE_FORMED`）。等锁的观察照 tf-face-algebra/05 的做法：池外独立连接看 `pg_stat_activity` 的等锁，带超时。
- [ ] 适配器用例：锁键对不同包裹、不同租户互不阻塞。
- [ ] `go build ./...`、`go vet ./...` 退 0，`gofmt -l .` 无输出；带 DSN 跑 `internal/nodeoperations/...`、`cmd/parcel-api`、`cmd/parcel-frontline-import`、`internal/architecture` 全绿；机制清点重生成。
- [ ] ADR-0196 文末追加复核记录：越权风险点「并发未串行」由本票收住（决定原文不动）。
- [ ] 走 Go 那条路（[并行会话](../../../docs/agents/parallel-sessions.md)）：分支、作者自验、评审、推送方在干净检出上带 DSN 跑全量。
