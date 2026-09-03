# 05 把类型可达性探针做成第二道棘轮：自己的基线文件，探针退役

Category: chore
Status: ready-for-agent
Blocked by: 无（票 04 已裁）

## 要建什么

按票 [04](./04-should-the-ratchet-cover-the-new-family.md) 的裁决：**不动** `production_wiring_ratchet_test.go`
与 `production_wiring_baseline.txt`，在 `internal/architecture/` 下**另立**一道按类型可达性量的棘轮：

1. **量法**照探针 [`.scratch/domain-executor-audit/probe/main.go`](../../domain-executor-audit/probe/main.go)：
   从生产代码（非 `_test.go`）指名的种子出发，沿字段类型与方法签名闭包，闭包外的 `internal/<上下文>/domain`
   导出类型即「零生产消费者」。判据与函数名棘轮互补——它看得见 `New*` 构造的聚合根（`ProductChannelMapping`
   那一类）与 `NewDecimal` 那种命名方向相反的影子。
2. **基线文件** `production_type_reachability_baseline.txt`：冻住首次运行的名单，头部照函数名棘轮基线的写法记
   「在哪个 SHA 的干净检出上量得 N」与每条的理由行；**只许减不许增**，新增条目要在理由行写明它等谁
   （owner 已裁过的写法见函数名棘轮基线对 MCP-6 两条新增的记法）。
3. **首次基线要先去噪**：探针报的 122 是上界，票 04 点名了一类噪声——`Resolution` 那种只由未导出函数产出、
   导出了却只在包内用的类型，那是该改小写的设计瑕疵而非缺执行器。**不要把它们冻进基线**：要么先改小写
   （各上下文地盘，逐个在频道占号），要么在基线里单列一节「待改小写」并写明理由。基线里的每一行都要
   有人能答「它在等什么」。
4. **探针退役**：测试落地后删 `.scratch/domain-executor-audit/probe/`，目录 README 留一句指向本测试。

## 红线

- 不改函数名棘轮的判据与基线（可比性）。
- 不在测试里放任何产品判断：它只报名单变化，不判「等租户」还是「烂了」（与函数名棘轮头注同一立场）。
- 基线数字一律带 SHA 与「在干净检出上量」的注记（`docs/agents/parallel-sessions.md`「数一份共享文件里的条目」节）。

## 验证

`go test ./internal/architecture/ -run TypeReachability -count=1` 在干净检出上绿；人为把一个已接线的领域类型
从生产调用点上摘掉，测试必红并指名该类型；把基线里一条对应的类型接上线后，测试提示「基线可以剪」（与函数名
棘轮同一形状）。`gofmt -l` 空、`go build`/`go vet` 退 0。

## 边界

只加测试与基线文件，不接任何线、不改任何领域类型（改小写属各上下文自己的票）。
