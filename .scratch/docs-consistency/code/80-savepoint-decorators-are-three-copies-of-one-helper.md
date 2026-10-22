# 80 跨上下文读口的 savepoint 装饰器是同一段代码的三份副本：抽成 `internal/platform` 的泛型助手

Category: enhancement
Status: needs-triage——建议 R3（重构，不改行为）。2026-10-21 协调方据票 [docs-consistency/code/67](./67-a-failed-cost-read-poisons-the-reassessment-transaction.md) 隔离评审立（作者子代理代笔；用户「你作为业务和系统专家自决……解决所有的问题」授权）。
Blocked by: 无
出处：code/67 隔离评审。

## 现象

「在调用方事务里开 savepoint，内层失败退回、错误原样交回，成功 RELEASE；ctx 不在事务里时直通；退不回时两条错误一并交回」这段逻辑，仓里有三份逐行同形的副本：

- NR `internal/networkrouting/adapters/postgres` 的 `SavepointedRouteCandidateCosts`（code/67）；
- NO `internal/nodeoperations/adapters/postgres` 的 `SavepointedServiceResultView`（ADR-0186 决定七）；
- NO 同一包的 `SavepointedParcelIdentityView`（ADR-0199 决定五）。

三份各自持有 `RequireExecutor` 判事务、三条 `Exec`、错误包装与「退不回」那一支。下一处要隔离的读口（code/67 越权风险点 2 的取号、code/78 修法 (a) 要隔的那几口）会再抄一份；哪一份日后修了一处边角（比如 ctx 已取消时 `ROLLBACK TO SAVEPOINT` 的处理），另几份不会跟着改。

## 三类

机制。规模 S。

## 最小的诚实修法

1. 在 `internal/platform` 下立一个泛型助手，形如 `Isolate[T any](ctx context.Context, db *bentopg.DB, name string, fn func(context.Context) (T, error)) (T, error)`，承接上述全部语义（含事务外直通与退不回时的 `errors.Join`）。savepoint 名由调用方给，同一笔事务里不同读口各用各的名字，这条约束写在助手的注释上。
2. 三个装饰器改成薄封装：构造期校验与端口实现留在各自上下文的适配器里，方法体只调助手。装饰器的类型名与构造函数不变，装配点不动。
3. 各装饰器既有的真库用例照旧绿；助手本身补一组真库用例（失败退回后同一事务照常写、可读一支 RELEASE、事务外直通、同一事务里先后失败失败成功），三处装饰器的用例可以收窄到「确实经过了助手」。

## 可选项

加一道 `internal/architecture` 门禁，钉住「候选成本取数侧的装配必须包隔离」：code/67 越权风险点 1 写的是隔离靠装配兑现，编排不校验，新接一条复核或初始路由的装配而忘了包，库里读失败照旧让失效回滚、报成写库故障。门禁形状待定：例如扫 `cmd/` 生产代码里 `nrapplication.ReassessRouteDeps`、`nrapplication.CreateInitialRouteDeps` 字面量的 `Costs` 字段，要求其值来自 `nrpostgres.NewSavepointedRouteCandidateCosts`。按语法认接线，与既有生产接线棘轮同一类做法。做不做、做到多宽（是否连 NO 那两口一并钉），由领票时定。

## 验收

- 三个装饰器的方法体里不再出现 `SAVEPOINT` 字面量；全仓 `SAVEPOINT` 字面量只在助手与 settlementaccounting 自己的撞键处理里。
- NR、NO 两边既有的 savepoint 真库用例、`cmd/parcel-dispatch` 的 `reassess_cost_read_isolation_test.go`、`cmd/parcel-dispatch` 的节点身份重核用例、`cmd/parcel-frontline-import` 与 `cmd/parcel-api` 收寄装配的相关用例照旧绿。

## 相关

- settlementaccounting 的 postgres 适配器里另有一批 `SAVEPOINT` 用法，吞的是本上下文自己的撞键，语义不同（不是隔离跨上下文读），不在本票范围；领票时看一眼能否同样用助手，不强求。
